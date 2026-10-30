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
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/merge"
	"github.com/jmcampanini/overlay/internal/tmpl"
)

const (
	// dotPrefix marks a path segment that renders with a leading dot instead.
	dotPrefix = "dot-"
	// fragmentMarker splits a fragment name NAME.olay.LAYER.EXT into the
	// target name NAME.EXT and the layer the fragment belongs to.
	fragmentMarker = ".olay."
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
	// KindMerge composes every active NAME.olay.LAYER.EXT fragment for one
	// target into a single JSON, TOML, or YAML document.
	KindMerge Kind = "merge"
	// KindTemplate executes the source as a text/template and writes the
	// output to the target path without the .tmpl suffix.
	KindTemplate Kind = "template"
)

// Request selects what Build plans.
type Request struct {
	// Home is the user's home directory, exposed to templates as .Home
	// whatever the target is.
	Home string
	// Hostname looks up the machine's hostname for .Hostname. Build calls it
	// once, and only when the plan holds a template, so a render of plain
	// files never depends on the lookup.
	Hostname func() (string, error)
	// Layers is the complete layer order, usually config.LayerOrder.
	Layers []string
	// OS is exposed to templates as .OS, usually runtime.GOOS.
	OS string
	// ProtectedPaths names files a render must never replace, including the
	// configuration files used to select its sources.
	ProtectedPaths []string
	// Sources are consulted in order within each layer.
	Sources []config.ResolvedSource
	// Target is the absolute directory that receives the files.
	Target string
}

// Origin names one file a source contributes.
type Origin struct {
	// Layer is the layer the file applies in: the layer its package was
	// listed under, or for a fragment the layer its name carries.
	Layer string
	// Path is the file's path relative to the source root, including the
	// package directory, using forward slashes.
	Path string
	// Source is the source name.
	Source string
}

// Fragment is one NAME.olay.LAYER.EXT file a merge entry composes.
type Fragment struct {
	Origin
	// SourceFile is the absolute path of the fragment.
	SourceFile string
	// Template reports whether the fragment, named NAME.olay.LAYER.EXT.tmpl,
	// executes as a text/template before it is parsed.
	Template bool
}

// Entry is one target file and the source file that produces it.
type Entry struct {
	Origin
	// Content holds the rendered bytes of a template or merge entry; a file
	// entry leaves it nil and streams from SourceFile instead.
	Content []byte
	Kind    Kind
	// Merges lists every fragment a merge entry composes, in layer order;
	// Origin is the last of them. Other kinds leave it nil.
	Merges []Fragment
	// Mode holds the permission bits copied to the target; a merge entry
	// keeps only the bits every fragment has.
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
// list, renders every winning template and merge, and returns the resulting
// plan without touching the target. A plain file or template applies in its
// package's layer; a fragment named NAME.olay.LAYER.EXT applies in LAYER
// however many layers list its package, and is skipped when LAYER is not in
// req.Layers or the source manifest. A fragment named
// NAME.olay.LAYER.EXT.tmpl executes as a template before it is parsed.
// Targets must stay within req.Target after resolving parent symlinks and
// must not replace ProtectedPaths. Later layers replace earlier ones at the
// same target, except that consecutive fragments merge into one document; a
// fragment above a plain file fails, two contributors within one layer fail
// with a *CollisionError, a merge target NAME.yaml beside any NAME.yml or
// the reverse fails, and a template or fragment that fails to render fails
// the whole build.
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

	contributions := make(map[string][]contribution)
	seenFragments := make(map[string]bool)
	for rank, layer := range req.Layers {
		for i, source := range req.Sources {
			for _, pkg := range manifests[i].Packages(layer) {
				err := walkPackage(source, pkg, func(filePath, sourceFile string, mode fs.FileMode) error {
					class, err := classify(pkg, filePath)
					if err != nil {
						return fmt.Errorf("source %s: %w", source.Name, err)
					}

					appliesIn, appliesAt := layer, rank
					if class.kind == KindMerge {
						if _, declared := manifests[i].Layers[class.layer]; !declared {
							return nil
						}
						fragmentID := source.Name + ":" + filePath
						if seenFragments[fragmentID] {
							return nil
						}
						seenFragments[fragmentID] = true
						appliesIn, appliesAt = class.layer, slices.Index(req.Layers, class.layer)
					}
					if appliesAt < 0 {
						return nil
					}
					if !filepath.IsLocal(class.target) {
						return fmt.Errorf("source %s: %s maps outside the target directory", source.Name, filePath)
					}

					target := filepath.Join(req.Target, class.target)
					contributions[target] = append(contributions[target], contribution{rank: appliesAt, template: class.template, Entry: Entry{
						Kind:       class.kind,
						Mode:       mode,
						Origin:     Origin{Layer: appliesIn, Path: filePath, Source: source.Name},
						SourceFile: sourceFile,
						Target:     target,
					}})
					return nil
				})
				if err != nil {
					return Plan{}, err
				}
			}
		}
	}

	plan := Plan{Entries: make([]Entry, 0, len(contributions)), Layers: req.Layers, Target: req.Target}
	for _, candidates := range contributions {
		entry, err := resolveTarget(candidates)
		if err != nil {
			return Plan{}, err
		}
		plan.Entries = append(plan.Entries, entry)
	}
	sort.Slice(plan.Entries, func(i, j int) bool { return plan.Entries[i].Target < plan.Entries[j].Target })
	if err := rejectMixedYAMLSpellings(plan.Entries); err != nil {
		return Plan{}, err
	}
	if err := validateTargets(req, plan.Entries); err != nil {
		return Plan{}, err
	}

	ctx, err := templateContext(req, plan.Entries)
	if err != nil {
		return Plan{}, err
	}
	for i := range plan.Entries {
		content, err := renderContent(plan.Entries[i], ctx)
		if err != nil {
			return Plan{}, err
		}
		plan.Entries[i].Content = content

		status, err := statusOf(plan.Entries[i])
		if err != nil {
			return Plan{}, err
		}
		plan.Entries[i].Status = status
	}
	return plan, nil
}

// rejectMixedYAMLSpellings fails when a merge target NAME.yaml has a sibling
// entry NAME.yml or the reverse. The two spellings would render as separate
// files, so fragments meant for one document would never meet.
func rejectMixedYAMLSpellings(entries []Entry) error {
	byTarget := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		byTarget[entry.Target] = entry
	}
	for _, entry := range entries {
		if entry.Kind != KindMerge {
			continue
		}
		sibling, ok := alternateYAMLSpelling(entry.Target)
		if !ok {
			continue
		}
		if other, ok := byTarget[sibling]; ok {
			return fmt.Errorf("%s:%s and %s:%s would render %s and %s as separate files; spell the YAML extension one way for one file",
				entry.Source, entry.Path, other.Source, other.Path, entry.Target, other.Target)
		}
	}
	return nil
}

// alternateYAMLSpelling returns target with .yaml swapped for .yml or the
// reverse, and false when target has neither extension.
func alternateYAMLSpelling(target string) (string, bool) {
	switch filepath.Ext(target) {
	case ".yaml":
		return strings.TrimSuffix(target, ".yaml") + ".yml", true
	case ".yml":
		return strings.TrimSuffix(target, ".yml") + ".yaml", true
	}
	return "", false
}

func validateTargets(req Request, entries []Entry) error {
	root, err := canonicalPath(req.Target)
	if err != nil {
		return fmt.Errorf("resolve target directory: %w", err)
	}
	protected := make(map[string]string, len(req.ProtectedPaths))
	for _, path := range req.ProtectedPaths {
		parent, err := canonicalPath(filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("resolve protected path %q: %w", path, err)
		}
		protected[filepath.Join(parent, filepath.Base(path))] = path
		resolved, err := canonicalPath(path)
		if err != nil {
			return fmt.Errorf("resolve protected path %q: %w", path, err)
		}
		protected[resolved] = path
	}
	protectedFiles := make(map[string]fs.FileInfo, len(protected))
	for path, original := range protected {
		info, err := os.Lstat(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("check protected path %q: %w", original, err)
		}
		if info != nil {
			protectedFiles[path] = info
		}
	}

	targets := make(map[string]string, len(entries))
	for _, entry := range entries {
		parent, err := canonicalPath(filepath.Dir(entry.Target))
		if err != nil {
			return fmt.Errorf("resolve target %q: %w", entry.Target, err)
		}
		target := filepath.Join(parent, filepath.Base(entry.Target))
		relative, err := filepath.Rel(root, target)
		if err != nil || !filepath.IsLocal(relative) {
			return fmt.Errorf("target %q resolves outside the target directory", entry.Target)
		}
		if path, ok := protected[target]; ok {
			return fmt.Errorf("target %q would replace configuration file %q", entry.Target, path)
		}
		info, err := os.Lstat(target)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("check target %q: %w", entry.Target, err)
		}
		if info != nil {
			for path, protectedInfo := range protectedFiles {
				if os.SameFile(info, protectedInfo) {
					return fmt.Errorf("target %q would replace configuration file %q", entry.Target, protected[path])
				}
			}
		}
		if previous, ok := targets[target]; ok {
			return fmt.Errorf("targets %q and %q resolve to the same file", previous, entry.Target)
		}
		targets[target] = entry.Target
	}
	return nil
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	_, err = os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		parent, err := canonicalPath(filepath.Dir(abs))
		if err != nil {
			return "", err
		}
		return filepath.Join(parent, filepath.Base(abs)), nil
	}
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// contribution is one source file before the files sharing its target are
// resolved into an entry.
type contribution struct {
	Entry
	// rank is the index in the layer order of the layer the file applies in.
	rank int
	// template reports whether a fragment executes as a template before it
	// is parsed; other kinds leave it false.
	template bool
}

func (c contribution) fragment() Fragment {
	return Fragment{Origin: c.Origin, SourceFile: c.SourceFile, Template: c.template}
}

// resolveTarget folds the contributions for one target, in layer order and
// then discovery order, into the entry a render writes. Consecutive
// fragments accumulate into one merge entry whose mode keeps only the bits
// every fragment has; a plain file or template replaces whatever came
// before it and records it under Replaces; a fragment arriving above a
// plain file or template is an error, as are two contributions in one
// layer.
func resolveTarget(candidates []contribution) (Entry, error) {
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].rank < candidates[j].rank })

	entry := candidates[0].Entry
	if entry.Kind == KindMerge {
		entry.Merges = []Fragment{candidates[0].fragment()}
	}
	for i, c := range candidates[1:] {
		previous := candidates[i]
		if c.rank == previous.rank {
			return Entry{}, &CollisionError{First: previous.Origin, Second: c.Origin, Target: c.Target}
		}
		switch {
		case c.Kind == KindMerge && entry.Kind == KindMerge:
			entry.Merges = append(entry.Merges, c.fragment())
			entry.Mode &= c.Mode
			entry.Origin = c.Origin
			entry.SourceFile = c.SourceFile
		case c.Kind == KindMerge:
			return Entry{}, fmt.Errorf("%s:%s is a fragment of %s, but %s:%s (%s) renders that file whole in an earlier layer; rename it to a fragment or move it above",
				c.Source, c.Path, c.Target, entry.Source, entry.Path, entry.Layer)
		default:
			replaces := entry.Replaces
			if entry.Kind == KindMerge {
				for _, fragment := range entry.Merges {
					replaces = append(replaces, fragment.Origin)
				}
			} else {
				replaces = append(replaces, entry.Origin)
			}
			entry = c.Entry
			entry.Replaces = replaces
		}
	}
	return entry, nil
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

// classification says how one source file renders and where.
type classification struct {
	kind Kind
	// layer is the layer a fragment's name carries; other kinds leave it
	// empty.
	layer string
	// target is the target-relative path the file renders to.
	target string
	// template reports whether a fragment executes as a template before it
	// is parsed; other kinds leave it false.
	template bool
}

// classify decides how a source file renders and where. A name holding
// .olay. is a fragment NAME.olay.LAYER.EXT that merges into NAME.EXT, where
// LAYER follows the layer grammar and EXT names a merge format, and a
// trailing .tmpl makes the fragment a template; a name ending in .tmpl is
// otherwise a template whose target drops the suffix; anything else is a
// file. A malformed fragment name or a template name that is nothing but the
// suffix is an error.
func classify(pkg, sourcePath string) (classification, error) {
	dir, name := path.Split(sourcePath)
	if stem, rest, isFragment := strings.Cut(name, fragmentMarker); isFragment {
		return classifyFragment(pkg, sourcePath, dir, stem, rest)
	}

	stem, isTemplate := strings.CutSuffix(sourcePath, templateSuffix)
	if !isTemplate {
		return classification{kind: KindFile, target: targetPath(pkg, sourcePath)}, nil
	}
	if name == templateSuffix || name == dotPrefix+templateSuffix {
		return classification{}, fmt.Errorf("%s: a template needs a name before %s", sourcePath, templateSuffix)
	}
	return classification{kind: KindTemplate, target: targetPath(pkg, stem)}, nil
}

// classifyFragment validates the LAYER.EXT or LAYER.EXT.tmpl part of a
// fragment name and maps the fragment to its merged target.
func classifyFragment(pkg, sourcePath, dir, stem, rest string) (classification, error) {
	if stem == "" {
		return classification{}, fmt.Errorf("%s: a fragment needs a name before %s", sourcePath, fragmentMarker)
	}
	if strings.Contains(rest, fragmentMarker) {
		return classification{}, fmt.Errorf("%s: a fragment name holds %s once", sourcePath, fragmentMarker)
	}

	rest, isTemplate := strings.CutSuffix(rest, templateSuffix)
	layer, ext, hasExt := strings.Cut(rest, ".")
	if !hasExt {
		return classification{}, fmt.Errorf("%s: a fragment needs LAYER.EXT after %s", sourcePath, fragmentMarker)
	}
	if err := config.ValidateAnyLayerName(layer); err != nil {
		return classification{}, fmt.Errorf("%s: fragment layer: %w", sourcePath, err)
	}
	if _, ok := merge.FormatOf(ext); !ok {
		return classification{}, fmt.Errorf("%s: fragment extension %q must be json, toml, yaml, or yml", sourcePath, ext)
	}
	return classification{kind: KindMerge, layer: layer, target: targetPath(pkg, dir+stem+"."+ext), template: isTemplate}, nil
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

// templateContext builds the data every template among entries sees. It
// returns an empty context without looking up the hostname when no entry is
// a template and no merge entry holds a templated fragment.
func templateContext(req Request, entries []Entry) (tmpl.Context, error) {
	if !slices.ContainsFunc(entries, usesTemplates) {
		return tmpl.Context{}, nil
	}
	if req.Hostname == nil {
		return tmpl.Context{}, errors.New("determine hostname: no lookup configured")
	}
	hostname, err := req.Hostname()
	if err != nil {
		return tmpl.Context{}, fmt.Errorf("determine hostname: %w", err)
	}

	sources := make([]tmpl.Source, len(req.Sources))
	for i, source := range req.Sources {
		sources[i] = tmpl.Source{Name: source.Name, Path: source.Path}
	}
	return tmpl.Context{Home: req.Home, Hostname: hostname, Layers: req.Layers, OS: req.OS, Sources: sources}, nil
}

// usesTemplates reports whether entry is a template or a merge holding a
// templated fragment.
func usesTemplates(entry Entry) bool {
	if entry.Kind == KindTemplate {
		return true
	}
	return slices.ContainsFunc(entry.Merges, func(fragment Fragment) bool { return fragment.Template })
}

// renderContent returns the bytes a template or merge entry writes, and nil
// for a file entry, which streams from its SourceFile instead.
func renderContent(entry Entry, ctx tmpl.Context) ([]byte, error) {
	switch entry.Kind {
	case KindTemplate:
		return renderTemplate(entry, ctx)
	case KindMerge:
		return renderMerge(entry, ctx)
	}
	return nil, nil
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

// renderMerge reads every fragment of a merge entry, executes the templated
// ones against ctx, and composes them, naming each fragment source:path so
// errors point at the file in the checkout. Every fragment renders before
// any is parsed, so a template error is reported as one.
func renderMerge(entry Entry, ctx tmpl.Context) ([]byte, error) {
	format, ok := merge.FormatOf(strings.TrimPrefix(filepath.Ext(entry.Target), "."))
	if !ok {
		return nil, fmt.Errorf("merge %s: no format for its extension", entry.Target)
	}

	fragments := make([]merge.Fragment, len(entry.Merges))
	for i, fragment := range entry.Merges {
		name := fragment.Source + ":" + fragment.Path
		data, err := os.ReadFile(fragment.SourceFile)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", fragment.Source, err)
		}
		if fragment.Template {
			data, err = tmpl.Render(name, data, ctx)
			if err != nil {
				return nil, fmt.Errorf("merge %s: fragment %s: render %w", entry.Target, name, err)
			}
		}
		fragments[i] = merge.Fragment{Data: data, Name: name}
	}
	content, err := merge.Render(format, fragments)
	if err != nil {
		return nil, fmt.Errorf("merge %s: %w", entry.Target, err)
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
