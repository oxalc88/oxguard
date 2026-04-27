package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// acquireLock creates .tsguard.pid at root. Returns (release, nil) on success.
// Returns (nil, err) if another live tsguard instance is detected.
// Stale locks (dead pid) are removed and retried once automatically.
func acquireLock(root string) (func(), error) {
	lockPath := filepath.Join(root, ".tsguard.pid")
	return acquireLockAt(lockPath)
}

func acquireLockAt(lockPath string) (func(), error) {
	for range 2 {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			return writePidAndRelease(f, lockPath)
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("cannot create lock file %s: %w", lockPath, err)
		}

		data, _ := os.ReadFile(lockPath)
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr != nil || !isAlive(pid) {
			_ = os.Remove(lockPath)
			continue
		}
		return nil, fmt.Errorf("another tsguard instance is already running (pid %d); refusing to start", pid)
	}
	return nil, fmt.Errorf("cannot acquire lock at %s after retry", lockPath)
}

func writePidAndRelease(f *os.File, lockPath string) (func(), error) {
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		f.Close()           //nolint:errcheck
		os.Remove(lockPath) //nolint:errcheck
		return nil, fmt.Errorf("cannot write lock file: %w", err)
	}
	f.Close() //nolint:errcheck
	return func() { os.Remove(lockPath) }, nil //nolint:errcheck
}
