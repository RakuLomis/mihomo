package constant

import (
	"net"
	"net/netip"
	"testing"
)

func TestCaptureOriginalFlowIsIdempotent(t *testing.T) {
	metadata := &Metadata{NetWork: TCP, SrcIP: netip.MustParseAddr("192.0.2.10"), SrcPort: 1234, DstIP: netip.MustParseAddr("198.51.100.20"), DstPort: 443}
	metadata.CaptureOriginalFlow()
	want := metadata.OriginalFlow
	metadata.DstIP = netip.MustParseAddr("203.0.113.30")
	metadata.DstPort = 8443
	metadata.CaptureOriginalFlow()
	if metadata.OriginalFlow != want || !want.Complete || want.Key != "tcp|192.0.2.10:1234|198.51.100.20:443" {
		t.Fatalf("unexpected original flow: got %+v want %+v", metadata.OriginalFlow, want)
	}
}

func TestCaptureOriginalFlowFallsBackToRawAddresses(t *testing.T) {
	metadata := &Metadata{NetWork: UDP, RawSrcAddr: &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 1234}, RawDstAddr: &net.UDPAddr{IP: net.ParseIP("198.51.100.20"), Port: 53}}
	metadata.CaptureOriginalFlow()
	flow := metadata.OriginalFlow
	if !flow.Complete || flow.Key != "udp|192.0.2.10:1234|198.51.100.20:53" || flow.Source != "metadata+raw_snapshot" {
		t.Fatalf("unexpected raw-address flow: %+v", flow)
	}
}

func TestCaptureOriginalFlowDoesNotUseListenerAsDomainDestination(t *testing.T) {
	metadata := &Metadata{NetWork: TCP, SrcIP: netip.MustParseAddr("192.0.2.10"), SrcPort: 1234, Host: "example.com", DstPort: 443, RawDstAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7890}}
	metadata.CaptureOriginalFlow()
	flow := metadata.OriginalFlow
	if flow.Complete || flow.Key != "" || flow.DstIP != "" || flow.DstHost != "example.com" || flow.DstPort != 443 || flow.Source != "proxy_request" {
		t.Fatalf("unexpected domain flow: %+v", flow)
	}
}
