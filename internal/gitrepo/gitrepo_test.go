package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmcampanini/overlay/internal/gittest"
)

func TestCloneCreatesCheckoutWithOrigin(t *testing.T) {
	bare := gittest.NewBareRepo(t)
	dest := filepath.Join(t.TempDir(), "dest")
	var progress bytes.Buffer

	if err := Clone(context.Background(), bare, dest, &progress); err != nil {
		t.Fatalf("Clone() error = %v\n%s", err, progress.String())
	}
	origin, err := OriginURL(context.Background(), dest)
	if err != nil {
		t.Fatalf("OriginURL() error = %v", err)
	}

	if origin != bare {
		t.Errorf("OriginURL() = %q, want %q", origin, bare)
	}
}

func TestCloneReportsGitFailure(t *testing.T) {
	gittest.NewBareRepo(t)
	missing := filepath.Join(t.TempDir(), "missing.git")
	dest := filepath.Join(t.TempDir(), "dest")

	err := Clone(context.Background(), missing, dest, &bytes.Buffer{})

	if err == nil || !strings.Contains(err.Error(), "git exited with status") {
		t.Errorf("Clone(missing) error = %v, want git exit status", err)
	}
}

func TestOriginURLFailsOutsideCheckout(t *testing.T) {
	gittest.NewBareRepo(t)

	_, err := OriginURL(context.Background(), t.TempDir())

	if err == nil || errors.Is(err, ErrGitNotFound) {
		t.Errorf("OriginURL(plain dir) error = %v, want git failure", err)
	}
}

func TestMissingGitIsReported(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := Clone(context.Background(), "ignored", filepath.Join(t.TempDir(), "dest"), &bytes.Buffer{})

	if !errors.Is(err, ErrGitNotFound) {
		t.Errorf("Clone() without git error = %v, want ErrGitNotFound", err)
	}
}
