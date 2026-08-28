package ui

import (
	"strconv"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"github.com/chromafish/peneira/reef"
)

func TestClickSelectsRevision(t *testing.T) {
	h := newHarness(t)
	if len(h.app.revs) < 2 {
		t.Fatalf("expected several revisions, got %d", len(h.app.revs))
	}
	first := h.app.revs[0].ChangeID

	h.click(h.paneX(PaneRevs), h.listY(1))

	if h.app.revSel != 1 {
		t.Fatalf("clicking the second row selected %d", h.app.revSel)
	}
	if h.app.rev.ChangeID == first {
		t.Error("the framed revision did not change")
	}
	if h.app.focus != PaneRevs {
		t.Errorf("focus = %v, want the revisions pane", h.app.focus)
	}
}

func TestClickSelectsFile(t *testing.T) {
	h := newHarness(t)
	if len(h.app.files) < 2 {
		t.Fatalf("expected several files, got %d", len(h.app.files))
	}
	want := h.app.files[1].Path

	h.click(h.paneX(PaneFiles), h.listY(1))

	if h.app.fileSel != 1 {
		t.Fatalf("clicking the second file selected %d", h.app.fileSel)
	}
	if h.app.diff == nil || h.app.diff.PathAt(h.app.diff.Cursor) != want {
		t.Errorf("the diff scrolled to %q, want %s", h.app.diff.PathAt(h.app.diff.Cursor), want)
	}
}

// Clicking a line of the diff must move the cursor there. Without this there
// is no way to say which line a note is about except by pressing j repeatedly.
func TestClickDiffLineMovesCursor(t *testing.T) {
	h := newHarness(t)
	h.selectFileWithRows(4)

	row := h.firstLineRow()
	h.click(h.paneX(PaneDiff), h.diffRowY(row))

	if h.app.diff.Cursor != row {
		t.Errorf("cursor = %d, want %d", h.app.diff.Cursor, row)
	}
	if h.app.focus != PaneDiff {
		t.Errorf("focus = %v, want the diff pane", h.app.focus)
	}
}

// Clicking the gutter is what starts a note, so the gesture is reachable with
// a mouse and not only from the keyboard.
func TestClickGutterStartsNote(t *testing.T) {
	h := newHarness(t)
	h.selectFileWithRows(2)

	row := h.firstLineRow()
	h.click(h.diffGutterX(), h.diffRowY(row))

	if h.app.draft == nil {
		t.Fatal("clicking the gutter did not open a note editor")
	}
	if h.app.draft.line == 0 {
		t.Error("the draft is not anchored to a line")
	}
}

// The affordance has to appear under the pointer, or nobody will discover it.
func TestGutterHoverIsTracked(t *testing.T) {
	h := newHarness(t)
	h.selectFileWithRows(2)

	row := h.firstLineRow()
	h.hover(h.diffGutterX(), h.diffRowY(row))
	if h.app.hoverRow != row {
		t.Errorf("hoverRow = %d, want %d", h.app.hoverRow, row)
	}

	// Moving away clears it, so the + does not stick to a stale line.
	h.hover(h.paneX(PaneDiff), h.diffRowY(row))
	if h.app.hoverRow == row {
		t.Error("the hover mark stayed on the row after leaving the gutter")
	}
}

func TestWritingAndCopyingANote(t *testing.T) {
	h := newHarness(t)
	// A Go file, so the export's language tagging is exercised too.
	h.selectFileNamed("main.go")

	row := h.firstLineRow()
	h.click(h.diffGutterX(), h.diffRowY(row))
	h.typeText("this allocates on every frame")
	h.press(key.NameReturn, key.ModShortcut)

	notes := h.app.openNotes()
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}
	if notes[0].Body != "this allocates on every frame" {
		t.Errorf("note body = %q", notes[0].Body)
	}
	if h.app.draft != nil {
		t.Error("the editor stayed open after saving")
	}

	h.press("C", key.ModShortcut|key.ModShift)
	out := h.clipboard()

	for _, want := range []string{
		"# Code review: 1 note to address",
		h.app.repo.Root(),
		"this allocates on every frame",
		notes[0].Path,
		"```go",
		"→",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the copied text is missing %q:\n%s", want, out)
		}
	}
}

// A message that never goes away is worse than no message.
func TestStatusMessagesExpire(t *testing.T) {
	h := newHarness(t)
	h.app.note("something happened")
	h.frame()
	if h.app.status == "" {
		t.Fatal("the message was dropped immediately")
	}

	h.advance(statusLife / 2)
	if h.app.status == "" {
		t.Error("the message went away too early")
	}

	h.advance(statusLife)
	if h.app.status != "" {
		t.Errorf("the message is still up after %v: %q", statusLife, h.app.status)
	}
}

func TestDraggingTheSplitterResizesColumns(t *testing.T) {
	h := newHarness(t)
	before := h.app.splits.Fraction(int(PaneRevs))

	gtx := h.gtx()
	revs := h.app.splits.Widths(gtx, h.size.X)[0]
	y := h.headerHeight() + 200
	h.drag(revs, y, revs+120, y)

	if h.app.splits.Fraction(int(PaneRevs)) <= before {
		t.Errorf("dragging right did not widen the revisions column: %v → %v",
			before, h.app.splits.Fraction(int(PaneRevs)))
	}

	// The rule has to land under the pointer, not at some multiple of the
	// distance dragged: a splitter that overshoots pins itself at its limit
	// and cannot be brought back.
	moved := h.app.splits.Widths(h.gtx(), h.size.X)[0]
	if off := moved - (revs + 120); off < -4 || off > 4 {
		t.Errorf("the rule landed %dpx from the pointer: %d, want %d", off, moved, revs+120)
	}

	// And it comes back.
	h.drag(moved, y, moved-160, y)
	back := h.app.splits.Widths(h.gtx(), h.size.X)[0]
	if off := back - (moved - 160); off < -4 || off > 4 {
		t.Errorf("dragging left did not return the rule: %d, want %d", back, moved-160)
	}
}

func TestDraggingTheSecondSplitterResizesTheManifest(t *testing.T) {
	h := newHarness(t)
	gtx := h.gtx()
	w := h.app.splits.Widths(gtx, h.size.X)
	revs, files := w[0], w[1]
	y := h.headerHeight() + 200

	h.drag(revs+files+1, y, revs+files+1+100, y)

	moved := h.app.splits.Widths(h.gtx(), h.size.X)[1]
	if off := moved - (files + 100); off < -4 || off > 4 {
		t.Errorf("the manifest rule landed %dpx from the pointer: %d, want %d",
			off, moved, files+100)
	}
}

func TestNotesSheetListsEveryNote(t *testing.T) {
	h := newHarness(t)
	h.selectFileWithRows(2)
	h.click(h.diffGutterX(), h.diffRowY(h.firstLineRow()))
	h.typeText("first")
	h.press(key.NameReturn, key.ModShortcut)

	h.press("N", key.ModShift)
	if !h.app.notesOpen {
		t.Fatal("shift-N did not open the notes sheet")
	}
	h.press(key.NameEscape, 0)
	if h.app.notesOpen {
		t.Error("escape did not close the notes sheet")
	}
}

func TestClearingNotesEmptiesTheChange(t *testing.T) {
	h := newHarness(t)
	h.selectFileWithRows(2)
	h.click(h.diffGutterX(), h.diffRowY(h.firstLineRow()))
	h.typeText("first")
	h.press(key.NameReturn, key.ModShortcut)
	if len(h.app.openNotes()) != 1 {
		t.Fatalf("wrote a note but the change has %d", len(h.app.openNotes()))
	}

	h.app.clearNotes()
	h.frame()
	if n := len(h.app.openNotes()); n != 0 {
		t.Errorf("%d notes survived the clear", n)
	}
	if !strings.Contains(h.app.status, "cleared") {
		t.Errorf("status = %q, want it to say the notes went", h.app.status)
	}
	if h.app.notesOpen {
		t.Error("the notes sheet stayed open over an empty change")
	}
	// The manifest badge and the diff rows both read from the store, so both
	// have to have noticed.
	for _, f := range h.app.files {
		if f.Comments != 0 {
			t.Errorf("%s still carries %d notes in the manifest", f.Path, f.Comments)
		}
	}
	for i := range h.app.diff.Rows {
		if h.app.diff.Row(i).Kind == rowComment {
			t.Error("a note row survived the clear")
			break
		}
	}
}

func TestClearWithNoNotesSaysSo(t *testing.T) {
	h := newHarness(t)
	h.app.clearNotes()
	if !strings.Contains(h.app.status, "no notes") {
		t.Errorf("status = %q, want it to say there is nothing to clear", h.app.status)
	}
}

func TestCopyWithNoNotesSaysSo(t *testing.T) {
	h := newHarness(t)
	h.press("C", key.ModShortcut|key.ModShift)
	if got := h.clipboard(); got != "" {
		t.Errorf("copied %q with no notes", got)
	}
	if !strings.Contains(h.app.status, "no notes") {
		t.Errorf("status = %q, want it to say there is nothing to copy", h.app.status)
	}
}

// The read/unread box has to be usable with a mouse; it is the only per-file
// state in the interface and it was previously keyboard-only.
func TestClickViewedBoxTogglesRead(t *testing.T) {
	h := newHarness(t)
	if len(h.app.files) < 2 {
		t.Fatalf("expected several files, got %d", len(h.app.files))
	}
	if h.app.files[0].Viewed {
		t.Fatal("a fresh review should start with nothing read")
	}

	h.click(h.manifestBoxX(), h.listY(0))

	if !h.app.files[0].Viewed {
		t.Error("clicking the box did not mark the file read")
	}
	// Unlike the keyboard, clicking a specific file should not then move to a
	// different one.
	if h.app.fileSel != 0 {
		t.Errorf("selection moved to %d after clicking the box", h.app.fileSel)
	}

	h.click(h.manifestBoxX(), h.listY(0))
	if h.app.files[0].Viewed {
		t.Error("clicking the box again did not unmark the file")
	}
}

func TestViewedBoxHoverIsTracked(t *testing.T) {
	h := newHarness(t)
	h.hover(h.manifestBoxX(), h.listY(0))
	if !h.app.hovered(viewedTag{0, false}) {
		t.Error("the box does not report being under the pointer")
	}
}

// Progress belongs where it can be seen without counting boxes.
func TestManifestHeaderCountsProgress(t *testing.T) {
	h := newHarness(t)
	total := len(h.app.files)
	if got, want := h.app.manifestTitle(), "MANIFEST · 0/"; !strings.HasPrefix(got, want) {
		t.Errorf("header = %q, want it to start %q", got, want)
	}

	h.click(h.manifestBoxX(), h.listY(0))

	want := "MANIFEST · 1/" + strconv.Itoa(total) + " READ"
	if got := h.app.manifestTitle(); got != want {
		t.Errorf("header = %q, want %q", got, want)
	}
}

// Marking a file read and then rewriting the change must show as stale rather
// than silently staying read.
func TestRewritingTheChangeMakesReadFilesStale(t *testing.T) {
	h := newHarness(t)
	h.click(h.manifestBoxX(), h.listY(0))
	if h.app.files[0].Stale {
		t.Fatal("a file just read should not be stale")
	}

	h.amendWorkingCopy("main.go", "package main\n\nfunc main() {\n\tprintln(\"four\")\n}\n")
	h.press("R", 0)

	for _, f := range h.app.files {
		if f.Viewed && !f.Stale {
			t.Errorf("%s is still marked read after the change was rewritten", f.Path)
		}
	}
}

func TestHidingATreeLeavesARailThatBringsItBack(t *testing.T) {
	h := newHarness(t)
	h.press("1", 0)
	if !h.app.splits.Hidden(int(PaneRevs)) {
		t.Fatal("pressing 1 did not put the revisions column away")
	}

	gtx := h.gtx()
	revs := h.app.splits.Widths(gtx, h.size.X)[0]
	if want := gtx.Dp(reef.RailW); revs != want {
		t.Errorf("the rail is %dpx wide, want %d", revs, want)
	}
	if h.app.focus == PaneRevs {
		t.Error("focus stayed in a column that is not on screen")
	}

	// The rail is the way back, without knowing the key.
	h.click(revs/2, h.headerHeight()+100)
	if h.app.splits.Hidden(int(PaneRevs)) {
		t.Error("clicking the rail did not bring the column back")
	}
}

func TestCodeOnlyPutsBothTreesAway(t *testing.T) {
	h := newHarness(t)
	h.press("Z", 0)
	if !h.app.splits.Hidden(int(PaneRevs)) || !h.app.splits.Hidden(int(PaneFiles)) {
		t.Fatal("z did not put both trees away")
	}
	if h.app.focus != PaneDiff {
		t.Errorf("focus is %v, want the diff", h.app.focus)
	}
	h.press("Z", 0)
	if h.app.splits.Hidden(int(PaneRevs)) || h.app.splits.Hidden(int(PaneFiles)) {
		t.Error("z did not bring the trees back")
	}
}

// The whole change is one sheet: every file is printed, in order, so it reads
// from the top without going back to the manifest.
func TestDiffPrintsEveryFile(t *testing.T) {
	h := newHarness(t)
	doc := h.app.diff
	if doc == nil {
		t.Fatal("no diff was loaded")
	}
	if len(doc.Files) != len(h.app.files) {
		t.Fatalf("the sheet has %d files, the manifest %d", len(doc.Files), len(h.app.files))
	}
	if len(doc.FileRows) != len(h.app.files) {
		t.Errorf("%d headings for %d files", len(doc.FileRows), len(h.app.files))
	}
	for i, fd := range doc.Files {
		if fd.Path != h.app.files[i].Path {
			t.Errorf("file %d is %s, the manifest says %s", i, fd.Path, h.app.files[i].Path)
		}
	}
}

// Scrolling the diff moves the manifest with it, so the two never disagree
// about which file is being read.
func TestScrollingTheDiffFollowsTheManifest(t *testing.T) {
	h := newHarness(t)
	doc := h.app.diff
	if doc == nil || len(doc.FileRows) < 2 {
		t.Skip("the fixture has only one file")
	}
	// Asked directly, because a change short enough to fit on screen cannot be
	// scrolled at all: the list would put the position straight back.
	h.app.diffList.Position.First = doc.FileRows[1]
	h.app.diffList.Position.Offset = 0
	h.app.syncFileSel()
	if h.app.fileSel != 1 {
		t.Errorf("scrolling to the second file left the manifest on %d", h.app.fileSel)
	}
}

// A diff hides the code around a change; it must say so, and opening it must
// not need a second trip to jj.
func TestExpandingAGapShowsSurroundingCode(t *testing.T) {
	h := newHarness(t)
	doc := h.app.diff
	gap, file := -1, -1
	for i := range doc.Rows {
		if r := doc.Row(i); r.Kind == rowGap && r.Gap.Count > 0 {
			gap, file = i, doc.FileOf(i)
			break
		}
	}
	if gap < 0 {
		t.Skip("no file in the fixture has code hidden between hunks")
	}
	before := countLines(doc.Files[file])

	h.app.selectFile(file)
	h.settle()
	h.app.expandGap(gapRowIn(doc, file), expandStep)
	h.settle()
	h.app.expandGap(gapRowIn(doc, file), expandStep)
	h.settle()

	if after := countLines(doc.Files[file]); after <= before {
		t.Errorf("expanding showed %d lines, was %d", after, before)
	}
}

// And the whole file is a toggle, not a one-way door.
func TestWholeFileTogglesBothWays(t *testing.T) {
	h := newHarness(t)
	doc := h.app.diff
	file := -1
	for i, fd := range doc.Files {
		if fd.File != nil && len(fd.File.Hunks) > 0 {
			file = i
			break
		}
	}
	if file < 0 {
		t.Skip("the fixture has no file with hunks")
	}
	h.app.selectFile(file)
	h.settle()
	before := countLines(doc.Files[file])

	h.app.expandFile(file)
	h.settle()
	h.app.expandFile(file)
	h.settle()
	whole := countLines(doc.Files[file])
	if whole < before {
		t.Fatalf("the whole file has %d lines, the hunks alone had %d", whole, before)
	}
	if doc.Files[file].Collapsed() {
		t.Error("the file still has something left out after being opened whole")
	}

	h.app.collapseFile(file)
	h.settle()
	if got := countLines(doc.Files[file]); got != before {
		t.Errorf("collapsing left %d lines, want the %d the hunks name", got, before)
	}
}

func countLines(fd *FileDoc) int {
	n := 0
	for _, r := range fd.base {
		if r.Kind == rowLine {
			n++
		}
	}
	return n
}

// gapRowIn finds the first gap of a file in the flattened rows.
func gapRowIn(doc *DiffDoc, file int) int {
	for i := range doc.Rows {
		if doc.Row(i).Kind == rowGap && doc.FileOf(i) == file {
			return i
		}
	}
	return -1
}

// Code has to be able to leave the tool: select it and copy it.
func TestSelectingLinesAndCopyingThem(t *testing.T) {
	h := newHarness(t)
	h.selectFileNamed("wide.go")
	row := h.firstLineRow()
	h.click(h.paneX(PaneDiff), h.diffRowY(row))

	h.press("J", key.ModShift)
	h.press("J", key.ModShift)
	if h.app.sel.Empty() {
		t.Fatal("shift-j did not start a selection")
	}
	first, last, ok := h.app.selectedLines()
	if !ok || last <= first {
		t.Fatalf("the selection covers rows %d..%d", first, last)
	}

	h.press("C", key.ModShortcut)
	out := h.clipboard()
	if out == "" {
		t.Fatal("cmd-c copied nothing")
	}
	want := h.app.diff.Row(first).Line.Text
	if !strings.Contains(out, want) {
		t.Errorf("the copied text is missing the first selected line %q:\n%s", want, out)
	}
	if lines := strings.Count(out, "\n") + 1; lines != last-first+1 {
		t.Errorf("copied %d lines, selected %d", lines, last-first+1)
	}
}

// The same thing with the pointer, which is how most people will reach for it.
func TestDraggingSelectsCode(t *testing.T) {
	h := newHarness(t)
	h.selectFileNamed("wide.go")
	row := h.firstLineRow()

	x := h.paneX(PaneDiff)
	h.drag(x, h.diffRowY(row), x, h.diffRowY(row+2))

	if h.app.sel.Empty() {
		t.Fatal("dragging over the code selected nothing")
	}
	from, to := h.app.sel.Ordered()
	if from.Row != row || to.Row != row+2 {
		t.Errorf("the selection covers rows %d..%d, want %d..%d", from.Row, to.Row, row, row+2)
	}
}

// A note is often about a passage rather than a line.
func TestCommentingOnSeveralLines(t *testing.T) {
	h := newHarness(t)
	h.selectFileNamed("wide.go")
	row := h.firstLineRow()
	h.click(h.paneX(PaneDiff), h.diffRowY(row))
	h.press("J", key.ModShift)
	h.press("J", key.ModShift)

	first, last, _ := h.app.selectedLines()
	h.press("C", 0)
	if h.app.draft == nil {
		t.Fatal("c did not open a note editor")
	}
	if h.app.draft.endLine <= h.app.draft.line {
		t.Fatalf("the note is anchored to line %d only, want a run",
			h.app.draft.line)
	}

	h.typeText("this whole block is dead")
	h.press(key.NameReturn, key.ModShortcut)

	notes := h.app.openNotes()
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}
	c := notes[0]
	if !c.Span() {
		t.Errorf("the saved note covers only line %d", c.Line)
	}
	if got := c.LastLine() - c.Line + 1; got != last-first+1 {
		t.Errorf("the note covers %d lines, %d were selected", got, last-first+1)
	}

	// It is filed under the last line of the run, and the lines it covers say so.
	marked := 0
	for i := range h.app.diff.Rows {
		if h.app.diff.Row(i).Noted {
			marked++
		}
	}
	if marked == 0 {
		t.Error("no line is marked as covered by the note")
	}
}

// Stepping between files scrolls the one sheet rather than loading a new one.
func TestSteppingBetweenFilesScrollsTheSheet(t *testing.T) {
	h := newHarness(t)
	doc := h.app.diff
	if doc == nil || len(doc.FileRows) < 2 {
		t.Skip("the fixture has only one file")
	}
	before := doc

	h.press("]", 0)
	if h.app.fileSel != 1 {
		t.Errorf("] moved to file %d, want 1", h.app.fileSel)
	}
	if h.app.diff != before {
		t.Error("stepping to the next file reloaded the diff")
	}
	if got := doc.PathAt(doc.Cursor); got != h.app.files[1].Path {
		t.Errorf("the cursor is in %s, want %s", got, h.app.files[1].Path)
	}

	h.press("[", 0)
	if h.app.fileSel != 0 {
		t.Errorf("[ moved to file %d, want 0", h.app.fileSel)
	}
}

// A pointer handler runs part way through the frame it belongs to, so whatever
// it changes is not on screen until another frame is drawn. Nothing else asks
// for that frame: without an explicit request the change sits there invisibly
// until some unrelated event happens to redraw, which reads as the window
// having frozen for seconds at a time.
func TestClickingAChevronAsksForAnotherFrame(t *testing.T) {
	h := newHarness(t)
	gtx := h.gtx()
	w := h.app.splits.Widths(gtx, h.size.X)
	revs, files := w[0], w[1]
	head := gtx.Dp(reef.ControlH)

	// The collapse control sits at the right of the manifest's header.
	x := revs + 1 + files - head/2
	y := h.headerHeight() + head/2

	h.router.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons:  pointer.ButtonPrimary,
		Position: f32.Pt(float32(x), float32(y)),
	})
	h.frame()

	if !h.app.splits.Hidden(int(PaneFiles)) {
		t.Fatalf("clicking the chevron at %d,%d did not put the manifest away", x, y)
	}
	if _, ok := h.router.WakeupTime(); !ok {
		t.Error("the click changed the layout without asking for the frame that would show it")
	}
}

// settleFrames draws until the interface stops asking to be drawn, and reports
// how many frames that took. What is on screen when it stops is what the user
// is left looking at.
func (h *harness) settleFrames() int {
	h.t.Helper()
	for i := 1; i <= 20; i++ {
		h.frame()
		if _, more := h.router.WakeupTime(); !more {
			return i
		}
	}
	return 20
}

// A pointer handler runs part way through the frame it belongs to, so what it
// changes is not on screen until another frame is drawn — and Gio only draws
// when an event asks it to. A mouse release nobody filters is not such an
// event, so without an explicit request the interface comes to rest still
// showing the old layout, which is exactly what a freeze looks like.
func TestClickingAChevronRedrawsTheLayout(t *testing.T) {
	h := newHarness(t)
	gtx := h.gtx()
	w := h.app.splits.Widths(gtx, h.size.X)
	revs, files := w[0], w[1]
	head := gtx.Dp(reef.ControlH)

	h.router.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons:  pointer.ButtonPrimary,
		Position: f32.Pt(float32(revs+1+files-head/2), float32(h.headerHeight()+head/2)),
	})
	h.settleFrames()

	if !h.app.splits.Hidden(int(PaneFiles)) {
		t.Fatal("clicking the chevron did not put the manifest away")
	}
	if want := gtx.Dp(reef.RailW); h.app.drawnFilesW != want {
		t.Errorf("the interface came to rest still drawing the manifest %dpx wide, want the %dpx rail",
			h.app.drawnFilesW, want)
	}
}

// noteControlX finds where one of a note's controls was drawn, by sweeping the
// pointer along its row until the design system reports that control as hovered.
func (h *harness) noteControlX(row int, tag any) int {
	h.t.Helper()
	y := h.diffRowY(row)
	for x := h.size.X - 4; x > h.app.diffColX; x -= 4 {
		h.hover(x, y)
		if h.app.hovered(tag) {
			return x
		}
	}
	h.t.Fatalf("no control %v was drawn on row %d", tag, row)
	return 0
}

// The controls on a note are drawn inside the diff list, so acting on one may
// not add or remove rows where it stands: the list is part way through walking
// a count it has already taken, and the row that goes is below everything left
// to draw. Deleting the last note in a change is what used to walk off the end.
func TestDeletingANoteFromItsOwnRowSurvivesTheFrame(t *testing.T) {
	h := newHarness(t)
	h.selectFileNamed("main.go")

	row := h.firstLineRow()
	h.click(h.diffGutterX(), h.diffRowY(row))
	h.typeText("delete me from my own row")
	h.press(key.NameReturn, key.ModShortcut)

	notes := h.app.openNotes()
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}

	noteRow := -1
	for i := range h.app.diff.Rows {
		if h.app.diff.Row(i).Kind == rowComment {
			noteRow = i
		}
	}
	if noteRow < 0 {
		t.Fatal("the note has no row of its own")
	}

	before := len(h.app.diff.Rows)
	h.click(h.noteControlX(noteRow, noteTag{notes[0].ID, "delete"}), h.diffRowY(noteRow))

	if n := len(h.app.openNotes()); n != 0 {
		t.Fatalf("%d notes survived the delete", n)
	}
	if after := len(h.app.diff.Rows); after >= before {
		t.Errorf("the sheet went from %d rows to %d, want the note's row gone", before, after)
	}
}

// A control drawn inside the diff list may not add or remove rows in the frame
// it is clicked in: the list is part way through walking a count it has already
// taken. Everything these controls do is queued for the frame after.
func TestTheGutterDoesNotRestructureTheFrameItIsClickedIn(t *testing.T) {
	h := newHarness(t)
	h.selectFileNamed("main.go")
	row := h.firstLineRow()

	before := len(h.app.diff.Rows)
	h.clickOnce(h.diffGutterX(), h.diffRowY(row))
	if during := len(h.app.diff.Rows); during != before {
		t.Errorf("the sheet went from %d rows to %d inside the frame the click landed in", before, during)
	}
	h.settle()
	if after := len(h.app.diff.Rows); after <= before {
		t.Errorf("the sheet has %d rows after settling, want the draft row to have appeared", after)
	}
}

func TestEditingANoteDoesNotRestructureTheFrameItIsClickedIn(t *testing.T) {
	h := newHarness(t)
	h.selectFileNamed("main.go")

	row := h.firstLineRow()
	h.click(h.diffGutterX(), h.diffRowY(row))
	h.typeText("a note to revise")
	h.press(key.NameReturn, key.ModShortcut)

	notes := h.app.openNotes()
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}
	noteRow := -1
	for i := range h.app.diff.Rows {
		if h.app.diff.Row(i).Kind == rowComment {
			noteRow = i
		}
	}
	if noteRow < 0 {
		t.Fatal("the note has no row of its own")
	}

	x := h.noteControlX(noteRow, noteTag{notes[0].ID, "edit"})
	before := len(h.app.diff.Rows)
	h.clickOnce(x, h.diffRowY(noteRow))
	if during := len(h.app.diff.Rows); during != before {
		t.Errorf("the sheet went from %d rows to %d inside the frame the click landed in", before, during)
	}
	h.settle()
	if h.app.draft == nil {
		t.Error("the editor never opened")
	}
}

// The two-column view has to be a view of the same change: the reader keeps
// their place across the switch, both halves of a line can be pointed at, and a
// note is as tall as it needs to be rather than one code row.
func TestSideBySideIsDrivenLikeTheUnifiedView(t *testing.T) {
	h := newHarness(t)
	h.selectFileNamed("wide.go")
	// Whatever the unified view settled on is what the split view must show.
	was := h.app.diffList.Position.First
	h.press("\\", 0)
	if !h.app.sideBySide {
		t.Fatal("the split view never opened")
	}

	// The reader keeps their place: the row they were on is still on screen.
	doc := h.app.diff
	if _, ok := h.app.rowTops[was]; !ok {
		t.Errorf("row %d was on screen in the unified view and is not in the split one", was)
	}

	// Both sides of a replaced run are placed, and at the same height.
	var left, right int = -1, -1
	for _, p := range doc.pairs() {
		if p.Span < 0 && p.Left >= 0 && p.Right >= 0 {
			if _, ok := h.app.rowTops[p.Left]; ok {
				left, right = p.Left, p.Right
				break
			}
		}
	}
	if left < 0 {
		t.Fatal("no two-column line was drawn")
	}
	if h.app.rowTops[left] != h.app.rowTops[right] {
		t.Errorf("the two halves of a line sit at %d and %d", h.app.rowTops[left], h.app.rowTops[right])
	}

	// The gutter of a half starts a note on that half's own line.
	y := h.diffRowY(right)
	x := h.app.diffColX + (h.gtx().Constraints.Max.X-h.app.diffColX)/2 + 6
	h.click(x, y)
	if h.app.draft == nil {
		t.Fatal("clicking the gutter of the right column started no note")
	}
	h.press(key.NameEscape, 0)

	// Switching back keeps the reader where they were.
	h.press("\\", 0)
	if h.app.sideBySide {
		t.Fatal("the split view never closed")
	}
	if _, ok := h.app.rowTops[was]; !ok {
		t.Errorf("row %d went off screen coming back to the unified view", was)
	}
}

// A note is drawn across both columns, so it gets the height it asks for
// rather than the height of one line of code.
func TestSideBySideGivesANoteItsOwnHeight(t *testing.T) {
	h := newHarness(t)
	h.selectFileNamed("main.go")
	row := h.firstLineRow()
	h.click(h.diffGutterX(), h.diffRowY(row))
	h.typeText("a note long enough to need more than one line of the pane to draw")
	h.press(key.NameReturn, key.ModShortcut)

	h.press("\\", 0)
	h.settle()

	noteRow := -1
	for i := range h.app.diff.Rows {
		if h.app.diff.Row(i).Kind == rowComment {
			noteRow = i
		}
	}
	if noteRow < 0 {
		t.Fatal("the note has no row of its own")
	}
	code := h.app.ui.CodeRow(h.gtx())
	if got := h.app.rowHeights[noteRow]; got <= code {
		t.Errorf("the note is %dpx tall in the split view, no more than one code row at %dpx", got, code)
	}
}
