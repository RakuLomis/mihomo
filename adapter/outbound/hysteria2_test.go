package outbound

import (
	"context"
	"net"
	"net/netip"
	"runtime"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/traffictrace"
)

func TestHysteria2GC(t *testing.T) {
	option := Hysteria2Option{}
	option.Server = "127.0.0.1"
	option.Ports = "200,204,401-429,501-503"
	option.HopInterval = 30
	option.Password = "password"
	option.Obfs = "salamander"
	option.ObfsPassword = "password"
	option.SNI = "example.com"
	option.ALPN = []string{"h3"}
	hy, err := NewHysteria2(option)
	if err != nil {
		t.Error(err)
		return
	}
	closeCh := make(chan struct{})
	hy.closeCh = closeCh
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	hy = nil
	runtime.GC()
	select {
	case <-closeCh:
		return
	case <-ctx.Done():
		t.Error("timeout not GC")
	}
}

func TestHysteria2CarrierRegistryPromotesAndReusesStableCarrier(t *testing.T) {
	registry := &hysteria2CarrierRegistry{}
	flow := traffictrace.NewFlowTuple(
		"udp",
		netip.MustParseAddrPort("192.0.2.1:40000"),
		netip.MustParseAddrPort("198.51.100.1:443"),
		"", "dialer_socket", "physical", false,
	)
	created := registry.promote(traffictrace.OuterFlowObservation{
		OuterConnID: "outer-1", Flow: flow,
	})
	if created.Relation != traffictrace.CarrierRelationCreated || created.Generation != 1 {
		t.Fatalf("unexpected created carrier: %+v", created)
	}
	if !created.Flow.Shared || created.Protocol != "hysteria2" || len(created.Paths) != 1 {
		t.Fatalf("missing shared Hysteria2 metadata: %+v", created)
	}
	reused, ok := registry.reuse()
	if !ok || reused.OuterConnID != "outer-1" || reused.Relation != traffictrace.CarrierRelationReused {
		t.Fatalf("unexpected reused carrier: %+v, ok=%v", reused, ok)
	}
}

func TestHysteria2CarrierRegistryTracksPortHopAndGeneration(t *testing.T) {
	registry := &hysteria2CarrierRegistry{}
	firstFlow := traffictrace.NewFlowTuple(
		"udp", netip.MustParseAddrPort("192.0.2.1:40000"),
		netip.MustParseAddrPort("198.51.100.1:443"),
		"", "dialer_socket", "physical", false,
	)
	registry.promote(traffictrace.OuterFlowObservation{OuterConnID: "outer-1", Flow: firstFlow})
	registry.updateRemote(&net.UDPAddr{IP: net.ParseIP("198.51.100.1"), Port: 8443})
	hopped, ok := registry.reuse()
	if !ok || hopped.Flow.DstPort != 8443 || len(hopped.Paths) != 2 {
		t.Fatalf("port hop not retained: %+v", hopped)
	}
	secondFlow := traffictrace.NewFlowTuple(
		"udp", netip.MustParseAddrPort("192.0.2.1:41000"),
		netip.MustParseAddrPort("203.0.113.1:443"),
		"", "dialer_socket", "physical", false,
	)
	second := registry.promote(traffictrace.OuterFlowObservation{OuterConnID: "outer-2", Flow: secondFlow})
	if second.Generation != 2 || len(second.Paths) != 1 {
		t.Fatalf("replacement carrier must start a new generation: %+v", second)
	}
	registry.clear()
	if _, ok := registry.reuse(); ok {
		t.Fatal("cleared registry returned an active carrier")
	}
}
