package cli

import (
	"strings"
	"testing"
)

func TestCompletionWritesTheSelectedShellScript(t *testing.T) {
	tests := []struct {
		shell string
		want  string
	}{
		{shell: "bash", want: "# bash completion V2 for overlay"},
		{shell: "fish", want: "# fish completion for overlay"},
		{shell: "powershell", want: "# powershell completion for overlay"},
		{shell: "zsh", want: "#compdef overlay"},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			stdout, stderr, err := executeForTest(t, "--color=always", "completion", tt.shell)

			if err != nil {
				t.Fatalf("completion %s error = %v", tt.shell, err)
			}
			if stderr != "" || !strings.HasPrefix(stdout, tt.want) || strings.Contains(stdout, "\x1b") {
				t.Errorf("completion %s stdout = %q, stderr = %q, want unstyled script starting with %q", tt.shell, stdout, stderr, tt.want)
			}
		})
	}
}

func TestBareCompletionPrintsHelp(t *testing.T) {
	stdout, stderr, err := executeForTest(t, "completion")
	if err != nil || stderr != "" {
		t.Fatalf("completion error = %v, stderr = %q", err, stderr)
	}
	want, _, err := executeForTest(t, "help", "completion")
	if err != nil {
		t.Fatalf("help completion error = %v", err)
	}
	if stdout != want {
		t.Errorf("bare completion stdout = %q, want help %q", stdout, want)
	}
}
