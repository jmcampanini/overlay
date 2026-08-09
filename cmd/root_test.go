package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
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
