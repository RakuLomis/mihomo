package tracer

import (
	"encoding/json"
	"os"
	"reflect"
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
	}
	if !reflect.DeepEqual(fixture.Capabilities, wantCapabilities) {
		t.Fatalf("fixture capabilities = %q, want %q", fixture.Capabilities, wantCapabilities)
	}
}
