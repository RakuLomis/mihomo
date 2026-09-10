package tracer

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBarrierByteBoundarySurvivesLaterWrites(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := tr.configure(ConfigPatch{Enabled: boolPtr(true), Output: stringPtr(path)}); err != nil {
		t.Fatal(err)
	}
	first, err := tr.barrier()
	if err != nil {
		t.Fatal(err)
	}
	second, err := tr.barrier()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.ByteSize <= 0 || second.ByteSize <= first.ByteSize || int64(len(data)) != second.ByteSize {
		t.Fatal("invalid byte boundaries")
	}
	events := decodeEvents(t, data[:first.ByteSize])
	if len(events) != 1 || events[0].EventSeq != first.EventSeq {
		t.Fatal("prefix does not end at first barrier")
	}
}
