package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidateValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
sources = ["."]
target = "~/"
profiles = ["work"]
`)
	if err := ValidateFile(path); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidateMissingTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `sources = ["."]`)
	err := ValidateFile(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "target") {
		t.Errorf("error should mention target: %v", err)
	}
}

func TestValidateRejectsEmptySources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
sources = []
target = "~/"
`)
	err := ValidateFile(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "sources") {
		t.Errorf("error should mention sources: %v", err)
	}
}

func TestValidateRejectsEmptySourceString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
sources = [""]
target = "~/"
`)
	err := ValidateFile(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "empty source") {
		t.Errorf("error should mention empty source: %v", err)
	}
}

func TestValidateReservedProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
profiles = ["base"]
`)
	err := ValidateFile(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error should mention 'reserved': %v", err)
	}
}

func TestValidateUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
mystery_field = "hello"
`)
	err := ValidateFile(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "mystery_field") && !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error should mention unknown field: %v", err)
	}
}

func TestValidateMissingFile(t *testing.T) {
	err := ValidateFile(filepath.Join(t.TempDir(), "missing.toml"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestValidateMalformedTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `not valid toml [[[`)
	if err := ValidateFile(path); err == nil {
		t.Error("expected parse error")
	}
}
