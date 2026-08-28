//go:build darwin

package ui

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import "unsafe"

// reviewPickDone receives the chosen path from the open panel. It is a
// separate file because cgo forbids C definitions in the preamble of a file
// that exports a function, and the panel itself is a definition.
//
//export reviewPickDone
func reviewPickDone(handle C.uintptr_t, path *C.char) {
	out := ""
	if path != nil {
		out = C.GoString(path)
		C.free(unsafe.Pointer(path))
	}
	pickMu.Lock()
	ch := pickWait[uintptr(handle)]
	delete(pickWait, uintptr(handle))
	pickMu.Unlock()
	// The channel is buffered: this runs on the main thread, which must not be
	// held up by whoever is waiting for the path.
	if ch != nil {
		ch <- out
	}
}
