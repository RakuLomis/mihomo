//go:build linux

package tracer

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestJournalLockPersistsUntilRetiredReferencesDrain(t *testing.T) {
	tr := newTracer(&bytes.Buffer{})
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := tr.configure(ConfigPatch{Enabled: boolPtr(true), Output: &path}); err != nil {
		t.Fatal(err)
	}
	sink := tr.acquireSink()
	boundary, err := tr.barrier()
	if err != nil || !boundary.JournalLocking {
		t.Fatalf("boundary: %+v, %v", boundary, err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	lock := func() error { return syscall.Flock(int(reader.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) }
	if err := lock(); err == nil {
		t.Fatal("active journal was not locked")
	}
	if err := tr.configure(ConfigPatch{Output: stringPtr("")}); err != nil {
		t.Fatal(err)
	}
	if err := lock(); err == nil {
		t.Fatal("retired journal with references was not locked")
	}
	tr.releaseSink(sink)
	if err := lock(); err != nil {
		t.Fatal(err)
	}
	if err := tr.configure(ConfigPatch{Output: &path}); err == nil {
		t.Fatal("core reopened exclusively locked journal")
	}
	if tr.sink.output != "" {
		t.Fatal("failed reconfiguration changed current sink")
	}
}
