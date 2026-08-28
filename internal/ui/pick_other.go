//go:build !darwin

package ui

// hasNativePicker is false everywhere the chooser has to be a helper program.
const hasNativePicker = false

func nativePick(string) (string, bool) { return "", false }
