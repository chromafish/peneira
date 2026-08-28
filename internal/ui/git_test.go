package ui

import (
	"strings"
	"testing"

	"gioui.org/io/key"
)

// The interface is the same interface on a git repository: the same panes, the
// same keys, the same notes. What differs is what the backend can answer, and
// these are the places that shows.

func TestGitRepositoryLoads(t *testing.T) {
	h := newGitHarness(t)

	if got := h.app.backend.Name; got != "git" {
		t.Fatalf("the backend is %q, want git", got)
	}
	if got := h.app.backend.QueryLabel; got != "REVS" {
		t.Errorf("the query field is labelled %q, want the git spelling", got)
	}
	if len(h.app.revs) == 0 {
		t.Fatal("no revisions were listed")
	}
	// Uncommitted work is a row, and it is the one the application opens on.
	top := h.app.revs[0]
	if !top.WorkingCopy {
		t.Fatalf("the top row is %q, want the working tree", top.Subject())
	}
	if len(h.app.files) == 0 {
		t.Fatal("the working tree row has no files")
	}
}

// The manifest and the diff come from the same parser as jj's, because both
// backends are asked for git-format diffs.
func TestGitDiffReads(t *testing.T) {
	h := newGitHarness(t)
	h.selectFileNamed("main.go")

	doc := h.app.diff
	if doc == nil || len(doc.Rows) == 0 {
		t.Fatal("no diff was built")
	}
	var added, removed bool
	for i := range doc.Rows {
		r := doc.Row(i)
		if r.Kind != rowLine {
			continue
		}
		if strings.Contains(r.Line.Text, "two") {
			added = true
		}
		if strings.Contains(r.Line.Text, "one") {
			removed = true
		}
	}
	if !added || !removed {
		t.Error("the working tree diff does not show the edit to main.go")
	}
}

// Expanding a gap reads whole files out of the repository, which on the
// working tree means the file on disk on one side and HEAD on the other.
func TestGitExpandsFromBothSides(t *testing.T) {
	h := newGitHarness(t)
	h.selectFileNamed("wide.go")
	h.press("E", key.ModShift)

	doc := h.app.diff
	if doc == nil {
		t.Fatal("no diff")
	}
	var context int
	for i := range doc.Rows {
		r := doc.Row(i)
		if r.Kind == rowLine && strings.Contains(r.Line.Text, "var v20") {
			context++
		}
	}
	if context == 0 {
		t.Error("whole-file expansion produced no unchanged lines")
	}
}

// Notes are the point, and the text handed to an agent has to tell it how to
// get to a git commit rather than to a jj change.
func TestGitNotesCopyForAgent(t *testing.T) {
	h := newGitHarness(t)
	h.selectFileNamed("main.go")
	// Put the cursor on a line of code the way a person would, by clicking it.
	h.click(h.paneX(PaneDiff), h.diffRowY(h.firstLineRow()))
	h.press("C", 0)
	if h.app.draft == nil {
		t.Fatal("c did not open a note editor")
	}
	h.typeText("this should be a fmt.Println")
	h.press(key.NameReturn, key.ModShortcut)

	if notes := h.app.openNotes(); len(notes) != 1 {
		t.Fatalf("wrote %d notes, want 1", len(notes))
	}
	h.press("C", key.ModShortcut|key.ModShift)

	out := h.clipboard()
	if !strings.Contains(out, "this should be a fmt.Println") {
		t.Errorf("the note is missing from the handoff:\n%s", out)
	}
	if !strings.Contains(out, "git checkout") {
		t.Errorf("the handoff does not say how to get to a git commit:\n%s", out)
	}
	if strings.Contains(out, "jj edit") {
		t.Errorf("the handoff talks about jj on a git repository:\n%s", out)
	}
}
