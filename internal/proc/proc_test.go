package proc

import (
	"context"
	"errors"
	"io"
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
