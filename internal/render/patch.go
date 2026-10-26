package render

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
// current target into its rendered content, in plan order. Each entry is a
// 'diff --git a/PATH b/PATH' block with PATH relative to plan.Target, quoted
// as git quotes it when it holds a space, quote, backslash, or control
// character, so the output feeds 'git apply' or 'patch -p1' run from the
// target directory. A new entry diffs against /dev/null; a symbolic link at
// the target is shown as git shows a type change, a deleted 120000 block
// followed by the new file block. Every file is treated as text. A regular
// file whose bytes already match is returned as an omission rather than
// written, and a target that is neither absent, a regular file, nor a
// symbolic link is an error.
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
		rendered := entry.Content
		if entry.Kind != KindTemplate {
			rendered, err = os.ReadFile(entry.SourceFile)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", entry.SourceFile, err)
			}
		}

		if entry.Status == StatusNew {
			if err := writeNewFilePatch(out, path, entry.Mode, rendered); err != nil {
				return nil, err
			}
			continue
		}
		info, err := os.Lstat(entry.Target)
		if err != nil {
			return nil, fmt.Errorf("check target %s: %w", entry.Target, err)
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			destination, err := os.Readlink(entry.Target)
			if err != nil {
				return nil, fmt.Errorf("read link %s: %w", entry.Target, err)
			}
			if err := writeDeletedLinkPatch(out, path, destination); err != nil {
				return nil, err
			}
			if err := writeNewFilePatch(out, path, entry.Mode, rendered); err != nil {
				return nil, err
			}
			continue
		case !info.Mode().IsRegular():
			return nil, fmt.Errorf("target %s is not a regular file (%s); a patch cannot describe it", entry.Target, info.Mode().Type())
		}

		current, err := os.ReadFile(entry.Target)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Target, err)
		}
		if bytes.Equal(current, rendered) {
			reason := fmt.Sprintf("set permission bits %o -> %o", info.Mode().Perm(), entry.Mode)
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
	a, b := quotePath("a/"+path), quotePath("b/"+path)
	_, err := fmt.Fprintf(out, "diff --git %s %s\nnew file mode %s\n%s",
		a, b, gitMode, udiff.Unified("/dev/null", b, "", string(rendered)))
	return err
}

func writeChangedFilePatch(out io.Writer, path string, current, rendered []byte) error {
	a, b := quotePath("a/"+path), quotePath("b/"+path)
	_, err := fmt.Fprintf(out, "diff --git %s %s\n%s",
		a, b, udiff.Unified(a, b, string(current), string(rendered)))
	return err
}

// writeDeletedLinkPatch removes a symbolic link the way git records it: the
// link's destination is its content, without a trailing newline.
func writeDeletedLinkPatch(out io.Writer, path, destination string) error {
	a, b := quotePath("a/"+path), quotePath("b/"+path)
	_, err := fmt.Fprintf(out, "diff --git %s %s\ndeleted file mode 120000\n%s",
		a, b, udiff.Unified(a, "/dev/null", destination, ""))
	return err
}

// quotePath returns path as git writes it in patch headers: unchanged when
// every byte is printable and not a space, quote, or backslash, and otherwise
// in double quotes with C escapes, which both 'git apply' and 'patch' read.
func quotePath(path string) string {
	if !strings.ContainsFunc(path, needsQuoting) {
		return path
	}

	var quoted strings.Builder
	quoted.WriteByte('"')
	for _, c := range []byte(path) {
		switch {
		case c == '"' || c == '\\':
			quoted.WriteByte('\\')
			quoted.WriteByte(c)
		case c == '\t':
			quoted.WriteString(`\t`)
		case c == '\n':
			quoted.WriteString(`\n`)
		case c == '\r':
			quoted.WriteString(`\r`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&quoted, `\%03o`, c)
		default:
			quoted.WriteByte(c)
		}
	}
	quoted.WriteByte('"')
	return quoted.String()
}

func needsQuoting(r rune) bool {
	return r <= ' ' || r == 0x7f || r == '"' || r == '\\'
}
