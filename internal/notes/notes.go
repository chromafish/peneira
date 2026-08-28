// Package notes renders review notes as text to hand to a coding agent: every
// note with the code it refers to, where that code lives, and enough about the
// change for the agent to find its way back to it.
package notes

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Note is one comment left on a line, or on a run of them.
type Note struct {
	Path string
	Old  bool // anchored to the pre-change side of the diff
	Line int
	// EndLine closes a note written about a run of lines. Zero, or the same as
	// Line, means the note is about that one line.
	EndLine int
	Body    string
}

// last is the end of the run a note covers.
func (n Note) last() int {
	if n.EndLine > n.Line {
		return n.EndLine
	}
	return n.Line
}

// Change identifies what was reviewed.
type Change struct {
	RepoRoot    string
	ChangeID    string
	CommitID    string
	Description string

	// Handoff tells the agent how to get to the change. It comes from the
	// backend, since checking out a git commit and editing a jj change are
	// not the same act, and takes the change's identifier as its one verb.
	Handoff string
}

// Source supplies the contents of a file on one side of the diff. It is an
// interface so the renderer can be tested without a repository.
type Source interface {
	// FileLines returns the lines of path as of the given side. Returning an
	// error is not fatal: the note is still exported, without its snippet.
	FileLines(ctx context.Context, path string, old bool) ([]string, error)
}

// Context is how many lines to show either side of the commented line.
const Context = 4

// Render produces the text to paste into an agent. Notes are grouped by file
// and ordered by line, which is the order someone would work through them.
func Render(ctx context.Context, change Change, notes []Note, src Source) string {
	if len(notes) == 0 {
		return ""
	}
	sorted := append([]Note(nil), notes...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		return sorted[i].Line < sorted[j].Line
	})

	var b strings.Builder
	plural := "notes"
	if len(sorted) == 1 {
		plural = "note"
	}
	fmt.Fprintf(&b, "# Code review: %d %s to address\n\n", len(sorted), plural)

	fmt.Fprintf(&b, "Repository: %s\n", change.RepoRoot)
	if change.ChangeID != "" {
		fmt.Fprintf(&b, "Change: %s (commit %s)\n", change.ChangeID, change.CommitID)
		if d := firstLine(change.Description); d != "" {
			fmt.Fprintf(&b, "Description: %s\n", d)
		}
		if change.Handoff != "" {
			fmt.Fprintf(&b, "\n"+change.Handoff+"\n", change.ChangeID)
		}
	}
	b.WriteString("\nLine numbers are as of that revision.\n")

	// Cache each file's contents: several notes usually land in one file.
	cache := map[string][]string{}
	lines := func(path string, old bool) []string {
		key := path
		if old {
			key = "-" + path
		}
		if got, ok := cache[key]; ok {
			return got
		}
		got, err := src.FileLines(ctx, path, old)
		if err != nil {
			got = nil
		}
		cache[key] = got
		return got
	}

	for i, n := range sorted {
		side := ""
		if n.Old {
			side = " (line as it was before the change)"
		}
		where := fmt.Sprintf("%s:%d", n.Path, n.Line)
		if n.last() > n.Line {
			where = fmt.Sprintf("%s:%d-%d", n.Path, n.Line, n.last())
		}
		fmt.Fprintf(&b, "\n## %d. %s%s\n\n", i+1, where, side)

		for _, line := range strings.Split(strings.TrimRight(n.Body, "\n"), "\n") {
			b.WriteString("> " + line + "\n")
		}

		if snippet := snippet(lines(n.Path, n.Old), n.Line, n.last()); snippet != "" {
			fmt.Fprintf(&b, "\n```%s\n%s```\n", language(n.Path), snippet)
		}
	}
	return b.String()
}

// snippet renders the commented lines with a few either side, numbered, and an
// arrow on every line the note is about.
func snippet(src []string, from, to int) string {
	if len(src) == 0 || from < 1 || from > len(src) {
		return ""
	}
	to = min(max(to, from), len(src))
	start := max(1, from-Context)
	end := min(len(src), to+Context)
	width := len(fmt.Sprint(end))

	var b strings.Builder
	for n := start; n <= end; n++ {
		marker := "  "
		if n >= from && n <= to {
			marker = "→ "
		}
		fmt.Fprintf(&b, "%s%*d  %s\n", marker, width, n, src[n-1])
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// language maps a file name to a fenced code block tag, so the snippet is
// highlighted wherever the text is pasted.
func language(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".jsx":
		return "jsx"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cc", ".cpp", ".hpp", ".cxx":
		return "cpp"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".sh", ".bash", ".zsh":
		return "bash"
	case ".sql":
		return "sql"
	case ".css":
		return "css"
	case ".html":
		return "html"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".md":
		return "markdown"
	case ".zig":
		return "zig"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".ex", ".exs":
		return "elixir"
	case ".clj", ".cljs":
		return "clojure"
	case ".lua":
		return "lua"
	case ".nix":
		return "nix"
	}
	switch filepath.Base(path) {
	case "Makefile", "makefile":
		return "make"
	case "Dockerfile":
		return "dockerfile"
	}
	return ""
}
