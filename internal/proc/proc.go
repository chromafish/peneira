// Package proc runs a command and hands back its output as it is written.
package proc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"time"
)

// waitDelay bounds how long Close waits on a command that will not go, or on
// pipes something else is still holding open.
const waitDelay = 5 * time.Second

// Run executes a command in dir and returns what it wrote to standard output.
// A command that fails is reported by what it printed on standard error, which
// is the message a person would have seen at a terminal.
func Run(ctx context.Context, dir, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, said(err, &stderr)
	}
	return stdout.Bytes(), nil
}

// Clean turns an exec failure into the message the command actually printed,
// for the calls that did not capture its output.
func Clean(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
			return errors.New(msg)
		}
	}
	return err
}

// said prefers what the command printed over the exit status it died with.
func said(err error, stderr *bytes.Buffer) error {
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		return errors.New(msg)
	}
	return err
}

// Stream starts a command in dir and returns its standard output, which the
// caller reads while the command is still producing it.
//
// The reader must be closed: closing is what reaps the command, and so what
// reports a failure the command signals only by exiting.
func Stream(ctx context.Context, dir, bin string, args ...string) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// Killing the command does not free a read blocked on a pipe one of its
	// own children still holds open, and Wait would then block behind that
	// read. Closing the pipe is what ends it; WaitDelay bounds the rest.
	cmd.Cancel = func() error {
		err := cmd.Process.Kill()
		out.Close()
		return err
	}
	cmd.WaitDelay = waitDelay
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &output{out: out, cmd: cmd, stderr: &stderr}, nil
}

type output struct {
	out    io.ReadCloser
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func (o *output) Read(b []byte) (int, error) { return o.out.Read(b) }

// Close reaps the command. Every way it can fail is reported, including the
// ones that leave usable output behind: a diff cut short is not a diff, and
// has to be reported rather than passed off as a change with a piece missing.
func (o *output) Close() error {
	o.out.Close()
	err := o.cmd.Wait()
	if err == nil {
		return nil
	}
	return said(err, o.stderr)
}
