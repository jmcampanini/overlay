package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmcampanini/overlay/internal/gittest"
)

const fixturePatch = `diff --git a/.ssh/config b/.ssh/config
new file mode 100644
--- /dev/null
+++ b/.ssh/config
@@ -0,0 +1 @@
+Host personal
`

func TestDiffPrintsAPatchThatGitAppliesFromTheTarget(t *testing.T) {
	f := newRenderFixture(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	out, errOut, err := executeForTest(t, "diff", "--config", f.configPath)
	if err != nil {
		t.Fatalf("diff error = %v, stderr = %s", err, errOut)
	}

	if out != fixturePatch || errOut != "" {
		t.Errorf("diff stdout = %q, stderr = %q, want the fixture patch and no stderr", out, errOut)
	}
	requireNoFile(t, filepath.Join(f.home, ".ssh", "config"))

	patch := filepath.Join(t.TempDir(), "overlay.patch")
	if err := os.WriteFile(patch, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, f.home, "apply", patch)
	if raw, err := os.ReadFile(filepath.Join(f.home, ".ssh", "config")); err != nil || string(raw) != "Host personal\n" {
		t.Errorf("applied ssh config = %q (%v), want the personal layer's file", raw, err)
	}

	after, afterErr, err := executeForTest(t, "diff", "--config", f.configPath)
	if err != nil {
		t.Fatalf("second diff error = %v, stderr = %s", err, afterErr)
	}
	if after != "" {
		t.Errorf("diff after apply printed a patch:\n%s", after)
	}
	if afterErr != "overlay: ~/.ssh/config: no content change, but render would set permission bits 644 -> 600\n" {
		t.Errorf("diff after apply stderr = %q, want the permission-bits note", afterErr)
	}
}

func TestDiffTargetFlagMakesPathsRelativeToTheTarget(t *testing.T) {
	f := newRenderFixture(t)
	target := filepath.Join(f.home, "staging")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}

	out, errOut, err := executeForTest(t, "diff", "--config", f.configPath, "--target", "~/staging", "--layers", "")
	if err != nil {
		t.Fatalf("diff --target error = %v, stderr = %s", err, errOut)
	}

	for _, want := range []string{"diff --git a/.bashrc b/.bashrc\n", "diff --git a/.ssh/config b/.ssh/config\n", "+Host base\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("diff output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "staging") {
		t.Errorf("diff output names the target directory:\n%s", out)
	}
	requireNoFile(t, filepath.Join(target, ".bashrc"))
}
