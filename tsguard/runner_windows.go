//go:build windows

package main

import (
	"os/exec"
	"strconv"
)

func setSysProcAttr(_ *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		// Packaged Node adapters have native/Node descendants. Kill the tree,
		// not only the adapter, so timeouts and cancellation leave no tool behind.
		if err := exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run(); err != nil {
			_ = cmd.Process.Kill()
		}
	}
}
