package tracer

import (
	"bytes"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/traffictrace"
)

func testFlow(network, src, dst string) traffictrace.FlowTuple {
	return traffictrace.NewFlowTuple(network, netip.MustParseAddrPort(src), netip.MustParseAddrPort(dst), "", "test", "logical", false)
}

func TestTCPEventsLinkNormalizedPreAndPostFlows(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	tr.enabled.Store(true)
	preFlow := testFlow("tcp", "192.0.2.10:1234", "198.51.100.20:443")
	postFlow := testFlow("tcp", "203.0.113.10:3000", "203.0.113.20:8443")
	postFlow.Source, postFlow.Scope = "dialer_socket", "physical"
	session := tr.beginTCPWithFlow("tcp-1", preFlow, "legacy-src", "legacy-dst", "", "", "", "")
	session.ObserveOuterFlow(traffictrace.OuterFlowObservation{OuterConnID: "outer-1", Flow: postFlow})
	session.ProxyDial("proxy", "ss", "proxy.example:8443", EndpointInfo{Remote: "logical.example:8443", Scope: "logical"})
	session.Close(0, 0, StatusClosed, "", nil)
	events := decodeEvents(t, output.Bytes())
	if len(events) != 3 || events[0].PreFlow == nil || events[0].PreFlow.Key != preFlow.Key {
		t.Fatalf("tcp_connect lost pre-flow: %+v", events)
	}
	if events[1].PostFlow == nil || events[1].PostFlow.Key != postFlow.Key || events[1].PostFlow.Scope != "physical" || events[1].OuterConnID != "outer-1" {
		t.Fatalf("tcp_proxy_dial lost post-flow: %+v", events[1])
	}
}

func TestUDPOutCarriesPerPacketPreFlow(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	tr.enabled.Store(true)
	first := testFlow("udp", "192.0.2.10:1234", "198.51.100.1:53")
	second := testFlow("udp", "192.0.2.10:1234", "198.51.100.2:53")
	post := testFlow("udp", "203.0.113.10:3000", "203.0.113.20:443")
	session := tr.beginUDPWithFlow("shared-source", first, "src", "dst", "", "", "", "")
	session.ObserveOuterFlow(traffictrace.OuterFlowObservation{OuterConnID: "outer-udp", Flow: post})
	session.ProxyDial("proxy", "tuic", "proxy.example:443", EndpointInfo{})
	session.PacketOutWithFlow(first, "src", "dst-1", 10)
	session.PacketOutWithFlow(second, "src", "dst-2", 20)
	session.Close(30, 0, StatusClosed, "", nil)
	events := decodeEvents(t, output.Bytes())
	if len(events) != 5 || events[1].PostFlow == nil || !events[1].PostFlow.Shared || events[1].OuterConnID != "outer-udp" {
		t.Fatalf("unexpected udp proxy event: %+v", events)
	}
	if events[2].PreFlow == nil || events[2].PreFlow.Key != first.Key || events[3].PreFlow == nil || events[3].PreFlow.Key != second.Key || events[2].ConnKey != events[3].ConnKey {
		t.Fatalf("udp packet flows were not preserved: %+v", events)
	}
}
