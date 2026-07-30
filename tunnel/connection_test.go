package tunnel

import (
	"testing"

	"github.com/metacubex/mihomo/component/sniffer"
	"github.com/metacubex/mihomo/component/tracer"
	C "github.com/metacubex/mihomo/constant"
)

func TestTracedPacketSenderRemainsOutermostAfterQUICWrapping(t *testing.T) {
	tracer.SetEnabled(false)
	trace := tracer.BeginUDP("key", "src", "dst", "", "", "", "")
	base := newPacketSender(trace)
	defer base.Close()

	quicSniffer, err := sniffer.NewQuicSniffer(sniffer.SnifferConfig{})
	if err != nil {
		t.Fatal(err)
	}
	wrapped := quicSniffer.WrapperSender(base, false)
	var sender C.PacketSender = &tracedPacketSender{PacketSender: wrapped, trace: trace}

	outer, ok := sender.(*tracedPacketSender)
	if !ok {
		t.Fatalf("outer sender type = %T, want *tracedPacketSender", sender)
	}
	if outer.trace != trace {
		t.Fatal("outer sender lost the shared UDP trace session")
	}
	if _, unwrapped := outer.PacketSender.(*packetSender); unwrapped {
		t.Fatal("test did not exercise a wrapped QUIC sender")
	}
}
