package tracer

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"
)

type protocolFixture struct {
	APIVersion         int              `json:"api_version"`
	EventSchemaVersion int              `json:"event_schema_version"`
	Capabilities       []CapabilityName `json:"capabilities"`
}

func TestProtocolVersionsArePositive(t *testing.T) {
	if TracingAPIVersion <= 0 {
		t.Fatalf("TracingAPIVersion = %d, want a positive integer", TracingAPIVersion)
	}
	if EventSchemaVersion <= 0 {
		t.Fatalf("EventSchemaVersion = %d, want a positive integer", EventSchemaVersion)
	}
}

func TestProtocolConstantsMatchFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/protocol.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture protocolFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode protocol fixture: %v", err)
	}

	if fixture.APIVersion != TracingAPIVersion {
		t.Fatalf("fixture api_version = %d, want %d", fixture.APIVersion, TracingAPIVersion)
	}
	if fixture.EventSchemaVersion != EventSchemaVersion {
		t.Fatalf("fixture event_schema_version = %d, want %d", fixture.EventSchemaVersion, EventSchemaVersion)
	}
	wantCapabilities := []CapabilityName{
		CapabilityTCP,
		CapabilityUDP,
		CapabilityNormalizedFlow,
		CapabilityOuterConnID,
		CapabilitySessionID,
		CapabilitySharedOuterFlow,
		CapabilityEgressOutcome,
		CapabilitySessionSink,
		CapabilityTraceBarrier,
	}
	if !reflect.DeepEqual(fixture.Capabilities, wantCapabilities) {
		t.Fatalf("fixture capabilities = %q, want %q", fixture.Capabilities, wantCapabilities)
	}
}

func TestCapabilitiesJSONKeysAndCurrentValues(t *testing.T) {
	data, err := json.Marshal(CurrentCapabilities())
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{
		"api_version",
		"event_schema_version",
		"supports_egress_outcome",
		"supports_normalized_flow",
		"supports_outer_conn_id",
		"supports_session_id",
		"supports_session_sink_isolation",
		"supports_shared_outer_flow",
		"supports_tcp",
		"supports_trace_barrier",
		"supports_udp",
	}
	gotKeys := make([]string, 0, len(got))
	for key := range got {
		gotKeys = append(gotKeys, key)
	}
	sort.Strings(gotKeys)
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("capability JSON keys = %q, want %q", gotKeys, wantKeys)
	}
	if got["api_version"] != float64(TracingAPIVersion) || got["event_schema_version"] != float64(EventSchemaVersion) {
		t.Fatalf("unexpected capability versions: %s", data)
	}
	for _, key := range wantKeys[2:] {
		if got[key] != true {
			t.Fatalf("%s = %v, want true", key, got[key])
		}
	}
}

func TestCapabilitiesZeroValueIsExplicit(t *testing.T) {
	data, err := json.Marshal(Capabilities{})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 11 {
		t.Fatalf("zero-value capabilities encoded %d fields, want 11: %s", len(got), data)
	}
	if got["api_version"] != float64(0) || got["supports_tcp"] != false {
		t.Fatalf("unexpected zero-value encoding: %s", data)
	}
}
