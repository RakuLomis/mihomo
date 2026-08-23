package tracer

// Protocol versions are incremented only for incompatible tracing API or event
// envelope changes. Additive fields keep the current version.
const (
	TracingAPIVersion  = 1
	EventSchemaVersion = 1
)

type CapabilityName string

const (
	CapabilityTCP              CapabilityName = "tcp"
	CapabilityUDP              CapabilityName = "udp"
	CapabilityNormalizedFlow   CapabilityName = "normalized_flow"
	CapabilityOuterConnID      CapabilityName = "outer_conn_id"
	CapabilitySessionID        CapabilityName = "session_id"
	CapabilitySharedOuterFlow  CapabilityName = "shared_outer_flow"
	CapabilityEgressOutcome    CapabilityName = "egress_outcome"
	CapabilitySessionSink      CapabilityName = "session_sink_isolation"
	CapabilityTraceBarrier     CapabilityName = "trace_barrier"
	CapabilityCarrierLifecycle CapabilityName = "carrier_lifecycle"
	CapabilityCarrierBinding   CapabilityName = "logical_carrier_binding"
	CapabilityMultiPathCarrier CapabilityName = "multi_path_carrier"
	CapabilityProtocolSnapshot CapabilityName = "protocol_snapshot"
)

// Capabilities is the stable response body for tracing feature discovery.
// Fields intentionally do not use omitempty so older clients can distinguish a
// missing field from an explicitly unsupported capability.
type Capabilities struct {
	APIVersion               int  `json:"api_version"`
	EventSchemaVersion       int  `json:"event_schema_version"`
	SupportsTCP              bool `json:"supports_tcp"`
	SupportsUDP              bool `json:"supports_udp"`
	SupportsNormalizedFlow   bool `json:"supports_normalized_flow"`
	SupportsOuterConnID      bool `json:"supports_outer_conn_id"`
	SupportsSessionID        bool `json:"supports_session_id"`
	SupportsSharedOuterFlow  bool `json:"supports_shared_outer_flow"`
	SupportsEgressOutcome    bool `json:"supports_egress_outcome"`
	SupportsSessionSink      bool `json:"supports_session_sink_isolation"`
	SupportsTraceBarrier     bool `json:"supports_trace_barrier"`
	SupportsCarrierLifecycle bool `json:"supports_carrier_lifecycle"`
	SupportsCarrierBinding   bool `json:"supports_logical_carrier_binding"`
	SupportsMultiPathCarrier bool `json:"supports_multi_path_carrier"`
	SupportsProtocolSnapshot bool `json:"supports_protocol_snapshot"`
}

func CurrentCapabilities() Capabilities {
	return Capabilities{
		APIVersion:               TracingAPIVersion,
		EventSchemaVersion:       EventSchemaVersion,
		SupportsTCP:              true,
		SupportsUDP:              true,
		SupportsNormalizedFlow:   true,
		SupportsOuterConnID:      true,
		SupportsSessionID:        true,
		SupportsSharedOuterFlow:  true,
		SupportsEgressOutcome:    true,
		SupportsSessionSink:      true,
		SupportsTraceBarrier:     true,
		SupportsCarrierLifecycle: true,
		SupportsCarrierBinding:   true,
		SupportsMultiPathCarrier: true,
		SupportsProtocolSnapshot: true,
	}
}
