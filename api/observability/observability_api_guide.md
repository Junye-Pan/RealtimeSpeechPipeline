# Observability API Guide

This guide is the implementation-ready contract reference for `api/observability`, with current codebase progress and divergence explicitly mapped to target architecture requirements from `docs/PRD.md`, `docs/RSPP_features_framework.json`, and `docs/rspp_SystemDesign.md`.

Code audit timestamp: `2026-02-16`.
Validation checkpoint: `go test ./api/observability ./internal/observability/replay` passed on `2026-02-16`.

## Ownership

- Primary: `ObsReplay-Team`
- Cross-review for `api/` contract changes: `Runtime-Team`, `ControlPlane-Team`
- Security-sensitive review: `Security-Team`

## Evidence Anchors

- PRD: `4.1.4`, `4.1.6`, `4.2.2`, `4.2.3`, `4.3`, `5.1`, `5.2`.
- System design: section `3.2` and section `4` (`internal/observability/{telemetry,timeline,replay}`).
- Feature IDs used in this guide: `F-026`, `F-067`-`F-077`, `F-121`-`F-124`, `F-127`, `F-138`-`F-142`, `F-149`-`F-154`, `F-156`, `F-160`, `F-164`, `F-175`, `F-176`, `NF-002`, `NF-008`, `NF-009`, `NF-024`-`NF-029`, `NF-038`.

## 0) Current Codebase Progress (`api/observability`)

| API surface | Status | Code evidence | Test evidence | Feature alignment |
| --- | --- | --- | --- | --- |
| Divergence classes + entry shape (`DivergenceClass`, `ReplayDivergence`) | Implemented | `api/observability/types.go` | exercised via comparator tests in `internal/observability/replay` | `F-077`, `NF-009` |
| Replay fidelity enum (`L0/L1/L2`) | Implemented | `api/observability/types.go` | `api/observability/types_test.go` | `F-153` |
| Replay access request validation (`ReplayAccessRequest.Validate`) | Implemented | `api/observability/types.go` | `api/observability/types_test.go`, `internal/observability/replay/access_test.go` | `F-073`, `F-153` |
| Replay access decision + redaction marker schema | Partial | structs exist in `api/observability/types.go`; no standalone `Validate()` on decision/marker | validated indirectly via `ReplayAuditEvent.Validate` | `F-073` |
| Immutable replay audit event schema + validation | Implemented | `api/observability/types.go` | `api/observability/types_test.go`, `internal/observability/replay/service_test.go` | `F-073` |
| Replay mode contract types at observability API layer | Missing | no `ReplayMode` type in `api/observability` | n/a | `F-151` |
| Replay cursor contract type at observability API layer | Missing | no `ReplayCursor` type in `api/observability` | n/a | `F-152` |
| OR-02 baseline manifest contract in `api/observability` | Missing (owned internally today) | OR-02 evidence types live in `internal/observability/timeline/recorder.go` | timeline tests under `internal/observability/timeline` | `F-149`, `F-150`, `F-153`, `NF-028` |
| API-level test coverage breadth | Partial | only `types_test.go` in `api/observability` | 2 tests currently | `NF-009`, `NF-028` |

## 1) Divergence Matrix (Current vs Target)

| Target capability | Current state | Divergence | Implementation action | Feature IDs |
| --- | --- | --- | --- | --- |
| Explicit replay mode contract in observability API | mode strings currently validated in `api/controlplane.RecordingPolicy` | Observability API cannot independently type-check replay mode values | add `ReplayMode` enum + validation in `api/observability` and map from control-plane strings | `F-151`, `F-153` |
| Deterministic replay cursor API | not present in `api/observability` | no shared cursor type for replay service/tooling contracts | add `ReplayCursor` type with deterministic stepping fields + `Validate` | `F-152`, `NF-009` |
| API contract for replay request/result/report payloads | comparator exists internally, no shared request/result/report types | toolchain integrations depend on internal package structs | add shared replay request/result/report schema in `api/observability` | `F-072`, `F-077`, `F-127` |
| API-level OR-02 evidence contract | evidence schema exists only internally (`timeline`) | API boundary does not expose OR-02 manifest types for contract-level conformance | promote minimal OR-02 manifest reference types to `api/observability` (without moving heavy payload internals yet) | `F-149`, `F-150`, `F-153`, `NF-028` |
| Decision-level validation symmetry | decision markers validated only when wrapped in `ReplayAuditEvent` | call sites can create invalid decisions before audit-stage validation | add `ReplayAccessDecision.Validate()` + marker validation helper | `F-073` |
| API test coverage for negative-path and compatibility invariants | minimal package-local tests | weak regression signal for API-only contract drift | expand `api/observability/types_test.go` table-driven negative-path coverage | `NF-009`, `NF-028` |

## 2) Target Architecture and Module Breakdown (With Status)

| Module | Path | Status | Responsibility | Evidence |
| --- | --- | --- | --- | --- |
| `OBS-API` | `api/observability` | Partial | Shared replay contract types (divergence/fidelity/access/audit), plus missing replay mode/cursor/report contracts. | `F-073`, `F-077`, `F-151`, `F-152`, `F-153` |
| `OBS-TELEMETRY` | `internal/observability/telemetry` | Implemented (core) | Non-blocking telemetry pipeline, sink/export boundaries, overload-safe behavior. | `F-026`, `F-069`, `F-070`, `F-142`, `NF-002` |
| `OBS-TIMELINE` | `internal/observability/timeline` | Partial | OR-02 baseline/detail recording, redaction, artifact write/read; durable export path still pending. | `F-071`, `F-149`, `F-150`, `F-153`, `F-154`, `F-176`, `NF-028` |
| `OBS-REPLAY` | `internal/observability/replay` | Partial | access control, audit sinks, retention controls, divergence comparator; replay execution engine/cursor still pending. | `F-072`, `F-073`, `F-077`, `F-116`, `F-151`, `F-152`, `NF-009` |
| `OBS-EXTENSIONS` | `internal/observability/extensions` (target) | Planned | plugin registry and conformance boundaries for exporters/comparators/policies. | `F-121`-`F-124`, `F-164` |

## 3) Key Interfaces

### 3.1 Implemented API Contracts (Now)

- `ReplayAccessRequest.Validate()` enforces deny-by-default request prerequisites and replay fidelity validity (`F-073`, `F-153`).
- `ReplayAuditEvent.Validate()` enforces timestamp/request/marker/deny-reason constraints (`F-073`).
- `ReplayDivergence` and `DivergenceClass` provide shared divergence taxonomy consumed by replay comparator logic (`F-077`, `NF-009`).
- `ReplayFidelity` defines `L0/L1/L2` API-level fidelity identifiers (`F-153`).

### 3.2 Target Contracts to Add (Implementation Backlog)

```go
package observability

// F-151
type ReplayMode string
const (
    ReplayModeReSimulateNodes               ReplayMode = "re_simulate_nodes"
    ReplayModePlaybackRecordedProvider      ReplayMode = "playback_recorded_provider_outputs"
    ReplayModeReplayDecisions               ReplayMode = "replay_decisions"
    ReplayModeRecomputeDecisions            ReplayMode = "recompute_decisions"
)

// F-152
type ReplayCursor struct {
    SessionID         string
    TurnID            string
    Lane              string
    RuntimeSequence   int64
    EventID           string
}

// F-072, F-077
type ReplayRunRequest struct {
    BaselineRef       string
    CandidatePlanRef  string
    Mode              ReplayMode
    Cursor            *ReplayCursor
}

type ReplayRunResult struct {
    RunID             string
    Divergences       []ReplayDivergence
    Cursor            *ReplayCursor
}
```

## 4) Data Model: Core Entities and Relations

### 4.1 Current entities (implemented)

- API entities: `ReplayAccessRequest`, `ReplayAccessDecision`, `ReplayRedactionMarker`, `ReplayAuditEvent`, `ReplayDivergence`, `ReplayFidelity` (`F-073`, `F-077`, `F-153`).
- Internal evidence entities: `BaselineEvidence`, `ProviderAttemptEvidence`, `InvocationOutcomeEvidence`, `HandoffEdgeEvidence`, `InvocationSnapshotEvidence`, `DetailEvent` (`F-071`, `F-149`, `F-150`, `F-160`, `F-176`).
- Replay retention entities: `ReplayArtifactRecord`, `RetentionPolicy`, `DeletionRequest`, `DeletionResult` (`F-116`).

### 4.2 Required relations (must hold)

- `Session 1 -> N Turn` lifecycle (`F-138`, `F-139`).
- `Accepted Turn 1 -> 1 complete OR-02 baseline evidence record` (`F-153`, `NF-028`).
- `Turn 1 -> N ProviderAttemptEvidence -> 1..N InvocationOutcomeEvidence` summaries (`F-160`).
- `ReplayAccessRequest 1 -> 1 ReplayAccessDecision -> 1 ReplayAuditEvent` fail-closed authorization chain (`F-073`).
- `Replay baseline + candidate -> N ReplayDivergence` deterministic diff contract (`F-077`, `NF-009`).

### 4.3 Missing API-layer entities (tracked divergence)

- `ReplayMode` (currently only as strings in control-plane recording policy) (`F-151`).
- `ReplayCursor` (`F-152`).
- Shared replay run request/result/report types (`F-072`, `F-077`).
- API-level OR-02 manifest reference contract (`F-149`, `F-150`, `NF-028`).

## 5) Extension Points

### 5.1 Existing extension boundaries

- Telemetry sink extension via `internal/observability/telemetry.Sink` (`F-070`, `NF-002`).
- Audit backend extension via `internal/observability/replay.ImmutableReplayAuditBackendResolver` (`F-073`).
- Retention policy extension via `internal/observability/replay.RetentionPolicyResolver` (`F-116`).

### 5.2 Planned extension boundaries

- API-level replay comparator/report plugin contracts (`F-077`, `F-164`).
- API-level replay engine/cursor interoperability contracts for external tooling (`F-072`, `F-151`, `F-152`).
- External/custom node observability conformance hooks (`F-121`-`F-124`, `F-164`).

## 6) Design Invariants (Must Preserve)

1. Telemetry must remain non-blocking and best-effort under load (`F-142`, `NF-002`, PRD `4.2.3`).
2. Correlation identifiers must remain consistent across metrics/logs/traces/timeline artifacts (`F-067`, `NF-008`).
3. Every accepted turn must remain OR-02 complete at baseline fidelity (`F-153`, `NF-028`).
4. Exactly one terminal outcome per accepted turn, followed by close (`F-139`, `NF-029`).
5. Pre-turn `admit/reject/defer` outcomes must not emit terminal turn events (`F-141`).
6. Turn execution evidence must remain tied to immutable turn plan provenance (`F-149`).
7. Determinism markers (seed, merge rule, ordering, nondeterministic refs) must be replay-visible (`F-150`).
8. Replay mode must be explicit in replay contracts and outputs (`F-151`).
9. Replay cursor stepping/resume semantics must be deterministic (`F-152`, `NF-009`).
10. Timeline recording must keep local append decoupled from durable export (`F-154`).
11. Deterministic recording downgrade must preserve replay-critical control evidence (`F-176`).
12. Lease/epoch safety must reject stale-authority artifacts (`F-156`, `NF-027`).
13. Post-cancel output fencing is strict; canceled scopes cannot re-emit accepted playback/output start signals (`F-175`, `NF-026`).
14. Replay access remains deny-by-default, tenant-scoped, and auditable (`F-073`).
15. Retention/deletion must remain tenant-scoped and payload-class aware (`F-116`).

## 7) Implementation-Ready Plan (Phased)

### P0: API Contract Hardening (low risk, immediate)

| Work item | Files | Required tests | Done criteria | Feature IDs |
| --- | --- | --- | --- | --- |
| Add `ReplayAccessDecision.Validate()` and marker helper validation | `api/observability/types.go` | extend `api/observability/types_test.go` for allow/deny + invalid marker cases | invalid decisions fail before audit wrapping | `F-073` |
| Add `ReplayDivergence.Validate()` (class, scope, message, diff constraints) | `api/observability/types.go` | new table-driven tests in `api/observability/types_test.go` | comparator outputs can be schema-validated before report write | `F-077`, `NF-009` |
| Expand negative-path tests for existing request/audit validation | `api/observability/types_test.go` | missing required fields, bad payload class/action, missing deny reason | package has strong API-only regression protection | `NF-009` |

### P1: Missing Replay API Contracts

| Work item | Files | Required tests | Done criteria | Feature IDs |
| --- | --- | --- | --- | --- |
| Add `ReplayMode` typed contract aligned with control-plane mode strings | `api/observability/types.go`, `api/controlplane/types.go` (mapping/compat) | API + control-plane compatibility tests | replay mode no longer stringly-typed at integration boundary | `F-151`, `F-153` |
| Add `ReplayCursor` contract with deterministic validation | `api/observability/types.go` | cursor validation tests + replay comparator integration tests | cursor values are portable and validated in tooling/runtime | `F-152`, `NF-009` |
| Add shared replay request/result/report types | `api/observability/types.go` | replay service/comparator tests updated | CI tooling can consume stable API schemas | `F-072`, `F-077`, `F-127` |

### P1: OR-02 API Boundary Completion

| Work item | Files | Required tests | Done criteria | Feature IDs |
| --- | --- | --- | --- | --- |
| Promote minimal OR-02 manifest reference contract into `api/observability` | `api/observability/types.go`, plus adapters in `internal/observability/timeline` | manifest schema/compat tests + baseline completeness tests | OR-02 gate semantics are enforceable from API package level | `F-149`, `F-150`, `F-153`, `NF-028` |

### P2: Extension/Conformance Surface

| Work item | Files | Required tests | Done criteria | Feature IDs |
| --- | --- | --- | --- | --- |
| Define observability extension interfaces and conformance fixture schemas | `api/observability/*` + `internal/observability/extensions` | conformance tests against sample plugin implementations | extensions cannot bypass replay/authorization invariants | `F-121`-`F-124`, `F-164` |

## 8) Tradeoffs

| Tradeoff | Chosen approach | Alternative | Why chosen |
| --- | --- | --- | --- |
| Fast path latency vs telemetry durability | non-blocking telemetry with bounded drops | synchronous reliable telemetry writes | protects control/cancel latency and lane priority (`F-142`, `NF-002`) |
| API minimalism vs full replay contract in one package | stage in replay mode/cursor/report types incrementally | add all replay contracts at once | reduces migration risk while closing highest-value gaps first (`F-151`, `F-152`, `F-077`) |
| Internal OR-02 ownership vs API promotion | keep heavy evidence payload internal, promote minimal manifest refs first | move full timeline schemas into `api/observability` now | keeps API lean while enabling contract-level gating (`F-149`, `NF-028`) |
| Strict deny-by-default vs easier operator access | deny-by-default with immutable audit | default-allow debug access | preserves tenancy/security posture (`F-073`) |
| Deterministic downgrade vs heuristic adaptation | deterministic, replay-visible recording downgrade | adaptive heuristic without strict transitions | preserves replay explainability and gate reproducibility (`F-176`, `NF-009`) |

## 9) Verification Checklist

Required checks after API contract edits:

1. `go test ./api/observability ./internal/observability/replay`
2. `make verify-quick`
3. `make security-baseline-check`

Gate alignment checkpoints:

1. Replay baseline completeness remains 100% for accepted turns (`NF-028`).
2. Terminal lifecycle correctness remains 100% (`NF-029`).
3. Cancel-fence latency gate integrity remains unaffected by observability changes (`NF-026`).

## Related Docs

- `docs/PRD.md`
- `docs/RSPP_features_framework.json`
- `docs/rspp_SystemDesign.md`
- `internal/observability/observability_module_guide.md`
- `api/controlplane/controlplane_api_guide.md`
- `docs/CIValidationGates.md`
