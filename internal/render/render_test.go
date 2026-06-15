package render

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"charm.land/log/v2"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
	"github.com/jmcampanini/overlay/internal/document"
	"github.com/jmcampanini/overlay/internal/substitute"
)

func newTestLogger() *log.Logger {
	l := log.New(os.Stderr)
	l.SetLevel(log.FatalLevel)
	return l
}

func TestRunBasic(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "dot-claude", "settings.olay.base.json"), `{"a":1,"nested":{"b":2}}`)
	writeFile(t, filepath.Join(src, "dot-claude", "settings.olay.work.json"), `{"nested":{"c":3}}`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			DotPrefix:  true,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := filepath.Join(target, ".claude", "settings.json")
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	// Expect keys alphabetized: a, nested (with b, c)
	want := `{
  "a": 1,
  "nested": {
    "b": 2,
    "c": 3
  }
}
`
	if string(data) != want {
		t.Errorf("content mismatch:\ngot:\n%s\nwant:\n%s", data, want)
	}
}

func TestRunScalarListDedupe(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "settings.olay.base.json"), `{"allow":["a","b"]}`)
	writeFile(t, filepath.Join(src, "settings.olay.work.json"), `{"allow":["b","c"]}`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(target, "settings.json"))
	want := `{
  "allow": [
    "a",
    "b",
    "c"
  ]
}
`
	if string(data) != want {
		t.Errorf("content mismatch:\ngot:\n%s\nwant:\n%s", data, want)
	}
}

func TestRunTOML(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "config.olay.base.toml"), `model = "gpt-5.4"`)
	writeFile(t, filepath.Join(src, "config.olay.local.toml"), `
[projects."/path"]
trust_level = "trusted"
`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(target, "config.toml"))
	content := string(data)
	if !strings.Contains(content, `model = 'gpt-5.4'`) && !strings.Contains(content, `model = "gpt-5.4"`) {
		t.Errorf("missing model: %s", data)
	}
	if !strings.Contains(content, "trust_level") {
		t.Errorf("missing trust_level: %s", data)
	}
}

func TestRunYAML(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "config.olay.base.yaml"), `app:
  name: overlay
  features:
    - json
    - toml
`)
	writeFile(t, filepath.Join(src, "config.olay.dark.yaml"), `app:
  features:
    - toml
    - yaml
  debug: true
`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   []string{"dark"},
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "config.yaml"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	want := `app:
  debug: true
  features:
    - json
    - toml
    - yaml
  name: overlay
`
	if string(data) != want {
		t.Errorf("content mismatch:\ngot:\n%s\nwant:\n%s", data, want)
	}
}

func TestRunYML(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "config.olay.base.yml"), `name: base`)
	writeFile(t, filepath.Join(src, "config.olay.work.yml"), `debug: true`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "config.yml"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	want := "debug: true\nname: base\n"
	if string(data) != want {
		t.Errorf("content mismatch:\ngot:\n%s\nwant:\n%s", data, want)
	}
}

func TestRunYAMLLazyGitInspired(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "lazygit", "config.olay.base.yml"), `gui:
  nerdFontsVersion: "3"
filterMode: fuzzy
git:
  mainBranches:
    - main
    - develop
    - master
  pagers:
    - colorArg: always
      pager: delta --paging=never --line-numbers
`)
	writeFile(t, filepath.Join(src, "lazygit", "config.olay.catppuccin.yml"), `gui:
  theme:
    activeBorderColor:
      - "#cba6f7"
      - bold
    selectedLineBgColor:
      - "#313244"
authorColors:
  "*": "#b4befe"
`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   []string{"catppuccin"},
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "lazygit", "config.yml"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`authorColors:
  '*': '#b4befe'`,
		"filterMode: fuzzy",
		"nerdFontsVersion: \"3\"",
		"activeBorderColor:\n      - '#cba6f7'\n      - bold",
		"pager: delta --paging=never --line-numbers",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q:\n%s", want, data)
		}
	}
}

func TestRunYAMLCopyRule(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "config.olay.base.yaml"), "a: 1\n")
	writeFile(t, filepath.Join(src, "config.olay.work.yaml"), "b: 2\n")

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		RenderRules: []config.RenderRule{{Path: "config.yaml", Strategy: config.RenderStrategyCopy}},
		Logger:      newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "config.yaml"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "b: 2\n" {
		t.Fatalf("content = %q, want work layer", data)
	}
}

func TestRunTOMLIndentTablesOption(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "config.olay.base.toml"), `model = "gpt-5.4"`)
	writeFile(t, filepath.Join(src, "config.olay.local.toml"), `
[projects."/path"]
trust_level = "trusted"
`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		TOMLIndentTables: true,
		Logger:           newTestLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(target, "config.toml"))
	content := string(data)
	if !strings.Contains(content, "  [projects.'/path']") || !strings.Contains(content, "    trust_level = 'trusted'") {
		t.Errorf("expected indented TOML tables:\n%s", data)
	}
}

func TestRunCopyThroughUsesWinningLayer(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	base := filepath.Join(src, "bin", "tool.olay.base.sh")
	writeFile(t, base, "base\n")
	writeFile(t, filepath.Join(src, "bin", "tool.olay.work.sh"), "work\n")
	writeFile(t, filepath.Join(src, "bin", "tool.olay.local.sh"), "local\n")
	if err := os.Chmod(base, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := filepath.Join(target, "bin", "tool.sh")
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "local\n" {
		t.Fatalf("content = %q, want local", data)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("output mode = %v, should not preserve executable bits", info.Mode().Perm())
	}
}

func TestRunExtensionlessCopyThrough(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "README.olay.base"), "base\n")
	writeFile(t, filepath.Join(src, "README.olay.work"), "work\n")

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "README"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "work\n" {
		t.Fatalf("content = %q, want work", data)
	}
}

func TestRunAppendRuleDotPrefixTargetPath(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "dot-npmrc.olay.base"), "allow-git=root\naudit=true\n")
	writeFile(t, filepath.Join(src, "dot-npmrc.olay.work"), "@company:registry=https://registry.example.com/\n")

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			DotPrefix:  true,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		RenderRules: []config.RenderRule{{Path: ".npmrc", Strategy: config.RenderStrategyAppend}},
		Logger:      newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, ".npmrc"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	want := "allow-git=root\naudit=true\n@company:registry=https://registry.example.com/\n"
	if string(data) != want {
		t.Fatalf("content = %q, want %q", data, want)
	}
}

func TestRunAppendRuleRespectsLayerOrderAndProfiles(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "rc.olay.base"), "base")
	writeFile(t, filepath.Join(src, "rc.olay.work"), "work")
	writeFile(t, filepath.Join(src, "rc.olay.personal"), "personal")
	writeFile(t, filepath.Join(src, "rc.olay.other"), "other")
	writeFile(t, filepath.Join(src, "rc.olay.local"), "local")

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   []string{"personal", "work"},
			Ignore:     discover.NoopIgnorer(),
		},
		RenderRules: []config.RenderRule{{Path: "rc", Strategy: config.RenderStrategyAppend}},
		Logger:      newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "rc"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	want := "base\npersonal\nwork\nlocal"
	if string(data) != want {
		t.Fatalf("content = %q, want %q", data, want)
	}
}

func TestRunAppendRuleHandlesSingleActiveLayer(t *testing.T) {
	cases := []struct {
		name     string
		profile  string
		profiles []string
	}{
		{name: "base only", profile: "base"},
		{name: "profile only", profile: "work", profiles: []string{"work"}},
		{name: "local only", profile: "local"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := t.TempDir()
			target := t.TempDir()
			writeFile(t, filepath.Join(src, "rc.olay."+tc.profile), tc.profile)

			err := Run(Options{
				Settings: discover.Settings{
					SourceDirs: []string{src},
					TargetDir:  target,
					Profiles:   tc.profiles,
					Ignore:     discover.NoopIgnorer(),
				},
				RenderRules: []config.RenderRule{{Path: "rc", Strategy: config.RenderStrategyAppend}},
				Logger:      newTestLogger(),
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(target, "rc"))
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			if string(data) != tc.profile {
				t.Fatalf("content = %q, want %q", data, tc.profile)
			}
		})
	}
}

func TestRunAppendRuleNewlineBoundaries(t *testing.T) {
	cases := []struct {
		name string
		base string
		work string
		want string
	}{
		{name: "base newline work non-empty", base: "base\n", work: "work", want: "base\nwork"},
		{name: "base no newline work non-empty", base: "base", work: "work", want: "base\nwork"},
		{name: "base newline work empty", base: "base\n", work: "", want: "base\n"},
		{name: "base empty work non-empty", base: "", work: "work", want: "work"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := t.TempDir()
			target := t.TempDir()
			writeFile(t, filepath.Join(src, "rc.olay.base"), tc.base)
			writeFile(t, filepath.Join(src, "rc.olay.work"), tc.work)

			err := Run(Options{
				Settings: discover.Settings{
					SourceDirs: []string{src},
					TargetDir:  target,
					Profiles:   []string{"work"},
					Ignore:     discover.NoopIgnorer(),
				},
				RenderRules: []config.RenderRule{{Path: "rc", Strategy: config.RenderStrategyAppend}},
				Logger:      newTestLogger(),
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(target, "rc"))
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			if string(data) != tc.want {
				t.Fatalf("content = %q, want %q", data, tc.want)
			}
		})
	}
}

func TestRunCopyRuleForJSON(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "settings.olay.base.json"), `{"a":1}`)
	writeFile(t, filepath.Join(src, "settings.olay.work.json"), `{"b":2}`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		RenderRules: []config.RenderRule{{Path: "settings.json", Strategy: config.RenderStrategyCopy}},
		Logger:      newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "settings.json"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != `{"b":2}` {
		t.Fatalf("content = %q, want work layer", data)
	}
}

func TestRunRuleSourceRelativeNameDoesNotMatch(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "dot-npmrc.olay.base"), "base")
	writeFile(t, filepath.Join(src, "dot-npmrc.olay.work"), "work")

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			DotPrefix:  true,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		RenderRules: []config.RenderRule{{Path: "dot-npmrc", Strategy: config.RenderStrategyAppend}},
		Logger:      newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, ".npmrc"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "work" {
		t.Fatalf("content = %q, want default copy winner", data)
	}
}

func TestRunNoFilesFound(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Errorf("no files should not error, got: %v", err)
	}
}

func TestRunMissingSourceNoop(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{filepath.Join(root, "missing")},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Errorf("missing source should not error, got: %v", err)
	}
}

func TestRunFailFast(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "bad.olay.base.json"), `not valid json`)
	writeFile(t, filepath.Join(src, "good.olay.base.json"), `{"ok":true}`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRunContinueOnError(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "bad.olay.base.json"), `not valid json`)
	writeFile(t, filepath.Join(src, "good.olay.base.json"), `{"ok":true}`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		ContinueOnError: true,
		Logger:          newTestLogger(),
	})
	if err == nil {
		t.Error("expected error summary")
	}
	if _, err := os.Stat(filepath.Join(target, "good.json")); err != nil {
		t.Errorf("good file should have been written: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newSubstituter(pins map[string]string, environ ...string) *substitute.Resolver {
	return substitute.NewResolver([]string{"PRE_"}, pins, environ)
}

func TestRunSubstitutesAcrossModesAndFormats(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "settings.olay.base.json"), `{"color":"${PRE_BG}"}`)
	writeFile(t, filepath.Join(src, "settings.olay.work.json"), `{"extra":"${PRE_FG}"}`)
	writeFile(t, filepath.Join(src, "app.olay.base.toml"), `color = "${PRE_BG}"`)
	writeFile(t, filepath.Join(src, "look.olay.base.yml"), `color: ${PRE_BG}`)
	writeFile(t, filepath.Join(src, "theme.olay.base.conf"), "bg=${PRE_BG}\nlit=$${PRE_BG}\nhome=${HOME}\n")
	writeFile(t, filepath.Join(src, "dot-npmrc.olay.base"), "registry=${PRE_BG}\n")
	writeFile(t, filepath.Join(src, "dot-npmrc.olay.work"), "token=${PRE_FG}\n")

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			DotPrefix:  true,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		RenderRules: []config.RenderRule{{Path: ".npmrc", Strategy: config.RenderStrategyAppend}},
		Substituter: newSubstituter(map[string]string{"PRE_BG": "dark", "PRE_FG": "light"}),
		Logger:      newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertContent := func(name, want string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(target, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(data) != want {
			t.Errorf("%s = %q, want %q", name, data, want)
		}
	}
	assertContent("settings.json", "{\n  \"color\": \"dark\",\n  \"extra\": \"light\"\n}\n")
	assertContent("app.toml", "color = 'dark'\n")
	assertContent("look.yml", "color: dark\n")
	assertContent("theme.conf", "bg=dark\nlit=${PRE_BG}\nhome=${HOME}\n")
	assertContent(".npmrc", "registry=dark\ntoken=light\n")
}

func TestRunMergeSubstitutionRoundTripsFormatSensitiveStrings(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		output   string
		format   document.Format
		contents string
	}{
		{
			name:   "json",
			file:   "settings.olay.base.json",
			output: "settings.json",
			format: document.FormatJSON,
			contents: `{
  "hash": "${PRE_HASH}",
  "list": ["${PRE_HASH}"],
  "boolish": "${PRE_BOOL}",
  "nullish": "${PRE_NULL}",
  "quoted": "${PRE_QUOTED}"
}`,
		},
		{
			name:   "toml",
			file:   "settings.olay.base.toml",
			output: "settings.toml",
			format: document.FormatTOML,
			contents: `hash = "${PRE_HASH}"
list = ["${PRE_HASH}"]
boolish = "${PRE_BOOL}"
nullish = "${PRE_NULL}"
quoted = "${PRE_QUOTED}"
`,
		},
		{
			name:   "yaml",
			file:   "settings.olay.base.yaml",
			output: "settings.yaml",
			format: document.FormatYAML,
			contents: `hash: "${PRE_HASH}"
list:
  - "${PRE_HASH}"
boolish: "${PRE_BOOL}"
nullish: "${PRE_NULL}"
quoted: "${PRE_QUOTED}"
`,
		},
	}

	pins := map[string]string{
		"PRE_HASH":   "#626880",
		"PRE_BOOL":   "true",
		"PRE_NULL":   "null",
		"PRE_QUOTED": `a'"\\b`,
	}
	want := map[string]any{
		"hash":    pins["PRE_HASH"],
		"list":    []any{pins["PRE_HASH"]},
		"boolish": pins["PRE_BOOL"],
		"nullish": pins["PRE_NULL"],
		"quoted":  pins["PRE_QUOTED"],
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := t.TempDir()
			target := t.TempDir()
			writeFile(t, filepath.Join(src, tt.file), tt.contents)

			err := Run(Options{
				Settings: discover.Settings{
					SourceDirs: []string{src},
					TargetDir:  target,
					Ignore:     discover.NoopIgnorer(),
				},
				Substituter: newSubstituter(pins),
				Logger:      newTestLogger(),
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}

			data, err := os.ReadFile(filepath.Join(target, tt.output))
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			parsed, err := document.Parse(data, tt.format)
			if err != nil {
				t.Fatalf("re-parse output:\n%s\n%v", data, err)
			}
			if !reflect.DeepEqual(parsed, want) {
				t.Fatalf("parsed output = %#v, want %#v\nrendered:\n%s", parsed, want, data)
			}
		})
	}
}

func TestRunMergeSubstitutionUsesLayerValuesBeforeMerge(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "config.olay.base.yaml"), `items:
  - ${PRE_ITEM}
"${PRE_KEY}":
  from: base
`)
	writeFile(t, filepath.Join(src, "config.olay.work.yaml"), `items:
  - stable
theme:
  from: work
  extra: true
`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		Substituter: newSubstituter(map[string]string{"PRE_ITEM": "stable", "PRE_KEY": "theme"}),
		Logger:      newTestLogger(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "config.yaml"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	parsed, err := document.Parse(data, document.FormatYAML)
	if err != nil {
		t.Fatalf("re-parse output:\n%s\n%v", data, err)
	}
	want := map[string]any{
		"items": []any{"stable"},
		"theme": map[string]any{"extra": true, "from": "work"},
	}
	if !reflect.DeepEqual(parsed, want) {
		t.Fatalf("parsed output = %#v, want %#v\nrendered:\n%s", parsed, want, data)
	}
}

func TestRunMergeSubstitutionRequiresVarsInOverriddenLayers(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "config.olay.base.yaml"), "value: ${PRE_GONE}\n")
	writeFile(t, filepath.Join(src, "config.olay.work.yaml"), "value: ok\n")

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   []string{"work"},
			Ignore:     discover.NoopIgnorer(),
		},
		Substituter: newSubstituter(nil),
		Logger:      newTestLogger(),
	})
	if err == nil {
		t.Fatal("expected missing variable error")
	}
	if !strings.Contains(err.Error(), "PRE_GONE") || !strings.Contains(err.Error(), "missing variables") {
		t.Fatalf("error = %v, want missing PRE_GONE", err)
	}
}

func TestRunMergeSubstitutionRejectsSameMapKeyCollisions(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "config.olay.base.yaml"), `fixed: literal
"${PRE_KEY}": dynamic
`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Substituter: newSubstituter(map[string]string{"PRE_KEY": "fixed"}),
		Logger:      newTestLogger(),
	})
	if err == nil {
		t.Fatal("expected key collision error")
	}
	for _, want := range []string{"config.yaml", "key collision", "fixed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func TestRunTwoPhaseWritesNothingOnMissingVars(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "good.olay.base.conf"), "ok=${PRE_SET}\n")
	writeFile(t, filepath.Join(src, "bad.olay.base.conf"), "a=${PRE_GONE}\nb=${PRE_GONE2}\n")

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Profiles:   nil,
			Ignore:     discover.NoopIgnorer(),
		},
		Substituter: newSubstituter(map[string]string{"PRE_SET": "v"}),
		Logger:      newTestLogger(),
	})
	if err == nil {
		t.Fatal("expected missing-vars error")
	}
	for _, want := range []string{"bad.conf", "PRE_GONE", "PRE_GONE2", "missing variables"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
	entries, readErr := os.ReadDir(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Errorf("two-phase render must write nothing on failure, found %v", entries)
	}
}

func TestRunTwoPhaseAggregatesAllFailures(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "one.olay.base.conf"), "a=${PRE_GONE}\n")
	writeFile(t, filepath.Join(src, "two.olay.base.json"), `{not json`)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Substituter: newSubstituter(nil),
		Logger:      newTestLogger(),
	})
	if err == nil {
		t.Fatal("expected aggregated failures")
	}
	for _, want := range []string{"one.conf", "PRE_GONE", "two.json", "parse"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Errorf("nothing should be written, found %v", entries)
	}
}

func TestRunContinueWritesCleanTargets(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "good.olay.base.conf"), "ok=${PRE_SET}\n")
	writeFile(t, filepath.Join(src, "bad.olay.base.conf"), "a=${PRE_GONE}\n")

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		ContinueOnError: true,
		Substituter:     newSubstituter(map[string]string{"PRE_SET": "v"}),
		Logger:          newTestLogger(),
	})
	if err == nil {
		t.Fatal("expected summary error with --continue")
	}
	data, readErr := os.ReadFile(filepath.Join(target, "good.conf"))
	if readErr != nil {
		t.Fatalf("clean target should be written: %v", readErr)
	}
	if string(data) != "ok=v\n" {
		t.Errorf("good.conf = %q", data)
	}
	if _, statErr := os.Stat(filepath.Join(target, "bad.conf")); statErr == nil {
		t.Error("failing target must not be written")
	}
}

func TestRunSubstituteExcludeOptOut(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "raw.olay.base.sh"), "echo ${PRE_SET}\n")
	writeFile(t, filepath.Join(src, "themed.olay.base.conf"), "bg=${PRE_SET}\n")
	exclude, err := discover.NewGlobIgnorer([]string{"raw.sh"})
	if err != nil {
		t.Fatal(err)
	}

	err = Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Substituter:       newSubstituter(map[string]string{"PRE_SET": "v"}),
		SubstituteExclude: exclude,
		Logger:            newTestLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(target, "raw.sh")); string(data) != "echo ${PRE_SET}\n" {
		t.Errorf("excluded target was substituted: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(target, "themed.conf")); string(data) != "bg=v\n" {
		t.Errorf("non-excluded target should substitute: %q", data)
	}
}

func TestRunFeatureOffByteIdentical(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	content := "a=${PRE_SET}\nb=$${PRE_SET}\nc=$$\n"
	writeFile(t, filepath.Join(src, "raw.olay.base.conf"), content)

	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Logger: newTestLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(target, "raw.conf"))
	if string(data) != content {
		t.Errorf("feature off must be byte-identical: %q", data)
	}
}

func TestRunWarnsUnusedPins(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "a.olay.base.conf"), "x=${PRE_USED}\n")

	var buf strings.Builder
	logger := log.New(&buf)
	logger.SetLevel(log.WarnLevel)
	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Substituter: newSubstituter(map[string]string{"PRE_USED": "1", "PRE_UNUSED": "2"}),
		Logger:      logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "pinned variable PRE_UNUSED was not consumed") {
		t.Errorf("expected unused-pin warning, got: %q", out)
	}
	if strings.Contains(out, "pinned variable PRE_USED was not consumed") {
		t.Errorf("consumed pin should not warn: %q", out)
	}
}

func TestRunWarnsUnusedPinsWhenNoGroups(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()

	var buf strings.Builder
	logger := log.New(&buf)
	logger.SetLevel(log.WarnLevel)
	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Substituter: newSubstituter(map[string]string{"PRE_ORPHAN": "1"}),
		Logger:      logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "pinned variable PRE_ORPHAN was not consumed") {
		t.Errorf("a pin should warn even when no targets exist, got: %q", buf.String())
	}
}

func TestRunWarnsUnusedPinsWhenOnlyMissingVarsFail(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "good.olay.base.conf"), "x=${PRE_USED}\n")
	writeFile(t, filepath.Join(src, "bad.olay.base.conf"), "y=${PRE_GONE}\n")

	var buf strings.Builder
	logger := log.New(&buf)
	logger.SetLevel(log.WarnLevel)
	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		ContinueOnError: true,
		Substituter:     newSubstituter(map[string]string{"PRE_USED": "1", "PRE_UNUSED": "2"}),
		Logger:          logger,
	})
	if err == nil {
		t.Fatal("expected the missing-var target to fail the run")
	}
	// A missing-var failure still ran substitution, so the consumed set is
	// complete and the unused-pin warning must fire.
	if !strings.Contains(buf.String(), "pinned variable PRE_UNUSED was not consumed") {
		t.Errorf("missing-var failures must not suppress the unused-pin warning: %q", buf.String())
	}
}

func TestRunSuppressesUnusedPinWarningOnComposeFailure(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "broken.olay.base.json"), `{not json`)

	var buf strings.Builder
	logger := log.New(&buf)
	logger.SetLevel(log.WarnLevel)
	err := Run(Options{
		Settings: discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		},
		Substituter: newSubstituter(map[string]string{"PRE_MAYBE": "1"}),
		Logger:      logger,
	})
	if err == nil {
		t.Fatal("expected compose failure")
	}
	if strings.Contains(buf.String(), "not consumed") {
		t.Errorf("unused-pin warning must be suppressed when a target failed before substitution: %q", buf.String())
	}
}

func TestComposeGroupReportsVarsOnFailure(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.olay.base.conf"), "x=${PRE_GONE}\ny=${PRE_SET}\n")
	groups, _, err := discover.Walk(discover.Settings{
		SourceDirs: []string{src},
		TargetDir:  t.TempDir(),
		Ignore:     discover.NoopIgnorer(),
	})
	if err != nil || len(groups) != 1 {
		t.Fatalf("walk: %v (%d groups)", err, len(groups))
	}
	cg := ComposeGroup(groups[0], MergeOptions{
		Substituter: newSubstituter(map[string]string{"PRE_SET": "v"}),
	})
	var missing *MissingVarsError
	if !errors.As(cg.Err, &missing) {
		t.Fatalf("expected MissingVarsError, got %v", cg.Err)
	}
	if !reflect.DeepEqual(cg.Vars.Consumed, []string{"PRE_GONE", "PRE_SET"}) {
		t.Errorf("consumed = %v", cg.Vars.Consumed)
	}
	if !reflect.DeepEqual(cg.Vars.Missing, []string{"PRE_GONE"}) {
		t.Errorf("missing = %v", cg.Vars.Missing)
	}
}
