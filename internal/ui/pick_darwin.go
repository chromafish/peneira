//go:build darwin

package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

// Implemented in Go, in pick_darwin_done.go.
extern void reviewPickDone(uintptr_t handle, char *path);

// reviewChooseDirectory shows the open panel and calls reviewPickDone with the
// chosen path, or NULL if the panel was dismissed. It returns as soon as the
// panel is scheduled.
//
// The panel is opened on a fresh turn of the main run loop rather than inside
// whatever callback asked for it. runModal spins a nested run loop, and
// nesting one inside the window driver's own event handling is what made the
// panel intermittently fail to appear: the request has to wait for a turn the
// driver is not already using.
static void reviewChooseDirectory(const char *start, uintptr_t handle) {
	char *dir = (start != NULL && start[0] != '\0') ? strdup(start) : NULL;

	dispatch_async(dispatch_get_main_queue(), ^{
		@autoreleasepool {
			NSOpenPanel *panel = [NSOpenPanel openPanel];
			panel.canChooseFiles = NO;
			panel.canChooseDirectories = YES;
			panel.allowsMultipleSelection = NO;
			panel.canCreateDirectories = NO;
			panel.message = @"Choose a repository to review";
			panel.prompt = @"Open";
			if (dir != NULL) {
				panel.directoryURL = [NSURL fileURLWithPath:[NSString stringWithUTF8String:dir]
				                                isDirectory:YES];
				free(dir);
			}

			// Without this the panel can open behind the window, or not come
			// forward at all, when the application is not the active one —
			// which looks exactly like the command having done nothing.
			[NSApp activateIgnoringOtherApps:YES];

			char *out = NULL;
			if ([panel runModal] == NSModalResponseOK && panel.URL.path != nil) {
				out = strdup([panel.URL.path UTF8String]);
			}
			reviewPickDone(handle, out);
		}
	});
}
*/
import "C"

import (
	"sync"
	"unsafe"
)

// hasNativePicker reports that this platform can show a directory chooser
// in-process, without spawning a helper program.
const hasNativePicker = true

// Panels in flight, keyed by a handle the completion callback hands back. A Go
// pointer cannot be parked in C, so the channel waiting for each answer lives
// here and only the handle crosses over.
var (
	pickMu   sync.Mutex
	pickLast uintptr
	pickWait = map[uintptr]chan string{}
)

// nativePick shows the system open panel and waits for the answer. It must be
// called from a background goroutine, never from the thread running the window
// loop: the panel needs that thread, and waiting for it there would deadlock.
func nativePick(start string) (string, bool) {
	ch := make(chan string, 1)
	pickMu.Lock()
	pickLast++
	handle := pickLast
	pickWait[handle] = ch
	pickMu.Unlock()

	cstart := C.CString(start)
	C.reviewChooseDirectory(cstart, C.uintptr_t(handle))
	C.free(unsafe.Pointer(cstart))

	path := <-ch
	return path, path != ""
}
