// Package render plans and applies the files that layered sources contribute
// to a target directory.
package render

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/tmpl"
)

const (
	// dotPrefix marks a path segment that renders with a leading dot instead.
	dotPrefix = "dot-"
	// templateSuffix marks a file that renders as a template to the path
	// without the suffix.
	templateSuffix = ".tmpl"
)

// Status says what applying an entry does to its target.
type Status string

const (
	// StatusNew means the target does not exist yet.
	StatusNew Status = "new"
	// StatusChanged means the target exists with different content, different
	// permission bits, or as something other than a regular file.
	StatusChanged Status = "changed"
	// StatusUnchanged means the target is a regular file with identical bytes
	// and permission bits, so Apply skips it.
	StatusUnchanged Status = "unchanged"
)

// Kind says how an entry's source content becomes its target.
type Kind string

const (
	// KindFile copies the source bytes verbatim.
	KindFile Kind = "file"
	// KindTemplate executes the source as a text/template and writes the
	// output to the target path without the .tmpl suffix.
	KindTemplate Kind = "template"
)

// Request selects what Build plans.
type Request struct {
	// Home is the user's home directory, exposed to templates as .Home
	// whatever the target is.
	Home string
	// Hostname is exposed to templates as .Hostname.
	Hostname string
	// Layers is the complete layer order, usually config.LayerOrder.
	Layers []string
	// OS is exposed to templates as .OS, usually runtime.GOOS.
	OS string
	// Sources are consulted in order within each layer.
	Sources []config.ResolvedSource
	// Target is the absolute directory that receives the files.
	Target string
}

// Origin names one file a source contributes.
type Origin struct {
	// Layer is the layer the package was listed under.
	Layer string
	// Path is the file's path relative to the source root, including the
	// package directory, using forward slashes.
	Path string
	// Source is the source name.
	Source string
}

// Entry is one target file and the source file that produces it.
type Entry struct {
	Origin
	// Content holds the rendered bytes of a template entry; a file entry
	// leaves it nil and streams from SourceFile instead.
	Content []byte
	Kind    Kind
	// Mode holds the permission bits copied to the target.
	Mode fs.FileMode
	// Replaces lists the origins earlier layers contributed for the same
	// target, in layer order.
	Replaces []Origin
	// SourceFile is the absolute path of the file to copy.
	SourceFile string
	Status     Status
	// Target is the absolute path the entry writes.
	Target string
}

// Plan is every file a render would write, sorted by target path.
type Plan struct {
	Entries []Entry
	Layers  []string
	Target  string
}

// CollisionError reports two files in the same layer mapping to one target.
type CollisionError struct {
	First  Origin
	Second Origin
	Target string
}

func (e *CollisionError) Error() string {
	return fmt.Sprintf("layer %q: %s:%s and %s:%s both render to %s",
		e.First.Layer, e.First.Source, e.First.Path, e.Second.Source, e.Second.Path, e.Target)
}

// Build reads every source manifest, walks the packages the requested layers
// list, renders every winning template, and returns the resulting plan
// without touching the target. Later layers replace earlier ones at the same
// target; two contributors within one layer fail with a *CollisionError, and
// a template that fails to parse or execute fails the whole build.
func Build(req Request) (Plan, error) {
	if err := requireDirectory(req.Target); err != nil {
		return Plan{}, fmt.Errorf("target directory: %w", err)
	}

	manifests := make([]config.Manifest, len(req.Sources))
	for i, source := range req.Sources {
		manifest, err := config.LoadManifest(source.Path)
		if err != nil {
			return Plan{}, fmt.Errorf("source %s: %w", source.Name, err)
		}
		manifests[i] = manifest
	}

	entries := make(map[string]Entry)
	for _, layer := range req.Layers {
		owners := make(map[string]Origin)
		for i, source := range req.Sources {
			for _, pkg := range manifests[i].Packages(layer) {
				err := walkPackage(source, pkg, func(path, sourceFile string, mode fs.FileMode) error {
					origin := Origin{Layer: layer, Path: path, Source: source.Name}
					kind, rel, err := classify(pkg, path)
					if err != nil {
						return fmt.Errorf("source %s: %w", source.Name, err)
					}
					target := filepath.Join(req.Target, rel)
					if first, ok := owners[target]; ok {
						return &CollisionError{First: first, Second: origin, Target: target}
					}
					owners[target] = origin

					entry := Entry{Kind: kind, Mode: mode, Origin: origin, SourceFile: sourceFile, Target: target}
					if previous, ok := entries[target]; ok {
						entry.Replaces = append(previous.Replaces, previous.Origin)
					}
					entries[target] = entry
					return nil
				})
				if err != nil {
					return Plan{}, err
				}
			}
		}
	}

	plan := Plan{Entries: make([]Entry, 0, len(entries)), Layers: req.Layers, Target: req.Target}
	for _, entry := range entries {
		plan.Entries = append(plan.Entries, entry)
	}
	sort.Slice(plan.Entries, func(i, j int) bool { return plan.Entries[i].Target < plan.Entries[j].Target })
	ctx := templateContext(req)
	for i := range plan.Entries {
		if plan.Entries[i].Kind == KindTemplate {
			content, err := renderTemplate(plan.Entries[i], ctx)
			if err != nil {
				return Plan{}, err
			}
			plan.Entries[i].Content = content
		}
		status, err := statusOf(plan.Entries[i])
		if err != nil {
			return Plan{}, err
		}
		plan.Entries[i].Status = status
	}
	return plan, nil
}

// Apply writes every entry whose status is not unchanged, in plan order. Each
// file is written to a temporary file in its final directory with the source
// permission bits and then renamed over the target, so a target is never
// left half-written. A failure stops Apply; files already written stay.
func Apply(plan Plan) error {
	for _, entry := range plan.Entries {
		if entry.Status == StatusUnchanged {
			continue
		}
		if err := writeEntry(entry); err != nil {
			return fmt.Errorf("write %s: %w", entry.Target, err)
		}
	}
	return nil
}

// walkPackage calls visit for every file under the package with its
// package-relative slash path, absolute source path, and permission bits.
// Symbolic links to regular files are followed; any other non-regular entry
// is an error.
func walkPackage(source config.ResolvedSource, pkg string, visit func(path, sourceFile string, mode fs.FileMode) error) error {
	root := filepath.Join(source.Path, pkg)
	return filepath.WalkDir(root, func(sourceFile string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("source %s: %w", source.Name, err)
		}
		if d.IsDir() {
			return nil
		}
		info, err := os.Stat(sourceFile)
		if err != nil {
			return fmt.Errorf("source %s: %w", source.Name, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("source %s: %s is not a regular file (%s)", source.Name, sourceFile, info.Mode().Type())
		}
		rel, err := filepath.Rel(source.Path, sourceFile)
		if err != nil {
			return fmt.Errorf("source %s: %w", source.Name, err)
		}
		return visit(filepath.ToSlash(rel), sourceFile, info.Mode().Perm())
	})
}

// classify decides how a source file renders and where: a name ending in
// .tmpl is a template whose target drops the suffix, and anything else is a
// file. A name that is nothing but the suffix has no target name and is an
// error.
func classify(pkg, sourcePath string) (Kind, string, error) {
	stem, isTemplate := strings.CutSuffix(sourcePath, templateSuffix)
	if !isTemplate {
		return KindFile, targetPath(pkg, sourcePath), nil
	}
	name := stem[strings.LastIndex(stem, "/")+1:]
	if name == "" || name == dotPrefix {
		return "", "", fmt.Errorf("%s: a template needs a name before %s", sourcePath, templateSuffix)
	}
	return KindTemplate, targetPath(pkg, stem), nil
}

// targetPath maps a source-relative path inside pkg to its target-relative
// path: the package segment is dropped and every remaining segment that
// starts with "dot-" gets a leading dot instead. A segment that is exactly
// "dot-" stays as it is.
func targetPath(pkg, sourcePath string) string {
	segments := strings.Split(strings.TrimPrefix(sourcePath, pkg+"/"), "/")
	for i, segment := range segments {
		if rest, ok := strings.CutPrefix(segment, dotPrefix); ok && rest != "" {
			segments[i] = "." + rest
		}
	}
	return filepath.FromSlash(strings.Join(segments, "/"))
}

// templateContext builds the data every template in the plan sees.
func templateContext(req Request) tmpl.Context {
	sources := make([]tmpl.Source, len(req.Sources))
	for i, source := range req.Sources {
		sources[i] = tmpl.Source{Name: source.Name, Path: source.Path}
	}
	return tmpl.Context{Home: req.Home, Hostname: req.Hostname, Layers: req.Layers, OS: req.OS, Sources: sources}
}

// renderTemplate executes a template entry's source, naming the template
// source:path so errors point at the file in the checkout.
func renderTemplate(entry Entry, ctx tmpl.Context) ([]byte, error) {
	text, err := os.ReadFile(entry.SourceFile)
	if err != nil {
		return nil, fmt.Errorf("source %s: %w", entry.Source, err)
	}
	content, err := tmpl.Render(entry.Source+":"+entry.Path, text, ctx)
	if err != nil {
		return nil, fmt.Errorf("render %w", err)
	}
	return content, nil
}

func statusOf(entry Entry) (Status, error) {
	info, err := os.Lstat(entry.Target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return StatusNew, nil
	case err != nil:
		return "", fmt.Errorf("check target %s: %w", entry.Target, err)
	case info.IsDir():
		return "", fmt.Errorf("target %s is a directory; %s:%s needs a file there", entry.Target, entry.Source, entry.Path)
	case !info.Mode().IsRegular() || info.Mode().Perm() != entry.Mode:
		return StatusChanged, nil
	}

	want := entry.Content
	if entry.Kind == KindFile {
		want, err = os.ReadFile(entry.SourceFile)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", entry.SourceFile, err)
		}
	}
	have, err := os.ReadFile(entry.Target)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", entry.Target, err)
	}
	if bytes.Equal(want, have) {
		return StatusUnchanged, nil
	}
	return StatusChanged, nil
}

func writeEntry(entry Entry) error {
	dir := filepath.Dir(entry.Target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var source io.Reader = bytes.NewReader(entry.Content)
	if entry.Kind == KindFile {
		file, err := os.Open(entry.SourceFile)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		source = file
	}

	temp, err := os.CreateTemp(dir, "."+filepath.Base(entry.Target)+".overlay-*")
	if err != nil {
		return err
	}
	if err := fillTemp(temp, source, entry.Mode); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	if err := os.Rename(temp.Name(), entry.Target); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	return nil
}

func fillTemp(temp *os.File, source io.Reader, mode fs.FileMode) error {
	if _, err := io.Copy(temp, source); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	return temp.Close()
}

func requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}
