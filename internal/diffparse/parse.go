// Package diffparse turns git-format unified diffs into structured files,
// hunks and lines.
package diffparse

import (
	"bufio"
	"io"
	"math"
	"strconv"
	"strings"
)

// Kind classifies a line within a hunk.
type Kind uint8

const (
	Context Kind = iota
	Added
	Removed
)

// Line is a single row of a hunk.
type Line struct {
	Kind   Kind
	OldNum int // line number on the "from" side, 0 if the line is an addition
	NewNum int // line number on the "to" side, 0 if the line is a removal
	Text   string

	// Segments marks the parts of Text that differ from the line this one was
	// paired with, for added and removed lines that are two versions of the
	// same line. Nil when there is nothing to highlight.
	Segments []Segment

	NoNewline bool // the file ends here without a trailing newline
}

// Segment is a byte range of a line, flagged as changed or unchanged.
type Segment struct {
	Start, End int
	Changed    bool
}

// Hunk is one contiguous region of a file's diff.
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Section            string // the "@@ ... @@ func foo()" trailer
	Lines              []Line
}

// The ceilings on how much of a diff is kept. They sit well above any change a
// person reads line by line; what they guard against is the degenerate case —
// a vendored dependency, a minified bundle, generated code — where holding the
// whole diff would cost hundreds of megabytes and stall the frame that walks
// it.
//
// MaxFileLines bounds one file. MaxTotalLines bounds the change, because a
// diff of ten thousand files each just under the per-file ceiling would
// otherwise slip past it.
const (
	MaxFileLines  = 100_000
	MaxTotalLines = 400_000
)

// File is the diff of a single path.
type File struct {
	OldPath string
	NewPath string
	OldMode string
	NewMode string

	IsNew     bool
	IsDeleted bool
	IsRename  bool
	IsCopy    bool
	IsBinary  bool

	// Truncated reports that the file ran past a ceiling and its body was
	// dropped: Hunks is empty, and LineCount says how large it was so the
	// interface can say what it is not showing.
	Truncated bool
	LineCount int

	Hunks []Hunk
}

// Path is the path the file should be identified by: where it ended up, or
// where it was for a deletion.
func (f *File) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// Counts returns the number of added and removed lines.
func (f *File) Counts() (added, removed int) {
	for i := range f.Hunks {
		for _, l := range f.Hunks[i].Lines {
			switch l.Kind {
			case Added:
				added++
			case Removed:
				removed++
			}
		}
	}
	return added, removed
}

// Parse reads a git-format unified diff, which may describe any number of
// files. Files past the ceilings are returned marked Truncated, without their
// bodies; ParseOne is how one of them is then read in full.
func Parse(diff string) []File {
	return parse(diff, MaxFileLines, MaxTotalLines)
}

// ParseOne reads a diff with no ceiling on it, for a single file asked for by
// name after Parse left it out. The caller has chosen to pay for it.
func ParseOne(diff string) []File {
	return parse(diff, math.MaxInt, math.MaxInt)
}

// Scan reads a diff as it is written and hands over each file as soon as that
// file's last line is in, so a change can be put on screen while the rest of
// it is still arriving. The ceilings are Parse's, and they hold across the
// whole stream: how a change is read does not change how much of it is kept.
func Scan(r io.Reader, emit func(File)) error {
	p := newParser(MaxFileLines, MaxTotalLines, emit)
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			p.line(strings.TrimSuffix(line, "\n"))
		}
		if err != nil {
			p.done()
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func parse(diff string, maxFile, maxTotal int) []File {
	var files []File
	p := newParser(maxFile, maxTotal, func(f File) { files = append(files, f) })
	for _, line := range splitLines(diff) {
		p.line(line)
	}
	p.done()
	return files
}

// parser reads a diff one line at a time, handing each file to emit as that
// file ends. Taking it line by line is what lets the same code serve a diff
// held whole in memory and one still coming out of the tool.
type parser struct {
	emit              func(File)
	maxFile, maxTotal int

	cur    *File
	hunk   *Hunk
	oldNum int
	newNum int
	budget int

	fileLines  int // how long the file is, counting past the ceiling
	fileKept   int // and how much of it was stored
	totalLines int
}

func newParser(maxFile, maxTotal int, emit func(File)) *parser {
	return &parser{emit: emit, maxFile: maxFile, maxTotal: maxTotal, budget: maxRefinedPairs}
}

func (p *parser) flushHunk() {
	if p.hunk != nil && p.cur != nil && !p.cur.Truncated {
		refineHunk(p.hunk, &p.budget)
		p.cur.Hunks = append(p.cur.Hunks, *p.hunk)
	}
	p.hunk = nil
}

func (p *parser) flushFile() {
	p.flushHunk()
	if p.cur != nil {
		p.cur.LineCount = p.fileLines
		if p.cur.Truncated {
			// Holding the body is the cost the ceiling exists to avoid. What it
			// read before giving up is handed back to the change's allowance, so
			// one bundle does not spend the budget twice.
			p.cur.Hunks = nil
			p.totalLines -= p.fileKept
		}
		p.emit(*p.cur)
	}
	p.cur = nil
	p.fileLines, p.fileKept = 0, 0
}

func (p *parser) done() { p.flushFile() }

// keep counts one line of a hunk and reports whether to store it. A file past
// a ceiling is still counted to its end, so the interface can say how much it
// is leaving out.
//
// Only lines that are kept count against the whole-diff ceiling: a dependency
// bundle should cost the change its own body and nothing more, rather than
// starving the hand-written files that follow it.
func (p *parser) keep() bool {
	p.fileLines++
	if p.fileLines > p.maxFile {
		p.cur.Truncated = true
	}
	if p.cur.Truncated || p.totalLines >= p.maxTotal {
		p.cur.Truncated = true
		return false
	}
	p.totalLines++
	p.fileKept++
	return true
}

func (p *parser) line(line string) {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		p.flushFile()
		p.cur = &File{}
		p.cur.OldPath, p.cur.NewPath = parseGitHeader(line)

	case p.cur == nil:
		// Preamble before the first file header; ignore.

	case strings.HasPrefix(line, "@@"):
		p.flushHunk()
		h, ok := parseHunkHeader(line)
		if !ok {
			return
		}
		p.hunk = &h
		p.oldNum, p.newNum = h.OldStart, h.NewStart

	case p.hunk == nil:
		// Extended header lines, which appear between the "diff --git" line and
		// the first hunk.
		switch {
		case strings.HasPrefix(line, "new file mode "):
			p.cur.IsNew = true
			p.cur.NewMode = strings.TrimPrefix(line, "new file mode ")
		case strings.HasPrefix(line, "deleted file mode "):
			p.cur.IsDeleted = true
			p.cur.OldMode = strings.TrimPrefix(line, "deleted file mode ")
		case strings.HasPrefix(line, "old mode "):
			p.cur.OldMode = strings.TrimPrefix(line, "old mode ")
		case strings.HasPrefix(line, "new mode "):
			p.cur.NewMode = strings.TrimPrefix(line, "new mode ")
		case strings.HasPrefix(line, "rename from "):
			p.cur.IsRename = true
			p.cur.OldPath = strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "rename to "):
			p.cur.IsRename = true
			p.cur.NewPath = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "copy from "):
			p.cur.IsCopy = true
			p.cur.OldPath = strings.TrimPrefix(line, "copy from ")
		case strings.HasPrefix(line, "copy to "):
			p.cur.IsCopy = true
			p.cur.NewPath = strings.TrimPrefix(line, "copy to ")
		case strings.HasPrefix(line, "--- "):
			if q := strings.TrimPrefix(line, "--- "); q == "/dev/null" {
				p.cur.IsNew = true
			} else {
				p.cur.OldPath = stripPrefix(q)
			}
		case strings.HasPrefix(line, "+++ "):
			if q := strings.TrimPrefix(line, "+++ "); q == "/dev/null" {
				p.cur.IsDeleted = true
			} else {
				p.cur.NewPath = stripPrefix(q)
			}
		case strings.HasPrefix(line, "Binary files ") || strings.HasSuffix(line, " differ"):
			p.cur.IsBinary = true
		}

	case strings.HasPrefix(line, "\\"):
		// "\ No newline at end of file" applies to the line just emitted.
		if n := len(p.hunk.Lines); n > 0 {
			p.hunk.Lines[n-1].NoNewline = true
		}

	case strings.HasPrefix(line, "+"):
		if p.keep() {
			p.hunk.Lines = append(p.hunk.Lines, Line{Kind: Added, NewNum: p.newNum, Text: line[1:]})
		}
		p.newNum++

	case strings.HasPrefix(line, "-"):
		if p.keep() {
			p.hunk.Lines = append(p.hunk.Lines, Line{Kind: Removed, OldNum: p.oldNum, Text: line[1:]})
		}
		p.oldNum++

	case line == "" || strings.HasPrefix(line, " "):
		text := ""
		if line != "" {
			text = line[1:]
		}
		if p.keep() {
			p.hunk.Lines = append(p.hunk.Lines, Line{Kind: Context, OldNum: p.oldNum, NewNum: p.newNum, Text: text})
		}
		p.oldNum++
		p.newNum++

	default:
		// Anything else ends the hunk: a trailing "diff --git" is handled above,
		// so this is stray output.
		p.flushHunk()
	}
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// parseGitHeader pulls the two paths out of `diff --git a/x b/y`. Paths with
// spaces make this ambiguous in general; the a/ and b/ prefixes resolve the
// common cases, and the ---/+++ lines correct the rest.
func parseGitHeader(line string) (oldPath, newPath string) {
	rest := strings.TrimPrefix(line, "diff --git ")
	if i := strings.Index(rest, " b/"); i > 0 {
		return stripPrefix(rest[:i]), stripPrefix(rest[i+1:])
	}
	if fields := strings.Fields(rest); len(fields) == 2 {
		return stripPrefix(fields[0]), stripPrefix(fields[1])
	}
	return "", ""
}

func stripPrefix(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "a/")
	p = strings.TrimPrefix(p, "b/")
	return p
}

// parseHunkHeader reads "@@ -old,count +new,count @@ section".
func parseHunkHeader(line string) (Hunk, bool) {
	end := strings.Index(line[2:], "@@")
	if end < 0 {
		return Hunk{}, false
	}
	end += 2
	var h Hunk
	h.Section = strings.TrimSpace(line[end+2:])
	for _, part := range strings.Fields(line[2:end]) {
		if len(part) < 2 {
			continue
		}
		start, count := parseRange(part[1:])
		switch part[0] {
		case '-':
			h.OldStart, h.OldCount = start, count
		case '+':
			h.NewStart, h.NewCount = start, count
		}
	}
	return h, true
}

func parseRange(s string) (start, count int) {
	count = 1
	if i := strings.Index(s, ","); i >= 0 {
		count, _ = strconv.Atoi(s[i+1:])
		s = s[:i]
	}
	start, _ = strconv.Atoi(s)
	return start, count
}
