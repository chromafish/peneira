package git

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chromafish/peneira/internal/vcs"
)

// newRepo builds a small git repository: two commits, a rename in the second,
// and whatever the caller does to the working tree afterwards.
func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_AUTHOR_DATE=2026-01-01T00:00:00+00:00",
			"GIT_COMMITTER_DATE=2026-01-01T00:00:00+00:00")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-b", "main")
	write("main.go", "package main\n\nfunc main() {\n\tprintln(\"one\")\n}\n")
	write("notes.txt", "first\n")
	run("add", ".")
	run("commit", "-m", "first")

	write("main.go", "package main\n\nfunc main() {\n\tprintln(\"two\")\n\tprintln(\"three\")\n}\n")
	run("mv", "notes.txt", "readme.txt")
	run("commit", "-am", "second, with a rename")
	return dir
}

func open(t *testing.T, dir string) *Repo {
	t.Helper()
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	return r
}

func TestLog(t *testing.T) {
	r := open(t, newRepo(t))
	revs, err := r.Log(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if len(revs) != 2 {
		t.Fatalf("got %d revisions, want 2", len(revs))
	}
	if revs[0].Description != "second, with a rename" {
		t.Errorf("newest is %q, want the second commit", revs[0].Description)
	}
	if revs[0].Author != "Test" || revs[0].Email != "test@example.com" {
		t.Errorf("author is %q <%q>", revs[0].Author, revs[0].Email)
	}
	if revs[0].Timestamp.IsZero() {
		t.Error("the timestamp did not parse")
	}
	// The two identifiers are the same in git, and the parent has to match the
	// next row's identifier or the ancestry graph cannot be drawn.
	if revs[0].ChangeID != revs[0].CommitID {
		t.Errorf("change %q and commit %q should be the same in git", revs[0].ChangeID, revs[0].CommitID)
	}
	if len(revs[0].Parents) != 1 || revs[0].Parents[0] != revs[1].ChangeID {
		t.Errorf("parents are %v, want [%s]", revs[0].Parents, revs[1].ChangeID)
	}
	if !revs[1].Root {
		t.Error("the first commit should be marked as a root")
	}
	if len(revs[0].Bookmarks) != 1 || revs[0].Bookmarks[0] != "main" {
		t.Errorf("bookmarks are %v, want [main]", revs[0].Bookmarks)
	}
	// git has no equivalent of these, and inventing one would be a lie.
	if revs[0].Immutable || revs[0].Divergent || revs[0].Hidden {
		t.Error("a git revision claimed a mark git cannot know")
	}
}

// Uncommitted work is a row at the head of the list, because it is what you
// are looking at before you commit.
func TestWorkingTreeIsARow(t *testing.T) {
	dir := newRepo(t)
	r := open(t, dir)
	ctx := context.Background()

	revs, _ := r.Log(ctx, "", 10)
	if revs[0].WorkingCopy {
		t.Fatal("a clean tree should not get a row")
	}

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	revs, err := r.Log(ctx, "", 10)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if len(revs) != 3 || !revs[0].WorkingCopy {
		t.Fatalf("got %d revisions with working copy %v, want a working row on top",
			len(revs), revs[0].WorkingCopy)
	}
	if revs[0].CommitID != "" {
		t.Errorf("the working row has commit ID %q; it is not a commit", revs[0].CommitID)
	}
	if len(revs[0].Parents) != 1 || revs[0].Parents[0] != revs[1].ChangeID {
		t.Errorf("the working row hangs from %v, want [%s]", revs[0].Parents, revs[1].ChangeID)
	}

	// And its diff is the uncommitted work.
	spec := vcs.DiffSpec{Kind: vcs.DiffChange, Rev: revs[0].ChangeID}
	files, err := r.Files(ctx, spec)
	if err != nil {
		t.Fatalf("files: %v", err)
	}
	if len(files) != 1 || files[0].Path != "main.go" {
		t.Fatalf("the working diff touches %v, want main.go alone", files)
	}
	if files[0].Removed == 0 {
		t.Error("the working diff counted no removals")
	}
	diff, err := readDiff(r, ctx, spec)
	if err != nil || !strings.Contains(diff, "-func main() {") {
		t.Errorf("the working diff does not show the removal: %v\n%s", err, diff)
	}
	// The new side of the working tree is the file on disk.
	src, err := r.FileContent(ctx, spec, "main.go", vcs.After)
	if err != nil || string(src) != "package main\n" {
		t.Errorf("working file content = %q, %v", src, err)
	}
	old, err := r.FileContent(ctx, spec, "main.go", vcs.Before)
	if err != nil || !strings.Contains(string(old), "two") {
		t.Errorf("old side = %q, %v", old, err)
	}
}

// A rename has to survive as a rename, or the manifest shows a file being
// deleted and an unrelated one appearing.
func TestFilesDetectsRenames(t *testing.T) {
	r := open(t, newRepo(t))
	ctx := context.Background()
	revs, _ := r.Log(ctx, "", 1)

	files, err := r.Files(ctx, vcs.DiffSpec{Kind: vcs.DiffChange, Rev: revs[0].ChangeID})
	if err != nil {
		t.Fatalf("files: %v", err)
	}
	var renamed, modified bool
	for _, f := range files {
		switch f.Status {
		case vcs.Renamed:
			renamed = true
			if f.OldPath != "notes.txt" || f.Path != "readme.txt" {
				t.Errorf("rename is %q → %q", f.OldPath, f.Path)
			}
			if f.Display() != "notes.txt → readme.txt" {
				t.Errorf("rename displays as %q", f.Display())
			}
		case vcs.Modified:
			modified = true
			if f.Path != "main.go" || f.Added == 0 {
				t.Errorf("modified entry is %+v", f)
			}
		}
	}
	if !renamed || !modified {
		t.Errorf("got %+v, want a rename and a modification", files)
	}
}

// The root commit has no parent, so the old side of its files has nowhere to
// come from. It must not take the whole diff down with it.
func TestRootCommit(t *testing.T) {
	r := open(t, newRepo(t))
	ctx := context.Background()
	revs, _ := r.Log(ctx, "", 10)
	root := revs[len(revs)-1]

	spec := vcs.DiffSpec{Kind: vcs.DiffChange, Rev: root.ChangeID}
	files, err := r.Files(ctx, spec)
	if err != nil {
		t.Fatalf("files on the root commit: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("the root commit adds %d files, want 2", len(files))
	}
	if diff, err := readDiff(r, ctx, spec); err != nil || !strings.Contains(diff, "+package main") {
		t.Errorf("root diff: %v\n%s", err, diff)
	}
	if _, err := r.FileContent(ctx, spec, "main.go", vcs.Before); err == nil {
		t.Error("the root commit has no old side, and said it did")
	}
}

func TestRangeAndDescription(t *testing.T) {
	r := open(t, newRepo(t))
	ctx := context.Background()
	revs, _ := r.Log(ctx, "", 10)

	spec := vcs.DiffSpec{Kind: vcs.DiffRange, From: revs[1].ChangeID, To: revs[0].ChangeID}
	files, err := r.Files(ctx, spec)
	if err != nil || len(files) == 0 {
		t.Fatalf("range diff: %v, %d files", err, len(files))
	}
	desc, err := r.Description(ctx, revs[0].ChangeID)
	if err != nil || !strings.HasPrefix(desc, "second, with a rename") {
		t.Errorf("description = %q, %v", desc, err)
	}
}

// The query is passed to git log as typed.
func TestQueryIsPassedThrough(t *testing.T) {
	r := open(t, newRepo(t))
	ctx := context.Background()
	revs, err := r.Log(ctx, "--author=Test HEAD", 10)
	if err != nil || len(revs) != 2 {
		t.Fatalf("query: %v, %d revisions", err, len(revs))
	}
	if _, err := r.Log(ctx, "no-such-ref", 10); err == nil {
		t.Error("a query naming nothing should report git's own error")
	}
}

func TestInfoAndVersion(t *testing.T) {
	r := open(t, newRepo(t))
	info := r.Info()
	if info.Name != "git" || info.QueryLabel == "" || info.Handoff == "" {
		t.Errorf("info = %+v", info)
	}
	if info.Version == "" || strings.Contains(info.Version, "git version") {
		t.Errorf("version = %q", info.Version)
	}
	if err := r.Snapshot(context.Background()); err != nil {
		t.Errorf("snapshot: %v", err)
	}
}

// readDiff drains a streamed diff into a string, for the tests that only care
// what it says.
func readDiff(r *Repo, ctx context.Context, spec vcs.DiffSpec) (string, error) {
	stream, err := r.DiffStream(ctx, spec, 3)
	if err != nil {
		return "", err
	}
	out, err := io.ReadAll(stream)
	if cerr := stream.Close(); err == nil {
		err = cerr
	}
	return string(out), err
}
