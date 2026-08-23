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
	session.ProxyDial("proxy", "ss", "proxy.example:8443", EndpointInfo{Local: "outer-1", Remote: "logical.example:8443", Scope: "logical"})
	session.Close(0, 0, StatusClosed, "", nil)
	events := decodeEvents(t, output.Bytes())
	if len(events) != 4 || events[0].PreFlow == nil || events[0].PreFlow.Key != preFlow.Key {
		t.Fatalf("tcp_connect lost pre-flow: %+v", events)
	}
	if events[1].PostFlow == nil || events[1].PostFlow.Key != postFlow.Key || events[1].PostFlow.Scope != "physical" || events[1].OuterConnID != "outer-1" {
		t.Fatalf("tcp_proxy_dial lost post-flow: %+v", events[1])
	}
	if events[1].OutSrc != "203.0.113.10:3000" || events[1].OutDst != "203.0.113.20:8443" {
		t.Fatalf("tcp legacy endpoints diverged from post-flow: %+v", events[1])
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
	session.ProxyDial("proxy", "tuic", "proxy.example:443", EndpointInfo{Local: "outer-udp", Remote: "logical.example:443"})
	session.PacketOutWithFlow(first, "src", "dst-1", 10)
	session.PacketOutWithFlow(second, "src", "dst-2", 20)
	session.Close(30, 0, StatusClosed, "", nil)
	events := decodeEvents(t, output.Bytes())
	if len(events) != 6 || events[1].PostFlow == nil || !events[1].PostFlow.Shared || events[1].OuterConnID != "outer-udp" {
		t.Fatalf("unexpected udp proxy event: %+v", events)
	}
	if events[1].OutSrc != "203.0.113.10:3000" || events[1].OutDst != "203.0.113.20:443" {
		t.Fatalf("udp legacy endpoints diverged from post-flow: %+v", events[1])
	}
	if events[3].PreFlow == nil || events[3].PreFlow.Key != first.Key || events[4].PreFlow == nil || events[4].PreFlow.Key != second.Key || events[3].ConnKey != events[4].ConnKey {
		t.Fatalf("udp packet flows were not preserved: %+v", events)
	}
}

func TestRejectLeafHasOutcomeWithoutInvalidPostFlow(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	tr.enabled.Store(true)
	session := tr.beginTCP("tcp-reject", "src", "dst", "blocked.example", "", "", "")
	session.ProxyDialWithLeaf(
		"policy-group", "Selector", "REJECT", "Reject", "",
		EndpointInfo{Scope: "unknown"},
	)
	session.Close(0, 0, StatusClosed, "", nil)

	events := decodeEvents(t, output.Bytes())
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	dial := events[1]
	if dial.PostFlow != nil || dial.EgressOutcome != EgressRejected ||
		dial.Proxy != "policy-group" || dial.ProxyType != "Selector" ||
		dial.LeafProxy != "REJECT" || dial.LeafProxyType != "Reject" {
		t.Fatalf("unexpected reject dial event: %+v", dial)
	}
	closeEvent := events[2]
	if closeEvent.Status != StatusRejected || closeEvent.Stage != "reject" {
		t.Fatalf("reject close was not normalized: %+v", closeEvent)
	}
}

func TestGroupLeafTypeControlsSharedPostFlowAndOutcome(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	tr.enabled.Store(true)
	session := tr.beginUDP("udp-shared-leaf", "src", "dst", "", "", "", "")
	session.ProxyDialWithLeaf(
		"automatic", "URLTest", "tuic-node", "Tuic", "proxy.example:443",
		EndpointInfo{
			Local: "192.0.2.10:3000", Remote: "198.51.100.20:443",
			Scope: "physical", Source: "socket",
		},
	)
	session.Close(0, 0, StatusClosed, "", nil)

	events := decodeEvents(t, output.Bytes())
	dial := events[1]
	if dial.PostFlow == nil || !dial.PostFlow.Complete || !dial.PostFlow.Shared {
		t.Fatalf("leaf shared semantics missing: %+v", dial)
	}
	if dial.EgressOutcome != EgressProxy || dial.LeafProxyType != "Tuic" {
		t.Fatalf("unexpected leaf outcome: %+v", dial)
	}
}
