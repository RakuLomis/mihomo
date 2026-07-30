package dialer

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/traffictrace"
)

type traceObserver struct {
	observation traffictrace.OuterFlowObservation
}

func (o *traceObserver) ObserveOuterFlow(observation traffictrace.OuterFlowObservation) {
	o.observation = observation
}

func TestDialContextObservesPhysicalFlow(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()
	observer := &traceObserver{}
	ctx := traffictrace.WithObserver(context.Background(), observer)
	conn, err := DialContext(ctx, "tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	serverConn := <-accepted
	defer serverConn.Close()
	got := observer.observation
	if got.OuterConnID == "" || !got.Flow.Complete || got.Flow.Source != "dialer_socket" || got.Flow.Scope != "physical" {
		t.Fatalf("unexpected observation: %+v", got)
	}
}

func TestListenPacketObservesPhysicalFlow(t *testing.T) {
	observer := &traceObserver{}
	ctx := traffictrace.WithObserver(context.Background(), observer)
	remote := netip.MustParseAddrPort("127.0.0.1:53")
	packetConn, err := ListenPacket(ctx, "udp4", "127.0.0.1:0", remote)
	if err != nil {
		t.Fatal(err)
	}
	defer packetConn.Close()
	got := observer.observation
	if got.OuterConnID == "" || !got.Flow.Complete || got.Flow.DstIP != "127.0.0.1" || got.Flow.DstPort != 53 {
		t.Fatalf("unexpected observation: %+v", got)
	}
}
