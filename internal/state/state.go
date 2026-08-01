// Package state persists the targets successfully written by overlay.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

// ErrNotExist identifies an uninitialized state file.
var ErrNotExist = errors.New("state file does not exist")

// Entry records one rendered target and its owning source directory.
type Entry struct {
	Target string `json:"target"`
	Source string `json:"source"`
}

// Load reads and validates a state file.
func Load(path string) ([]Entry, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotExist, path)
		}
		return nil, fmt.Errorf("load state %q: %w", path, err)
	}
	if err := validateStateJSON(contents); err != nil {
		return nil, invalidError(path, err)
	}

	var stored struct {
		Entries []Entry `json:"entries"`
	}
	if err := json.Unmarshal(contents, &stored); err != nil {
		return nil, invalidError(path, err)
	}
	if err := validateEntries(stored.Entries); err != nil {
		return nil, invalidError(path, err)
	}
	return stored.Entries, nil
}

// Save garbage-collects stale entries and atomically replaces the state file.
func Save(path string, entries []Entry) error {
	if err := validateEntries(entries); err != nil {
		return fmt.Errorf("save state %q: %w", path, err)
	}

	live := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		info, err := os.Lstat(entry.Target)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
				continue
			}
			return fmt.Errorf("inspect state target %q: %w", entry.Target, err)
		}
		if info.Mode().IsRegular() {
			live = append(live, entry)
		}
	}
	sort.Slice(live, func(i, j int) bool {
		return live[i].Target < live[j].Target
	})

	contents, err := json.MarshalIndent(struct {
		Entries []Entry `json:"entries"`
	}{Entries: live}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state %q: %w", path, err)
	}
	contents = append(contents, '\n')

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary state file for %q: %w", path, err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set temporary state permissions for %q: %w", path, err)
	}
	if _, err := temp.Write(contents); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary state file for %q: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary state file for %q: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace state %q: %w", path, err)
	}

	return nil
}

// Merge adds claims to prior state, replacing ownership for matching targets.
func Merge(prior, claimed []Entry) []Entry {
	byTarget := make(map[string]Entry, len(prior)+len(claimed))
	for _, entry := range prior {
		byTarget[entry.Target] = entry
	}
	for _, entry := range claimed {
		byTarget[entry.Target] = entry
	}

	merged := make([]Entry, 0, len(byTarget))
	for _, entry := range byTarget {
		merged = append(merged, entry)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Target < merged[j].Target
	})
	return merged
}

func validateStateJSON(contents []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	if err := requireDelimiter(decoder, '{'); err != nil {
		return err
	}

	seenEntries := false
	for decoder.More() {
		key, err := readObjectKey(decoder)
		if err != nil {
			return err
		}
		if key != "entries" {
			return fmt.Errorf("unknown field %q", key)
		}
		if seenEntries {
			return errors.New("duplicate field \"entries\"")
		}
		seenEntries = true
		if err := requireDelimiter(decoder, '['); err != nil {
			return fmt.Errorf("entries must be an array: %w", err)
		}
		for decoder.More() {
			if err := validateEntryJSON(decoder); err != nil {
				return err
			}
		}
		if err := requireDelimiter(decoder, ']'); err != nil {
			return err
		}
	}
	if err := requireDelimiter(decoder, '}'); err != nil {
		return err
	}
	if !seenEntries {
		return errors.New("entries must be an array")
	}
	return rejectTrailingJSON(decoder)
}

func validateEntryJSON(decoder *json.Decoder) error {
	if err := requireDelimiter(decoder, '{'); err != nil {
		return fmt.Errorf("entry must be an object: %w", err)
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := readObjectKey(decoder)
		if err != nil {
			return err
		}
		if key != "target" && key != "source" {
			return fmt.Errorf("unknown entry field %q", key)
		}
		if seen[key] {
			return fmt.Errorf("duplicate entry field %q", key)
		}
		seen[key] = true
		value, err := decoder.Token()
		if err != nil {
			return err
		}
		if _, ok := value.(string); !ok {
			return fmt.Errorf("entry field %q must be a string", key)
		}
	}
	if err := requireDelimiter(decoder, '}'); err != nil {
		return err
	}
	for _, key := range []string{"target", "source"} {
		if !seen[key] {
			return fmt.Errorf("entry is missing field %q", key)
		}
	}
	return nil
}

func readObjectKey(decoder *json.Decoder) (string, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", err
	}
	key, ok := token.(string)
	if !ok {
		return "", errors.New("object field name must be a string")
	}
	return key, nil
}

func requireDelimiter(decoder *json.Decoder, want json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != want {
		return fmt.Errorf("expected %q, got %v", want, token)
	}
	return nil
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}

func validateEntries(entries []Entry) error {
	seen := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		if entry.Target == "" {
			return fmt.Errorf("entry %d has an empty target", index)
		}
		if !filepath.IsAbs(entry.Target) {
			return fmt.Errorf("entry %d target %q is not absolute", index, entry.Target)
		}
		if entry.Source == "" {
			return fmt.Errorf("entry %d has an empty source", index)
		}
		if !filepath.IsAbs(entry.Source) {
			return fmt.Errorf("entry %d source %q is not absolute", index, entry.Source)
		}
		if _, exists := seen[entry.Target]; exists {
			return fmt.Errorf("duplicate target %q", entry.Target)
		}
		seen[entry.Target] = struct{}{}
	}
	return nil
}

func invalidError(path string, err error) error {
	return fmt.Errorf("invalid state file %q: %v; delete it and re-run overlay render to establish a baseline", path, err)
}
