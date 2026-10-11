package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestExitCodesTopicPrintsSameHelpFromBothEntryPoints(t *testing.T) {
	direct, errOut, err := executeForTest(t, "exit-codes")
	if err != nil {
		t.Fatalf("exit-codes error = %v, stderr = %s", err, errOut)
	}
	viaHelp, errOut, err := executeForTest(t, "help", "exit-codes")
	if err != nil {
		t.Fatalf("help exit-codes error = %v, stderr = %s", err, errOut)
	}

	if direct != viaHelp {
		t.Fatalf("exit-codes output differs between entry points:\n%s\n---\n%s", direct, viaHelp)
	}
	for _, want := range []string{"\n  0  ", "\n  1  ", "overlay: error:"} {
		if !strings.Contains(direct, want) {
			t.Errorf("exit-codes help missing %q:\n%s", want, direct)
		}
	}
}

func TestEveryApplicationCommandHasWrappedLongHelp(t *testing.T) {
	root := NewRootCommand(&bytes.Buffer{}, &bytes.Buffer{})

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

func TestEveryApplicationCommandDeclaresItsGrammar(t *testing.T) {
	root := NewRootCommand(&bytes.Buffer{}, &bytes.Buffer{})

	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if command.Name() == "help" || command.Name() == "completion" {
			return
		}
		if command != root && command.Args == nil {
			t.Errorf("%q has no Args validator", command.CommandPath())
		}
		if command != root && command.HasSubCommands() && command.RunE == nil {
			t.Errorf("%q is a command group without a runner", command.CommandPath())
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
}

func executeForTest(t *testing.T, args ...string) (stdout string, stderr string, err error) {
	t.Helper()

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}
