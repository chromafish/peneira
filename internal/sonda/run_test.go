package sonda

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func shell(t *testing.T, script string) Target {
	t.Helper()
	return Target{Name: "t", Dir: t.TempDir(), Run: []string{"/bin/sh", "-c", script}}
}

// wait blocks until the run ends or the test has waited long enough to call
// it wedged.
func wait(t *testing.T, r *Run) State {
	t.Helper()
	select {
	case <-r.Done():
	case <-time.After(30 * time.Second):
		t.Fatal("the run never ended")
	}
	return r.State()
}

// The two streams are read apart, so which of two lines written to different
// streams is read first is not something a test can rely on. Within a stream
// the order is the program's, and the stamps agree with the list.
func TestLinesArriveInOrderWithTheirStreams(t *testing.T) {
	r := Start(shell(t, "echo one; echo two >&2; printf 'three'"), "x", nil)
	st := wait(t, r)
	if st.Ended != "exited 0" {
		t.Errorf("ended %q", st.Ended)
	}
	logs, _ := r.Sync(nil)
	if len(logs) != 3 {
		t.Fatalf("got %d logs: %+v", len(logs), logs)
	}
	var out, errs []string
	for _, l := range logs {
		if l.Stream == Stderr {
			errs = append(errs, l.Message)
		} else {
			out = append(out, l.Message)
		}
	}
	// The fragment without a newline is emitted as it stands once the
	// process has ended.
	if strings.Join(out, ",") != "one,three" {
		t.Errorf("stdout = %v", out)
	}
	if strings.Join(errs, ",") != "two" {
		t.Errorf("stderr = %v", errs)
	}
	for i := 1; i < len(logs); i++ {
		if logs[i].At.Before(logs[i-1].At) {
			t.Errorf("log %d arrived before log %d", i, i-1)
		}
	}
}

func TestOutputIsHandedOnWhileTheProcessRuns(t *testing.T) {
	woke := make(chan struct{}, 64)
	r := Start(shell(t, "echo early; sleep 30"), "x", func() {
		select {
		case woke <- struct{}{}:
		default:
		}
	})
	defer r.Stop()
	deadline := time.After(10 * time.Second)
	for {
		if logs, _ := r.Sync(nil); len(logs) > 0 {
			if logs[0].Message != "early" {
				t.Errorf("read %q", logs[0].Message)
			}
			return
		}
		select {
		case <-woke:
		case <-deadline:
			t.Fatal("the first line was held until exit")
		}
	}
}

// Stopping takes the whole process group, so a child the program left behind
// does not keep running with nothing to say which run it belongs to.
func TestStopTakesTheProcessGroup(t *testing.T) {
	r := Start(shell(t, "sleep 60 & echo $!; wait"), "x", nil)
	var child int
	deadline := time.Now().Add(10 * time.Second)
	for child == 0 {
		if logs, _ := r.Sync(nil); len(logs) > 0 {
			n, err := strconv.Atoi(strings.TrimSpace(logs[0].Message))
			if err != nil {
				t.Fatalf("the child's pid came back as %q", logs[0].Message)
			}
			child = n
		}
		if time.Now().After(deadline) {
			t.Fatal("the script never printed its child's pid")
		}
		time.Sleep(5 * time.Millisecond)
	}

	r.Stop()
	st := wait(t, r)
	if st.Ended != "stopped" {
		t.Errorf("ended %q, want stopped", st.Ended)
	}
	for time.Now().Before(deadline) {
		if err := syscall.Kill(child, 0); err == syscall.ESRCH {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("the child %d outlived the run", child)
}

func TestAProgramThatIgnoresTheStopIsKilled(t *testing.T) {
	r := Start(shell(t, "trap '' TERM; echo ready; while :; do sleep 1; done"), "x", nil)
	deadline := time.Now().Add(10 * time.Second)
	for logs, _ := r.Sync(nil); len(logs) == 0; logs, _ = r.Sync(nil) {
		if time.Now().After(deadline) {
			t.Fatal("the script never became ready")
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.Stop()
	if st := wait(t, r); !strings.HasPrefix(st.Ended, "killed") {
		t.Errorf("ended %q, want it killed", st.Ended)
	}
}

func TestAFailingBuildIsReportedByWhatItPrinted(t *testing.T) {
	tgt := shell(t, "echo never")
	tgt.Build = []string{"/bin/sh", "-c", "echo 'main.go:3: undefined: x' >&2; exit 2"}
	r := Start(tgt, "x", nil)
	st := wait(t, r)
	if st.Ended != "build failed" {
		t.Errorf("ended %q", st.Ended)
	}
	logs, _ := r.Sync(nil)
	if len(logs) != 1 || logs[0].Stream != Build || logs[0].Message != "main.go:3: undefined: x" {
		t.Errorf("logs = %+v, want the compiler's line", logs)
	}
}

func TestABuildThatPassesIsNotReported(t *testing.T) {
	tgt := shell(t, "echo ran")
	tgt.Build = []string{"/bin/sh", "-c", "echo building"}
	r := Start(tgt, "x", nil)
	wait(t, r)
	logs, _ := r.Sync(nil)
	if len(logs) != 1 || logs[0].Message != "ran" {
		t.Errorf("logs = %+v, want only what the program wrote", logs)
	}
}

func TestTheEnvironmentAndDirectoryAreTheTargets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "here"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tgt := Target{Name: "t", Dir: dir, Run: []string{"/bin/sh", "-c", "ls here; echo $SONDA_TEST"}, Env: map[string]string{"SONDA_TEST": "set"}}
	r := Start(tgt, "x", nil)
	wait(t, r)
	logs, _ := r.Sync(nil)
	if len(logs) != 2 || logs[0].Message != "here" || logs[1].Message != "set" {
		t.Errorf("logs = %+v", logs)
	}
}

func TestACommandThatCannotStartSaysSo(t *testing.T) {
	r := Start(Target{Name: "t", Dir: t.TempDir(), Run: []string{"peneira-no-such-command"}}, "x", nil)
	st := wait(t, r)
	if !strings.HasPrefix(st.Ended, "cannot start") {
		t.Errorf("ended %q", st.Ended)
	}
}

// A copy handed out earlier is brought up to date rather than rebuilt, and a
// line that continued a log already handed out reaches the copy, and is
// reported.
func TestSyncCorrectsWhatGrew(t *testing.T) {
	r := Start(shell(t, `echo 'level=info msg=start'; echo '  detail'; echo done`), "x", nil)
	wait(t, r)
	logs, grown := r.Sync(nil)
	if len(logs) != 2 || len(grown) != 0 {
		t.Fatalf("got %d logs (%d grown): %+v", len(logs), len(grown), logs)
	}
	if logs[0].Message != "start\n  detail" {
		t.Errorf("first = %q", logs[0].Message)
	}
	again, _ := r.Sync(logs[:1])
	if len(again) != 2 || again[0].Message != "start\n  detail" || again[1].Message != "done" {
		t.Errorf("resynced = %+v", again)
	}

	// Handed the first log alone before the continuation arrived, the copy is
	// told which log grew.
	held := []Log{parse("level=info msg=start")}
	r.mu.Lock()
	r.changed = append(r.changed, 0)
	r.mu.Unlock()
	got, grown := r.Sync(held)
	if len(grown) != 1 || grown[0] != 0 || got[0].Message != "start\n  detail" {
		t.Errorf("grown = %v, first = %q", grown, got[0].Message)
	}
}

// A line on one stream arriving in the middle of a trace on the other does
// not cut the trace short: each stream continues its own newest log.
func TestEachStreamContinuesItsOwnLog(t *testing.T) {
	r := Start(shell(t, `echo 'panic: boom' >&2; echo '{"level":"info","msg":"between"}'; sleep 0.2; echo 'goroutine 1 [running]:' >&2; printf '\tmain.go:3\n' >&2`), "x", nil)
	wait(t, r)
	logs, _ := r.Sync(nil)
	if len(logs) != 2 {
		t.Fatalf("got %d logs: %+v", len(logs), logs)
	}
	var trace *Log
	for i := range logs {
		if logs[i].Stream == Stderr {
			trace = &logs[i]
		}
	}
	if trace == nil || trace.Raw != "panic: boom\ngoroutine 1 [running]:\n\tmain.go:3" {
		t.Errorf("trace = %+v", trace)
	}
}
