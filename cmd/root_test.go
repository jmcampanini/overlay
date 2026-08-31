package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type rootResult struct {
	code   int
	stdout string
	stderr string
}

func runRoot(t *testing.T, args ...string) rootResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := newRootCmd()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return rootResult{stdout: stdout.String(), stderr: stderr.String()}
	}
	var code ExitCode
	if errors.As(err, &code) {
		return rootResult{code: int(code), stdout: stdout.String(), stderr: stderr.String()}
	}
	_, _ = fmt.Fprintln(&stderr, "overlay:", err)
	return rootResult{code: 1, stdout: stdout.String(), stderr: stderr.String()}
}

func TestExitCodesTopicPrintsSameHelpFromBothEntryPoints(t *testing.T) {
	direct := runRoot(t, "exit-codes")
	if direct.code != 0 {
		t.Fatalf("exit-codes exit = %d, want 0; stderr:\n%s", direct.code, direct.stderr)
	}
	viaHelp := runRoot(t, "help", "exit-codes")
	if viaHelp.code != 0 {
		t.Fatalf("help exit-codes exit = %d, want 0; stderr:\n%s", viaHelp.code, viaHelp.stderr)
	}

	if direct.stdout != viaHelp.stdout {
		t.Fatalf("exit-codes output differs between entry points:\n%s\n---\n%s", direct.stdout, viaHelp.stdout)
	}
	for _, want := range []string{"\n  0  ", "\n  1  ", "\n  2  "} {
		if !strings.Contains(direct.stdout, want) {
			t.Fatalf("exit-codes help missing %q:\n%s", want, direct.stdout)
		}
	}

	extra := runRoot(t, "exit-codes", "extra")
	if extra.code == 0 || !strings.Contains(extra.stderr, "extra") {
		t.Fatalf("exit-codes extra = exit %d stderr %q, want nonzero exit naming the operand", extra.code, extra.stderr)
	}
}

func TestEveryApplicationCommandHasWrappedLongHelp(t *testing.T) {
	root := newRootCmd()

	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if command.Name() == "help" || command.Name() == "completion" {
			return
		}
		if strings.TrimSpace(command.Long) == "" {
			t.Errorf("%q has no long help", command.CommandPath())
		}
		for field, text := range map[string]string{"Long": command.Long, "Example": command.Example} {
			for i, line := range strings.Split(text, "\n") {
				if len(line) > 80 {
					t.Errorf("%q %s line %d is %d columns, want at most 80: %q", command.CommandPath(), field, i+1, len(line), line)
				}
			}
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
}
