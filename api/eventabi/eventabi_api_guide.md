# Event ABI API Guide

This guide defines the target architecture and module breakdown for Event ABI API contracts.
It is grounded in:
- `docs/PRD.md` (sections 4.1.3, 4.1.5, 4.1.6, 4.2, 4.3, 5.2, 5.3)
- `docs/rspp_SystemDesign.md` (sections 3 and 4)
- `docs/RSPP_features_framework.json` (feature IDs cited below)

Current implementation note:
- The repository currently exposes split artifacts (`EventRecord`, `ControlSignal`) in `api/eventabi/types.go`.
- The target design below keeps backward compatibility while converging on a unified logical Event ABI envelope contract (`F-020`, `F-021`, `F-022`).

## Ownership

- Primary: Runtime-Team
- Required cross-review for Event ABI contract changes: CP-Team, ObsReplay-Team
- Security-sensitive Event ABI changes: Security-Team

## Evidence map

| Contract area | PRD / system design anchor | Feature IDs |
| --- | --- | --- |
| Unified envelope + schema evolution | PRD 4.1.3, 5.3; SystemDesign 3.1/4 | `F-020` `F-021` `F-028` `F-032` `F-162` `F-163` `NF-010` `NF-032` |
| Event kinds + lane semantics | PRD 4.1.3, 4.2, 4.3 | `F-022` `F-023` `F-024` `F-025` `F-142` `F-147` |
| Cancellation + turn lifecycle | PRD 4.1.5, 4.2, 4.3 | `F-033` `F-034` `F-038` `F-039` `F-139` `F-141` `F-175` |
| Replay + determinism provenance | PRD 4.1.6, 4.3 | `F-029` `F-149` `F-150` `F-151` `F-153` `F-154` `F-176` `NF-009` |
| Authority + dedupe safety | PRD 4.1.8, 4.3 | `F-156` `F-158` `F-159` `NF-018` |
| Multi-encoding transport parity | PRD 4.1.3, 4.1.8; SystemDesign 3.3 | `F-030` `F-031` `F-086` `F-090` |
| Extensibility boundaries | PRD 4.1.4, 4.1.7, 4.1.9 | `F-011` `F-121` `F-122` `F-123` `F-124` `F-164` `F-171` |
| Security and data handling | PRD 4.1.9; SystemDesign 4 | `F-117` `F-118` `F-168` `F-169` `F-170` `NF-011` `NF-014` |

## 0) Current Codebase Progress and Divergence (`api/eventabi`) (as of February 16, 2026)

Evidence baseline:
- Package files: `api/eventabi/types.go`, `api/eventabi/types_test.go`.
- Verification run: `go test ./api/eventabi` passed on February 16, 2026.

### Implemented now (code-backed)

| Capability | Status | Code evidence | Feature relevance |
| --- | --- | --- | --- |
| Core enums for lane/scope/payload/redaction | implemented | `types.go` defines `Lane`, `EventScope`, `PayloadClass`, `RedactionAction` | `F-021` `F-022` `F-025` `F-117` |
| Split contract artifacts (`EventRecord`, `ControlSignal`) with shared core identity/timing fields | implemented | `EventRecord` + `ControlSignal` structs and `Validate()` methods | `F-020` `F-021` |
| Control signal name registry and emitter enum validation | implemented | `isControlSignalName`, `isControlSignalEmitter` | `F-025` |
| Strict validation for key signal families (turn lifecycle, admission, flow control, drop/discontinuity, provider, authority, connection, output markers) | implemented | `ControlSignal.Validate()` branch checks | `F-033` `F-035` `F-139` `F-141` `F-145` `F-175` |
| Redaction decision validation | implemented | `RedactionDecision.Validate()` and `TestRedactionDecisionValidate` | `F-117` `NF-014` |
| Baseline unit tests | partial | `types_test.go` has 2 tests, focused on selected control-signal and redaction paths | `F-028` `NF-013` |

### Divergences from target proposal (must be tracked explicitly)

| Target area | Current divergence in `api/eventabi` | Impact | Feature relevance |
| --- | --- | --- | --- |
| Unified Event envelope object | Not implemented as a first-class single type; package still uses split `EventRecord` and `ControlSignal` structs | Extra mapping burden for runtime/transport/tooling integrations | `F-020` `F-086` |
| Full envelope field set | Missing fields in Go types: `node_id`, `causal_parent_id`, `idempotency_key`, `provider_invocation_id`, `sync_id`, `merge_group_id`, `merged_from_event_ids`, `late_after_cancel`, and extension-safe metadata slots | Prevents full replay/dedupe/lineage semantics in this API package | `F-149` `F-150` `F-158` `F-160` |
| Unknown event handling policy | No `unknown_event_action` contract type or validation path in this package | Unknown-field/signal behavior is not contract-governed at API boundary | `F-020` `NF-013` |
| Explicit event kind taxonomy | No explicit `event_kind` contract (`audio|text|control|metrics|debug|error`) in shared types | Event-kind level validation and tooling generation are weaker | `F-022` `F-032` |
| Control signal parity gaps | `ControlSignal.Validate()` does not enforce schema-level strict mappings for `turn_open_proposed`, `barge_in|stop|cancel` emitter constraints, and `budget_warning|budget_exhausted|degrade|fallback` emitter/reason constraints | Allows signal combinations that violate intended control-lane semantics | `F-025` `F-033` `F-035` `F-144` |
| Cancel scope value constraints | `scope` is only checked for non-empty on `cancel`; allowed values are not constrained in Go validation | Risk of invalid scope values leaking into runtime paths | `F-034` `F-175` |
| Compatibility governance surfaces | No first-class compatibility report/deprecation/version-skew types in this package | Implementation of schema lifecycle policy is not API-visible here | `F-028` `F-162` `F-163` `NF-010` |
| Encoding/compression negotiation surfaces | No API types/interfaces for negotiated encoding/compression contract | Transport boundary behavior is not represented in eventabi API types | `F-030` `F-031` `F-086` |
| Extension namespace governance | No explicit extension namespace contract in current structs | Plugin/custom-runtime fields cannot be governed safely at contract level | `F-121` `F-164` |
| Test coverage depth | No dedicated tests for `EventRecord.Validate()` branches and many `ControlSignal.Validate()` branches | Regressions in contract strictness can slip through | `F-028` `NF-009` |

## 1) Target module list with responsibilities

The Event ABI contract surface should be organized into these logical modules.

| Module | Responsibilities | Representative contract types | Feature trace |
| --- | --- | --- | --- |
| `envelope_core` | Canonical event identity, scope, lane, time, authority, and correlation fields | `EventEnvelope`, `EventID`, `EventScope`, `Lane`, `AuthorityRef` | `F-020` `F-021` `F-013` `F-156` `NF-018` |
| `payload_union` | Strong payload taxonomy and lane-legal payload pairings | `AudioPayload`, `TextPayload`, `ControlPayload`, `MetricsPayload`, `DebugPayload`, `ErrorPayload` | `F-022` `F-023` `F-024` `F-026` `F-027` |
| `control_contract` | Control signal vocabulary, emitter mapping, scope restrictions, and sequencing constraints | `ControlSignalName`, `ControlEmitter`, `ControlScope`, `SeqRange` | `F-025` `F-033` `F-035` `F-139` `F-141` `F-175` |
| `lane_qos_contract` | Priority, buffering, flow-control, and discontinuity metadata contracts | `TargetLane`, `FlowControlSignal`, `DropNotice`, `DiscontinuityMarker` | `F-142` `F-143` `F-144` `F-145` `F-146` `F-147` `F-148` |
| `compatibility_contract` | Schema versions, compatibility policy, unknown-event policy, deprecation lifecycle | `SchemaVersion`, `UnknownEventAction`, `CompatibilityReport`, `DeprecationWindow` | `F-020` `F-028` `F-032` `F-162` `F-163` `NF-010` |
| `encoding_transport_contract` | Encoding/compression negotiation boundaries; transport equivalence guarantees | `EncodingKind`, `CompressionKind`, `TransportNegotiation`, `CodecPlan` | `F-030` `F-031` `F-086` `F-090` |
| `ordering_dedupe_contract` | Ordering lineage and idempotency/dedupe semantics across retries and failovers | `IdempotencyKey`, `CausalParentRef`, `MergeGroupRef`, `DedupDecision` | `F-008` `F-029` `F-158` `NF-009` |
| `replay_provenance_contract` | Replay-critical markers and recording-level obligations | `DeterminismMarker`, `PlanProvenanceRef`, `RecordingLevelMarker` | `F-149` `F-150` `F-151` `F-153` `F-154` `F-176` `NF-028` |
| `security_redaction_contract` | Payload class to redaction decision contract and data-safety semantics | `PayloadClass`, `RedactionAction`, `RedactionDecision` | `F-117` `F-118` `F-168` `F-170` `NF-011` `NF-014` |
| `extension_contract` | Namespaced extension envelopes for plugins/custom nodes/custom runtimes | `ExtensionNamespace`, `ExtensionField`, `ExtensionValidationRule` | `F-011` `F-121` `F-122` `F-124` `F-164` `F-171` |

## 2) Key interfaces (pseudocode)

```go
// Core unified contract view. Backward-compatible wrappers can expose
// EventRecord/ControlSignal while internally mapping to this shape.
// (F-020, F-022, F-025)
type Event interface {
    Envelope() EventEnvelope
    Kind() EventKind // audio|text|control|metrics|debug|error
    Validate(ctx ValidationContext) error
}

// Envelope fields needed for compatibility, authority, ordering, and dedupe.
// (F-021, F-156, F-158, NF-018)
type EventEnvelope struct {
    SchemaVersion string
    EventScope    EventScope   // session|turn
    SessionID     string
    TurnID        string       // required when scope=turn
    PipelineVersion string
    EventID       string
    NodeID        string
    EdgeID        string
    Lane          Lane         // DataLane|ControlLane|TelemetryLane

    CausalParentID string
    IdempotencyKey string
    ProviderInvocationID string

    TransportSequence int64
    RuntimeSequence   int64
    RuntimeTimestampMS int64
    WallClockTimestampMS int64
    MediaTime *MediaTime

    SyncID           string
    SyncDomain       string
    DiscontinuityID  string
    MergeGroupID     string
    MergedFromEventIDs []string
    LateAfterCancel  bool

    AuthorityEpoch int64
}

// Contract-level validation, including extension-profile checks where enabled.
// (F-028, F-164, NF-013)
type Validator interface {
    ValidateEvent(evt Event, policy ValidationPolicy) ([]Violation, error)
}

// Schema compatibility and skew/deprecation policy checks.
// (F-028, F-162, F-163, NF-010)
type CompatibilityChecker interface {
    Check(prev SchemaDescriptor, next SchemaDescriptor) CompatibilityReport
}

// Required unknown-event handling path with auditable decisions.
// (F-020)
type UnknownEventResolver interface {
    ResolveUnknown(evt RawEvent, action UnknownEventAction) (Decision, AuditMarker, error)
}

// Encoding/compression negotiation without changing node ABI semantics.
// (F-030, F-031, F-086, F-090)
type TransportCodecNegotiator interface {
    Negotiate(cap TransportCapabilities, req SessionTransportRequest) (CodecPlan, error)
}

// Dedupe semantics for retries/reconnect/failover.
// (F-158, NF-009)
type DedupeEngine interface {
    Evaluate(envelope EventEnvelope, window DedupWindow) DedupDecision
}

// Namespaced extension governance for plugins/custom integrations.
// (F-121, F-164)
type ExtensionRegistry interface {
    RegisterNamespace(ns ExtensionNamespace, rule ExtensionValidationRule) error
    ValidateExtensions(evt Event) error
}

// Custom runtime adapter boundary: every adapter must preserve Event ABI semantics.
// (F-086, F-083, F-084, F-085)
type RuntimeAdapter interface {
    Name() string
    NormalizeIngress(raw TransportFrame) (Event, error)
    EncodeEgress(evt Event, plan CodecPlan) (TransportFrame, error)
    CapabilitySnapshot() CapabilitySnapshot
}

// Custom/external node boundary: stable Event in/out ABI regardless of isolation mode.
// (F-122, F-123, F-124, F-171)
type ExternalNodeRuntime interface {
    Invoke(ctx NodeExecutionContext, in <-chan Event) (<-chan Event, error)
}
```

## 3) Data model: core entities and relations

### Core entities

| Entity | Key fields | Notes |
| --- | --- | --- |
| `EventEnvelope` | `event_id`, `session_id`, `turn_id`, `pipeline_version`, `lane`, sequence/timestamps, authority, idempotency, merge/sync lineage | Shared by every event (`F-020`, `F-021`, `F-158`) |
| `EventPayload` | `kind` + kind-specific payload body | One-of payload kinds (`F-022`) |
| `ControlMetadata` | `signal`, `emitted_by`, `scope`, `reason`, `target_lane`, `seq_range` | Applies only when `lane=ControlLane` (`F-025`, `F-145`) |
| `ReplayMarker` | `plan_hash`, `determinism_seed`, ordering markers, recording level | Replay-critical provenance (`F-149`, `F-150`, `F-153`) |
| `AuthorityRef` | `authority_epoch` (+ lease token when available) | Required for authority safety (`F-156`) |
| `RedactionDecision` | `payload_class`, `action` | Security and compliance contract (`F-117`, `NF-014`) |
| `ExtensionBlock` | `namespace -> opaque extension value` | Plugin/custom-runtime extensibility with namespacing (`F-121`, `F-164`) |

### Relations

- One `Session` has many `Turn`s; one `Turn` has many `Event`s (`F-013`, `F-139`).
- One `Event` has exactly one `EventEnvelope` and one `EventPayload` (`F-020`, `F-022`).
- A `Control` event references `ControlMetadata`; non-control payload kinds must not (`F-025`, `F-142`).
- An `Event` may reference one parent event (`causal_parent_id`) and may aggregate many source events (`merged_from_event_ids`) (`F-146`, `F-150`).
- Many events can share one `provider_invocation_id`; this is the boundary unit for provider outcomes and cancellation scope (`F-160`).
- A `ReplayMarker` links an event stream to resolved-turn-plan provenance and recording-level behavior (`F-149` to `F-154`, `F-176`).
- `RedactionDecision` applies by payload class; redaction cannot alter core correlation fields needed for replay/audit (`F-117`, `F-170`, `NF-014`).

## 4) Extension points: plugins, custom nodes, custom runtimes

| Extension point | Contract boundary | Requirements |
| --- | --- | --- |
| Plugin metadata extensions | `ExtensionBlock` with strict namespace ownership | Must be namespaced and non-conflicting with core fields; unknown fields follow explicit unknown-event policy + audit marker (`F-020`, `F-164`) |
| Custom node ABI | `in(EventStream) -> out(EventStream)` with stable envelope/payload semantics | Node packaging/versioning independent of PipelineSpec; same Event ABI on in-process and external modes (`F-011`, `F-121`) |
| External node boundary | `ExternalNodeRuntime` over isolation transport (e.g., gRPC/WASM boundary) | Must preserve cancellation, timeout/resource limits, and observability correlation (`F-122`, `F-123`, `F-124`, `F-171`) |
| Custom runtime adapters | `RuntimeAdapter` normalize/encode boundary | Any transport/runtime host must produce contract-equivalent Event ABI (types, timestamps, ordering, orchestration metadata) (`F-086`, `F-083` to `F-085`) |
| Runtime capability plugin | `CapabilitySnapshot` attached at turn boundary | Capability changes apply at turn boundaries and are provenance-visible (`F-161`) |

## 5) Design invariants (must preserve)

1. Every inter-node event MUST carry the unified envelope contract; unknown-event handling MUST be explicit and auditable (`F-020`).
2. `schema_version` MUST be present and compatibility-checked in CI/publish paths (`F-021`, `F-028`, `F-032`).
3. Turn-scoped events MUST include `turn_id`; authority-sensitive events MUST carry current `authority_epoch` (`F-139`, `F-156`).
4. Exactly one terminal turn outcome (`commit` or `abort`) MUST occur, and `close` MUST follow for accepted turns (`F-139`, `NF-029`).
5. Pre-turn `admit`/`reject`/`defer` outcomes MUST NOT emit terminal turn events (`F-141`).
6. After `cancel(scope)` acceptance, new `output_accepted`/`playback_started` for that scope MUST be rejected (`F-175`).
7. ControlLane MUST preempt DataLane and TelemetryLane under pressure (`F-142`).
8. TelemetryLane MUST remain non-blocking and best-effort under overload (`F-147`, `NF-008`).
9. Buffer/flow-control actions (drop, merge, xoff/xon, credit) MUST be deterministic and replay-visible (`F-144`, `F-145`, `F-146`).
10. Low-latency profiles MUST avoid indefinite blocking by enforcing bounded block time with deterministic shedding (`F-148`).
11. Sequence/order lineage MUST remain stable enough for deterministic replay modes (`F-029`, `F-150`, `NF-009`).
12. Idempotency and dedupe semantics MUST hold across retry/reconnect/failover paths (`F-158`).
13. Replay-critical markers MUST be present for accepted turns even during recording fidelity downgrade (`F-149`, `F-153`, `F-176`, `NF-028`).
14. Encoding and compression negotiation MUST NOT change node-visible event semantics (`F-030`, `F-031`, `F-086`).
15. Extension namespaces MUST NOT override or weaken core envelope semantics (`F-164`).
16. Sensitive fields MUST be redacted per payload class policy and must never leak secrets into logs/events (`F-117`, `NF-011`, `NF-014`).

## 6) Tradeoffs and alternatives

| Tradeoff | Chosen direction | Alternative | Why chosen |
| --- | --- | --- | --- |
| Unified logical event contract vs split structs | Keep a unified logical contract, allow compatibility wrappers for current split types | Keep permanently separate `EventRecord` and `ControlSignal` models | Unified contract better satisfies cross-transport equivalence and replay tooling while allowing non-breaking migration (`F-020`, `F-086`, `F-029`) |
| Strict validation vs permissive pass-through | Strict validation on core fields/signals; explicit `unknown_event_action` for extensions | Best-effort pass-through for unknown fields/signals | Maintains correctness and reproducibility while still supporting controlled extensibility (`F-020`, `F-028`, `NF-013`) |
| Rich envelope metadata vs minimal envelope | Keep replay/authority/dedupe metadata in envelope | Minimal metadata to reduce payload size | Rich metadata is required for deterministic replay, failover safety, and auditability (`F-149`, `F-156`, `F-158`) |
| Plan-frozen semantics vs mid-turn adaptive mutation | Freeze contract-critical behavior per turn | Allow mid-turn policy/capability changes | Turn-frozen behavior is required for deterministic outcomes and replay stability (`F-149`, `F-150`, `F-161`) |
| Negotiated encoding/compression vs fixed single encoding | Negotiate at boundary; node ABI remains unchanged | Force one encoding/transport profile for all environments | Supports debug/prod diversity and transport portability without forking node contracts (`F-030`, `F-031`, `F-086`) |
| Strict cancel fencing vs tolerant late outputs | Enforce strict fence and reject post-cancel output-start signals | Permit late audio/output if generated before fence propagation | Prevents contradictory user-observable states and enforces authority/cancel correctness (`F-033`, `F-038`, `F-175`) |
| External node isolation vs max performance in-process | Default stable external-node boundary for untrusted/extensible workloads | Run all custom code in-process | Isolation improves trust and governance posture despite overhead; performance-sensitive paths can remain in-process (`F-122`, `F-124`, `F-171`) |

## 7) Implementation-Ready Execution Plan (file-level)

This section translates the target architecture into concrete implementation steps for `api/eventabi`.

### Phase A: Envelope and shape convergence (non-breaking first)

Files:
- `api/eventabi/types.go`
- `api/eventabi/types_test.go`

Changes:
- Add a first-class shared `EventEnvelope` type and keep current `EventRecord`/`ControlSignal` as compatibility wrappers during migration (`F-020`, `F-021`).
- Extend envelope fields with optional lineage/dedupe/provenance fields now missing from the package (`F-149`, `F-150`, `F-158`, `F-160`).
- Add explicit `EventKind` contract enum for `audio|text|control|metrics|debug|error` and wire validation boundaries (`F-022`).

Acceptance:
- Existing tests remain green.
- New tests verify old payloads still validate and new fields round-trip safely.

### Phase B: Validation parity hardening for control semantics

Files:
- `api/eventabi/types.go`
- `api/eventabi/types_test.go`

Changes:
- Enforce strict `turn_open_proposed` mapping (`event_scope=session`, `emitted_by=RK-02`) (`F-025`, `F-139`).
- Enforce strict emitter constraints for `barge_in|stop|cancel` and reason/emitter constraints for `budget_warning|budget_exhausted|degrade|fallback` (`F-033`, `F-035`, `F-144`).
- Enforce allowed `cancel(scope)` values (`session|turn|node|provider_invocation`) instead of non-empty string only (`F-034`, `F-175`).

Acceptance:
- Add negative tests for each invalid emitter/scope combination.
- Add positive tests for all valid combinations.

### Phase C: Compatibility and unknown-event policy contracts

Files:
- `api/eventabi/types.go`
- `api/eventabi/types_test.go`

Changes:
- Add `UnknownEventAction` contract type and decision path (`drop`, `pass_through`, optional strict reject policy) (`F-020`, `NF-013`).
- Add compatibility/deprecation contract structs used by tooling and CI checks (`CompatibilityReport`, deprecation metadata) (`F-028`, `F-162`, `F-163`, `NF-010`).
- Add lightweight version-skew representation for runtime/control-plane/schema compatibility checks at API surface (`F-162`, `NF-032`).

Acceptance:
- Table-driven tests for unknown-event policy decisions and compatibility-rule outcomes.

### Phase D: Transport/extensibility contracts and test completeness

Files:
- `api/eventabi/types.go`
- `api/eventabi/types_test.go`

Changes:
- Add encoding/compression negotiation types used by transport adapters (`F-030`, `F-031`, `F-086`).
- Add extension namespace governance contracts for plugin/custom-runtime fields (`F-121`, `F-164`).
- Expand tests to cover:
  - `EventRecord.Validate()` branch matrix
  - Full `ControlSignal.Validate()` signal-family matrix
  - Redaction and media-time edge cases
  - Envelope lineage/idempotency field validation (`F-028`, `NF-009`)

Acceptance:
- `go test ./api/eventabi` must include exhaustive positive/negative contract cases for all exported validation paths.

### Implementation readiness gate (definition of done for this package)

1. Package-level API contracts cover required MVP event ABI fields and policy hooks for unknown events and compatibility (`F-020`, `F-021`, `F-028`).
2. Control signal validation parity is strict for all currently declared signal families (`F-025`, `F-033`, `F-139`, `F-141`, `F-175`).
3. Test matrix includes both happy-path and negative-path coverage for every validation branch in `types.go` (`F-028`, `NF-013`).
4. `api/eventabi` contract changes stay schema-aligned with `docs/ContractArtifacts.schema.json` and remain replay/dedupe safe (`F-149`, `F-158`, `NF-009`).

## Change checklist

1. Keep `api/eventabi/types.go` and `docs/ContractArtifacts.schema.json` aligned with this target contract model.
2. Expand contract fixtures under `test/contract/fixtures/event` and `test/contract/fixtures/control_signal` as fields and invariants evolve.
3. Keep feature traceability current when adding or removing envelope fields or control signals.
4. Re-run:
   - `go test ./api/eventabi ./test/contract`
   - `make verify-quick`

## Related docs

- `docs/PRD.md`
- `docs/RSPP_features_framework.json`
- `docs/rspp_SystemDesign.md`
- `docs/ContractArtifacts.schema.json`
- `docs/SecurityDataHandlingBaseline.md`
