package state

import "path/filepath"

// Settings are preferences, as opposed to the review of any particular
// change: they belong to the person, so they are kept once,
// beside the recent list, rather than per repository.
//
// Every field is optional, and a zero value means "whatever the design system
// says". Nothing here is required for the application to start, so a missing
// or unreadable file is not an error — it is the defaults.
type Settings struct {
	// FontFamily is the monospaced typeface the interface is set in. Empty
	// means the face that ships in the binary.
	FontFamily string `json:"font_family,omitempty"`
	// FontSize is the body size in points, which the whole type scale is
	// anchored on. Zero means the scale's own body size.
	FontSize int `json:"font_size,omitempty"`
	// Theme is the id of the colour scheme, which is the file name of a
	// scheme in ThemesDir. Empty means the one that ships in the binary.
	Theme string `json:"theme,omitempty"`
	// Dark is which mode of the scheme was last in force.
	Dark bool `json:"dark,omitempty"`
}

// ThemesDir is where colour schemes are read from: base16 files, in the same
// directory as the settings. Nothing writes here — a person drops in whatever
// scheme they already use.
func ThemesDir() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "themes"), nil
}

// LoadSettings reads the preferences, returning the defaults if there are none
// to read.
func LoadSettings() Settings {
	var s Settings
	readJSON("settings.json", &s)
	return s
}

// SaveSettings writes the preferences out atomically.
func SaveSettings(s Settings) error { return writeJSON("settings.json", s) }
