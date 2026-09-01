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
	"github.com/jmcampanini/overlay/internal/state"
	"github.com/jmcampanini/overlay/internal/substitute"
)

func newTestLogger() *log.Logger {
	l := log.New(os.Stderr)
	l.SetLevel(log.FatalLevel)
	return l
}

func runTest(t *testing.T, opts Options) error {
	t.Helper()
	if opts.StatePath == "" {
		opts.StatePath = filepath.Join(t.TempDir(), ".overlay.state.json")
	}
	return Run(opts)
}

func TestRunBasic(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(src, "dot-claude", "settings.olay.base.json"), `{"a":1,"nested":{"b":2}}`)
	writeFile(t, filepath.Join(src, "dot-claude", "settings.olay.work.json"), `{"nested":{"c":3}}`)

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

			err := runTest(t, Options{
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

			err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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
	err := runTest(t, Options{
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
	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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
	return substitute.NewResolver([]string{"PRE_*"}, pins, environ)
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

	err := runTest(t, Options{
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

			err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err := runTest(t, Options{
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

	err = runTest(t, Options{
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

	err := runTest(t, Options{
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
	err := runTest(t, Options{
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
	err := runTest(t, Options{
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
	err := runTest(t, Options{
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
	err := runTest(t, Options{
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

func TestRunStateLifecycle(t *testing.T) {
	t.Run("corrupt state aborts before writing", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		statePath := filepath.Join(t.TempDir(), ".overlay.state.json")
		writeFile(t, filepath.Join(src, "a.olay.base.conf"), "a\n")
		writeFile(t, statePath, "{")
		before, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}

		err = Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			Logger:    newTestLogger(),
		})
		if err == nil || !strings.Contains(err.Error(), "invalid state file") {
			t.Fatalf("error = %v, want invalid state failure", err)
		}
		if _, statErr := os.Stat(filepath.Join(target, "a.conf")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("target must not be written, stat error = %v", statErr)
		}
		after, readErr := os.ReadFile(statePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("corrupt state changed: before %q after %q", before, after)
		}
	})

	t.Run("no-state ignores corrupt state and leaves it byte identical", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		statePath := filepath.Join(t.TempDir(), ".overlay.state.json")
		writeFile(t, filepath.Join(src, "a.olay.base.conf"), "a\n")
		writeFile(t, statePath, "{")
		before, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}

		err = Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			NoState:   true,
			Logger:    newTestLogger(),
		})
		if err != nil {
			t.Fatal(err)
		}
		contents, readErr := os.ReadFile(filepath.Join(target, "a.conf"))
		if readErr != nil || string(contents) != "a\n" {
			t.Fatalf("target = %q, %v", contents, readErr)
		}
		after, readErr := os.ReadFile(statePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("corrupt state changed: before %q after %q", before, after)
		}
	})

	t.Run("state path is required only when maintaining state", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		writeFile(t, filepath.Join(src, "a.olay.base.conf"), "a\n")
		settings := discover.Settings{
			SourceDirs: []string{src},
			TargetDir:  target,
			Ignore:     discover.NoopIgnorer(),
		}
		if err := Run(Options{Settings: settings, Logger: newTestLogger()}); err == nil || !strings.Contains(err.Error(), "state path is required") {
			t.Fatalf("stateful error = %v, want required state path", err)
		}
		if err := Run(Options{Settings: settings, NoState: true, Logger: newTestLogger()}); err != nil {
			t.Fatalf("stateless Run: %v", err)
		}
		contents, err := os.ReadFile(filepath.Join(target, "a.conf"))
		if err != nil || string(contents) != "a\n" {
			t.Fatalf("stateless target = %q, %v", contents, err)
		}
	})

	t.Run("no-state zero groups leaves sidecar absent", func(t *testing.T) {
		statePath := filepath.Join(t.TempDir(), ".overlay.state.json")
		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{t.TempDir()},
				TargetDir:  t.TempDir(),
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			NoState:   true,
			Logger:    newTestLogger(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, statErr := os.Stat(statePath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("state sidecar must remain absent, stat error = %v", statErr)
		}
	})

	t.Run("no-state continue writes clean targets without partial claims", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		statePath := filepath.Join(t.TempDir(), ".overlay.state.json")
		legacy := filepath.Join(target, "legacy.conf")
		writeFile(t, legacy, "legacy\n")
		if err := state.Save(statePath, []state.Entry{{Target: legacy, Source: src}}); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(src, "bad.olay.base.json"), "{")
		writeFile(t, filepath.Join(src, "good.olay.base.conf"), "good\n")

		err = Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				Ignore:     discover.NoopIgnorer(),
			},
			ContinueOnError: true,
			StatePath:       statePath,
			NoState:         true,
			Logger:          newTestLogger(),
		})
		if err == nil {
			t.Fatal("expected render summary error")
		}
		contents, readErr := os.ReadFile(filepath.Join(target, "good.conf"))
		if readErr != nil || string(contents) != "good\n" {
			t.Fatalf("clean target = %q, %v", contents, readErr)
		}
		after, readErr := os.ReadFile(statePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("state changed on stateless partial render:\nbefore: %s\nafter: %s", before, after)
		}
	})

	t.Run("no-state still rejects state path collision", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		statePath := filepath.Join(target, ".overlay.state.json")
		writeFile(t, filepath.Join(src, "dot-overlay.state.olay.base.json"), `{"value":true}`)

		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				DotPrefix:  true,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			NoState:   true,
			Logger:    newTestLogger(),
		})
		if err == nil || !strings.Contains(err.Error(), "collides with a rendered target") {
			t.Fatalf("error = %v, want state target collision", err)
		}
		if _, statErr := os.Stat(statePath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("colliding target must not be written, stat error = %v", statErr)
		}
	})

	t.Run("success claims every output", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		statePath := filepath.Join(t.TempDir(), ".overlay.state.json")
		writeFile(t, filepath.Join(src, "a.olay.base.conf"), "a\n")
		writeFile(t, filepath.Join(src, "b.olay.base.conf"), "b\n")

		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			Logger:    newTestLogger(),
		})
		if err != nil {
			t.Fatal(err)
		}

		entries := loadState(t, statePath)
		want := []state.Entry{
			{Target: filepath.Join(target, "a.conf"), Source: src},
			{Target: filepath.Join(target, "b.conf"), Source: src},
		}
		if !reflect.DeepEqual(entries, want) {
			t.Fatalf("state entries = %#v, want %#v", entries, want)
		}
	})

	t.Run("relative target is recorded as absolute", func(t *testing.T) {
		root := t.TempDir()
		t.Chdir(root)
		src := t.TempDir()
		statePath := filepath.Join(t.TempDir(), ".overlay.state.json")
		writeFile(t, filepath.Join(src, "a.olay.base.conf"), "a\n")

		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  "target",
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			Logger:    newTestLogger(),
		})
		if err != nil {
			t.Fatal(err)
		}
		entries := loadState(t, statePath)
		want := filepath.Join(root, "target", "a.conf")
		if len(entries) != 1 || entries[0].Target != want {
			t.Fatalf("state entries = %#v, want target %q", entries, want)
		}
	})

	t.Run("state path collision writes nothing", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		statePath := filepath.Join(target, ".overlay.state.json")
		writeFile(t, filepath.Join(src, "dot-overlay.state.olay.base.json"), `{"value":true}`)

		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				DotPrefix:  true,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			Logger:    newTestLogger(),
		})
		if err == nil || !strings.Contains(err.Error(), "collides with a rendered target") {
			t.Fatalf("error = %v, want state target collision", err)
		}
		if _, statErr := os.Stat(statePath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("colliding target must not be written, stat error = %v", statErr)
		}
	})

	t.Run("state path alias collision writes nothing", func(t *testing.T) {
		src := t.TempDir()
		stateDir := t.TempDir()
		alias := filepath.Join(t.TempDir(), "target-link")
		if err := os.Symlink(stateDir, alias); err != nil {
			t.Fatal(err)
		}
		statePath := filepath.Join(stateDir, ".overlay.state.json")
		writeFile(t, filepath.Join(src, "dot-overlay.state.olay.base.json"), `{"value":true}`)

		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  alias,
				DotPrefix:  true,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			Logger:    newTestLogger(),
		})
		if err == nil || !strings.Contains(err.Error(), "collides with a rendered target") {
			t.Fatalf("error = %v, want aliased state target collision", err)
		}
		if _, statErr := os.Stat(statePath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("colliding target must not be written, stat error = %v", statErr)
		}
	})

	t.Run("dangling target symlink to state path writes nothing", func(t *testing.T) {
		src := t.TempDir()
		stateDir := t.TempDir()
		target := t.TempDir()
		statePath := filepath.Join(stateDir, ".overlay.state.json")
		targetPath := filepath.Join(target, ".overlay.state.json")
		if err := os.Symlink(statePath, targetPath); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(src, "dot-overlay.state.olay.base.json"), `{"value":true}`)

		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				DotPrefix:  true,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			Logger:    newTestLogger(),
		})
		if err == nil || !strings.Contains(err.Error(), "collides with a rendered target") {
			t.Fatalf("error = %v, want dangling state target collision", err)
		}
		if _, statErr := os.Stat(statePath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("state path must not be written, stat error = %v", statErr)
		}
		info, lstatErr := os.Lstat(targetPath)
		if lstatErr != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("target symlink changed, info = %v, error = %v", info, lstatErr)
		}
	})

	t.Run("state path case alias collision writes nothing", func(t *testing.T) {
		stateDir := t.TempDir()
		writeFile(t, filepath.Join(stateDir, "CaseProbe"), "probe\n")
		if !caseInsensitiveDirectory(stateDir) {
			t.Skip("filesystem is case-sensitive")
		}
		src := t.TempDir()
		statePath := filepath.Join(stateDir, ".overlay.state.json")
		writeFile(t, filepath.Join(src, "dot-Overlay.state.olay.base.json"), `{"value":true}`)

		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  stateDir,
				DotPrefix:  true,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			Logger:    newTestLogger(),
		})
		if err == nil || !strings.Contains(err.Error(), "collides with a rendered target") {
			t.Fatalf("error = %v, want case-aliased state target collision", err)
		}
		if _, statErr := os.Stat(statePath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("colliding target must not be written, stat error = %v", statErr)
		}
	})

	t.Run("zero groups initializes state", func(t *testing.T) {
		statePath := filepath.Join(t.TempDir(), ".overlay.state.json")
		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{t.TempDir()},
				TargetDir:  t.TempDir(),
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			Logger:    newTestLogger(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if entries := loadState(t, statePath); len(entries) != 0 {
			t.Fatalf("state entries = %#v, want empty", entries)
		}
	})

	t.Run("fail fast leaves state byte identical", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		statePath := filepath.Join(t.TempDir(), ".overlay.state.json")
		legacy := filepath.Join(target, "legacy.conf")
		writeFile(t, legacy, "legacy\n")
		if err := state.Save(statePath, []state.Entry{{Target: legacy, Source: src}}); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(src, "bad.olay.base.json"), "{not json")
		writeFile(t, filepath.Join(src, "good.olay.base.conf"), "good\n")

		err = Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			Logger:    newTestLogger(),
		})
		if err == nil {
			t.Fatal("expected compose failure")
		}
		after, readErr := os.ReadFile(statePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("state changed on fail-fast compose:\nbefore: %s\nafter: %s", before, after)
		}
		if _, statErr := os.Stat(filepath.Join(target, "good.conf")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("good target must not be written, stat error = %v", statErr)
		}
	})

	t.Run("continue claims clean subset and retains failed ownership", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		statePath := filepath.Join(t.TempDir(), ".overlay.state.json")
		badTarget := filepath.Join(target, "bad.json")
		writeFile(t, badTarget, "old\n")
		if err := state.Save(statePath, []state.Entry{{Target: badTarget, Source: src}}); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(src, "bad.olay.base.json"), "{not json")
		writeFile(t, filepath.Join(src, "good.olay.base.conf"), "good\n")

		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				Ignore:     discover.NoopIgnorer(),
			},
			ContinueOnError: true,
			StatePath:       statePath,
			Logger:          newTestLogger(),
		})
		if err == nil {
			t.Fatal("expected render summary error")
		}
		entries := loadState(t, statePath)
		want := []state.Entry{
			{Target: badTarget, Source: src},
			{Target: filepath.Join(target, "good.conf"), Source: src},
		}
		if !reflect.DeepEqual(entries, want) {
			t.Fatalf("state entries = %#v, want %#v", entries, want)
		}
	})

	t.Run("write error saves prefix and retains unattempted ownership", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		statePath := filepath.Join(t.TempDir(), ".overlay.state.json")
		writeFile(t, filepath.Join(src, "a.olay.base.conf"), "a\n")
		writeFile(t, filepath.Join(src, "b.olay.base.conf"), "b\n")
		writeFile(t, filepath.Join(src, "c.olay.base.conf"), "new c\n")
		if err := os.Mkdir(filepath.Join(target, "b.conf"), 0o755); err != nil {
			t.Fatal(err)
		}
		cTarget := filepath.Join(target, "c.conf")
		writeFile(t, cTarget, "old c\n")
		if err := state.Save(statePath, []state.Entry{{Target: cTarget, Source: src}}); err != nil {
			t.Fatal(err)
		}

		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: statePath,
			Logger:    newTestLogger(),
		})
		if err == nil {
			t.Fatal("expected write failure")
		}
		entries := loadState(t, statePath)
		want := []state.Entry{
			{Target: filepath.Join(target, "a.conf"), Source: src},
			{Target: cTarget, Source: src},
		}
		if !reflect.DeepEqual(entries, want) {
			t.Fatalf("state entries = %#v, want %#v", entries, want)
		}
		data, readErr := os.ReadFile(cTarget)
		if readErr != nil || string(data) != "old c\n" {
			t.Fatalf("unattempted target = %q, %v", data, readErr)
		}
	})

	t.Run("state save failure follows successful writes", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		writeFile(t, filepath.Join(src, "blocker.olay.base"), "written\n")
		output := filepath.Join(target, "blocker")

		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: filepath.Join(output, ".overlay.state.json"),
			Logger:    newTestLogger(),
		})
		if err == nil || !strings.Contains(err.Error(), "save state") {
			t.Fatalf("error = %v, want state save failure", err)
		}
		data, readErr := os.ReadFile(output)
		if readErr != nil || string(data) != "written\n" {
			t.Fatalf("output = %q, %v", data, readErr)
		}
	})

	t.Run("render and state save failures both surface", func(t *testing.T) {
		src := t.TempDir()
		target := t.TempDir()
		writeFile(t, filepath.Join(src, "a.olay.base"), "a\n")
		writeFile(t, filepath.Join(src, "b.olay.base"), "b\n")
		if err := os.Mkdir(filepath.Join(target, "b"), 0o755); err != nil {
			t.Fatal(err)
		}

		err := Run(Options{
			Settings: discover.Settings{
				SourceDirs: []string{src},
				TargetDir:  target,
				Ignore:     discover.NoopIgnorer(),
			},
			StatePath: filepath.Join(target, "a", ".overlay.state.json"),
			Logger:    newTestLogger(),
		})
		if err == nil {
			t.Fatal("expected render and state save failures")
		}
		for _, want := range []string{filepath.Join(target, "b"), "save state"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %v, want %q", err, want)
			}
		}
	})
}

func loadState(t *testing.T, path string) []state.Entry {
	t.Helper()
	entries, err := state.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return entries
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
