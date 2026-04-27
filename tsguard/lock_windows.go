//go:build windows

package main

import (
	"golang.org/x/sys/windows"
)

const stillActive = 259 // STILL_ACTIVE / STATUS_PENDING

// isAlive reports whether pid belongs to a live process.
// Uses OpenProcess + GetExitCodeProcess; os.FindProcess is NOT sufficient
// on Windows (it always succeeds regardless of whether the pid exists).
func isAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle) //nolint:errcheck
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}
