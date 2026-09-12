package ui

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"

	"github.com/chromafish/check/internal/vcs"

	"github.com/chromafish/check/reef"
)

// layoutFiles draws the manifest: every path the diff touches, with its size
// and whether it has been read.
func (a *App) layoutFiles(gtx layout.Context) {
	if len(a.files) == 0 {
		if a.busy > 0 {
			a.placeholder(gtx, "LOADING")
		} else {
			a.placeholder(gtx, "NO CHANGES")
		}
		return
	}
	size := gtx.Constraints.Max
	row := a.ui.Row(gtx)

	a.fileList.Layout(gtx, len(a.files), func(gtx layout.Context, i int) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(size.X, row))
		a.fileRow(gtx, i)
		return layout.Dimensions{Size: image.Pt(size.X, row)}
	})
}

func (a *App) fileRow(gtx layout.Context, i int) {
	ui := a.ui
	size := gtx.Constraints.Max
	f := a.files[i]
	selected := i == a.fileSel

	fg, muted := ui.P.Fg, ui.P.Muted
	focused := selected && a.focus == PaneFiles
	// Selection is a moss tint plus a two pixel edge, never an inversion: the
	// row keeps its own colours, so the status letter and the counts still say
	// what they said, and the edge carries the state in monochrome. A pane
	// without the keyboard says so with the neutral pair.
	switch {
	case selected:
		ui.Selection(gtx, size, focused)
	case a.hovered(fileTag{i, false}) || a.hovered(fileTag{i, true}):
		reef.Fill(gtx, size, ui.P.Hover)
	}
	cell := ui.Cell(gtx, reef.SizeUI, false)
	x := gtx.Dp(reef.PadInline)

	// The read/unread box is its own control, so the row's click area is
	// carved around it rather than sitting underneath and firing too. Its
	// position is recorded for hit testing rather than recomputed elsewhere.
	boxX := x + cell.X + gtx.Dp(reef.Sp2)
	boxW := cell.X * 3
	a.viewedBoxX, a.viewedBoxW = boxX, boxW
	selectRow := func() {
		a.focus = PaneFiles
		a.selectFile(i)
	}
	a.clickArea(gtx, image.Rect(0, 0, boxX, size.Y), fileTag{i, false}, selectRow)
	a.clickArea(gtx, image.Rect(boxX+boxW, 0, size.X, size.Y), fileTag{i, true}, selectRow)

	a.cellText(gtx, x, size.Y, size.X, reef.WeightLabel, a.statusColor(f.Status), f.Status.String())
	x += cell.X + gtx.Dp(reef.Sp3)

	a.viewedBox(gtx, image.Rect(boxX, 0, boxX+boxW, size.Y), i, f, false)
	x = boxX + boxW + gtx.Dp(reef.Sp2)

	// The right hand side carries the counts and comment badge; the path gets
	// whatever is left, elided from the front so the file name stays visible.
	rightX := size.X - gtx.Dp(reef.PadInline)
	if f.Comments > 0 {
		badge := fmt.Sprintf("%d", f.Comments)
		c := ui.P.Action
		if f.Open == 0 {
			c = muted
		}
		rightX -= a.cellTextRight(gtx, rightX, size.Y, reef.WeightLabel, c, "*"+badge) + gtx.Dp(reef.Sp4)
	}
	if f.Added > 0 || f.Removed > 0 {
		counts := fmt.Sprintf("+%d −%d", f.Added, f.Removed)
		rightX -= a.cellTextRight(gtx, rightX, size.Y, font.Normal, muted, counts) + gtx.Dp(reef.Sp4)
	}

	// A file already read is dimmed, so what is left to do is what stands out.
	// It is dimmed and never hidden: nothing in this system collapses.
	pathColor := fg
	if f.Viewed && !f.Stale {
		pathColor = ui.P.Faint
	}
	avail := (rightX - x) / cell.X
	a.cellText(gtx, x, size.Y, rightX, font.Normal, pathColor, elideLeft(f.Display(), avail))

	reef.HLine(gtx, size.X, size.Y-1, ui.P.RuleFaint)
}

// statusColor is the colour a diff sets a status letter in. A rename is
// neither an addition nor a removal, so it takes the advisory teal rather than
// borrowing one of them.
func (a *App) statusColor(s vcs.Status) reef.ColorNRGBA {
	switch s {
	case vcs.Added:
		return a.ui.P.AddFg
	case vcs.Deleted:
		return a.ui.P.DelFg
	case vcs.Renamed, vcs.Copied:
		return a.ui.P.Info
	}
	return a.ui.P.Fg
}

// fileTag identifies one half of a manifest row's click area.
type fileTag struct {
	row   int
	right bool
}

// viewedBox draws the read/unread mark and makes it a control: a filled square
// for read, hollow for unread, and the accent colour when the file has been
// rewritten since it was read. Clicking it toggles, which is the only way to
// do it without knowing the keyboard.
func (a *App) viewedBox(gtx layout.Context, r image.Rectangle, i int, f FileRow, head bool) {
	ui := a.ui
	tag := viewedTag{i, head}

	// The colour says which of the three states it is in, and the shape says
	// it again: filled for read, hollow for unread, and filled with a bite out
	// of it for read at a version that has since been rewritten.
	c := ui.P.Muted
	switch {
	case f.Viewed && f.Stale:
		c = ui.P.Warn
	case f.Viewed:
		c = ui.P.Ok
	}

	ui.Box(gtx, r, tag, f.Viewed, c, func() {
		a.after(func() {
			a.focus = PaneFiles
			a.selectFile(i)
			a.setViewed(i, !f.Viewed || f.Stale)
		})
	})
	if f.Viewed && f.Stale {
		reef.FillRect(gtx, reef.BoxRect(gtx, r).Inset(gtx.Dp(reef.Sp3)/2), ui.P.Bg)
	}
}

// elideLeft shortens a path from the front, which keeps the file name and its
// immediate directory, the parts that identify it.
func elideLeft(s string, cols int) string {
	if cols <= 1 || len([]rune(s)) <= cols {
		return s
	}
	runes := []rune(s)
	tail := string(runes[len(runes)-(cols-1):])
	if i := strings.IndexByte(tail, '/'); i >= 0 && i < len(tail)-1 {
		tail = tail[i+1:]
	}
	return "…" + tail
}

// selectFile moves to one entry of the manifest and takes the diff with it.
// The whole change is already printed, so this is a scroll rather than a load:
// the file you were reading is still above you and the next one still below.
func (a *App) selectFile(i int) {
	if i < 0 || i >= len(a.files) {
		return
	}
	a.fileSel = i
	a.showFile(i)
	a.lightFile(i)
}

// showFile scrolls the diff to a file's heading and puts the cursor on it.
func (a *App) showFile(i int) {
	doc := a.diff
	if doc == nil || i < 0 || i >= len(doc.FileRows) {
		return
	}
	row := doc.FileRows[i]
	doc.Cursor = row
	a.diffList.Position.First = row
	a.diffList.Position.Offset = 0
	a.pinFile = i
}

// syncFileSel points the manifest at whichever file the diff has scrolled to.
// Reading down the change is the same gesture as walking the manifest, so the
// two are never out of step.
func (a *App) syncFileSel() {
	doc := a.diff
	if doc == nil || len(doc.Rows) == 0 || len(a.files) == 0 {
		return
	}
	first := clamp(a.diffList.Position.First, 0, len(doc.Rows)-1)

	// A jump made from the manifest wins the frame it happens in. Only real
	// scrolling — the position moving on its own — drags the selection with
	// it, which also means a change short enough to fit on screen never snaps
	// the selection back to its first file.
	if a.pinFile >= 0 {
		a.lastFirst = first
		a.pinFile = -1
		return
	}
	if first == a.lastFirst {
		return
	}
	a.lastFirst = first

	i := doc.FileOf(first)
	if i < 0 || i >= len(a.files) || i == a.fileSel {
		return
	}
	a.fileSel = i
	a.scrollList(&a.fileList, i)
	a.lightFile(i)
}
