package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chromafish/check/internal/diffparse"
)

// bigRepo is a git repository whose one commit adds a file past the per-file
// ceiling, alongside an ordinary one.
func bigRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_AUTHOR_DATE=2026-01-01T00:00:00+00:00",
			"GIT_COMMITTER_DATE=2026-01-01T00:00:00+00:00")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-q", "-b", "main")
	write("small.go", "package main\n\nfunc main() {}\n")
	run("add", "-A")
	run("commit", "-qm", "first")

	var b strings.Builder
	for i := range diffparse.MaxFileLines + 500 {
		b.WriteString("generated line ")
		b.WriteString(strings.Repeat("x", 3))
		b.WriteByte(byte('0' + i%10))
		b.WriteByte('\n')
	}
	write("bundle.min.js", b.String())
	write("small.go", "package main\n\nfunc main() { println(1) }\n")
	run("add", "-A")
	run("commit", "-qm", "add a bundle")
	return dir
}

// A change with a file past the ceiling still opens, still shows every other
// file, and can be told to fetch the one it left out.
func TestLargeFileIsLeftOutThenFetchedOnDemand(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	h := openHarness(t, bigRepo(t))
	a := h.app
	if a.diff == nil {
		t.Fatal("the change never loaded")
	}

	var big *FileDoc
	var bigIdx int
	for i, fd := range a.diff.Files {
		if strings.HasSuffix(fd.Path, "bundle.min.js") {
			big, bigIdx = fd, i
		}
	}
	if big == nil {
		t.Fatal("the bundle is missing from the manifest")
	}
	if big.File == nil || !big.File.Truncated {
		t.Fatal("the bundle was not truncated, so the ceiling did not bite")
	}
	if !strings.Contains(big.Note, "LARGE DIFF") || !strings.Contains(big.Note, "NOT SHOWN") {
		t.Errorf("note = %q, want it to say the diff is large and not shown", big.Note)
	}
	if big.Note == "" || strings.Contains(big.Note, "TOO LARGE TO SHOW") {
		t.Errorf("note = %q, want it not to claim the diff cannot be shown", big.Note)
	}

	// The ordinary file alongside it is unaffected.
	for _, fd := range a.diff.Files {
		if strings.HasSuffix(fd.Path, "small.go") && fd.File.Truncated {
			t.Error("the small file was truncated too")
		}
	}

	// Now ask for it.
	a.showWhole(bigIdx)
	h.settle()
	if big.File.Truncated {
		t.Fatal("the bundle is still truncated after being asked for")
	}
	if big.Note != "" {
		t.Errorf("note = %q, want it cleared once the diff is in", big.Note)
	}
	added, _ := big.File.Counts()
	if added < diffparse.MaxFileLines {
		t.Errorf("fetched %d added lines, want at least %d", added, diffparse.MaxFileLines)
	}
	// The rows are really there to scroll through.
	rows := 0
	start, end := a.diff.rowRange(bigIdx)
	for i := start; i < end; i++ {
		if a.diff.Row(i).Kind == rowLine {
			rows++
		}
	}
	if rows < diffparse.MaxFileLines {
		t.Errorf("the sheet carries %d lines for the bundle, want at least %d", rows, diffparse.MaxFileLines)
	}
}
