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
}

// grammarOutcome is what a row of the grammar inventory expects when it runs
// with --config pointing at a file that does not exist.
type grammarOutcome int

const (
	// succeedsWithoutConfig exits 0 with a payload and never loads the config.
	succeedsWithoutConfig grammarOutcome = iota
	// reachesConfigLoad fails naming the missing config file, so the operands
	// were accepted and the runner ran.
	reachesConfigLoad
	// rejectsLastOperand fails naming the last operand and never mentions the
	// config file, so rejection happened before the runner.
	rejectsLastOperand
)

func TestEveryApplicationCommandDeclaresPositionalGrammar(t *testing.T) {
	const missingConfig = "/nonexistent/overlay-grammar/.overlay.toml"
	rows := []struct {
		args    []string
		outcome grammarOutcome
	}{
		{args: nil, outcome: succeedsWithoutConfig},
		{args: []string{"nosuch"}, outcome: rejectsLastOperand},
		{args: []string{"render"}, outcome: reachesConfigLoad},
		{args: []string{"render", "pi"}, outcome: reachesConfigLoad},
		{args: []string{"render", "pi", "codex"}, outcome: reachesConfigLoad},
		{args: []string{"diff"}, outcome: reachesConfigLoad},
		{args: []string{"diff", "pi"}, outcome: reachesConfigLoad},
		{args: []string{"diff", "pi", "codex"}, outcome: reachesConfigLoad},
		{args: []string{"orphans"}, outcome: reachesConfigLoad},
		{args: []string{"orphans", "pi"}, outcome: reachesConfigLoad},
		{args: []string{"orphans", "pi", "codex"}, outcome: reachesConfigLoad},
		{args: []string{"plan"}, outcome: reachesConfigLoad},
		{args: []string{"plan", "pi"}, outcome: reachesConfigLoad},
		{args: []string{"plan", "pi", "codex"}, outcome: reachesConfigLoad},
		{args: []string{"config"}, outcome: reachesConfigLoad},
		{args: []string{"config", "extra"}, outcome: rejectsLastOperand},
		{args: []string{"docs"}, outcome: succeedsWithoutConfig},
		{args: []string{"docs", "extra"}, outcome: rejectsLastOperand},
		{args: []string{"exit-codes"}, outcome: succeedsWithoutConfig},
		{args: []string{"exit-codes", "extra"}, outcome: rejectsLastOperand},
	}

	root := newRootCmd()
	covered := map[string]bool{}
	for _, row := range rows {
		command, _, err := root.Find(row.args)
		if err != nil {
			t.Fatalf("find %v: %v", row.args, err)
		}
		covered[command.CommandPath()] = true
	}
	for _, command := range applicationCommands(root) {
		if command.Args == nil {
			t.Errorf("%q has no Args validator", command.CommandPath())
		}
		if !covered[command.CommandPath()] {
			t.Errorf("%q has no row in the grammar inventory", command.CommandPath())
		}
	}

	for _, row := range rows {
		name := strings.Join(append([]string{"overlay"}, row.args...), " ")
		t.Run(name, func(t *testing.T) {
			result := runRoot(t, append(row.args, "--config", missingConfig)...)

			switch row.outcome {
			case succeedsWithoutConfig:
				if result.code != 0 || result.stdout == "" {
					t.Fatalf("exit = %d stdout %q, want 0 with a payload; stderr:\n%s", result.code, result.stdout, result.stderr)
				}
			case reachesConfigLoad:
				if result.code == 0 || !strings.Contains(result.stderr, missingConfig) {
					t.Fatalf("exit = %d, want nonzero naming %q; stderr:\n%s", result.code, missingConfig, result.stderr)
				}
			case rejectsLastOperand:
				operand := row.args[len(row.args)-1]
				if result.code == 0 || !strings.Contains(result.stderr, operand) {
					t.Fatalf("exit = %d, want nonzero naming %q; stderr:\n%s", result.code, operand, result.stderr)
				}
				if strings.Contains(result.stderr, missingConfig) {
					t.Fatalf("rejected operand reached config loading; stderr:\n%s", result.stderr)
				}
			}
		})
	}
}

func TestEveryApplicationCommandHasWrappedLongHelp(t *testing.T) {
	for _, command := range applicationCommands(newRootCmd()) {
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
	}
}

// applicationCommands returns the root and every command it owns, skipping
// Cobra's help and completion commands.
func applicationCommands(root *cobra.Command) []*cobra.Command {
	var commands []*cobra.Command
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if command.Name() == "help" || command.Name() == "completion" {
			return
		}
		commands = append(commands, command)
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
	return commands
}
