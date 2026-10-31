package cli

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

func TestExecuteReturnsStatusAndReportsErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.toml")
	tests := []struct {
		name       string
		args       []string
		wantStatus int
		wantText   string
	}{
		{name: "success", args: []string{"exit-codes"}},
		{name: "runner failure", args: []string{"config", "--config", missing}, wantStatus: 1, wantText: missing},
		{name: "unknown command", args: []string{"bogus"}, wantStatus: 2, wantText: `"bogus"`},
		{name: "unknown operand", args: []string{"render", "extra"}, wantStatus: 2, wantText: `"extra"`},
		{name: "unknown flag", args: []string{"render", "--bogus"}, wantStatus: 2, wantText: "--bogus"},
		{name: "flag value that does not parse", args: []string{"render", "--color", "sometimes"}, wantStatus: 2, wantText: `"sometimes"`},
		{name: "unknown help topic", args: []string{"help", "bogus"}, wantStatus: 2, wantText: `"bogus"`},
		{name: "extra help operand", args: []string{"help", "render", "extra"}, wantStatus: 2, wantText: `"render extra"`},
		{name: "completion grammar", args: []string{"completion", "bash", "extra"}, wantStatus: 2, wantText: `"extra"`},
		{name: "unknown command after version", args: []string{"--version", "bogus"}, wantStatus: 2, wantText: `"bogus"`},
		{name: "root operand after delimiter", args: []string{"--", "bogus"}, wantStatus: 2, wantText: `"bogus"`},
		{name: "version operand after delimiter", args: []string{"--version", "--", "bogus"}, wantStatus: 2, wantText: `"bogus"`},
		{name: "empty version operand", args: []string{"--version", ""}, wantStatus: 2, wantText: `""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer

			status := Execute(tt.args, &out, &errOut)

			if status != tt.wantStatus {
				t.Errorf("Execute status = %d, want %d", status, tt.wantStatus)
			}
			if tt.wantStatus == 0 {
				if out.Len() == 0 || errOut.Len() != 0 {
					t.Errorf("successful Execute stdout = %q, stderr = %q", out.String(), errOut.String())
				}
				return
			}
			stderr := errOut.String()
			if out.Len() != 0 || !strings.HasPrefix(stderr, "overlay: ") || strings.Contains(stderr, "rror:") ||
				strings.Contains(stderr, "Usage:") || !strings.Contains(stderr, tt.wantText) || strings.Count(stderr, "\n") != 1 {
				t.Errorf("failed Execute stdout = %q, stderr = %q, want one 'overlay: ' line naming %q and no usage", out.String(), stderr, tt.wantText)
			}
		})
	}
}

func TestExitCodesTopicListsEveryStatus(t *testing.T) {
	help, errOut, err := executeForTest(t, "help", "exit-codes")
	if err != nil {
		t.Fatalf("help exit-codes error = %v, stderr = %s", err, errOut)
	}

	for _, want := range []string{"\n  0  ", "\n  1  ", "\n  2  ", "'overlay: <message>'"} {
		if !strings.Contains(help, want) {
			t.Errorf("exit-codes help missing %q:\n%s", want, help)
		}
	}
}

func TestTemplatesTopicDocumentsEveryFunctionAndTheDirective(t *testing.T) {
	help, errOut, err := executeForTest(t, "help", "templates")
	if err != nil {
		t.Fatalf("help templates error = %v, stderr = %s", err, errOut)
	}

	for _, want := range []string{
		"\n  .Layers ", "\n  .Sources ", "\n  .Home ", "\n  .OS ", "\n  .Hostname ", "\n  .Values ",
		"\n  fileExists PATH ", "\n  files PATTERN... ", "\n  hasLayer NAME ",
		"\n  join SEP LIST ", "\n  quote S ", "\n  readFile PATH ", "\n  shellQuote S ",
		"overlay:delims LEFT RIGHT", "Machine-specific input belongs in .Values",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("templates help missing %q:\n%s", want, help)
		}
	}
}

// TestEveryApplicationCommandMeetsTheHelpBaseline renders each command's
// help as --help does, so the Usage line and flag usage count toward the
// width limit.
func TestEveryApplicationCommandMeetsTheHelpBaseline(t *testing.T) {
	root := newRootCommand(&bytes.Buffer{}, &bytes.Buffer{})

	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		path := strings.Fields(command.CommandPath())[1:]
		if strings.TrimSpace(command.Short) == "" {
			t.Errorf("%q has no short help", command.CommandPath())
		}
		if strings.TrimSpace(command.Long) == "" {
			t.Errorf("%q has no long help", command.CommandPath())
		}

		help, errOut, err := executeForTest(t, append(path, "--help")...)
		if err != nil {
			t.Fatalf("%q --help error = %v, stderr = %s", command.CommandPath(), err, errOut)
		}
		for i, line := range strings.Split(help, "\n") {
			if width := utf8.RuneCountInString(line); width > 80 {
				t.Errorf("%q help line %d is %d columns, want at most 80: %q", command.CommandPath(), i+1, width, line)
			}
		}

		if isHelpTopic(command) {
			direct, errOut, err := executeForTest(t, path...)
			if err != nil {
				t.Fatalf("%q error = %v, stderr = %s", command.CommandPath(), err, errOut)
			}
			viaHelp, errOut, err := executeForTest(t, append([]string{"help"}, path...)...)
			if err != nil {
				t.Fatalf("help %q error = %v, stderr = %s", command.CommandPath(), err, errOut)
			}
			if direct != viaHelp {
				t.Errorf("%q prints different text directly and through help:\n%s\n---\n%s", command.CommandPath(), direct, viaHelp)
			}
		}

		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
}

func TestEveryApplicationCommandDeclaresItsGrammar(t *testing.T) {
	root := newRootCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if root.Args != nil {
		t.Error("root declares an Args validator; command discovery must reject unknown commands")
	}
	if root.Version != "" {
		t.Errorf("root Version = %q; version handling must belong to overlay", root.Version)
	}

	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if command != root && command.Args == nil {
			t.Errorf("%q has no Args validator", command.CommandPath())
		}
		if command != root && (command.HasSubCommands() || isHelpTopic(command)) && !command.Runnable() {
			t.Errorf("%q is a command group or help topic without a runner", command.CommandPath())
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
}

// isHelpTopic reports whether command belongs to the help topics group.
func isHelpTopic(command *cobra.Command) bool {
	return command.GroupID == helpTopicsGroupID
}

func TestRootVersionDoesNotLoadConfiguration(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "overlay", "config.toml"), "invalid TOML [")
	t.Setenv("XDG_CONFIG_HOME", dir)

	stdout, stderr, err := executeForTest(t, "--version")

	if err != nil || stderr != "" || stdout != "overlay version "+Version+"\n" {
		t.Errorf("--version stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
}

func TestRootHelpGroupsTopicsSeparately(t *testing.T) {
	stdout, stderr, err := executeForTest(t)
	if err != nil || stderr != "" {
		t.Fatalf("root stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}

	_, topics, ok := strings.Cut(stdout, "Help topics:\n")
	if !ok || !strings.HasPrefix(topics, "  exit-codes") {
		t.Errorf("root help does not group exit-codes under Help topics:\n%s", stdout)
	}
}

func TestExecutePrintsAMultiLineFailureAsOneLine(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	writeConfig(t, configPath, "layers = []\n")
	t.Setenv("OVERLAY_LAYERS", "")
	var errOut bytes.Buffer

	status := Execute([]string{"config", "--config", configPath}, failingWriter{}, &errOut)

	if status != exitFailure {
		t.Errorf("Execute status = %d, want %d", status, exitFailure)
	}
	if got, want := errOut.String(), "overlay: output unavailable: disk full\n"; got != want {
		t.Errorf("Execute stderr = %q, want %q", got, want)
	}
}

// failingWriter fails every write with a two-line message.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("output unavailable:\ndisk full")
}

func executeForTest(t *testing.T, args ...string) (stdout string, stderr string, err error) {
	t.Helper()

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}
