package sonda

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Grace is how long a process is given to leave after being asked, before it
// is killed.
const Grace = 3 * time.Second

// waitDelay bounds how long a run waits, once the process has exited, on
// pipes a child of it still holds open.
const waitDelay = 2 * time.Second

// Phase is where a run is in its life.
type Phase uint8

const (
	Building Phase = iota
	Running
	Ended
)

// Run is one execution of a target at one revision, start to exit.
//
// A run is driven from its own goroutines and read from another: State and
// Sync take snapshots, and everything else is internal.
type Run struct {
	Target Target
	Rev    string
	Began  time.Time

	notify func()

	mu      sync.Mutex
	phase   Phase
	ended   string
	logs    []Log
	cmd     *exec.Cmd
	stopped bool
	cancel  context.CancelFunc
	done    chan struct{}

	// last is the newest log of each stream, which is the one a line on
	// that stream can continue, and changed lists the logs a line has been
	// joined to since Sync last looked.
	last    [3]int
	changed []int

	// formats counts the logs by how far each was understood, for the
	// behaviour log's account of the run.
	formats [3]int
}

// State is a snapshot of a run's progress.
type State struct {
	Phase Phase
	// Ended says how the process ended, once it has: its exit status, the
	// signal that took it, or that it was stopped, or that it never started.
	Ended string
	Count int
}

// Start builds the target, if it has a build command, and starts it. Each
// line read and the end of the process is followed by a call
// to notify, from whichever goroutine made it.
//
// rev names the revision the run is made at; the package does not check out
// anything itself, and only records the name.
func Start(t Target, rev string, notify func()) *Run {
	if notify == nil {
		notify = func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Run{
		Target: t,
		Rev:    rev,
		Began:  time.Now(),
		notify: notify,
		cancel: cancel,
		done:   make(chan struct{}),
		last:   [3]int{-1, -1, -1},
	}
	go r.run(ctx)
	return r
}

func (r *Run) run(ctx context.Context) {
	defer close(r.done)

	if len(r.Target.Build) > 0 {
		slog.Info("build started", "target", r.Target.Name, "rev", r.Rev, "cmd", strings.Join(r.Target.Build, " "))
		out, err := build(ctx, r.Target)
		if err != nil {
			if ctx.Err() != nil {
				r.finish("stopped while building", false)
				return
			}
			slog.Error("build exited", "target", r.Target.Name, "rev", r.Rev, "exit", exitCode(err))
			r.report(out)
			r.finish("build failed", false)
			return
		}
		slog.Info("build exited", "target", r.Target.Name, "rev", r.Rev, "exit", 0)
	}

	cmd := exec.Command(r.Target.Run[0], r.Target.Run[1:]...)
	cmd.Dir = r.Target.Dir
	cmd.Env = r.Target.environ()
	cmd.WaitDelay = waitDelay
	out := &splitter{run: r, stream: Stdout}
	errs := &splitter{run: r, stream: Stderr}
	cmd.Stdout, cmd.Stderr = out, errs
	inGroup(cmd)

	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		r.finish("stopped before it started", false)
		return
	}
	if err := cmd.Start(); err != nil {
		r.mu.Unlock()
		r.finish("cannot start: "+err.Error(), true)
		return
	}
	r.cmd, r.phase = cmd, Running
	r.mu.Unlock()
	slog.Info("run started", "target", r.Target.Name, "rev", r.Rev, "cmd", strings.Join(r.Target.Run, " "))
	r.notify()

	err := cmd.Wait()
	out.flush()
	errs.flush()
	how, failed := r.outcome(err)
	r.finish(how, failed)
}

// exitCode is what a command exited with, or -1 where it did not exit on
// its own.
func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// build runs the build command against the working copy as it stands, and
// returns what it printed. A failure is reported by that output, never by the
// exit status alone.
func build(ctx context.Context, t Target) ([]byte, error) {
	cmd := exec.CommandContext(ctx, t.Build[0], t.Build[1:]...)
	cmd.Dir = t.Dir
	cmd.Env = t.environ()
	cmd.WaitDelay = waitDelay
	inGroup(cmd)
	cmd.Cancel = func() error { return signalGroup(cmd, syscall.SIGKILL) }
	out, err := cmd.CombinedOutput()
	if err != nil && len(bytes.TrimSpace(out)) == 0 {
		out = []byte(err.Error())
	}
	return out, err
}

// report records a failed build's output as logs, so it is read where the
// program's own output would have been.
func (r *Run) report(out []byte) {
	r.mu.Lock()
	now := time.Now()
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		r.logs = append(r.logs, Log{At: now, Stream: Build, Raw: line, Format: Plain, Message: line})
	}
	r.mu.Unlock()
}

// finish records how the run ended and writes its account to the behaviour
// log: how it ended, and how much of what it wrote was understood. A run
// that failed of its own accord is an error; one that was stopped is not.
func (r *Run) finish(how string, failed bool) {
	r.mu.Lock()
	r.phase, r.ended = Ended, how
	formats := r.formats
	r.mu.Unlock()
	level := slog.LevelInfo
	if failed {
		level = slog.LevelError
	}
	slog.Log(context.Background(), level, "run exited", "target", r.Target.Name, "rev", r.Rev, "how", how,
		"json", formats[JSON], "logfmt", formats[Logfmt], "plain", formats[Plain])
	r.notify()
}

// outcome says how the process ended, and whether that counts as a failure.
// A stop is reported as one rather than as the signal that carried it, except
// where the process had to be killed, which is worth knowing about a program.
func (r *Run) outcome(err error) (string, bool) {
	r.mu.Lock()
	stopped := r.stopped
	r.mu.Unlock()
	if err == nil {
		return "exited 0", false
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return err.Error(), true
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		switch {
		case stopped && ws.Signal() == syscall.SIGKILL:
			return "killed after ignoring a stop", false
		case stopped:
			return "stopped", false
		}
		return "killed by " + ws.Signal().String(), true
	}
	return strings.Replace(ee.Error(), "exit status", "exited", 1), true
}

// inGroup puts the command in a process group of its own, so that a program
// which spawns children can be stopped as a whole.
func inGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, sig)
}

// Stop asks the process to terminate, as a group, and kills whatever is left
// of it after Grace. It returns at once; Done reports the end.
func (r *Run) Stop() {
	r.mu.Lock()
	if r.stopped || r.phase == Ended {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	cmd := r.cmd
	r.mu.Unlock()

	r.cancel()
	if cmd == nil {
		return
	}
	signalGroup(cmd, syscall.SIGTERM)
	go func() {
		select {
		case <-r.done:
		case <-time.After(Grace):
			signalGroup(cmd, syscall.SIGKILL)
		}
	}()
}

// Done is closed once the run has ended, however it ended.
func (r *Run) Done() <-chan struct{} { return r.done }

// State takes a snapshot of the run's progress.
func (r *Run) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return State{Phase: r.phase, Ended: r.ended, Count: len(r.logs)}
}

// Sync brings a copy of the run's logs up to date and returns it, along
// with the indices of logs handed out before that have grown since, by a
// line continuing them. It serves one reader: what has grown is reported
// once.
func (r *Run) Sync(dst []Log) ([]Log, []int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(dst)
	if n > len(r.logs) {
		n = 0
		dst = dst[:0]
	}
	var grown []int
	for _, i := range r.changed {
		if i < n {
			dst[i] = r.logs[i]
			grown = append(grown, i)
		}
	}
	r.changed = r.changed[:0]
	return append(dst, r.logs[n:]...), grown
}

// line takes one complete line from a stream. The arrival stamp is taken
// under the lock, so the order of the logs and the order of their stamps
// agree. The two streams are read apart, so a line on one can arrive in
// the middle of a trace on the other; each stream continues its own newest
// log.
func (r *Run) line(stream Stream, text string) {
	text = strings.TrimSuffix(text, "\r")
	l := Parse(text)
	l.Stream = stream
	r.mu.Lock()
	l.At = time.Now()
	if i := r.last[stream]; i >= 0 && continues(&r.logs[i], l) {
		r.logs[i].join(text)
		r.changed = append(r.changed, i)
	} else {
		r.logs = append(r.logs, l)
		r.last[stream] = len(r.logs) - 1
		r.formats[l.Format]++
	}
	r.mu.Unlock()
}

// splitter is one of the process's output streams. It is given the bytes as
// the process writes them and hands on every complete line at once; a
// trailing fragment is held until the next write completes it or the process
// ends.
type splitter struct {
	run    *Run
	stream Stream
	frag   []byte
}

func (s *splitter) Write(p []byte) (int, error) {
	data := p
	if len(s.frag) > 0 {
		data = append(s.frag, p...)
	}
	lines := 0
	for {
		nl := bytes.IndexByte(data, '\n')
		if nl < 0 {
			break
		}
		s.run.line(s.stream, string(data[:nl]))
		data = data[nl+1:]
		lines++
	}
	// The buffer handed to Write belongs to the copier and is reused, so
	// what is held over has to be copied out of it.
	s.frag = append(s.frag[:0], data...)
	if lines > 0 {
		s.run.notify()
	}
	return len(p), nil
}

// flush emits a trailing fragment as it stands, once nothing more can arrive.
func (s *splitter) flush() {
	if len(s.frag) == 0 {
		return
	}
	s.run.line(s.stream, string(s.frag))
	s.frag = nil
}
