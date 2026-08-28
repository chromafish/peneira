package ui

import (
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"

	"github.com/chromafish/peneira/internal/vcs"

	"github.com/chromafish/peneira/reef"
)

// layoutRevs draws the revision list with its ancestry graph.
func (a *App) layoutRevs(gtx layout.Context) {
	ui := a.ui
	size := gtx.Constraints.Max
	if len(a.revs) == 0 {
		a.placeholder(gtx, "NO REVISIONS")
		return
	}
	row := ui.Row(gtx)
	cell := ui.Cell(gtx, reef.SizeUI, false)

	// One graph column width for the whole list, so change IDs line up no
	// matter how many lanes a particular row happens to use.
	lanes := 1
	for _, rv := range a.revs {
		lanes = max(lanes, rv.Graph.Width)
	}
	a.graphLanes = min(lanes, maxLanes)

	a.revList.Layout(gtx, len(a.revs), func(gtx layout.Context, i int) layout.Dimensions {
		rv := a.revs[i]
		gtx.Constraints = layout.Exact(image.Pt(size.X, row))
		a.revRow(gtx, i, rv, cell)
		return layout.Dimensions{Size: image.Pt(size.X, row)}
	})
}

func (a *App) revRow(gtx layout.Context, i int, rv vcs.Revision, cell image.Point) {
	ui := a.ui
	size := gtx.Constraints.Max
	selected := i == a.revSel

	fg, muted := ui.P.Fg, ui.P.Muted
	switch {
	case selected:
		ui.Selection(gtx, size, a.focus == PaneRevs)
	case a.hovered(&a.revs[i]):
		reef.Fill(gtx, size, ui.P.Hover)
	}
	a.clickable(gtx, size, &a.revs[i], func() {
		a.focus = PaneRevs
		a.selectRev(i)
	})

	laneW := cell.X + gtx.Dp(reef.Sp1)
	a.drawGraph(gtx, rv, laneW, size.Y, selected)

	x := graphInset + laneW*a.graphLanes + gtx.Dp(reef.Sp4)
	pad := gtx.Dp(reef.PadInline)

	// The change ID is what a jj user actually types, so it leads the row.
	x += a.cellText(gtx, x, size.Y, size.X, reef.WeightLabel, fg, rv.ChangeID) + cell.X

	// A compact marker column: whether this is the working copy, whether it is
	// immutable, whether it conflicts. A conflict is a danger state, not a
	// highlight, and takes the rust the system reserves for one.
	marker, markerColor := "", muted
	switch {
	case rv.Conflict:
		marker, markerColor = "!", ui.P.Error
	case rv.WorkingCopy:
		marker, markerColor = "@", ui.P.Action
	case rv.Immutable:
		marker = "="
	case rv.Empty:
		marker = "·"
	}
	if marker != "" {
		a.cellText(gtx, x, size.Y, size.X, reef.WeightLabel, markerColor, marker)
	}
	x += cell.X + gtx.Dp(reef.Sp1)

	// Bookmarks are the names a person navigates by, so they come before the
	// description and keep the olive accent even when the row is selected: the
	// system spends its other families on status.
	if names := append(append([]string{}, rv.Bookmarks...), rv.RemoteBookmarks...); len(names) > 0 {
		label := strings.Join(names, " ")
		x += a.cellText(gtx, x, size.Y, size.X, reef.WeightLabel, ui.P.Accent, label) + cell.X
	}

	desc, descColor := rv.Subject(), fg
	if rv.Description == "" {
		descColor = muted
	}
	if x < size.X-pad {
		a.cellText(gtx, x, size.Y, size.X-pad, font.Normal, descColor, desc)
	}

	reef.HLine(gtx, size.X, size.Y-1, ui.P.RuleFaint)
}

// drawGraph renders one row of the ancestry graph as hairlines and a node,
// using right-angled elbows rather than diagonals to match the rest of the
// interface.
func (a *App) drawGraph(gtx layout.Context, rv vcs.Revision, laneW, height int, selected bool) {
	ui := a.ui
	c := ui.P.Muted
	nodeColor := ui.P.Fg
	if selected {
		nodeColor = ui.P.Strong
	}

	lane := func(col int) int { return graphInset + col*laneW + laneW/2 }
	mid := height / 2
	nodeX := lane(rv.Graph.Column)

	for _, col := range rv.Graph.Through {
		x := lane(col)
		reef.FillRect(gtx, image.Rect(x, 0, x+1, height), c)
	}
	// Lines arriving from children above bend into the node.
	for _, col := range rv.Graph.In {
		x := lane(col)
		reef.FillRect(gtx, image.Rect(x, 0, x+1, mid), c)
		reef.FillRect(gtx, image.Rect(min(x, nodeX), mid, max(x, nodeX)+1, mid+1), c)
	}
	// Lines leaving towards parents below.
	for _, col := range rv.Graph.Out {
		x := lane(col)
		reef.FillRect(gtx, image.Rect(x, mid, x+1, height), c)
		if x != nodeX {
			reef.FillRect(gtx, image.Rect(min(x, nodeX), mid, max(x, nodeX)+1, mid+1), c)
		}
	}

	// The node itself: filled for an ordinary commit, hollow for one that
	// cannot be edited or has no content, ringed for the working copy.
	n := gtx.Dp(6)
	half := n / 2
	box := image.Rect(nodeX-half, mid-half, nodeX-half+n, mid-half+n)
	switch {
	case rv.WorkingCopy:
		reef.FillRect(gtx, box, nodeColor)
		reef.Stroke(gtx, box.Inset(-gtx.Dp(3)), nodeColor)
	case rv.Immutable, rv.Root, rv.Empty:
		reef.Stroke(gtx, box, nodeColor)
	default:
		reef.FillRect(gtx, box, nodeColor)
	}
}

const (
	// maxLanes caps how much of the revision pane the graph may take before
	// extra branches are simply left undrawn.
	maxLanes = 6
	// graphInset keeps the leftmost lane clear of the pane edge, so the ring
	// around the working copy is not clipped.
	graphInset = 7
)
