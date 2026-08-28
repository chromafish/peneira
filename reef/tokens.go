package reef

import "image/color"

// The token layer, in two levels: a Ramp holds the raw steps, and a Palette
// holds the aliases everything is drawn with. Nothing outside this file names
// a step. That is what lets an interface invert by swapping one struct.

// Ramp is the base palette: two neutral ramps and four accent families. The
// light neutrals are a green-grey rather than white, and carry the same green
// undertone as the greens.
//
// Only some steps of the accent families are defined. The gaps are left as
// zero values, so indexing one gives a visibly wrong colour rather than a
// plausible one.
type Ramp struct {
	// Page is the page colour and Raised the surface panels, fields and
	// floating sheets are drawn on. Left zero they come from Paper[1] and
	// Paper[0]; set them when the surface is outside the ramp.
	Page   color.NRGBA
	Raised color.NRGBA

	Paper [5]color.NRGBA // paper-0…4: raised, page, sunken, hairline, hard rule
	Ink   [5]color.NRGBA // ink-0…4: strong text down to faint
	Moss  [7]color.NRGBA // moss-0…6: the brand green, every primary action
	Olive [7]color.NRGBA // olive-1…6: the lichen accent (0 undefined)
	Amber [7]color.NRGBA // amber-2,3,4,6: warning only
	Rust  [7]color.NRGBA // rust-2,3,4,6: danger and removed lines only
	Teal  [7]color.NRGBA // teal-2,3,4,6: advisory only
	Clay  color.NRGBA    // clay-3: the single step the system defines
}

// LightRamp is the default; the system is tuned light first.
var LightRamp = Ramp{
	Page:   Hex(0xE7EBDF),
	Raised: Hex(0xF1F4EA),
	Paper:  [5]color.NRGBA{Hex(0xF1F4EA), Hex(0xE7EBDF), Hex(0xD9DECC), Hex(0xC3CAB3), Hex(0x99A489)},
	Ink:    [5]color.NRGBA{Hex(0x101410), Hex(0x1C211B), Hex(0x343B32), Hex(0x576053), Hex(0x828B7C)},
	Moss:   [7]color.NRGBA{Hex(0x16301F), Hex(0x254A2F), Hex(0x2F5D3A), Hex(0x3C7549), Hex(0x5E9A68), Hex(0x9BC4A1), Hex(0xCFE3C9)},
	Olive:  [7]color.NRGBA{{}, Hex(0x4E5722), Hex(0x6B7730), Hex(0x7C8A3C), Hex(0xA5B25C), Hex(0xCDD69E), Hex(0xE2E9BA)},
	Amber:  [7]color.NRGBA{{}, {}, Hex(0x7A4E0C), Hex(0xB5761F), Hex(0xD79A3C), {}, Hex(0xF0D8A0)},
	Rust:   [7]color.NRGBA{{}, {}, Hex(0x78270F), Hex(0xA33B1F), Hex(0xC9613F), {}, Hex(0xF1CCBC)},
	Teal:   [7]color.NRGBA{{}, {}, Hex(0x123F3C), Hex(0x24625E), Hex(0x4E938D), {}, Hex(0xBFDDD9)},
	Clay:   Hex(0x8A5A3B),
}

// BoneRamp is LightRamp with near-white neutrals.
var BoneRamp = boneRamp()

func boneRamp() Ramp {
	r := LightRamp
	r.Page = Hex(0xF4F5F2)
	r.Raised = Hex(0xFBFCFA)
	r.Paper = [5]color.NRGBA{Hex(0xFBFCFA), Hex(0xF4F5F2), Hex(0xE8EAE6), Hex(0xD3D6D0), Hex(0xA6ABA3)}
	return r
}

// DarkRamp keeps every token name and swaps the values for terminal ones:
// near-black paper and phosphor greens. Because the ramps invert step for
// step, every semantic alias below is written once and holds in all three.
var DarkRamp = Ramp{
	Page:   Hex(0x0C0F0B),
	Raised: Hex(0x151912),
	Paper:  [5]color.NRGBA{Hex(0x151912), Hex(0x1B2017), Hex(0x242A1E), Hex(0x333B29), Hex(0x4A533C)},
	Ink:    [5]color.NRGBA{Hex(0xE6E9DC), Hex(0xD3D8C6), Hex(0xAAB29A), Hex(0x8C947C), Hex(0x656D57)},
	Moss:   [7]color.NRGBA{Hex(0xDCE8DB), Hex(0x9BC4A1), Hex(0x6FAE79), Hex(0x589F63), Hex(0x3F7A4B), Hex(0x28502F), Hex(0x16301F)},
	Olive:  [7]color.NRGBA{{}, Hex(0xEDEFD8), Hex(0xC3CE86), Hex(0xA8B85C), Hex(0x7C8A3C), Hex(0x454E1F), Hex(0x252A11)},
	Amber:  [7]color.NRGBA{{}, {}, Hex(0xF0D6A4), Hex(0xD79A3C), Hex(0xB5761F), {}, Hex(0x3A2A0D)},
	Rust:   [7]color.NRGBA{{}, {}, Hex(0xF0CBBE), Hex(0xD46A4C), Hex(0xA33B1F), {}, Hex(0x3A1A10)},
	Teal:   [7]color.NRGBA{{}, {}, Hex(0xCDE3E1), Hex(0x59A8A0), Hex(0x24625E), {}, Hex(0x0F2C2A)},
	Clay:   Hex(0xB98763),
}

// Palette is the semantic layer: the only colours an interface should name.
// Everything in the package draws from these fields.
type Palette struct {
	// Surfaces. At most two background tints should appear in any one view.
	Bg       color.NRGBA // the page
	BgAlt    color.NRGBA // the raised plane: a panel, a field, a sheet
	BgSunken color.NRGBA // headers, wells, strips
	Hover    color.NRGBA // one paper step darker, under the pointer
	Active   color.NRGBA // one step darker again, while pressed
	BgInvert color.NRGBA // the inverse surface

	// Text.
	Strong   color.NRGBA // headings and the thing being looked at
	Fg       color.NRGBA // body
	Muted    color.NRGBA // secondary
	Faint    color.NRGBA // labels, units, things read only when looked for
	FgInvert color.NRGBA // text on an inverted surface

	// Rules. Hairlines do all the structural work in this system, so there is
	// no shadow, gradient or gap doing it instead.
	RuleFaint  color.NRGBA // between rows, inside panels
	Rule       color.NRGBA // between panels and under headers
	RuleStrong color.NRGBA // around controls and floating things
	RuleAccent color.NRGBA // the 2px selected edge

	// Action. Moss is the brand green and every primary action; there is no
	// blue anywhere in the system.
	Action       color.NRGBA
	ActionHover  color.NRGBA
	ActionActive color.NRGBA
	ActionFg     color.NRGBA // text on a filled action
	Accent       color.NRGBA // the olive highlight

	// Status. Pair each with a word or a glyph at the point of use.
	Ok          color.NRGBA
	OkBg        color.NRGBA
	OkLine      color.NRGBA
	Warn        color.NRGBA
	WarnBg      color.NRGBA
	WarnLine    color.NRGBA
	Error       color.NRGBA
	ErrorBg     color.NRGBA
	ErrorLine   color.NRGBA
	Info        color.NRGBA
	InfoBg      color.NRGBA
	InfoLine    color.NRGBA
	Neutral     color.NRGBA
	NeutralBg   color.NRGBA
	NeutralLine color.NRGBA

	// Selection: a tint plus a 2px accent edge. A pane without the keyboard
	// uses the idle pair.
	SelBg       color.NRGBA
	SelEdge     color.NRGBA
	SelIdleBg   color.NRGBA
	SelIdleEdge color.NRGBA

	// Diff. Tint plus a 2px gutter, and the sign column beside it.
	AddBg     color.NRGBA
	AddGutter color.NRGBA
	AddFg     color.NRGBA
	DelBg     color.NRGBA
	DelGutter color.NRGBA
	DelFg     color.NRGBA

	// The intraline runs: a second, hotter step for marking what actually
	// changed inside a line whose whole row is already tinted.
	AddBgHot color.NRGBA
	DelBgHot color.NRGBA

	// The strip that separates two runs of a comparison.
	HunkBg color.NRGBA
	HunkFg color.NRGBA

	TextSel color.NRGBA // the tint behind selected text
	Focus   color.NRGBA // the focus ring
	GridDot color.NRGBA // the 8px desk grid

	// Syntax colours source text. The accent families are spent on status, so
	// syntax borrows them at their darkest step, where they read as weight on
	// the page rather than as a state. Index it with a Syntax constant.
	Syntax [SyntaxRoles]color.NRGBA
}

// Syntax is a coarse token class for colouring source text. The set is
// deliberately small: a system defines a handful of colours rather than
// mirroring a lexer's full token tree, and a highlighter maps its own classes
// onto these.
type Syntax uint8

const (
	SyntaxPlain Syntax = iota
	SyntaxKeyword
	SyntaxName
	SyntaxFunction
	SyntaxType
	SyntaxString
	SyntaxNumber
	SyntaxComment
	SyntaxOperator
	SyntaxPunctuation
	SyntaxPreproc
	SyntaxError

	// SyntaxRoles is the number of classes, and the length of Palette.Syntax.
	SyntaxRoles = iota
)

// ColorNRGBA is an alias kept short because colours are threaded through
// nearly every drawing helper in this package and in the code that uses it.
type ColorNRGBA = color.NRGBA

// Hex builds an opaque colour from a 0xRRGGBB literal, which is how the tokens
// above are written and how an application should write any colour of its own.
func Hex(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

// Mix blends a towards b by t, in the range 0 to 1. It exists for the derived
// diff tints and is deliberately not a general tinting facility: everything
// else in the system is a token, not a calculation.
func Mix(a, b color.NRGBA, t float32) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return color.NRGBA{R: f(a.R, b.R), G: f(a.G, b.G), B: f(a.B, b.B), A: 0xff}
}

// Palette builds the alias layer from the ramp. Every theme the system ships
// goes through this one method, which is what keeps them the same design. An
// application with colours of its own should write a Ramp and call this,
// rather than filling in a Palette by hand. Base16.Palette is the other way
// in.
func (r Ramp) Palette() Palette {
	page, raised := r.Page, r.Raised
	if page.A == 0 {
		page = r.Paper[1]
	}
	if raised.A == 0 {
		raised = r.Paper[0]
	}
	return Palette{
		Bg:       page,
		BgAlt:    raised,
		BgSunken: r.Paper[2],
		Hover:    r.Paper[2],
		Active:   r.Paper[3],
		BgInvert: r.Ink[0],

		Strong:   r.Ink[0],
		Fg:       r.Ink[1],
		Muted:    r.Ink[3],
		Faint:    r.Ink[4],
		FgInvert: raised,

		RuleFaint:  r.Paper[3],
		Rule:       r.Paper[4],
		RuleStrong: r.Ink[2],
		RuleAccent: r.Moss[3],

		Action:       r.Moss[2],
		ActionHover:  r.Moss[1],
		ActionActive: r.Moss[0],
		ActionFg:     raised,
		Accent:       r.Olive[2],

		Ok:          r.Moss[2],
		OkBg:        r.Moss[6],
		OkLine:      r.Moss[3],
		Warn:        r.Amber[2],
		WarnBg:      r.Amber[6],
		WarnLine:    r.Amber[3],
		Error:       r.Rust[2],
		ErrorBg:     r.Rust[6],
		ErrorLine:   r.Rust[3],
		Info:        r.Teal[2],
		InfoBg:      r.Teal[6],
		InfoLine:    r.Teal[3],
		Neutral:     r.Ink[3],
		NeutralBg:   r.Paper[2],
		NeutralLine: r.Paper[4],

		SelBg:       r.Moss[6],
		SelEdge:     r.Moss[3],
		SelIdleBg:   r.Paper[2],
		SelIdleEdge: r.Paper[4],

		AddBg:     r.Moss[6],
		AddGutter: r.Moss[3],
		AddFg:     r.Moss[2],
		DelBg:     r.Rust[6],
		DelGutter: r.Rust[3],
		DelFg:     r.Rust[2],

		AddBgHot: Mix(r.Moss[6], r.Moss[4], 0.45),
		DelBgHot: Mix(r.Rust[6], r.Rust[4], 0.35),

		HunkBg: r.Paper[2],
		HunkFg: r.Ink[3],

		TextSel: r.Moss[5],
		Focus:   r.Moss[3],
		GridDot: r.Paper[3],

		Syntax: [SyntaxRoles]color.NRGBA{
			SyntaxPlain:       r.Ink[1],
			SyntaxKeyword:     r.Rust[2],
			SyntaxName:        r.Ink[1],
			SyntaxFunction:    r.Clay,
			SyntaxType:        r.Teal[2],
			SyntaxString:      r.Moss[2],
			SyntaxNumber:      r.Amber[2],
			SyntaxComment:     r.Ink[4],
			SyntaxOperator:    r.Ink[3],
			SyntaxPunctuation: r.Ink[4],
			SyntaxPreproc:     r.Olive[2],
			SyntaxError:       r.Rust[2],
		},
	}
}

// The themes the system ships.
var (
	Light = LightRamp.Palette()
	Bone  = BoneRamp.Palette()
	Dark  = DarkRamp.Palette()
)
