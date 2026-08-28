package reef

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// The primitives: filled rectangles and hairlines. No radius, no gradient, no
// blur, no shadow.

// Fill paints a solid rectangle at the origin of the current coordinate space.
// Pass gtx.Constraints.Max to fill whatever a widget was given.
func Fill(gtx layout.Context, size image.Point, c color.NRGBA) {
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

// FillRect paints a solid rectangle at an explicit position.
func FillRect(gtx layout.Context, r image.Rectangle, c color.NRGBA) {
	defer op.Offset(r.Min).Push(gtx.Ops).Pop()
	Fill(gtx, r.Size(), c)
}

// HLine draws a hairline across width at y. Structure is drawn with 1px rules
// and negative space, never with shadows or gradients.
func HLine(gtx layout.Context, width, y int, c color.NRGBA) {
	FillRect(gtx, image.Rect(0, y, width, y+1), c)
}

// VLine draws a hairline down height at x.
func VLine(gtx layout.Context, x, height int, c color.NRGBA) {
	FillRect(gtx, image.Rect(x, 0, x+1, height), c)
}

// Edge draws the 2px accent bar the system puts down the leading edge of a
// selected row, a changed line, and anything else whose state has to survive
// being seen out of the corner of an eye — or printed in monochrome.
func Edge(gtx layout.Context, height int, c color.NRGBA) {
	FillRect(gtx, image.Rect(0, 0, gtx.Dp(BorderThick), height), c)
}

// Outline strokes a rectangle at a given width in device pixels, inside its
// own bounds. Use gtx.Dp(BorderHair) for the hairline the system draws
// controls and floating things with.
func Outline(gtx layout.Context, r image.Rectangle, w int, c color.NRGBA) {
	FillRect(gtx, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+w), c)
	FillRect(gtx, image.Rect(r.Min.X, r.Max.Y-w, r.Max.X, r.Max.Y), c)
	FillRect(gtx, image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y), c)
	FillRect(gtx, image.Rect(r.Max.X-w, r.Min.Y, r.Max.X, r.Max.Y), c)
}

// Stroke outlines a rectangle one device pixel wide, whatever the density.
// It is for the small marks — a node on a graph, a checkbox, a swatch — where
// a hairline that thickened with the display would stop being a hairline.
func Stroke(gtx layout.Context, r image.Rectangle, c color.NRGBA) {
	Outline(gtx, r, 1, c)
}

// FocusRing draws the 2px ring at 1px offset that marks keyboard focus. Do not
// suppress it.
func (t *Theme) FocusRing(gtx layout.Context, r image.Rectangle) {
	Outline(gtx, r.Inset(-gtx.Dp(BorderHair)), gtx.Dp(BorderThick), t.P.Focus)
}

// Sheet paints a floating panel: the raised surface and an ink hairline round
// it. Draw the contents inside r afterwards.
func (t *Theme) Sheet(gtx layout.Context, r image.Rectangle) {
	FillRect(gtx, r, t.P.BgAlt)
	Outline(gtx, r, gtx.Dp(BorderHair), t.P.RuleStrong)
}

// CentreRect returns a rectangle of the given size centred within an area,
// which is where a sheet goes.
func CentreRect(within image.Point, w, h int) image.Rectangle {
	x, y := (within.X-w)/2, (within.Y-h)/2
	return image.Rect(x, y, x+w, y+h)
}

// gridTile caches the dot grid. The grid is painted as a repeated tile rather
// than as one rectangle per dot, which keeps a full window desk at a few dozen
// draw calls instead of tens of thousands.
type gridTile struct {
	img  *image.NRGBA
	op   paint.ImageOp
	step int
	c    color.NRGBA
}

const gridTilePx = 256

// DotGrid fills a region with the 8px desk grid that goes under a sheet and
// behind an empty region. It is the one texture, and it means "nothing here".
func (t *Theme) DotGrid(gtx layout.Context, size image.Point) {
	step := max(1, gtx.Dp(GridDotStep))
	if t.grid.img == nil || t.grid.step != step || t.grid.c != t.P.GridDot {
		n := gridTilePx / step * step
		if n < step {
			n = step
		}
		img := image.NewNRGBA(image.Rect(0, 0, n, n))
		for y := 0; y < n; y += step {
			for x := 0; x < n; x += step {
				img.SetNRGBA(x, y, t.P.GridDot)
			}
		}
		t.grid = gridTile{img: img, op: paint.NewImageOp(img), step: step, c: t.P.GridDot}
	}

	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	n := t.grid.img.Bounds().Dx()
	for y := 0; y < size.Y; y += n {
		for x := 0; x < size.X; x += n {
			off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			t.grid.op.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			off.Pop()
		}
	}
}

// CornerTicks draws short registration marks in the corners of an area, the
// way a printed sheet carries them.
func (t *Theme) CornerTicks(gtx layout.Context, size image.Point) {
	n := gtx.Dp(Sp3)
	c := t.P.Rule
	corners := [][2]int{{0, 0}, {size.X - n, 0}, {0, size.Y - 1}, {size.X - n, size.Y - 1}}
	for i, p := range corners {
		x, y := p[0], p[1]
		FillRect(gtx, image.Rect(x, y, x+n, y+1), c)
		vy := y
		if i >= 2 {
			vy = y - n + 1
		}
		vx := x
		if i%2 == 1 {
			vx = x + n - 1
		}
		FillRect(gtx, image.Rect(vx, vy, vx+1, vy+n), c)
	}
}
