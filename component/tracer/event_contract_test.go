package tracer

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

var eventContractTypes = map[EventType]struct{}{
	TCPConnect:   {},
	TCPProxyDial: {},
	TCPClose:     {},
	UDPConnect:   {},
	UDPProxyDial: {},
	UDPOut:       {},
	UDPIn:        {},
	UDPClose:     {},
	TraceBarrier: {},
}

func TestEveryEventTypeCarriesVersionedSessionEnvelope(t *testing.T) {
	var output bytes.Buffer
	tr := newTracer(&output)
	tr.enabled.Store(true)
	sessionID := "session-contract"
	if err := tr.configure(ConfigPatch{SessionID: &sessionID}); err != nil {
		t.Fatal(err)
	}

	tcp := tr.beginTCP("tcp-contract", "127.0.0.1:12000", "1.1.1.1:443", "example.com", "curl", "/usr/bin/curl", "tun")
	tcp.ProxyDial("proxy", "ss", "proxy.example:443", EndpointInfo{
		Local: "192.0.2.10:30000", Remote: "198.51.100.20:443", Scope: "physical", Source: "socket",
	})
	tcp.Close(10, 20, StatusDialError, "read", errContractFixture)

	udp := tr.beginUDP("udp-contract", "127.0.0.1:13000", "8.8.8.8:53", "dns.google", "dig", "/usr/bin/dig", "tun")
	udp.ProxyDial("proxy", "tuic", "proxy.example:443", EndpointInfo{
		Local: "192.0.2.10:30001", Remote: "198.51.100.20:443", Scope: "physical", Source: "socket",
	})
	udp.PacketOut("127.0.0.1:13000", "8.8.8.8:53", 30)
	udp.PacketIn("8.8.8.8:53", 40)
	udp.Close(30, 40, StatusClosed, "", nil)
	if _, err := tr.barrier(); err != nil {
		t.Fatal(err)
	}

	seen := make(map[EventType]int, len(eventContractTypes))
	for _, event := range decodeEvents(t, output.Bytes()) {
		seen[event.Type]++
		if event.SchemaVersion != EventSchemaVersion {
			t.Errorf("%s schema_version = %d, want %d", event.Type, event.SchemaVersion, EventSchemaVersion)
		}
		if event.SessionID != sessionID {
			t.Errorf("%s session_id = %q, want %q", event.Type, event.SessionID, sessionID)
		}
	}
	if len(seen) != len(eventContractTypes) {
		t.Fatalf("observed event types = %v, want %v", seen, eventContractTypes)
	}
	for eventType := range eventContractTypes {
		if seen[eventType] != 1 {
			t.Errorf("%s count = %d, want 1", eventType, seen[eventType])
		}
	}
}

var errContractFixture = errors.New("fixture failure")

func TestEventContractTableCoversEveryDeclaredEventType(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "tracer.go", nil, 0)
	if err != nil {
		t.Fatalf("parse tracer.go: %v", err)
	}
	declared := make(map[EventType]struct{})
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || spec.Type == nil {
			return true
		}
		name, ok := spec.Type.(*ast.Ident)
		if !ok || name.Name != "EventType" || len(spec.Values) != 1 {
			return true
		}
		literal, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Errorf("decode EventType constant %s: %v", literal.Value, err)
			return true
		}
		declared[EventType(value)] = struct{}{}
		return true
	})
	if len(declared) == 0 {
		t.Fatal("no EventType constants found in tracer.go")
	}
	if !sameEventTypeSet(declared, eventContractTypes) {
		t.Fatalf("declared EventTypes = %v, contract table = %v", declared, eventContractTypes)
	}
}

func sameEventTypeSet(left, right map[EventType]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for eventType := range left {
		if _, ok := right[eventType]; !ok {
			return false
		}
	}
	return true
}
