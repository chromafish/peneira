// Command specimen draws every colour, size and control reef defines, in both
// themes.
//
// The colour grid is built by reflecting over Palette, so a new alias shows up
// here without anyone remembering to add it.
//
//	go run ./reef/cmd/specimen              # window; t inverts, b changes paper
//	go run ./reef/cmd/specimen -png out     # out-light.png and out-dark.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/gpu/headless"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"github.com/chromafish/check/reef"
)

func main() {
	out := flag.String("png", "", "render both themes to <prefix>-light.png and <prefix>-dark.png")
	width := flag.Int("w", 900, "width in points")
	scheme := flag.String("scheme", "", "a base16 scheme file to draw instead of reef's own")
	accent := flag.Int("accent", 0, "which base16 colour drives actions, e.g. 11 for base0B")
	flag.Parse()

	s := newSpecimen()
	if *scheme != "" {
		if err := s.load(*scheme, *accent); err != nil {
			fmt.Fprintln(os.Stderr, "specimen:", err)
			os.Exit(1)
		}
	}
	if *out != "" {
		for _, dark := range []bool{false, true} {
			if !s.ui.Scheme().Has(dark) {
				continue
			}
			name := *out + "-light.png"
			if dark {
				name = *out + "-dark.png"
			}
			if err := s.shoot(name, *width, dark); err != nil {
				fmt.Fprintln(os.Stderr, "specimen:", err)
				os.Exit(1)
			}
			fmt.Println("wrote", name)
		}
		return
	}

	go func() {
		w := new(app.Window)
		w.Option(app.Title("reef · specimen"), app.Size(unit.Dp(940), unit.Dp(900)))
		var ops op.Ops
		for {
			switch e := w.Event().(type) {
			case app.DestroyEvent:
				if e.Err != nil {
					fmt.Fprintln(os.Stderr, "specimen:", e.Err)
					os.Exit(1)
				}
				os.Exit(0)
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				s.frame(gtx)
				e.Frame(gtx.Ops)
			}
		}
	}()
	app.Main()
}

type specimen struct {
	ui     *reef.UI
	field  *reef.Field
	scroll int
}

func newSpecimen() *specimen {
	s := &specimen{ui: reef.New(), field: reef.NewField("mine()")}
	s.field.Placeholder = "(everything)"
	return s
}

// load reads a base16 scheme, puts it in force, and prints whatever is wrong
// with it. A scheme that fails a contrast check still gets drawn: seeing it is
// the point, and the report says what to look at.
func (s *specimen) load(path string, accent int) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	b, err := reef.ParseBase16(raw)
	if err != nil {
		return err
	}
	b.Accent = accent
	id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	scheme := b.Scheme(id)
	s.ui.SetScheme(scheme)
	for _, problem := range scheme.Check() {
		fmt.Fprintln(os.Stderr, "  ", problem)
	}
	return nil
}

// frame draws one frame: the page, scrolled, with the status strip below it.
func (s *specimen) frame(gtx layout.Context) {
	gtx = s.ui.Sized(gtx)
	size := gtx.Constraints.Max
	s.keys(gtx)

	reef.Fill(gtx, size, s.ui.P.Bg)
	statusH := s.ui.StripHeight(gtx)
	viewport := image.Pt(size.X, size.Y-statusH)

	// The page is recorded at its full height and then offset, so the window
	// and the PNG share one drawing path.
	macro := op.Record(gtx.Ops)
	page := gtx
	page.Constraints = layout.Exact(image.Pt(viewport.X, 1<<20))
	height := s.page(page)
	call := macro.Stop()

	s.scroll = clamp(s.scroll, 0, max(0, height-viewport.Y))
	area := clip.Rect{Max: viewport}.Push(gtx.Ops)
	off := op.Offset(image.Pt(0, -s.scroll)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	s.wheel(gtx, viewport)
	area.Pop()

	off = op.Offset(image.Pt(0, size.Y-statusH)).Push(gtx.Ops)
	sub := gtx
	sub.Constraints = layout.Exact(image.Pt(size.X, statusH))
	s.status(sub)
	off.Pop()

	s.ui.CornerTicks(gtx, size)
}

// shoot renders the page at its full height to a PNG, at two pixels per point.
func (s *specimen) shoot(name string, width int, dark bool) error {
	s.ui.SetDark(dark)

	var (
		ops    op.Ops
		router input.Router
	)
	newGtx := func(h int) layout.Context {
		return s.ui.Sized(layout.Context{
			Ops:         &ops,
			Metric:      unit.Metric{PxPerDp: 2, PxPerSp: 2},
			Constraints: layout.Exact(image.Pt(width*2, h)),
			Source:      router.Source(),
		})
	}

	measure := newGtx(1 << 20)
	height := s.page(measure) + measure.Dp(reef.Sp6)

	win, err := headless.NewWindow(width*2, height)
	if err != nil {
		return err
	}
	defer win.Release()

	ops.Reset()
	gtx := newGtx(height)
	reef.Fill(gtx, gtx.Constraints.Max, s.ui.P.Bg)
	s.page(gtx)
	router.Frame(&ops)
	if err := win.Frame(&ops); err != nil {
		return err
	}

	img := image.NewRGBA(image.Rect(0, 0, width*2, height))
	if err := win.Screenshot(img); err != nil {
		return err
	}
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func (s *specimen) keys(gtx layout.Context) {
	s.field.Update(gtx)
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: "T"},
			key.Filter{Name: "B"},
			key.Filter{Name: key.NameDownArrow},
			key.Filter{Name: key.NameUpArrow},
			key.Filter{Name: key.NameSpace, Optional: key.ModShift},
		)
		if !ok {
			return
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		switch ke.Name {
		case "T":
			s.ui.ToggleDark()
		case "B":
			s.paper()
		case key.NameDownArrow:
			s.scroll += gtx.Dp(reef.Sp9)
		case key.NameUpArrow:
			s.scroll -= gtx.Dp(reef.Sp9)
		case key.NameSpace:
			step := gtx.Constraints.Max.Y / 2
			if ke.Modifiers.Contain(key.ModShift) {
				step = -step
			}
			s.scroll += step
		}
		reef.Redraw(gtx)
	}
}

// paper steps through the built-in schemes. A scheme loaded from a file is not
// one of them, so it lands on the first.
func (s *specimen) paper() {
	builtin := reef.Builtin()
	i := slices.IndexFunc(builtin, func(sc reef.Scheme) bool { return sc.ID == s.ui.Scheme().ID })
	s.ui.SetScheme(builtin[(i+1)%len(builtin)])
}

func (s *specimen) wheel(gtx layout.Context, size image.Point) {
	stack := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, s)
	stack.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  s,
			Kinds:   pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
		})
		if !ok {
			return
		}
		if pe, ok := ev.(pointer.Event); ok {
			s.scroll += int(pe.Scroll.Y)
			reef.Redraw(gtx)
		}
	}
}

// page draws the sections in order and returns the height they took.
func (s *specimen) page(gtx layout.Context) int {
	width := gtx.Constraints.Max.X
	y := 0
	for _, section := range []func(layout.Context) int{
		s.head, s.typography, s.colours, s.controls, s.structure,
	} {
		off := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		sub := gtx
		sub.Constraints.Min = image.Point{}
		sub.Constraints.Max = image.Pt(width, 1<<20)
		y += section(sub)
		off.Pop()
	}
	return y
}

// head is the masthead: the wordmark, and which theme is in force.
func (s *specimen) head(gtx layout.Context) int {
	ui := s.ui
	w := gtx.Constraints.Max.X
	h := ui.MastheadHeight(gtx)
	ui.Masthead(gtx, image.Pt(w, h))

	pad := gtx.Dp(reef.Sp6)
	d := draw(gtx, pad, 0, func(gtx layout.Context) layout.Dimensions {
		return ui.Wordmark(gtx, reef.SizeWordmark, ui.P.Action, "reef")
	})
	x := pad + d.Size.X + gtx.Dp(reef.Sp5)
	// The masthead was drawn at height h, so the labels centre on it.
	x += ui.LabelAt(gtx, ui.P.Muted, x, h, w, "SPECIMEN") + gtx.Dp(reef.Sp5)

	mode := "LIGHT"
	if ui.Dark {
		mode = "DARK"
	}
	name := strings.ToUpper(ui.Scheme().Name) + " · " + mode
	if ui.Scheme().Modes() > 1 {
		name += " · T INVERTS"
	}
	ui.LabelAt(gtx, ui.P.Faint, x, h, w, name)
	return h
}

// typography: the scale, the weights, and the three ways the system sets a
// name rather than a value.
func (s *specimen) typography(gtx layout.Context) int {
	ui := s.ui
	w := gtx.Constraints.Max.X
	pad := gtx.Dp(reef.Sp6)
	y := s.heading(gtx, "TYPE")

	sizes := []struct {
		name string
		size unit.Sp
	}{
		{"SizeMicro", reef.SizeMicro},
		{"SizeLabel", reef.SizeLabel},
		{"SizeMeta", reef.SizeMeta},
		{"SizeData · SizeUI · SizeCode", reef.SizeData},
		{"SizeBody", reef.SizeBody},
		{"SizeBodyLg", reef.SizeBodyLg},
		{"SizeH4 · SizeSection", reef.SizeH4},
		{"SizeH3", reef.SizeH3},
		{"SizeH2", reef.SizeH2},
	}
	for _, sz := range sizes {
		row := ui.TextRow(gtx, sz.size)
		off := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		x := pad
		x += ui.TextAt(gtx, reef.Run{Size: sz.size, Weight: reef.WeightBody, Color: ui.P.Fg},
			x, row, w-pad, "Handgloves 0123") + gtx.Dp(reef.Sp5)
		ui.TextAt(gtx, reef.Run{Size: reef.SizeMicro, Weight: reef.WeightBody, Color: ui.P.Faint},
			x, row, w-pad, fmt.Sprintf("%s  %gsp", sz.name, float32(sz.size)))
		off.Pop()
		y += row
	}

	y += gtx.Dp(reef.Sp4)
	row := ui.Row(gtx)
	weights := []struct {
		name string
		w    font.Weight
	}{
		{"WeightLight", reef.WeightLight},
		{"WeightBody", reef.WeightBody},
		{"WeightLabel", reef.WeightLabel},
		{"WeightDisplay", reef.WeightDisplay},
	}
	off := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	x := pad
	for _, wt := range weights {
		x += ui.TextAt(gtx, reef.Run{Size: reef.SizeBodyLg, Weight: wt.w, Color: ui.P.Fg},
			x, row, w-pad, "Aa") + gtx.Dp(reef.Sp2)
		x += ui.TextAt(gtx, reef.Run{Size: reef.SizeMicro, Weight: reef.WeightBody, Color: ui.P.Faint},
			x, row, w-pad, wt.name) + gtx.Dp(reef.Sp6)
	}
	off.Pop()
	y += row

	// Label, Micro and Wordmark: capitals and tracking mean a label, the
	// display face is only ever a wordmark.
	off = op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	x = pad
	x += ui.LabelAt(gtx, ui.P.Strong, x, row, w-pad, "LABEL · TRACKED CAPITALS") + gtx.Dp(reef.Sp6)
	sub := gtx
	sub.Constraints.Min = image.Point{}
	sub.Constraints.Max.X = w - x
	x += draw(sub, x, (row-ui.Cell(gtx, reef.SizeMicro, true).Y)/2, func(gtx layout.Context) layout.Dimensions {
		return ui.Micro(gtx, ui.P.Muted, "MICRO 12")
	}).Size.X + gtx.Dp(reef.Sp6)
	draw(gtx, x, 0, func(gtx layout.Context) layout.Dimensions {
		return ui.Wordmark(gtx, reef.SizeH4, ui.P.Accent, "wordmark")
	})
	off.Pop()
	y += row + gtx.Dp(reef.Sp6)
	return y
}

// role is one named colour of the palette.
type role struct {
	name string
	c    color.NRGBA
}

// roles walks the Palette struct so that every alias reaches the page, and
// names the syntax array by its constants.
func roles(p reef.Palette) []role {
	syntax := []string{
		"SyntaxPlain", "SyntaxKeyword", "SyntaxName", "SyntaxFunction",
		"SyntaxType", "SyntaxString", "SyntaxNumber", "SyntaxComment",
		"SyntaxOperator", "SyntaxPunctuation", "SyntaxPreproc", "SyntaxError",
	}
	var out []role
	v := reflect.ValueOf(p)
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		switch {
		case f.Type == reflect.TypeOf(color.NRGBA{}):
			out = append(out, role{f.Name, v.Field(i).Interface().(color.NRGBA)})
		case f.Type.Kind() == reflect.Array:
			arr := v.Field(i)
			for j := range arr.Len() {
				name := fmt.Sprintf("%s[%d]", f.Name, j)
				if f.Name == "Syntax" && j < len(syntax) {
					name = syntax[j]
				}
				out = append(out, role{name, arr.Index(j).Interface().(color.NRGBA)})
			}
		}
	}
	return out
}

// colours draws a swatch per alias, with its name and value under it.
func (s *specimen) colours(gtx layout.Context) int {
	ui := s.ui
	w := gtx.Constraints.Max.X
	pad := gtx.Dp(reef.Sp6)
	y := s.heading(gtx, "COLOUR")

	all := roles(ui.P)
	cellW := gtx.Dp(126)
	cols := max(1, (w-pad*2)/cellW)
	cellW = (w - pad*2) / cols
	swatchH := gtx.Dp(reef.Sp9)
	nameRow := ui.TextRow(gtx, reef.SizeMicro)
	cellH := swatchH + nameRow*2 + gtx.Dp(reef.Sp3)

	for i, r := range all {
		col, line := i%cols, i/cols
		x := pad + col*cellW
		top := y + line*cellH

		box := image.Rect(x, top, x+cellW-gtx.Dp(reef.Sp3), top+swatchH)
		reef.FillRect(gtx, box, r.c)
		// A swatch the colour of the page needs an edge to be a swatch at all.
		reef.Stroke(gtx, box, ui.P.RuleFaint)
		if r.c.A == 0 {
			ui.TextCentred(gtx, reef.Run{Size: reef.SizeMicro, Weight: reef.WeightLabel, Color: ui.P.Error}, box, "UNSET")
		}

		off := op.Offset(image.Pt(0, top+swatchH)).Push(gtx.Ops)
		ui.TextAt(gtx, reef.Run{Size: reef.SizeMicro, Weight: reef.WeightLabel, Color: ui.P.Fg},
			x, nameRow, x+cellW-gtx.Dp(reef.Sp3), r.name)
		off.Pop()
		off = op.Offset(image.Pt(0, top+swatchH+nameRow)).Push(gtx.Ops)
		ui.TextAt(gtx, reef.Run{Size: reef.SizeMicro, Weight: reef.WeightBody, Color: ui.P.Faint},
			x, nameRow, x+cellW-gtx.Dp(reef.Sp3), hex(r.c))
		off.Pop()
	}

	lines := (len(all) + cols - 1) / cols
	return y + lines*cellH + gtx.Dp(reef.Sp6)
}

func hex(c color.NRGBA) string {
	if c.A == 0 {
		return "—"
	}
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// controls draws each control in every state it has, including the hovered
// ones, which SetHover stands in for.
func (s *specimen) controls(gtx layout.Context) int {
	ui := s.ui
	w := gtx.Constraints.Max.X
	pad := gtx.Dp(reef.Sp6)
	y := s.heading(gtx, "CONTROLS")
	row := ui.Row(gtx)
	ctrlH := gtx.Dp(reef.ControlH)

	// Control, at rest and under the pointer, in the three colours it takes.
	off := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	x := pad
	x += ui.Control(gtx, x, ctrlH, "rest", "COPY NOTES  ⌘⇧C", ui.P.Action, nil) + gtx.Dp(reef.Sp3)
	x += ui.Control(gtx, x, ctrlH, "muted", "KEYS  ?", ui.P.Muted, nil) + gtx.Dp(reef.Sp3)
	x += ui.Control(gtx, x, ctrlH, "warn", "STALE  R", ui.P.Warn, nil) + gtx.Dp(reef.Sp5)
	ui.SetHover("hovered")
	x += ui.Control(gtx, x, ctrlH, "hovered", "HOVERED", ui.P.Action, nil) + gtx.Dp(reef.Sp3)
	ui.SetHover(nil)
	ui.LabelAt(gtx, ui.P.Faint, x, ctrlH, w-pad, "UI.CONTROL")
	off.Pop()
	y += ctrlH + gtx.Dp(reef.Sp4)

	// Box, in its two states and under the pointer.
	off = op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	x = pad
	boxW := gtx.Dp(reef.Sp8)
	for _, b := range []struct {
		tag event.Tag
		set bool
		c   color.NRGBA
	}{
		{"unread", false, ui.P.Muted},
		{"read", true, ui.P.Ok},
		{"stale", true, ui.P.Warn},
	} {
		ui.Box(gtx, image.Rect(x, 0, x+boxW, row), b.tag, b.set, b.c, nil)
		x += boxW
	}
	ui.SetHover("boxhover")
	ui.Box(gtx, image.Rect(x, 0, x+boxW, row), "boxhover", false, ui.P.Muted, nil)
	ui.SetHover(nil)
	x += boxW + gtx.Dp(reef.Sp4)
	ui.LabelAt(gtx, ui.P.Faint, x, row, w-pad, "UI.BOX · UNSET SET STALE HOVERED")
	off.Pop()
	y += row + gtx.Dp(reef.Sp4)

	// A row in each of its states, on the pitch a list is laid out on.
	rowW := min(gtx.Dp(320), w-pad*2)
	for _, st := range []struct {
		label string
		paint func(layout.Context, image.Point)
	}{
		{"SELECTION · FOCUSED", func(gtx layout.Context, size image.Point) { ui.Selection(gtx, size, true) }},
		{"SELECTION · IDLE", func(gtx layout.Context, size image.Point) { ui.Selection(gtx, size, false) }},
		{"HOVERFILL", func(gtx layout.Context, size image.Point) { ui.HoverFill(gtx, size) }},
	} {
		off := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
		sub := gtx
		sub.Constraints = layout.Exact(image.Pt(rowW, row))
		st.paint(sub, sub.Constraints.Max)
		ui.TextAt(sub, reef.Strong(ui.P.Fg), gtx.Dp(reef.PadInline), row, rowW, "internal/ui/app.go")
		reef.HLine(sub, rowW, row-1, ui.P.RuleFaint)
		off.Pop()
		off = op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		ui.LabelAt(gtx, ui.P.Faint, pad+rowW+gtx.Dp(reef.Sp4), row, w-pad, st.label)
		off.Pop()
		y += row
	}
	y += gtx.Dp(reef.Sp4)

	// The field, at the height the system gives a single line of text.
	fieldW := min(gtx.Dp(260), w-pad*2)
	fieldH := ui.FieldHeight(gtx)
	off = op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
	sub := gtx
	sub.Constraints = layout.Exact(image.Pt(fieldW, fieldH))
	s.field.Layout(sub, ui.Theme)
	off.Pop()
	off = op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	ui.LabelAt(gtx, ui.P.Faint, pad+fieldW+gtx.Dp(reef.Sp4), fieldH, w-pad, "FIELD")
	off.Pop()
	y += fieldH + gtx.Dp(reef.Sp5)

	// A panel and the rail its column leaves behind, side by side.
	panelW := min(gtx.Dp(300), (w-pad*3)/2)
	panelH := row * 4
	off = op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
	sub = gtx
	sub.Constraints = layout.Exact(image.Pt(panelW, panelH))
	ui.Panel(sub, reef.Panel{
		Title:   "PANEL · FOCUSED",
		Focused: true,
		Reserve: ui.StripHeight(gtx),
		Header: func(gtx layout.Context) {
			size := gtx.Constraints.Max
			ui.Collapse(gtx, size.X, size.Y, "collapse", nil)
		},
		Body: func(gtx layout.Context) {
			ui.TextAt(gtx, reef.Body(ui.P.Fg), gtx.Dp(reef.PadInline), row, gtx.Constraints.Max.X, "a row of the body")
		},
	})
	off.Pop()

	off = op.Offset(image.Pt(pad+panelW+gtx.Dp(reef.Sp5), y)).Push(gtx.Ops)
	sub = gtx
	sub.Constraints = layout.Exact(image.Pt(gtx.Dp(reef.RailW), panelH))
	ui.Rail(sub, "rail", "R", 12, nil)
	off.Pop()

	// Placeholder: the dot grid, and what an empty pane says.
	phX := pad + panelW + gtx.Dp(reef.Sp5) + gtx.Dp(reef.RailW) + gtx.Dp(reef.Sp5)
	if phW := w - pad - phX; phW > gtx.Dp(120) {
		off = op.Offset(image.Pt(phX, y)).Push(gtx.Ops)
		sub = gtx
		sub.Constraints = layout.Exact(image.Pt(phW, panelH))
		ui.Placeholder(sub, "PLACEHOLDER · NO CHANGES")
		reef.Stroke(sub, image.Rect(0, 0, phW, panelH), ui.P.RuleFaint)
		off.Pop()
	}
	y += panelH + gtx.Dp(reef.Sp6)
	return y
}

// structure draws the marks the system builds everything else out of.
func (s *specimen) structure(gtx layout.Context) int {
	ui := s.ui
	w := gtx.Constraints.Max.X
	pad := gtx.Dp(reef.Sp6)
	y := s.heading(gtx, "STRUCTURE")
	row := ui.Row(gtx)
	boxW, boxH := gtx.Dp(120), row*2

	marks := []struct {
		name string
		draw func(layout.Context, image.Point)
	}{
		{"FILL", func(gtx layout.Context, size image.Point) { reef.Fill(gtx, size, ui.P.BgSunken) }},
		{"HLINE / VLINE", func(gtx layout.Context, size image.Point) {
			reef.HLine(gtx, size.X, size.Y/2, ui.P.Rule)
			reef.VLine(gtx, size.X/2, size.Y, ui.P.Rule)
		}},
		{"EDGE", func(gtx layout.Context, size image.Point) {
			reef.Fill(gtx, size, ui.P.AddBg)
			reef.Edge(gtx, size.Y, ui.P.AddGutter)
		}},
		{"OUTLINE", func(gtx layout.Context, size image.Point) {
			reef.Outline(gtx, image.Rectangle{Max: size}, gtx.Dp(reef.BorderHair), ui.P.RuleStrong)
		}},
		{"STROKE", func(gtx layout.Context, size image.Point) {
			reef.Stroke(gtx, image.Rectangle{Max: size}, ui.P.RuleStrong)
		}},
		{"FOCUSRING", func(gtx layout.Context, size image.Point) {
			ui.FocusRing(gtx, image.Rectangle{Max: size}.Inset(gtx.Dp(reef.BorderThick)))
		}},
		{"DOTGRID", func(gtx layout.Context, size image.Point) { ui.DotGrid(gtx, size) }},
		{"SHEET", func(gtx layout.Context, size image.Point) {
			ui.DotGrid(gtx, size)
			ui.Sheet(gtx, image.Rectangle{Max: size}.Inset(gtx.Dp(reef.Sp3)))
		}},
	}

	cols := max(1, (w-pad*2)/(boxW+gtx.Dp(reef.Sp5)))
	nameRow := ui.TextRow(gtx, reef.SizeMicro)
	for i, m := range marks {
		col, line := i%cols, i/cols
		x := pad + col*(boxW+gtx.Dp(reef.Sp5))
		top := y + line*(boxH+nameRow+gtx.Dp(reef.Sp4))

		off := op.Offset(image.Pt(x, top)).Push(gtx.Ops)
		sub := gtx
		sub.Constraints = layout.Exact(image.Pt(boxW, boxH))
		m.draw(sub, sub.Constraints.Max)
		off.Pop()

		off = op.Offset(image.Pt(0, top+boxH)).Push(gtx.Ops)
		ui.LabelAt(gtx, ui.P.Faint, x, nameRow, x+boxW+gtx.Dp(reef.Sp5), m.name)
		off.Pop()
	}
	lines := (len(marks) + cols - 1) / cols
	return y + lines*(boxH+nameRow+gtx.Dp(reef.Sp4)) + gtx.Dp(reef.Sp5)
}

// status is the strip at the foot of the window.
func (s *specimen) status(gtx layout.Context) {
	ui := s.ui
	size := gtx.Constraints.Max
	ui.StatusBar(gtx, size)
	pad := gtx.Dp(reef.PadInline)
	ui.LabelAt(gtx, ui.P.Muted, pad, size.Y, size.X, "T INVERTS · B PAPER · SPACE PAGES · SCROLL")
	ui.LabelRight(gtx, ui.P.Faint, size.X-pad, size.Y, "REEF · "+string(reef.Mono))
}

// heading names a section, with a rule under it, and returns the y the section
// body starts at.
func (s *specimen) heading(gtx layout.Context, txt string) int {
	ui := s.ui
	w := gtx.Constraints.Max.X
	pad := gtx.Dp(reef.Sp6)
	row := ui.Row(gtx)
	ui.LabelAt(gtx, ui.P.Strong, pad, row, w-pad, txt)
	reef.HLine(gtx, w, row-1, ui.P.Rule)
	return row + gtx.Dp(reef.Sp4)
}

// draw runs f at a position in the current space.
func draw(gtx layout.Context, x, y int, f func(layout.Context) layout.Dimensions) layout.Dimensions {
	off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	sub := gtx
	sub.Constraints.Min = image.Point{}
	d := f(sub)
	off.Pop()
	return d
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }
