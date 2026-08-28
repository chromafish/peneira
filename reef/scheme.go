package reef

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A Scheme is a named set of colours a person can pick: one palette for light,
// one for dark, or just one of the two. It is what a settings list offers, what
// a file on disk parses to, and what Theme.SetScheme puts in force.
//
// Reef ships the two Builtin returns. Everything else comes from a Ramp of
// your own or from a base16 file — see Base16, which is the format most editor
// and terminal colour schemes are already published in.
type Scheme struct {
	// ID is stable and safe to write to a settings file. Name is shown.
	ID   string
	Name string

	// Light and Dark are the two modes. A scheme that defines only one — an
	// imported base16 scheme, say — leaves the other nil, and inverting does
	// nothing while it is in force.
	Light *Palette
	Dark  *Palette
}

// Default is the scheme reef opens on: the moss and paper of the light and
// dark ramps.
func Default() Scheme {
	light, dark := Light, Dark
	return Scheme{ID: "reef", Name: "Reef", Light: &light, Dark: &dark}
}

// BoneScheme is Default with BoneRamp in light mode and the same dark mode.
func BoneScheme() Scheme {
	light, dark := Bone, Dark
	return Scheme{ID: "bone", Name: "Bone", Light: &light, Dark: &dark}
}

// Builtin is the schemes that ship in the binary, in the order a picker should
// offer them.
func Builtin() []Scheme { return []Scheme{Default(), BoneScheme()} }

// Has reports whether the scheme defines a mode.
func (s Scheme) Has(dark bool) bool {
	if dark {
		return s.Dark != nil
	}
	return s.Light != nil
}

// Modes reports how many of the two the scheme defines. A scheme with one mode
// cannot be inverted.
func (s Scheme) Modes() int {
	n := 0
	if s.Light != nil {
		n++
	}
	if s.Dark != nil {
		n++
	}
	return n
}

// palette returns the mode asked for, or the only one there is.
func (s Scheme) palette(dark bool) (Palette, bool) {
	if dark && s.Dark != nil {
		return *s.Dark, true
	}
	if !dark && s.Light != nil {
		return *s.Light, true
	}
	switch {
	case s.Dark != nil:
		return *s.Dark, true
	case s.Light != nil:
		return *s.Light, true
	}
	return Palette{}, false
}

// SetScheme puts a scheme in force, keeping the current mode where the scheme
// has it and moving to the one it does have where it does not.
func (t *Theme) SetScheme(s Scheme) {
	if s.Modes() == 0 {
		return
	}
	t.scheme = s
	if !s.Has(t.Dark) {
		t.Dark = !t.Dark
	}
	p, _ := s.palette(t.Dark)
	t.SetPalette(p)
}

// Scheme returns the scheme in force.
func (t *Theme) Scheme() Scheme { return t.scheme }

// LoadSchemes reads every base16 file in a directory — .yaml, .yml or .json —
// and returns what parsed, sorted by name. Files that do not parse are
// reported in the error; the schemes that did are still returned, so one bad
// file in a directory does not empty a picker.
//
// The ID of each scheme is its file name without the extension.
func LoadSchemes(dir string) ([]Scheme, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var (
		out  []Scheme
		bad  []string
		seen = map[string]bool{}
	)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".yaml", ".yml", ".json":
		default:
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			bad = append(bad, e.Name())
			continue
		}
		b, err := ParseBase16(raw)
		if err != nil {
			bad = append(bad, e.Name()+": "+err.Error())
			continue
		}
		id := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, b.Scheme(id))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	if len(bad) > 0 {
		return out, fmt.Errorf("%d file(s) did not parse: %s", len(bad), strings.Join(bad, "; "))
	}
	return out, nil
}
