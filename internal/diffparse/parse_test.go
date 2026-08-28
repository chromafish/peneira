package diffparse

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

const sample = `diff --git a/a.txt b/b.txt
rename from a.txt
rename to b.txt
diff --git a/c.txt b/c.txt
new file mode 100644
index 0000000000..fa49b07797
--- /dev/null
+++ b/c.txt
@@ -0,0 +1,1 @@
+new file
\ No newline at end of file
diff --git a/main.go b/main.go
index 0e8cf86131..cfa8402412 100644
--- a/main.go
+++ b/main.go
@@ -2,9 +2,9 @@ package main
 
 import "fmt"
 
-// greet says hi
+// greet says hi to name
 func greet(name string) string {
-	return fmt.Sprintf("hello, ", name)
+	return fmt.Sprintf("hello, %s!", name)
 }
 
 func main() {
diff --git a/gone.txt b/gone.txt
deleted file mode 100644
--- a/gone.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-one
-two
`

func TestParseFiles(t *testing.T) {
	files := Parse(sample)
	if len(files) != 4 {
		t.Fatalf("got %d files, want 4", len(files))
	}

	if f := files[0]; !f.IsRename || f.OldPath != "a.txt" || f.NewPath != "b.txt" || len(f.Hunks) != 0 {
		t.Errorf("rename parsed as %+v", f)
	}

	f := files[1]
	if !f.IsNew || f.Path() != "c.txt" {
		t.Errorf("addition parsed as %+v", f)
	}
	if len(f.Hunks) != 1 || len(f.Hunks[0].Lines) != 1 {
		t.Fatalf("addition hunks = %+v", f.Hunks)
	}
	if l := f.Hunks[0].Lines[0]; l.Kind != Added || l.NewNum != 1 || l.Text != "new file" || !l.NoNewline {
		t.Errorf("added line = %+v", l)
	}

	f = files[2]
	if f.Path() != "main.go" || f.IsNew || f.IsDeleted {
		t.Errorf("modification parsed as %+v", f)
	}
	h := f.Hunks[0]
	if h.OldStart != 2 || h.OldCount != 9 || h.NewStart != 2 || h.NewCount != 9 {
		t.Errorf("hunk range = %+v", h)
	}
	if h.Section != "package main" {
		t.Errorf("section = %q", h.Section)
	}
	if added, removed := f.Counts(); added != 2 || removed != 2 {
		t.Errorf("counts = %d/%d, want 2/2", added, removed)
	}
	// Line numbering must keep counting through added and removed lines.
	last := h.Lines[len(h.Lines)-1]
	if last.Text != "func main() {" || last.OldNum != 10 || last.NewNum != 10 {
		t.Errorf("last line = %+v", last)
	}

	if f := files[3]; !f.IsDeleted || f.Path() != "gone.txt" {
		t.Errorf("deletion parsed as %+v", f)
	}
}

func TestRefineMarksOnlyTheChange(t *testing.T) {
	files := Parse(sample)
	h := files[2].Hunks[0]

	var removed, added Line
	for _, l := range h.Lines {
		if l.Text == `	return fmt.Sprintf("hello, ", name)` {
			removed = l
		}
		if l.Text == `	return fmt.Sprintf("hello, %s!", name)` {
			added = l
		}
	}
	if added.Segments == nil {
		t.Fatal("added line was not refined")
	}
	if got := changedText(added); got != "%s!" {
		t.Errorf("added line marks %q, want %q", got, "%s!")
	}
	if got := changedText(removed); got != "" {
		t.Errorf("removed line marks %q, want nothing", got)
	}
}

func TestRefineSkipsUnrelatedLines(t *testing.T) {
	files := Parse(`diff --git a/x b/x
--- a/x
+++ b/x
@@ -1,1 +1,1 @@
-alpha beta gamma delta
+zulu yankee xray whiskey
`)
	for _, l := range files[0].Hunks[0].Lines {
		if l.Segments != nil {
			t.Errorf("line %q was refined but shares nothing", l.Text)
		}
	}
}

func TestRefineHandlesUnevenRuns(t *testing.T) {
	files := Parse(`diff --git a/x b/x
--- a/x
+++ b/x
@@ -1,2 +1,3 @@
-value := compute(a)
+value := compute(a, b)
+extra := 1
 done
`)
	lines := files[0].Hunks[0].Lines
	if got := changedText(lines[0]); got != "" {
		t.Errorf("removed line marks %q, want nothing", got)
	}
	if got := changedText(lines[1]); got != ", b" {
		t.Errorf("marked %q, want %q", got, ", b")
	}
	if lines[2].Segments != nil {
		t.Error("the unpaired addition should not be refined")
	}
}

func changedText(l Line) string {
	out := ""
	for _, s := range l.Segments {
		if s.Changed {
			out += l.Text[s.Start:s.End]
		}
	}
	return out
}

// sameFiles compares two parses closely enough to catch a stream that read the
// diff differently: paths, flags, the ceiling verdict, and every line.
func sameFiles(t *testing.T, got, want []File) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("read %d files, want %d", len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Path() != w.Path() || g.OldPath != w.OldPath || g.IsNew != w.IsNew ||
			g.IsDeleted != w.IsDeleted || g.IsRename != w.IsRename || g.IsBinary != w.IsBinary ||
			g.Truncated != w.Truncated || g.LineCount != w.LineCount {
			t.Fatalf("file %d differs:\n got  %+v\n want %+v", i, g, w)
		}
		if len(g.Hunks) != len(w.Hunks) {
			t.Fatalf("file %s has %d hunks, want %d", w.Path(), len(g.Hunks), len(w.Hunks))
		}
		for h := range w.Hunks {
			gh, wh := g.Hunks[h], w.Hunks[h]
			if gh.OldStart != wh.OldStart || gh.NewStart != wh.NewStart || gh.Section != wh.Section {
				t.Fatalf("%s hunk %d differs:\n got  %+v\n want %+v", w.Path(), h, gh, wh)
			}
			if len(gh.Lines) != len(wh.Lines) {
				t.Fatalf("%s hunk %d has %d lines, want %d", w.Path(), h, len(gh.Lines), len(wh.Lines))
			}
			for l := range wh.Lines {
				gl, wl := gh.Lines[l], wh.Lines[l]
				if gl.Kind != wl.Kind || gl.Text != wl.Text || gl.OldNum != wl.OldNum ||
					gl.NewNum != wl.NewNum || gl.NoNewline != wl.NoNewline ||
					changedText(gl) != changedText(wl) {
					t.Fatalf("%s hunk %d line %d differs:\n got  %+v\n want %+v", w.Path(), h, l, gl, wl)
				}
			}
		}
	}
}

// scanAll drains Scan into a slice, so a stream can be held to what Parse says
// about the same diff.
func scanAll(t *testing.T, r io.Reader) []File {
	t.Helper()
	var files []File
	if err := Scan(r, func(f File) { files = append(files, f) }); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return files
}

// A diff read as it arrives has to come out as the same change as one read
// whole, however the reads happen to land.
func TestScanReadsTheSameDiffAsParse(t *testing.T) {
	want := Parse(sample)
	sameFiles(t, scanAll(t, strings.NewReader(sample)), want)
	sameFiles(t, scanAll(t, iotest.OneByteReader(strings.NewReader(sample))), want)
	sameFiles(t, scanAll(t, iotest.DataErrReader(strings.NewReader(sample))), want)
}

// A diff whose last line has no newline still ends the file it was in.
func TestScanEndsAFileWithNoTrailingNewline(t *testing.T) {
	diff := strings.TrimSuffix(sample, "\n")
	sameFiles(t, scanAll(t, strings.NewReader(diff)), Parse(diff))
}

// The ceilings hold across a stream, and cut in the same place: how a change is
// read is not allowed to change how much of it is kept.
func TestScanKeepsTheCeilings(t *testing.T) {
	var b strings.Builder
	// One file past the per-file ceiling, then an ordinary one after it, so the
	// test covers both the file that is cut and the allowance handed back.
	b.WriteString("diff --git a/big.js b/big.js\n--- a/big.js\n+++ b/big.js\n")
	fmt.Fprintf(&b, "@@ -1,%d +1,%d @@\n", MaxFileLines+50, MaxFileLines+50)
	for i := range MaxFileLines + 50 {
		fmt.Fprintf(&b, "+generated %d\n", i)
	}
	b.WriteString("diff --git a/small.go b/small.go\n--- a/small.go\n+++ b/small.go\n@@ -1,1 +1,1 @@\n-old\n+new\n")

	diff := b.String()
	want := Parse(diff)
	if len(want) != 2 || !want[0].Truncated || want[1].Truncated {
		t.Fatalf("the fixture did not exercise the ceiling: %+v", want)
	}
	sameFiles(t, scanAll(t, strings.NewReader(diff)), want)
}
