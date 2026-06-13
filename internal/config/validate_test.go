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

func TestValidateRejectsBlankEnvProfileEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
env_profiles = ["DOTFILES_PROFILE", " "]
`)
	err := ValidateFile(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "env_profiles contains an empty environment variable name") {
		t.Errorf("error should mention empty env_profiles entry: %v", err)
	}
}

func TestValidateRejectsPaddedEnvProfileName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
env_profiles = [" DOTFILES_PROFILE"]
`)
	err := ValidateFile(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "leading or trailing whitespace") {
		t.Errorf("error should mention whitespace: %v", err)
	}
}

func TestValidateAcceptsEnvProfilesList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
env_profiles = ["DOTFILES_PROFILE", "HOST_PROFILE"]
`)
	if err := ValidateFile(path); err != nil {
		t.Errorf("expected valid, got: %v", err)
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

func TestValidateRenderRulesValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
substitute_prefixes = ["DOTFILES_THM_", "DOTFILES_THEME_"]

[[render_rules]]
path = ".npmrc"
strategy = "append"

[[render_rules]]
path = ".some/generated.json"
strategy = "copy"

[[render_rules]]
path = ".config/starship.toml"
strategy = "merge"

[[render_rules]]
path = ".config/ghostty/config"

[[render_rules]]
path = ".config/fzf/.fzfrc"
substitute = true

[[render_rules]]
path = ".config/shell/theme.sh"
substitute = false
`)
	if err := ValidateFile(path); err != nil {
		t.Errorf("expected valid render rules, got: %v", err)
	}
}

func TestValidateSubstituteRejectsNonBoolean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".overlay.toml")
	writeFile(t, path, `
target = "~/"
substitute_prefixes = ["DOTFILES_"]

[[render_rules]]
path = ".npmrc"
substitute = "true"
`)
	err := ValidateFile(path)
	if err == nil {
		t.Fatal("expected error for string substitute value")
	}
	if !strings.Contains(err.Error(), "boolean") {
		t.Errorf("error = %v, want mention of boolean", err)
	}
}

func TestValidateSubstitutePrefixes(t *testing.T) {
	if err := ValidateSubstitutePrefixes([]string{"DOTFILES_", "_X", "A1"}); err != nil {
		t.Errorf("expected valid prefixes, got: %v", err)
	}
	for _, p := range []string{"", " ", "1A", "A-B", "A.B"} {
		if err := ValidateSubstitutePrefixes([]string{p}); err == nil {
			t.Errorf("prefix %q: expected error", p)
		}
	}
}

func TestValidateSubstitutionRequiresPrefixes(t *testing.T) {
	rules := []RenderRule{{Path: ".npmrc", Substitute: TriStateTrue}}
	if err := ValidateSubstitution(nil, rules); err == nil {
		t.Error("substitute=true with no prefixes should error")
	}
	if err := ValidateSubstitution([]string{"DOTFILES_"}, rules); err != nil {
		t.Errorf("substitute=true with prefixes should be valid, got: %v", err)
	}
	offRules := []RenderRule{{Path: ".npmrc", Substitute: TriStateFalse}, {Path: ".x"}}
	if err := ValidateSubstitution(nil, offRules); err != nil {
		t.Errorf("substitute=false/unset with no prefixes should be valid, got: %v", err)
	}
}

func TestTriState(t *testing.T) {
	if v, set := TriStateUnset.Bool(); v || set {
		t.Error("unset TriState should report (false, false)")
	}
	if v, set := TriStateTrue.Bool(); !v || !set {
		t.Error("true TriState should report (true, true)")
	}
	if v, set := TriStateFalse.Bool(); v || !set {
		t.Error("false TriState should report (false, true)")
	}
	out, err := TriStateTrue.MarshalTOML()
	if err != nil || string(out) != "true" {
		t.Errorf("MarshalTOML = %q, %v; want bare true", out, err)
	}
	if _, err := TriStateUnset.MarshalTOML(); err == nil {
		t.Error("marshaling unset TriState should error")
	}
}

func TestValidateRenderRulesRejectsInvalidConfig(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing path",
			body: `[[render_rules]]
strategy = "append"`,
			want: "path",
		},
		{
			name: "empty path",
			body: `[[render_rules]]
path = ""
strategy = "append"`,
			want: "path",
		},
		{
			name: "absolute path",
			body: `[[render_rules]]
path = "/.npmrc"
strategy = "append"`,
			want: "target-relative",
		},
		{
			name: "path traversal",
			body: `[[render_rules]]
path = "../.npmrc"
strategy = "append"`,
			want: "..",
		},
		{
			name: "nested path traversal",
			body: `[[render_rules]]
path = ".ssh/../config"
strategy = "append"`,
			want: "..",
		},
		{
			name: "unsupported auto",
			body: `[[render_rules]]
path = ".npmrc"
strategy = "auto"`,
			want: "unsupported",
		},
		{
			name: "unsupported replace",
			body: `[[render_rules]]
path = ".npmrc"
strategy = "replace"`,
			want: "unsupported",
		},
		{
			name: "duplicate normalized path",
			body: `[[render_rules]]
path = "./.npmrc"
strategy = "append"

[[render_rules]]
path = ".npmrc"
strategy = "copy"`,
			want: "duplicates",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".overlay.toml")
			writeFile(t, path, "target = \"~/\"\n\n"+tc.body)
			err := ValidateFile(path)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestNormalizeRenderRulePath(t *testing.T) {
	got, err := NormalizeRenderRulePath("./.ssh//config")
	if err != nil {
		t.Fatal(err)
	}
	if got != ".ssh/config" {
		t.Errorf("NormalizeRenderRulePath = %q, want .ssh/config", got)
	}
}
