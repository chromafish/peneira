package ui

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// pickFolder opens a directory chooser and returns what was selected, or
// errPickCancelled if it was dismissed. It must be called from a background
// goroutine: it blocks until the chooser is answered.
//
// Where the platform has an in-process chooser this is the whole
// implementation: spawning a helper program to ask for a path costs seconds,
// which is unacceptable on a path a click sits on.
func pickFolder(ctx context.Context, start string) (string, error) {
	if hasNativePicker {
		path, ok := nativePick(start)
		if !ok {
			return "", errPickCancelled
		}
		return path, nil
	}
	return pickFolderCommand(ctx, start)
}

// pickFolderCommand asks a helper program, for platforms with no in-process
// chooser wired up.
func pickFolderCommand(ctx context.Context, start string) (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		script := `POSIX path of (choose folder with prompt "Choose a repository to review")`
		if start != "" {
			script = `POSIX path of (choose folder with prompt "Choose a repository to review" default location POSIX file ` +
				quoteAppleScript(start) + `)`
		}
		cmd = exec.CommandContext(ctx, "osascript", "-e", script)
	case "windows":
		const ps = `Add-Type -AssemblyName System.Windows.Forms; ` +
			`$d = New-Object System.Windows.Forms.FolderBrowserDialog; ` +
			`if ($d.ShowDialog() -eq 'OK') { Write-Output $d.SelectedPath } else { exit 1 }`
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", ps)
	default:
		if path, err := exec.LookPath("zenity"); err == nil {
			cmd = exec.CommandContext(ctx, path, "--file-selection", "--directory",
				"--title=Choose a repository to review")
		} else if path, err := exec.LookPath("kdialog"); err == nil {
			cmd = exec.CommandContext(ctx, path, "--getexistingdirectory", start)
		} else {
			return "", errors.New("no folder chooser found; install zenity or kdialog, or pass a path on the command line")
		}
	}

	out, err := cmd.Output()
	if err != nil {
		// Every one of these dialogs exits non-zero when dismissed, and there
		// is nothing to report when someone changes their mind.
		return "", errPickCancelled
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", errPickCancelled
	}
	return strings.TrimSuffix(path, "/"), nil
}

var errPickCancelled = errors.New("cancelled")

// quoteAppleScript renders a Go string as an AppleScript string literal.
func quoteAppleScript(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}
