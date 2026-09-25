package session

import (
	"net"
	"testing"
)

func TestStreamExposesCarrierSessionID(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	sess := NewClientSession(client, nil)
	sess.seq = 42
	stream := newStream(7, sess)

	if got := stream.CarrierSessionID(); got != 42 {
		t.Fatalf("CarrierSessionID() = %d, want 42", got)
	}
	if stream.id == uint32(stream.CarrierSessionID()) {
		t.Fatal("stream ID must remain distinct from the physical session ID")
	}
}
