# Public Package Contract Scaffold Guide

This guide proposes the target architecture for public contract scaffolds under `pkg/`, derived from:
- `docs/PRD.md`
- `docs/RSPP_features_framework.json`
- `docs/rspp_SystemDesign.md`

Current status:
- `pkg/contracts` remains scaffold-only.
- Canonical implemented contract sources remain `api/controlplane`, `api/eventabi`, and `api/observability`.
- This document defines the promotion target so public `pkg/*` contracts are import-safe and semantically stable.

## Evidence Basis

Primary PRD semantic anchors:
- stable contract primitives and runtime guarantees (PRD section 4.1)
- lane model and runtime rules (PRD sections 4.2 and 4.3)
- MVP quality gates and phase boundaries (PRD section 5)

Primary feature-catalog anchors:
- graph/spec contracts (`F-001` to `F-019`)
- Event ABI (`F-020` to `F-032`)
- cancellation/runtime semantics/buffering (`F-033` to `F-054`, `F-138` to `F-148`, `F-175`)
- providers/control plane/transport/state/security (`F-055` to `F-066`, `F-083` to `F-124`, `F-155` to `F-160`)
- observability/determinism/tooling/governance (`F-067` to `F-082`, `F-125` to `F-154`, `F-161` to `F-165`, `F-176`, `NF-010`, `NF-024` to `NF-029`, `NF-032`, `NF-038`)

System-design anchors:
- contract surfaces and module split (`docs/rspp_SystemDesign.md`, sections 3 and 4)
- MVP vs post-MVP boundaries (`docs/rspp_SystemDesign.md`, section 5)

## Current Progress and Divergence (pkg-scoped, 2026-02-16)

Observed codebase state in `pkg/`:
- `pkg/` currently contains only two markdown files: `pkg/public_package_scaffold_guide.md` and `pkg/contracts/contracts_package_guide.md`.
- There are no Go source files under `pkg/` (no public importable contract package yet).
- `pkg/contracts/v1/*` target module folders do not exist yet.

Observed implemented contract surfaces in `api/*` (promotion candidates):
- Event ABI contracts exist as `EventRecord` and `ControlSignal` in `api/eventabi/types.go` (`F-020` to `F-026`, `F-028`).
- Control-plane contracts exist as `DecisionOutcome` and `ResolvedTurnPlan` in `api/controlplane/types.go` (`F-092`, `F-098`, `F-141`, `F-149`, `F-159`).
- `ResolvedTurnPlan` already includes buffer/flow/determinism/recording and optional streaming-handoff policy fields in `api/controlplane/types.go` (`F-143`, `F-145`, `F-149`, `F-150`, `F-153`, `F-176`).
- Observability contracts exist as `ReplayDivergence`, `ReplayAccessRequest`, and `ReplayAuditEvent` in `api/observability/types.go` (`F-067`, `F-071`, `F-072`, `F-077`, `NF-028`).

Divergence summary:
- Target public module architecture (`pkg/contracts/v1/*`) is designed but not implemented yet.
- Target universal Event contract surface is not yet represented in code; current implementation uses split artifacts (`EventRecord`, `ControlSignal`) (`F-020` to `F-025`).
- Public-package contract tests under `pkg/` are not present; validation currently runs through `api/*` and contract/test suites.

Divergence-to-action matrix:

| Area | Current codebase reality | Divergence from target | Implementation action |
| --- | --- | --- | --- |
| Public package root | No `pkg/contracts/v1` Go package exists | Import path not available to external consumers | Create `pkg/contracts/v1` scaffold packages with `doc.go` + `types.go` stubs (`NF-010`) |
| Event ABI public surface | `api/eventabi` only; split `EventRecord` and `ControlSignal` | Target public package absent; universal-event target not modeled | Phase 1 promote split ABI first, then add universal facade adapter without breaking v1 (`F-020` to `F-025`, `F-028`) |
| Control-plane public surface | `api/controlplane` only | No public `ResolvedTurnPlan` import path | Promote `DecisionOutcome`, `ResolvedTurnPlan`, and dependent structs into `pkg/contracts/v1/controlplane` (`F-149`, `F-159`) |
| Observability public surface | `api/observability` only | No public replay contract import path | Promote replay divergence/access/audit contracts into `pkg/contracts/v1/observability` (`F-071`, `F-072`, `F-077`, `NF-028`) |
| Verification at public boundary | No `pkg` tests | API behavior could drift when promoted | Add package-compat, JSON-shape, and validation parity tests for `pkg/contracts/v1/*` (`NF-010`, `NF-032`) |

## 1) Target Module List and Responsibilities

Proposed public scaffold root:
- `pkg/contracts/v1/*` (versioned package root; additive-only inside a major line)

| Module | Responsibility | Canonical source-of-truth today | Feature trace |
| --- | --- | --- | --- |
| `pkg/contracts/v1/core` | IDs, versions, lane/scope enums, shared error/outcome codes, and correlation primitives (`F-013`, `F-021`, `F-158`, `NF-010`) | `api/eventabi`, `api/controlplane` | `F-013`, `F-021`, `F-138`, `F-155`, `F-158`, `NF-010` |
| `pkg/contracts/v1/spec` | `PipelineSpec`, `GraphDefinition`, node/edge typing, subgraph composition, and fallback graph intent (`F-001` to `F-007`, `F-019`) | `api/controlplane` + schema/tooling | `F-001` to `F-019`, `F-092`, `F-100` |
| `pkg/contracts/v1/eventabi` | Unified event envelope, lane taxonomy, control/data/telemetry event union, and schema-version compatibility metadata (`F-020` to `F-032`) | `api/eventabi` | `F-020` to `F-032`, `F-142`, `F-147` |
| `pkg/contracts/v1/runtime` | Turn/session lifecycle contracts, arbitration outcomes, cancellation semantics, and budget/flow contracts (`F-033` to `F-054`, `F-138` to `F-148`, `F-175`) | `api/controlplane`, `api/eventabi` | `F-033` to `F-054`, `F-138` to `F-148`, `F-175`, `NF-029` |
| `pkg/contracts/v1/controlplane` | Resolved-turn-plan artifact, snapshot provenance refs, admission/authority decision contracts, and rollout/routing decisions (`F-092` to `F-102`, `F-149`, `F-159`) | `api/controlplane` | `F-092` to `F-102`, `F-149`, `F-159`, `NF-027` |
| `pkg/contracts/v1/provider` | Provider capability contract, invocation envelope, normalized outcomes, and retry/switch policy result surface (`F-055` to `F-066`, `F-160`) | `api/controlplane`, provider adapter surfaces | `F-055` to `F-066`, `F-160` |
| `pkg/contracts/v1/transport` | Transport boundary normalization contract, session route/token envelope, and connection-state control signals (`F-083`, `F-086` to `F-091`) | `api/eventabi`, control-plane route metadata types | `F-083`, `F-086` to `F-091` |
| `pkg/contracts/v1/observability` | OR-02 evidence contract, replay fidelity/replay mode/replay cursor metadata, divergence taxonomy, and replay access/audit (`F-067` to `F-077`, `F-149` to `F-154`, `F-176`, `NF-028`) | `api/observability`, `api/controlplane` | `F-067` to `F-077`, `F-149` to `F-154`, `F-176`, `NF-028` |
| `pkg/contracts/v1/security` | Tenant/auth context envelope, redaction markers, retention policy refs, and token/mTLS config contracts (`F-114` to `F-120`, `F-073`) | `api/observability`, event payload metadata | `F-114` to `F-120`, `F-073`, `NF-027` |
| `pkg/contracts/v1/extensibility` | Custom node manifests, external-node boundary contracts, resource/time limits, and plugin compatibility markers (`F-011`, `F-121` to `F-124`, `F-128`) | future promotion from runtime/provider boundaries | `F-011`, `F-121` to `F-124`, `F-128`, `F-164` |
| `pkg/contracts/v1/compat` | Compatibility policy primitives: schema policy, skew window, deprecation metadata, and conformance profile IDs (`F-161` to `F-165`, `NF-010`, `NF-032`) | cross-cutting governance contracts | `F-161` to `F-165`, `NF-010`, `NF-032` |
| `pkg/contracts/v1/tooling` | Lint/validation and generated-doc metadata contracts consumed by CLI/CI/replay tooling (`F-125` to `F-130`) | tooling schemas | `F-125` to `F-130`, `F-126`, `F-127`, `F-129` |

Phase boundary notes:
- MVP-first public scaffolds should prioritize `core/spec/eventabi/runtime/controlplane/observability` (`F-001` to `F-032`, `F-033` to `F-054`, `F-067` to `F-082`, `F-138` to `F-154`, `NF-024` to `NF-029`).
- Post-MVP expansions are expected mainly in `transport` (`F-084`, `F-085`), `compat` (`F-161` to `F-165`), and multi-region policy payloads (`F-110` to `F-113`).

## 2) Key Interfaces (Pseudocode)

Interface-to-feature mapping (inline anchors):
- `Event` and `EventEnvelope` exist to satisfy unified envelope/lane/schema guarantees (`F-020`, `F-021`, `F-022`, `F-025`, `F-028`).
- `NodeContract` and `NodeExecutor` represent stable custom-node/lifecycle contracts (`F-011`, `F-012`, `F-013`).
- `TurnPlanResolver` and `AdmissionOutcome` enforce turn-frozen pre-turn decisions (`F-092`, `F-098`, `F-141`, `F-149`).
- `ProviderAdapter` captures normalized provider contract/cancel behavior (`F-055`, `F-059`, `F-063`, `F-064`, `F-160`).
- `TransportAdapter` preserves shared session/audio/control transport contract (`F-083`, `F-086`, `F-089`, `F-091`).
- `ReplayRecorder`/`DivergenceClassifier` align with timeline/replay and deterministic downgrade behavior (`F-071`, `F-072`, `F-077`, `F-154`, `F-176`, `NF-028`).

```go
// pkg/contracts/v1/eventabi
type Event interface {
    Envelope() EventEnvelope
    Lane() Lane
    Validate() error
}

type EventEnvelope struct {
    SchemaVersion      string
    SessionID          string
    TurnID             *string
    PipelineVersion    string
    EventID            string
    CausalParentID     *string
    IdempotencyKey     *string
    AuthorityEpoch     *uint64
    RuntimeSequence    uint64
    TransportSequence  *uint64
    RuntimeMonotonicNs int64
    WallClockUnixNs    int64
}
```

```go
// pkg/contracts/v1/spec
type NodeContract interface {
    Descriptor() NodeDescriptor
    Init(ctx SessionContext) error
    Start(ctx NodeExecutionContext) error
    Stop(ctx SessionContext) error
}

type NodeExecutor interface {
    Process(ctx NodeExecutionContext, in Event) ([]Event, error)
}
```

```go
// pkg/contracts/v1/controlplane
type TurnPlanResolver interface {
    ResolveTurnPlan(req TurnOpenRequest, snap TurnStartSnapshots) (ResolvedTurnPlan, error)
}

type AdmissionOutcome interface {
    Kind() string // admit | reject | defer
    Scope() string // pre_turn | active_turn
}
```

```go
// pkg/contracts/v1/provider
type ProviderAdapter interface {
    Capabilities() ProviderCapabilities
    Invoke(ctx ProviderInvocationContext, req ProviderRequest) (ProviderStream, error)
    Cancel(ctx context.Context, invocationID string) error
}
```

```go
// pkg/contracts/v1/transport
type TransportAdapter interface {
    OpenSession(ctx context.Context, route SessionRoute) (TransportSession, error)
    Ingress(msg TransportIngress) ([]Event, error)
    Egress(evt Event) ([]TransportEgress, error)
    CloseSession(ctx context.Context, sessionID string, reason string) error
}
```

```go
// pkg/contracts/v1/observability
type ReplayRecorder interface {
    AppendLocal(evt Event) error // non-blocking path
    ExportAsync(cursor ReplayCursor) error
}

type DivergenceClassifier interface {
    Classify(base ReplayArtifact, candidate ReplayArtifact) ([]ReplayDivergence, error)
}
```

## 3) Data Model: Core Entities and Relations

Core entities:
- `PipelineSpec` (versioned immutable graph contract, `F-001`, `F-015`, `F-016`)
- `NodeSpec` and `EdgeSpec` (typed execution topology, `F-003`, `F-004`, `F-005`, `F-018`)
- `ExecutionProfile` (defaulting and latency posture, `simple/v1` baseline, `F-078`, `F-079`, `F-080`)
- `ResolvedTurnPlan` (turn-frozen executable contract, `F-149`, `F-150`)
- `Session` and `Turn` (runtime identity scopes and lifecycle, `F-138`, `F-139`, `F-141`)
- `EventRecord`/`ControlSignal` (Event ABI units, `F-020`, `F-021`, `F-025`)
- `ProviderInvocation` (normalized provider call unit, `F-160`)
- `Or02Evidence` and `ReplayArtifact` (replay-critical observability contracts, `F-071`, `F-153`, `F-154`, `F-176`, `NF-028`)
- `PlacementLease` and snapshot references (authority/provenance, `F-095`, `F-156`, `F-159`)

Primary relations:
- `PipelineSpec (1) -> (N) NodeSpec` (`F-003`, `F-019`)
- `PipelineSpec (1) -> (N) EdgeSpec` (`F-004`, `F-005`, `F-006`, `F-018`)
- `ExecutionProfile (1) -> (N) PipelineSpec defaults` (`F-078`, `F-080`, `F-081`)
- `Session (1) -> (N) Turn` (`F-138`, `F-139`)
- `Turn (1) -> (1) ResolvedTurnPlan` (`F-149`)
- `Turn (1) -> (N) EventRecord/ControlSignal` (`F-020` to `F-026`)
- `Turn (1) -> (N) ProviderInvocation` (`F-160`)
- `ResolvedTurnPlan (1) -> (N) SnapshotRef` (`F-149`, `F-159`)
- `Turn (1) -> (1) Or02Evidence` (minimum mandatory manifest, `NF-028`)
- `ReplayArtifact (1) -> (N) ReplayDivergence` (`F-072`, `F-077`)

## 4) Extension Points

Plugin and extension targets:
- Provider plugins:
  - Implement `ProviderAdapter` with stable capability and outcome normalization (`F-055`, `F-065`, `F-160`).
  - Must preserve retry/switch policy and deterministic fallback semantics (`F-057`, `F-059`, `F-066`).
- Custom in-process nodes:
  - Implement `NodeContract` and `NodeExecutor` (`F-011`).
  - Must honor lifecycle hooks, cancellation responsiveness, and context propagation (`F-012`, `F-013`, `F-036`, `F-041`).
- External/out-of-process nodes:
  - Must implement external boundary contract (timeouts, limits, cancellation, observability injection) (`F-121` to `F-124`).
- Custom runtime kernels:
  - Allowed if they consume `ResolvedTurnPlan` and emit contract-valid lane events (`F-149`, `F-142`, `F-147`).
  - Must preserve lifecycle, lane priority, output fencing, and OR-02 evidence obligations (`F-138` to `F-141`, `F-175`, `NF-028`, `NF-029`).
- Transport adapters:
  - Implement shared transport contract with normalized session/audio/control semantics (`F-086`, `F-089`, `F-090`).
  - MVP implementation path is LiveKit (`F-083`), with websocket/telephony extension later (`F-084`, `F-085`).

## 5) Design Invariants (Must Preserve)

1. A turn plan is immutable for the life of the turn; policy/snapshot updates apply only at boundaries. (`F-149`, `F-150`, PRD 4.3)
2. Accepted turns follow `OPEN -> ACTIVE -> (COMMIT|ABORT) -> CLOSE` with exactly one terminal outcome. (`F-139`, `NF-029`)
3. `admit/reject/defer` are pre-turn outcomes and cannot emit turn terminal events. (`F-141`)
4. Lane priority is always `ControlLane > DataLane > TelemetryLane` under pressure. (`F-142`, PRD 4.2)
5. Every edge is bounded and has explicit/defaulted buffer + watermark behavior. (`F-043`, `F-143`, `F-144`)
6. Low-latency profiles cannot block indefinitely; deterministic shedding is required after bound expiry. (`F-148`)
7. Cancellation must propagate across graph and provider boundaries, with measured latency. (`F-033`, `F-034`, `F-040`, `F-046`)
8. After cancel acceptance, runtime and transport egress are fenced for that scope. (`F-175`, PRD 4.2.2)
9. Authority epoch is enforced on ingress and egress; stale outputs are rejected. (`F-095`, `F-156`, `NF-027`)
10. PipelineSpec versions are immutable once published; multiple versions can run concurrently per session pinning. (`F-015`, `F-016`)
11. Event schema versioning and compatibility checks are mandatory at publish/CI boundaries. (`F-028`, `NF-010`, `NF-032`)
12. Correlation identity (`session_id`, `turn_id` where required) must be preserved across events/logs/traces. (`F-013`, `F-067`)
13. OR-02 replay-critical evidence is required for every accepted turn. (`NF-028`, PRD 4.1.6)
14. Timeline recording cannot block control progression; local append and durable export remain decoupled. (`F-154`)
15. Recording-fidelity downgrades must be deterministic and preserve replay-critical control evidence. (`F-176`, `F-153`)
16. Telemetry is best-effort and must not block control or degrade data-lane latency behavior. (`F-147`, PRD 4.2.3)
17. Tenant/security isolation and secret redaction are contract-level requirements, not implementation options. (`F-114`, `F-117`, `F-119`)
18. Public contract packages evolve additively within a major version and require explicit compatibility metadata for any non-additive change. (`NF-010`, `F-163`)

## 6) Major Tradeoffs and Alternatives

1. Monolithic public contract package vs domain-split modules (`F-164`).
   - Chosen: domain-split modules under `pkg/contracts/v1/*`.
   - Why: isolates compatibility risk by area and matches system-design ownership boundaries.
   - Alternative: single package with all types.
   - Cost: more packages and import surfaces.
   - Trace: `docs/rspp_SystemDesign.md` section 4, `F-164`.

2. Strict typed contracts vs permissive map-based payloads (`F-028`, `F-149`, `F-150`, `NF-028`).
   - Chosen: strict typed contracts for replay-critical and control-plane artifacts.
   - Why: determinism/replay and CI compatibility checks need unambiguous schemas.
   - Alternative: open-ended map payloads for flexibility.
   - Cost: slower schema evolution and explicit migration work.
   - Trace: `F-028`, `F-149`, `F-150`, `NF-028`.

3. Turn-frozen plan vs mid-turn dynamic policy mutation (`F-149`, `F-140`).
   - Chosen: turn-frozen `ResolvedTurnPlan`.
   - Why: deterministic runtime behavior and reproducible replay.
   - Alternative: dynamic policy updates during active turns.
   - Cost: less reactive in-turn adaptation.
   - Trace: PRD 4.1.1/4.3, `F-149`, `F-140`.

4. Universal Event contract target vs split current artifacts (`EventRecord` and `ControlSignal`) (`F-020` to `F-025`).
   - Chosen: scaffold a universal public Event contract while supporting compatibility shims to current split model.
   - Why: PRD contract is universal-event centric, but current code has split structs.
   - Alternative: preserve split model as the permanent public API.
   - Cost: short-term adapter complexity.
   - Trace: PRD 4.1.3, `F-020` to `F-025`.

5. Minimal L0-only public replay surface vs exposing richer L1/L2 constructs now (`NF-028`, `F-153`, `F-176`).
   - Chosen: require L0 OR-02 core in MVP public scaffolds, keep L1/L2 additive.
   - Why: MVP gate compliance with bounded complexity.
   - Alternative: expose full L2-first contracts immediately.
   - Cost: additional future public expansion work.
   - Trace: `NF-028`, `F-153`, `F-176`, PRD 5.2.

6. In-process custom nodes only vs first-class external-node boundary (`F-121` to `F-124`).
   - Chosen: include external-node boundary contracts from the start in public scaffolds.
   - Why: extensibility and trust-boundary requirements are explicit framework goals.
   - Alternative: postpone external-node contract until later.
   - Cost: larger initial surface area and compliance checks.
   - Trace: `F-121` to `F-124`, PRD 4.1.7.

7. MVP-only transport contract (LiveKit) vs transport-agnostic public contract now (`F-083`, `F-084`, `F-085`, `F-086`).
   - Chosen: transport-agnostic contract with MVP implementation notes.
   - Why: preserves core framework positioning while respecting MVP reality.
   - Alternative: hardcode LiveKit semantics into public contracts.
   - Cost: some contract fields remain unused in MVP.
   - Trace: PRD 4.4.5/5.2, `F-083`, `F-086`, post-MVP `F-084`, `F-085`.

## Implementation-Ready Rollout Plan

Phase 0: Public scaffold bootstrap (no semantic move)
- Create package directories:
  - `pkg/contracts/v1/core`
  - `pkg/contracts/v1/spec`
  - `pkg/contracts/v1/eventabi`
  - `pkg/contracts/v1/runtime`
  - `pkg/contracts/v1/controlplane`
  - `pkg/contracts/v1/provider`
  - `pkg/contracts/v1/transport`
  - `pkg/contracts/v1/observability`
  - `pkg/contracts/v1/security`
  - `pkg/contracts/v1/extensibility`
  - `pkg/contracts/v1/compat`
  - `pkg/contracts/v1/tooling`
- Add `doc.go` for each package with ownership, compatibility policy, and PRD feature references (`NF-010`, `F-163`).
- Exit criteria:
  - `go test ./...` passes with no behavioral changes.

Phase 1: MVP public contract promotion (facade-first)
- Promote currently implemented MVP-critical types first:
  - from `api/eventabi`: `Lane`, `EventScope`, `PayloadClass`, `RedactionAction`, `EventRecord`, `ControlSignal` (`F-020` to `F-026`, `F-028`)
  - from `api/controlplane`: `DecisionOutcome`, `TurnTransition`, `ResolvedTurnPlan` and dependent policy structs (`F-092`, `F-141`, `F-149`, `F-159`)
  - from `api/observability`: `ReplayDivergence`, `ReplayFidelity`, `ReplayAccessRequest`, `ReplayAccessDecision`, `ReplayAuditEvent` (`F-071`, `F-072`, `F-077`, `NF-028`)
- Keep behavior parity by reusing validation semantics and schema tags exactly.
- Exit criteria:
  - `go test ./api/... ./pkg/... ./test/contract` passes.
  - `make verify-quick` passes.

Phase 2: Public compatibility hardening
- Add explicit v1 compatibility tests in `pkg/contracts/v1/...`:
  - JSON compatibility snapshots (field names/tags)
  - enum value freeze tests
  - validation parity tests vs API source contracts
- Add CI check to prevent breaking changes to `pkg/contracts/v1/*` without compatibility notes (`NF-010`, `NF-032`, `F-163`).
- Exit criteria:
  - `make verify-quick` and compatibility checks pass.

Phase 3: Universal event facade + advanced module expansion
- Add optional universal-event facade types in `pkg/contracts/v1/eventabi` while preserving split artifacts for backward compatibility (`F-020` to `F-025`).
- Promote post-MVP modules when underlying contracts exist in code (`F-110` to `F-113`, `F-161` to `F-165`).
- Exit criteria:
  - replay and determinism suites remain green (`F-149`, `F-150`, `F-176`, `NF-028`, `NF-029`).

Implementation readiness checklist:
- Public import path exists and is documented.
- MVP-critical contract types are promotable without semantic drift.
- Compatibility policy + tests are enforced in CI.
- Replay-critical and authority-critical invariants remain unchanged after promotion (`NF-027`, `NF-028`, `NF-029`).

## Promotion Rules for `pkg/*`

- Promote only surfaces that external consumers must import.
- Keep `pkg/contracts/v1` compatibility stricter than `internal/*`.
- Require cross-review from Runtime + Control Plane + Observability owners for any promoted contract changes.
- Require feature-trace and compatibility note updates in this guide when public types change.
- Do not promote contracts that violate PRD invariants or MVP quality-gate semantics.
