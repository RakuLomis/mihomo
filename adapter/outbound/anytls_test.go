package outbound

import (
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/traffictrace"
)

func anyTLSTestFlow(src, dst string) traffictrace.FlowTuple {
	return traffictrace.NewFlowTuple(
		"tcp", netip.MustParseAddrPort(src), netip.MustParseAddrPort(dst),
		"", "dialer_socket", "physical", false,
	)
}

func TestAnyTLSCarrierRegistryTracksSessionsIndependently(t *testing.T) {
	registry := newAnyTLSCarrierRegistry()
	first := registry.promote(11, traffictrace.OuterFlowObservation{
		OuterConnID: "outer-1",
		Flow:        anyTLSTestFlow("192.0.2.1:40000", "198.51.100.1:443"),
	})
	second := registry.promote(22, traffictrace.OuterFlowObservation{
		OuterConnID: "outer-2",
		Flow:        anyTLSTestFlow("192.0.2.1:40001", "198.51.100.2:443"),
	})

	if first.Generation != 1 || second.Generation != 2 {
		t.Fatalf("carrier generations are not monotonic: first=%+v second=%+v", first, second)
	}
	if first.Protocol != "anytls" || !first.Flow.Shared || len(first.Paths) != 1 {
		t.Fatalf("first carrier is missing AnyTLS shared metadata: %+v", first)
	}
	if second.Protocol != "anytls" || !second.Flow.Shared || len(second.Paths) != 1 {
		t.Fatalf("second carrier is missing AnyTLS shared metadata: %+v", second)
	}

	reusedFirst, ok := registry.reuse(11)
	if !ok || reusedFirst.OuterConnID != "outer-1" ||
		reusedFirst.Relation != traffictrace.CarrierRelationReused {
		t.Fatalf("unexpected first reuse: %+v, ok=%v", reusedFirst, ok)
	}
	reusedSecond, ok := registry.reuse(22)
	if !ok || reusedSecond.OuterConnID != "outer-2" ||
		reusedSecond.Relation != traffictrace.CarrierRelationReused {
		t.Fatalf("unexpected second reuse: %+v, ok=%v", reusedSecond, ok)
	}
}

func TestAnyTLSCarrierRegistryCloseDoesNotAffectOtherSessions(t *testing.T) {
	registry := newAnyTLSCarrierRegistry()
	registry.promote(11, traffictrace.OuterFlowObservation{
		OuterConnID: "outer-1",
		Flow:        anyTLSTestFlow("192.0.2.1:40000", "198.51.100.1:443"),
	})
	registry.promote(22, traffictrace.OuterFlowObservation{
		OuterConnID: "outer-2",
		Flow:        anyTLSTestFlow("192.0.2.1:40001", "198.51.100.2:443"),
	})

	registry.close(11)
	if _, ok := registry.reuse(11); ok {
		t.Fatal("closed AnyTLS session remained reusable")
	}
	if carrier, ok := registry.reuse(22); !ok || carrier.OuterConnID != "outer-2" {
		t.Fatalf("closing one session removed another: %+v, ok=%v", carrier, ok)
	}
	registry.clear()
	if _, ok := registry.reuse(22); ok {
		t.Fatal("cleared AnyTLS registry returned an active carrier")
	}
}

func TestAnyTLSCarrierRegistryHandlesCloseBeforePromote(t *testing.T) {
	registry := newAnyTLSCarrierRegistry()
	registry.close(33)
	registry.promote(33, traffictrace.OuterFlowObservation{
		OuterConnID: "outer-closed",
		Flow:        anyTLSTestFlow("192.0.2.1:40000", "198.51.100.1:443"),
	})
	if _, ok := registry.reuse(33); ok {
		t.Fatal("a carrier promoted after session close must not become reusable")
	}
}

func TestAnyTLSCarrierRegistryBoundsUnmatchedCloseTombstones(t *testing.T) {
	registry := newAnyTLSCarrierRegistry()
	for sessionID := uint64(1); sessionID <= 100; sessionID++ {
		registry.close(sessionID)
	}
	if len(registry.closed) > maxAnyTLSClosedSessionTombstones {
		t.Fatalf("closed tombstones grew to %d", len(registry.closed))
	}
	if len(registry.closedIDs) > maxAnyTLSClosedSessionTombstones {
		t.Fatalf("closed tombstone order grew to %d", len(registry.closedIDs))
	}
}
