package ui

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/chromafish/check/internal/sonda"
)

// A log is set as lines of runs, each run in one role, and the role decides
// the colour. The same distinctions the diff draws are drawn here: keys,
// quoted strings and numbers are set apart, and the rest is text.

type textRole uint8

const (
	rolePlain textRole = iota
	roleKey
	rolePunct
	roleString
	roleNumber
	roleWord
	roleSource
)

type textRun struct {
	text string
	role textRole
}

// logLines lays one log out. Formatted, the first line is the message with
// the structured fields after it and the source location last; raw, every
// line is the text as it was written. Continuation lines follow either way.
func logLines(l sonda.Log, raw bool) [][]textRun {
	text := l.Message
	if raw {
		text = l.Raw
	}
	var lines [][]textRun
	for _, line := range strings.Split(expandTabs(text), "\n") {
		lines = append(lines, scanText(line))
	}
	if raw {
		return lines
	}
	first := lines[0]
	for _, f := range l.Fields {
		if len(first) > 0 {
			first = append(first, textRun{" ", rolePlain})
		}
		first = append(first, textRun{f.Key, roleKey}, textRun{"=", rolePunct}, fieldValue(f))
	}
	if l.Source.Path != "" {
		if len(first) > 0 {
			first = append(first, textRun{" ", rolePlain})
		}
		loc := l.Source.Path
		if l.Source.Line > 0 {
			loc += ":" + strconv.Itoa(l.Source.Line)
		}
		first = append(first, textRun{loc, roleSource})
	}
	lines[0] = first
	return lines
}

// fieldValue sets a value in the role its kind calls for. A string is quoted
// only where leaving it bare would run it into its neighbours.
func fieldValue(f sonda.Field) textRun {
	switch f.Kind {
	case sonda.Number:
		return textRun{f.Value, roleNumber}
	case sonda.Bool, sonda.Null:
		return textRun{f.Value, roleWord}
	case sonda.Object:
		return textRun{f.Value, rolePlain}
	}
	if f.Value == "" || strings.ContainsAny(f.Value, " \t\"=") {
		return textRun{strconv.Quote(f.Value), roleString}
	}
	return textRun{f.Value, roleString}
}

// scanText sets a line of text apart into quoted strings, numbers, and the
// rest. Nothing is parsed: a number is a word made of digits, with a point
// and a unit allowed, and a string is whatever sits between a pair of
// quotes.
func scanText(s string) []textRun {
	var runs []textRun
	add := func(text string, role textRole) {
		if text == "" {
			return
		}
		if n := len(runs); n > 0 && runs[n-1].role == role {
			runs[n-1].text += text
			return
		}
		runs = append(runs, textRun{text, role})
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"':
			end := closingQuote(s, i)
			add(s[i:end], roleString)
			i = end
		case c >= '0' && c <= '9' && (i == 0 || !wordByte(s[i-1])):
			end := i
			for end < len(s) && ((s[end] >= '0' && s[end] <= '9') || s[end] == '.') {
				end++
			}
			unit := end
			for unit < len(s) && wordByte(s[unit]) {
				unit++
			}
			// Digits with letters after them are a measurement; digits with
			// anything else running on are part of a word, such as a date or
			// an identifier, and stay text.
			if unit == end || isUnit(s[end:unit]) {
				add(s[i:unit], roleNumber)
			} else {
				add(s[i:unit], rolePlain)
			}
			i = unit
		default:
			end := i + 1
			for end < len(s) && s[end] != '"' && !(s[end] >= '0' && s[end] <= '9' && !wordByte(s[end-1])) {
				end++
			}
			add(s[i:end], rolePlain)
			i = end
		}
	}
	return runs
}

// closingQuote finds the end of a double-quoted run starting at i, honouring
// backslash escapes, or the end of the line when it never closes.
func closingQuote(s string, i int) int {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return len(s)
}

func wordByte(c byte) bool {
	return c == '_' || c == '-' || c == '.' || c == ':' || c == '/' || (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= utf8.RuneSelf
}

// isUnit accepts the suffixes a measurement carries: ms, µs, s, m, KB, MiB.
func isUnit(s string) bool {
	if utf8.RuneCountInString(s) > 3 {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth))
}

// width is a line's length in cells.
func width(line []textRun) int {
	n := 0
	for _, r := range line {
		n += utf8.RuneCountInString(r.text)
	}
	return n
}

// levelText is the level as it goes in its column: the short name the
// severity carries, or the first letters of a level the parser did not know,
// and nothing for a line that had none.
func levelText(l sonda.Log) string {
	if name := l.Severity.Name(); name != "" {
		return name
	}
	if l.Level == "" {
		return ""
	}
	name := strings.ToUpper(l.Level)
	if utf8.RuneCountInString(name) > levelCols {
		name = string([]rune(name)[:levelCols])
	}
	return name
}

// levelCols is the width of the level column, in cells.
const levelCols = 5
