package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmcampanini/overlay/internal/merge"
)

const (
	valuesExtension = ".toml"
	templateSuffix  = ".tmpl"
)

// Values is the per-machine values document templates read as .Values.
type Values struct {
	// Dir is the values directory searched, whether or not it exists.
	Dir string
	// Document is the merged document, empty when no file loaded.
	Document map[string]any
	// Files lists every file merged, as absolute paths in merge order.
	Files []string
	// Origins maps each top-level key to the file that last contributed to
	// it.
	Origins map[string]string
}

// ValuesOptions selects what LoadValues reads.
type ValuesOptions struct {
	// Dir is the values directory, usually DefaultValuesDir; an absent
	// directory contributes nothing.
	Dir string
	// Paths are files or directories merged after Dir in the order given.
	// Each must exist.
	Paths []string
}

// LoadValues merges every values file into one document. The *.toml files
// directly in opts.Dir load in byte order of name; names starting with a
// dot, directories, and other extensions are ignored, and a name ending in
// .tmpl is an error. Each of opts.Paths then loads: a directory by the same
// rule, a file as TOML whatever its name. The files merge in that order as
// TOML fragments, with the rules merge.Merge documents, so a later file
// overrides a key an earlier one set.
func LoadValues(opts ValuesOptions) (Values, error) {
	var files []string
	entries, err := os.ReadDir(opts.Dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Values{}, fmt.Errorf("read values directory: %w", err)
	default:
		if files, err = valuesFiles(opts.Dir, entries); err != nil {
			return Values{}, err
		}
	}
	for _, path := range opts.Paths {
		more, err := explicitValuesFiles(path)
		if err != nil {
			return Values{}, err
		}
		files = append(files, more...)
	}

	documents := make([]merge.Document, 0, len(files))
	origins := make(map[string]string)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return Values{}, fmt.Errorf("read values file: %w", err)
		}
		document, err := merge.Parse(merge.TOML, merge.Fragment{Data: data, Name: path})
		if err != nil {
			return Values{}, fmt.Errorf("load values: %w", err)
		}
		for key := range document.Table {
			origins[key] = path
		}
		documents = append(documents, document)
	}

	merged, err := merge.Merge(merge.TOML, documents)
	if err != nil {
		return Values{}, fmt.Errorf("load values: %w", err)
	}
	return Values{Dir: opts.Dir, Document: merged, Files: files, Origins: origins}, nil
}

// explicitValuesFiles resolves one --values operand to the files it names.
func explicitValuesFiles(path string) ([]string, error) {
	if path == "" {
		return nil, errors.New("--values must name a file or directory")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("--values %q: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("--values: %w", err)
	}

	if info.IsDir() {
		entries, err := os.ReadDir(abs)
		if err != nil {
			return nil, fmt.Errorf("read values directory: %w", err)
		}
		return valuesFiles(abs, entries)
	}
	if strings.HasSuffix(abs, templateSuffix) {
		return nil, templateValuesError(abs)
	}
	return []string{abs}, nil
}

// valuesFiles selects the *.toml files among dir's entries, which os.ReadDir
// lists in byte order of name, following a symbolic link to a file.
func valuesFiles(dir string, entries []fs.DirEntry) ([]string, error) {
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if strings.HasSuffix(name, templateSuffix) {
			return nil, templateValuesError(filepath.Join(dir, name))
		}
		if !strings.HasSuffix(name, valuesExtension) {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("check values file: %w", err)
		}
		if info.IsDir() {
			continue
		}
		files = append(files, path)
	}
	return files, nil
}

func templateValuesError(path string) error {
	return fmt.Errorf("values file %s: a values file cannot be a template; values are the data templates read", path)
}
