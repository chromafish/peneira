package reef

import (
	"embed"
	"fmt"

	"gioui.org/font"
	"gioui.org/font/opentype"
)

// The system is set in IBM Plex: Plex Mono is the working typeface — data,
// code, labels, controls — and Plex Sans Condensed is display only. Both are
// embedded, so an interface looks the same everywhere and on the first
// frame. They are licensed under the SIL Open Font License; see
// plex/LICENSE.txt, which License returns for a colophon. Plex Serif is not
// carried.
//
//go:embed plex/*.ttf plex/LICENSE.txt
var plexFS embed.FS

// The two typefaces, and the family they are both cut from, which is what a
// colophon calls the interface's type.
const (
	Mono     = font.Typeface("Plex Mono")
	Display  = font.Typeface("Plex Sans Condensed")
	PlexName = "IBM Plex"
)

// Neither family carries a bold cut here, so the semibold ones answer to the
// display weight.
var faces = []struct {
	typeface font.Typeface
	file     string
	weight   font.Weight
	style    font.Style
}{
	{Mono, "plex/IBMPlexMono-Light.ttf", WeightLight, font.Regular},
	{Mono, "plex/IBMPlexMono-Regular.ttf", WeightBody, font.Regular},
	{Mono, "plex/IBMPlexMono-Medium.ttf", WeightLabel, font.Regular},
	{Mono, "plex/IBMPlexMono-SemiBold.ttf", WeightDisplay, font.Regular},
	{Mono, "plex/IBMPlexMono-Italic.ttf", WeightBody, font.Italic},
	{Display, "plex/IBMPlexSansCondensed-Medium.ttf", WeightLabel, font.Regular},
	{Display, "plex/IBMPlexSansCondensed-SemiBold.ttf", WeightDisplay, font.Regular},
}

// Fonts is a loaded collection and the two roles the system sets type in. Pass
// one to NewThemeWith to set an interface in typefaces of your own; the
// collection has to carry the weights of the scale, since the system asks for
// medium and bold by name rather than synthesising them.
type Fonts struct {
	Collection []font.FontFace
	// Name is what a colophon calls the family, e.g. "IBM Plex".
	Name string
	// Mono is the working typeface: data, code, labels, controls.
	Mono font.Typeface
	// Display is reserved for display type. Nothing read as data is set in it.
	Display font.Typeface
}

// LoadFonts parses the embedded faces. A failure here is a build problem, not
// a runtime one: the files are compiled in.
func LoadFonts() Fonts {
	out := make([]font.FontFace, 0, len(faces))
	for _, f := range faces {
		raw, err := plexFS.ReadFile(f.file)
		if err != nil {
			panic(fmt.Errorf("embedded font %s: %w", f.file, err))
		}
		face, err := opentype.Parse(raw)
		if err != nil {
			panic(fmt.Errorf("parsing %s: %w", f.file, err))
		}
		out = append(out, font.FontFace{
			Font: font.Font{Typeface: f.typeface, Weight: f.weight, Style: f.style},
			Face: face,
		})
	}
	return Fonts{Collection: out, Name: PlexName, Mono: Mono, Display: Display}
}

// License returns the licence text of the embedded faces, for a colophon or an
// about box. Shipping the faces means shipping this with them.
func License() string {
	raw, _ := plexFS.ReadFile("plex/LICENSE.txt")
	return string(raw)
}
