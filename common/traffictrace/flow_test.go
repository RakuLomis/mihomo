package traffictrace

import (
	"context"
	"net"
	"net/netip"
	"testing"
)

func TestNewFlowTupleNormalizesAddresses(t *testing.T) {
	flow := NewFlowTuple("TCP4", netip.MustParseAddrPort("[::ffff:192.0.2.10]:1234"), netip.MustParseAddrPort("[fe80::1%eth0]:443"), "Example.COM.", "test", "logical", false)
	if !flow.Complete || flow.Network != "tcp" || flow.SrcIP != "192.0.2.10" || flow.DstIP != "fe80::1" {
		t.Fatalf("unexpected normalized flow: %+v", flow)
	}
	if flow.Key != "tcp|192.0.2.10:1234|[fe80::1]:443" || flow.DstHost != "Example.COM" {
		t.Fatalf("unexpected normalized identity: %+v", flow)
	}
}

func TestNewFlowTupleRejectsUnspecifiedEndpoint(t *testing.T) {
	flow := NewFlowTuple("udp", netip.MustParseAddrPort("0.0.0.0:1234"), netip.MustParseAddrPort("198.51.100.1:53"), "", "test", "physical", false)
	if flow.Complete || flow.Key != "" || flow.SrcIP != "" || flow.SrcPort != 1234 {
		t.Fatalf("unspecified endpoint must be incomplete: %+v", flow)
	}
}

func TestNewFlowTupleOmitsIPv6UnspecifiedAddressButKeepsPort(t *testing.T) {
	flow := NewFlowTuple("udp", netip.MustParseAddrPort("[::]:56571"), netip.MustParseAddrPort("101.227.12.8:443"), "", "dialer_socket", "physical", false)
	if flow.Complete || flow.Key != "" || flow.SrcIP != "" || flow.SrcPort != 56571 {
		t.Fatalf("IPv6 unspecified endpoint must retain only its port: %+v", flow)
	}
}

func TestNewFlowTupleFromStringsPreservesDomainAndPort(t *testing.T) {
	flow := NewFlowTupleFromStrings("tcp", "192.0.2.1:1000", "example.com:443", "proxy", "logical", false)
	if flow.Complete || flow.Key != "" || flow.DstHost != "example.com" || flow.DstPort != 443 {
		t.Fatalf("unexpected domain flow: %+v", flow)
	}
}

type recordingObserver struct{ observation OuterFlowObservation }

func (o *recordingObserver) ObserveOuterFlow(observation OuterFlowObservation) {
	o.observation = observation
}

func TestObserveOuterFlowFromContext(t *testing.T) {
	observer := &recordingObserver{}
	ctx := WithObserver(context.Background(), observer)
	ObserveOuterFlow(ctx, "tcp", &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 2000}, &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: 443}, "dialer_socket")
	got := observer.observation
	if got.OuterConnID == "" || !got.Flow.Complete || got.Flow.Scope != "physical" || got.Flow.Source != "dialer_socket" {
		t.Fatalf("unexpected observation: %+v", got)
	}
}
