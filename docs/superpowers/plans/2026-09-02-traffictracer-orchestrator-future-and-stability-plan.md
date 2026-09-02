# TrafficTracer Orchestrator Future Plan and Current Stability Roadmap

Status: cross-repository product plan, recorded on 2026-09-02.

Applies to:

- `TrafficTracer/Complete` (capture, analysis, Worker, contracts);
- `clash-verge-rev/feat/traffic-tracer` (UI and experiment supervisor);
- `mihomo/TrafficTracer` (bounded tracing and carrier evidence).

This copy is stored with the instrumented core because the current workspace
only grants write access to this repository. When the deferred work is
activated, the normative plan should also be indexed from the TrafficTracer
and Clash Verge documentation.

## 1. Current product decision

The current manual candidate workflow is accepted and remains the supported
design for the stability-focused release line:

1. activate a Profile;
2. select and manually validate a concrete node;
3. record the effective `(profile, selector, node)` tuple with **Add current
   pair**;
4. execute one complete serial `sites.yaml` Batch per tuple.

Providers and operational checks differ between protocols. Human validation is
therefore useful and is not treated as a defect. Automatic Profile inspection,
matrix selection, and broad experiment-planning features are deferred.

## 2. Deferred full orchestrator

### 2.1 Goals

The future system should let an operator construct, validate, execute, resume,
compare, and export a complete experiment without activating every Profile
manually. It must preserve the serial evidence model:

```text
experiment
  -> ordered Profile/selector/node run
    -> ordered target Batch
      -> isolated Session
```

It must not infer runtime usability solely from static YAML. Provider
expansion, merge scripts, Verge enhancements, selector chains, and Mihomo's
effective graph must be resolved before a tuple is executable.

### 2.2 Effective-config inspector

Add an isolated Mihomo validation service that can inspect an inactive Profile
without replacing the live proxy session. Its sanitized result should include:

- Profile UID and content fingerprint;
- selectors and selectable members;
- resolved chains and concrete leaves;
- adapter protocol type;
- provider readiness and validation failures;
- optional delay and reachability evidence.

Inspection cache entries must expire when the Profile, provider snapshot, core
version, or enhancement inputs change.

### 2.3 Matrix and immutable experiment plan

The future UI should support search, protocol filters, bulk selection,
duplicate detection, ordering, concrete-leaf display, and explicit warnings for
mutable `url-test` or `fallback` groups. Starting an experiment freezes a
versioned plan containing:

- candidate and target order plus fingerprints;
- `node_major` or `site_major` serial topology;
- repetitions, cache policy, settling, retry, and stop/continue policy;
- application success requirements;
- provenance and privacy-safe labels.

Resume must use this immutable snapshot and reject silent substitutions.

### 2.4 Transactional supervisor

Extend the current Pipeline into a durable transaction journal. Checkpoint and
verify every Profile, selector, connection-drain, Worker, Batch, Session, and
restore transition. If a future plan is allowed to control them, the journal
also covers core, TUN, system-proxy, and tracing state. Recovery must work after
completion, failure, interrupt, cancel, application crash, and machine restart.

### 2.5 Quality, history, and export

Expose three independent quality planes:

1. capture integrity;
2. request/pre-flow/post-flow/carrier correlation quality;
3. application outcome.

A media Session with valid correlation but no primary playback must remain
available and be marked application-invalid. Add durable experiment history,
per-run/target filters, retry-only-failed actions, protocol comparisons, and
privacy-audited dataset export with checksums and component identities.

### 2.6 Deferred atomic stages

- `FUTURE-ORCH-001`: versioned experiment-plan contract.
- `FUTURE-ORCH-002`: isolated effective-Profile inspector.
- `FUTURE-ORCH-003`: Profile/selector/node matrix UI.
- `FUTURE-ORCH-004`: whole-plan preflight.
- `FUTURE-ORCH-005`: transaction journal and full-state restore.
- `FUTURE-ORCH-006`: topology, repetition, settling, and bounded retry policy.
- `FUTURE-ORCH-007`: three-plane quality aggregation.
- `FUTURE-ORCH-008`: experiment comparison and dataset export.

This work should start only when manual candidate entry becomes the dominant
operational cost and the current stability gates below are met.

## 3. Current stability scope

The next release line focuses on correctness, recovery, and observability. It
does not add the automatic matrix, parallel capture, multi-tab capture, storage
compression, Windows packaging, or a broad UI redesign.

### Stability invariants

- A queued tuple is never silently replaced.
- Capture cannot start before Profile/node readback, chain resolution,
  connection draining, preflight, and a durable checkpoint pass.
- Only one Pipeline, Batch, or Session owns capture resources at a time.
- A terminal Batch is not released until its Worker Job is terminal.
- Original state is restored and verified on every terminal path.
- Application failure remains distinct from correlation success.
- Partial evidence is retained and classified rather than discarded.

## 4. P0 correctness tasks

### STAB-001: Three-plane quality reporting

Aggregate capture integrity, correlation quality, and application outcome
separately. `PLAYER_NOT_CREATED`, `MEDIA_NOT_ADVANCING`, blocking pages, final
URL, primary seconds, and desired seconds must be visible in Pipeline status.
Do not break existing Session correlation contracts.

Acceptance: the 2026-09-02 three-protocol experiment reports VLESS as
playback-valid and the observed Hy2/SS YouTube Sessions as
application-degraded while preserving their valid correlation data.

### STAB-002: Evidence-based node transition barrier

Replace fixed sleeps as the only settling proof with bounded polling:

1. activate Profile and wait for the real Controller;
2. verify the effective Profile fingerprint;
3. select and read back the requested node;
4. resolve and persist the chain and concrete leaf;
5. close old connections and poll until drained or deadline;
6. retain a short minimum quiet interval;
7. snapshot the chain immediately before Batch start.

Failure to drain must stop the run before capture rather than mix node traffic.

### STAB-003: Node/protocol drift detection

At Batch completion, resolve selector and leaf again and compare them with the
start snapshot and bounded trace evidence. Report `node_drift`,
`protocol_mismatch`, and `observation_unavailable` separately. Equal protocol
names do not prove that the requested node remained selected.

### STAB-004: Startup and finalization handoff tests

Status: completed on 2026-09-02.

Treat initial Batch-manifest persistence, Job-manager registration, supervisor
polling, and capture-lock ownership as one ordered handoff. Preserve the
`starting_batch`, `reconciling_batch`, and `finalizing_batch` stages. Inject
faults at each persistence boundary and prove that the UI cannot report a
terminal error while capture is still active.

Implemented with initial-manifest, Job-registration and thread-start fault
injection; ambiguous Worker responses retain ownership and reconcile the
pre-generated Job ID. Terminal Batch state cannot release ownership until the
Worker Job is also terminal.

### STAB-005: Verified restoration

Read back the original Profile and every affected selector after restoration.
Persist request failures, readback mismatches, and Controller unavailability as
different outcomes. A restoration warning remains visible until acknowledged
or successfully recovered.

## 5. P1 resilience tasks

### STAB-006: Lightweight whole-queue preflight

Status: completed on 2026-09-02.

Before the first Session, validate manually recorded candidate identity,
Profile fingerprints, tuple uniqueness, target/config hashes, output path,
interfaces, tools, TUN, tracing capabilities, and capture ownership. Inactive
Profiles still receive just-in-time runtime validation; static YAML must not be
presented as provider-readiness proof.

The start gate now reuses Batch validation, validates the frozen config and
candidate queue, checks active runtime evidence, acquires the pipeline lock and
runs the full environment diagnostic before launching the supervisor.

### STAB-007: Bounded application recovery

Allow one opt-in retry for explicitly classified transient outcomes such as a
blocking interstitial, player-not-created, or media-not-advancing result. Use a
fresh managed Chrome process and new Session attempt, preserve the failed
attempt, and never loop indefinitely.

### STAB-008: Worker ownership watchdog

Status: completed on 2026-09-02.

Persist a lightweight heartbeat and owner record. On UI reload or application
restart, reconcile Pipeline, Worker Job, Batch manifest, capture lock, and OS
process evidence before declaring interruption. Never terminate an unrelated
core or browser process.

`pipeline-owner.json` records the supervisor PID, stage, Batch ID and bounded
heartbeat. Recovery cross-checks it with the in-memory supervisor, capture
lock, Worker active Job and OS process evidence; it never kills a process.

### STAB-009: Interrupt/cancel/resume state matrix

Status: completed on 2026-09-02.

Test requests during Profile activation, selection, drain, preflight, Batch
startup, capture, analysis, finalization, and restore. Interrupt retains a
cursor, cancel is terminal, completed targets never rerun, and the interrupted
target creates a new Session attempt in the same Batch.

Stop checks now cover every pre-Batch boundary and the active Batch loop with
consistent cancel precedence. The Worker exposes `job.interrupt` so a Batch
manifest visibility gap cannot downgrade resumable interruption to terminal
cancellation. Existing resume tests prove completed children are retained and
the interrupted child receives a new Session attempt.

### STAB-010: Trace-tail classification

Keep the trace barrier authoritative. Separate causal late events, close-only
cleanup, explicit pre-socket failures, and unattributed capture-tail flows. Use
a bounded final drain only while causal activity remains; do not wait for
unrelated long-lived connections.

## 6. P2 observability and release gates

### STAB-011: Persistent progress and failures

Keep Pipeline status across navigation and refresh. Show candidate, target,
attempt, stage, elapsed time, last durable checkpoint, and full persistent
error. A toast is never the only failure record.

### STAB-012: Pipeline validation summary

Generate machine-readable and human-readable summaries with the run/target
matrix, requested/resolved/observed node and protocol, correlation/PCAP
coverage, application outcome, retries, interruptions, drift, restoration,
component revisions, config hashes, and package identity.

### STAB-013: Three-protocol release gate

Use manually verified VLESS, Hysteria2, and Shadowsocks nodes with the small
four-target configuration. A candidate passes when:

- all runs execute serially and restore original state;
- protocol evidence agrees or is explicitly classified;
- every proxy logical flow has a valid carrier binding;
- page-attributed unexpected missing post-flows are zero;
- application failures are independently visible;
- interrupt/resume and application-restart recovery pass;
- no terminal UI error appears while a Worker Job is active.

Network-dependent application outcomes do not have to succeed on every node,
but classification must be correct and failure must not corrupt later runs.

## 7. Recommended order

1. `STAB-001`, `STAB-011`: truthful quality and persistent error reporting.
2. `STAB-002`, `STAB-003`, `STAB-005`: transition and restoration proof.
3. `STAB-004`: startup/finalization fault-injection coverage.
4. `STAB-006`, `STAB-008`, `STAB-009`: queue and recovery resilience.
5. `STAB-007`, `STAB-010`: bounded application and trace-tail recovery.
6. `STAB-012`, `STAB-013`: summary and three-protocol release gate.

Each task lands as an independently testable change. Any dataset-schema change
requires an explicit version decision; UI aggregation should reuse existing
evidence fields where possible.
