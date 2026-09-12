package reef_test

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/chromafish/check/reef"
)

// A whole application in the shape reef expects: one UI carried on the
// application struct, one Layout that draws a frame, and controls identified
// by tags that stay the same between frames.
type files struct {
	ui   *reef.UI
	rows []string
	sel  int
	read map[int]bool
}

// rowTag identifies one row of the list to the pointer. A value type is all it
// takes: it has to be comparable and the same from frame to frame.
type rowTag struct{ row int }

func (f *files) Layout(gtx layout.Context) layout.Dimensions {
	// The chosen body size, applied once, at the top.
	gtx = f.ui.Sized(gtx)
	reef.Fill(gtx, gtx.Constraints.Max, f.ui.P.Bg)

	f.ui.Panel(gtx, reef.Panel{
		Title:   "MANIFEST",
		Focused: true,
		Body:    f.list,
	})
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// list draws the rows on the pitch the system fixes for a list, so this list
// and every other one in the application print on the same rhythm.
func (f *files) list(gtx layout.Context) {
	row := f.ui.Row(gtx)
	width := gtx.Constraints.Max.X
	cell := f.ui.Cell(gtx, reef.SizeUI, false)

	for i, name := range f.rows {
		off := op.Offset(image.Pt(0, i*row)).Push(gtx.Ops)
		sub := gtx
		sub.Constraints = layout.Exact(image.Pt(width, row))
		f.row(sub, i, name, cell.X)
		off.Pop()
	}
}

func (f *files) row(gtx layout.Context, i int, name string, cellW int) {
	size := gtx.Constraints.Max
	var tag event.Tag = rowTag{i}

	switch {
	case i == f.sel:
		f.ui.Selection(gtx, size, true)
	case f.ui.Hovered(tag):
		f.ui.HoverFill(gtx, size)
	}
	f.ui.Click(gtx, size, tag, func() { f.sel = i })

	// The read/unread square is a control of its own, so clicking it marks the
	// file without moving the selection.
	pad := gtx.Dp(reef.PadInline)
	box := image.Rect(pad, 0, pad+cellW*3, size.Y)
	mark := f.ui.P.Muted
	if f.read[i] {
		mark = f.ui.P.Ok
	}
	f.ui.Box(gtx, box, boxTag{i}, f.read[i], mark, func() { f.read[i] = !f.read[i] })

	// A file already read is dimmed: what is left to do is what stands out.
	ink := f.ui.P.Fg
	if f.read[i] {
		ink = f.ui.P.Faint
	}
	f.ui.TextAt(gtx, reef.Run{Size: reef.SizeUI, Weight: font.Normal, Color: ink},
		box.Max.X+gtx.Dp(reef.Sp2), size.Y, size.X-pad, name)

	reef.HLine(gtx, size.X, size.Y-1, f.ui.P.RuleFaint)
}

type boxTag struct{ row int }

// The design system is one value on the application struct. Everything the
// interface draws goes through it, and the whole thing inverts by asking it
// for the other palette.
func Example() {
	f := &files{
		ui:   reef.New(),
		rows: []string{"internal/ui/app.go", "internal/ui/diffpane.go"},
		read: map[int]bool{},
	}
	f.ui.SetSize(15)  // the interface size the whole type scale is anchored on
	f.ui.ToggleDark() // and the sheet at night

	// In an application this context comes from app.NewContext on a frame
	// event; built by hand, the same layout draws into an operation list with
	// no window at all, which is how the interface is tested.
	var ops op.Ops
	f.Layout(layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(600, 400)),
	})
}
