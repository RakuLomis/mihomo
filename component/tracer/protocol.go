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
)
