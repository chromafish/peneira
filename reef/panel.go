package reef

import (
	"image"
	"strconv"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
)

// A panel is a strip of the sunken surface with a heading in tracked
// capitals, a rule under it, and a body below. The rule does the separating: no gaps, no
// shadows, no floating cards.

// Panel is a titled column or region.
//
//	reef.Panel{
//		Title:   "MANIFEST · 4/8 READ",
//		Focused: a.focus == paneFiles,
//		Body:    a.layoutFiles,
//	}.Layout(gtx, a.ui.Theme)
//
// The title is a heading: pass it in capitals, and put the state of the panel
// in it rather than beside it — a count of how much of it is done says more
// than a progress bar and costs no space.
type Panel struct {
	// Title is drawn as a label at the left of the header.
	Title string

	// Focused marks the panel that has the keyboard. A focused panel is
	// marked by the accent rule under its header — two pixels, the same edge a
	// selected row wears — rather than by a glow or a border, so the grid
	// stays intact.
	Focused bool

	// Reserve is space, in pixels, kept clear at the right of the header for
	// controls drawn by Header. The title stops short of it rather than
	// printing under it.
	Reserve int

	// Header, if set, is called with the header strip's own space, to draw
	// controls into it. It runs after the title and before the rule.
	Header func(gtx layout.Context)

	// Body is called with everything below the header.
	Body func(gtx layout.Context)
}

// Layout draws the panel into the space it is given and returns the height of
// its header, which is where the body starts.
func (p Panel) Layout(gtx layout.Context, t *Theme) int {
	size := gtx.Constraints.Max
	headH := t.StripHeight(gtx)

	// A panel header is a table heading: the sunken surface, an uppercase
	// label, and a rule under it doing the separating.
	FillRect(gtx, image.Rect(0, 0, size.X, headH), t.P.BgSunken)

	labelColor := t.P.Muted
	if p.Focused {
		labelColor = t.P.Action
	}
	pad := gtx.Dp(PadInline)
	t.LabelAt(gtx, labelColor, pad, headH, max(0, size.X-pad-p.Reserve), p.Title)

	if p.Header != nil {
		sub := gtx
		sub.Constraints = layout.Exact(image.Pt(size.X, headH))
		p.Header(sub)
	}

	if p.Focused {
		FillRect(gtx, image.Rect(0, headH-gtx.Dp(BorderThick), size.X, headH), t.P.RuleAccent)
	} else {
		HLine(gtx, size.X, headH-1, t.P.Rule)
	}

	if p.Body != nil && size.Y > headH+1 {
		off := op.Offset(image.Pt(0, headH+1)).Push(gtx.Ops)
		sub := gtx
		sub.Constraints = layout.Exact(image.Pt(size.X, size.Y-headH-1))
		p.Body(sub)
		off.Pop()
	}
	return headH
}

// Panel draws a panel through the UI, which is the same as calling
// Panel.Layout with the theme and reads better where everything else in the
// frame is being drawn through the UI as well.
func (u *UI) Panel(gtx layout.Context, p Panel) int { return p.Layout(gtx, u.Theme) }

// Collapse draws the control that puts a panel away: a chevron at the right of
// its header, in a square the height of the strip. Nothing in this system
// disappears silently, and nothing is put away by a shortcut alone.
func (u *UI) Collapse(gtx layout.Context, width, height int, tag event.Tag, onHide func()) {
	box := image.Rect(width-height, 0, width, height)
	if u.Hovered(tag) {
		FillRect(gtx, box, u.P.Hover)
	}
	u.TextCentred(gtx, Strong(u.P.Muted), box, "<")
	u.Area(gtx, box, tag, pointer.CursorPointer, onHide)
}

// Rail draws a column that has been put away: a chevron to bring it back, the
// letter it is filed under, and how many rows are waiting in it. It fills the
// space it is given, which should be RailW wide.
func (u *UI) Rail(gtx layout.Context, tag event.Tag, letter string, count int, onShow func()) {
	size := gtx.Constraints.Max
	headH := gtx.Dp(ControlH)

	FillRect(gtx, image.Rect(0, headH, size.X, size.Y), u.P.BgAlt)
	if u.Hovered(tag) {
		FillRect(gtx, image.Rect(0, headH, size.X, size.Y), u.P.Hover)
	}
	FillRect(gtx, image.Rect(0, 0, size.X, headH), u.P.BgSunken)
	HLine(gtx, size.X, headH-1, u.P.Rule)

	u.TextCentred(gtx, Strong(u.P.Action), image.Rect(0, 0, size.X, headH), ">")

	row := gtx.Dp(GapRow)
	u.TextCentred(gtx, Strong(u.P.Muted), image.Rect(0, headH, size.X, headH+row), letter)
	if count > 0 {
		u.TextCentred(gtx, Strong(u.P.Faint), image.Rect(0, headH+row, size.X, headH+row*2), strconv.Itoa(count))
	}

	u.Area(gtx, image.Rect(0, 0, size.X, size.Y), tag, pointer.CursorPointer, onShow)
}

// Masthead paints the top strip of a window: the page colour and the 2px rule
// under it. Draw the contents into the same space afterwards; see
// Theme.MastheadHeight.
func (t *Theme) Masthead(gtx layout.Context, size image.Point) {
	Fill(gtx, size, t.P.Bg)
	FillRect(gtx, image.Rect(0, size.Y-gtx.Dp(BorderThick), size.X, size.Y), t.P.Rule)
}

// StatusBar paints the bottom strip: the raised surface with a hairline over
// it.
func (t *Theme) StatusBar(gtx layout.Context, size image.Point) {
	Fill(gtx, size, t.P.BgAlt)
	HLine(gtx, size.X, 0, t.P.Rule)
}

// Placeholder fills an empty region with the dot grid and one line of label
// capitals.
func (t *Theme) Placeholder(gtx layout.Context, txt string) {
	size := gtx.Constraints.Max
	t.DotGrid(gtx, size)
	pad := gtx.Dp(PadInline)
	off := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
	sub := gtx
	sub.Constraints.Min = image.Point{}
	sub.Constraints.Max.X = max(0, size.X-pad*2)
	t.Label(sub, t.P.Faint, txt)
	off.Pop()
}
