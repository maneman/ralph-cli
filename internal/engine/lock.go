package engine

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// AcquireLock writes the current PID to lockPath. If a non-stale lock already
// exists an error is returned.
func AcquireLock(lockPath string) error {
	if _, err := os.Stat(lockPath); err == nil {
		if isStale(lockPath) {
			_ = os.Remove(lockPath)
		} else {
			data, _ := os.ReadFile(lockPath)
			return fmt.Errorf("another ralph instance is running (PID %s)", strings.TrimSpace(string(data)))
		}
	}
	return os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())), 0o644)
}

// ReleaseLock removes the lock file.
func ReleaseLock(lockPath string) error {
	return os.Remove(lockPath)
}

// isStale returns true when the PID recorded in the lock file is no longer
// running.
func isStale(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return true
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return true
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return false
	}
	// EPERM means the process exists but belongs to another user.
	if errors.Is(err, syscall.EPERM) {
		return false
	}
	return true
}
