package reef

import (
	"image"
	"image/color"
	"strings"
	"unicode/utf8"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// Setting type. Nothing here wraps: a run that does not fit is truncated with
// an ellipsis. Break prose with Wrap, measure it with Cell, and draw it a line
// at a time.
//
// Every helper takes a Run — one line in one style — and none of them draws
// past the width it was given, so two runs cannot overprint.

// Run is a single line of text in one style: a size from the scale, a weight
// and slant of the working face, and the colour it is set in.
type Run struct {
	Size   unit.Sp
	Weight font.Weight
	Style  font.Style
	Color  color.NRGBA
}

// Body returns a run of interface text: body size, regular weight.
func Body(c color.NRGBA) Run { return Run{Size: SizeUI, Weight: WeightBody, Color: c} }

// Strong returns a run of interface text at label weight, which is how a row
// says which of its columns identifies it.
func Strong(c color.NRGBA) Run { return Run{Size: SizeUI, Weight: WeightLabel, Color: c} }

// Code returns a run at the size and weight a source listing is set in.
func Code(c color.NRGBA) Run { return Run{Size: SizeCode, Weight: WeightBody, Color: c} }

// At returns the run in a different colour, so a caller can keep one style
// around and vary what it is set in.
func (r Run) At(c color.NRGBA) Run { r.Color = c; return r }

// Weighted returns the run at another weight.
func (r Run) Weighted(w font.Weight) Run { r.Weight = w; return r }

// Slanted returns the run in italic, which the system uses for an aside inside
// a body of text — a note against a line of code, an inferred value.
func (r Run) Slanted() Run { r.Style = font.Italic; return r }

// Text draws one line at the origin of the current coordinate space and
// returns its dimensions. It fills the width it is given and truncates with an
// ellipsis; it never wraps.
func (t *Theme) Text(gtx layout.Context, r Run, txt string) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	paint.ColorOp{Color: r.Color}.Add(gtx.Ops)
	material := macro.Stop()
	return widget.Label{MaxLines: 1, Truncator: "…"}.Layout(
		gtx, t.Shaper, t.Font(r.Weight, r.Style), r.Size, txt, material)
}

// TextAt draws one line starting at x, vertically centred in a band of the
// given height, clipped to stop before limit, and returns how wide it was.
// This is the workhorse: a row of an interface is a sequence of TextAt calls
// whose returned widths advance x.
//
//	x += th.TextAt(gtx, reef.Strong(th.P.Fg), x, row, right, name)
//	x += th.TextAt(gtx, reef.Body(th.P.Muted), x+gap, row, right, detail)
func (t *Theme) TextAt(gtx layout.Context, r Run, x, height, limit int, txt string) int {
	if x >= limit {
		return 0
	}
	cell := t.Cell(gtx, r.Size, false)
	off := op.Offset(image.Pt(x, (height-cell.Y)/2)).Push(gtx.Ops)
	sub := gtx
	sub.Constraints.Min = image.Point{}
	sub.Constraints.Max = image.Pt(max(0, limit-x), cell.Y)
	d := t.Text(sub, r, txt)
	off.Pop()
	return d.Size.X
}

// TextRight draws one line ending at rightX, vertically centred in a band of
// the given height, and returns its width. Lay a row's right-hand side out
// first, subtracting each width from rightX as you go, and give what is left
// to the run that may be truncated.
func (t *Theme) TextRight(gtx layout.Context, r Run, rightX, height int, txt string) int {
	if txt == "" {
		return 0
	}
	cell := t.Cell(gtx, r.Size, false)
	macro := op.Record(gtx.Ops)
	sub := gtx
	sub.Constraints.Min = image.Point{}
	sub.Constraints.Max = image.Pt(max(0, rightX), cell.Y)
	d := t.Text(sub, r, txt)
	call := macro.Stop()

	off := op.Offset(image.Pt(rightX-d.Size.X, (height-cell.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	return d.Size.X
}

// TextCentred draws one short run centred both ways in a box: a glyph in a
// rail, a letter in a marker, a count in a stub.
func (t *Theme) TextCentred(gtx layout.Context, r Run, box image.Rectangle, txt string) {
	macro := op.Record(gtx.Ops)
	sub := gtx
	sub.Constraints.Min = image.Point{}
	sub.Constraints.Max = box.Size()
	d := t.Text(sub, r, txt)
	call := macro.Stop()

	off := op.Offset(image.Pt(
		box.Min.X+(box.Dx()-d.Size.X)/2,
		box.Min.Y+(box.Dy()-d.Size.Y)/2,
	)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
}

// Label draws a label or a table heading: 11px medium, capitals, tracked wide.
// Pass the text in capitals; content keeps sentence case.
//
// Tracking is applied by drawing character by character, which is exact on a
// monospace grid and costs nothing at these lengths.
func (t *Theme) Label(gtx layout.Context, c color.NRGBA, txt string) layout.Dimensions {
	return t.tracked(gtx, SizeLabel, WeightLabel, c, txt, TrackCaps)
}

// LabelAt draws a label at x, vertically centred in a band of the given
// height, and returns its width.
func (t *Theme) LabelAt(gtx layout.Context, c color.NRGBA, x, height, limit int, txt string) int {
	if x >= limit {
		return 0
	}
	lh := t.CellWeight(gtx, SizeLabel, WeightLabel).Y
	off := op.Offset(image.Pt(x, (height-lh)/2)).Push(gtx.Ops)
	sub := gtx
	sub.Constraints.Min = image.Point{}
	sub.Constraints.Max.X = max(0, limit-x)
	d := t.Label(sub, c, txt)
	off.Pop()
	return d.Size.X
}

// LabelRight draws a label ending at rightX, vertically centred in a band of
// the given height, and returns its width.
func (t *Theme) LabelRight(gtx layout.Context, c color.NRGBA, rightX, height int, txt string) int {
	lh := t.CellWeight(gtx, SizeLabel, WeightLabel).Y
	macro := op.Record(gtx.Ops)
	sub := gtx
	sub.Constraints.Min = image.Point{}
	sub.Constraints.Max.X = max(0, rightX)
	d := t.Label(sub, c, txt)
	call := macro.Stop()
	off := op.Offset(image.Pt(rightX-d.Size.X, (height-lh)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	return d.Size.X
}

// Micro is Label one step down, for the places where a count or a unit sits
// beside something already labelled.
func (t *Theme) Micro(gtx layout.Context, c color.NRGBA, txt string) layout.Dimensions {
	return t.tracked(gtx, SizeMicro, WeightLabel, c, txt, TrackCaps)
}

// Wordmark draws a name in the display face, which is used nowhere else.
//
// The scale's display tracking is not applied: the condensed face is
// proportional, and negative tracking on a proportional face means shaping the
// string glyph by glyph and losing kerning, which costs more than the third of
// a pixel it would buy at this size.
func (t *Theme) Wordmark(gtx layout.Context, size unit.Sp, c color.NRGBA, txt string) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	material := macro.Stop()
	return widget.Label{MaxLines: 1}.Layout(
		gtx, t.Shaper, t.DisplayFont(WeightDisplay), size, txt, material)
}

// tracked draws a run letter by letter with tracking between the letters.
func (t *Theme) tracked(gtx layout.Context, size unit.Sp, weight font.Weight, c color.NRGBA, txt string, em float32) layout.Dimensions {
	cell := t.CellWeight(gtx, size, weight)
	tracking := int(float32(gtx.Sp(size))*em + 0.5)
	advance := cell.X + tracking

	macro := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	material := macro.Stop()

	// A tracked run is drawn glyph by glyph, so it has to respect the width it
	// was given itself: two runs of text never overprint in this system. No
	// width at all means no room — a caller that has run out of space passes
	// zero, and the run has to disappear rather than print over what is there.
	limit := gtx.Constraints.Max.X
	x := 0
	for _, r := range txt {
		if x+cell.X > limit {
			break
		}
		off := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		sub := gtx
		sub.Constraints.Min = image.Point{}
		sub.Constraints.Max.X = cell.X * 2
		widget.Label{MaxLines: 1}.Layout(sub, t.Shaper, t.Font(weight, font.Regular), size, string(r), material)
		off.Pop()
		x += advance
	}
	return layout.Dimensions{Size: image.Pt(max(0, x-tracking), cell.Y)}
}

// Wrap breaks a string into lines of at most cols characters, on spaces where
// it can and mid-word where it must. Text does not wrap in this system, so
// wrapping is the caller's business: measure a column in cells, wrap to it,
// and draw the lines on the rhythm of TextRow.
//
// A blank line in the input stays a blank line in the output, and so does an
// empty paragraph at the end of one, so the result can be counted to work out
// how tall a block of text will be before drawing it.
func Wrap(s string, cols int) []string {
	if cols <= 0 {
		return []string{""}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			switch {
			case line == "":
				line = word
			case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= cols:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
			// A word longer than the column is cut rather than allowed to
			// print past the edge. The cut is by character, not by byte: a
			// column is a count of cells on the grid.
			for utf8.RuneCountInString(line) > cols {
				runes := []rune(line)
				out = append(out, string(runes[:cols]))
				line = string(runes[cols:])
			}
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}
