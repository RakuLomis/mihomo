# TrafficTracer Hysteria2 Carrier Correlation Plan

## Status

Implemented across the Mihomo TrafficTracer, TrafficTracer Complete, and Clash
Verge feat/traffic-tracer branches on 2026-08-23.

The implementation provides:

- an always-on, concurrency-safe Hysteria2 carrier registry;
- stable carrier IDs, generations, path updates, and close events;
- a complete carrier snapshot on every logical stream or packet-association bind;
- strict or observational capture-group proxy-protocol invariants;
- protocol-neutral analyzer states and shared-carrier PCAP artifacts;
- carrier, inbound, and proxy-protocol coverage in canonical reports and the UI.

The implementation deliberately does not claim that encrypted QUIC carrier bytes
can be divided uniquely among logical streams. A shared carrier is captured once
and referenced by every bound logical connection.

The plan is based on the capture groups:

- VLESS baseline: `20260822-084942-516-Vless-USA01-OKYun`
- Hysteria2 experiment: `20260822-133515-580`

Both captures used the same TrafficTracer, Clash Verge, and Mihomo revisions.

## Problem Statement

The Hysteria2 experiment completed all 64 page jobs and preserved browser-side
attribution:

- all 10,633 network requests were associated;
- all 1,404 browser transports were associated with Mihomo logical flows;
- 1,182 of 1,325 page flows that should have had egress evidence contained a
  post-proxy flow;
- 143 page flows unexpectedly lacked a post-proxy flow.

Raw Mihomo dial events isolate the problem to Hysteria2 carrier observation. All
2,104 VLESS proxy dial observations in the Hysteria2 capture had physical egress
information, while only 31 of 631 Hysteria2 observations did. Physical captures
nevertheless contain the active Hysteria2 UDP traffic. This means the browser and
logical-flow correlation is working, but reused QUIC/UDP carriers are not reliably
bound back to each logical flow.

Hysteria2 multiplexes logical TCP streams and UDP associations over a long-lived
QUIC/UDP carrier. The first stream can cause the physical socket to be created and
observed; later streams reuse it and therefore do not pass through the normal
socket-dial observation point. Port hopping can also change the carrier's remote
path without creating a new logical flow.

## Architecture Decision

Implement one protocol-neutral state machine, configured by runtime carrier
capabilities and supported by small protocol adapters. Do not implement a separate
analysis state machine for every proxy protocol.

Protocol names alone are insufficient to select connection semantics. VLESS can
use an exclusive TCP connection or a multiplexed WebSocket, gRPC, or other
transport. Hysteria2 and TUIC usually use shared QUIC carriers. WireGuard represents
a shared tunnel, while REJECT creates no socket. Runtime carrier behavior must be
authoritative.

The common behavior families are:

- `exclusive_socket`: a logical flow owns an observable physical socket;
- `shared_stream_carrier`: many logical streams share one carrier;
- `packet_association`: a logical UDP association uses a carrier;
- `tunnel_carrier`: many flows share a tunnel-level carrier;
- `no_socket`: policy or internal handling does not create egress traffic.

A capture group may require one proxy protocol, but this is a capture invariant,
not a reason to fork the state machine. DIRECT, REJECT, DNS, and other explicitly
non-proxy outcomes are exempt from the single-proxy-protocol check.

## State Model

### Logical flow states

```text
OBSERVED
  -> ROUTED
  -> EGRESS_SELECTED
  -> CARRIER_BINDING
       -> EXCLUSIVE_BOUND
       -> SHARED_BOUND
       -> NOT_APPLICABLE
       -> FAILED_BEFORE_CARRIER
       -> OBSERVATION_MISSING
  -> CLOSED | FAILED | CANCELLED
```

### Carrier states

```text
CREATED
  -> ACTIVE
  -> PATH_UPDATED | PORT_HOPPED
  -> REPLACED
  -> CLOSED
```

Logical flows and physical carriers have a many-to-one relationship. A shared
carrier must never be presented as an exclusive NAT mapping for one logical flow.

## Event Contract

Add protocol-neutral, additive events while retaining the existing `post_flow` and
`outer_conn_id` fields for compatibility:

- `carrier_open`
- `carrier_path_update`
- `logical_carrier_bind`
- `carrier_close`

Required data includes:

```json
{
  "carrier_id": "stable-id",
  "logical_conn_id": "logical-flow-id",
  "proxy_type": "Hysteria2",
  "relation": "created-or-reused",
  "shared": true,
  "generation": 1,
  "physical_paths": []
}
```

Add capability discovery fields:

- `supports_carrier_lifecycle`
- `supports_logical_carrier_binding`
- `supports_multi_path_carrier`
- `supports_protocol_snapshot`

## Atomic Implementation Tasks

### HY2-001: Freeze the experiment baseline

- Create sanitized fixtures for carrier creation, carrier reuse, missing `:0`
  endpoints, TCP-to-UDP binding, port hopping, DIRECT, REJECT, and mixed routing.
- Preserve the current VLESS and Hysteria2 coverage metrics as regression inputs.
- Ensure tests reproduce the current missing-carrier behavior before fixing it.

Acceptance: the fixture suite deterministically demonstrates the current defect.

### CORE-001: Add the carrier contract

- Define carrier, path, binding, lifecycle, generation, and sharing fields.
- Add the new event types and capability flags.
- Keep existing normalized flow events readable by old consumers.

Acceptance: golden contract tests validate old and new event forms.

### CORE-002: Add an always-on carrier registry

- Track active carriers independently of whether a trace sink is enabled.
- Store carrier identity, selected node and type, current and historical physical
  paths, generation, lifecycle state, sharing mode, and timestamps.
- Bound registry memory and remove closed or superseded carriers safely.

Acceptance: a carrier created before a page trace can still be described when a
later logical flow reuses it.

### CORE-003: Instrument Hysteria2 creation and reuse

- Observe the packet socket used to create a Hysteria2 QUIC connection.
- Promote a candidate carrier only after the QUIC/authentication path succeeds.
- Bind every TCP stream and UDP association to the current carrier.
- Replay the same stable carrier ID for reused streams.
- Allocate a new generation when the Hysteria2 client replaces its carrier.
- Prevent failed carrier attempts from replacing the active registry entry.
- Remove observer cross-talk around the mutable shared Sing dialer and concurrent
  `DialConn` calls.

Acceptance: concurrent Hysteria2 logical streams bind to the correct shared carrier
without creating false exclusive post-proxy flows.

### CORE-004: Track port hopping and path changes

- Keep the carrier ID stable when only the remote Hysteria2 port changes.
- Emit path-update events with event-sequence or timestamp validity boundaries.
- Preserve all observed paths and select the path active at logical-flow binding
  time as the compatibility `post_flow`.

Acceptance: port hopping does not create a false logical flow or lose physical
capture filters.

### CORE-005: Make carrier reuse visible across trace files

- Emit a complete current carrier snapshot with each logical binding.
- Do not require the current page trace to contain the original socket-creation
  event.
- Treat carrier closure after a page barrier as lifecycle information rather than
  a reason to hold the page analysis open indefinitely.

Acceptance: consecutive single-page Chrome sessions can reuse one core carrier and
still produce self-contained analysis inputs.

### TT-001: Extend TrafficTracer models and parsers

- Add `Carrier`, `CarrierPath`, `CarrierBinding`, `BindingStatus`, and
  `ProtocolProfile` models.
- Parse the additive core events.
- Retain the legacy single `post_flow` projection.
- Add `carrier_binding` and `physical_paths` to canonical artifacts.

Acceptance: old captures remain analyzable and new captures retain complete carrier
evidence.

### TT-002: Implement the unified state machine

- Drive state from observed lifecycle and binding evidence.
- Classify independent sockets as `EXCLUSIVE_BOUND`.
- Classify reused carriers as `SHARED_BOUND`.
- Classify REJECT and internal outcomes as `NOT_APPLICABLE`.
- Separate real dial failures from missing instrumentation.

Acceptance: normal Hysteria2 reuse no longer appears as `unexpected_missing`.

### TT-003: Enforce single-proxy-protocol capture groups

- Add `strict_single` and optional diagnostic `mixed` modes.
- Record the expected protocol at capture-group creation.
- Resolve selected leaf nodes for relevant proxy groups before capture.
- Exempt DIRECT, REJECT, DNS, and internal outcomes.
- Block strict captures when selected proxy leaves contain multiple protocols.
- Continue checking actual `leaf_proxy_type` values during capture and mark any
  runtime mismatch without discarding captured evidence.

Acceptance: a nominal Hysteria2 run cannot silently include VLESS proxy flows as the
latest experiment did.

### TT-004: Enforce a deterministic inbound

- Record the expected inbound mode and interface in the group manifest.
- For the current product, require TUN and validate `DEFAULT-TUN` observations.
- Launch the capture browser without an application-level HTTP or SOCKS proxy.
- Detect and report loopback mixed-port traffic as an inbound mismatch.

Acceptance: system-proxy state cannot silently change the experimental ingress
path.

### TT-005: Split shared-carrier packet captures correctly

- Store each shared physical capture once under a carrier artifact directory.
- Reference that artifact from each bound logical flow.
- Build filters from all carrier paths active in the relevant time range.
- Do not claim that encrypted QUIC packets can be uniquely assigned to individual
  internal streams.

Acceptance: no useful connection evidence is discarded and no duplicated carrier
PCAP is presented as an exclusive flow capture.

### TT-006: Correct reporting semantics

Report separately:

- request-to-transport coverage;
- transport-to-logical-flow coverage;
- logical-flow-to-carrier binding coverage;
- exclusive socket count;
- shared carrier count and fan-out;
- physical carrier observation coverage;
- expected and observed proxy protocols;
- protocol consistency;
- real network failures and instrumentation gaps.

Acceptance: quality status is based on meaningful protocol semantics instead of
requiring one independent physical socket per logical flow.

### UI-001: Expose protocol and carrier state

- Show inbound mode, expected proxy protocol, observed proxy protocols, consistency
  status, carrier behavior, carrier count, bound logical-flow count, and missing
  binding count.
- List the exact proxy groups that violate strict single-protocol mode.
- Distinguish a shared carrier from an exclusive post-proxy flow in connection
  details.

Acceptance: the user can verify experiment invariants before capture and understand
shared-carrier results afterward.

### QA-001: Regression and end-to-end verification

Cover Hysteria2 first-stream creation, stream reuse, concurrent streams, UDP
associations, reconnection, failed carrier creation, port hopping, trace-file
rotation, VLESS regression, DIRECT/REJECT classification, and strict protocol
validation. Run race tests around the shared Hysteria2 client and carrier registry.

Acceptance criteria:

- every successful Hysteria2 proxy flow is `SHARED_BOUND` or has an evidence-backed
  failure state;
- normal Hysteria2 reuse produces no `unexpected_missing` result;
- VLESS post-proxy coverage remains at the established baseline;
- logical flows reference the real shared UDP carrier and its actual paths;
- no association is invented without core or socket evidence.

## Operational Semantics

The core registry is independent of an active trace sink. Consequently, a carrier
opened by an earlier page can still be bound during a later Session. Every binding
contains a self-contained snapshot so rotating the trace file cannot erase the
evidence needed by the analyzer.

Port hopping appends a physical path while retaining the carrier ID. A successful
replacement increments the generation; a failed connection attempt never replaces
the active carrier. Lifecycle callbacks run outside the registry lock. Older
consumers may ignore every new additive field and event.

## Recommended Implementation Order

1. HY2-001
2. CORE-001 and CORE-002
3. CORE-003 and CORE-004
4. CORE-005
5. TT-001 and TT-002
6. TT-003 and TT-004
7. TT-005 and TT-006
8. UI-001
9. QA-001 and documentation updates

The core contract and Hysteria2 instrumentation must land before the analyzer starts
treating carrier reuse as successfully bound. The analyzer must not infer bindings
that the core cannot substantiate.
