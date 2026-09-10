//go:build linux

package tracer

import (
	"os"
	"syscall"
)

const journalLockingSupported = true

// All live and retired sinks hold this lock until their descriptors close.
// The local Worker may archive only after acquiring an exclusive lock.
func lockJournal(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
}
