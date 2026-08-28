package reef

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
)

// base16 is how most editor and terminal colour schemes are already
// published: sixteen colours with fixed roles, in a two-line-per-colour file.
// There are hundreds of them, and a person who has a scheme they like almost
// certainly has it in this form.
//
// The roles, from the format's own definition:
//
//	base00  background            base08  red      variables, deletions
//	base01  panel background      base09  orange   numbers, constants
//	base02  selection background  base0A  yellow   types, classes
//	base03  comments              base0B  green    strings, insertions
//	base04  secondary text        base0C  cyan     escapes, support
//	base05  body text             base0D  blue     functions, links
//	base06  brighter text         base0E  magenta  keywords
//	base07  brightest text        base0F  brown    deprecated
//
// base00 to base07 always run background to foreground, so the mapping below
// does not care whether a scheme is light or dark; Base16.Dark works that out
// from the two ends.

// Base16 is a parsed base16 scheme.
type Base16 struct {
	Name string
	Base [16]color.NRGBA

	// Accent chooses which of the sixteen drives actions, selection and focus.
	// Zero means base0D, the blue a base16 scheme uses for functions and
	// links. Set it to 0x0B for a green interface, 0x0E for purple, and so on.
	Accent int
}

// ParseBase16 reads a base16 scheme. Both forms the schemes are published in
// are accepted: the YAML of the base16 repositories, and JSON with the same
// keys. Colours may be written as "#RRGGBB" or bare "RRGGBB".
//
// Only the subset of YAML these files use is understood — one "key: value" per
// line, with # starting a comment. That is all a base16 file has ever been.
func ParseBase16(data []byte) (Base16, error) {
	fields := map[string]string{}

	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") {
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return Base16{}, fmt.Errorf("base16 json: %w", err)
		}
		for k, v := range raw {
			if s, ok := v.(string); ok {
				fields[strings.ToLower(k)] = s
			}
		}
	} else {
		for _, line := range strings.Split(string(data), "\n") {
			line = cutComment(line)
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			key = strings.ToLower(strings.TrimSpace(key))
			value = strings.TrimSpace(value)
			value = strings.Trim(value, `"'`)
			if key != "" && value != "" {
				fields[key] = value
			}
		}
	}

	var b Base16
	b.Name = firstOf(fields, "scheme", "name")
	for i := range b.Base {
		key := fmt.Sprintf("base%02X", i)
		v, ok := fields[strings.ToLower(key)]
		if !ok {
			return Base16{}, fmt.Errorf("base16: %s is missing", key)
		}
		c, err := parseHex(v)
		if err != nil {
			return Base16{}, fmt.Errorf("base16: %s: %w", key, err)
		}
		b.Base[i] = c
	}
	if b.Name == "" {
		b.Name = "Untitled"
	}
	return b, nil
}

// cutComment drops a YAML comment: the first # that is not inside quotes. The
// files write colours both as "282828" # a note and as "#282828", so the
// quotes have to be tracked rather than assumed.
func cutComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return line[:i]
		}
	}
	return line
}

func firstOf(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := m[k]; v != "" {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

func parseHex(s string) (color.NRGBA, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	if len(s) != 6 {
		return color.NRGBA{}, fmt.Errorf("%q is not a six digit colour", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("%q is not a colour", s)
	}
	return Hex(uint32(v)), nil
}

// Dark reports whether the scheme is a dark one, by comparing the background
// with the body text.
func (b Base16) Dark() bool {
	return luminance(b.Base[0x00]) < luminance(b.Base[0x05])
}

// Scheme wraps the palette with an identity, ready for a picker or for
// Theme.SetScheme. Only the mode the scheme actually is gets filled in, so
// inverting a base16 scheme does nothing.
func (b Base16) Scheme(id string) Scheme {
	p := b.Palette()
	s := Scheme{ID: id, Name: b.Name}
	if b.Dark() {
		s.Dark = &p
	} else {
		s.Light = &p
	}
	return s
}

// Palette maps the sixteen colours onto reef's aliases.
//
// Where reef wants a step base16 does not have — the tint behind a selected
// row, the two hairline weights, a hover one step along — the colour is mixed
// from the ones it does have. Mixing towards the background gives a tint and
// towards the text gives emphasis, which holds whichever way round the scheme
// is.
func (b Base16) Palette() Palette {
	var (
		bg    = b.Base[0x00]
		panel = b.Base[0x01]
		sel   = b.Base[0x02]
		faint = b.Base[0x03]
		muted = b.Base[0x04]
		body  = b.Base[0x05]
		// base06 and base07 are the bright end, but not every scheme puts the
		// brightest there — Nord's base07 is one of its frost colours — so the
		// strong text is whichever of the three reads best on the background.
		strong = furthest(bg, b.Base[0x05], b.Base[0x06], b.Base[0x07])

		red     = b.Base[0x08]
		orange  = b.Base[0x09]
		yellow  = b.Base[0x0A]
		green   = b.Base[0x0B]
		cyan    = b.Base[0x0C]
		blue    = b.Base[0x0D]
		magenta = b.Base[0x0E]
		brown   = b.Base[0x0F]
	)
	accent := blue
	if b.Accent > 0 && b.Accent < len(b.Base) {
		accent = b.Base[b.Accent]
	}

	// tint puts a colour behind text as a wash on the page; emphasis pushes
	// one further from the page.
	tint := func(c color.NRGBA, t float32) color.NRGBA { return Mix(bg, c, t) }
	emphasis := func(c color.NRGBA, t float32) color.NRGBA { return Mix(c, strong, t) }

	return Palette{
		Bg:       bg,
		BgAlt:    panel,
		BgSunken: Mix(panel, sel, 0.5),
		Hover:    Mix(bg, sel, 0.5),
		Active:   sel,
		BgInvert: body,

		Strong:   strong,
		Fg:       body,
		Muted:    muted,
		Faint:    faint,
		FgInvert: bg,

		RuleFaint:  Mix(bg, faint, 0.35),
		Rule:       Mix(bg, faint, 0.75),
		RuleStrong: muted,
		RuleAccent: accent,

		Action:       accent,
		ActionHover:  emphasis(accent, 0.25),
		ActionActive: emphasis(accent, 0.45),
		ActionFg:     bg,
		Accent:       magenta,

		Ok:          green,
		OkBg:        tint(green, 0.18),
		OkLine:      green,
		Warn:        yellow,
		WarnBg:      tint(yellow, 0.18),
		WarnLine:    yellow,
		Error:       red,
		ErrorBg:     tint(red, 0.18),
		ErrorLine:   red,
		Info:        cyan,
		InfoBg:      tint(cyan, 0.18),
		InfoLine:    cyan,
		Neutral:     muted,
		NeutralBg:   Mix(bg, sel, 0.7),
		NeutralLine: Mix(bg, faint, 0.75),

		SelBg:       tint(accent, 0.16),
		SelEdge:     accent,
		SelIdleBg:   Mix(bg, sel, 0.8),
		SelIdleEdge: muted,

		AddBg:     tint(green, 0.15),
		AddGutter: green,
		AddFg:     green,
		DelBg:     tint(red, 0.15),
		DelGutter: red,
		DelFg:     red,

		AddBgHot: tint(green, 0.32),
		DelBgHot: tint(red, 0.32),

		HunkBg: Mix(panel, sel, 0.5),
		HunkFg: muted,

		TextSel: tint(accent, 0.3),
		Focus:   accent,
		GridDot: Mix(bg, faint, 0.4),

		Syntax: [SyntaxRoles]color.NRGBA{
			SyntaxPlain:       body,
			SyntaxKeyword:     magenta,
			SyntaxName:        body,
			SyntaxFunction:    blue,
			SyntaxType:        yellow,
			SyntaxString:      green,
			SyntaxNumber:      orange,
			SyntaxComment:     faint,
			SyntaxOperator:    body,
			SyntaxPunctuation: muted,
			SyntaxPreproc:     cyan,
			SyntaxError:       brown,
		},
	}
}

// furthest returns whichever candidate has the most contrast against bg.
func furthest(bg color.NRGBA, candidates ...color.NRGBA) color.NRGBA {
	best, ratio := bg, 0.0
	for _, c := range candidates {
		if r := contrast(c, bg); r > ratio {
			best, ratio = c, r
		}
	}
	return best
}

// luminance is the WCAG relative luminance of a colour, used to tell a light
// scheme from a dark one and to check contrast.
func luminance(c color.NRGBA) float64 {
	f := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

// contrast is the WCAG contrast ratio between two colours, from 1 to 21.
func contrast(a, b color.NRGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}
