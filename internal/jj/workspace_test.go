package jj

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo builds a jj repository with two described changes and a working
// copy on top of the second.
func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("jj", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"JJ_USER=Test", "JJ_EMAIL=test@example.com",
			"JJ_TIMESTAMP=2026-01-01T00:00:00+00:00")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("jj %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("git", "init")
	write(".gitignore", "artefact\n")
	write("main.go", "one\n")
	run("describe", "-m", "first")
	run("new", "-m", "second")
	write("main.go", "two\n")
	run("new")
	return dir
}

func TestWorkspaceIsMovedBetweenRevisions(t *testing.T) {
	dir := newRepo(t)
	ctx := context.Background()
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	revs, err := r.Log(ctx, "subject(first) | subject(second)", 10)
	if err != nil || len(revs) != 2 {
		t.Fatalf("log: %v, %d revisions", err, len(revs))
	}
	second, first := revs[0].ChangeID, revs[1].ChangeID

	ws := filepath.Join(t.TempDir(), "cache", "repo-1234")
	if err := r.Workspace(ctx, ws, first); err != nil {
		t.Fatalf("adding the workspace: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(ws, "main.go"))
	if err != nil || string(body) != "one\n" {
		t.Fatalf("at the first change main.go = %q, %v", body, err)
	}

	// A build leaves ignored output behind, and something it should not have.
	if err := os.WriteFile(filepath.Join(ws, "artefact"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "stray"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Workspace(ctx, ws, second); err != nil {
		t.Fatalf("moving the workspace: %v", err)
	}
	body, _ = os.ReadFile(filepath.Join(ws, "main.go"))
	if string(body) != "two\n" {
		t.Errorf("at the second change main.go = %q", body)
	}
	if _, err := os.Stat(filepath.Join(ws, "artefact")); err != nil {
		t.Error("moving the workspace threw away the ignored output the build left in it")
	}
	// The two described changes, and one working copy for each workspace:
	// nothing the workspace picked up is left behind as a head.
	if revs, err := r.Log(ctx, "all() ~ root()", 20); err != nil || len(revs) != 4 {
		t.Errorf("the log has %d revisions (%v), want 4", len(revs), err)
	}

	// Neither change was rewritten by being checked out.
	for _, rev := range []string{first, second} {
		if _, err := r.read(ctx, "log", "-r", rev, "--no-graph", "-T", "commit_id"); err != nil {
			t.Errorf("%s is no longer there: %v", rev, err)
		}
	}
	out, err := r.read(ctx, "log", "-r", "subject(second)", "--no-graph", "-T", "commit_id")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != revs[0].CommitIDFull {
		t.Errorf("the second change was rewritten to %s", out)
	}

	// A directory that went away is added again rather than refused.
	if err := os.RemoveAll(ws); err != nil {
		t.Fatal(err)
	}
	if err := r.Workspace(ctx, ws, first); err != nil {
		t.Fatalf("re-adding the workspace: %v", err)
	}
	body, _ = os.ReadFile(filepath.Join(ws, "main.go"))
	if string(body) != "one\n" {
		t.Errorf("after re-adding main.go = %q", body)
	}
}
