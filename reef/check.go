package reef

import (
	"fmt"
	"image/color"
	"reflect"
)

// A palette that came from outside — a base16 file a person downloaded, a Ramp
// someone wrote — can be legal Go and still unreadable. Check looks for the
// two ways that happens: an alias nobody filled in, and a pair of colours that
// are used together but cannot be told apart.

// Severity says whether a problem makes the palette unusable or only worth
// looking at.
type Severity int

const (
	// Warning: legible, but tighter than it should be.
	Warning Severity = iota
	// Fault: an alias that will draw nothing, or text that cannot be read.
	Fault
)

func (s Severity) String() string {
	if s == Fault {
		return "fault"
	}
	return "warning"
}

// A Problem is one thing wrong with a palette.
type Problem struct {
	Alias    string
	Severity Severity
	Message  string
	// Ratio is the contrast the pair achieved, where the problem is contrast,
	// and 0 where it is not.
	Ratio float64
}

func (p Problem) String() string {
	if p.Ratio > 0 {
		return fmt.Sprintf("%s: %s (%s, %.1f:1)", p.Alias, p.Message, p.Severity, p.Ratio)
	}
	return fmt.Sprintf("%s: %s (%s)", p.Alias, p.Message, p.Severity)
}

// Contrast returns the WCAG contrast ratio between two colours, from 1 (the
// same colour) to 21 (black on white). 4.5 is the threshold for body text and
// 3 for large or secondary text.
func Contrast(a, b color.NRGBA) float64 { return contrast(a, b) }

// Check reports what is wrong with a palette: aliases left transparent, and
// pairs of colours that are drawn on each other without enough contrast to be
// read. It is meant for palettes that came from outside — run it when a scheme
// is loaded and show what it says beside the scheme's name.
//
// It returns nil for a palette with nothing wrong with it. The thresholds are
// WCAG's: 4.5 for text, 3 for the secondary text and the marks that only have
// to be distinguishable.
func (p Palette) Check() []Problem {
	var out []Problem

	// Every alias has to be opaque. A zero value is the commonest mistake in a
	// hand-written Ramp, and it draws nothing at all.
	for _, r := range paletteRoles(p) {
		if r.c.A == 0 {
			out = append(out, Problem{
				Alias:    r.name,
				Severity: Fault,
				Message:  "is transparent, and will draw nothing",
			})
		}
	}

	pairs := []struct {
		alias    string
		fg, bg   color.NRGBA
		want     float64
		severity Severity
	}{
		{"Fg on Bg", p.Fg, p.Bg, 4.5, Fault},
		{"Strong on Bg", p.Strong, p.Bg, 4.5, Fault},
		{"Fg on BgAlt", p.Fg, p.BgAlt, 4.5, Warning},
		{"Muted on Bg", p.Muted, p.Bg, 3, Warning},
		{"Faint on BgSunken", p.Faint, p.BgSunken, 2.5, Warning},
		{"ActionFg on Action", p.ActionFg, p.Action, 4.5, Warning},
		{"Action on Bg", p.Action, p.Bg, 3, Warning},
		{"Error on Bg", p.Error, p.Bg, 3, Warning},
		{"Warn on Bg", p.Warn, p.Bg, 3, Warning},
		{"Ok on Bg", p.Ok, p.Bg, 3, Warning},
		{"Fg on SelBg", p.Fg, p.SelBg, 4.5, Warning},
		{"AddFg on AddBg", p.AddFg, p.AddBg, 3, Warning},
		{"DelFg on DelBg", p.DelFg, p.DelBg, 3, Warning},
		{"Fg on AddBg", p.Fg, p.AddBg, 4.5, Warning},
		{"Fg on DelBg", p.Fg, p.DelBg, 4.5, Warning},
	}
	for _, pair := range pairs {
		if got := contrast(pair.fg, pair.bg); got < pair.want {
			out = append(out, Problem{
				Alias:    pair.alias,
				Severity: pair.severity,
				Message:  fmt.Sprintf("wants %.1f:1", pair.want),
				Ratio:    got,
			})
		}
	}

	// The rules carry the structure, so one that cannot be seen against the
	// surface it is drawn on takes the structure with it.
	for _, rule := range []struct {
		alias string
		c, bg color.NRGBA
	}{
		{"RuleFaint on Bg", p.RuleFaint, p.Bg},
		{"Rule on Bg", p.Rule, p.Bg},
	} {
		if got := contrast(rule.c, rule.bg); got < 1.15 {
			out = append(out, Problem{
				Alias:    rule.alias,
				Severity: Warning,
				Message:  "is invisible against the surface it separates",
				Ratio:    got,
			})
		}
	}
	return out
}

// Check runs Check over both modes of a scheme, naming which mode each problem
// came from.
func (s Scheme) Check() []Problem {
	var out []Problem
	for _, mode := range []struct {
		name string
		p    *Palette
	}{{"light", s.Light}, {"dark", s.Dark}} {
		if mode.p == nil {
			continue
		}
		for _, problem := range mode.p.Check() {
			problem.Alias = mode.name + " " + problem.Alias
			out = append(out, problem)
		}
	}
	return out
}

// role is one named colour of a palette.
type role struct {
	name string
	c    color.NRGBA
}

// paletteRoles walks the Palette struct, so an alias added to the system is
// checked without anyone remembering to add it here.
func paletteRoles(p Palette) []role {
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
				out = append(out, role{fmt.Sprintf("%s[%d]", f.Name, j), arr.Index(j).Interface().(color.NRGBA)})
			}
		}
	}
	return out
}
