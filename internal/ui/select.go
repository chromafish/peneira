package ui

import (
	"image"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"

	"github.com/chromafish/peneira/internal/diffparse"
	"github.com/chromafish/peneira/internal/state"

	"github.com/chromafish/peneira/reef"
)

// Code has to be able to leave the window: into a message, into a terminal,
// into a note. The diff is drawn rather than laid out as text, so selection is
// done here: the body is a character grid, which makes a position on screen and
// a position in the text the same arithmetic.

// Spot is a place in the diff: a row, and a column measured in character cells
// of the displayed line, with tabs already expanded.
type Spot struct {
	Row int
	Col int
}

// Before reports whether one spot comes earlier than another.
func (s Spot) Before(o Spot) bool {
	if s.Row != o.Row {
		return s.Row < o.Row
	}
	return s.Col < o.Col
}

// Sel is the run of the diff that is selected. Whole-line selections keep the
// same shape with the columns thrown wide, so one piece of code paints and
// copies both.
type Sel struct {
	Anchor Spot
	Head   Spot
	Active bool
	Lines  bool // the selection covers whole lines, however it was made
}

// Ordered returns the selection's ends in reading order.
func (s Sel) Ordered() (from, to Spot) {
	if s.Head.Before(s.Anchor) {
		return s.Head, s.Anchor
	}
	return s.Anchor, s.Head
}

// Empty reports whether the selection covers nothing.
func (s Sel) Empty() bool {
	return !s.Active || (s.Anchor == s.Head && !s.Lines)
}

// Cols returns the columns of one row that the selection covers, or false when
// the row is outside it. A whole-line selection reports the widest possible
// range, which each row then clips to its own length.
func (s Sel) Cols(row int) (c0, c1 int, ok bool) {
	if s.Empty() {
		return 0, 0, false
	}
	from, to := s.Ordered()
	if row < from.Row || row > to.Row {
		return 0, 0, false
	}
	if s.Lines {
		return 0, 1 << 30, true
	}
	c0, c1 = 0, 1<<30
	if row == from.Row {
		c0 = from.Col
	}
	if row == to.Row {
		c1 = to.Col
	}
	if c1 <= c0 && from.Row == to.Row {
		return 0, 0, false
	}
	return c0, c1, true
}

// selectArea makes one line of code a place text can be swept out from. It is
// registered per row rather than over the whole body because the scrolling
// list puts its own gesture area over the body, and only the topmost area
// hears a press.
func (a *App) selectArea(gtx layout.Context, r image.Rectangle, row, gut int, cell image.Point) {
	tag := selTag{row}
	stack := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	pointer.CursorText.Add(gtx.Ops)
	stack.Pop()

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: tag,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		col := max(0, (int(pe.Position.X)-gut)/max(1, cell.X)+a.diffX)

		switch pe.Kind {
		case pointer.Press:
			// The press that starts a selection also puts the cursor on the
			// line.
			if doc := a.diff; doc != nil && row < len(doc.Rows) {
				doc.Cursor = row
			}
			a.focus = PaneDiff
			spot := Spot{Row: row, Col: col}
			a.sel = Sel{Anchor: spot, Head: spot, Active: true}
			a.selDrag, a.selRow = true, row
			reef.Redraw(gtx)
		case pointer.Drag:
			if !a.selDrag || a.selRow != row {
				continue
			}
			// A drag keeps coming to the row it started on, so where the
			// pointer has got to is worked out from that row's own position.
			over := row
			if top, ok := a.rowTops[row]; ok {
				if at := a.rowAtY(top + int(pe.Position.Y)); at >= 0 {
					over = at
				}
			}
			a.sel.Head = Spot{Row: over, Col: col}
			reef.Redraw(gtx)
		case pointer.Release, pointer.Cancel:
			reef.Redraw(gtx)
			// Only the row the sweep started on can end it. Rows scrolled out
			// from under the pointer are cancelled by the router as they go,
			// and those cancellations are not the end of a drag.
			if !a.selDrag || a.selRow != row {
				continue
			}
			a.selDrag = false
			if a.sel.Anchor == a.sel.Head {
				a.sel.Active = false
			}
		}
	}
}

// selTag names the selectable region of one row.
type selTag struct{ row int }

// SelectLines marks a run of whole rows, which is how the keyboard selects and
// how a comment on several lines is asked for.
func (a *App) selectLines(from, to int) {
	a.sel = Sel{
		Anchor: Spot{Row: from},
		Head:   Spot{Row: to},
		Active: true,
		Lines:  true,
	}
}

// extendSelection grows a whole-line selection from the cursor, one row at a
// time. Starting one is the same gesture as moving with it already open.
func (a *App) extendSelection(delta int) {
	doc := a.diff
	if doc == nil || len(doc.Rows) == 0 {
		return
	}
	a.focus = PaneDiff
	if a.sel.Empty() || !a.sel.Lines {
		a.selectLines(doc.Cursor, doc.Cursor)
	}
	head := clamp(a.sel.Head.Row+delta, 0, len(doc.Rows)-1)
	a.sel.Head = Spot{Row: head}
	doc.Cursor = head
	a.scrollTo(head)
}

// clearSelection drops the selection, which escape does and any fresh cursor
// move does too.
func (a *App) clearSelection() {
	a.sel = Sel{}
	a.selDrag = false
}

// selectedLines reports the run of code lines the selection covers, as row
// indices. Rows that are not code — headings, hunk rules, notes — are ignored,
// so sweeping across one does not break the run.
func (a *App) selectedLines() (first, last int, ok bool) {
	doc := a.diff
	if doc == nil || a.sel.Empty() {
		return 0, 0, false
	}
	from, to := a.sel.Ordered()
	first, last = -1, -1
	for i := max(0, from.Row); i <= min(to.Row, len(doc.Rows)-1); i++ {
		if doc.Row(i).Kind != rowLine {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	return first, last, first >= 0
}

// SelectedText is the selection as text, ready for the clipboard. Line numbers
// and markers are left behind: what comes out is the code as it is written in
// the file, tabs and all.
func (a *App) selectedText() string {
	doc := a.diff
	if doc == nil || a.sel.Empty() {
		return ""
	}
	from, to := a.sel.Ordered()
	var b strings.Builder
	n := 0
	for i := max(0, from.Row); i <= min(to.Row, len(doc.Rows)-1); i++ {
		r := doc.Row(i)
		if r.Kind != rowLine {
			continue
		}
		c0, c1, ok := a.sel.Cols(i)
		if !ok {
			continue
		}
		if n > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(sliceCols(r.Line.Text, c0, c1))
		n++
	}
	return b.String()
}

// sliceCols cuts a line between two display columns. Tabs advance the column
// by their width but are copied whole, so what lands on the clipboard is what
// is in the file rather than what happened to be drawn.
func sliceCols(text string, c0, c1 int) string {
	if c1 <= c0 {
		return ""
	}
	var b strings.Builder
	col := 0
	for _, r := range text {
		w := 1
		if r == '\t' {
			w = tabWidth - col%tabWidth
		}
		if col >= c0 && col < c1 {
			b.WriteRune(r)
		}
		col += w
		if col >= c1 {
			break
		}
	}
	return b.String()
}

// copySelection puts the selected code on the clipboard. With nothing selected
// it takes the line under the cursor, because that is what copying means with
// nothing swept out.
func (a *App) copySelection() {
	text := a.selectedText()
	lines := 0
	if text != "" {
		lines = strings.Count(text, "\n") + 1
	} else if doc := a.diff; doc != nil && doc.Cursor < len(doc.Rows) {
		if r := doc.Row(doc.Cursor); r.Kind == rowLine {
			text, lines = r.Line.Text, 1
		}
	}
	if text == "" {
		a.note("nothing to copy")
		return
	}
	a.clipboard = text
	if lines == 1 {
		a.note("copied 1 line")
	} else {
		a.note("copied %d lines", lines)
	}
}

// selectedRange turns a run of selected rows into the side and the two line
// numbers a note about them is filed under. A run that is entirely removals is
// a note about the old side; anything else is about the new one.
func (a *App) selectedRange(first, last int) (side state.Side, from, to int, ok bool) {
	doc := a.diff
	if doc == nil || first < 0 || last >= len(doc.Rows) {
		return "", 0, 0, false
	}
	file := doc.FileOf(first)
	allRemoved := true
	for i := first; i <= last; i++ {
		r := doc.Row(i)
		if r.Kind != rowLine || doc.FileOf(i) != file {
			continue
		}
		if r.Line.Kind != diffparse.Removed {
			allRemoved = false
		}
	}

	side = state.SideNew
	if allRemoved {
		side = state.SideOld
	}
	for i := first; i <= last; i++ {
		r := doc.Row(i)
		if r.Kind != rowLine || doc.FileOf(i) != file {
			continue
		}
		n := r.Line.NewNum
		if side == state.SideOld {
			if r.Line.Kind == diffparse.Added {
				continue
			}
			n = r.Line.OldNum
		} else if r.Line.Kind == diffparse.Removed {
			continue
		}
		if n == 0 {
			continue
		}
		if from == 0 || n < from {
			from = n
		}
		if n > to {
			to = n
		}
	}
	if from == 0 {
		return "", 0, 0, false
	}
	if to <= from {
		to = 0
	}
	return side, from, to, true
}
