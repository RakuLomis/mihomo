package tracer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
)

func boolPtr(v bool) *bool       { return &v }
func stringPtr(v string) *string { return &v }

func decodeEvents(t *testing.T, data []byte) []event {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) == 1 && len(lines[0]) == 0 {
		return nil
	}
	events := make([]event, 0, len(lines))
	for _, line := range lines {
		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
		events = append(events, e)
	}
	return events
}

func TestTCPConfiguredStdoutCompletesAfterDisable(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	if err := tr.configure(ConfigPatch{Enabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}

	session := tr.beginTCP("tcp-1", "127.0.0.1:1000", "1.1.1.1:443", "example.com", "curl", "/usr/bin/curl", "mixed")
	tr.enabled.Store(false)
	session.ProxyDial("proxy", "ss", "proxy.example:443", EndpointInfo{
		Local: "192.0.2.1:2000", Remote: "198.51.100.1:443", Scope: "physical", Source: "socket",
	})
	session.Close(12, 34, StatusClosed, "", nil)
	session.Close(99, 99, StatusClosed, "", nil)

	events := decodeEvents(t, output.Bytes())
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	if events[0].Type != TCPConnect || events[1].Type != TCPProxyDial || events[2].Type != TCPClose {
		t.Fatalf("unexpected lifecycle: %s, %s, %s", events[0].Type, events[1].Type, events[2].Type)
	}
	if events[1].OutDst != "198.51.100.1:443" || events[1].EndpointScope != "physical" {
		t.Fatalf("unexpected proxy endpoints: %+v", events[1])
	}
	if events[2].Status != StatusClosed || events[2].BytesUp != 12 || events[2].BytesDown != 34 {
		t.Fatalf("unexpected close event: %+v", events[2])
	}
	status := tr.status()
	if status.Output != "" || status.ActiveSessions != 0 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestUDPPacketSequenceAndProxySurviveSessionState(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	tr.enabled.Store(true)

	session := tr.beginUDP("udp-key", "127.0.0.1:1000", "8.8.8.8:53", "dns.google", "dig", "/usr/bin/dig", "tun")
	session.ProxyDial("quic-proxy", "tuic", "proxy.example:443", EndpointInfo{Scope: "logical"})
	session.PacketOut("127.0.0.1:1000", "8.8.8.8:53", 10)
	session.PacketOut("127.0.0.1:1000", "8.8.8.8:53", 20)
	session.PacketIn("8.8.8.8:53", 30)
	tr.enabled.Store(false)
	session.PacketOut("127.0.0.1:1000", "8.8.8.8:53", 40)
	session.Close(30, 30, StatusClosed, "", nil)

	events := decodeEvents(t, output.Bytes())
	if len(events) != 6 {
		t.Fatalf("got %d events, want 6", len(events))
	}
	if events[2].Type != UDPOut || events[2].Seq != 1 || events[2].Proxy != "quic-proxy" {
		t.Fatalf("unexpected first udp_out: %+v", events[2])
	}
	if events[3].Type != UDPOut || events[3].Seq != 2 {
		t.Fatalf("unexpected second udp_out: %+v", events[3])
	}
	if events[4].Type != UDPIn || events[4].Seq != 1 {
		t.Fatalf("unexpected udp_in: %+v", events[4])
	}
	if events[5].Type != UDPClose || events[5].Status != StatusClosed {
		t.Fatalf("unexpected udp_close: %+v", events[5])
	}
}

func TestConfigureFailureIsAtomic(t *testing.T) {
	var stdout bytes.Buffer
	tr := newTracer(&stdout)
	validPath := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := tr.configure(ConfigPatch{Enabled: boolPtr(false), Output: stringPtr(validPath)}); err != nil {
		t.Fatal(err)
	}
	before := tr.status()

	invalidPath := filepath.Join(t.TempDir(), "missing", "trace.jsonl")
	if err := tr.configure(ConfigPatch{Enabled: boolPtr(true), Output: &invalidPath}); err == nil {
		t.Fatal("expected invalid output path to fail")
	}
	after := tr.status()
	if after.Enabled != before.Enabled || after.Output != before.Output {
		t.Fatalf("configuration changed after failed patch: before=%+v after=%+v", before, after)
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestWriteFailureDisablesTracingAndIsObservable(t *testing.T) {
	tr := newTracer(errorWriter{})
	tr.enabled.Store(true)
	session := tr.beginTCP("tcp-error", "src", "dst", "", "", "", "")
	session.Close(0, 0, StatusClosed, "", nil)

	status := tr.status()
	if status.Enabled || status.WriteErrors != 1 || status.LastError != "disk full" || status.ActiveSessions != 0 {
		t.Fatalf("unexpected failure status: %+v", status)
	}
	if err := tr.configure(ConfigPatch{Enabled: boolPtr(true)}); err == nil {
		t.Fatal("expected re-enable without replacing failed output to fail")
	}
	recoveryPath := filepath.Join(t.TempDir(), "recovered.jsonl")
	if err := tr.configure(ConfigPatch{Enabled: boolPtr(true), Output: &recoveryPath}); err != nil {
		t.Fatalf("replace failed output: %v", err)
	}
	status = tr.status()
	if !status.Enabled || status.LastError != "" || status.WriteErrors != 1 {
		t.Fatalf("unexpected recovered status: %+v", status)
	}
}

func TestTCPDialFailureClosesLifecycle(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	tr.enabled.Store(true)
	session := tr.beginTCP("tcp-failure", "src", "dst", "", "", "", "")
	session.Close(0, 0, StatusDialError, "dial", errors.New("connection refused"))

	events := decodeEvents(t, output.Bytes())
	if len(events) != 2 || events[1].Type != TCPClose {
		t.Fatalf("unexpected failure lifecycle: %+v", events)
	}
	if events[1].Status != StatusDialError || events[1].Stage != "dial" || events[1].Error != "connection refused" || events[1].ErrorClass != "dial_failure" {
		t.Fatalf("unexpected failure close event: %+v", events[1])
	}
}

func TestEgressClassificationCoversCoreAdapterTypes(t *testing.T) {
	tests := []struct{ proxyType, want string }{
		{"DIRECT", EgressDirect},
		{"REJECT", EgressRejected},
		{"REJECT-DROP", EgressRejectedDrop},
		{"DNS", EgressInternalDNS},
		{"PASS", EgressPass},
		{"Compatible", EgressCompatible},
		{"", EgressUnknown},
		{"Shadowsocks", EgressProxy},
	}
	for _, tt := range tests {
		if got := classifyEgress(tt.proxyType); got != tt.want {
			t.Fatalf("classifyEgress(%q) = %q, want %q", tt.proxyType, got, tt.want)
		}
	}
}

func TestTerminalErrorClassUsesStableErrorEvidence(t *testing.T) {
	tests := []struct {
		name, status, stage, want string
		err                       error
	}{
		{"dns status", StatusResolveError, "resolve", "dns_resolution", errors.New("arbitrary text")},
		{"deadline", StatusDialError, "dial", "timeout", context.DeadlineExceeded},
		{"net timeout", StatusDialError, "dial", "timeout", &net.DNSError{IsTimeout: true}},
		{"refused", StatusDialError, "dial", "connection_refused", syscall.ECONNREFUSED},
		{"unreachable", StatusDialError, "dial", "network_unreachable", syscall.ENETUNREACH},
		{"canceled", StatusCanceled, "dial", "canceled", context.Canceled},
		{"generic dial", StatusDialError, "dial", "dial_failure", errors.New("opaque")},
		{"reject", StatusRejected, "reject", "policy_rejected", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyTerminalError(tt.status, tt.stage, tt.err); got != tt.want {
				t.Fatalf("classifyTerminalError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConcurrentEventsHaveUniqueSequence(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	tr.enabled.Store(true)
	session := tr.beginUDP("udp-concurrent", "src", "dst", "", "", "", "")
	session.ProxyDial("proxy", "ss", "proxy:443", EndpointInfo{})

	const count = 100
	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			session.PacketOut("src", "dst", 1)
		}()
	}
	wg.Wait()
	session.Close(count, 0, StatusClosed, "", nil)

	events := decodeEvents(t, output.Bytes())
	seenEvent := make(map[uint64]struct{}, len(events))
	seenPacket := make(map[uint64]struct{}, count)
	for _, e := range events {
		if _, ok := seenEvent[e.EventSeq]; ok {
			t.Fatalf("duplicate event sequence %d", e.EventSeq)
		}
		seenEvent[e.EventSeq] = struct{}{}
		if e.Type == UDPOut {
			seenPacket[e.Seq] = struct{}{}
		}
	}
	if len(seenPacket) != count {
		t.Fatalf("got %d unique packet sequences, want %d", len(seenPacket), count)
	}
}

type staticAddr string

func (a staticAddr) Network() string { return "test" }
func (a staticAddr) String() string  { return string(a) }

type endpointWrapper struct {
	local, remote net.Addr
	upstream      any
}

func (w *endpointWrapper) LocalAddr() net.Addr  { return w.local }
func (w *endpointWrapper) RemoteAddr() net.Addr { return w.remote }
func (w *endpointWrapper) Upstream() any        { return w.upstream }

type cyclicEndpoint struct{}

func (c *cyclicEndpoint) Upstream() any { return c }

func TestExtractEndpoints(t *testing.T) {
	inner := &endpointWrapper{local: staticAddr("inner-local"), remote: staticAddr("inner-remote")}
	outer := &endpointWrapper{local: staticAddr("outer-local"), upstream: inner}
	got := ExtractEndpoints(outer, "fallback:443")
	if got.Local != "outer-local" || got.Remote != "inner-remote" || got.Scope != "logical" {
		t.Fatalf("unexpected logical endpoints: %+v", got)
	}

	got = ExtractEndpoints(&cyclicEndpoint{}, "fallback:443")
	if got.Remote != "fallback:443" || got.Scope != "logical" || got.Source != "proxy_config" {
		t.Fatalf("unexpected cyclic fallback: %+v", got)
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	got = ExtractEndpoints(conn, "198.51.100.1:443")
	if got.Local == "" || got.Remote != "198.51.100.1:443" || got.Scope != "physical" {
		t.Fatalf("unexpected physical endpoints: %+v", got)
	}
}

func TestConfigureSessionIDPatchSemantics(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)

	first := "session-one"
	if err := tr.configure(ConfigPatch{SessionID: &first}); err != nil {
		t.Fatal(err)
	}
	if got := tr.status().SessionID; got != first {
		t.Fatalf("session ID = %q, want %q", got, first)
	}

	if err := tr.configure(ConfigPatch{}); err != nil {
		t.Fatal(err)
	}
	if got := tr.status().SessionID; got != first {
		t.Fatalf("omitted session ID changed value to %q", got)
	}

	empty := ""
	if err := tr.configure(ConfigPatch{SessionID: &empty}); err != nil {
		t.Fatal(err)
	}
	if got := tr.status().SessionID; got != "" {
		t.Fatalf("empty session ID did not clear value: %q", got)
	}
}

func TestConfigureOutputFailureDoesNotChangeSessionID(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	original := "session-original"
	if err := tr.configure(ConfigPatch{SessionID: &original}); err != nil {
		t.Fatal(err)
	}

	invalidPath := filepath.Join(t.TempDir(), "missing", "trace.jsonl")
	replacement := "session-replacement"
	if err := tr.configure(ConfigPatch{Output: &invalidPath, SessionID: &replacement}); err == nil {
		t.Fatal("expected invalid output path to fail")
	}
	if got := tr.status().SessionID; got != original {
		t.Fatalf("failed output patch changed session ID to %q", got)
	}
}

func decodeRawEvents(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) == 1 && len(lines[0]) == 0 {
		return nil
	}
	events := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("decode raw event %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

func TestEventsCarrySchemaVersionAndOptionalSessionID(t *testing.T) {
	var withoutSession bytes.Buffer
	plain := newTracer(&withoutSession)
	plain.enabled.Store(true)
	plainSession := plain.beginTCP("tcp-plain", "src", "dst", "", "", "", "")
	plainSession.Close(0, 0, StatusClosed, "", nil)
	for _, event := range decodeRawEvents(t, withoutSession.Bytes()) {
		if event["schema_version"] != float64(EventSchemaVersion) {
			t.Fatalf("schema_version = %v, want %d", event["schema_version"], EventSchemaVersion)
		}
		if _, ok := event["session_id"]; ok {
			t.Fatalf("unset session_id must be omitted: %v", event)
		}
	}

	var withSession bytes.Buffer
	tagged := newTracer(&withSession)
	tagged.enabled.Store(true)
	sessionID := "session-tagged"
	if err := tagged.configure(ConfigPatch{SessionID: &sessionID}); err != nil {
		t.Fatal(err)
	}
	taggedSession := tagged.beginUDP("udp-tagged", "src", "dst", "", "", "", "")
	taggedSession.ProxyDial("proxy", "ss", "proxy:443", EndpointInfo{})
	taggedSession.PacketOut("src", "dst", 10)
	taggedSession.Close(10, 0, StatusClosed, "", nil)
	for _, event := range decodeRawEvents(t, withSession.Bytes()) {
		if event["schema_version"] != float64(EventSchemaVersion) || event["session_id"] != sessionID {
			t.Fatalf("unexpected tagged event envelope: %v", event)
		}
	}
}

func TestConcurrentSessionConfigurationAndEvents(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	tr.enabled.Store(true)
	first := "session-a"
	if err := tr.configure(ConfigPatch{SessionID: &first}); err != nil {
		t.Fatal(err)
	}
	session := tr.beginUDP("udp-race", "src", "dst", "", "", "", "")

	const count = 100
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < count; i++ {
			value := "session-a"
			if i%2 == 1 {
				value = "session-b"
			}
			if err := tr.configure(ConfigPatch{SessionID: &value}); err != nil {
				t.Errorf("configure session ID: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < count; i++ {
			session.PacketOut("src", "dst", 1)
		}
	}()
	wg.Wait()
	session.Close(count, 0, StatusClosed, "", nil)

	for _, event := range decodeEvents(t, output.Bytes()) {
		if event.SchemaVersion != EventSchemaVersion {
			t.Fatalf("schema version = %d, want %d", event.SchemaVersion, EventSchemaVersion)
		}
		if event.SessionID != "session-a" && event.SessionID != "session-b" {
			t.Fatalf("unexpected session ID %q", event.SessionID)
		}
	}
}

func TestLegacyEventParserIgnoresCompleteEnvelopeFields(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	tr.enabled.Store(true)
	session := tr.beginTCP("legacy-conn", "127.0.0.1:14000", "1.1.1.1:443", "example.com", "curl", "/usr/bin/curl", "mixed")
	session.Close(4, 8, StatusClosed, "", nil)

	legacy := struct {
		Type    EventType `json:"type"`
		Network string    `json:"network"`
		ConnID  string    `json:"conn_id"`
		Src     string    `json:"src"`
		Dst     string    `json:"dst"`
		Host    string    `json:"host"`
	}{}
	firstLine := bytes.Split(output.Bytes(), []byte{'\n'})[0]
	if err := json.Unmarshal(firstLine, &legacy); err != nil {
		t.Fatalf("legacy parser rejected versioned event: %v", err)
	}
	if legacy.Type != TCPConnect || legacy.Network != "tcp" || legacy.ConnID != "legacy-conn" ||
		legacy.Src != "127.0.0.1:14000" || legacy.Dst != "1.1.1.1:443" || legacy.Host != "example.com" {
		t.Fatalf("legacy fields changed: %+v", legacy)
	}
}

func TestSessionKeepsSinkGenerationAfterReconfigure(t *testing.T) {
	tr := newTracer(&bytes.Buffer{})
	firstPath := filepath.Join(t.TempDir(), "first.jsonl")
	secondPath := filepath.Join(t.TempDir(), "second.jsonl")
	firstID := "session-first"
	secondID := "session-second"
	enabled := true

	if err := tr.configure(ConfigPatch{
		Enabled: &enabled, Output: &firstPath, SessionID: &firstID,
	}); err != nil {
		t.Fatal(err)
	}
	first := tr.beginTCP(
		"tcp-first", "src-a", "dst-a", "first.example", "", "", "",
	)

	if err := tr.configure(ConfigPatch{
		Output: &secondPath, SessionID: &secondID,
	}); err != nil {
		t.Fatal(err)
	}
	second := tr.beginTCP(
		"tcp-second", "src-b", "dst-b", "second.example", "", "", "",
	)
	second.ProxyDial("DIRECT", "Direct", "", EndpointInfo{
		Local: "192.0.2.2:2000", Remote: "198.51.100.2:443",
		Scope: "physical", Source: "socket",
	})
	second.Close(2, 3, StatusClosed, "", nil)

	// These late events belong to the first capture even though the global
	// tracing configuration now points at the second capture.
	first.ProxyDial("DIRECT", "Direct", "", EndpointInfo{
		Local: "192.0.2.1:1000", Remote: "198.51.100.1:443",
		Scope: "physical", Source: "socket",
	})
	first.Close(4, 5, StatusClosed, "", nil)

	read := func(path string) []event {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return decodeEvents(t, data)
	}
	firstEvents := read(firstPath)
	secondEvents := read(secondPath)
	if len(firstEvents) != 3 || len(secondEvents) != 3 {
		t.Fatalf("event counts first=%d second=%d", len(firstEvents), len(secondEvents))
	}
	for _, event := range firstEvents {
		if event.SessionID != firstID || event.ConnID != "tcp-first" {
			t.Fatalf("first sink contamination: %+v", event)
		}
	}
	for _, event := range secondEvents {
		if event.SessionID != secondID || event.ConnID != "tcp-second" {
			t.Fatalf("second sink contamination: %+v", event)
		}
	}
}
