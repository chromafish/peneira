package diffparse

import (
	"fmt"
	"strings"
	"testing"
)

// oneFile is a diff of a single path with n added lines.
func oneFile(path string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n", path, path, path, path)
	fmt.Fprintf(&b, "@@ -1,0 +1,%d @@\n", n)
	for i := range n {
		fmt.Fprintf(&b, "+line %d\n", i)
	}
	return b.String()
}

func TestFileUnderTheCeilingIsKeptWhole(t *testing.T) {
	files := Parse(oneFile("small.go", 500))
	if len(files) != 1 {
		t.Fatalf("parsed %d files, want 1", len(files))
	}
	f := files[0]
	if f.Truncated {
		t.Error("a 500 line file was truncated")
	}
	added, _ := f.Counts()
	if added != 500 {
		t.Errorf("kept %d lines, want 500", added)
	}
}

func TestOneEnormousFileIsDropped(t *testing.T) {
	n := MaxFileLines + 1000
	files := Parse(oneFile("bundle.min.js", n))
	if len(files) != 1 {
		t.Fatalf("parsed %d files, want 1", len(files))
	}
	f := files[0]
	if !f.Truncated {
		t.Fatal("a file past MaxFileLines was kept")
	}
	if len(f.Hunks) != 0 {
		t.Errorf("a truncated file kept %d hunks, want none", len(f.Hunks))
	}
	if f.LineCount != n {
		t.Errorf("LineCount = %d, want %d — it should say how big it really was", f.LineCount, n)
	}
	if f.Path() != "bundle.min.js" {
		t.Errorf("path = %q, want it still identified", f.Path())
	}
}

// The per-file ceiling alone would let a change of very many small files
// through, so the whole-diff ceiling has to catch it.
func TestManySmallFilesHitTheTotalCeiling(t *testing.T) {
	const per = 1000
	var b strings.Builder
	for i := range MaxTotalLines/per + 10 {
		b.WriteString(oneFile(fmt.Sprintf("pkg/f%d.go", i), per))
	}
	files := Parse(b.String())

	kept, truncated := 0, 0
	for _, f := range files {
		if f.Truncated {
			truncated++
			continue
		}
		a, _ := f.Counts()
		kept += a
	}
	if truncated == 0 {
		t.Fatal("no file was truncated, so the total ceiling never bit")
	}
	if kept > MaxTotalLines {
		t.Errorf("kept %d lines, over the %d ceiling", kept, MaxTotalLines)
	}
	// Everything before the ceiling is still readable.
	if files[0].Truncated {
		t.Error("the first file was truncated, but the ceiling had not been reached")
	}
}

func TestTruncationDoesNotDisturbLaterFiles(t *testing.T) {
	diff := oneFile("huge.js", MaxFileLines+5) + oneFile("after.go", 10)
	files := Parse(diff)
	if len(files) != 2 {
		t.Fatalf("parsed %d files, want 2", len(files))
	}
	if !files[0].Truncated {
		t.Error("the huge file was not truncated")
	}
	if files[1].Truncated {
		t.Error("the small file after it was truncated too")
	}
	if added, _ := files[1].Counts(); added != 10 {
		t.Errorf("the file after the truncated one kept %d lines, want 10", added)
	}
}
