package cmd

import (
	"bytes"
	"errors"
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
	if !errors.As(err, &code) {
		t.Fatalf("command returned a non-ExitCode error: %v\nstderr:\n%s", err, stderr.String())
	}
	return rootResult{code: int(code), stdout: stdout.String(), stderr: stderr.String()}
}
