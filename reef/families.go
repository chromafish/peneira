package reef

import (
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	sfnt "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/fontscan"
)

// Letting a person choose the typeface means finding out what there is to
// choose from. Gio's shaper already resolves a typeface by name against the
// system fonts, so nothing here loads a font: this only reads enough of each
// file to answer two questions — what is this family called, and is it fixed
// pitch — and hands the names back for a picker to show and Theme.SetFamily to
// take.

// The one table that has to be read to answer the second question. Reading it
// alone is what keeps a scan of a few hundred files down to a quarter of a
// second: parsing each font in full, which is what asking the shaping library
// the same question would do, reads the outlines and the character map of
// every face on the machine and costs orders of magnitude more.
var tagPost = opentype.MustNewTag("post")

// isFixedPitch reports whether a face declares itself monospaced, which is the
// post table's own flag. Fonts that leave the flag unset can still be
// monospaced in fact, and the shaping library will work that out by comparing
// every glyph's advance — but that means loading the whole face, and the
// families anyone would actually want all declare themselves properly.
func isFixedPitch(ld *opentype.Loader) bool {
	raw, err := ld.RawTable(tagPost)
	// version, italicAngle, underlinePosition, underlineThickness, then the
	// flag: 16 bytes in, whatever the table's version.
	if err != nil || len(raw) < 16 {
		return false
	}
	return binary.BigEndian.Uint32(raw[12:16]) != 0
}

// noLogger swallows the font scanner's warnings. A font file that cannot be
// read is not something a person can act on, and the scan simply carries on to
// the next one.
type noLogger struct{}

func (noLogger) Printf(string, ...interface{}) {}

// MonoFamilies returns the monospaced typefaces installed on this machine,
// named as they name themselves and ordered for reading. Every name it returns
// is one Theme.SetFamily will take.
//
// It walks the font directories rather than consulting an index, so it takes
// long enough to be worth doing once, off the main goroutine, and keeping the
// answer:
//
//	go func() {
//		found := reef.MonoFamilies()
//		w.Run(func() { app.families = found })   // back on the loop
//	}()
func MonoFamilies() []string {
	dirs, err := fontscan.DefaultFontDirectories(noLogger{})
	if err != nil {
		return nil
	}

	seen := map[string]bool{}
	var out []string
	var buf []byte

	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				// A directory that cannot be read is skipped rather than
				// abandoning the walk: one unreadable font folder should not
				// empty the picker.
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".ttf", ".ttc", ".otf", ".otc":
			default:
				return nil
			}
			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()

			// One file can hold several faces — a collection, or the weights
			// of one family — and each is asked separately.
			loaders, err := opentype.NewLoaders(f)
			if err != nil {
				return nil
			}
			for _, ld := range loaders {
				if !isFixedPitch(ld) {
					continue
				}
				var desc sfnt.Description
				desc, buf = sfnt.Describe(ld, buf)
				name := strings.TrimSpace(desc.Family)
				if !UsableFamily(name) {
					continue
				}
				key := strings.ToLower(name)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, name)
			}
			return nil
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

// UsableFamily reports whether a family name can be offered to a person and
// passed to Theme.SetFamily. It rejects the ones a picker has no business
// showing: the empty name, the ones a system hides behind a leading dot, and
// the ones containing a comma, which a typeface is read as a list of families
// and so cannot carry.
func UsableFamily(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || strings.Contains(name, ",") {
		return false
	}
	return true
}
