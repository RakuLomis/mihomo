package outbound

import (
	"context"
	"errors"
	"net"
	"runtime"
	"strconv"
	"sync"
	"time"

	CN "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/traffictrace"
	"github.com/metacubex/mihomo/component/dialer"
	"github.com/metacubex/mihomo/component/proxydialer"
	"github.com/metacubex/mihomo/component/resolver"
	tlsC "github.com/metacubex/mihomo/component/tls"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/anytls"
	"github.com/metacubex/mihomo/transport/vmess"

	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/uot"
)

type AnyTLS struct {
	*Base
	client *anytls.Client
	dialer proxydialer.SingDialer
	option *AnyTLSOption

	dialMu   sync.Mutex
	carriers *anyTLSCarrierRegistry
}

const maxAnyTLSClosedSessionTombstones = 64

type anyTLSCarrierRegistry struct {
	mu         sync.RWMutex
	active     map[uint64]traffictrace.OuterFlowObservation
	closed     map[uint64]struct{}
	closedIDs  []uint64
	generation uint64
}

func newAnyTLSCarrierRegistry() *anyTLSCarrierRegistry {
	return &anyTLSCarrierRegistry{
		active: make(map[uint64]traffictrace.OuterFlowObservation),
		closed: make(map[uint64]struct{}),
	}
}

func (r *anyTLSCarrierRegistry) promote(sessionID uint64, observation traffictrace.OuterFlowObservation) traffictrace.OuterFlowObservation {
	r.mu.Lock()
	_, alreadyClosed := r.closed[sessionID]
	delete(r.closed, sessionID)
	r.generation++
	observation.Flow.Shared = true
	observation.Relation = traffictrace.CarrierRelationCreated
	observation.Generation = r.generation
	observation.Protocol = "anytls"
	observation.Paths = []traffictrace.FlowTuple{observation.Flow}
	if !alreadyClosed {
		r.active[sessionID] = observation.Clone()
	}
	promoted := observation.Clone()
	r.mu.Unlock()
	traffictrace.NotifyCarrierLifecycle(traffictrace.CarrierLifecycleObservation{
		Type:        traffictrace.CarrierLifecycleOpen,
		Observation: promoted,
	})
	if alreadyClosed {
		traffictrace.NotifyCarrierLifecycle(traffictrace.CarrierLifecycleObservation{
			Type:        traffictrace.CarrierLifecycleClose,
			Observation: promoted,
		})
	}
	return promoted
}

func (r *anyTLSCarrierRegistry) reuse(sessionID uint64) (traffictrace.OuterFlowObservation, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	observation, ok := r.active[sessionID]
	if !ok || observation.OuterConnID == "" || !observation.Flow.Complete {
		return traffictrace.OuterFlowObservation{}, false
	}
	observation = observation.Clone()
	observation.Relation = traffictrace.CarrierRelationReused
	return observation, true
}

func (r *anyTLSCarrierRegistry) close(sessionID uint64) {
	r.mu.Lock()
	observation, ok := r.active[sessionID]
	delete(r.active, sessionID)
	if !ok {
		if _, exists := r.closed[sessionID]; !exists {
			r.closed[sessionID] = struct{}{}
			r.closedIDs = append(r.closedIDs, sessionID)
			if len(r.closedIDs) > maxAnyTLSClosedSessionTombstones {
				delete(r.closed, r.closedIDs[0])
				r.closedIDs = r.closedIDs[1:]
			}
		}
	}
	r.mu.Unlock()
	if !ok || observation.OuterConnID == "" {
		return
	}
	traffictrace.NotifyCarrierLifecycle(traffictrace.CarrierLifecycleObservation{
		Type:        traffictrace.CarrierLifecycleClose,
		Observation: observation,
	})
}

func (r *anyTLSCarrierRegistry) clear() {
	r.mu.Lock()
	observations := make([]traffictrace.OuterFlowObservation, 0, len(r.active))
	for _, observation := range r.active {
		observations = append(observations, observation.Clone())
	}
	r.active = make(map[uint64]traffictrace.OuterFlowObservation)
	r.closed = make(map[uint64]struct{})
	r.closedIDs = nil
	r.mu.Unlock()
	for _, observation := range observations {
		traffictrace.NotifyCarrierLifecycle(traffictrace.CarrierLifecycleObservation{
			Type:        traffictrace.CarrierLifecycleClose,
			Observation: observation,
		})
	}
}

type anyTLSOuterFlowCapture struct {
	mu          sync.Mutex
	observation traffictrace.OuterFlowObservation
}

func (c *anyTLSOuterFlowCapture) ObserveOuterFlow(observation traffictrace.OuterFlowObservation) {
	c.mu.Lock()
	c.observation = observation.Clone()
	c.mu.Unlock()
}

func (c *anyTLSOuterFlowCapture) result() (traffictrace.OuterFlowObservation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.observation.Clone(), c.observation.OuterConnID != ""
}

type anyTLSCarrierConn interface {
	CarrierSessionID() uint64
}

type AnyTLSOption struct {
	BasicOption
	Name                     string   `proxy:"name"`
	Server                   string   `proxy:"server"`
	Port                     int      `proxy:"port"`
	Password                 string   `proxy:"password"`
	ALPN                     []string `proxy:"alpn,omitempty"`
	SNI                      string   `proxy:"sni,omitempty"`
	ClientFingerprint        string   `proxy:"client-fingerprint,omitempty"`
	SkipCertVerify           bool     `proxy:"skip-cert-verify,omitempty"`
	Fingerprint              string   `proxy:"fingerprint,omitempty"`
	UDP                      bool     `proxy:"udp,omitempty"`
	IdleSessionCheckInterval int      `proxy:"idle-session-check-interval,omitempty"`
	IdleSessionTimeout       int      `proxy:"idle-session-timeout,omitempty"`
	MinIdleSession           int      `proxy:"min-idle-session,omitempty"`
}

func (t *AnyTLS) DialContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (_ C.Conn, err error) {
	t.dialMu.Lock()
	defer t.dialMu.Unlock()
	options := t.Base.DialOptions(opts...)
	t.dialer.SetDialer(dialer.NewDialer(options...))
	capture := &anyTLSOuterFlowCapture{}
	traceCtx := traffictrace.WithObserver(ctx, capture)
	c, err := t.client.CreateProxy(traceCtx, M.ParseSocksaddrHostPort(metadata.String(), metadata.DstPort))
	if err != nil {
		return nil, err
	}
	t.publishCarrierBinding(ctx, c, capture)
	return NewConn(CN.NewRefConn(c, t), t), nil
}

func (t *AnyTLS) ListenPacketContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (_ C.PacketConn, err error) {
	t.dialMu.Lock()
	defer t.dialMu.Unlock()
	// create tcp
	options := t.Base.DialOptions(opts...)
	t.dialer.SetDialer(dialer.NewDialer(options...))
	capture := &anyTLSOuterFlowCapture{}
	traceCtx := traffictrace.WithObserver(ctx, capture)
	c, err := t.client.CreateProxy(traceCtx, uot.RequestDestination(2))
	if err != nil {
		return nil, err
	}
	t.publishCarrierBinding(ctx, c, capture)

	// create uot on tcp
	if !metadata.Resolved() {
		ip, err := resolver.ResolveIP(ctx, metadata.Host)
		if err != nil {
			_ = c.Close()
			return nil, errors.New("can't resolve ip")
		}
		metadata.DstIP = ip
	}
	destination := M.SocksaddrFromNet(metadata.UDPAddr())
	return newPacketConn(CN.NewRefPacketConn(CN.NewThreadSafePacketConn(uot.NewLazyConn(c, uot.Request{Destination: destination})), t), t), nil
}

func (t *AnyTLS) publishCarrierBinding(ctx context.Context, conn net.Conn, capture *anyTLSOuterFlowCapture) {
	carrierConn, ok := conn.(anyTLSCarrierConn)
	if !ok || carrierConn.CarrierSessionID() == 0 {
		return
	}
	sessionID := carrierConn.CarrierSessionID()
	if observation, observed := capture.result(); observed {
		traffictrace.NotifyOuterFlow(ctx, t.carriers.promote(sessionID, observation))
		return
	}
	if observation, reused := t.carriers.reuse(sessionID); reused {
		traffictrace.NotifyOuterFlow(ctx, observation)
	}
}

func closeAnyTLS(t *AnyTLS) {
	if t.client != nil {
		_ = t.client.Close()
	}
	if t.carriers != nil {
		t.carriers.clear()
	}
}

// SupportUOT implements C.ProxyAdapter
func (t *AnyTLS) SupportUOT() bool {
	return true
}

// ProxyInfo implements C.ProxyAdapter
func (t *AnyTLS) ProxyInfo() C.ProxyInfo {
	info := t.Base.ProxyInfo()
	info.DialerProxy = t.option.DialerProxy
	return info
}

func NewAnyTLS(option AnyTLSOption) (*AnyTLS, error) {
	addr := net.JoinHostPort(option.Server, strconv.Itoa(option.Port))
	carrierRegistry := newAnyTLSCarrierRegistry()

	singDialer := proxydialer.NewByNameSingDialer(option.DialerProxy, dialer.NewDialer())

	tOption := anytls.ClientConfig{
		Password:                 option.Password,
		Server:                   M.ParseSocksaddrHostPort(option.Server, uint16(option.Port)),
		Dialer:                   singDialer,
		IdleSessionCheckInterval: time.Duration(option.IdleSessionCheckInterval) * time.Second,
		IdleSessionTimeout:       time.Duration(option.IdleSessionTimeout) * time.Second,
		MinIdleSession:           option.MinIdleSession,
		SessionClosed:            carrierRegistry.close,
	}
	tlsConfig := &vmess.TLSConfig{
		Host:              option.SNI,
		SkipCertVerify:    option.SkipCertVerify,
		NextProtos:        option.ALPN,
		FingerPrint:       option.Fingerprint,
		ClientFingerprint: option.ClientFingerprint,
	}
	if tlsConfig.Host == "" {
		tlsConfig.Host = option.Server
	}
	if tlsC.HaveGlobalFingerprint() && len(option.ClientFingerprint) == 0 {
		tlsConfig.ClientFingerprint = tlsC.GetGlobalFingerprint()
	}
	tOption.TLSConfig = tlsConfig

	outbound := &AnyTLS{
		Base: &Base{
			name:   option.Name,
			addr:   addr,
			tp:     C.AnyTLS,
			udp:    option.UDP,
			tfo:    option.TFO,
			mpTcp:  option.MPTCP,
			iface:  option.Interface,
			rmark:  option.RoutingMark,
			prefer: C.NewDNSPrefer(option.IPVersion),
		},
		client:   anytls.NewClient(context.TODO(), tOption),
		option:   &option,
		dialer:   singDialer,
		carriers: carrierRegistry,
	}
	runtime.SetFinalizer(outbound, closeAnyTLS)

	return outbound, nil
}
