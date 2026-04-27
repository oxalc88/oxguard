//go:build !windows

package main

import "syscall"

// isAlive reports whether pid belongs to a live process.
// Uses signal 0: succeeds for live processes, returns ESRCH for dead ones.
func isAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil
}
