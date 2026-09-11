package ui

import (
	"path/filepath"
	"strings"
)

// IsDoc reports whether a path is a rendered document by extension alone.
// The set is {.md, .adoc, .asciidoc}, case-insensitive, no path sniffing.
func IsDoc(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".adoc", ".asciidoc":
		return true
	default:
		return false
	}
}
