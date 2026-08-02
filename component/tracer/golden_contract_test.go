package tracer

import (
	"bufio"
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/metacubex/mihomo/common/traffictrace"
)

type goldenCoverage struct {
	tcp, udp, ipv4, ipv6, shared, dialError bool
}

func TestCompleteGoldenEvents(t *testing.T) {
	fixtureRoot := os.Getenv("TRAFFICTRACER_GOLDEN_DIR")
	if fixtureRoot == "" {
		fixtureRoot = "testdata/complete"
	}
	paths, err := filepath.Glob(filepath.Join(fixtureRoot, "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no Complete tracing fixtures found")
	}

	var coverage goldenCoverage
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			validateGoldenFile(t, path, &coverage)
		})
	}
	if !coverage.tcp || !coverage.udp || !coverage.ipv4 || !coverage.ipv6 || !coverage.shared || !coverage.dialError {
		t.Fatalf("incomplete golden coverage: %+v", coverage)
	}
}

func validateGoldenFile(t *testing.T, path string, coverage *goldenCoverage) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var previous uint64
	lineNumber := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNumber++
		var event event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("line %d: decode JSON: %v", lineNumber, err)
		}
		if event.SchemaVersion != EventSchemaVersion {
			t.Fatalf("line %d: schema_version = %d, want %d", lineNumber, event.SchemaVersion, EventSchemaVersion)
		}
		if event.SessionID == "" {
			t.Fatalf("line %d: session_id is required in Complete fixtures", lineNumber)
		}
		if event.EventSeq <= previous {
			t.Fatalf("line %d: event_seq = %d, previous = %d", lineNumber, event.EventSeq, previous)
		}
		previous = event.EventSeq
		if _, ok := eventContractTypes[event.Type]; !ok {
			t.Fatalf("line %d: unsupported event type %q", lineNumber, event.Type)
		}
		switch event.Network {
		case "tcp":
			coverage.tcp = true
		case "udp":
			coverage.udp = true
		default:
			t.Fatalf("line %d: unsupported network %q", lineNumber, event.Network)
		}
		validateGoldenFlow(t, lineNumber, event.PreFlow, coverage)
		validateGoldenFlow(t, lineNumber, event.PostFlow, coverage)
		if event.Status == StatusDialError && event.Error != "" {
			coverage.dialError = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if lineNumber == 0 {
		t.Fatal("fixture is empty")
	}
}

func validateGoldenFlow(t *testing.T, lineNumber int, flow *traffictrace.FlowTuple, coverage *goldenCoverage) {
	t.Helper()
	if flow == nil {
		return
	}
	if flow.Shared {
		coverage.shared = true
	}
	for _, raw := range []string{flow.SrcIP, flow.DstIP} {
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			t.Fatalf("line %d: invalid flow IP %q: %v", lineNumber, raw, err)
		}
		coverage.ipv4 = coverage.ipv4 || addr.Is4()
		coverage.ipv6 = coverage.ipv6 || addr.Is6()
	}
	if !flow.Complete {
		if flow.Key != "" {
			t.Fatalf("line %d: incomplete flow has key %q", lineNumber, flow.Key)
		}
		return
	}
	src := net.JoinHostPort(flow.SrcIP, strconv.Itoa(int(flow.SrcPort)))
	dst := net.JoinHostPort(flow.DstIP, strconv.Itoa(int(flow.DstPort)))
	want := traffictrace.NewFlowTupleFromStrings(flow.Network, src, dst, flow.Source, flow.Scope, flow.Shared)
	if flow.Key != want.Key {
		t.Fatalf("line %d: flow key = %q, want %q", lineNumber, flow.Key, want.Key)
	}
}
