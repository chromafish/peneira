package reef_test

import (
	"image"
	"strings"
	"testing"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/chromafish/check/reef"
)

// gtx returns a context of the size given, with the metrics a test measures
// in: one pixel per point, so a Dp in the source is a pixel here.
func gtx(ops *op.Ops, src input.Source, w, h int) layout.Context {
	return layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, h)),
		Source:      src,
	}
}

// The whole system has to draw without a window or a GPU: laying out only
// builds operations. This is what lets an application built on reef be tested
// at all, so the package holds itself to it.
func TestEveryWidgetLaysOutHeadless(t *testing.T) {
	var (
		ops    op.Ops
		router input.Router
	)
	ui := reef.New()
	field := reef.NewField("mine()")
	field.Placeholder = "(everything)"
	splits := reef.NewSplits(0.25, 0.25)

	for _, dark := range []bool{false, true} {
		ui.SetDark(dark)
		ops.Reset()
		c := gtx(&ops, router.Source(), 800, 600)
		field.Update(c)

		reef.Fill(c, c.Constraints.Max, ui.P.Bg)
		widths := splits.Widths(c, 800)
		if len(widths) != 2 {
			t.Fatalf("Widths returned %d columns, want 2", len(widths))
		}
		splits.Handles(c, c.Constraints.Max, widths)

		reef.Panel{Title: "REVISIONS", Focused: true, Reserve: 28, Body: func(c layout.Context) {
			ui.Selection(c, image.Pt(200, 28), true)
			ui.TextAt(c, reef.Strong(ui.P.Fg), 8, 28, 200, "szuwkqnr")
			ui.Box(c, image.Rect(0, 0, 24, 28), "box", true, ui.P.Ok, func() {})
			ui.Placeholder(c, "NOTHING HERE")
		}}.Layout(c, ui.Theme)

		ui.Rail(c, "rail", "R", 12, func() {})
		ui.Collapse(c, 200, 28, "collapse", func() {})
		ui.Control(c, 8, 28, "control", "COPY  ⌘C", ui.P.Action, func() {})
		ui.ControlRight(c, 780, 28, "right", "KEYS  ?", ui.P.Muted, func() {})
		ui.Sheet(c, reef.CentreRect(c.Constraints.Max, 300, 200))
		ui.CornerTicks(c, c.Constraints.Max)
		ui.Wordmark(c, reef.SizeWordmark, ui.P.Action, "reef")
		ui.Label(c, ui.P.Muted, "LABEL")
		ui.Micro(c, ui.P.Faint, "12")
		ui.TextCentred(c, reef.Code(ui.P.Fg), image.Rect(0, 0, 40, 20), "@")
		ui.TextRight(c, reef.Body(ui.P.Muted), 780, 28, "+12 −4")
		field.Layout(c, ui.Theme)

		router.Frame(&ops)
	}
}

// The two themes are one design: every alias is filled in by Ramp.Palette, so a
// component that names one cannot land on an invisible colour in one theme and
// a visible one in the other.
func TestBothPalettesAreComplete(t *testing.T) {
	for name, p := range map[string]reef.Palette{"light": reef.Light, "dark": reef.Dark} {
		if p.Bg == p.Fg {
			t.Errorf("%s: the page and the text it carries are the same colour", name)
		}
		for role := range p.Syntax {
			if p.Syntax[role].A == 0 {
				t.Errorf("%s: syntax role %d is transparent", name, role)
			}
		}
		if p.Rule.A == 0 || p.RuleFaint.A == 0 || p.RuleStrong.A == 0 {
			t.Errorf("%s: a rule is transparent, and rules are what draw the structure", name)
		}
	}
}

// A column is a fraction of the window until that leaves too little of it, and
// a column that has been put away comes back as a rail rather than as nothing.
func TestSplitsKeepTheirProportions(t *testing.T) {
	var ops op.Ops
	c := gtx(&ops, input.Source{}, 1000, 600)
	s := reef.NewSplits(0.2, 0.3)

	w := s.Widths(c, 1000)
	if w[0] != 200 || w[1] != 300 {
		t.Errorf("columns are %v, want [200 300]", w)
	}

	// The remainder has a minimum of its own, and the widest column gives way
	// to it rather than the narrow one disappearing.
	s.SetFraction(0, 0.45)
	s.SetFraction(1, 0.45)
	w = s.Widths(c, 1000)
	if w[0]+w[1] > 1000-c.Dp(260) {
		t.Errorf("columns are %v, leaving %d for the body, which has a minimum of 260",
			w, 1000-w[0]-w[1])
	}

	s.Toggle(0)
	if !s.Hidden(0) {
		t.Fatal("toggling did not put the column away")
	}
	if w := s.Widths(c, 1000); w[0] != c.Dp(reef.RailW) {
		t.Errorf("a put-away column is %dpx wide, want a %dpx rail", w[0], c.Dp(reef.RailW))
	}
}

// Wrapping is the caller's business in this system, so the one helper that
// does it has to be exact: a column is a count of cells, not of bytes.
func TestWrapCountsCharacters(t *testing.T) {
	lines := reef.Wrap("the quick brown fox jumps", 10)
	for _, l := range lines {
		if len([]rune(l)) > 10 {
			t.Errorf("%q is wider than the column it was wrapped to", l)
		}
	}
	if strings.Join(lines, " ") != "the quick brown fox jumps" {
		t.Errorf("wrapping changed the text: %q", lines)
	}

	// A word longer than the column is cut by character, so a multi-byte rune
	// is never split in half.
	long := reef.Wrap(strings.Repeat("é", 25), 10)
	if len(long) != 3 {
		t.Fatalf("a 25 character word wrapped to %d lines of 10, want 3", len(long))
	}
	for _, l := range long {
		if strings.ContainsRune(l, '�') {
			t.Errorf("wrapping cut a character in half: %q", l)
		}
	}

	// A paragraph break survives, so a block of text can be measured by
	// counting what comes back.
	if got := reef.Wrap("one\n\ntwo", 10); len(got) != 3 || got[1] != "" {
		t.Errorf("a blank line did not survive wrapping: %q", got)
	}
}

// The type scale is anchored on the interface size, so setting it moves every
// step of the scale together and the interface keeps its proportions.
func TestSizeAnchorsTheScale(t *testing.T) {
	var ops op.Ops
	th := reef.NewTheme()
	small := th.Row(th.Sized(gtx(&ops, input.Source{}, 100, 100)))

	th.SetSize(reef.MaxFontSize + 10)
	if th.Size != reef.MaxFontSize {
		t.Errorf("the interface size is %v, want it clamped to %v", th.Size, reef.MaxFontSize)
	}
	if large := th.Row(th.Sized(gtx(&ops, input.Source{}, 100, 100))); large <= small {
		t.Errorf("a row is %dpx at %v and was %dpx at the default; it should have grown",
			large, th.Size, small)
	}

	th.SetSize(0)
	if th.Size != reef.SizeUI || th.Scale != 1 {
		t.Errorf("zero left the scale at %v (×%v), want the scale's own %v", th.Size, th.Scale, reef.SizeUI)
	}
}

// A family a typeface cannot name is a family a picker must not offer.
func TestUsableFamily(t *testing.T) {
	for _, name := range []string{"", ".SF NS Mono", "Comma, Mono"} {
		if reef.UsableFamily(name) {
			t.Errorf("%q was offered, and a typeface cannot carry it", name)
		}
	}
	if !reef.UsableFamily("IBM Plex Mono") {
		t.Error("IBM Plex Mono was rejected")
	}
}
