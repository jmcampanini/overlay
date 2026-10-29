package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"os"
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

func TestOriginURLRequiresCheckoutRoot(t *testing.T) {
	bare := gittest.NewBareRepo(t)
	checkout := filepath.Join(t.TempDir(), "checkout")
	gittest.Run(t, filepath.Dir(checkout), "clone", "-q", bare, checkout)
	nested := filepath.Join(checkout, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(t.TempDir(), "checkout-link")
	if err := os.Symlink(checkout, symlink); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(t.TempDir(), "worktree")
	gittest.Run(t, checkout, "worktree", "add", "-q", "--detach", worktree, "origin/main")

	for _, tt := range []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "checkout", path: checkout},
		{name: "symlink", path: symlink},
		{name: "linked worktree", path: worktree},
		{name: "nested directory", path: nested, wantErr: true},
		{name: "bare repository", path: bare, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			origin, err := OriginURL(context.Background(), tt.path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("OriginURL(%q) = %q, want error", tt.path, origin)
				}
				return
			}
			if err != nil {
				t.Fatalf("OriginURL(%q) error = %v", tt.path, err)
			}
			if origin != bare {
				t.Errorf("OriginURL(%q) = %q, want %q", tt.path, origin, bare)
			}
		})
	}
}

func TestMissingGitIsReported(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := Clone(context.Background(), "ignored", filepath.Join(t.TempDir(), "dest"), &bytes.Buffer{})

	if !errors.Is(err, ErrGitNotFound) {
		t.Errorf("Clone() without git error = %v, want ErrGitNotFound", err)
	}
}
