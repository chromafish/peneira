package proc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func sh(t *testing.T, ctx context.Context, script string) io.ReadCloser {
	t.Helper()
	r, err := Stream(ctx, t.TempDir(), "/bin/sh", "-c", script)
	if err != nil {
		t.Fatalf("starting the script: %v", err)
	}
	return r
}

func TestOutputIsReadableWhileItIsWritten(t *testing.T) {
	r := sh(t, t.Context(), "printf 'one\\ntwo\\n'")
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if cerr := r.Close(); cerr != nil {
		t.Fatalf("close: %v", cerr)
	}
	if string(out) != "one\ntwo\n" {
		t.Errorf("read %q", out)
	}
}

// A command that writes something and then fails has still failed. Reporting
// only what arrived would show a change with a piece missing and nothing to
// say so.
func TestPartialOutputFollowedByAFailureIsReported(t *testing.T) {
	r := sh(t, t.Context(), "printf 'half\\n'; echo 'it went wrong' >&2; exit 3")
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if string(out) != "half\n" {
		t.Errorf("read %q, want the part that was written", out)
	}
	cerr := r.Close()
	if cerr == nil {
		t.Fatal("a command that exited 3 was reported as success")
	}
	if !strings.Contains(cerr.Error(), "it went wrong") {
		t.Errorf("close reported %q, want what the command printed", cerr)
	}
}

// A command killed by a broken pipe has also failed, however the pipe broke.
func TestDeathBySignalIsReported(t *testing.T) {
	r := sh(t, t.Context(), "printf 'half\\n'; kill -PIPE $$")
	if _, err := io.ReadAll(r); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if err := r.Close(); err == nil {
		t.Fatal("a command killed by SIGPIPE was reported as success")
	}
}

func TestCancellingStopsTheCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	r := sh(t, ctx, "printf 'start\\n'; sleep 60")

	buf := make([]byte, 6)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatalf("reading the first line: %v", err)
	}
	cancel()

	done := make(chan error, 1)
	go func() {
		io.Copy(io.Discard, r)
		done <- r.Close()
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a cancelled command was reported as success")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the reader never came back after the command was cancelled")
	}
}

// A child of the command holding the pipe open must not wedge the reader: the
// pipe is closed on cancellation and Wait is bounded, so Close always returns.
func TestAGrandchildHoldingThePipeDoesNotWedgeTheReader(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	r := sh(t, ctx, "printf 'start\\n'; sleep 60 & sleep 60")

	buf := make([]byte, 6)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatalf("reading the first line: %v", err)
	}
	cancel()

	done := make(chan error, 1)
	go func() {
		io.Copy(io.Discard, r)
		done <- r.Close()
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a cancelled command was reported as success")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the reader is still waiting on a pipe the command's child holds")
	}
}

// Every command is written to the behaviour log with what it was asked and
// how it ended, whichever way it was run.
func TestEveryCommandReachesTheLog(t *testing.T) {
	var buf bytes.Buffer
	was := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(was) })

	if _, err := Run(t.Context(), t.TempDir(), "/bin/sh", "-c", "exit 3"); err == nil {
		t.Fatal("exit 3 was reported as success")
	}
	r := sh(t, t.Context(), "echo streamed")
	io.Copy(io.Discard, r)
	r.Close()
	Stream(t.Context(), t.TempDir(), "peneira-no-such-command")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines:\n%s", len(lines), buf.String())
	}
	for i, want := range []string{
		`msg="command ran" cmd=sh args="-c exit 3" dir=`,
		`msg="command ran" cmd=sh args="-c echo streamed" dir=`,
		`msg="command ran" cmd=peneira-no-such-command args="" dir=`,
	} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want it to hold %q", i, lines[i], want)
		}
	}
	if !strings.HasSuffix(lines[0], "exit=3") || !strings.HasSuffix(lines[1], "exit=0") {
		t.Errorf("exit statuses:\n%s", buf.String())
	}
	if !strings.Contains(lines[2], "err=") {
		t.Errorf("a command that could not start has no err: %q", lines[2])
	}
}

func TestAMissingCommandIsReportedAtTheStart(t *testing.T) {
	_, err := Stream(t.Context(), t.TempDir(), "peneira-no-such-command")
	if err == nil {
		t.Fatal("starting a command that does not exist succeeded")
	}
	var ee *exec.Error
	if !errors.As(err, &ee) {
		t.Errorf("error is %v, want one naming the command", err)
	}
}
