package proxysemantics

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/metacubex/mihomo/common/traffictrace"
)

const (
	SchemaVersion          = 1
	RedactionPolicyVersion = 1
	BehaviorHashVersion    = 1
)

type FieldState string

const (
	StateKnown         FieldState = "known"
	StateUnknown       FieldState = "unknown"
	StateNotApplicable FieldState = "not_applicable"
	StateUnsupported   FieldState = "unsupported"
)

type Field struct {
	State  FieldState `json:"state"`
	Value  any        `json:"value,omitempty"`
	Reason string     `json:"reason,omitempty"`
}

func Known(value any) Field {
	return Field{State: StateKnown, Value: value}
}

func Unknown(reason string) Field {
	return Field{State: StateUnknown, Reason: reason}
}

func NotApplicable(reason string) Field {
	return Field{State: StateNotApplicable, Reason: reason}
}

func Unsupported(reason string) Field {
	return Field{State: StateUnsupported, Reason: reason}
}

type Evidence struct {
	Configured map[string]Field `json:"configured"`
	Effective  map[string]Field `json:"effective"`
	Negotiated map[string]Field `json:"negotiated"`
	Observed   map[string]Field `json:"observed"`
}

type MissingField struct {
	Layer  string `json:"layer"`
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

type Coverage struct {
	SchemaVersion         int            `json:"schema_version"`
	ImplementationVersion int            `json:"implementation_version"`
	Status                string         `json:"status"`
	Missing               []MissingField `json:"missing,omitempty"`
}

type Snapshot struct {
	SchemaVersion          int           `json:"schema_version"`
	RedactionPolicyVersion int           `json:"redaction_policy_version"`
	SnapshotID             string        `json:"snapshot_id"`
	ConfigGeneration       uint64        `json:"config_generation"`
	AdapterInstanceID      string        `json:"adapter_instance_id"`
	Protocol               string        `json:"protocol"`
	Evidence               Evidence      `json:"evidence"`
	Coverage               Coverage      `json:"coverage"`
	BehaviorFingerprint    string        `json:"behavior_fingerprint"`
	Build                  BuildIdentity `json:"build"`
}

func (s Snapshot) Reference() traffictrace.AdapterReference {
	return traffictrace.AdapterReference{
		SnapshotID: s.SnapshotID, ConfigGeneration: s.ConfigGeneration,
		AdapterInstanceID: s.AdapterInstanceID, Protocol: s.Protocol,
		BehaviorFingerprint: s.BehaviorFingerprint,
	}
}

type Provider interface {
	TrafficTraceSemantics() Snapshot
}

type Builder struct {
	protocol          string
	configured        map[string]Field
	effective         map[string]Field
	missing           []MissingField
	implementationVer int
	configGeneration  uint64
}

func NewBuilder(protocol string, implementationVersion int) *Builder {
	return NewBuilderWithGeneration(protocol, implementationVersion, NextConfigGeneration())
}

func NewBuilderWithGeneration(protocol string, implementationVersion int, generation uint64) *Builder {
	return &Builder{
		protocol: protocol, configured: make(map[string]Field), effective: make(map[string]Field),
		implementationVer: implementationVersion, configGeneration: generation,
	}
}

func (b *Builder) Configured(field string, value Field) {
	b.configured[field] = value
}

func (b *Builder) Effective(field string, value Field) {
	b.effective[field] = value
}

func (b *Builder) Missing(layer, field, reason string) {
	b.missing = append(b.missing, MissingField{Layer: layer, Field: field, Reason: reason})
}

var (
	configGenerationCounter atomic.Uint64
	instanceCounter         atomic.Uint64
	bootToken               = newBootToken()
)

func NextConfigGeneration() uint64 {
	return configGenerationCounter.Add(1)
}

func newBootToken() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(raw[:])
}

func nextInstanceID() string {
	return fmt.Sprintf("adapter-%s-%06d", bootToken, instanceCounter.Add(1))
}

func canonicalHash(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("proxy semantics contains non-JSON value: %v", err))
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (b *Builder) Build() Snapshot {
	instanceID := nextInstanceID()
	generation := b.configGeneration
	negotiated := map[string]Field{
		"status": Unknown("no connection-specific negotiation evidence has been attached"),
	}
	observed := map[string]Field{
		"status": Unknown("no connection-specific trace evidence has been attached"),
	}
	behavior := struct {
		Version    int              `json:"version"`
		Protocol   string           `json:"protocol"`
		Configured map[string]Field `json:"configured"`
		Effective  map[string]Field `json:"effective"`
	}{
		Version: BehaviorHashVersion, Protocol: b.protocol,
		Configured: b.configured, Effective: b.effective,
	}
	behaviorFingerprint := canonicalHash(behavior)
	snapshotCore := struct {
		Generation uint64 `json:"config_generation"`
		InstanceID string `json:"adapter_instance_id"`
		Behavior   string `json:"behavior_fingerprint"`
		Schema     int    `json:"schema_version"`
		Redaction  int    `json:"redaction_policy_version"`
	}{
		Generation: generation, InstanceID: instanceID,
		Behavior: behaviorFingerprint, Schema: SchemaVersion, Redaction: RedactionPolicyVersion,
	}
	b.Missing("negotiated", "status", "requires connection-specific handshake evidence")
	b.Missing("observed", "status", "requires connection-specific trace evidence")
	coverageStatus := "complete"
	if len(b.missing) != 0 {
		coverageStatus = "partial"
	}
	return Snapshot{
		SchemaVersion: SchemaVersion, RedactionPolicyVersion: RedactionPolicyVersion,
		SnapshotID: canonicalHash(snapshotCore), ConfigGeneration: generation,
		AdapterInstanceID: instanceID, Protocol: b.protocol,
		Evidence: Evidence{
			Configured: b.configured, Effective: b.effective,
			Negotiated: negotiated, Observed: observed,
		},
		Coverage: Coverage{
			SchemaVersion: SchemaVersion, ImplementationVersion: b.implementationVer,
			Status: coverageStatus, Missing: b.missing,
		},
		BehaviorFingerprint: behaviorFingerprint,
		Build:               CurrentBuildIdentity(),
	}
}

func Clone(snapshot Snapshot) Snapshot {
	encoded, _ := json.Marshal(snapshot)
	var cloned Snapshot
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}
