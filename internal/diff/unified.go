package diff

import (
	"fmt"
	"strings"
)

// Unified returns a git-style unified diff of a vs b. aLabel and bLabel
// populate the --- / +++ header lines. An empty string result means the
// inputs are byte-identical.
func Unified(a, b []byte, aLabel, bLabel string) string {
	if string(a) == string(b) {
		return ""
	}
	aLines := splitLines(a)
	bLines := splitLines(b)

	hunks := diffHunks(aLines, bLines, 3)
	if len(hunks) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n", aLabel)
	fmt.Fprintf(&sb, "+++ %s\n", bLabel)
	for _, h := range hunks {
		aStart := h.aStart + 1
		if h.aLen == 0 {
			aStart = 0
		}
		bStart := h.bStart + 1
		if h.bLen == 0 {
			bStart = 0
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", aStart, h.aLen, bStart, h.bLen)
		for _, line := range h.lines {
			sb.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				sb.WriteByte('\n')
			}
		}
	}
	return sb.String()
}

type hunk struct {
	aStart, aLen int
	bStart, bLen int
	lines        []string
}

// diffHunks computes unified-diff hunks from an LCS-derived edit script
// with `context` lines of surrounding context.
func diffHunks(a, b []string, context int) []hunk {
	ops := editScript(a, b)
	if len(ops) == 0 {
		return nil
	}

	var hunks []hunk
	i := 0
	for i < len(ops) {
		for i < len(ops) && ops[i].kind == opEqual {
			i++
		}
		if i >= len(ops) {
			break
		}
		start := max(i-context, 0)
		end := i
		for end < len(ops) {
			if ops[end].kind != opEqual {
				end++
				continue
			}
			runStart := end
			for end < len(ops) && ops[end].kind == opEqual {
				end++
			}
			// An equal run ending at EOF only needs `context` trailing
			// lines; a run followed by another change needs 2*context so
			// both hunks keep their full context and stay separate.
			limit := 2 * context
			if end == len(ops) {
				limit = context
			}
			if end-runStart > limit {
				end = runStart + context
				break
			}
		}

		var h hunk
		h.aStart, h.bStart = opIndexes(ops, start)
		for j := start; j < end; j++ {
			op := ops[j]
			switch op.kind {
			case opEqual:
				h.lines = append(h.lines, " "+op.text)
				h.aLen++
				h.bLen++
			case opDelete:
				h.lines = append(h.lines, "-"+op.text)
				h.aLen++
			case opInsert:
				h.lines = append(h.lines, "+"+op.text)
				h.bLen++
			}
		}
		hunks = append(hunks, h)
		i = end
	}
	return hunks
}

// opIndexes returns the (aIndex, bIndex) at position `at` in ops - i.e.,
// how many a-lines and b-lines precede the op at index `at`.
func opIndexes(ops []op, at int) (int, int) {
	ai, bi := 0, 0
	for j := 0; j < at && j < len(ops); j++ {
		switch ops[j].kind {
		case opEqual:
			ai++
			bi++
		case opDelete:
			ai++
		case opInsert:
			bi++
		}
	}
	return ai, bi
}

type opKind int

const (
	opEqual opKind = iota
	opDelete
	opInsert
)

type op struct {
	kind opKind
	text string
}

// editScript computes a line-level LCS edit script from a to b.
// Returns ops in source order, each annotated with the original line text.
func editScript(a, b []string) []op {
	n, m := len(a), len(b)
	// lcs[i][j] = length of LCS of a[:i] and b[:j]
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			switch {
			case a[i-1] == b[j-1]:
				lcs[i][j] = lcs[i-1][j-1] + 1
			case lcs[i-1][j] >= lcs[i][j-1]:
				lcs[i][j] = lcs[i-1][j]
			default:
				lcs[i][j] = lcs[i][j-1]
			}
		}
	}
	var ops []op
	var walk func(i, j int)
	walk = func(i, j int) {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			walk(i-1, j-1)
			ops = append(ops, op{kind: opEqual, text: a[i-1]})
		case j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]):
			walk(i, j-1)
			ops = append(ops, op{kind: opInsert, text: b[j-1]})
		case i > 0 && (j == 0 || lcs[i][j-1] < lcs[i-1][j]):
			walk(i-1, j)
			ops = append(ops, op{kind: opDelete, text: a[i-1]})
		}
	}
	walk(n, m)
	return ops
}

// splitLines splits b into lines, preserving trailing newlines when present.
// The result never has an empty tail from a trailing newline.
func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	parts := strings.SplitAfter(s, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
