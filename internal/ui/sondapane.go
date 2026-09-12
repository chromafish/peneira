package ui

import (
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"

	"github.com/chromafish/check/internal/sonda"

	"github.com/chromafish/check/reef"
)

const (
	tagSondaRun   tag = "sonda-run"
	tagSondaStop  tag = "sonda-stop"
	tagSondaRaw   tag = "sonda-raw"
	tagSondaLevel tag = "sonda-level"
	tagSondaBack  tag = "sonda-back"
	tagSonda      tag = "sonda"
)

type sondaTargetTag struct{ i int }

type sondaRowTag struct{ pane, row int }

// timeCols is the width of the arrival time column, in cells.
const timeCols = 12

// layoutSonda draws the screen: a strip of controls across the top, and the
// two runs beneath it, the baseline on the left and the working copy on the
// right.
func (a *App) layoutSonda(gtx layout.Context) {
	a.syncSonda()
	ui := a.ui
	size := gtx.Constraints.Max
	stripH := ui.Row(gtx) + gtx.Dp(reef.Sp4)

	fill(gtx, image.Point{}, image.Pt(size.X, stripH), a.sondaStrip)

	bodyY := stripH
	bodyH := size.Y - bodyY
	if bodyH <= 0 {
		return
	}
	if len(a.sonda.targets) == 0 {
		fill(gtx, image.Pt(0, bodyY), image.Pt(size.X, bodyH), a.sondaNoTargets)
		return
	}
	half := size.X / 2
	fill(gtx, image.Pt(0, bodyY), image.Pt(half, bodyH), func(gtx layout.Context) {
		a.sondaPanel(gtx, paneBaseline)
	})
	fill(gtx, image.Pt(half+1, bodyY), image.Pt(size.X-half-1, bodyH), func(gtx layout.Context) {
		a.sondaPanel(gtx, paneReview)
	})
	reef.FillRect(gtx, image.Rect(half, bodyY, half+1, size.Y), ui.P.Rule)
}

// sondaStrip draws the controls: what is being run on the left, what can be
// done to it on the right, and the filter between.
func (a *App) sondaStrip(gtx layout.Context) {
	ui := a.ui
	s := a.sonda
	size := gtx.Constraints.Max
	pad := gtx.Dp(reef.PadInline)
	cell := ui.Cell(gtx, reef.SizeUI, false)
	h := size.Y

	reef.Fill(gtx, size, ui.P.BgSunken)
	reef.HLine(gtx, size.X, h-1, ui.P.Rule)

	// The right of the strip is measured first, and the left takes what is
	// left, so two runs never overprint.
	rightX := size.X - pad
	rightX -= a.controlRight(gtx, rightX, h, tagSondaBack, "BACK  ESC", ui.P.Muted, a.toggleSonda) + gtx.Dp(reef.Sp5)

	running := false
	for i := range s.panes {
		if r := s.panes[i].run; r != nil && s.panes[i].state.Phase != sonda.Ended {
			running = true
		}
	}
	stopColor, stop := ui.P.Faint, func() {}
	if running {
		stopColor, stop = ui.P.Error, a.stopSonda
	}
	rightX -= a.controlRight(gtx, rightX, h, tagSondaStop, "STOP  X", stopColor, stop) + gtx.Dp(reef.Sp3)
	runColor := ui.P.Action
	if len(s.targets) == 0 || s.preparing {
		runColor = ui.P.Faint
	}
	rightX -= a.controlRight(gtx, rightX, h, tagSondaRun, "RUN  R", runColor, a.startSonda) + gtx.Dp(reef.Sp5)

	rawColor := ui.P.Muted
	if s.raw {
		rawColor = ui.P.Action
	}
	rightX -= a.controlRight(gtx, rightX, h, tagSondaRaw, "RAW  W", rawColor, a.toggleRaw) + gtx.Dp(reef.Sp3)
	levelColor := ui.P.Muted
	if s.minLevel != sonda.Unlevelled {
		levelColor = ui.P.Action
	}
	rightX -= a.controlRight(gtx, rightX, h, tagSondaLevel, "LEVEL "+levelLabel(s.minLevel)+"  L", levelColor, a.cycleLevel) + gtx.Dp(reef.Sp5)

	// The filter well, labelled in place of a border.
	fieldW := min(28*cell.X, max(0, rightX-size.X/3))
	if fieldW > 8*cell.X {
		fh := ui.FieldHeight(gtx)
		fx := rightX - fieldW
		fill(gtx, image.Pt(fx, (h-fh)/2), image.Pt(fieldW, fh), func(gtx layout.Context) {
			s.filter.Layout(gtx, ui.Theme)
		})
		rightX = fx - gtx.Dp(reef.Sp3)
		rightX -= ui.LabelRight(gtx, ui.P.Faint, rightX, h, "FILTER  /") + gtx.Dp(reef.Sp5)
	}

	x := pad
	x += ui.LabelAt(gtx, ui.P.Strong, x, h, rightX, "SONDA") + gtx.Dp(reef.Sp5)
	switch len(s.targets) {
	case 0:
		a.cellText(gtx, x, h, rightX, font.Normal, ui.P.Muted, "no target declared")
	case 1:
		x += ui.LabelAt(gtx, ui.P.Faint, x, h, rightX, "TARGET") + gtx.Dp(reef.Sp3)
		a.cellText(gtx, x, h, rightX, reef.WeightLabel, ui.P.Fg, s.targets[0].Name)
	default:
		// Several targets are a row of controls, the chosen one in the
		// action colour, since choosing is what the row is for.
		x += ui.LabelAt(gtx, ui.P.Faint, x, h, rightX, "TARGET") + gtx.Dp(reef.Sp3)
		for i, t := range s.targets {
			if x >= rightX {
				break
			}
			c := ui.P.Muted
			if i == s.target {
				c = ui.P.Action
			}
			x += a.control(gtx, x, h, sondaTargetTag{i}, strings.ToUpper(t.Name), c, func() { a.chooseTarget(i) }) + gtx.Dp(reef.Sp3)
		}
	}
}

// sondaNoTargets fills the body when the directory declares no program to
// run, and says where one is declared.
func (a *App) sondaNoTargets(gtx layout.Context) {
	ui := a.ui
	size := gtx.Constraints.Max
	row := ui.Row(gtx)
	pad := gtx.Dp(reef.Gutter)
	cell := ui.Cell(gtx, reef.SizeUI, false)

	ui.DotGrid(gtx, size)
	lines := []string{
		"[[target]]",
		`name = "` + a.repoName + `"`,
		`dir = "."`,
		`build = ["make", "build"]`,
		`run = ["./` + a.repoName + `"]`,
	}
	w := min(size.X-gtx.Dp(80), 64*cell.X)
	h := min(size.Y-gtx.Dp(40), row*(len(lines)+4)+pad*2)
	x, y := (size.X-w)/2, max(gtx.Dp(24), (size.Y-h)/3)
	sheet := image.Rect(x, y, x+w, y+h)
	ui.Sheet(gtx, sheet)

	ly := y + pad
	fit(gtx, image.Pt(0, ly), image.Pt(x+w-pad, row), func(gtx layout.Context) {
		ui.LabelAt(gtx, ui.P.Strong, x+pad, row, x+w-pad, "NO TARGET DECLARED")
	})
	ly += row
	fit(gtx, image.Pt(0, ly), image.Pt(x+w-pad, row), func(gtx layout.Context) {
		a.cellText(gtx, x+pad, row, x+w-pad, font.Normal, ui.P.Muted,
			"Declare the program to run in "+sonda.File+" in "+shortenHome(a.dir)+":")
	})
	ly += row + row/2
	cc := ui.Cell(gtx, reef.SizeCode, false)
	for _, line := range lines {
		fit(gtx, image.Pt(x+pad, ly+(row-cc.Y)/2), image.Pt(w-pad*2, cc.Y), func(gtx layout.Context) {
			ui.Text(gtx, reef.Code(ui.P.Fg), line)
		})
		ly += row
	}
}

// sondaPanel frames one run.
func (a *App) sondaPanel(gtx layout.Context, i int) {
	s := a.sonda
	p := &s.panes[i]
	filtered := s.currentFilter() != (logFilter{})
	a.ui.Panel(gtx, reef.Panel{
		Title:   p.sondaTitle(filtered),
		Focused: s.focus == i,
		Reserve: gtx.Dp(reef.PadInline),
		Body:    func(gtx layout.Context) { a.sondaBody(gtx, i) },
	})
}

// sondaBody draws the logs of one run, or says why there are none.
func (a *App) sondaBody(gtx layout.Context, i int) {
	ui := a.ui
	s := a.sonda
	p := &s.panes[i]
	size := gtx.Constraints.Max

	switch {
	case p.run == nil && p.note != "":
		a.placeholder(gtx, strings.ToUpper(p.note))
		return
	case p.run == nil:
		a.placeholder(gtx, "NOT RUN · R STARTS BOTH")
		return
	case len(p.logs) == 0 && p.state.Phase == sonda.Building:
		a.placeholder(gtx, "BUILDING")
		return
	case len(p.logs) == 0 && p.state.Phase == sonda.Running:
		a.placeholder(gtx, "RUNNING · NOTHING WRITTEN YET")
		return
	case len(p.logs) == 0:
		a.placeholder(gtx, "NOTHING WAS WRITTEN")
		return
	case len(p.view) == 0:
		a.placeholder(gtx, "NOTHING MATCHES THE FILTER")
		return
	}

	// Clicking anywhere in the pane gives it the keyboard, and the body takes
	// a sideways scroll while the list takes the vertical one.
	a.clickArea(gtx, image.Rectangle{Max: size}, p, func() { s.focus = i })
	a.sondaScroll(gtx, p, size)

	cell := ui.Cell(gtx, reef.SizeCode, false)
	row := ui.CodeRow(gtx)
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()

	// Following the run means laying the list out from its end, which Gio
	// does when asked to scroll to the end and told it is there. That is only
	// asked for once the logs overflow the pane: a shorter list laid out from
	// the end sits at the bottom with blank space over it.
	list := &p.list
	list.ScrollToEnd = p.follow && p.overflow
	if p.follow {
		list.Position.BeforeEnd = false
	}
	list.Layout(gtx, len(p.view), func(gtx layout.Context, n int) layout.Dimensions {
		l := p.logs[p.view[n]]
		lines := logLines(l, s.raw)
		h := row * len(lines)
		gtx.Constraints = layout.Exact(image.Pt(size.X, h))
		a.sondaRow(gtx, i, n, l, lines, cell, row)
		return layout.Dimensions{Size: image.Pt(size.X, h)}
	})

	// A wheel taken away from the end is the pointer's way of saying stop
	// following; the reading position comes along to where the pane is.
	if list.ScrollToEnd && list.Position.BeforeEnd {
		p.follow = false
		p.cursor = clamp(list.Position.First, 0, len(p.view)-1)
	}
	if overflow := list.Position.Length > size.Y; overflow != p.overflow {
		p.overflow = overflow
		reef.Redraw(gtx)
	}
}

// sondaScroll lets a trackpad pan a pane sideways.
func (a *App) sondaScroll(gtx layout.Context, p *logPane, size image.Point) {
	stack := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &p.x)
	stack.Pop()
	cell := a.ui.Cell(gtx, reef.SizeCode, false)
	visible := 0
	if cell.X > 0 {
		visible = (size.X - a.sondaTextX(gtx, cell)) / cell.X
	}
	limit := max(0, p.cols-visible)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &p.x,
			Kinds:   pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: -200, Max: 200},
		})
		if !ok {
			break
		}
		if pe, ok := ev.(pointer.Event); ok {
			p.x = clamp(p.x+int(pe.Scroll.X), 0, limit)
		}
	}
}

// sondaTextX is where the text of a row starts: after the time and level
// columns.
func (a *App) sondaTextX(gtx layout.Context, cell image.Point) int {
	return gtx.Dp(reef.PadInline) + (timeCols+1+levelCols+1)*cell.X
}

// sondaRow draws one log: arrival time, level, and the lines of its text.
func (a *App) sondaRow(gtx layout.Context, pane, n int, l sonda.Log, lines [][]textRun, cell image.Point, row int) {
	ui := a.ui
	s := a.sonda
	p := &s.panes[pane]
	size := gtx.Constraints.Max
	cursor := n == p.cursor
	tag := sondaRowTag{pane, n}

	switch {
	case cursor:
		ui.Selection(gtx, size, s.focus == pane)
	case a.hovered(tag):
		ui.HoverFill(gtx, size)
	}
	// Where a line came from is carried down its leading edge: nothing for
	// standard output, the rule colour for standard error, and the error
	// colour for what a failed build printed.
	if !cursor {
		switch l.Stream {
		case sonda.Stderr:
			reef.Edge(gtx, size.Y, ui.P.Rule)
		case sonda.Build:
			reef.Edge(gtx, size.Y, ui.P.ErrorLine)
		}
	}
	a.clickable(gtx, size, tag, func() {
		s.focus = pane
		p.cursor = n
		p.follow = n == len(p.view)-1
	})

	x := gtx.Dp(reef.PadInline)
	a.codeText(gtx, x, row, size.X, font.Normal, ui.P.Faint, l.At.Format("15:04:05.000"))
	x += (timeCols + 1) * cell.X

	if level := levelText(l); level != "" {
		weight, c := font.Normal, a.levelColor(l.Severity)
		if l.Severity >= sonda.Error {
			weight = reef.WeightLabel
		}
		a.codeText(gtx, x, row, size.X, weight, c, level)
	}
	x += (levelCols + 1) * cell.X

	if size.X <= x {
		return
	}
	// Runs are placed end to end by the width each one shaped to. The cell
	// grid is a whole number of pixels and the glyph advance is not, so a
	// long run set on the grid would drift from the run after it.
	area := clip.Rect(image.Rect(x, 0, size.X, size.Y)).Push(gtx.Ops)
	for k, line := range lines {
		cx := x - p.x*cell.X
		for _, r := range line {
			if cx >= size.X {
				break
			}
			w := 0
			fill(gtx, image.Pt(cx, k*row), image.Pt(size.X-cx, row), func(gtx layout.Context) {
				w = a.codeText(gtx, 0, row, size.X-cx, font.Normal, a.roleColor(r.role), r.text)
			})
			cx += w
		}
	}
	area.Pop()
}

// levelColor spends the status families on levels, so an error is found
// without reading for it.
func (a *App) levelColor(s sonda.Severity) reef.ColorNRGBA {
	P := a.ui.P
	switch s {
	case sonda.Error, sonda.Fatal, sonda.Panic:
		return P.Error
	case sonda.Warn:
		return P.Warn
	case sonda.Info:
		return P.Info
	case sonda.Trace, sonda.Debug:
		return P.Faint
	}
	return P.Muted
}

// roleColor maps the roles a line is set in onto the syntax colours the diff
// uses for the same things.
func (a *App) roleColor(r textRole) reef.ColorNRGBA {
	P := a.ui.P
	switch r {
	case roleKey:
		return P.Muted
	case rolePunct:
		return P.Faint
	case roleString:
		return P.Syntax[reef.SyntaxString]
	case roleNumber:
		return P.Syntax[reef.SyntaxNumber]
	case roleWord:
		return P.Syntax[reef.SyntaxKeyword]
	case roleSource:
		return P.Syntax[reef.SyntaxComment]
	}
	return P.Fg
}
