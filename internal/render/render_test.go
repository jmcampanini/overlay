package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/log/v2"

	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/discover"
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
