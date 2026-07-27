package tracer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type EventType string

const (
	TCPConnect     EventType = "tcp_connect"
	TCPProxyDial   EventType = "tcp_proxy_dial"
	TCPClose       EventType = "tcp_close"
	UDPOut         EventType = "udp_out"
	UDPIn          EventType = "udp_in"
	UDPConnect     EventType = "udp_connect"
	UDPClose       EventType = "udp_close"
)

type event struct {
	Ts          string `json:"ts"`
	Type        EventType `json:"type"`
	ConnID      string `json:"conn_id,omitempty"`
	ConnKey     string `json:"conn_key,omitempty"`
	Seq         int    `json:"seq,omitempty"`
	Src         string `json:"src,omitempty"`
	Dst         string `json:"dst,omitempty"`
	Host        string `json:"host,omitempty"`
	Process     string `json:"process,omitempty"`
	ProcessPath string `json:"process_path,omitempty"`
	InName      string `json:"in_name,omitempty"`
	Proxy       string `json:"proxy,omitempty"`
	ProxyType   string `json:"proxy_type,omitempty"`
	ProxyAddr   string `json:"proxy_addr,omitempty"`
	OutSrc      string `json:"out_src,omitempty"`
	Len         int    `json:"len,omitempty"`
	From        string `json:"from,omitempty"`
	BytesUp     int64  `json:"bytes_up,omitempty"`
	BytesDown   int64  `json:"bytes_down,omitempty"`
	DurationMs  int64  `json:"duration_ms,omitempty"`
}

type Tracer struct {
	enabled atomic.Bool
	mu      sync.Mutex
	writer  *bufio.Writer
	file    *os.File
}

var globalTracer = &Tracer{}

func SetEnabled(v bool) {
	globalTracer.enabled.Store(v)
}

func IsEnabled() bool {
	return globalTracer.enabled.Load()
}

func Output() string {
	globalTracer.mu.Lock()
	defer globalTracer.mu.Unlock()
	if globalTracer.file != nil {
		return globalTracer.file.Name()
	}
	if globalTracer.writer != nil {
		return "stdout"
	}
	return ""
}

func SetOutput(path string) error {
	globalTracer.mu.Lock()
	defer globalTracer.mu.Unlock()

	if globalTracer.file != nil {
		globalTracer.file.Close()
		globalTracer.file = nil
		globalTracer.writer = nil
	}

	if path == "" {
		globalTracer.writer = bufio.NewWriter(os.Stdout)
	} else {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("tracer: open output file: %w", err)
		}
		globalTracer.file = f
		globalTracer.writer = bufio.NewWriter(f)
	}
	return nil
}

func (t *Tracer) write(e event) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.writer == nil {
		return
	}
	b, _ := json.Marshal(e)
	t.writer.Write(b)
	t.writer.WriteByte('\n')
	_ = t.writer.Flush()
}

func now() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func Connect(id, src, dst, host, process, processPath, inName string) {
	t := globalTracer
	if !t.enabled.Load() {
		return
	}
	t.write(event{
		Ts: now(), Type: TCPConnect, ConnID: id,
		Src: src, Dst: dst, Host: host,
		Process: process, ProcessPath: processPath, InName: inName,
	})
}

func ProxyDial(id, proxy, proxyType, proxyAddr, outSrc string) {
	t := globalTracer
	if !t.enabled.Load() {
		return
	}
	t.write(event{
		Ts: now(), Type: TCPProxyDial, ConnID: id,
		Proxy: proxy, ProxyType: proxyType, ProxyAddr: proxyAddr, OutSrc: outSrc,
	})
}

func Close(id string, bytesUp, bytesDown int64, duration time.Duration) {
	t := globalTracer
	if !t.enabled.Load() {
		return
	}
	t.write(event{
		Ts: now(), Type: TCPClose, ConnID: id,
		BytesUp: bytesUp, BytesDown: bytesDown, DurationMs: duration.Milliseconds(),
	})
}

func UdpOut(key string, seq int, src, dst string, length int, proxy string) {
	t := globalTracer
	if !t.enabled.Load() {
		return
	}
	t.write(event{
		Ts: now(), Type: UDPOut, ConnKey: key, Seq: seq,
		Src: src, Dst: dst, Len: length, Proxy: proxy,
	})
}

func UdpIn(key string, seq int, from string, length int) {
	t := globalTracer
	if !t.enabled.Load() {
		return
	}
	t.write(event{
		Ts: now(), Type: UDPIn, ConnKey: key, Seq: seq,
		From: from, Len: length,
	})
}

func UDPConnectFn(key, src, dst, host, process, processPath, inName string) {
	t := globalTracer
	if !t.enabled.Load() {
		return
	}
	t.write(event{
		Ts: now(), Type: UDPConnect, ConnKey: key,
		Src: src, Dst: dst, Host: host,
		Process: process, ProcessPath: processPath, InName: inName,
	})
}

func UDPCloseFn(key string, bytesUp, bytesDown int64, duration time.Duration) {
	t := globalTracer
	if !t.enabled.Load() {
		return
	}
	t.write(event{
		Ts: now(), Type: UDPClose, ConnKey: key,
		BytesUp: bytesUp, BytesDown: bytesDown, DurationMs: duration.Milliseconds(),
	})
}
