// Package highlight turns source text into per-line coloured spans.
//
// Highlighting works on whole files rather than on individual diff lines: a
// line pulled out of a hunk has no way of knowing it sits inside a block
// comment or a raw string, and lexing it alone gets those cases wrong. The
// caller fetches each side of the file from the repository, highlights it
// once, and looks up lines by number.
package highlight

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Class is a coarse token category. Keeping the set small lets the UI theme
// define a handful of colours instead of mirroring chroma's full token tree.
type Class uint8

const (
	Plain Class = iota
	Keyword
	Name
	Function
	Type
	String
	Number
	Comment
	Operator
	Punctuation
	Preproc
	Error
)

// Span is a byte range within a single line, all of one class.
type Span struct {
	Start, End int
	Class      Class
}

// Lines holds the spans of a highlighted file, indexed by line number - 1.
type Lines [][]Span

// Line returns the spans for a 1-based line number, or nil if out of range.
func (l Lines) Line(n int) []Span {
	if n < 1 || n > len(l) {
		return nil
	}
	return l[n-1]
}

// Limits past which highlighting is not worth the latency; the diff still
// renders, just in a single colour.
const (
	maxBytes = 4 << 20
	maxLines = 200_000
)

// File highlights src, choosing a lexer from the file name and falling back to
// the content. It returns nil when the file has no known syntax or is too
// large, which callers render as plain text.
func File(path string, src []byte) Lines {
	if len(src) == 0 || len(src) > maxBytes {
		return nil
	}
	lexer := lexers.Match(path)
	if lexer == nil {
		lexer = lexers.Analyse(string(src))
	}
	if lexer == nil {
		return nil
	}
	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, string(src))
	if err != nil {
		return nil
	}

	var (
		out  Lines
		line []Span
		col  int
	)
	endLine := func() {
		out = append(out, line)
		line = nil
		col = 0
	}
	for tok := iterator(); tok != chroma.EOF; tok = iterator() {
		class := classOf(tok.Type)
		// A token may span newlines, so it is cut at each one.
		for {
			nl := strings.IndexByte(tok.Value, '\n')
			if nl < 0 {
				break
			}
			if nl > 0 {
				line = appendSpan(line, col, col+nl, class)
			}
			endLine()
			if len(out) > maxLines {
				return out
			}
			tok.Value = tok.Value[nl+1:]
		}
		if tok.Value != "" {
			line = appendSpan(line, col, col+len(tok.Value), class)
			col += len(tok.Value)
		}
	}
	if line != nil || col > 0 {
		endLine()
	}
	return out
}

// appendSpan extends the previous span when the class matches, so a run of
// identical tokens costs one entry rather than many.
func appendSpan(line []Span, start, end int, class Class) []Span {
	if class == Plain {
		return line
	}
	if n := len(line); n > 0 && line[n-1].Class == class && line[n-1].End == start {
		line[n-1].End = end
		return line
	}
	return append(line, Span{start, end, class})
}

// classOf maps chroma's token tree onto the coarse classes above.
func classOf(t chroma.TokenType) Class {
	switch {
	case t.InCategory(chroma.Comment):
		return Comment
	case t.InCategory(chroma.LiteralString):
		return String
	case t.InCategory(chroma.LiteralNumber):
		return Number
	case t.InCategory(chroma.Operator):
		return Operator
	case t.InCategory(chroma.Punctuation):
		return Punctuation
	case t.InCategory(chroma.Error):
		return Error
	}
	switch t {
	case chroma.KeywordType:
		return Type
	case chroma.NameFunction, chroma.NameFunctionMagic, chroma.NameDecorator:
		return Function
	case chroma.NameClass, chroma.NameNamespace, chroma.NameBuiltin, chroma.NameException:
		return Type
	case chroma.CommentPreproc, chroma.CommentPreprocFile:
		return Preproc
	}
	switch {
	case t.InCategory(chroma.Keyword):
		return Keyword
	case t.InSubCategory(chroma.NameAttribute), t.InCategory(chroma.Name):
		return Name
	}
	return Plain
}
