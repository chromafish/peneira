package reef

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
)

// A control is a hairline rectangle with a label in tracked capitals and its
// keyboard shortcut printed on it. No radius, no fill until the pointer is on
// it, no motion.
//
//	x += ui.Control(gtx, x, height, tagSave, "SAVE  ⌘S", ui.P.Action, app.save)

// Control draws a control with its left edge at x, vertically centred in a
// band of the given height, and returns its width. Advance x by the return
// value plus a gap from the spacing scale to put the next one beside it.
func (u *UI) Control(gtx layout.Context, x, height int, tag event.Tag, label string, c color.NRGBA, onClick func()) int {
	w, draw := u.MeasureControl(gtx, tag, label, c)
	draw(gtx, x, height, onClick)
	return w
}

// ControlRight is Control anchored to its right edge instead, for the controls
// that live at the end of a strip. Subtract the return value from rightX to
// put the next one to its left.
func (u *UI) ControlRight(gtx layout.Context, rightX, height int, tag event.Tag, label string, c color.NRGBA, onClick func()) int {
	w, draw := u.MeasureControl(gtx, tag, label, c)
	draw(gtx, rightX-w, height, onClick)
	return w
}

// MeasureControl lays a control's label out once and returns its width along
// with a function that draws it at a position decided later. Use it when the
// rest of a row has to know how much room the control will take before it can
// be laid out — a title block whose fields must stop short of the control at
// the end of the line.
//
// The draw function may be called at most once, and only during the same
// frame: it replays operations recorded against that frame's context.
func (u *UI) MeasureControl(gtx layout.Context, tag event.Tag, label string, c color.NRGBA) (int, func(gtx layout.Context, x, height int, onClick func())) {
	padX := gtx.Dp(PadInline)

	// Under the pointer the control fills in and the label takes the colour
	// that reads on a filled action.
	fg := c
	if u.Hovered(tag) {
		fg = u.P.ActionFg
	}
	macro := op.Record(gtx.Ops)
	sub := gtx
	sub.Constraints.Min = image.Point{}
	sub.Constraints.Max.X = 1 << 20
	d := u.Label(sub, fg, label)
	call := macro.Stop()

	w := d.Size.X + padX*2
	h := max(gtx.Dp(ControlHSm), d.Size.Y)

	return w, func(gtx layout.Context, x, height int, onClick func()) {
		y := (height - h) / 2
		box := image.Rect(x, y, x+w, y+h)
		if u.Hovered(tag) {
			FillRect(gtx, box, c)
		} else {
			Stroke(gtx, box, c)
		}
		off := op.Offset(image.Pt(x+padX, y+(h-d.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		off.Pop()
		u.Area(gtx, box, tag, pointer.CursorPointer, onClick)
	}
}

// Box draws a checkbox: a small square, filled when set and hollow when not,
// with a ring round it under the pointer. It carries no label — the row it
// sits in is the label — and it is a control of its own, so clicking it
// toggles without moving the selection. The colour is the caller's.
func (u *UI) Box(gtx layout.Context, within image.Rectangle, tag event.Tag, set bool, c color.NRGBA, onClick func()) {
	box := BoxRect(gtx, within)
	if set {
		FillRect(gtx, box, c)
	} else {
		Stroke(gtx, box, c)
	}
	if u.Hovered(tag) {
		Stroke(gtx, box.Inset(-gtx.Dp(Sp3)/2), u.P.Action)
	}
	u.Area(gtx, within, tag, pointer.CursorPointer, onClick)
}

// BoxRect returns the square Box will draw inside a region, for a caller that
// wants to mark it further — a filled square with a bite taken out of it, say,
// for a thing that was true and has since gone stale.
func BoxRect(gtx layout.Context, within image.Rectangle) image.Rectangle {
	n := gtx.Dp(Sp5) - gtx.Dp(Sp1)
	return image.Rect(0, 0, n, n).Add(image.Pt(
		within.Min.X+(within.Dx()-n)/2,
		within.Min.Y+(within.Dy()-n)/2,
	))
}

// Selection paints the background of a row in a list: a tint plus the 2px edge
// down its leading side. Pass focused false for a pane that does not have the
// keyboard, which uses the neutral pair. The text is left alone, so a status
// letter or a count keeps its own colour.
func (t *Theme) Selection(gtx layout.Context, size image.Point, focused bool) {
	bg, edge := t.P.SelIdleBg, t.P.SelIdleEdge
	if focused {
		bg, edge = t.P.SelBg, t.P.SelEdge
	}
	Fill(gtx, size, bg)
	Edge(gtx, size.Y, edge)
}

// SelectionRect is Selection at an explicit position, for a row drawn inside a
// sheet.
func (t *Theme) SelectionRect(gtx layout.Context, r image.Rectangle, focused bool) {
	bg, edge := t.P.SelIdleBg, t.P.SelIdleEdge
	if focused {
		bg, edge = t.P.SelBg, t.P.SelEdge
	}
	FillRect(gtx, r, bg)
	FillRect(gtx, image.Rect(r.Min.X, r.Min.Y, r.Min.X+gtx.Dp(BorderThick), r.Max.Y), edge)
}

// HoverFill paints the tint a row takes under the pointer: one paper step
// darker, and nothing else.
func (t *Theme) HoverFill(gtx layout.Context, size image.Point) {
	Fill(gtx, size, t.P.Hover)
}
