package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chromafish/peneira/internal/diffparse"
	"github.com/chromafish/peneira/internal/highlight"
	"github.com/chromafish/peneira/internal/state"
)

// threeFileDiff is a change over three files, each with one hunk of one
// changed line and one context line.
func threeFileDiff() string {
	var b strings.Builder
	for f := range 3 {
		fmt.Fprintf(&b, "diff --git a/f%d.go b/f%d.go\n--- a/f%d.go\n+++ b/f%d.go\n", f, f, f, f)
		b.WriteString("@@ -1,2 +1,2 @@\n")
		fmt.Fprintf(&b, "-old%d\n+new%d\n ctx\n", f, f)
	}
	return b.String()
}

func threeFileDoc(t *testing.T) (*App, *DiffDoc) {
	t.Helper()
	parsed := diffparse.Parse(threeFileDiff())
	rows := make([]FileRow, len(parsed))
	for i := range parsed {
		rows[i].Path = parsed[i].Path()
	}
	doc := buildDoc(rows, parsed)
	a := &App{store: state.New(), diff: doc}
	a.rev.ChangeIDFull = "change"
	a.rebuildRows()
	return a, doc
}

// A file's rows run from its own heading to the next file's, which is what
// lets highlighting recolour one file without rebuilding the sheet.
func TestRowRangeCoversExactlyOneFile(t *testing.T) {
	_, doc := threeFileDoc(t)
	if len(doc.Files) != 3 || len(doc.FileRows) != 3 {
		t.Fatalf("built %d files and %d headings, want 3 of each", len(doc.Files), len(doc.FileRows))
	}
	seen := map[int]bool{}
	for i := range doc.Files {
		start, end := doc.rowRange(i)
		if start >= end {
			t.Fatalf("file %d has an empty range %d..%d", i, start, end)
		}
		for r := start; r < end; r++ {
			if doc.FileOf(r) != i {
				t.Errorf("row %d is in file %d's range but belongs to file %d", r, i, doc.FileOf(r))
			}
			if seen[r] {
				t.Errorf("row %d appears in two ranges", r)
			}
			seen[r] = true
		}
	}
	if len(seen) != len(doc.Rows) {
		t.Errorf("the ranges cover %d of %d rows", len(seen), len(doc.Rows))
	}
}

// Colouring a file shows through the sheet without a rebuild, because the
// sheet points at the file's own rows — and it reaches no further than that
// file.
func TestRecolouringTouchesOnlyItsOwnFile(t *testing.T) {
	_, doc := threeFileDoc(t)

	// Colour every row, so that anything left uncoloured is the change. Each
	// file is two lines long, and both need spans.
	kw := []highlight.Span{{Start: 0, End: 3, Class: highlight.Keyword}}
	all := highlight.Lines{kw, kw}
	for _, fd := range doc.Files {
		fd.applyHighlight(all, all)
	}

	// Now recolour the middle file with nothing, as an unlexable file would.
	doc.Files[1].applyHighlight(nil, nil)

	lit := 0
	for i := range doc.Rows {
		r := doc.Row(i)
		if r.Kind != rowLine {
			continue
		}
		file := doc.FileOf(i)
		want := file != 1
		if got := r.Spans != nil; got != want {
			t.Errorf("row %d (file %d) coloured=%v, want %v", i, file, got, want)
		}
		if want {
			lit++
		}
	}
	if lit == 0 {
		t.Error("the sheet shows none of the colouring the files were given")
	}
}

// Notes are attached by pointer, so the rows must still see the right bodies.
func TestRowsCarryTheirOwnNotes(t *testing.T) {
	a, doc := threeFileDoc(t)
	for i, fd := range doc.Files {
		a.store.AddComment("change", state.Comment{
			Path: fd.Path, Side: state.SideNew, Line: 1, Body: fmt.Sprintf("note %d", i),
		})
	}
	a.rebuildRows()

	found := 0
	for i := range a.diff.Rows {
		r := a.diff.Row(i)
		if r.Kind != rowComment {
			continue
		}
		found++
		if r.Comment == nil {
			t.Fatal("a comment row carries no note")
		}
		file := a.diff.FileOf(i)
		if want := fmt.Sprintf("note %d", file); r.Comment.Body != want {
			t.Errorf("file %d shows %q, want %q", file, r.Comment.Body, want)
		}
	}
	if found != 3 {
		t.Errorf("found %d note rows, want 3", found)
	}
}

// rebuildFile has to land in exactly the state a full rebuild would, or the two
// paths drift and the sheet quietly goes wrong.
func assertSameAsFullRebuild(t *testing.T, a *App) {
	t.Helper()
	spliced := a.diff
	rows := make([]Row, len(spliced.Rows))
	for i := range rows {
		rows[i] = spliced.Row(i)
	}
	hunks := append([]int(nil), spliced.Hunks...)
	fileRows := append([]int(nil), spliced.FileRows...)
	a.rebuildRows()
	full := a.diff

	if len(rows) != len(full.Rows) {
		t.Fatalf("spliced %d rows, a full rebuild gives %d", len(rows), len(full.Rows))
	}
	for i := range rows {
		got, want := rows[i], full.Row(i)
		if got.Kind != want.Kind || spliced.FileOf(i) != full.FileOf(i) || got.Hunk != want.Hunk ||
			got.Text != want.Text || got.Noted != want.Noted || got.Gap != want.Gap ||
			!sameLine(got.Line, want.Line) || noteID(got) != noteID(want) {
			t.Fatalf("row %d differs:\n spliced %+v\n full    %+v", i, got, want)
		}
	}
	if !equalInts(hunks, full.Hunks) {
		t.Errorf("hunks:\n spliced %v\n full    %v", hunks, full.Hunks)
	}
	if !equalInts(fileRows, full.FileRows) {
		t.Errorf("file headings:\n spliced %v\n full    %v", fileRows, full.FileRows)
	}
}

// noteID names the note a row carries, if it carries one. The two rebuilds
// group the change's notes separately, so the rows hold different pointers to
// the same note.
func noteID(r Row) string {
	if r.Comment == nil {
		return ""
	}
	return r.Comment.ID
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRebuildFileMatchesFullRebuildWhenRowsAppear(t *testing.T) {
	a, doc := threeFileDoc(t)
	// A note on the middle file adds a row to it and shifts everything after.
	a.store.AddComment("change", state.Comment{
		Path: doc.Files[1].Path, Side: state.SideNew, Line: 1, Body: "here",
	})
	a.rebuildFile(1)
	assertSameAsFullRebuild(t, a)
}

func TestRebuildFileMatchesFullRebuildWhenRowsGoAway(t *testing.T) {
	a, doc := threeFileDoc(t)
	c := a.store.AddComment("change", state.Comment{
		Path: doc.Files[0].Path, Side: state.SideNew, Line: 1, Body: "here",
	})
	a.rebuildRows()
	a.store.DeleteComment("change", c.ID)
	a.rebuildFile(0)
	assertSameAsFullRebuild(t, a)
}

func TestRebuildFileMatchesFullRebuildOnTheLastFile(t *testing.T) {
	a, doc := threeFileDoc(t)
	a.store.AddComment("change", state.Comment{
		Path: doc.Files[2].Path, Side: state.SideNew, Line: 1, Body: "last",
	})
	a.rebuildFile(2)
	assertSameAsFullRebuild(t, a)
}

// Pairs are built on demand, so asking for them after a splice must give the
// alignment for the rows as they now are.
func TestPairsAreRebuiltAfterASplice(t *testing.T) {
	a, doc := threeFileDoc(t)
	before := len(doc.pairs())
	a.store.AddComment("change", state.Comment{
		Path: doc.Files[1].Path, Side: state.SideNew, Line: 1, Body: "here",
	})
	a.rebuildFile(1)
	if doc.Pairs != nil {
		t.Error("a splice left stale pairs in place")
	}
	if after := len(doc.pairs()); after != before+1 {
		t.Errorf("pairs went from %d to %d, want one more for the note row", before, after)
	}
}

// sameLine compares the parts of a line that say which line it is. Line holds
// a slice, so it cannot be compared whole.
func sameLine(a, b diffparse.Line) bool {
	return a.Kind == b.Kind && a.OldNum == b.OldNum && a.NewNum == b.NewNum && a.Text == b.Text
}

// gappyDoc is one file whose three hunks sit far apart, so there is a gap
// before each of them and one after the last, with the file's own text on hand
// to fill them from.
func gappyDoc(t *testing.T) (*App, *FileDoc) {
	t.Helper()
	var b strings.Builder
	b.WriteString("diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n")
	for h := range 3 {
		at := 40*h + 10
		fmt.Fprintf(&b, "@@ -%d,2 +%d,2 @@\n", at, at)
		fmt.Fprintf(&b, "-old %d\n+new %d\n", h, h)
	}
	parsed := diffparse.ParseOne(b.String())
	rows := make([]FileRow, 1)
	rows[0].Path = parsed[0].Path()
	doc := buildDoc(rows, parsed)
	a := &App{store: state.New(), diff: doc}
	a.rev.ChangeIDFull = "change"
	a.rebuildRows()

	fd := doc.Files[0]
	fd.lines = make([]string, 200)
	for i := range fd.lines {
		fd.lines[i] = fmt.Sprintf("line %d", i+1)
	}
	fd.read = true
	return a, fd
}

// Opening every gap at once has to land where opening them one at a time did,
// or the whole-file toggle shows a different file than it used to.
func TestExpandAllMatchesOpeningGapsOneAtATime(t *testing.T) {
	_, fd := gappyDoc(t)
	for {
		gap, found := fd.firstGap()
		if !found {
			break
		}
		if !fd.expand(gap, -1) {
			break
		}
	}
	want := append([]Row(nil), fd.base...)
	wantCols := fd.MaxCols

	_, other := gappyDoc(t)
	if !other.expandAll() {
		t.Fatal("expandAll opened nothing")
	}
	got := other.base

	if len(got) != len(want) {
		t.Fatalf("expandAll gives %d rows, one at a time gives %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Kind != want[i].Kind || !sameLine(got[i].Line, want[i].Line) ||
			got[i].Hunk != want[i].Hunk {
			t.Fatalf("row %d differs:\n expandAll %+v\n one by one %+v", i, got[i], want[i])
		}
	}
	if other.MaxCols != wantCols {
		t.Errorf("MaxCols %d, want %d", other.MaxCols, wantCols)
	}
	if other.Collapsed() {
		t.Error("a gap survived expandAll")
	}
}

// Opening one gap leaves the rows on either side of it alone and keeps the
// rest of the run hidden behind a smaller gap.
func TestExpandingOneGapKeepsTheRestHidden(t *testing.T) {
	_, fd := gappyDoc(t)
	before := append([]Row(nil), fd.base...)
	gap, found := fd.firstGap()
	if !found {
		t.Fatal("the file has no gap to open")
	}
	if !fd.expand(gap, 4) {
		t.Fatal("the gap did not open")
	}

	// The heading still leads, then the four lines that were hidden, then what
	// is left of the gap, then everything that followed it.
	if fd.base[0].Kind != rowFile {
		t.Fatalf("row 0 is %v, want the heading", fd.base[0].Kind)
	}
	for n := range 4 {
		r := fd.base[1+n]
		if r.Kind != rowLine || r.Line.NewNum != gap.NewFrom+n {
			t.Fatalf("row %d is %+v, want context line %d", 1+n, r, gap.NewFrom+n)
		}
		if r.Line.Kind != diffparse.Context {
			t.Errorf("row %d is %v, want context", 1+n, r.Line.Kind)
		}
	}
	rest := fd.base[5]
	if rest.Kind != rowGap || rest.Gap.Count != gap.Count-4 || rest.Gap.NewFrom != gap.NewFrom+4 {
		t.Fatalf("what is left of the gap is %+v, want %d lines from %d", rest, gap.Count-4, gap.NewFrom+4)
	}
	// before[0] is the heading and before[1] the gap just opened.
	if got, want := fd.base[6:], before[2:]; len(got) != len(want) {
		t.Fatalf("%d rows follow the gap, want %d", len(got), len(want))
	} else {
		for i := range want {
			if got[i].Kind != want[i].Kind || !sameLine(got[i].Line, want[i].Line) {
				t.Fatalf("row %d after the gap differs:\n got  %+v\n want %+v", i, got[i], want[i])
			}
		}
	}
}

// The mark on a line a note covers is written onto the file's own row, so it
// has to come off again when the note goes.
func TestTheNotedMarkGoesWhenTheNoteDoes(t *testing.T) {
	a, doc := threeFileDoc(t)
	noted := func() int {
		n := 0
		for i := range doc.Rows {
			if doc.Row(i).Noted {
				n++
			}
		}
		return n
	}
	if n := noted(); n != 0 {
		t.Fatalf("%d rows are marked before any note was written", n)
	}

	c := a.store.AddComment("change", state.Comment{
		Path: doc.Files[1].Path, Side: state.SideNew, Line: 1, EndLine: 2, Body: "over both lines",
	})
	a.rebuildFile(1)
	if noted() == 0 {
		t.Fatal("a note over two lines marked neither of them")
	}

	a.store.DeleteComment("change", c.ID)
	a.rebuildFile(1)
	if n := noted(); n != 0 {
		t.Errorf("%d rows are still marked after the note went", n)
	}
}

// waitingDoc is the sheet as it goes up before any of the diff has arrived:
// every file the manifest names, with nothing under it yet.
func waitingDoc(t *testing.T) (*App, *DiffDoc, []diffparse.File) {
	t.Helper()
	parsed := diffparse.Parse(threeFileDiff())
	rows := make([]FileRow, len(parsed))
	for i := range parsed {
		rows[i].Path = parsed[i].Path()
	}
	doc := buildDoc(rows, nil)
	a := &App{store: state.New(), diff: doc}
	a.rev.ChangeIDFull = "change"
	a.rebuildRows()
	return a, doc, parsed
}

func TestTheSheetGoesUpBeforeTheDiffArrives(t *testing.T) {
	_, doc, _ := waitingDoc(t)
	if len(doc.Rows) != 3 {
		t.Fatalf("the waiting sheet has %d rows, want one heading per file", len(doc.Rows))
	}
	for i := range doc.Rows {
		if r := doc.Row(i); r.Kind != rowFile {
			t.Errorf("row %d is %v, want a file heading", i, r.Kind)
		}
	}
	for _, fd := range doc.Files {
		if fd.Note != "" {
			t.Errorf("%s says %q while its diff is still on its way", fd.Path, fd.Note)
		}
	}
}

// Each file's hunks land under its own heading, and the sheet ends up where
// building the whole change at once would have put it.
func TestFilesFillInAsTheDiffArrives(t *testing.T) {
	a, doc, parsed := waitingDoc(t)
	for i := range parsed {
		j := doc.attach(&parsed[i])
		if j < 0 {
			t.Fatalf("%s is not in the manifest", parsed[i].Path())
		}
		a.rebuildFile(j)
	}
	doc.settle()
	if len(doc.Rows) <= 3 {
		t.Fatalf("the sheet still has %d rows once every file has arrived", len(doc.Rows))
	}
	assertSameAsFullRebuild(t, a)
}

// A file the diff never mentions stops waiting when the diff ends.
func TestAFileTheDiffNeverMentionsSaysSo(t *testing.T) {
	a, doc, parsed := waitingDoc(t)
	a.rebuildFile(doc.attach(&parsed[0]))
	doc.settle()
	a.rebuildRows()

	if got := doc.Files[1].Note; got != "NO DIFF CONTENT" {
		t.Errorf("a file the diff never mentioned says %q, want NO DIFF CONTENT", got)
	}
	if got := doc.Files[0].Note; got != "" {
		t.Errorf("a file whose diff arrived says %q", got)
	}
}

// Rows appearing above the reading position must not slide the sheet out from
// under it. A file still arriving is the case the streamed load makes routine:
// its diff lands above whatever is being read, and the cursor and the viewport
// have to move with the rows they are on.
func TestRowsAppearingAboveDoNotMoveTheReadingPosition(t *testing.T) {
	a, doc, parsed := waitingDoc(t)
	a.files = make([]FileRow, len(doc.Files))
	for i, fd := range doc.Files {
		a.files[i].Path = fd.Path
	}

	// Reading the last file, before the diff of the earlier ones has arrived.
	a.showFile(2)
	was := doc.FileRows[2]
	if doc.Cursor != was || a.diffList.Position.First != was {
		t.Fatalf("showFile left the cursor at %d and the viewport at %d, want %d",
			doc.Cursor, a.diffList.Position.First, was)
	}

	a.rebuildFile(doc.attach(&parsed[0]))

	head := doc.FileRows[2]
	if head == was {
		t.Fatal("the file did not move, so nothing was tested")
	}
	if doc.Cursor != head {
		t.Errorf("the cursor is on row %d, want %d, where the file it was on moved to", doc.Cursor, head)
	}
	if a.diffList.Position.First != head {
		t.Errorf("the viewport starts at row %d, want %d", a.diffList.Position.First, head)
	}
	if got := doc.FileOf(doc.Cursor); got != 2 {
		t.Errorf("the cursor ended up in file %d, want the one it was reading", got)
	}
}

// A note belongs to a file, which is not necessarily the file the manifest is
// pointing at: acting on one from its own row leaves the selection where it is.
func TestANoteRefreshesItsOwnManifestRow(t *testing.T) {
	a, doc := threeFileDoc(t)
	a.files = make([]FileRow, len(doc.Files))
	for i, fd := range doc.Files {
		a.files[i].Path = fd.Path
	}
	a.fileSel = 0

	c := a.store.AddComment("change", state.Comment{
		Path: doc.Files[2].Path, Side: state.SideNew, Line: 1, Body: "on the last file",
	})
	a.afterCommentChange(c.Path)

	if a.files[2].Comments != 1 || a.files[2].Open != 1 {
		t.Errorf("the note's own row counts %d notes (%d open), want 1 and 1",
			a.files[2].Comments, a.files[2].Open)
	}
	if a.files[0].Comments != 0 {
		t.Errorf("the selected file's row counts %d notes, want none", a.files[0].Comments)
	}

	// Clearing the change touches every file, so every row has to be refreshed.
	a.store.ClearComments("change")
	a.afterCommentChange("")
	if a.files[2].Comments != 0 {
		t.Errorf("after clearing, the note's row still counts %d", a.files[2].Comments)
	}
}

func TestShiftRowMovesARowAcrossASplice(t *testing.T) {
	// A file occupying rows 10 to 19, rebuilt to a different length.
	const start, end = 10, 20
	for _, c := range []struct {
		about            string
		row, delta, want int
	}{
		{"before the file, which grew", 4, 5, 4},
		{"before the file, which shrank", 4, -4, 4},
		{"after the file, which grew", 25, 5, 30},
		{"after the file, which shrank", 25, -4, 21},
		{"inside the file, which grew", 15, 5, 15},
		{"inside the file, past where it now ends", 18, -5, 14},
		{"the file's own first row, which shrank to almost nothing", 10, -9, 10},
	} {
		if got := shiftRow(c.row, start, end, c.delta); got != c.want {
			t.Errorf("%s: row %d with delta %d moved to %d, want %d", c.about, c.row, c.delta, got, c.want)
		}
	}
}

// A full rebuild renumbers every row. The reading position is a place in the
// change rather than a row number, so it has to come back on the same
// code — which is what clearing every note, or a file arriving late, does.
func TestAFullRebuildKeepsTheReadingPosition(t *testing.T) {
	a, doc := threeFileDoc(t)
	a.showFile(2)
	was := doc.Cursor
	a.sel = Sel{Anchor: Spot{Row: was}, Head: Spot{Row: was}, Active: true, Lines: true}

	// A note on the first file puts a row above everything.
	a.store.AddComment("change", state.Comment{
		Path: doc.Files[0].Path, Side: state.SideNew, Line: 1, Body: "above it all",
	})
	a.rebuildRows()

	head := doc.FileRows[2]
	if head == was {
		t.Fatal("the file did not move, so nothing was tested")
	}
	if doc.Cursor != head {
		t.Errorf("the cursor is on row %d, want %d, where the file it was on moved to", doc.Cursor, head)
	}
	if got := doc.FileOf(doc.Cursor); got != 2 {
		t.Errorf("the cursor ended up in file %d, want the one it was reading", got)
	}
	if a.diffList.Position.First != head {
		t.Errorf("the viewport starts at row %d, want %d", a.diffList.Position.First, head)
	}
	if a.sel.Anchor.Row != head || a.sel.Head.Row != head {
		t.Errorf("the selection is on rows %d..%d, want %d", a.sel.Anchor.Row, a.sel.Head.Row, head)
	}
}

// settle only reports work when there was some: a change whose diff covered
// every file needs no second layout.
func TestSettleReportsOnlyTheFilesTheDiffMissed(t *testing.T) {
	_, doc, parsed := waitingDoc(t)
	for i := range parsed {
		doc.attach(&parsed[i])
	}
	if doc.settle() {
		t.Error("settle asked for a rebuild when every file had its diff")
	}

	_, other, parsedOther := waitingDoc(t)
	other.attach(&parsedOther[0])
	if !other.settle() {
		t.Error("settle left two files waiting and asked for no rebuild")
	}
}

// A deletion has no new side, so the runs it leaves out are measured on the old
// one. Read off the new side there is nothing there, and the file could not be
// opened up at all.
func TestADeletionHasItsGapsOnTheOldSide(t *testing.T) {
	parsed := diffparse.ParseOne("diff --git a/gone.go b/gone.go\n" +
		"deleted file mode 100644\n--- a/gone.go\n+++ /dev/null\n" +
		"@@ -10,2 +0,0 @@\n-ten\n-eleven\n")
	rows := make([]FileRow, 1)
	rows[0].Path = parsed[0].Path()
	fd := buildDoc(rows, parsed).Files[0]

	gap, found := fd.firstGap()
	if !found {
		t.Fatal("a deletion starting at line 10 shows no run of lines before it")
	}
	if gap.OldFrom != 1 || gap.Count != 9 {
		t.Errorf("the gap is %+v, want the nine old lines from line 1", gap)
	}
}

// widest is the largest line number in a set of lexed lines, which is enough to
// tell the two sides of a file apart.
func longestLine(l highlight.Lines, n int) int {
	end := 0
	for _, s := range l.Line(n) {
		end = max(end, s.End)
	}
	return end
}

// Where neither side can be named as a revision the colouring is lexed out of
// the diff itself. Dropping the removals leaves the new file, dropping the
// additions the old one; swapping the two coloured added lines from the old
// side and removed lines from the new.
func TestTheHighlightFallbackKeepsEachSideOnItsOwnLines(t *testing.T) {
	parsed := diffparse.ParseOne("diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n" +
		"@@ -1,1 +1,1 @@\n-var removedSideIsLonger = 1\n+var short = 2\n")
	f := &parsed[0]

	newHL := highlightFromHunks(f, diffparse.Removed)
	oldHL := highlightFromHunks(f, diffparse.Added)

	if got, want := longestLine(newHL, 1), len("var short = 2"); got != want {
		t.Errorf("the new side's line 1 is %d columns of spans, want %d", got, want)
	}
	if got, want := longestLine(oldHL, 1), len("var removedSideIsLonger = 1"); got != want {
		t.Errorf("the old side's line 1 is %d columns of spans, want %d", got, want)
	}
}
