# Runtime Kernel Guide

This guide proposes the target runtime architecture and module breakdown for RSPP runtime internals.

Evidence baseline:
- `docs/PRD.md` sections `4.1`, `4.2`, `4.3`, `5.1`, `5.2`
- `docs/rspp_SystemDesign.md` sections `3`, `4`
- `docs/RSPP_features_framework.json` (feature IDs cited inline)

## Runtime target architecture (PRD-aligned)

```text
Transport Adapter -> Event ABI Gateway -> Turn Lifecycle/Plan Freeze
                 -> Lane Scheduler (Control > Data > Telemetry)
                 -> Graph Executor -> Node Host -> Provider Invocation
                 -> Egress Fence/Transport
                              |-> Determinism + Timeline markers
                              |-> State/Authority/Identity enforcement
```

The runtime remains compute-stateless across pods while enforcing per-session turn semantics with lease/epoch safety and replay evidence (`F-103`, `F-149`, `F-155`, `F-156`).

## 1) Module list with responsibilities

| Module | Paths | Responsibilities | Feature evidence |
| --- | --- | --- | --- |
| `RK-01` Turn Lifecycle Orchestrator | `internal/runtime/session`, `internal/runtime/prelude`, `internal/runtime/turnarbiter`, `internal/runtime/planresolver` | Accept pre-turn intent, run admit/defer/reject, freeze immutable `ResolvedTurnPlan`, enforce canonical turn lifecycle. | `F-138`, `F-139`, `F-140`, `F-141`, `F-149` |
| `RK-02` Event ABI Gateway | `internal/runtime/eventabi` | Validate envelope/schema version, normalize ingress events, enforce unknown-event policy, preserve correlation markers. | `F-020`, `F-021`, `F-022`, `F-028`, `F-032` |
| `RK-03` Lane Scheduler & Edge Control | `internal/runtime/lanes`, `internal/runtime/buffering`, `internal/runtime/flowcontrol`, `internal/runtime/sync` | Enforce lane priority, bounded queues, watermark actions, flow control mode, and sync-drop/discontinuity rules. | `F-043`, `F-044`, `F-142`, `F-143`, `F-144`, `F-145`, `F-146`, `F-148` |
| `RK-04` Graph Executor | `internal/runtime/executor` | Execute compiled graph paths (fan-out/fan-in/select/fallback), preserve deterministic edge contract and terminalization rules. | `F-002`, `F-004`, `F-005`, `F-006`, `F-007`, `F-008`, `F-018` |
| `RK-05` Node Runtime Host | `internal/runtime/nodehost` | Run built-in and custom nodes, inject execution context, enforce hook timeouts and node-level isolation. | `F-010`, `F-011`, `F-012`, `F-036`, `F-037`, `F-134` |
| `RK-06` Cancellation & Fence Controller | `internal/runtime/cancellation`, `internal/runtime/transport` | Propagate cancel scopes, enforce barge-in behavior, flush/fence egress and reject post-cancel playback acceptance. | `F-033`, `F-034`, `F-035`, `F-038`, `F-039`, `F-041`, `F-175` |
| `RK-07` Budget/Degrade Engine | `internal/runtime/budget` | Track turn/node/provider budgets; produce deterministic continue/degrade/fallback/terminate outcomes. | `F-045`, `F-046`, `F-047`, `F-048`, `F-052`, `F-137` |
| `RK-08` Provider Runtime | `internal/runtime/provider/registry`, `internal/runtime/provider/invocation`, `internal/runtime/provider/bootstrap`, `providers/*` | Resolve provider bindings, invoke normalized modality adapters, apply retry/switch/circuit policy with deterministic fallback. | `F-055`, `F-056`, `F-057`, `F-059`, `F-063`, `F-064`, `F-065`, `F-160` |
| `RK-09` State/Authority/Identity | `internal/runtime/state`, `internal/runtime/guard`, `internal/runtime/identity` | Enforce lease epoch at ingress/egress, state class boundaries (turn/session hot/durable), idempotency and dedupe guarantees. | `F-013`, `F-155`, `F-156`, `F-158`, `F-159`, `NF-018` |
| `RK-10` Admission & Execution Pool | `internal/runtime/localadmission`, `internal/runtime/executionpool` | Enforce local resource admission, per-tenant/session fairness keys, and pooled scheduling with overload shedding. | `F-049`, `F-050`, `F-105`, `F-109`, `F-172`, `F-173`, `F-174` |
| `RK-11` Timebase & Determinism Bridge | `internal/runtime/timebase`, `internal/runtime/determinism` | Maintain monotonic/wall/media time mapping; emit replay-critical determinism markers and recording-level downgrade events. | `F-150`, `F-151`, `F-152`, `F-153`, `F-154`, `F-176`, `NF-009` |
| `RK-12` Transport Boundary Adapter Surface | `internal/runtime/transport`, `transports/livekit` | Map transport lifecycles/audio/control to Event ABI without owning media-stack concerns. | `F-083`, `F-086`, `F-087`, `F-089`, `F-090`, `F-091`, `NF-019` |
| `RK-13` External Node Boundary | `internal/runtime/externalnode` | Isolated out-of-process node execution with runtime-owned cancellation, resource limits, and observability injection. | `F-121`, `F-122`, `F-123`, `F-124`, `F-171` |

## 1.1) Current runtime progress and divergence (code-backed, 2026-02-16)

Legend:
- `implemented baseline`: code exists and is wired for the primary path.
- `partial`: code exists but does not yet satisfy full target contract.
- `scaffold/missing`: guide/spec exists but runtime code is not yet implemented.

| Module | Current status | Runtime evidence | Divergence from target contract |
| --- | --- | --- | --- |
| `RK-01` Turn Lifecycle Orchestrator | partial | Prelude and turn arbitration are implemented in `internal/runtime/prelude` and `internal/runtime/turnarbiter`; session manager package has guide-only scaffold. | Session-scoped lifecycle ownership is still concentrated in turn arbiter flow; missing concrete `session` manager runtime. |
| `RK-02` Event ABI Gateway | implemented baseline | `internal/runtime/eventabi/gateway.go` validates/normalizes events and control signals. | No major divergence for MVP baseline path. |
| `RK-03` Lane Scheduler & Edge Control | partial | Routing exists in `internal/runtime/lanes/router.go`; pressure and flow signals exist in `internal/runtime/buffering/pressure.go` and `internal/runtime/flowcontrol/controller.go`. | Full queue scheduler that enforces runtime dispatch priority (`Control > Data > Telemetry`) is not yet realized as a single runtime scheduling loop. |
| `RK-04` Graph Executor | partial | Executor scheduling and topological dispatch logic exists in `internal/runtime/executor/plan.go` and `internal/runtime/executor/scheduler.go`. | Resolver/executor path still uses simplified plan materialization and does not yet provide full graph compilation/policy freeze coverage. |
| `RK-05` Node Runtime Host | partial | Failure handling path exists in `internal/runtime/nodehost/failure.go`. | Full node host lifecycle contract (registration/lifecycle/timeouts/isolation) is not yet fully implemented in runtime package surface. |
| `RK-06` Cancellation & Fence Controller | implemented baseline | Cancellation fence in `internal/runtime/cancellation/fence.go`; transport output fence in `internal/runtime/transport/fence.go`. | No major divergence for cancel-fence baseline semantics. |
| `RK-07` Budget/Degrade Engine | implemented baseline | Deterministic budget decision logic in `internal/runtime/budget/manager.go`. | No major divergence for decision shape; downstream enforcement completeness depends on executor integration depth. |
| `RK-08` Provider Runtime | implemented baseline | Registry/invocation/bootstrap in `internal/runtime/provider/*`; concrete adapters under `providers/*`. | No major divergence on MVP provider invocation path. |
| `RK-09` State/Authority/Identity | partial | Authority and identity are implemented (`internal/runtime/guard/guard.go`, `internal/runtime/identity/context.go`); `internal/runtime/state` is scaffold-only. | State model contract (`turn/session hot/durable`) is not yet implemented as runtime service. |
| `RK-10` Admission & Execution Pool | implemented baseline | Admission logic in `internal/runtime/localadmission/localadmission.go`; pool manager in `internal/runtime/executionpool/pool.go`. | No major divergence for baseline admission/scheduling path. |
| `RK-11` Timebase & Determinism Bridge | partial | Determinism service exists (`internal/runtime/determinism/service.go`); `internal/runtime/timebase` remains scaffold-only. | Timebase mapping contract (monotonic/wall/media bridge) is not implemented yet. |
| `RK-12` Transport Boundary Adapter Surface | implemented baseline | Classification and signals in `internal/runtime/transport/classification.go` and `internal/runtime/transport/signals.go`; fence path in `internal/runtime/transport/fence.go`. | Transport baseline path is present; post-MVP adapters remain future scope. |
| `RK-13` External Node Boundary | scaffold/missing | `internal/runtime/externalnode` currently has guide scaffolding only. | No isolated external execution runtime yet (timeouts/resources/cancel propagation contract pending). |
| Sync integrity submodule | scaffold/missing | `internal/runtime/sync` is guide/scaffold only. | Deterministic sync-drop/discontinuity runtime behavior is not implemented as engine yet. |

## 1.2) Implementation-ready work plan (priority order)

The following sequence should be used as the implementation plan for runtime parity with this guide.

| Priority | Work package | Scope | Done criteria |
| --- | --- | --- | --- |
| `P0` | `WP-01 Session Lifecycle Manager` | Implement `internal/runtime/session` to own session-scoped authority state, turn registry, and prelude/arbiter coordination. | Session manager is runtime entrypoint for turn-open and terminalization; turn arbiter runs under session-owned state; integration tests cover pre-turn reject/defer and accepted-turn terminal sequence. |
| `P0` | `WP-02 Plan Freeze Upgrade` | Expand `internal/runtime/planresolver` + executor wiring to materialize full graph/runtime policy into immutable `ResolvedTurnPlan` per turn. | Resolver no longer returns hard-coded defaults; plan hash and resolved lane/budget/provider settings are replay-visible and used by executor. |
| `P0` | `WP-03 Lane Dispatch Scheduler` | Add concrete scheduler loop combining `lanes`, `buffering`, and `flowcontrol` with explicit per-lane dispatch priority and bounded queue behavior. | Saturation tests prove `ControlLane` preempts data/telemetry; watermark crossings cause deterministic shedding/degrade outcomes. |
| `P1` | `WP-04 Node Host Completion` | Implement runtime node registry/lifecycle/start-stop/cancel hook orchestration in `internal/runtime/nodehost`. | Node lifecycle hooks are enforced with timeout behavior; custom node registration path is stable and tested. |
| `P1` | `WP-05 State Service` | Implement `internal/runtime/state` contract for turn-ephemeral and session-hot boundaries (durable integration via external service handles). | Runtime state API exists with scope enforcement; migration/failover tests verify legal reset/reattach behavior. |
| `P1` | `WP-06 Timebase Service` | Implement `internal/runtime/timebase` mapping for monotonic + wall + media clocks and runtime markers. | Latency anchors are emitted from a single timebase source; replay tooling consumes mapping consistently. |
| `P1` | `WP-07 Sync Integrity Engine` | Implement `internal/runtime/sync` group/drop/discontinuity logic for synchronized streams. | Sync policies (`atomic_drop` / `drop_with_discontinuity`) execute deterministically under replay tests. |
| `P2` | `WP-08 External Node Runtime` | Implement isolated execution boundary in `internal/runtime/externalnode` with timeout/resource/cancel propagation and telemetry injection. | Out-of-process node path supports deterministic cancel and bounded resources with conformance tests. |

## 1.3) What changed in this guide for implementation-readiness

1. Added runtime code-backed status and divergence table for every major runtime module.
2. Converted abstract target architecture into a phased, prioritized implementation plan (`WP-01` to `WP-08`) with done criteria.
3. Explicitly marked scaffold-only modules so contributors do not treat guide targets as already implemented.

## 2) Key interfaces (pseudocode)

```go
// Top-level runtime surface.
type RuntimeKernel interface {
    OpenSession(ctx context.Context, req SessionOpenRequest) (SessionHandle, error)
    Ingest(ctx context.Context, event EventEnvelope) error
    Cancel(ctx context.Context, scope CancelScope, reason CancelReason) error
    CloseSession(ctx context.Context, sessionID string, reason string) error
}
```

```go
// Freeze turn behavior at turn start.
type PlanResolver interface {
    ResolveTurnPlan(ctx context.Context, in TurnPlanInput) (ResolvedTurnPlan, error)
}

type TurnArbiter interface {
    ProposeTurn(ctx context.Context, intent TurnIntent) (PreTurnOutcome, error) // admit|reject|defer
    OnControlEvent(ctx context.Context, ev EventEnvelope) ([]LifecycleEvent, error)
}
```

```go
// Edge-level scheduling and pressure handling.
type LaneScheduler interface {
    Enqueue(edge EdgeRef, ev EventEnvelope) (EnqueueResult, error)
    NextDispatch() (DispatchItem, bool)
    OnWatermark(signal WatermarkSignal) []ControlEvent
}
```

```go
// Node/plugin runtime contract.
type Node interface {
    Metadata() NodeMetadata
    Init(ctx context.Context, cfg NodeConfig) error
    OnEvent(ctx NodeExecutionContext, ev EventEnvelope, emit EmitFunc) error
    OnCancel(ctx NodeExecutionContext, reason CancelReason) error
    Shutdown(ctx context.Context) error
}

type NodeHost interface {
    Register(factory NodeFactory) error
    Dispatch(ctx NodeExecutionContext, target NodeRef, ev EventEnvelope) error
}
```

```go
// Provider extension contract.
type ProviderAdapter interface {
    Kind() ProviderKind // stt|llm|tts|...
    Capabilities() ProviderCapabilities
    Invoke(ctx ProviderContext, req ProviderRequest) (ProviderStream, error)
    Cancel(ctx context.Context, invocationID string, reason CancelReason) error
}
```

```go
// Transport extension contract.
type TransportAdapter interface {
    Bind(ctx context.Context, route SessionRoute) (ConnectionHandle, error)
    Inbound() <-chan TransportFrame
    Send(ctx context.Context, out TransportEgress) error
    Fence(ctx context.Context, scope CancelScope) error
}
```

```go
// Replay-critical timeline contract.
type TimelineRecorder interface {
    AppendLocal(ev EventEnvelope, markers ReplayMarkers) error // non-blocking path
    ExportAsync(batch []TimelineRecord) error
}
```

## 3) Data model: core entities and relations

### Core entities

| Entity | Key fields | Notes |
| --- | --- | --- |
| `Session` | `session_id`, `tenant_id`, `pipeline_version`, `routing_snapshot_ref`, `lease_epoch` | Long-lived conversation scope with authority context (`F-155`, `F-156`). |
| `Turn` | `turn_id`, `session_id`, `state`, `terminal_reason`, `opened_at`, `closed_at` | Canonical lifecycle `OPEN -> ACTIVE -> (COMMIT|ABORT) -> CLOSE` (`F-139`). |
| `ResolvedTurnPlan` | `resolved_turn_plan_id`, `plan_hash`, `execution_profile`, `provider_bindings`, `lane_policies` | Immutable per turn; determinism boundary (`F-149`, `F-078`). |
| `NodeBinding` | `node_id`, `node_type`, `scope`, `resource_limits`, `policy_refs` | Binds plan node to runtime host/external host (`F-011`, `F-121`). |
| `EdgeRuntime` | `edge_id`, `from_node`, `to_node`, `lane`, `buffer_spec`, `flow_mode` | Explicit buffering and lane behavior (`F-143`, `F-145`, `F-147`). |
| `EventEnvelope` | `event_id`, `event_type`, `lane`, `session_id`, `turn_id`, `schema_version`, `runtime_sequence` | Unified ABI for all traffic (`F-020`, `F-021`, `F-022`). |
| `ProviderInvocation` | `provider_invocation_id`, `turn_id`, `provider_ref`, `budget_scope`, `outcome` | First-class invocation tracking (`F-160`). |
| `PlacementLease` | `session_id`, `lease_epoch`, `lease_token`, `expires_at` | Single-writer authority control (`F-156`, `F-095`). |
| `DeterminismContext` | `seed`, `merge_rule_id`, `ordering_markers`, `nondeterministic_inputs[]` | Replay-critical metadata (`F-150`, `F-151`). |
| `TimelineRecord` | `record_id`, `session_id`, `turn_id`, `recording_level`, `event_ref`, `control_markers` | OR-02 evidence carrier (`F-153`, `F-154`, `NF-028`). |

### Relation model

```text
Session 1--N Turn
Turn 1--1 ResolvedTurnPlan
ResolvedTurnPlan 1--N NodeBinding
ResolvedTurnPlan 1--N EdgeRuntime
Turn 1--N EventEnvelope
Turn 1--N ProviderInvocation
Turn 1--1 DeterminismContext
Session 1--1 PlacementLease (current epoch)
Turn 1--N TimelineRecord
```

## 4) Extension points: plugins, custom nodes, custom runtimes

### A) Plugins

1. Provider plugins (`STT/LLM/TTS`) via `ProviderAdapter` registry.
- Contract: capability declaration, normalized outcomes, cancellation, metrics.
- Evidence: `F-055` to `F-066`, `F-160`.
2. Transport plugins via `TransportAdapter`.
- MVP: LiveKit; post-MVP: WebSocket and telephony.
- Evidence: `F-083`, `F-084`, `F-085`, `F-086`, `NF-019`.
3. Policy plugins for provider routing/fallback/degrade and optional compliance insertions.
- Evidence: `F-056`, `F-057`, `F-062`, `F-066`, `F-102`.

### B) Custom nodes

1. In-process custom nodes implement `Node` and run in `NodeHost`.
2. Out-of-process custom nodes run through `externalnode` boundary with explicit limits and trust controls.
3. Custom nodes must emit ABI-compliant events and honor cancellation hooks.
- Evidence: `F-011`, `F-012`, `F-036`, `F-121`, `F-122`, `F-123`, `F-124`.

### C) Custom runtimes

1. Runtime variants may replace scheduler/executionpool strategies while preserving the runtime contract.
2. Custom runtime profiles must still enforce lifecycle, lane priority, cancellation fencing, and OR-02 evidence.
3. Conformance profile testing is the compatibility gate for custom runtime distributions.
- Evidence: `F-078`, `F-138`, `F-142`, `F-175`, `NF-024` to `NF-029`, `F-164` (post-MVP conformance).

## 5) Design invariants (must preserve)

1. `turn_open` is emitted only after valid authority + admission + plan freeze (`F-141`, `F-149`, `F-156`).
2. Accepted turn lifecycle is exactly `OPEN -> ACTIVE -> (COMMIT|ABORT) -> CLOSE` (`F-139`).
3. Pre-turn `reject/defer/stale_epoch_reject` never emit accepted-turn terminal signals (`F-141`, `F-156`).
4. Exactly one terminal outcome (`commit` xor `abort`) per accepted turn (`NF-029`).
5. `ControlLane` always preempts `DataLane`; `TelemetryLane` is non-blocking best effort (`F-142`, `F-147`).
6. Every edge has bounded buffering (explicit or profile-derived), never implicit unbounded queues (`F-043`, `F-143`, `NF-004`).
7. Low-latency profiles cannot block indefinitely; bounded block time must transition deterministically to shedding/degrade (`F-148`).
8. Watermark crossings must produce deterministic pressure actions recorded in control evidence (`F-144`).
9. Cancellation propagates by declared scope and includes provider + egress fencing (`F-033`, `F-034`, `F-046`, `F-175`).
10. After cancel fence, new `output_accepted`/`playback_started` for canceled scope is invalid (`F-175`).
11. Lease epoch is enforced on ingress and egress; stale-authority outputs are rejected (`F-156`, `NF-027`).
12. `ResolvedTurnPlan` is immutable during a turn; policy changes apply only at explicit boundaries (`F-149`, `F-102`).
13. Session/turn correlation fields are present on all runtime events, metrics, and traces (`F-013`, `F-067`, `NF-008`).
14. OR-02 replay-critical evidence is mandatory for every accepted turn (`NF-028`, `F-153`, `F-154`).
15. Recording-level downgrade under timeline pressure must be deterministic and preserve control evidence (`F-176`).
16. Runtime honors boundary non-goals: no SFU/WebRTC stack ownership, no model-quality guarantees (`NF-019`, `NF-021`).

## 6) Tradeoffs and alternatives

| Decision | Why chosen | Alternative | Cost/Risk |
| --- | --- | --- | --- |
| Immutable per-turn `ResolvedTurnPlan` | Deterministic replay and predictable lifecycle behavior. | Allow mid-turn dynamic policy rewrites. | Less adaptability to sudden provider/price changes mid-turn. (`F-149`, `F-151`) |
| Strict lane priority (`Control > Data > Telemetry`) | Fast cancel/authority/admission control under pressure. | Weighted fair queueing across all lanes. | Data throughput may dip during heavy control bursts. (`F-142`, `F-175`) |
| Bounded queues + deterministic shedding | Prevents memory blowups and tail-latency collapse. | Unbounded queues or purely blocking backpressure. | Potential quality loss from dropped/coalesced interim data. (`F-043`, `F-044`, `NF-004`) |
| Compute-stateless runtime with externalized durable state | Easier horizontal scaling and failover behavior. | Sticky stateful runtime pods with rich in-memory session state. | Migration may reset hot state and reduce continuity quality. (`F-103`, `F-155`) |
| Unified event ABI for all lanes/types | Single contract for tooling, replay, and cross-module interoperability. | Modality-specific interfaces per subsystem. | Envelope overhead and stricter schema governance burden. (`F-020`, `F-021`, `F-022`, `F-028`) |
| In-process nodes by default, isolated external nodes optional | Best latency for trusted code while preserving a safe boundary for untrusted extensions. | All nodes out-of-process by default. | Dual execution models increase operational complexity. (`F-121`, `F-122`, `F-124`) |
| Dual replay modes (re-simulate + provider-playback) | Supports both determinism checks and realistic provider behavior regression testing. | Only one replay mode. | More tooling complexity and evidence requirements. (`F-151`, `F-153`, `NF-009`) |

## MVP scope notes

1. Live transport path is the MVP runtime path (`F-083`); WebSocket/telephony are post-MVP (`F-084`, `F-085`).
2. `ExecutionProfile simple/v1` is the MVP baseline (`F-078`, `F-079`).
3. Runtime quality gates remain mandatory: turn-open latency, first output latency, cancel-fence latency, authority safety, replay completeness, terminal correctness, and end-to-end latency (`NF-024` to `NF-029`, `NF-038`).
