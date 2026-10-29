// Package gitrepo runs the git binary to clone and inspect source checkouts.
package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ErrGitNotFound reports that the git binary is not on PATH.
var ErrGitNotFound = errors.New("git not found in PATH; install git or adjust PATH")

// Clone clones url into path, streaming git's own output to progress. Git
// never prompts for credentials; authentication must already be configured.
func Clone(ctx context.Context, url, path string, progress io.Writer) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--", url, path)
	cmd.Env = nonInteractiveEnv()
	cmd.Stdout = progress
	cmd.Stderr = progress

	if err := cmd.Run(); err != nil {
		return describeFailure(fmt.Sprintf("clone %q into %q", url, path), err, "")
	}
	return nil
}

// OriginURL returns the fetch URL of the remote named origin in the checkout
// rooted at path. It fails when path is not a checkout root or has no origin
// remote. Symbolic links to a checkout root and linked worktrees are accepted.
func OriginURL(ctx context.Context, path string) (string, error) {
	root, err := gitOutput(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	rootInfo, err := os.Stat(strings.TrimSuffix(root, "\n"))
	if err != nil {
		return "", fmt.Errorf("check checkout root for %q: %w", path, err)
	}
	pathInfo, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("check checkout path %q: %w", path, err)
	}
	if !os.SameFile(rootInfo, pathInfo) {
		return "", fmt.Errorf("path %q is not a checkout root", path)
	}

	origin, err := gitOutput(ctx, path, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(origin), nil
}

func gitOutput(ctx context.Context, path string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
	cmd.Env = nonInteractiveEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", describeFailure(fmt.Sprintf("git %s in %q", strings.Join(args, " "), path), err, stderr.String())
	}
	return stdout.String(), nil
}

func nonInteractiveEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
}

func describeFailure(action string, err error, stderr string) error {
	if errors.Is(err, exec.ErrNotFound) {
		return ErrGitNotFound
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			return fmt.Errorf("%s: git exited with status %d", action, exitErr.ExitCode())
		}
		return fmt.Errorf("%s: git exited with status %d: %s", action, exitErr.ExitCode(), detail)
	}
	return fmt.Errorf("%s: %w", action, err)
}
