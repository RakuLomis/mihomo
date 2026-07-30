package tracer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
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

type event struct {
	Ts             string    `json:"ts"`
	EventSeq       uint64    `json:"event_seq"`
	Type           EventType `json:"type"`
	Network        string    `json:"network"`
	ConnID         string    `json:"conn_id,omitempty"`
	ConnKey        string    `json:"conn_key,omitempty"`
	Seq            uint64    `json:"seq,omitempty"`
	Src            string    `json:"src,omitempty"`
	Dst            string    `json:"dst,omitempty"`
	Host           string    `json:"host,omitempty"`
	Process        string    `json:"process,omitempty"`
	ProcessPath    string    `json:"process_path,omitempty"`
	InName         string    `json:"in_name,omitempty"`
	Proxy          string    `json:"proxy,omitempty"`
	ProxyType      string    `json:"proxy_type,omitempty"`
	ProxyAddr      string    `json:"proxy_addr,omitempty"`
	OutSrc         string    `json:"out_src,omitempty"`
	OutDst         string    `json:"out_dst,omitempty"`
	EndpointScope  string    `json:"endpoint_scope,omitempty"`
	EndpointSource string    `json:"endpoint_source,omitempty"`
	Len            int       `json:"len,omitempty"`
	From           string    `json:"from,omitempty"`
	BytesUp        int64     `json:"bytes_up,omitempty"`
	BytesDown      int64     `json:"bytes_down,omitempty"`
	DurationMs     int64     `json:"duration_ms,omitempty"`
	Status         string    `json:"status,omitempty"`
	Stage          string    `json:"stage,omitempty"`
	Error          string    `json:"error,omitempty"`
}

type ConfigPatch struct {
	Enabled *bool
	Output  *string
}

type Status struct {
	Enabled        bool   `json:"enabled"`
	Output         string `json:"output"`
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
	writer    *bufio.Writer
	file      *os.File
	output    string
	lastError string
	failed    bool
}

var globalTracer = newTracer(os.Stdout)

func newTracer(stdout io.Writer) *Tracer {
	return &Tracer{stdout: stdout, writer: bufio.NewWriter(stdout)}
}

func Configure(patch ConfigPatch) error {
	return globalTracer.configure(patch)
}

func (t *Tracer) configure(patch ConfigPatch) error {
	var (
		nextWriter *bufio.Writer
		nextFile   *os.File
	)
	if patch.Output != nil {
		if *patch.Output == "" {
			nextWriter = bufio.NewWriter(t.stdout)
		} else {
			f, err := os.OpenFile(*patch.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return fmt.Errorf("tracer: open output file: %w", err)
			}
			nextFile = f
			nextWriter = bufio.NewWriter(f)
		}
	}

	t.mu.Lock()
	if patch.Output != nil {
		oldWriter, oldFile := t.writer, t.file
		t.writer, t.file, t.output = nextWriter, nextFile, *patch.Output
		t.lastError = ""
		t.failed = false
		if oldWriter != nil {
			_ = oldWriter.Flush()
		}
		if oldFile != nil {
			_ = oldFile.Close()
		}
	}
	if patch.Enabled != nil {
		if *patch.Enabled && t.failed {
			err := fmt.Errorf("tracer: output unavailable: %s", t.lastError)
			t.mu.Unlock()
			return err
		}
		t.enabled.Store(*patch.Enabled)
	}
	t.mu.Unlock()
	return nil
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
		Output:         t.output,
		LastError:      t.lastError,
		WriteErrors:    t.writeErrors.Load(),
		ActiveSessions: t.activeSessions.Load(),
	}
}

func (t *Tracer) write(e event) {
	e.EventSeq = t.eventSeq.Add(1)
	e.Ts = time.Now().UTC().Format(time.RFC3339Nano)

	b, err := json.Marshal(e)
	if err != nil {
		t.recordWriteError(err)
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.writer == nil || t.failed {
		return
	}
	if _, err = t.writer.Write(b); err == nil {
		err = t.writer.WriteByte('\n')
	}
	if err == nil {
		err = t.writer.Flush()
	}
	if err != nil {
		t.failed = true
		t.lastError = err.Error()
		t.writeErrors.Add(1)
		t.enabled.Store(false)
	}
}

func (t *Tracer) recordWriteError(err error) {
	t.mu.Lock()
	t.failed = true
	t.lastError = err.Error()
	t.writeErrors.Add(1)
	t.enabled.Store(false)
	t.mu.Unlock()
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
	tracer *Tracer
	id     string
	start  time.Time
	active bool
	once   sync.Once
}

func BeginTCP(id, src, dst, host, process, processPath, inName string) *TCPSession {
	return globalTracer.beginTCP(id, src, dst, host, process, processPath, inName)
}

func (t *Tracer) beginTCP(id, src, dst, host, process, processPath, inName string) *TCPSession {
	s := &TCPSession{tracer: t, id: id, start: time.Now(), active: t.enabled.Load()}
	if !s.active {
		return s
	}
	t.activeSessions.Add(1)
	t.write(event{
		Type: TCPConnect, Network: "tcp", ConnID: id,
		Src: src, Dst: dst, Host: host,
		Process: process, ProcessPath: processPath, InName: inName,
	})
	return s
}

func (s *TCPSession) ProxyDial(proxy, proxyType, proxyAddr string, endpoint EndpointInfo) {
	if !s.active {
		return
	}
	s.tracer.write(event{
		Type: TCPProxyDial, Network: "tcp", ConnID: s.id,
		Proxy: proxy, ProxyType: proxyType, ProxyAddr: proxyAddr,
		OutSrc: endpoint.Local, OutDst: endpoint.Remote,
		EndpointScope: endpoint.Scope, EndpointSource: endpoint.Source,
	})
}

func (s *TCPSession) Close(bytesUp, bytesDown int64, status, stage string, cause error) {
	if !s.active {
		return
	}
	s.once.Do(func() {
		s.tracer.write(event{
			Type: TCPClose, Network: "tcp", ConnID: s.id,
			BytesUp: bytesUp, BytesDown: bytesDown,
			DurationMs: time.Since(s.start).Milliseconds(),
			Status:     status, Stage: stage, Error: errorString(cause),
		})
		s.tracer.activeSessions.Add(-1)
	})
}

type UDPSession struct {
	tracer *Tracer
	key    string
	start  time.Time
	active bool
	once   sync.Once
	seqOut atomic.Uint64
	seqIn  atomic.Uint64

	mu        sync.RWMutex
	proxyName string
}

func BeginUDP(key, src, dst, host, process, processPath, inName string) *UDPSession {
	return globalTracer.beginUDP(key, src, dst, host, process, processPath, inName)
}

func (t *Tracer) beginUDP(key, src, dst, host, process, processPath, inName string) *UDPSession {
	s := &UDPSession{tracer: t, key: key, start: time.Now(), active: t.enabled.Load()}
	if !s.active {
		return s
	}
	t.activeSessions.Add(1)
	t.write(event{
		Type: UDPConnect, Network: "udp", ConnKey: key,
		Src: src, Dst: dst, Host: host,
		Process: process, ProcessPath: processPath, InName: inName,
	})
	return s
}

func (s *UDPSession) ProxyDial(proxy, proxyType, proxyAddr string, endpoint EndpointInfo) {
	s.mu.Lock()
	s.proxyName = proxy
	s.mu.Unlock()
	if !s.active {
		return
	}
	s.tracer.write(event{
		Type: UDPProxyDial, Network: "udp", ConnKey: s.key,
		Proxy: proxy, ProxyType: proxyType, ProxyAddr: proxyAddr,
		OutSrc: endpoint.Local, OutDst: endpoint.Remote,
		EndpointScope: endpoint.Scope, EndpointSource: endpoint.Source,
	})
}

func (s *UDPSession) PacketOut(src, dst string, length int) {
	if !s.active || !s.tracer.enabled.Load() {
		return
	}
	s.mu.RLock()
	proxy := s.proxyName
	s.mu.RUnlock()
	s.tracer.write(event{
		Type: UDPOut, Network: "udp", ConnKey: s.key,
		Seq: s.seqOut.Add(1), Src: src, Dst: dst, Len: length, Proxy: proxy,
	})
}

func (s *UDPSession) PacketIn(from string, length int) {
	if !s.active || !s.tracer.enabled.Load() {
		return
	}
	s.tracer.write(event{
		Type: UDPIn, Network: "udp", ConnKey: s.key,
		Seq: s.seqIn.Add(1), From: from, Len: length,
	})
}

func (s *UDPSession) Close(bytesUp, bytesDown int64, status, stage string, cause error) {
	if !s.active {
		return
	}
	s.once.Do(func() {
		s.tracer.write(event{
			Type: UDPClose, Network: "udp", ConnKey: s.key,
			BytesUp: bytesUp, BytesDown: bytesDown,
			DurationMs: time.Since(s.start).Milliseconds(),
			Status:     status, Stage: stage, Error: errorString(cause),
		})
		s.tracer.activeSessions.Add(-1)
	})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
