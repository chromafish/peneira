package jj

import (
	"strings"
	"testing"

	"github.com/chromafish/peneira/internal/vcs"
)

// record builds a log record with the field separators the template emits.
func record(fields ...string) string {
	return strings.Join(fields, fs) + rs
}

func TestParseLog(t *testing.T) {
	out := record("zoqmommk", "zoqmommkfull", "f04566a9", "f04566a9full",
		"third: renames and adds", "Reviewer", "reviewer@example.com",
		"2026-08-27T23:44:23+01:00", "1000000", "main", "main@git", "", "rvromurn") +
		"\n" + record("zzzzzzzz", "zzzzzzzzfull", "00000000", "00000000full",
		"", "", "", "1970-01-01T00:00:00+00:00", "0101100", "", "", "", "")

	revs, err := parseLog(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 2 {
		t.Fatalf("got %d revisions, want 2", len(revs))
	}

	r := revs[0]
	if r.ChangeID != "zoqmommk" || r.CommitID != "f04566a9" {
		t.Errorf("ids = %q %q", r.ChangeID, r.CommitID)
	}
	if !r.WorkingCopy || r.Immutable || r.Conflict || r.Empty {
		t.Errorf("flags = %+v", r)
	}
	if got := r.Timestamp.Format("2006-01-02 15:04"); got != "2026-08-27 23:44" {
		t.Errorf("timestamp = %s", got)
	}
	if len(r.Bookmarks) != 1 || r.Bookmarks[0] != "main" {
		t.Errorf("bookmarks = %v", r.Bookmarks)
	}
	if len(r.RemoteBookmarks) != 1 || r.RemoteBookmarks[0] != "main@git" {
		t.Errorf("remote bookmarks = %v", r.RemoteBookmarks)
	}
	if len(r.Parents) != 1 || r.Parents[0] != "rvromurn" {
		t.Errorf("parents = %v", r.Parents)
	}

	// The root commit has no description, and Subject stands in for it.
	if got := revs[1].Subject(); got != "(no description set)" {
		t.Errorf("subject = %q", got)
	}
	if !revs[1].Immutable || !revs[1].Empty || !revs[1].Root {
		t.Errorf("root flags = %+v", revs[1])
	}
	if revs[1].Parents != nil {
		t.Errorf("root parents = %v", revs[1].Parents)
	}
}

func TestParseSummary(t *testing.T) {
	files := parseSummary("R {a.txt => b.txt}\nA c.txt\nM main.go\nD gone.txt\n")
	want := []vcs.FileChange{
		{Status: vcs.Renamed, OldPath: "a.txt", Path: "b.txt"},
		{Status: vcs.Added, Path: "c.txt"},
		{Status: vcs.Modified, Path: "main.go"},
		{Status: vcs.Deleted, Path: "gone.txt"},
	}
	if len(files) != len(want) {
		t.Fatalf("got %d files, want %d", len(files), len(want))
	}
	for i := range want {
		if files[i] != want[i] {
			t.Errorf("file %d = %+v, want %+v", i, files[i], want[i])
		}
	}
	if got := files[0].Display(); got != "a.txt → b.txt" {
		t.Errorf("rename display = %q", got)
	}
}

func TestParseStat(t *testing.T) {
	counts := parseStat("{a.txt => b.txt} | 0\nc.txt | 1 +\nmain.go | 4 ++--\n" +
		"big.go | 200 +++++++---\n3 files changed, 3 insertions(+), 2 deletions(-)\n")

	if got := counts["b.txt"]; got != [2]int{0, 0} {
		t.Errorf("rename counts = %v", got)
	}
	if got := counts["c.txt"]; got != [2]int{1, 0} {
		t.Errorf("addition counts = %v", got)
	}
	if got := counts["main.go"]; got != [2]int{2, 2} {
		t.Errorf("modification counts = %v", got)
	}
	// The bar graph is scaled when it does not fit, so the split has to be
	// recovered from its ratio against the exact total.
	if got := counts["big.go"]; got[0]+got[1] != 200 || got[0] != 140 {
		t.Errorf("scaled counts = %v, want 140/60", got)
	}
	if _, ok := counts["3 files changed"]; ok {
		t.Error("summary line was parsed as a file")
	}
}

func TestExpandBraces(t *testing.T) {
	cases := []struct{ in, old, new string }{
		{"main.go", "", "main.go"},
		{"{a.txt => b.txt}", "a.txt", "b.txt"},
		{"internal/{old => new}/file.go", "internal/old/file.go", "internal/new/file.go"},
		{"a.txt => b.txt", "a.txt", "b.txt"},
	}
	for _, c := range cases {
		old, new := expandBraces(c.in)
		if old != c.old || new != c.new {
			t.Errorf("expandBraces(%q) = %q, %q; want %q, %q", c.in, old, new, c.old, c.new)
		}
	}
}

func TestDiffSpecArgs(t *testing.T) {
	cases := []struct {
		spec vcs.DiffSpec
		want string
	}{
		{vcs.DiffSpec{Kind: vcs.DiffChange, Rev: "@"}, "diff -r @"},
		{vcs.DiffSpec{Kind: vcs.DiffRange, From: "a", To: "b"}, "diff --from a --to b"},
	}
	for _, c := range cases {
		if got := strings.Join(args(c.spec), " "); got != c.want {
			t.Errorf("args = %q, want %q", got, c.want)
		}
	}
	if !(vcs.DiffSpec{Kind: vcs.DiffChange}).Empty() {
		t.Error("a spec with no revision should be empty")
	}
	if (vcs.DiffSpec{Kind: vcs.DiffRange, From: "a", To: "b"}).Empty() {
		t.Error("a complete range should not be empty")
	}
}
