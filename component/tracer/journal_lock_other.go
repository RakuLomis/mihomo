//go:build !linux

package tracer

import "os"

const journalLockingSupported = false

func lockJournal(file *os.File) error { return nil }
