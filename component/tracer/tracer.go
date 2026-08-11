package tracer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/common/traffictrace"
)

type EventType string

const (
	TCPConnect   EventType = "tcp_connect"
	TCPProxyDial EventType = "tcp_proxy_dial"
	TCPClose     EventType = "tcp_close"
	UDPConnect   EventType = "udp_connect"
	UDPProxyDial EventType = "udp_proxy_dial"
	UDPOut       EventType = "udp_out"
	UDPIn        EventType = "udp_in"
	UDPClose     EventType = "udp_close"
)

const (
	StatusClosed       = "closed"
	StatusDialError    = "dial_error"
	StatusResolveError = "resolve_error"
	StatusRejected     = "rejected"
	StatusCanceled     = "canceled"
)

const (
	EgressDirect       = "direct"
	EgressProxy        = "proxy"
	EgressRejected     = "rejected"
	EgressRejectedDrop = "rejected_drop"
	EgressInternalDNS  = "internal_dns"
	EgressPass         = "pass"
	EgressCompatible   = "compatible"
	EgressUnknown      = "unknown"
)

type event struct {
	SchemaVersion  int                     `json:"schema_version"`
	SessionID      string                  `json:"session_id,omitempty"`
	Ts             string                  `json:"ts"`
	EventSeq       uint64                  `json:"event_seq"`
	Type           EventType               `json:"type"`
	Network        string                  `json:"network"`
	PreFlow        *traffictrace.FlowTuple `json:"pre_flow,omitempty"`
	PostFlow       *traffictrace.FlowTuple `json:"post_flow,omitempty"`
	OuterConnID    string                  `json:"outer_conn_id,omitempty"`
	ConnID         string                  `json:"conn_id,omitempty"`
	ConnKey        string                  `json:"conn_key,omitempty"`
	Seq            uint64                  `json:"seq,omitempty"`
	Src            string                  `json:"src,omitempty"`
	Dst            string                  `json:"dst,omitempty"`
	Host           string                  `json:"host,omitempty"`
	Process        string                  `json:"process,omitempty"`
	ProcessPath    string                  `json:"process_path,omitempty"`
	InName         string                  `json:"in_name,omitempty"`
	Proxy          string                  `json:"proxy,omitempty"`
	ProxyType      string                  `json:"proxy_type,omitempty"`
	LeafProxy      string                  `json:"leaf_proxy,omitempty"`
	LeafProxyType  string                  `json:"leaf_proxy_type,omitempty"`
	EgressOutcome  string                  `json:"egress_outcome,omitempty"`
	ProxyAddr      string                  `json:"proxy_addr,omitempty"`
	OutSrc         string                  `json:"out_src,omitempty"`
	OutDst         string                  `json:"out_dst,omitempty"`
	EndpointScope  string                  `json:"endpoint_scope,omitempty"`
	EndpointSource string                  `json:"endpoint_source,omitempty"`
	Len            int                     `json:"len,omitempty"`
	From           string                  `json:"from,omitempty"`
	BytesUp        int64                   `json:"bytes_up,omitempty"`
	BytesDown      int64                   `json:"bytes_down,omitempty"`
	DurationMs     int64                   `json:"duration_ms,omitempty"`
	Status         string                  `json:"status,omitempty"`
	Stage          string                  `json:"stage,omitempty"`
	Error          string                  `json:"error,omitempty"`
}

type ConfigPatch struct {
	Enabled   *bool
	Output    *string
	SessionID *string
}

type Status struct {
	Enabled        bool   `json:"enabled"`
	Output         string `json:"output"`
	SessionID      string `json:"session_id,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	WriteErrors    uint64 `json:"write_errors"`
	ActiveSessions int64  `json:"active_sessions"`
}

type Tracer struct {
	enabled        atomic.Bool
	eventSeq       atomic.Uint64
	writeErrors    atomic.Uint64
	activeSessions atomic.Int64

	mu        sync.Mutex
	stdout    io.Writer
	sink      *traceSink
	lastError string
}

type traceSink struct {
	writer    *bufio.Writer
	file      *os.File
	output    string
	sessionID string
	refs      int64
	retired   bool
	failed    bool
}

var globalTracer = newTracer(os.Stdout)

func newTracer(stdout io.Writer) *Tracer {
	return &Tracer{
		stdout: stdout,
		sink:   &traceSink{writer: bufio.NewWriter(stdout)},
	}
}

func Configure(patch ConfigPatch) error {
	return globalTracer.configure(patch)
}

func (t *Tracer) configure(patch ConfigPatch) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	nextOutput := t.sink.output
	nextSessionID := t.sink.sessionID
	if patch.Output != nil {
		nextOutput = *patch.Output
	}
	if patch.SessionID != nil {
		nextSessionID = *patch.SessionID
	}
	rotateSink := patch.Output != nil || nextSessionID != t.sink.sessionID
	if rotateSink {
		nextSink, err := t.openSink(nextOutput, nextSessionID)
		if err != nil {
			return err
		}
		oldSink := t.sink
		t.sink = nextSink
		oldSink.retired = true
		t.lastError = ""
		t.closeSinkIfUnused(oldSink)
	}
	if patch.Enabled != nil {
		if *patch.Enabled && t.sink.failed {
			err := fmt.Errorf("tracer: output unavailable: %s", t.lastError)
			return err
		}
		t.enabled.Store(*patch.Enabled)
	}
	return nil
}

func (t *Tracer) openSink(output, sessionID string) (*traceSink, error) {
	if output == "" {
		return &traceSink{writer: bufio.NewWriter(t.stdout), sessionID: sessionID}, nil
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("tracer: open output file: %w", err)
	}
	return &traceSink{writer: bufio.NewWriter(f), file: f, output: output, sessionID: sessionID}, nil
}

func SetEnabled(v bool) {
	_ = Configure(ConfigPatch{Enabled: &v})
}

func IsEnabled() bool {
	return globalTracer.enabled.Load()
}

func Output() string {
	return GetStatus().Output
}

func SetOutput(path string) error {
	return Configure(ConfigPatch{Output: &path})
}

func GetStatus() Status {
	return globalTracer.status()
}

func (t *Tracer) status() Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	return Status{
		Enabled:        t.enabled.Load(),
		Output:         t.sink.output,
		SessionID:      t.sink.sessionID,
		LastError:      t.lastError,
		WriteErrors:    t.writeErrors.Load(),
		ActiveSessions: t.activeSessions.Load(),
	}
}

func (t *Tracer) acquireSink() *traceSink {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled.Load() || t.sink == nil || t.sink.failed {
		return nil
	}
	t.sink.refs++
	return t.sink
}

func (t *Tracer) releaseSink(sink *traceSink) {
	if sink == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if sink.refs > 0 {
		sink.refs--
	}
	t.closeSinkIfUnused(sink)
}

func (t *Tracer) closeSinkIfUnused(sink *traceSink) {
	if sink == nil || !sink.retired || sink.refs != 0 {
		return
	}
	if sink.writer != nil {
		_ = sink.writer.Flush()
	}
	if sink.file != nil {
		_ = sink.file.Close()
	}
	sink.writer = nil
	sink.file = nil
}

func (t *Tracer) writeTo(sink *traceSink, e event) {
	if sink == nil {
		return
	}
	e.SchemaVersion = EventSchemaVersion
	e.EventSeq = t.eventSeq.Add(1)
	e.Ts = time.Now().UTC().Format(time.RFC3339Nano)

	t.mu.Lock()
	defer t.mu.Unlock()
	e.SessionID = sink.sessionID
	b, err := json.Marshal(e)
	if err != nil {
		sink.failed = true
		t.lastError = err.Error()
		t.writeErrors.Add(1)
		if sink == t.sink {
			t.enabled.Store(false)
		}
		return
	}
	if sink.writer == nil || sink.failed {
		return
	}
	if _, err = sink.writer.Write(b); err == nil {
		err = sink.writer.WriteByte('\n')
	}
	if err == nil {
		err = sink.writer.Flush()
	}
	if err != nil {
		sink.failed = true
		t.lastError = err.Error()
		t.writeErrors.Add(1)
		if sink == t.sink {
			t.enabled.Store(false)
		}
	}
}

type EndpointInfo struct {
	Local  string
	Remote string
	Scope  string
	Source string
}

type upstream interface {
	Upstream() any
}

func ExtractEndpoints(conn any, fallbackRemote string) EndpointInfo {
	const maxDepth = 32

	current := conn
	physical := false
	var result EndpointInfo
walk:
	for i := 0; i < maxDepth; i++ {
		if current == nil {
			break
		}
		switch c := current.(type) {
		case *net.TCPConn:
			result.Local = addrString(c.LocalAddr())
			result.Remote = addrString(c.RemoteAddr())
			physical = true
			break walk
		case *net.UDPConn:
			result.Local = addrString(c.LocalAddr())
			result.Remote = addrString(c.RemoteAddr())
			physical = true
			break walk
		}
		if c, ok := current.(interface{ LocalAddr() net.Addr }); ok && result.Local == "" {
			result.Local = addrString(c.LocalAddr())
		}
		if c, ok := current.(interface{ RemoteAddr() net.Addr }); ok && result.Remote == "" {
			result.Remote = addrString(c.RemoteAddr())
		}
		u, ok := current.(upstream)
		if !ok {
			break
		}
		next := u.Upstream()
		if next == current {
			break
		}
		current = next
	}
	if result.Remote == "" {
		result.Remote = fallbackRemote
		if result.Remote != "" && !physical {
			result.Source = "proxy_config"
		}
	}
	if physical {
		result.Scope = "physical"
		result.Source = "socket"
	} else if result.Local != "" || result.Remote != "" {
		result.Scope = "logical"
		if result.Source == "" {
			result.Source = "connection"
		}
	} else {
		result.Scope = "unknown"
	}
	return result
}

func addrString(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	return addr.String()
}

type TCPSession struct {
	tracer  *Tracer
	sink    *traceSink
	id      string
	start   time.Time
	active  bool
	once    sync.Once
	preFlow traffictrace.FlowTuple

	mu            sync.RWMutex
	outer         traffictrace.OuterFlowObservation
	egressOutcome string
}

var _ traffictrace.OuterFlowObserver = (*TCPSession)(nil)

func BeginTCP(id, src, dst, host, process, processPath, inName string) *TCPSession {
	return globalTracer.beginTCPWithFlow(id, traffictrace.FlowTuple{}, src, dst, host, process, processPath, inName)
}

func BeginTCPWithFlow(id string, preFlow traffictrace.FlowTuple, src, dst, host, process, processPath, inName string) *TCPSession {
	return globalTracer.beginTCPWithFlow(id, preFlow, src, dst, host, process, processPath, inName)
}

func (t *Tracer) beginTCP(id, src, dst, host, process, processPath, inName string) *TCPSession {
	return t.beginTCPWithFlow(id, traffictrace.FlowTuple{}, src, dst, host, process, processPath, inName)
}

func (t *Tracer) beginTCPWithFlow(id string, preFlow traffictrace.FlowTuple, src, dst, host, process, processPath, inName string) *TCPSession {
	sink := t.acquireSink()
	s := &TCPSession{tracer: t, sink: sink, id: id, start: time.Now(), active: sink != nil, preFlow: preFlow}
	if !s.active {
		return s
	}
	t.activeSessions.Add(1)
	t.writeTo(s.sink, event{
		Type: TCPConnect, Network: "tcp", ConnID: id, PreFlow: flowPointer(preFlow),
		Src: src, Dst: dst, Host: host,
		Process: process, ProcessPath: processPath, InName: inName,
	})
	return s
}

func (s *TCPSession) ObserveOuterFlow(observation traffictrace.OuterFlowObservation) {
	if !s.active {
		return
	}
	s.mu.Lock()
	s.outer = observation
	s.mu.Unlock()
}

func (s *TCPSession) ProxyDial(proxy, proxyType, proxyAddr string, endpoint EndpointInfo) {
	s.ProxyDialWithLeaf(proxy, proxyType, proxy, proxyType, proxyAddr, endpoint)
}

func (s *TCPSession) ProxyDialWithLeaf(proxy, proxyType, leafProxy, leafProxyType, proxyAddr string, endpoint EndpointInfo) {
	if !s.active {
		return
	}
	s.mu.Lock()
	outer := s.outer
	s.egressOutcome = classifyEgress(leafProxyType)
	egressOutcome := s.egressOutcome
	s.mu.Unlock()
	postFlow, outerConnID := normalizedPostFlow("tcp", leafProxyType, endpoint, outer)
	outSrc, outDst := legacyPostEndpoints(endpoint, postFlow)
	endpointScope, endpointSource := normalizedEndpointMetadata(endpoint, postFlow)
	s.tracer.writeTo(s.sink, event{
		Type: TCPProxyDial, Network: "tcp", ConnID: s.id,
		Proxy: proxy, ProxyType: proxyType, ProxyAddr: proxyAddr,
		LeafProxy: leafProxy, LeafProxyType: leafProxyType, EgressOutcome: egressOutcome,
		OutSrc: outSrc, OutDst: outDst,
		EndpointScope: endpointScope, EndpointSource: endpointSource,
		PostFlow: postFlow, OuterConnID: outerConnID,
	})
}

func (s *TCPSession) Close(bytesUp, bytesDown int64, status, stage string, cause error) {
	if !s.active {
		return
	}
	s.once.Do(func() {
		s.mu.RLock()
		egressOutcome := s.egressOutcome
		s.mu.RUnlock()
		status, stage = terminalForEgress(egressOutcome, status, stage)
		s.tracer.writeTo(s.sink, event{
			Type: TCPClose, Network: "tcp", ConnID: s.id,
			BytesUp: bytesUp, BytesDown: bytesDown,
			DurationMs: time.Since(s.start).Milliseconds(),
			Status:     status, Stage: stage, Error: errorString(cause),
		})
		s.tracer.activeSessions.Add(-1)
		s.tracer.releaseSink(s.sink)
	})
}

type UDPSession struct {
	tracer  *Tracer
	key     string
	sink    *traceSink
	start   time.Time
	active  bool
	once    sync.Once
	seqOut  atomic.Uint64
	seqIn   atomic.Uint64
	preFlow traffictrace.FlowTuple

	mu            sync.RWMutex
	proxyName     string
	outer         traffictrace.OuterFlowObservation
	egressOutcome string
}

var _ traffictrace.OuterFlowObserver = (*UDPSession)(nil)

func BeginUDP(key, src, dst, host, process, processPath, inName string) *UDPSession {
	return globalTracer.beginUDPWithFlow(key, traffictrace.FlowTuple{}, src, dst, host, process, processPath, inName)
}

func BeginUDPWithFlow(key string, preFlow traffictrace.FlowTuple, src, dst, host, process, processPath, inName string) *UDPSession {
	return globalTracer.beginUDPWithFlow(key, preFlow, src, dst, host, process, processPath, inName)
}

func (t *Tracer) beginUDP(key, src, dst, host, process, processPath, inName string) *UDPSession {
	return t.beginUDPWithFlow(key, traffictrace.FlowTuple{}, src, dst, host, process, processPath, inName)
}

func (t *Tracer) beginUDPWithFlow(key string, preFlow traffictrace.FlowTuple, src, dst, host, process, processPath, inName string) *UDPSession {
	sink := t.acquireSink()
	s := &UDPSession{tracer: t, sink: sink, key: key, start: time.Now(), active: sink != nil, preFlow: preFlow}
	if !s.active {
		return s
	}
	t.activeSessions.Add(1)
	t.writeTo(s.sink, event{
		Type: UDPConnect, Network: "udp", ConnKey: key, PreFlow: flowPointer(preFlow),
		Src: src, Dst: dst, Host: host,
		Process: process, ProcessPath: processPath, InName: inName,
	})
	return s
}

func (s *UDPSession) ObserveOuterFlow(observation traffictrace.OuterFlowObservation) {
	if !s.active {
		return
	}
	s.mu.Lock()
	s.outer = observation
	s.mu.Unlock()
}

func (s *UDPSession) ProxyDial(proxy, proxyType, proxyAddr string, endpoint EndpointInfo) {
	s.ProxyDialWithLeaf(proxy, proxyType, proxy, proxyType, proxyAddr, endpoint)
}

func (s *UDPSession) ProxyDialWithLeaf(proxy, proxyType, leafProxy, leafProxyType, proxyAddr string, endpoint EndpointInfo) {
	s.mu.Lock()
	s.proxyName = proxy
	outer := s.outer
	s.egressOutcome = classifyEgress(leafProxyType)
	egressOutcome := s.egressOutcome
	s.mu.Unlock()
	if !s.active {
		return
	}
	postFlow, outerConnID := normalizedPostFlow("udp", leafProxyType, endpoint, outer)
	outSrc, outDst := legacyPostEndpoints(endpoint, postFlow)
	endpointScope, endpointSource := normalizedEndpointMetadata(endpoint, postFlow)
	s.tracer.writeTo(s.sink, event{
		Type: UDPProxyDial, Network: "udp", ConnKey: s.key,
		Proxy: proxy, ProxyType: proxyType, ProxyAddr: proxyAddr,
		LeafProxy: leafProxy, LeafProxyType: leafProxyType, EgressOutcome: egressOutcome,
		OutSrc: outSrc, OutDst: outDst,
		EndpointScope: endpointScope, EndpointSource: endpointSource,
		PostFlow: postFlow, OuterConnID: outerConnID,
	})
}

func (s *UDPSession) PacketOut(src, dst string, length int) {
	s.PacketOutWithFlow(traffictrace.FlowTuple{}, src, dst, length)
}

func (s *UDPSession) PacketOutWithFlow(preFlow traffictrace.FlowTuple, src, dst string, length int) {
	if !s.active || !s.tracer.enabled.Load() {
		return
	}
	s.mu.RLock()
	proxy := s.proxyName
	s.mu.RUnlock()
	s.tracer.writeTo(s.sink, event{
		Type: UDPOut, Network: "udp", ConnKey: s.key, PreFlow: flowPointer(preFlow),
		Seq: s.seqOut.Add(1), Src: src, Dst: dst, Len: length, Proxy: proxy,
	})
}

func (s *UDPSession) PacketIn(from string, length int) {
	if !s.active || !s.tracer.enabled.Load() {
		return
	}
	s.tracer.writeTo(s.sink, event{
		Type: UDPIn, Network: "udp", ConnKey: s.key, PreFlow: flowPointer(s.preFlow),
		Seq: s.seqIn.Add(1), From: from, Len: length,
	})
}

func (s *UDPSession) Close(bytesUp, bytesDown int64, status, stage string, cause error) {
	if !s.active {
		return
	}
	s.once.Do(func() {
		s.mu.RLock()
		egressOutcome := s.egressOutcome
		s.mu.RUnlock()
		status, stage = terminalForEgress(egressOutcome, status, stage)
		s.tracer.writeTo(s.sink, event{
			Type: UDPClose, Network: "udp", ConnKey: s.key,
			BytesUp: bytesUp, BytesDown: bytesDown,
			DurationMs: time.Since(s.start).Milliseconds(),
			Status:     status, Stage: stage, Error: errorString(cause),
		})
		s.tracer.activeSessions.Add(-1)
		s.tracer.releaseSink(s.sink)
	})
}

func normalizedPostFlow(network, proxyType string, endpoint EndpointInfo, outer traffictrace.OuterFlowObservation) (*traffictrace.FlowTuple, string) {
	if outer.OuterConnID != "" {
		flow := outer.Flow
		flow.Shared = potentiallySharedProxy(proxyType)
		if flow.Complete {
			return &flow, outer.OuterConnID
		}
		return nil, outer.OuterConnID
	}
	flow := traffictrace.NewFlowTupleFromStrings(network, endpoint.Local, endpoint.Remote, endpoint.Source, endpoint.Scope, potentiallySharedProxy(proxyType))
	if !flow.Complete {
		return nil, ""
	}
	return &flow, ""
}

func classifyEgress(proxyType string) string {
	switch strings.ToLower(strings.ReplaceAll(proxyType, "-", "")) {
	case "direct":
		return EgressDirect
	case "reject":
		return EgressRejected
	case "rejectdrop":
		return EgressRejectedDrop
	case "dns":
		return EgressInternalDNS
	case "pass":
		return EgressPass
	case "compatible":
		return EgressCompatible
	case "":
		return EgressUnknown
	default:
		return EgressProxy
	}
}

func terminalForEgress(outcome, status, stage string) (string, string) {
	if (outcome == EgressRejected || outcome == EgressRejectedDrop) && status == StatusClosed {
		return StatusRejected, "reject"
	}
	return status, stage
}

func normalizedEndpointMetadata(endpoint EndpointInfo, flow *traffictrace.FlowTuple) (string, string) {
	if flow != nil {
		return flow.Scope, flow.Source
	}
	return endpoint.Scope, endpoint.Source
}

func legacyPostEndpoints(endpoint EndpointInfo, flow *traffictrace.FlowTuple) (string, string) {
	outSrc, outDst := endpoint.Local, endpoint.Remote
	if flow != nil && flow.Complete {
		return net.JoinHostPort(flow.SrcIP, fmt.Sprintf("%d", flow.SrcPort)),
			net.JoinHostPort(flow.DstIP, fmt.Sprintf("%d", flow.DstPort))
	}

	// Preserve best-effort legacy behavior for incomplete observations.
	if flow == nil {
		return outSrc, outDst
	}
	if outSrc == "" && flow.SrcIP != "" && flow.SrcPort != 0 {
		outSrc = net.JoinHostPort(flow.SrcIP, fmt.Sprintf("%d", flow.SrcPort))
	}
	if outDst == "" && flow.DstIP != "" && flow.DstPort != 0 {
		outDst = net.JoinHostPort(flow.DstIP, fmt.Sprintf("%d", flow.DstPort))
	}
	return outSrc, outDst
}

func flowPointer(flow traffictrace.FlowTuple) *traffictrace.FlowTuple {
	if flow.Network == "" {
		return nil
	}
	copyFlow := flow
	return &copyFlow
}

func potentiallySharedProxy(proxyType string) bool {
	switch strings.ToLower(proxyType) {
	case "tuic", "hysteria", "hysteria2", "anytls":
		return true
	default:
		return false
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
