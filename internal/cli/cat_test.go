package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatRawPreservesBytesAndAddsOnlyMissingNewlines(t *testing.T) {
	f := newFileFixture(t)
	writeFixtureFile(t, filepath.Join(f.sources[0], "a.base"), "", 0o644)
	writeFixtureFile(t, filepath.Join(f.sources[0], "b.base"), " \t# comment\r\n\n\xff\x00no newline", 0o644)
	writeFixtureFile(t, filepath.Join(f.sources[1], "a.work"), "already\n\n", 0o644)
	writeFixtureFile(t, filepath.Join(f.sources[1], "b.work"), "already\n\n", 0o644)
	writeFixtureFile(t, filepath.Join(f.sources[0], "a.local"), "", 0o644)

	requireFileCommandOutput(t, []string{"cat", "--config", f.configPath, "a.{layer}", "b.{layer}"},
		" \t# comment\r\n\n\xff\x00no newline\nalready\n\nalready\n\n")
}

func TestCatListTrimsCommentsAndKeepsFirstEntryPositions(t *testing.T) {
	f := newFileFixture(t)
	writeFixtureFile(t, filepath.Join(f.sources[0], "p.base"),
		" \tdocker  # build box\r\n\t\r\n # full comment\n\t# another\n"+
			"c#\nfoo#bar\nhttps://example.com/docs#setup\n"+
			"two  spaces\ninside\ttab\nitem\t# comment\n"+
			"\u00a0kept\u00a0\n\xff\nfirst\n", 0o644)
	writeFixtureFile(t, filepath.Join(f.sources[1], "p.local"),
		"new\n docker\t\n first # duplicate\n two  spaces\r\n"+
			"two spaces\ninside\ttab\nCase\ncase\n\xff\nlast", 0o644)

	requireFileCommandOutput(t, []string{"cat", "--list", "--config", f.configPath, "p.{layer}"},
		"docker\nc#\nfoo#bar\nhttps://example.com/docs#setup\ntwo  spaces\n"+
			"inside\ttab\nitem\n\u00a0kept\u00a0\n\xff\nfirst\nnew\ntwo spaces\nCase\ncase\nlast\n")
}

func TestCatUnreadableFileLeavesStdoutEmpty(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file read permissions")
	}
	f := newFileFixture(t)
	writeFixtureFile(t, filepath.Join(f.sources[0], "p.base"), "earlier match\n", 0o644)
	path := filepath.Join(f.sources[1], "p.local")
	writeFixtureFile(t, path, "unreadable\n", 0o000)
	for _, flags := range [][]string{nil, {"--list"}} {
		args := append([]string{"cat", "--config", f.configPath}, flags...)
		requireFileCommandFailure(t, append(args, "p.{layer}"), 1, "read file", "p.local")
	}
}
