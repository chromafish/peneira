package notes

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeSource map[string][]string

func (f fakeSource) FileLines(_ context.Context, path string, old bool) ([]string, error) {
	key := path
	if old {
		key = "-" + path
	}
	lines, ok := f[key]
	if !ok {
		return nil, errors.New("no such file")
	}
	return lines, nil
}

var src = fakeSource{
	"main.go": {
		"package main", "", "import \"fmt\"", "",
		"func main() {", "\tfmt.Println(\"hi\")", "}",
	},
	"-old.go": {"one", "two", "three"},
}

func TestRenderGroupsAndOrdersNotes(t *testing.T) {
	out := Render(context.Background(), Change{
		RepoRoot:    "/src/project",
		ChangeID:    "zoqmommk",
		CommitID:    "f04566a9",
		Description: "third: renames and adds\n\nbody text",
		Handoff:     "otherwise run `jj edit %[1]s` before making the edits below.",
	}, []Note{
		{Path: "main.go", Line: 6, Body: "this should be Printf"},
		{Path: "main.go", Line: 1, Body: "package comment missing"},
	}, src)

	if !strings.HasPrefix(out, "# Code review: 2 notes to address") {
		t.Errorf("header = %q", firstLine(out))
	}
	if !strings.Contains(out, "Repository: /src/project") {
		t.Error("the repository path is missing")
	}
	// The instruction comes from the backend, with the change's identifier
	// filled in: getting to a jj change and getting to a git commit are not
	// the same act, and the exporter does not know which it has.
	if !strings.Contains(out, "jj edit zoqmommk") {
		t.Error("the export should say how to get to the change")
	}
	// Only the first line of the description belongs in a one line summary.
	if strings.Contains(out, "body text") {
		t.Error("the description body should not be included")
	}

	first := strings.Index(out, "## 1. main.go:1")
	second := strings.Index(out, "## 2. main.go:6")
	if first < 0 || second < 0 || first > second {
		t.Errorf("notes are not ordered by line:\n%s", out)
	}
	if !strings.Contains(out, "> this should be Printf") {
		t.Error("the note body should be quoted")
	}
	if !strings.Contains(out, "```go") {
		t.Error("the snippet should be fenced with the file's language")
	}
}

func TestRenderMarksTheCommentedLine(t *testing.T) {
	out := Render(context.Background(), Change{RepoRoot: "/src"},
		[]Note{{Path: "main.go", Line: 6, Body: "here"}}, src)

	var marked string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "→") {
			marked = line
		}
	}
	if !strings.Contains(marked, `fmt.Println("hi")`) {
		t.Errorf("the arrow is on %q, want the line the note is about", marked)
	}
	// Context is clipped to the file, not padded past its end.
	if strings.Contains(out, "  8  ") {
		t.Error("the snippet ran past the end of the file")
	}
	if !strings.Contains(out, "  2  ") {
		t.Errorf("the snippet should reach back %d lines:\n%s", Context, out)
	}
}

func TestRenderSurvivesAMissingFile(t *testing.T) {
	out := Render(context.Background(), Change{RepoRoot: "/src"},
		[]Note{{Path: "gone.go", Line: 3, Body: "still worth saying"}}, src)

	if !strings.Contains(out, "> still worth saying") {
		t.Error("a note whose file cannot be read should still be exported")
	}
	if strings.Contains(out, "```") {
		t.Error("there should be no empty code fence when there is no snippet")
	}
}

func TestRenderUsesTheOldSideWhenAsked(t *testing.T) {
	out := Render(context.Background(), Change{RepoRoot: "/src"},
		[]Note{{Path: "old.go", Line: 2, Old: true, Body: "why was this dropped"}}, src)

	if !strings.Contains(out, "line as it was before the change") {
		t.Error("a note on the removed side should say so")
	}
	if !strings.Contains(out, "two") {
		t.Errorf("the snippet should come from the old side:\n%s", out)
	}
}

func TestRenderEmpty(t *testing.T) {
	if out := Render(context.Background(), Change{}, nil, src); out != "" {
		t.Errorf("no notes should render nothing, got %q", out)
	}
}
