package ui

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
)

// Tracing exists for one kind of question: the interface stopped drawing for a
// while and nothing in the code obviously explains it.
//
// A stall can be in one of two places, and telling them apart is most of the
// work. Either a frame is taking a long time — layout or paint — or frames
// have stopped arriving altogether, which means the window is not asking for
// them and the time is being spent below this program. Both are watched here.
//
// REVIEW_TRACE=1 writes to stderr; REVIEW_TRACE=/some/file writes there, which
// is what to use when the application was launched from the Finder and has no
// terminal attached. It is off unless asked for, and costs two atomic stores
// per frame when it is on.

var (
	traceTo io.Writer
	traceOn bool
)

// These are variables rather than constants so that the test can lower them
// and watch the trace do its job without waiting a second for it.
var (
	// traceSlow is the frame worth printing. A frame that misses a 60Hz
	// refresh is not news; one that misses ten of them is.
	traceSlow = 150 * time.Millisecond
	// traceHang is how long a stall has to last before every goroutine's stack
	// is worth dumping.
	traceHang = 700 * time.Millisecond
)

var (
	frameStart atomic.Int64 // when the running frame began, or 0 between frames
	frameEnd   atomic.Int64 // when the last frame finished
)

// startTrace opens the trace and runs the watchdog. It returns immediately
// when tracing is off.
func startTrace() {
	dest := os.Getenv("REVIEW_TRACE")
	if dest == "" {
		return
	}
	traceTo = os.Stderr
	if dest != "1" && dest != "true" {
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "check: cannot write the trace to %s: %v\n", dest, err)
		} else {
			traceTo = f
		}
	}
	traceOn = true
	tracef("tracing: frames and gaps over %v; kill -USR1 %d dumps every stack",
		traceSlow, os.Getpid())

	go func() {
		var dumped int64
		for range time.Tick(100 * time.Millisecond) {
			// A frame that has not finished: the stall is inside layout or
			// paint, and the stack says which.
			began := frameStart.Load()
			if began == 0 {
				continue
			}
			if stuck := time.Since(time.Unix(0, began)); stuck > traceHang && began != dumped {
				dumped = began
				dump("a frame has been running for " + stuck.Round(time.Millisecond).String())
			}
		}
	}()

	// A gap between frames is not evidence by itself: this interface draws
	// when something asks it to, so any pause looks like one. What tells a
	// stall apart from stillness is what the program is *in* at the time, and
	// the surest way to catch a two second stall by hand is not to try —
	// REVIEW_TRACE_SAMPLE=1 writes every goroutine's stack on a tick, and the
	// samples that span the freeze name the call it is sitting in.
	if every := os.Getenv("REVIEW_TRACE_SAMPLE"); every != "" {
		period := 250 * time.Millisecond
		if d, err := time.ParseDuration(every); err == nil && d > 0 {
			period = d
		}
		tracef("sampling every stack every %v", period)
		go func() {
			buf := make([]byte, 1<<20)
			for t := range time.Tick(period) {
				n := runtime.Stack(buf, true)
				fmt.Fprintf(traceTo, "\n=== sample %s ===\n", t.Format("15:04:05.000"))
				traceTo.Write(buf[:n])
			}
		}()
	}

	// A stall between frames looks exactly like sitting still and reading the
	// screen, so it is not dumped automatically. Ask for it instead, from
	// another terminal, at the moment the window is stuck:
	//
	//	kill -USR1 $(pgrep -x check)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGUSR1)
	go func() {
		for range sig {
			dump("asked for by signal")
		}
	}()
}

// traceFrame brackets one frame. The returned function ends it and reports the
// frame, and the gap before it, when either was long enough to matter.
func traceFrame() func(layout time.Duration) {
	if !traceOn {
		return func(time.Duration) {}
	}
	begin := time.Now()
	if last := frameEnd.Load(); last != 0 {
		if gap := begin.Sub(time.Unix(0, last)); gap > traceSlow {
			tracef("gap: no frame for %v", gap.Round(time.Millisecond))
		}
	}
	frameStart.Store(begin.UnixNano())
	return func(layout time.Duration) {
		now := time.Now()
		frameStart.Store(0)
		frameEnd.Store(now.UnixNano())
		if total := now.Sub(begin); total >= traceSlow {
			tracef("slow frame: layout %v, paint %v, total %v",
				layout.Round(time.Millisecond),
				(total - layout).Round(time.Millisecond),
				total.Round(time.Millisecond))
		}
	}
}

func tracef(format string, args ...any) {
	if traceTo == nil {
		return
	}
	fmt.Fprintf(traceTo, "check %s: "+format+"\n",
		append([]any{time.Now().Format("15:04:05.000")}, args...)...)
}

// dump writes every goroutine's stack, which names the call the program is
// sitting in rather than the one it might be.
func dump(why string) {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	tracef("%s — every goroutine follows", why)
	traceTo.Write(buf[:n])
	if f, ok := traceTo.(*os.File); ok {
		f.Sync()
	}
}
