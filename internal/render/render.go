// Package render orchestrates the full overlay pipeline: discover groups,
// merge their layers, substitute variables, and write the outputs to disk.
package render

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"charm.land/log/v2"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/document"
	"github.com/jmcampanini/overlay/internal/merge"
	"github.com/jmcampanini/overlay/internal/rendermode"
	"github.com/jmcampanini/overlay/internal/state"
	"github.com/jmcampanini/overlay/internal/substitute"
)

// Options carries everything Run needs beyond the resolved discover.Settings.
type Options struct {
	Settings          discover.Settings
	ContinueOnError   bool
	TOMLIndentTables  bool
	RenderRules       []config.RenderRule
	Substituter       *substitute.Resolver
	SubstituteExclude discover.Ignorer
	StatePath         string
	NoState           bool
	Logger            *log.Logger
}

// MergeOptions controls composition for one group: output formatting, render
// rules, and variable substitution.
type MergeOptions struct {
	TOMLIndentTables  bool
	RenderRules       []config.RenderRule
	TargetDir         string
	Substituter       *substitute.Resolver
	SubstituteExclude discover.Ignorer
}

// ComposedGroup is the in-memory result of composing one group. Content is the
// final bytes to write and is valid only when Err is nil; on failure (including
// MissingVarsError, where Content would have references stripped) it is nil and
// must not be written or displayed. Vars is populated whenever substitution
// ran, even on failure, so dry-run views can list names.
type ComposedGroup struct {
	Group       discover.Group
	Mode        rendermode.Mode
	Content     []byte
	Substituted bool
	Vars        substitute.Result
	Err         error
}

// MissingVarsError reports a target's referenced-but-unset variables.
type MissingVarsError struct {
	Names []string
}

func (e *MissingVarsError) Error() string {
	return "missing variables: " + strings.Join(e.Names, ", ")
}

// ComposeFailures aggregates every failed group so one run reports every
// failing target at once; a missing-variable failure lists all of that
// target's missing names.
type ComposeFailures []ComposedGroup

func (f ComposeFailures) Error() string {
	lines := make([]string, len(f))
	for i, cg := range f {
		lines[i] = fmt.Sprintf("render %s: %v", cg.Group.TargetPath, cg.Err)
	}
	return strings.Join(lines, "\n")
}

// Run discovers groups, composes all of them in memory, and only then writes.
// When any group fails to compose (parse error, missing variables) and
// ContinueOnError is false, nothing is written and the returned error names
// every failing target. With ContinueOnError, clean groups are written and a
// non-nil summary error is returned.
func Run(opts Options) error {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	maintainState := !opts.NoState
	if maintainState && opts.StatePath == "" {
		return fmt.Errorf("state path is required")
	}

	var prior []state.Entry
	if maintainState {
		var err error
		prior, err = state.Load(opts.StatePath)
		if err != nil && !errors.Is(err, state.ErrNotExist) {
			return err
		}
	}

	result, err := discover.WalkDetailed(opts.Settings)
	if err != nil {
		return fmt.Errorf("discover: %w", err)
	}
	for _, source := range result.MissingSources {
		opts.Logger.Warnf("source %q not found, skipping", source)
	}
	for _, stem := range result.Inactive {
		opts.Logger.Infof("skipping %s (no active layers)", stem)
	}
	groups := result.Active
	if opts.StatePath != "" {
		if err := rejectStateTargetCollision(opts.StatePath, groups); err != nil {
			return err
		}
	}
	if len(groups) == 0 {
		WarnUnusedPins(opts.Substituter, nil, opts.Logger)
		opts.Logger.Debugf("no overlay files found in %s", strings.Join(opts.Settings.SourceDirs, ", "))
		return saveRenderStateIfEnabled(maintainState, opts.StatePath, prior, nil, nil)
	}

	mergeOptions := MergeOptions{
		TOMLIndentTables:  opts.TOMLIndentTables,
		RenderRules:       opts.RenderRules,
		TargetDir:         opts.Settings.TargetDir,
		Substituter:       opts.Substituter,
		SubstituteExclude: opts.SubstituteExclude,
	}
	clean, failed := ComposeGroups(groups, mergeOptions)
	WarnUnusedPins(opts.Substituter, failed, opts.Logger)
	if len(failed) > 0 && !opts.ContinueOnError {
		opts.Logger.Errorf("%d %s failed to compose, nothing written", len(failed), pluralize(len(failed), "file", "files"))
		return ComposeFailures(failed)
	}
	for _, cg := range failed {
		opts.Logger.Errorf("render %s: %v", cg.Group.TargetPath, cg.Err)
	}

	var claimed []state.Entry
	if maintainState {
		claimed = make([]state.Entry, 0, len(clean))
	}
	var writeFailed int
	for _, cg := range clean {
		var entry state.Entry
		if maintainState {
			var entryErr error
			entry, entryErr = stateEntry(cg.Group)
			if entryErr != nil {
				renderErr := fmt.Errorf("render %s: record state: %w", cg.Group.TargetPath, entryErr)
				if opts.ContinueOnError {
					opts.Logger.Error(renderErr)
					writeFailed++
					continue
				}
				return saveRenderStateIfEnabled(maintainState, opts.StatePath, prior, claimed, renderErr)
			}
		}
		if err := writeGroup(cg, opts.Logger); err != nil {
			if opts.ContinueOnError {
				opts.Logger.Errorf("render %s: %v", cg.Group.TargetPath, err)
				writeFailed++
				continue
			}
			return saveRenderStateIfEnabled(maintainState, opts.StatePath, prior, claimed, fmt.Errorf("render %s: %w", cg.Group.TargetPath, err))
		}
		if maintainState {
			claimed = append(claimed, entry)
		}
	}
	totalFailed := len(failed) + writeFailed
	succeeded := len(clean) - writeFailed
	opts.Logger.Infof("overlayed %d %s", succeeded, pluralize(succeeded, "file", "files"))
	if totalFailed > 0 {
		return saveRenderStateIfEnabled(maintainState, opts.StatePath, prior, claimed, fmt.Errorf("%d %s failed to render", totalFailed, pluralize(totalFailed, "file", "files")))
	}
	return saveRenderStateIfEnabled(maintainState, opts.StatePath, prior, claimed, nil)
}

// A target can alias the manifest through symlinks or filesystem case rules.
// Reject it before writing, or the state rename would replace rendered output.
func rejectStateTargetCollision(statePath string, groups []discover.Group) error {
	resolvedStatePath, err := resolveFilesystemPath(statePath)
	if err != nil {
		return fmt.Errorf("resolve state path: %w", err)
	}
	stateInfo, _ := os.Stat(statePath)
	for _, group := range groups {
		resolvedTarget, err := resolveFilesystemPath(group.TargetPath)
		if err != nil {
			return fmt.Errorf("resolve target path: %w", err)
		}
		collides := resolvedTarget == resolvedStatePath || caseFoldedPathsAlias(resolvedStatePath, resolvedTarget)
		if !collides && stateInfo != nil {
			if targetInfo, statErr := os.Stat(group.TargetPath); statErr == nil {
				collides = os.SameFile(stateInfo, targetInfo)
			}
		}
		if collides {
			return fmt.Errorf("state path %q collides with a rendered target", resolvedStatePath)
		}
	}
	return nil
}

// Case behavior can vary by volume or directory, so probe filesystem identity
// instead of inferring it from the operating system.
func caseFoldedPathsAlias(first, second string) bool {
	if !strings.EqualFold(filepath.Base(first), filepath.Base(second)) {
		return false
	}
	firstDir := filepath.Dir(first)
	secondDir := filepath.Dir(second)
	firstInfo, firstErr := os.Stat(firstDir)
	secondInfo, secondErr := os.Stat(secondDir)
	if firstErr != nil || secondErr != nil || !os.SameFile(firstInfo, secondInfo) {
		return false
	}
	return caseInsensitiveDirectory(firstDir)
}

func caseInsensitiveDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			alternate := alternateFilenameCase(entry.Name())
			if alternate == entry.Name() {
				continue
			}
			originalInfo, originalErr := os.Lstat(filepath.Join(dir, entry.Name()))
			alternateInfo, alternateErr := os.Lstat(filepath.Join(dir, alternate))
			if originalErr != nil || alternateErr != nil {
				if errors.Is(alternateErr, fs.ErrNotExist) {
					return false
				}
				continue
			}
			return os.SameFile(originalInfo, alternateInfo)
		}
	}

	parent := filepath.Dir(dir)
	alternate := alternateFilenameCase(filepath.Base(dir))
	if parent == dir || alternate == filepath.Base(dir) {
		return false
	}
	originalInfo, originalErr := os.Stat(dir)
	alternateInfo, alternateErr := os.Stat(filepath.Join(parent, alternate))
	return originalErr == nil && alternateErr == nil && os.SameFile(originalInfo, alternateInfo)
}

func alternateFilenameCase(name string) string {
	for i := range len(name) {
		switch {
		case name[i] >= 'a' && name[i] <= 'z':
			return name[:i] + string(name[i]-'a'+'A') + name[i+1:]
		case name[i] >= 'A' && name[i] <= 'Z':
			return name[:i] + string(name[i]-'A'+'a') + name[i+1:]
		}
	}
	return name
}

// Resolve existing prefixes and dangling symlinks so not-yet-created targets
// can be compared by the filesystem location they will ultimately reach.
func resolveFilesystemPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := absolute
	var missing []string
	seenSymlinks := map[string]struct{}{}
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if errors.Is(err, syscall.ENOTDIR) {
			return absolute, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if info, lstatErr := os.Lstat(current); lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
			if _, seen := seenSymlinks[current]; seen {
				return "", fmt.Errorf("symlink cycle at %q", current)
			}
			seenSymlinks[current] = struct{}{}
			destination, readErr := os.Readlink(current)
			if readErr != nil {
				return "", readErr
			}
			if filepath.IsAbs(destination) {
				current = filepath.Clean(destination)
			} else {
				current = filepath.Join(filepath.Dir(current), destination)
			}
			continue
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absolute, nil
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func stateEntry(group discover.Group) (state.Entry, error) {
	target, err := filepath.Abs(group.TargetPath)
	if err != nil {
		return state.Entry{}, fmt.Errorf("resolve target path: %w", err)
	}
	source, err := filepath.Abs(group.SourceDir)
	if err != nil {
		return state.Entry{}, fmt.Errorf("resolve source path: %w", err)
	}
	return state.Entry{Target: target, Source: source}, nil
}

func saveRenderStateIfEnabled(enabled bool, path string, prior, claimed []state.Entry, renderErr error) error {
	if !enabled {
		return renderErr
	}
	return saveRenderState(path, prior, claimed, renderErr)
}

func saveRenderState(path string, prior, claimed []state.Entry, renderErr error) error {
	if err := state.Save(path, state.Merge(prior, claimed)); err != nil {
		saveErr := fmt.Errorf("save state: %w", err)
		if renderErr != nil {
			return errors.Join(renderErr, saveErr)
		}
		return saveErr
	}
	return renderErr
}

// WarnUnusedPins logs pinned variables that no composed target consumed. It is
// shared by render, diff, and plan so every command surfaces the same signal.
// It stays silent when any group failed before substitution ran (parse or
// decision error), because the resolver's consumed set is then incomplete and
// the warnings would be false positives; missing-variable failures still ran
// substitution, so they do not suppress it.
func WarnUnusedPins(substituter *substitute.Resolver, failed []ComposedGroup, logger *log.Logger) {
	if !substituter.Enabled() || logger == nil || !substitutionComplete(failed) {
		return
	}
	for _, name := range substituter.UnusedPins() {
		logger.Warnf("pinned variable %s was not consumed by any target", name)
	}
}

// substitutionComplete reports whether every failed group failed only because
// of missing variables (substitution ran) rather than a parse or decision
// error (substitution never ran), so the resolver's consumed set is trustworthy.
func substitutionComplete(failed []ComposedGroup) bool {
	for _, cg := range failed {
		var missing *MissingVarsError
		if !errors.As(cg.Err, &missing) {
			return false
		}
	}
	return true
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func writeGroup(cg ComposedGroup, logger *log.Logger) error {
	if err := os.MkdirAll(filepath.Dir(cg.Group.TargetPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(cg.Group.TargetPath, cg.Content, 0o644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	logger.Infof("%s ← [%s]", cg.Group.TargetPath, layerNames(cg.Group))
	return nil
}

// ComposeGroups composes every group in memory, partitioning the results
// into clean and failed. It performs no disk writes.
func ComposeGroups(groups []discover.Group, opts MergeOptions) (clean, failed []ComposedGroup) {
	for _, g := range groups {
		cg := ComposeGroup(g, opts)
		if cg.Err != nil {
			failed = append(failed, cg)
			continue
		}
		clean = append(clean, cg)
	}
	return clean, failed
}

// ComposeGroup composes one group's final content: decide mode and
// substitution, fold layers, and apply variables either inside merge-layer
// value trees or over final copy/append bytes. Vars is populated even when
// missing variables fail the group, so dry-run views can still report names.
func ComposeGroup(g discover.Group, opts MergeOptions) ComposedGroup {
	decision, err := rendermode.Decide(g, opts.TargetDir, opts.RenderRules, opts.Substituter.Enabled(), opts.SubstituteExclude)
	if err != nil {
		return ComposedGroup{Group: g, Err: err}
	}
	cg := ComposedGroup{Group: g, Mode: decision.Mode}
	if decision.Mode == rendermode.ModeMerge && decision.Substitute {
		cg.Substituted = true
		content, vars, err := mergeLayersSubstituting(g, opts)
		cg.Vars = vars
		if err != nil {
			cg.Err = err
			return cg
		}
		if len(cg.Vars.Missing) > 0 {
			cg.Err = &MissingVarsError{Names: cg.Vars.Missing}
			return cg
		}
		cg.Content = content
		return cg
	}

	content, err := composeContent(g, decision.Mode, opts)
	if err != nil {
		cg.Err = err
		return cg
	}
	if decision.Substitute {
		cg.Substituted = true
		content, cg.Vars = opts.Substituter.Apply(content)
		if len(cg.Vars.Missing) > 0 {
			// Content here has the missing references stripped, so leave it nil
			// per ComposedGroup's contract — callers must not write failures.
			// Names shares Vars.Missing's backing array; both are read-only.
			cg.Err = &MissingVarsError{Names: cg.Vars.Missing}
			return cg
		}
	}
	cg.Content = content
	return cg
}

func composeContent(g discover.Group, mode rendermode.Mode, opts MergeOptions) ([]byte, error) {
	switch mode {
	case rendermode.ModeCopy:
		return copyWinningLayer(g)
	case rendermode.ModeAppend:
		return appendLayers(g)
	case rendermode.ModeMerge:
		return mergeLayers(g, opts)
	default:
		return nil, fmt.Errorf("unknown render mode %q for %q", mode, g.TargetPath)
	}
}

func mergeLayers(g discover.Group, opts MergeOptions) ([]byte, error) {
	var merged any = map[string]any{}
	for _, layer := range g.Layers {
		parsed, err := parseLayer(layer, g.Format)
		if err != nil {
			return nil, err
		}
		merged = merge.Merge(merged, parsed)
	}
	return serializeContent(merged, g.Format, opts)
}

func mergeLayersSubstituting(g discover.Group, opts MergeOptions) ([]byte, substitute.Result, error) {
	var merged any = map[string]any{}
	collector := &substitutionCollector{}
	for _, layer := range g.Layers {
		parsed, err := parseLayer(layer, g.Format)
		if err != nil {
			return nil, collector.result(), err
		}
		substituted, err := substituteTree(parsed, opts.Substituter, collector, "$")
		if err != nil {
			return nil, collector.result(), fmt.Errorf("substitute %s: %w", layer.Path, err)
		}
		merged = merge.Merge(merged, substituted)
	}
	vars := collector.result()
	if len(vars.Missing) > 0 {
		return nil, vars, nil
	}
	content, err := serializeContent(merged, g.Format, opts)
	return content, vars, err
}

func parseLayer(layer discover.Layer, format document.Format) (any, error) {
	data, err := readLayer(layer)
	if err != nil {
		return nil, err
	}
	parsed, err := document.Parse(data, format)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", layer.Path, err)
	}
	return parsed, nil
}

func serializeContent(value any, format document.Format, opts MergeOptions) ([]byte, error) {
	return document.SerializeWithOptions(value, format, document.SerializeOptions{
		TOMLIndentTables: opts.TOMLIndentTables,
	})
}

func readLayer(layer discover.Layer) ([]byte, error) {
	data, err := os.ReadFile(layer.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", layer.Path, err)
	}
	return data, nil
}

func copyWinningLayer(g discover.Group) ([]byte, error) {
	if len(g.Layers) == 0 {
		return nil, fmt.Errorf("copy group %q has no active layers", g.Stem)
	}
	return readLayer(g.Layers[len(g.Layers)-1])
}

func appendLayers(g discover.Group) ([]byte, error) {
	if len(g.Layers) == 0 {
		return nil, fmt.Errorf("append group %q has no active layers", g.Stem)
	}
	var out []byte
	for _, layer := range g.Layers {
		data, err := readLayer(layer)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			continue
		}
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		out = append(out, data...)
	}
	return out, nil
}

func layerNames(g discover.Group) string {
	names := make([]string, len(g.Layers))
	for i, l := range g.Layers {
		names[i] = l.Profile
	}
	return strings.Join(names, ", ")
}
