package reef

import (
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"golang.org/x/image/math/fixed"
)

// Theme is the design system as one value: the palette in force, the shaper
// the type is set with, the body size the scale is anchored on, and the
// measurements everything is laid out against. It is the argument every
// drawing helper in the package takes.
//
// A Theme is not safe for concurrent use — it caches measurements as it draws
// — and belongs to one window's event loop. Build one at startup, keep it, and
// change it only from the loop that draws with it.
type Theme struct {
	// Shaper lays out the text. It is exported for code that wants to measure
	// or draw a string the package has no helper for.
	Shaper *text.Shaper

	// FontName is the family the interface is set in, for a colophon.
	FontName string

	// Family is the system typeface chosen with SetFamily, or empty when the
	// interface is set in the face that ships in the binary.
	Family string

	// Size is the interface size the type scale is anchored on, and Scale that
	// size over the scale's own — the factor Sized multiplies point sizes by.
	Size  unit.Sp
	Scale float32

	// P is the semantic palette. Every colour an interface draws should come
	// from here. Swapping it swaps the theme; see SetPalette and ToggleDark.
	P Palette

	// Dark reports which of the two themes is in force.
	Dark bool

	scheme   Scheme
	fonts    Fonts
	mono     font.Typeface
	fallback font.Typeface
	cells    map[cellKey]image.Point
	grid     gridTile
}

// A cell is measured at a point size and a weight, in whatever pixels per
// point the frame is drawing at — which is where the chosen body size lands,
// so the key covers it without the scale having to appear in it.
type cellKey struct {
	size   unit.Sp
	sp     float32
	weight font.Weight
}

// NewTheme returns a theme set in the embedded faces, on the light palette.
// This is the one-line way in:
//
//	th := reef.NewTheme()
func NewTheme() *Theme { return NewThemeWith(LoadFonts()) }

// NewThemeWith returns a theme set in typefaces of your own. The collection is
// handed to the shaper as it stands, so it has to carry the weights the scale
// asks for; see Fonts.
func NewThemeWith(fonts Fonts) *Theme {
	return &Theme{
		Shaper:   text.NewShaper(text.WithCollection(fonts.Collection)),
		FontName: fonts.Name,
		Size:     SizeUI,
		Scale:    1,
		P:        Light,
		scheme:   Default(),
		fonts:    fonts,
		mono:     fonts.Mono,
		fallback: fonts.Mono,
		cells:    map[cellKey]image.Point{},
	}
}

// SetPalette puts one palette in force, leaving the scheme alone. Use it for a
// palette built by hand; SetScheme is the way to a named pair that can be
// inverted and remembered.
func (t *Theme) SetPalette(p Palette) {
	t.P = p
	t.grid = gridTile{}
}

// SetDark moves between the two modes of the scheme in force. A scheme that
// defines only one mode — an imported base16 scheme, say — stays as it is.
func (t *Theme) SetDark(dark bool) {
	p, ok := t.scheme.palette(dark)
	if !ok || !t.scheme.Has(dark) {
		return
	}
	t.Dark = dark
	t.SetPalette(p)
}

// ToggleDark inverts the palette, where the scheme has both modes.
func (t *Theme) ToggleDark() { t.SetDark(!t.Dark) }

// SetFamily sets the working typeface to a family installed on the machine, or
// back to the embedded one when the name is empty. MonoFamilies lists what
// there is to choose from.
//
// Nothing is loaded here. Gio resolves a typeface against the system fonts
// itself, and reads the name as a list of families in the manner of CSS, so
// naming the embedded face after the chosen one leaves it standing behind:
// anything the chosen family has no glyph for is still drawn.
func (t *Theme) SetFamily(name string) {
	t.Family = name
	if name == "" {
		t.mono, t.FontName = t.fallback, t.fonts.Name
	} else {
		t.mono, t.FontName = font.Typeface(name+", "+string(t.fallback)), name
	}
	clear(t.cells)
}

// SetSize anchors the type scale on an interface size in points, clamped to
// MinFontSize…MaxFontSize. Zero restores the scale's own SizeUI.
func (t *Theme) SetSize(size unit.Sp) {
	if size <= 0 {
		size = SizeUI
	}
	t.Size = min(max(size, MinFontSize), MaxFontSize)
	t.Scale = float32(t.Size) / float32(SizeUI)
	clear(t.cells)
}

// Sized returns the context an interface should draw in: the chosen body size
// applied to the frame's own point size. Call it once, at the top of the
// window's layout, and use what it returns from there down.
//
//	func (a *App) Layout(gtx layout.Context) layout.Dimensions {
//		gtx = a.th.Sized(gtx)
//		...
//	}
//
// Everything set in points — all of the type — then grows and shrinks with the
// interface size, while the chrome, which the system sets in device pixels, stays
// where it is. Rows laid out on the taller of the two follow the type up and
// stay put on the way down.
func (t *Theme) Sized(gtx layout.Context) layout.Context {
	gtx.Metric.PxPerSp *= t.Scale
	return gtx
}

// Font returns the working typeface at a weight and style.
func (t *Theme) Font(weight font.Weight, style font.Style) font.Font {
	return font.Font{Typeface: t.mono, Weight: weight, Style: style}
}

// DisplayFont returns the display face. Nothing read as data is set in it.
func (t *Theme) DisplayFont(weight font.Weight) font.Font {
	return font.Font{Typeface: t.fonts.Display, Weight: weight}
}

// Cell measures one character cell: the advance of a monospace glyph and the
// height a row of text at that size should occupy. Lists and listings are laid
// out on this grid, which keeps columns aligned without measuring strings.
func (t *Theme) Cell(gtx layout.Context, size unit.Sp, bold bool) image.Point {
	weight := WeightBody
	if bold {
		weight = WeightLabel
	}
	return t.CellWeight(gtx, size, weight)
}

// CellWeight measures a cell at an explicit weight. Results are cached per
// size, weight and pixel density, so calling it in a hot loop is free.
func (t *Theme) CellWeight(gtx layout.Context, size unit.Sp, weight font.Weight) image.Point {
	key := cellKey{size, gtx.Metric.PxPerSp, weight}
	if c, ok := t.cells[key]; ok {
		return c
	}
	px := fixed.I(gtx.Sp(size))
	t.Shaper.LayoutString(text.Parameters{
		Font:     t.Font(weight, font.Regular),
		PxPerEm:  px,
		MaxWidth: 1 << 20,
		Locale:   gtx.Locale,
	}, "0")
	var advance, ascent, descent fixed.Int26_6
	for g, ok := t.Shaper.NextGlyph(); ok; g, ok = t.Shaper.NextGlyph() {
		advance += g.Advance
		ascent = max(ascent, g.Ascent)
		descent = max(descent, g.Descent)
	}
	cell := image.Pt(advance.Ceil(), (ascent + descent).Ceil())
	if cell.X <= 0 {
		cell.X = gtx.Sp(size) * 3 / 5
	}
	if cell.Y <= 0 {
		cell.Y = gtx.Sp(size) * 5 / 4
	}
	t.cells[key] = cell
	return cell
}

// Row is the height of one row of a list or a table. The system fixes this at
// GapRow rather than deriving it from the type, so that every list in an
// application prints on the same pitch.
func (t *Theme) Row(gtx layout.Context) int {
	return max(gtx.Dp(GapRow), t.Cell(gtx, SizeUI, false).Y+gtx.Dp(Sp2))
}

// CodeRow is one line of a source listing, set on the code line height, which
// is tighter than a table row so that more of a file fits on screen.
func (t *Theme) CodeRow(gtx layout.Context) int {
	lh := int(float32(gtx.Sp(SizeCode))*LineCode + 0.5)
	return max(lh, t.Cell(gtx, SizeCode, false).Y)
}

// TextRow is one line of running text — a note, a help line, a sheet. It
// follows the normal line height, looser than code and tighter than a row.
func (t *Theme) TextRow(gtx layout.Context, size unit.Sp) int {
	lh := int(float32(gtx.Sp(size))*LineNormal + 0.5)
	return max(lh, t.Cell(gtx, size, false).Y)
}

// StripHeight is the height of a header, status bar or panel head: a strip
// deep enough for a line of label capitals, and never shallower than a small
// control. Chrome is fixed in device pixels, so it holds still as the type is
// set larger — up to the point where a strip would clip its own label, which
// is worse than a strip a few pixels taller.
func (t *Theme) StripHeight(gtx layout.Context) int {
	return max(gtx.Dp(ControlH), t.Cell(gtx, SizeLabel, true).Y+gtx.Dp(Sp2))
}

// MastheadHeight is the height of the topmost strip of a window, which is
// deeper than the rest: it is the one navigation layer on the screen.
func (t *Theme) MastheadHeight(gtx layout.Context) int {
	return max(gtx.Dp(TopbarH), t.Row(gtx)+gtx.Dp(Sp5))
}
