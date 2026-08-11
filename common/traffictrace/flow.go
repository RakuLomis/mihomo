package traffictrace

import (
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/metacubex/mihomo/common/utils"
)

// FlowTuple is a normalized network five-tuple. Complete is true only when
// both endpoints are numeric, non-unspecified IP addresses with non-zero ports.
type FlowTuple struct {
	Network string `json:"network"`

	SrcIP   string `json:"src_ip,omitempty"`
	SrcPort uint16 `json:"src_port,omitempty"`
	DstIP   string `json:"dst_ip,omitempty"`
	DstPort uint16 `json:"dst_port,omitempty"`
	DstHost string `json:"dst_host,omitempty"`

	Key      string `json:"key,omitempty"`
	Complete bool   `json:"complete"`
	Source   string `json:"source,omitempty"`
	Scope    string `json:"scope,omitempty"`
	Shared   bool   `json:"shared,omitempty"`
}

type OuterFlowObservation struct {
	OuterConnID string
	Flow        FlowTuple
}

func NewOuterConnID() string {
	return utils.NewUUIDV4().String()
}

func NewFlowTuple(network string, src, dst netip.AddrPort, dstHost, source, scope string, shared bool) FlowTuple {
	flow := FlowTuple{
		Network: normalizeNetwork(network),
		DstHost: strings.TrimSuffix(dstHost, "."),
		Source:  source,
		Scope:   scope,
		Shared:  shared,
	}
	if src.IsValid() {
		src = normalizeAddrPort(src)
		flow.SrcPort = src.Port()
		if !src.Addr().IsUnspecified() {
			flow.SrcIP = src.Addr().String()
		}
	}
	if dst.IsValid() {
		dst = normalizeAddrPort(dst)
		flow.DstPort = dst.Port()
		if !dst.Addr().IsUnspecified() {
			flow.DstIP = dst.Addr().String()
		}
	}
	flow.Complete = isUsableEndpoint(src) && isUsableEndpoint(dst) && flow.Network != ""
	if flow.Complete {
		flow.Key = flow.Network + "|" + src.String() + "|" + dst.String()
	}
	return flow
}

func NewFlowTupleFromAddrs(network string, src, dst net.Addr, dstHost, source, scope string, shared bool) FlowTuple {
	srcAddrPort, _ := AddrPortFromNetAddr(src)
	dstAddrPort, _ := AddrPortFromNetAddr(dst)
	return NewFlowTuple(network, srcAddrPort, dstAddrPort, dstHost, source, scope, shared)
}

func NewFlowTupleFromStrings(network, src, dst, source, scope string, shared bool) FlowTuple {
	srcAddrPort, _ := ParseAddrPort(src)
	dstAddrPort, dstOK := ParseAddrPort(dst)
	flow := NewFlowTuple(network, srcAddrPort, dstAddrPort, "", source, scope, shared)
	if !dstOK {
		host, port, err := net.SplitHostPort(dst)
		if err == nil {
			flow.DstHost = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
			if portValue, parseErr := strconv.ParseUint(port, 10, 16); parseErr == nil {
				flow.DstPort = uint16(portValue)
			}
		}
	}
	return flow
}

func AddrPortFromNetAddr(addr net.Addr) (netip.AddrPort, bool) {
	if addr == nil {
		return netip.AddrPort{}, false
	}
	if addrPorter, ok := addr.(interface{ AddrPort() netip.AddrPort }); ok {
		addrPort := addrPorter.AddrPort()
		if addrPort.IsValid() {
			return normalizeAddrPort(addrPort), true
		}
	}
	return ParseAddrPort(addr.String())
}

func ParseAddrPort(raw string) (netip.AddrPort, bool) {
	addrPort, err := netip.ParseAddrPort(raw)
	if err == nil && addrPort.IsValid() {
		return normalizeAddrPort(addrPort), true
	}

	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return netip.AddrPort{}, false
	}
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return netip.AddrPort{}, false
	}
	portValue, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return netip.AddrPort{}, false
	}
	return normalizeAddrPort(netip.AddrPortFrom(addr, uint16(portValue))), true
}

func normalizeAddrPort(addrPort netip.AddrPort) netip.AddrPort {
	addr := addrPort.Addr().Unmap()
	if addr.Is6() {
		addr = addr.WithZone("")
	}
	return netip.AddrPortFrom(addr, addrPort.Port())
}

func isUsableEndpoint(addrPort netip.AddrPort) bool {
	return addrPort.IsValid() &&
		addrPort.Port() != 0 &&
		addrPort.Addr().IsValid() &&
		!addrPort.Addr().IsUnspecified()
}

func normalizeNetwork(network string) string {
	network = strings.ToLower(network)
	switch {
	case strings.HasPrefix(network, "tcp"):
		return "tcp"
	case strings.HasPrefix(network, "udp"):
		return "udp"
	default:
		return ""
	}
}
