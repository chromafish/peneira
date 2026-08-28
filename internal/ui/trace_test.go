package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The trace is only worth having if it writes: a diagnostic that stays silent
// during the fault it was built for is worse than none.
func TestTraceReportsSlowFramesAndGaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.log")
	t.Setenv("REVIEW_TRACE", path)

	traceSlow, traceHang = time.Millisecond, time.Hour
	t.Cleanup(func() {
		traceSlow, traceHang = 150*time.Millisecond, 700*time.Millisecond
		traceOn, traceTo = false, nil
	})

	startTrace()
	if !traceOn {
		t.Fatal("tracing did not start")
	}

	// A frame that takes longer than the threshold, then a gap before the next.
	done := traceFrame()
	busy(2 * time.Millisecond)
	done(time.Millisecond)

	busy(3 * time.Millisecond)
	traceFrame()(0)

	log, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the trace file was not written: %v", err)
	}
	out := string(log)
	for _, want := range []string{"tracing:", "slow frame:", "gap: no frame for"} {
		if !strings.Contains(out, want) {
			t.Errorf("the trace is missing %q:\n%s", want, out)
		}
	}
}

// busy spins for d, so the test does not sleep its way through the clock.
func busy(d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
	}
}
