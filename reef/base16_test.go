package reef_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chromafish/check/reef"
)

// Three schemes people actually use, in the form they are published in. Nord
// is here because its base07 is a frost colour rather than the brightest text,
// which is the case a naive mapping gets wrong.
const (
	gruvboxDark = `scheme: "Gruvbox dark, medium"
author: "Dawid Kurek"
base00: "282828" # ----
base01: "3c3836" # ---
base02: "504945" # --
base03: "665c54" # -
base04: "bdae93" # +
base05: "d5c4a1" # ++
base06: "ebdbb2" # +++
base07: "fbf1c7" # ++++
base08: "fb4934"
base09: "fe8019"
base0A: "fabd2f"
base0B: "b8bb26"
base0C: "8ec07c"
base0D: "83a598"
base0E: "d3869b"
base0F: "d65d0e"
`
	solarizedLight = `scheme: "Solarized Light"
base00: "fdf6e3"
base01: "eee8d5"
base02: "93a1a1"
base03: "839496"
base04: "657b83"
base05: "586e75"
base06: "073642"
base07: "002b36"
base08: "dc322f"
base09: "cb4b16"
base0A: "b58900"
base0B: "859900"
base0C: "2aa198"
base0D: "268bd2"
base0E: "6c71c4"
base0F: "d33682"
`
	nordJSON = `{
	"scheme": "Nord",
	"base00": "#2e3440", "base01": "#3b4252", "base02": "#434c5e", "base03": "#4c566a",
	"base04": "#d8dee9", "base05": "#e5e9f0", "base06": "#eceff4", "base07": "#8fbcbb",
	"base08": "#bf616a", "base09": "#d08770", "base0A": "#ebcb8b", "base0B": "#a3be8c",
	"base0C": "#88c0d0", "base0D": "#81a1c1", "base0E": "#b48ead", "base0F": "#5e81ac"
}`
)

func parse(t *testing.T, src string) reef.Base16 {
	t.Helper()
	b, err := reef.ParseBase16([]byte(src))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	return b
}

// The two forms these schemes are published in both parse, comments and
// quoting and all, and the scheme knows which way round it is.
func TestParseBase16(t *testing.T) {
	for _, tc := range []struct {
		src  string
		name string
		dark bool
	}{
		{gruvboxDark, "Gruvbox dark, medium", true},
		{solarizedLight, "Solarized Light", false},
		{nordJSON, "Nord", true},
	} {
		b := parse(t, tc.src)
		if b.Name != tc.name {
			t.Errorf("scheme is named %q, want %q", b.Name, tc.name)
		}
		if b.Dark() != tc.dark {
			t.Errorf("%s: Dark() is %v, want %v", b.Name, b.Dark(), tc.dark)
		}
		if b.Base[0x00].A != 0xff || b.Base[0x0F].A != 0xff {
			t.Errorf("%s: colours came back transparent", b.Name)
		}
	}
}

// A scheme someone downloaded has to produce a palette that is filled in and
// readable, or the mapping is not doing its job.
func TestBase16PalettesAreUsable(t *testing.T) {
	for _, src := range []string{gruvboxDark, solarizedLight, nordJSON} {
		b := parse(t, src)
		p := b.Palette()

		for _, problem := range p.Check() {
			if problem.Severity == reef.Fault {
				t.Errorf("%s: %s", b.Name, problem)
			}
		}
		if got := reef.Contrast(p.Fg, p.Bg); got < 4.5 {
			t.Errorf("%s: body text on the page is %.1f:1, want 4.5", b.Name, got)
		}
		// The mapping derives the surface steps, so they have to come out
		// distinct or a panel is invisible against the page.
		if p.Bg == p.BgAlt || p.BgAlt == p.BgSunken {
			t.Errorf("%s: the surface steps collapsed", b.Name)
		}
		if p.SelBg == p.Hover {
			t.Errorf("%s: a selected row and a hovered one are the same colour", b.Name)
		}
	}
}

// Nord puts a frost colour in base07, so the strong text has to be chosen by
// contrast rather than by position.
func TestStrongTextIsTheReadableOne(t *testing.T) {
	p := parse(t, nordJSON).Palette()
	if got := reef.Contrast(p.Strong, p.Bg); got < reef.Contrast(p.Fg, p.Bg) {
		t.Errorf("strong text is %.1f:1 against the page and body text is %.1f:1; "+
			"strong should be at least as readable", got, reef.Contrast(p.Fg, p.Bg))
	}
}

// The accent is what a dev will want to change first.
func TestAccentPicksTheActionColour(t *testing.T) {
	b := parse(t, gruvboxDark)
	if got := b.Palette().Action; got != b.Base[0x0D] {
		t.Errorf("the default action is %v, want base0D %v", got, b.Base[0x0D])
	}
	b.Accent = 0x0B
	if got := b.Palette().Action; got != b.Base[0x0B] {
		t.Errorf("with Accent set the action is %v, want base0B %v", got, b.Base[0x0B])
	}
}

func TestParseBase16Rejects(t *testing.T) {
	for _, src := range []string{
		"scheme: \"Half\"\nbase00: \"282828\"\n",
		"scheme: \"Bad colour\"\n" + strings.Repeat("base00: \"zzzzzz\"\n", 1),
		"",
	} {
		if _, err := reef.ParseBase16([]byte(src)); err == nil {
			t.Errorf("%q parsed, want an error", src)
		}
	}
}

// A scheme with one mode cannot be inverted, and saying so is better than
// throwing the person's colours away when they press the key.
func TestSingleModeSchemeDoesNotInvert(t *testing.T) {
	th := reef.NewTheme()
	s := parse(t, gruvboxDark).Scheme("gruvbox")
	if s.Modes() != 1 || !s.Has(true) {
		t.Fatalf("gruvbox dark came back with %d modes", s.Modes())
	}

	th.SetScheme(s)
	if !th.Dark {
		t.Error("a dark scheme did not put the theme in dark mode")
	}
	was := th.P
	th.ToggleDark()
	if th.P != was {
		t.Error("inverting a one-mode scheme changed the palette")
	}

	// The shipped scheme has both, and still inverts.
	th.SetScheme(reef.Default())
	th.ToggleDark()
	if th.P != reef.Dark && th.P != reef.Light {
		t.Error("the default scheme stopped inverting")
	}
}

// A directory of scheme files is what a dev tool will actually read.
func TestLoadSchemes(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("gruvbox-dark.yaml", gruvboxDark)
	write("solarized-light.yml", solarizedLight)
	write("nord.json", nordJSON)
	write("notes.txt", "not a scheme")
	write("broken.yaml", "scheme: \"Broken\"\nbase00: \"nope\"\n")

	schemes, err := reef.LoadSchemes(dir)
	if err == nil {
		t.Error("a file that does not parse was not reported")
	}
	if len(schemes) != 3 {
		t.Fatalf("loaded %d schemes, want the 3 good ones", len(schemes))
	}
	if schemes[0].Name != "Gruvbox dark, medium" {
		t.Errorf("schemes are not sorted by name: %q first", schemes[0].Name)
	}
	for _, s := range schemes {
		if s.ID == "" || s.Modes() == 0 {
			t.Errorf("%q loaded with no id or no mode", s.Name)
		}
	}
}
