package outbound

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"sync"
	"time"

	CN "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/traffictrace"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/component/ca"
	"github.com/metacubex/mihomo/component/dialer"
	"github.com/metacubex/mihomo/component/proxydialer"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
	tuicCommon "github.com/metacubex/mihomo/transport/tuic/common"

	"github.com/metacubex/sing-quic/hysteria2"

	"github.com/metacubex/quic-go"
	"github.com/metacubex/randv2"
	M "github.com/sagernet/sing/common/metadata"
)

func init() {
	hysteria2.SetCongestionController = tuicCommon.SetCongestionController
}

const minHopInterval = 5
const defaultHopInterval = 30

type Hysteria2 struct {
	*Base

	option *Hysteria2Option
	client *hysteria2.Client
	dialer proxydialer.SingDialer

	dialMu   sync.Mutex
	carriers *hysteria2CarrierRegistry

	closeCh chan struct{} // for test
}

type hysteria2CarrierRegistry struct {
	mu         sync.RWMutex
	active     traffictrace.OuterFlowObservation
	generation uint64
}

func (r *hysteria2CarrierRegistry) promote(observation traffictrace.OuterFlowObservation) traffictrace.OuterFlowObservation {
	r.mu.Lock()
	r.generation++
	observation.Flow.Shared = true
	observation.Relation = traffictrace.CarrierRelationCreated
	observation.Generation = r.generation
	observation.Protocol = "hysteria2"
	observation.Paths = []traffictrace.FlowTuple{observation.Flow}
	r.active = observation.Clone()
	promoted := observation.Clone()
	r.mu.Unlock()
	traffictrace.NotifyCarrierLifecycle(traffictrace.CarrierLifecycleObservation{
		Type:        traffictrace.CarrierLifecycleOpen,
		Observation: promoted,
	})
	return promoted
}

func (r *hysteria2CarrierRegistry) reuse() (traffictrace.OuterFlowObservation, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.active.OuterConnID == "" || !r.active.Flow.Complete {
		return traffictrace.OuterFlowObservation{}, false
	}
	observation := r.active.Clone()
	observation.Relation = traffictrace.CarrierRelationReused
	return observation, true
}

func (r *hysteria2CarrierRegistry) updateRemote(remote net.Addr) {
	r.mu.Lock()
	if r.active.OuterConnID == "" || !r.active.Flow.Complete {
		r.mu.Unlock()
		return
	}
	remoteAddr, ok := traffictrace.AddrPortFromNetAddr(remote)
	if !ok {
		r.mu.Unlock()
		return
	}
	flow := r.active.Flow
	flow.DstIP = remoteAddr.Addr().String()
	flow.DstPort = remoteAddr.Port()
	flow.Key = flow.Network + "|" + net.JoinHostPort(flow.SrcIP, strconv.Itoa(int(flow.SrcPort))) + "|" + remoteAddr.String()
	for _, path := range r.active.Paths {
		if path.Key == flow.Key {
			r.active.Flow = flow
			r.mu.Unlock()
			return
		}
	}
	r.active.Flow = flow
	r.active.Paths = append(r.active.Paths, flow)
	observation := r.active.Clone()
	r.mu.Unlock()
	traffictrace.NotifyCarrierLifecycle(traffictrace.CarrierLifecycleObservation{
		Type:        traffictrace.CarrierLifecyclePathUpdate,
		Observation: observation,
	})
}

func (r *hysteria2CarrierRegistry) clear() {
	r.mu.Lock()
	observation := r.active.Clone()
	r.active = traffictrace.OuterFlowObservation{}
	r.mu.Unlock()
	if observation.OuterConnID == "" {
		return
	}
	traffictrace.NotifyCarrierLifecycle(traffictrace.CarrierLifecycleObservation{
		Type:        traffictrace.CarrierLifecycleClose,
		Observation: observation,
	})
}

type hysteria2OuterFlowCapture struct {
	mu          sync.Mutex
	observation traffictrace.OuterFlowObservation
}

func (c *hysteria2OuterFlowCapture) ObserveOuterFlow(observation traffictrace.OuterFlowObservation) {
	c.mu.Lock()
	c.observation = observation.Clone()
	c.mu.Unlock()
}

func (c *hysteria2OuterFlowCapture) result() (traffictrace.OuterFlowObservation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.observation.Clone(), c.observation.OuterConnID != ""
}

type Hysteria2Option struct {
	BasicOption
	Name           string   `proxy:"name"`
	Server         string   `proxy:"server"`
	Port           int      `proxy:"port,omitempty"`
	Ports          string   `proxy:"ports,omitempty"`
	HopInterval    int      `proxy:"hop-interval,omitempty"`
	Up             string   `proxy:"up,omitempty"`
	Down           string   `proxy:"down,omitempty"`
	Password       string   `proxy:"password,omitempty"`
	Obfs           string   `proxy:"obfs,omitempty"`
	ObfsPassword   string   `proxy:"obfs-password,omitempty"`
	SNI            string   `proxy:"sni,omitempty"`
	SkipCertVerify bool     `proxy:"skip-cert-verify,omitempty"`
	Fingerprint    string   `proxy:"fingerprint,omitempty"`
	ALPN           []string `proxy:"alpn,omitempty"`
	CustomCA       string   `proxy:"ca,omitempty"`
	CustomCAString string   `proxy:"ca-str,omitempty"`
	CWND           int      `proxy:"cwnd,omitempty"`
	UdpMTU         int      `proxy:"udp-mtu,omitempty"`

	// quic-go special config
	InitialStreamReceiveWindow     uint64 `proxy:"initial-stream-receive-window,omitempty"`
	MaxStreamReceiveWindow         uint64 `proxy:"max-stream-receive-window,omitempty"`
	InitialConnectionReceiveWindow uint64 `proxy:"initial-connection-receive-window,omitempty"`
	MaxConnectionReceiveWindow     uint64 `proxy:"max-connection-receive-window,omitempty"`
}

func (h *Hysteria2) DialContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (_ C.Conn, err error) {
	h.dialMu.Lock()
	defer h.dialMu.Unlock()
	options := h.Base.DialOptions(opts...)
	h.dialer.SetDialer(dialer.NewDialer(options...))
	capture := &hysteria2OuterFlowCapture{}
	traceCtx := traffictrace.WithObserver(ctx, capture)
	c, err := h.client.DialConn(traceCtx, M.ParseSocksaddrHostPort(metadata.String(), metadata.DstPort))
	if err != nil {
		return nil, err
	}
	h.publishCarrierBinding(ctx, capture)
	return NewConn(CN.NewRefConn(c, h), h), nil
}

func (h *Hysteria2) ListenPacketContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (_ C.PacketConn, err error) {
	h.dialMu.Lock()
	defer h.dialMu.Unlock()
	options := h.Base.DialOptions(opts...)
	h.dialer.SetDialer(dialer.NewDialer(options...))
	capture := &hysteria2OuterFlowCapture{}
	traceCtx := traffictrace.WithObserver(ctx, capture)
	pc, err := h.client.ListenPacket(traceCtx)
	if err != nil {
		return nil, err
	}
	if pc == nil {
		return nil, errors.New("packetConn is nil")
	}
	h.publishCarrierBinding(ctx, capture)
	return newPacketConn(CN.NewRefPacketConn(CN.NewThreadSafePacketConn(pc), h), h), nil
}

func (h *Hysteria2) publishCarrierBinding(ctx context.Context, capture *hysteria2OuterFlowCapture) {
	if observation, ok := capture.result(); ok {
		traffictrace.NotifyOuterFlow(ctx, h.carriers.promote(observation))
		return
	}
	if observation, ok := h.carriers.reuse(); ok {
		traffictrace.NotifyOuterFlow(ctx, observation)
	}
}

func closeHysteria2(h *Hysteria2) {
	if h.carriers != nil {
		h.carriers.clear()
	}
	if h.client != nil {
		_ = h.client.CloseWithError(errors.New("proxy removed"))
	}
	if h.closeCh != nil {
		close(h.closeCh)
	}
}

// ProxyInfo implements C.ProxyAdapter
func (h *Hysteria2) ProxyInfo() C.ProxyInfo {
	info := h.Base.ProxyInfo()
	info.DialerProxy = h.option.DialerProxy
	return info
}

func NewHysteria2(option Hysteria2Option) (*Hysteria2, error) {
	addr := net.JoinHostPort(option.Server, strconv.Itoa(option.Port))
	var salamanderPassword string
	if len(option.Obfs) > 0 {
		if option.ObfsPassword == "" {
			return nil, errors.New("missing obfs password")
		}
		switch option.Obfs {
		case hysteria2.ObfsTypeSalamander:
			salamanderPassword = option.ObfsPassword
		default:
			return nil, fmt.Errorf("unknown obfs type: %s", option.Obfs)
		}
	}

	serverName := option.Server
	if option.SNI != "" {
		serverName = option.SNI
	}

	tlsConfig := &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: option.SkipCertVerify,
		MinVersion:         tls.VersionTLS13,
	}

	var err error
	tlsConfig, err = ca.GetTLSConfig(tlsConfig, option.Fingerprint, option.CustomCA, option.CustomCAString)
	if err != nil {
		return nil, err
	}

	if len(option.ALPN) > 0 {
		tlsConfig.NextProtos = option.ALPN
	}

	if option.UdpMTU == 0 {
		// "1200" from quic-go's MaxDatagramSize
		// "-3" from quic-go's DatagramFrame.MaxDataLen
		option.UdpMTU = 1200 - 3
	}

	quicConfig := &quic.Config{
		InitialStreamReceiveWindow:     option.InitialStreamReceiveWindow,
		MaxStreamReceiveWindow:         option.MaxStreamReceiveWindow,
		InitialConnectionReceiveWindow: option.InitialConnectionReceiveWindow,
		MaxConnectionReceiveWindow:     option.MaxConnectionReceiveWindow,
	}

	singDialer := proxydialer.NewByNameSingDialer(option.DialerProxy, dialer.NewDialer())
	carrierRegistry := &hysteria2CarrierRegistry{}

	clientOptions := hysteria2.ClientOptions{
		Context:            context.TODO(),
		Dialer:             singDialer,
		Logger:             log.SingLogger,
		SendBPS:            StringToBps(option.Up),
		ReceiveBPS:         StringToBps(option.Down),
		SalamanderPassword: salamanderPassword,
		Password:           option.Password,
		TLSConfig:          tlsConfig,
		QUICConfig:         quicConfig,
		UDPDisabled:        false,
		CWND:               option.CWND,
		UdpMTU:             option.UdpMTU,
		ServerAddress: func(ctx context.Context) (*net.UDPAddr, error) {
			remote, resolveErr := resolveUDPAddrWithPrefer(ctx, "udp", addr, C.NewDNSPrefer(option.IPVersion))
			if resolveErr == nil && traffictrace.ObserverFromContext(ctx) == nil {
				carrierRegistry.updateRemote(remote)
			}
			return remote, resolveErr
		},
	}

	var ranges utils.IntRanges[uint16]
	var serverAddress []string
	if option.Ports != "" {
		ranges, err = utils.NewUnsignedRanges[uint16](option.Ports)
		if err != nil {
			return nil, err
		}
		ranges.Range(func(port uint16) bool {
			serverAddress = append(serverAddress, net.JoinHostPort(option.Server, strconv.Itoa(int(port))))
			return true
		})
		if len(serverAddress) > 0 {
			clientOptions.ServerAddress = func(ctx context.Context) (*net.UDPAddr, error) {
				remote, resolveErr := resolveUDPAddrWithPrefer(ctx, "udp", serverAddress[randv2.IntN(len(serverAddress))], C.NewDNSPrefer(option.IPVersion))
				if resolveErr == nil && traffictrace.ObserverFromContext(ctx) == nil {
					carrierRegistry.updateRemote(remote)
				}
				return remote, resolveErr
			}

			if option.HopInterval == 0 {
				option.HopInterval = defaultHopInterval
			} else if option.HopInterval < minHopInterval {
				option.HopInterval = minHopInterval
			}
			clientOptions.HopInterval = time.Duration(option.HopInterval) * time.Second
		}
	}
	if option.Port == 0 && len(serverAddress) == 0 {
		return nil, errors.New("invalid port")
	}

	client, err := hysteria2.NewClient(clientOptions)
	if err != nil {
		return nil, err
	}

	outbound := &Hysteria2{
		Base: &Base{
			name:   option.Name,
			addr:   addr,
			tp:     C.Hysteria2,
			udp:    true,
			iface:  option.Interface,
			rmark:  option.RoutingMark,
			prefer: C.NewDNSPrefer(option.IPVersion),
		},
		option:   &option,
		client:   client,
		dialer:   singDialer,
		carriers: carrierRegistry,
	}
	runtime.SetFinalizer(outbound, closeHysteria2)

	return outbound, nil
}
