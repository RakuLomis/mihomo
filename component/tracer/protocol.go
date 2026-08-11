package tracer

// Protocol versions are incremented only for incompatible tracing API or event
// envelope changes. Additive fields keep the current version.
const (
	TracingAPIVersion  = 1
	EventSchemaVersion = 1
)

type CapabilityName string

const (
	CapabilityTCP             CapabilityName = "tcp"
	CapabilityUDP             CapabilityName = "udp"
	CapabilityNormalizedFlow  CapabilityName = "normalized_flow"
	CapabilityOuterConnID     CapabilityName = "outer_conn_id"
	CapabilitySessionID       CapabilityName = "session_id"
	CapabilitySharedOuterFlow CapabilityName = "shared_outer_flow"
	CapabilityEgressOutcome   CapabilityName = "egress_outcome"
	CapabilitySessionSink     CapabilityName = "session_sink_isolation"
)

// Capabilities is the stable response body for tracing feature discovery.
// Fields intentionally do not use omitempty so older clients can distinguish a
// missing field from an explicitly unsupported capability.
type Capabilities struct {
	APIVersion              int  `json:"api_version"`
	EventSchemaVersion      int  `json:"event_schema_version"`
	SupportsTCP             bool `json:"supports_tcp"`
	SupportsUDP             bool `json:"supports_udp"`
	SupportsNormalizedFlow  bool `json:"supports_normalized_flow"`
	SupportsOuterConnID     bool `json:"supports_outer_conn_id"`
	SupportsSessionID       bool `json:"supports_session_id"`
	SupportsSharedOuterFlow bool `json:"supports_shared_outer_flow"`
	SupportsEgressOutcome   bool `json:"supports_egress_outcome"`
	SupportsSessionSink     bool `json:"supports_session_sink_isolation"`
}

func CurrentCapabilities() Capabilities {
	return Capabilities{
		APIVersion:              TracingAPIVersion,
		EventSchemaVersion:      EventSchemaVersion,
		SupportsTCP:             true,
		SupportsUDP:             true,
		SupportsNormalizedFlow:  true,
		SupportsOuterConnID:     true,
		SupportsSessionID:       true,
		SupportsSharedOuterFlow: true,
		SupportsEgressOutcome:   true,
		SupportsSessionSink:     true,
	}
}
