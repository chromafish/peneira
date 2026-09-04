package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/chromafish/peneira/internal/sonda"
)

// declareTarget writes a target into the repository whose program is a shell
// script, so a test can decide what each run writes.
func declareTarget(t *testing.T, dir, script string) {
	t.Helper()
	body := "[[target]]\nname = \"probe\"\nrun = [\"/bin/sh\", \"-c\", " + strconv.Quote(script) + "]\n"
	if err := os.WriteFile(filepath.Join(dir, sonda.File), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// until draws frames until a condition holds. A run reports through frames,
// so the condition is read after each one.
func (h *harness) until(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		h.frame()
		if cond() {
			h.frame()
			return
		}
		if time.Now().After(deadline) {
			var panes []string
			if s := h.app.sonda; s != nil {
				for i := range s.panes {
					p := s.panes[i]
					panes = append(panes, p.label+": run="+strconv.FormatBool(p.run != nil)+
						" phase="+strconv.Itoa(int(p.state.Phase))+" ended="+p.state.Ended+" note="+p.note)
				}
			}
			h.t.Fatalf("%s never happened; failure=%q status=%q %s", what, h.app.failure, h.app.status, strings.Join(panes, "; "))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (h *harness) sondaEnded() bool {
	s := h.app.sonda
	for i := range s.panes {
		if s.panes[i].run == nil || s.panes[i].state.Phase != sonda.Ended {
			return false
		}
	}
	return true
}

func messages(logs []sonda.Log) string {
	var out []string
	for _, l := range logs {
		out = append(out, l.Message)
	}
	return strings.Join(out, "\n")
}

// The baseline runs from a second working copy at the parent of the change,
// the review from the working copy as it stands, and what each printed says
// which is which.
func testSondaRunsBothRevisions(t *testing.T, h *harness) {
	t.Helper()
	root := h.app.repo.Root()
	declareTarget(t, root, "cat main.go")

	h.press("S", 0)
	if !h.app.sondaOpen() {
		t.Fatal("S did not open the sonda screen")
	}
	if n := len(h.app.sonda.targets); n != 1 {
		t.Fatalf("read %d targets, want the one declared", n)
	}

	h.press("R", 0)
	h.until("both runs ending", h.sondaEnded)

	base, rev := h.app.sonda.panes[paneBaseline], h.app.sonda.panes[paneReview]
	if got := messages(base.logs); !strings.Contains(got, `println("one")`) || strings.Contains(got, `"two"`) {
		t.Errorf("the baseline printed:\n%s\nwant the parent's main.go", got)
	}
	if got := messages(rev.logs); !strings.Contains(got, `println("two")`) {
		t.Errorf("the review printed:\n%s\nwant the working copy's main.go", got)
	}
	if base.rev != h.app.rev.Parents[0] {
		t.Errorf("the baseline ran at %q, want the parent %q", base.rev, h.app.rev.Parents[0])
	}
	if base.state.Ended != "exited 0" || rev.state.Ended != "exited 0" {
		t.Errorf("runs ended %q and %q", base.state.Ended, rev.state.Ended)
	}

	// The second working copy is outside the repository at a stable path,
	// and running again moves it rather than making another.
	ws, err := sonda.WorkspaceDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(ws, root) {
		t.Errorf("the workspace %s is inside the repository", ws)
	}
	if _, err := os.Stat(ws); err != nil {
		t.Fatalf("no workspace at %s: %v", ws, err)
	}
	h.press("R", 0)
	h.until("the second pair of runs ending", h.sondaEnded)
	if got := messages(h.app.sonda.panes[paneBaseline].logs); !strings.Contains(got, `println("one")`) {
		t.Errorf("the second baseline run printed:\n%s", got)
	}

	// Back to the review, with the runs where they were.
	h.press(key.NameEscape, 0)
	if h.app.sondaOpen() {
		t.Error("escape did not leave the screen")
	}
	if h.app.sonda.panes[paneBaseline].run == nil {
		t.Error("leaving the screen dropped the runs")
	}
}

func TestSondaRunsBothRevisions(t *testing.T) {
	testSondaRunsBothRevisions(t, newHarness(t))
}

func TestSondaRunsBothRevisionsOnGit(t *testing.T) {
	testSondaRunsBothRevisions(t, newGitHarness(t))
}

func TestSondaStopsBothRuns(t *testing.T) {
	h := newHarness(t)
	declareTarget(t, h.app.repo.Root(), "echo up; sleep 60")
	h.press("S", 0)
	h.press("R", 0)
	h.until("both programs writing", func() bool {
		s := h.app.sonda
		return len(s.panes[paneBaseline].logs) > 0 && len(s.panes[paneReview].logs) > 0
	})
	for i := range h.app.sonda.panes {
		if p := h.app.sonda.panes[i]; p.state.Phase != sonda.Running {
			t.Errorf("%s is %v, want it running", p.label, p.state.Phase)
		}
	}

	h.press("X", 0)
	h.until("both runs stopping", h.sondaEnded)
	for i := range h.app.sonda.panes {
		if p := h.app.sonda.panes[i]; p.state.Ended != "stopped" {
			t.Errorf("%s ended %q, want stopped", p.label, p.state.Ended)
		}
	}
}

// One filter applies to both panes, so what is on screen stays comparable.
func TestSondaFiltersBothPanesAtOnce(t *testing.T) {
	h := newHarness(t)
	declareTarget(t, h.app.repo.Root(),
		`echo '{"level":"info","msg":"fine"}'; echo '{"level":"error","msg":"bad"}'; echo plain`)
	h.press("S", 0)
	h.press("R", 0)
	h.until("both runs ending", h.sondaEnded)

	shown := func() [2]int {
		var n [2]int
		for i := range h.app.sonda.panes {
			n[i] = len(h.app.sonda.panes[i].view)
		}
		return n
	}
	if got := shown(); got != [2]int{3, 3} {
		t.Fatalf("shown %v, want every line in both panes", got)
	}
	// Levels: info and above keeps everything, since the plain line has no
	// level to be judged by; warn and above drops the info line.
	h.press("L", 0)
	if got := shown(); got != [2]int{3, 3} {
		t.Errorf("at info and above %v, want 3 in each", got)
	}
	h.press("L", 0)
	if got := shown(); got != [2]int{2, 2} {
		t.Errorf("at warn and above %v, want 2 in each", got)
	}
	h.press("L", 0)
	h.press("L", 0)
	if h.app.sonda.minLevel != sonda.Unlevelled {
		t.Errorf("the level did not cycle back to all")
	}

	h.press("/", 0)
	h.typeText("BAD")
	h.frame()
	if got := shown(); got != [2]int{1, 1} {
		t.Errorf("filtered by text %v, want the one matching line in each", got)
	}
	h.press(key.NameEscape, 0)
	if h.app.sonda.filter.Focused() {
		t.Error("escape left the filter focused")
	}
	if !h.app.sondaOpen() {
		t.Error("escape in the filter left the screen")
	}
}

func TestSondaWithoutATargetSaysSo(t *testing.T) {
	h := newHarness(t)
	h.press("S", 0)
	if !h.app.sondaOpen() || len(h.app.sonda.targets) != 0 {
		t.Fatalf("open %v with %d targets", h.app.sondaOpen(), len(h.app.sonda.targets))
	}
	h.press("R", 0)
	if !strings.Contains(h.app.status, sonda.File) {
		t.Errorf("status %q does not say where a target is declared", h.app.status)
	}
	if h.app.sonda.panes[paneReview].run != nil {
		t.Error("a run was started with nothing to run")
	}
}

// A build that fails is reported by what it printed, in the pane the program
// would have written to.
func TestSondaReportsAFailedBuild(t *testing.T) {
	h := newHarness(t)
	body := "[[target]]\nname = \"probe\"\nbuild = [\"/bin/sh\", \"-c\", \"echo 'main.go:3: undefined: x' >&2; exit 1\"]\nrun = [\"true\"]\n"
	if err := os.WriteFile(filepath.Join(h.app.repo.Root(), sonda.File), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h.press("S", 0)
	h.press("R", 0)
	h.until("both builds failing", h.sondaEnded)
	p := h.app.sonda.panes[paneReview]
	if p.state.Ended != "build failed" {
		t.Errorf("ended %q", p.state.Ended)
	}
	if got := messages(p.logs); got != "main.go:3: undefined: x" {
		t.Errorf("the pane holds %q, want the compiler's line", got)
	}
	if !strings.Contains(p.sondaTitle(false), "BUILD FAILED") {
		t.Errorf("title %q", p.sondaTitle(false))
	}
}
