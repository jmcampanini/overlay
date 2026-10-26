package render

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aymanbagabas/go-udiff"
)

// Omission is an entry Apply would rewrite without changing its bytes. A
// patch carries only content, so WritePatch reports these instead of printing
// them.
type Omission struct {
	Entry Entry
	// Reason says what Apply would do, phrased to follow "render would", for
	// example "set permission bits 644 -> 600".
	Reason string
}

// WritePatch writes to out a patch that turns every new or changed entry's
// current target into its rendered content, in plan order. Each entry is one
// 'diff --git a/PATH b/PATH' block with PATH relative to plan.Target, so the
// output feeds 'git apply' or 'patch -p1' run from the target directory. A new
// entry diffs against /dev/null. Every file is treated as text. Entries whose
// rendered bytes already match the target are returned as omissions rather
// than written, and a target that is neither absent, a regular file, nor a
// symbolic link to one is an error.
func WritePatch(out io.Writer, plan Plan) ([]Omission, error) {
	var omissions []Omission
	for _, entry := range plan.Entries {
		if entry.Status == StatusUnchanged {
			continue
		}
		rel, err := filepath.Rel(plan.Target, entry.Target)
		if err != nil {
			return nil, fmt.Errorf("relate %s to %s: %w", entry.Target, plan.Target, err)
		}
		path := filepath.ToSlash(rel)
		rendered, err := renderedBytes(entry)
		if err != nil {
			return nil, err
		}

		if entry.Status == StatusNew {
			if err := writeNewFilePatch(out, path, entry.Mode, rendered); err != nil {
				return nil, err
			}
			continue
		}
		current, err := currentBytes(entry)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(current, rendered) {
			reason, err := omissionReason(entry)
			if err != nil {
				return nil, err
			}
			omissions = append(omissions, Omission{Entry: entry, Reason: reason})
			continue
		}
		if err := writeChangedFilePatch(out, path, current, rendered); err != nil {
			return nil, err
		}
	}
	return omissions, nil
}

func writeNewFilePatch(out io.Writer, path string, mode os.FileMode, rendered []byte) error {
	gitMode := "100644"
	if mode&0o111 != 0 {
		gitMode = "100755"
	}
	_, err := fmt.Fprintf(out, "diff --git a/%s b/%s\nnew file mode %s\n%s",
		path, path, gitMode, udiff.Unified("/dev/null", "b/"+path, "", string(rendered)))
	return err
}

func writeChangedFilePatch(out io.Writer, path string, current, rendered []byte) error {
	_, err := fmt.Fprintf(out, "diff --git a/%s b/%s\n%s",
		path, path, udiff.Unified("a/"+path, "b/"+path, string(current), string(rendered)))
	return err
}

// renderedBytes returns the bytes Apply would write: a template's rendered
// content or a file's source bytes.
func renderedBytes(entry Entry) ([]byte, error) {
	if entry.Kind == KindTemplate {
		return entry.Content, nil
	}
	content, err := os.ReadFile(entry.SourceFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", entry.SourceFile, err)
	}
	return content, nil
}

// currentBytes reads the target through any symbolic link; Apply replaces the
// link, so the patch shows the change to the content it resolves to.
func currentBytes(entry Entry) ([]byte, error) {
	info, err := os.Stat(entry.Target)
	if err != nil {
		return nil, fmt.Errorf("check target %s: %w", entry.Target, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("target %s is not a regular file (%s); a patch cannot describe it", entry.Target, info.Mode().Type())
	}
	content, err := os.ReadFile(entry.Target)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", entry.Target, err)
	}
	return content, nil
}

// omissionReason explains why Apply would rewrite a target whose bytes
// already match: it is a symbolic link, or its permission bits differ.
func omissionReason(entry Entry) (string, error) {
	info, err := os.Lstat(entry.Target)
	if err != nil {
		return "", fmt.Errorf("check target %s: %w", entry.Target, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "replace the symbolic link with a regular file", nil
	}
	return fmt.Sprintf("set permission bits %o -> %o", info.Mode().Perm(), entry.Mode), nil
}
