# RSPP Modular Design: What Modules the Framework Needs

## 1. Purpose and alignment boundary

This document defines the module architecture required to realize `docs/rspp_SystemDesign.md` sections 1-3, while treating section 4 contracts as normative.

Primary objective:
- Define modules, ownership boundaries, and contract interactions.
- Keep implementation choices open.
- Make requirement and non-goal traceability explicit.

## 2. Requirement baseline extracted from sections 1-3

### 2.1 Required capabilities

R1. Executable real-time streaming graphs with deterministic routing/ordering.

R2. Unified versioned Event ABI across audio/text/control/telemetry.

R3. Cancellation-first behavior (barge-in/interrupt propagation).

R4. Explicit backpressure/budget handling with bounded queues.

R5. Provider/model swapability through adapters and policy.

R6. Built-in observability and replay timelines.

R7. Low-overhead runtime semantics and measured overhead budget.

R8. First-class turn detection (VAD/turn detector as swappable primitives).

R9. Simple mode with minimal required fields and deterministic defaults.

R10. K8s-native disposable runtime instances with horizontal scaling.

R11. Multi-region active-active fleet with single-authority execution per session via leases/epochs.

R12. Transport-agnostic orchestration.

R13. Control-plane versioning/rollouts/admission/placement basics plus external-node execution boundaries.

R14. Developer experience flow: quickstart, integration, release, CI/replay validation, operations.

### 2.2 Non-goals to enforce as hard boundaries

NG1. Not an RTC transport stack (no SFU/WebRTC/ICE/TURN ownership).

NG2. Not an agent SDK/planner framework.

NG3. Not a model-quality guarantor.

NG4. Not owner of business-system integrations.

NG5. Not a model API replacement.

## 3. Module planes

RSPP modules are partitioned into four planes:

1. Control plane modules: spec governance, policy, placement, admission, routing views.
2. Runtime kernel modules: turn/session execution, lanes, buffering, cancellation, state, provider and transport boundaries.
3. Observability and replay modules: timeline, telemetry, replay diagnostics.
4. Developer tooling modules: local runner, validation/replay test harness, release and operational workflows.

### 3.1 Ownership split (authoring vs enforcement)

Developer-owned:
- `PipelineSpec` author intent, custom node logic, optional policy intent, provider configuration inputs.

Platform-owned (control plane):
- Version resolution, rollout decisions, admission, placement authority, routing view publication, policy and security evaluation.

Runtime-owned (kernel):
- Turn execution semantics, cancellation propagation, buffering/flow-control behavior, determinism, replay correlation, and lane-priority enforcement.

## 4. Control plane module set

### CP-01 Pipeline Registry

Owns immutable storage/versioning of `PipelineSpec`, `ExecutionProfile`, and rollout metadata.

Inputs:
- Developer-published artifacts.

Outputs:
- Resolved pipeline version + artifact references.

Governance additions:
- Owns ABI schema publication registry and deprecation policy metadata used by RK-05 validation.
- Publishes compatibility windows and supported code-generation targets for contract consumers.

### CP-02 Spec Normalizer and Validator

Normalizes spec/profile intent into explicit semantics.

Responsibilities:
- Apply Simple mode defaults.
- Validate graph typing, buffer/budget completeness, lane rules, security and tenancy annotations.
- Validate policy/admission references.

Output:
- Normalized spec ready for graph compilation.

### CP-03 GraphDefinition Compiler

Compiles normalized spec into canonical `GraphDefinition` with stable node/edge identities, legal paths, fan-in/fan-out rules, and terminal paths.

### CP-04 Policy Evaluation Engine

Evaluates `Policy Contract` intent for provider binding, degrade posture, routing preference, cost and circuit behavior.

Rule:
- Decisions affecting turn behavior are frozen into `ResolvedTurnPlan` at turn start.
- Policy evaluation consumes `ProviderHealthSnapshot` inputs for deterministic turn-start binding decisions.
- Publishes `PolicyResolutionSnapshot` artifacts for runtime-local turn-start resolution.

### CP-05 Resource and Admission Engine

Implements `Resource & Admission Contract`.

Responsibilities:
- Define authoritative admission/scheduling policy at tenant/session/turn scopes.
- Define fairness keys and overload precedence (ControlLane protected, TelemetryLane best-effort).
- Publish `AdmissionPolicySnapshot` artifacts (and optional quota/token leasing semantics) for runtime-local enforcement.
- Emit deterministic policy-change and snapshot-version events.
- Control-plane direct admit/defer/reject decisions, when used, are limited to explicit control-plane boundaries (for example session bootstrap) and are not required on runtime hot-path scheduling points.

### CP-06 Security and Tenant Isolation Engine

Implements `Security & Tenant Isolation Contract`.

Responsibilities:
- Authn/authz/tenant context validation and propagation.
- Secret and data-access boundary enforcement.
- Tenant isolation for execution and telemetry domains.

Isolation and safety rules:
- Tenant identity context MUST be immutable once a turn becomes `Active`; cross-tenant context mutation is invalid.
- Secrets MUST not be persisted in replay timelines; OR-02 stores references/redacted forms only.
- Telemetry exports MUST preserve tenant partitioning and access controls equivalent to execution isolation.
- Replay access/export MUST enforce tenant-scoped authorization and auditable access records for timeline artifacts.
- External-node calls must receive least-privilege scoped credentials, bounded to declared node capabilities.
- Payload data classes (for example `PII`, `PHI`, `audio_raw`, `text_raw`, `derived_summary`) MUST be tagged at ingest or first transformation boundary.
- Retention and delete semantics MUST be declared per data class and enforce tenant right-to-delete for persisted timeline artifacts.

### CP-07 Placement Lease Authority

Issues and rotates `PlacementLease` epochs and validates execution authority.

Responsibilities:
- Prevent split-brain execution.
- Invalidate stale placement writers/readers.

### CP-08 Routing View Publisher

Publishes read-optimized `Routing View / Registry Handle` snapshots for runtime and adapters.

Responsibilities:
- Fast endpoint and placement discovery.
- Migration visibility with monotonic epoch updates.
- Distribute runtime-consumable snapshots (`AdmissionPolicySnapshot`, `AbiCompatibilitySnapshot`, `VersionResolutionSnapshot`, `PolicyResolutionSnapshot`) with monotonic versioning.

### CP-09 Rollout and Version Resolver

Owns rollout policy evaluation and concrete pipeline version resolution for each admitted session/turn.

Output:
- `VersionResolutionSnapshot` artifacts consumed by runtime turn-start resolution.

### CP-10 Provider Health Aggregator

Aggregates provider health and circuit state across instances/tenants for policy and runtime consumption.

Responsibilities:
- Aggregate provider error/latency/overload signals into `ProviderHealthSnapshot`.
- Publish snapshot versions consumed at turn start and by runtime fast-reaction paths.
- Provide deterministic health-state provenance for policy decisions and replay analysis.

## 5. Runtime kernel module set

### RK-01 Session Lifecycle Manager

Manages logical session open/resume/migrate/close independent of runtime instance identity.

### RK-02 Session Prelude Engine

Runs `SessionPreludeGraph` (session-scoped continuous nodes like VAD/turn detection).

Output:
- Turn boundary intent signals, including non-authoritative `turn_open_proposed`.

### RK-03 Turn Arbiter

Enforces `Turn` state machine and `Turn Arbitration Rules`.

Responsibilities:
- Accept/reject `turn_open_proposed` based on admission, authority, and arbitration policy; emit authoritative `turn_open` only on acceptance.
- Enforce terminal outcome: one `commit` or `abort(reason)`, then `close`.
- Handle overlap, preemption, grace windows, and late events.
- Emit `commit` only after terminal turn evidence is appended to OR-02 Stage-A local append log: final DataLane output if present, otherwise a control-only terminal marker.
- Consume admission outcomes emitted by RK-25 to gate turn lifecycle transitions; RK-03 is not the authoritative emitter of runtime `admit/reject/defer`.

### RK-04 ResolvedTurnPlan Resolver

Materializes immutable `ResolvedTurnPlan` at turn start by combining:
- normalized spec and `GraphDefinition`
- placement/version/admission outcomes
- policy outcomes
- capability snapshots

Frozen fields include provider binding, flow-control mode/thresholds, edge buffering semantics, determinism markers, and `AllowedAdaptiveActions`.
Frozen fields also include snapshot provenance references (`RoutingViewSnapshot`, `AdmissionPolicySnapshot`, `AbiCompatibilitySnapshot`, `VersionResolutionSnapshot`, `PolicyResolutionSnapshot`, `ProviderHealthSnapshot`) so turn-start decisions remain replay-explainable.

Snapshot validity rule:
- RK-04 MUST validate required snapshot freshness/monotonicity before turn acceptance.
- If required snapshots are stale, missing, or version-incompatible, runtime MUST follow deterministic configured outcomes (`defer` or `reject`) and MUST NOT emit `turn_open`.
- RK-04 materialization is required only for candidate turns that are still admissible after local admission/authority gating; rejected/deferred pre-turn outcomes MUST NOT force plan materialization.

Adaptive execution rule:
- In-turn adaptive choices are allowed only if pre-authorized in `AllowedAdaptiveActions`.
- Every adaptive choice MUST emit a ControlLane decision event and be recorded by OR-02.
- If an adaptive choice is not provably deterministic from recorded deterministic inputs, it MUST be captured as a `nondeterministic_inputs[]` entry in DeterminismContext.

### RK-05 Event ABI Gateway

Validates and stamps Event ABI envelope fields and compatibility rules.

Responsibilities:
- Required envelope fields: `session_id`, `pipeline_version`, `event_id`, `lane`, and `sequence` markers; `turn_id` is mandatory for turn-scoped events.
- Correlation and causality fields: `node_id/edge_id`, `causal_parent_id`, `idempotency_key`, provider invocation correlation when present.
- Sync/discontinuity fields: `sync_id`, `sync_domain`, `discontinuity_id` when sync policies require them.
- Merge/drop lineage fields: `merge_group_id`, `merged_from_event_ids` (bounded), and deterministic late/drop lineage markers.
- Timestamp set completeness: monotonic runtime timestamp, wall-clock correlation timestamp, and media-time fields when payload is media-bound.
- Schema version negotiation and compatibility validation (forward/backward envelope rules).
- Deprecation enforcement windows (reject-after deadline for retired schema versions).
- Lane classification integrity.
- Event envelope overhead guardrails for hot paths (size/cost validation with policy-defined thresholds).
- Enforce payload classification-tag presence by ABI crossing time (source tag from ingress or first transform boundary).

Contract rules:
- ABI validation failure MUST produce deterministic ControlLane error outcomes (not silent drop).
- ABI compatibility decisions MUST be recorded in OR-02 for replay explainability.
- Dedupe keys and ordering markers MUST remain stable across retries/reconnects/failovers.
- ABI compatibility enforcement uses local `AbiCompatibilitySnapshot` cache; runtime behavior under stale snapshots must be deterministic (grace/reject policy by configured deadline).
- Reject-after deprecation enforcement must be driven by a declared timebase with skew tolerance policy.

### RK-06 Lane Router

Routes DataLane, ControlLane, TelemetryLane with strict priority contract.

Rule:
- ControlLane preempts and cannot be blocked by DataLane/TelemetryLane pressure.

### RK-07 Graph Runtime Executor

Executes nodes and edges of `GraphDefinition` under `ResolvedTurnPlan` constraints.

Responsibilities:
- Scope-aware node scheduling (session-scoped vs turn-scoped).
- Deterministic merge/fan-in behavior.
- Pre-resolved fallback/degrade branch execution.

### RK-08 Node Runtime Host

Hosts built-in and custom nodes.

Responsibilities:
- Inject `NodeExecutionContext`.
- Enforce preemption hooks (`on_cancel`, `on_barge_in`, `on_budget_exhausted`).
- Enforce scope-safe `StateHandle` access.
- Enforce declared payload-class transformation rules across node boundaries.

### RK-09 External Node Boundary Gateway

Implements `External Node Boundary Contract`.

Responsibilities:
- Isolated execution boundary enforcement.
- Timeout/resource/cancel propagation guarantees.
- Mandatory observability and identity context injection.

### RK-10 Provider Adapter Manager

Manages provider adapter plugins and normalized capability surfaces (`Provider Contract`).

Adapter conformance requirements:
- Expose streaming behavior profiles (ASR partial rollback model, LLM delta granularity, TTS chunk sizing expectations).
- Use a stable versioned error taxonomy enum so metrics remain comparable across adapters.

### RK-11 Provider Invocation Controller

Owns `ProviderInvocation` lifecycle.

Responsibilities:
- Invocation identity/idempotency.
- Cancel/timeout/retry semantics.
- Normalized outcome production.
- Consume provider-health/circuit snapshots for runtime fast-path protections, with all in-turn changes recorded as decision evidence.

Outcome normalization rules (provider + external parity):
- Success: final or partial outputs accepted with terminal success marker.
- Timeout: explicit timeout outcome with retryability flag from CP-04 policy.
- Overload/rate limit: normalized overload outcome, optional backoff metadata.
- Safety/policy block: non-retryable blocked outcome unless policy explicitly permits alternate route.
- Transport/provider disconnect: normalized infrastructure failure with deterministic fallback decision.
- Cancelled: scope-aware cancellation outcome with bounded cancel-latency measurements.

Cross-boundary consistency:
- RK-09 (External Node Boundary Gateway) MUST expose the same outcome classes so downstream arbitration and replay do not branch by execution location.
- OR-02 MUST record normalized outcome class and retry decision provenance for each invocation-like unit.

### RK-12 Edge Buffer Manager

Implements `BufferSpec` bounds/strategies/lane handling on every edge.

Rule:
- Every edge must have effective BufferSpec (explicit or ExecutionProfile default).

### RK-13 Watermark Engine

Implements `Watermarks` and emits pressure signals.

Rule:
- Triggered actions are only those pre-resolved in `ResolvedTurnPlan`.

### RK-14 Flow Control Engine

Implements `FlowControlMode` (`signal`, `credit`, `hybrid`).

Responsibilities:
- Emit/honor `flow_xoff`, `flow_xon`, `credit_grant`.
- Enforce bounded blocking (`max_block_time`) with deterministic shedding.

### RK-15 Sync Integrity Engine

Implements `SyncDropPolicy` and discontinuity semantics.

Responsibilities:
- `atomic_drop`, `drop_with_discontinuity`, `no_sync`.
- Domain-aware sync behavior (`sync_domain`).
- Emit `discontinuity(sync_domain, discontinuity_id, reason)`.

### RK-16 Cancellation Manager

Implements `CancellationScope + CancelToken` propagation across node/provider/buffer scopes with bounded cancel latency.

Cancellation tiers:
- Tier 1 (hard guarantee): runtime output fencing. After cancel acceptance or authority loss, downstream output emission for that scope is fenced immediately.
- Tier 2 (best-effort): provider/external cancellation attempts with fully observable outcomes.

Required cancel observability:
- `cancel_sent_at`
- `cancel_ack_at` (when provider supports acknowledgement)
- `late_provider_output_dropped_count`

Cancel latency definition:
- `cancel_latency` is measured to local runtime output fencing, not provider ACK completion.

### RK-17 Budget Manager

Implements `Budget` accounting across turn/node/path/edge scopes.

Outputs:
- continue/degrade/fallback/terminate outcomes via ControlLane.

### RK-18 Timebase Service

Implements `Timebase Contract` mapping monotonic, wall-clock, and media time references.

Timebase responsibilities:
- Canonical media timeline is sample-index time for audio-bound paths.
- Canonical internal audio format requirements (sample rate/channels/frame size) are profile-defined.
- Resample/normalization responsibility must be explicit per profile: either at RK-22 ingress normalization or via a declared graph node.

### RK-19 Determinism Service

Implements `Determinism Contract` and manages `DeterminismContext`.

Responsibilities:
- Seed and ordering marker issuance.
- Merge-rule identity/version issuance for deterministic fan-in replay.
- Declared nondeterminism boundaries.
- Turn-level replay guarantee metadata.

### RK-20 State Access Layer

Implements `State Contract (Hot vs Durable)`.

Responsibilities:
- Turn-ephemeral state APIs.
- Session-hot state APIs (non-required failover persistence, not cross-region replicated by default).
- Session-durable state APIs (externalized and reattachable).

### RK-21 Identity, Correlation, and Idempotency Service

Implements canonical IDs and dedupe behavior across retry/reconnect/failover; includes lease epoch in correlation semantics.

### RK-22 Transport Mapping Adapter

Implements `Transport Boundary Contract` mapping transport events to Event ABI and outputs back to transport.

Responsibilities:
- Enforce egress output fencing/queue flush on cancellation or authority loss before delivery signaling.
- Tag raw ingress payloads with initial data classification (for example `audio_raw`) before ABI gateway enforcement.

Boundary:
- No transport-stack ownership (NG1).

### RK-23 Connection Adapter

Normalizes transport connection state and silence/mute/stall semantics into control signals.

Boundary:
- Signal normalization only; no transport media-plane implementation.

### RK-24 Runtime Contract Guard

Continuously verifies runtime invariants during execution:
- plan immutability per turn
- lane priority enforcement
- one terminal outcome per turn
- authority epoch validity
- runtime overhead budget conformance (triggering deterministic degrade/shedding paths when envelopes are at risk)

### RK-25 Local Admission and Scheduling Enforcer

Applies authoritative admission/scheduling policy at runtime hot-path scheduling points.

Inputs:
- `AdmissionPolicySnapshot` from CP distribution.

Responsibilities:
- Enforce fairness, quotas, and shedding at edge enqueue, edge dequeue, and node dispatch.
- Maintain runtime-local counters/tokens for hot-path decisions.
- Emit deterministic admission/shedding decision events used by observability and replay.

### RK-26 Execution Pool Manager

Owns runtime execution pools, concurrency control, and resource-aware dispatch.

Responsibilities:
- Allocate execution slots for node invocations and manage worker-pool sizing.
- Integrate with RK-16 cancellation for preemptible/cooperative task handling.
- Integrate with RK-17 budgets for deadline-aware scheduling and overload behavior.
- Expose execution-queue time vs execution-time metrics for latency accountability.

## 6. Observability and replay module set

### OR-01 Telemetry Pipeline

TelemetryLane collection with overload-safe best-effort behavior.

Telemetry security posture:
- Redact or tokenize sensitive payload segments before persistence/export.
- Preserve tenant-scoped correlation without exposing cross-tenant identifiers.
- Mark security-relevant telemetry with audit flags so CP-06 policies can enforce retention and access boundaries.

Telemetry conventions (OTel-friendly):
- Standard span hierarchy: `turn_span` -> `node_span` -> `provider_invocation_span`.
- Stable metric names/tags for critical operations (`queue_depth`, `drops_total`, `cancel_latency_ms`, `provider_rtt_ms`, `shed_rate`).
- Correlation tags MUST include session/turn/lease context without leaking sensitive payload data.

### OR-02 Event Timeline Recorder

Persists replay-critical timeline with session/turn/lane ordering, plan hash, determinism seed, ordering markers, and invocation lineage.

Two-stage architecture:
- Stage A (hard requirement): non-blocking local append log for turn-critical evidence.
- Stage B (asynchronous): persistence/export pipeline with bounded retry and backpressure handling.

OR-02 invariants:
- MUST NOT block ControlLane delivery.
- MUST NOT block cancellation propagation.
- MUST apply `TimelineAppendBufferSpec`-style bounded-memory append buffers with explicit overflow behavior.
- MUST reserve replay-critical Stage-A append capacity for baseline evidence (plan hash, determinism markers, authority markers, admission/policy decisions, terminal outcomes) so best-effort detail cannot starve correctness evidence.

Recorded evidence baseline (all recording levels):
- event envelope snapshot (post-ABI validation view)
- payload classification tags and redaction decisions
- `ResolvedTurnPlan` hash and pipeline version
- snapshot provenance references used for turn start (`RoutingViewSnapshot`, `AdmissionPolicySnapshot`, `AbiCompatibilitySnapshot`, `VersionResolutionSnapshot`, `PolicyResolutionSnapshot`, `ProviderHealthSnapshot`)
- admission/policy/authority decision outcomes that influence turn opening and execution gating
- determinism seed and ordering markers
- merge_rule_id and merge_rule_version used for timeline merge
- lease epoch and migration/handoff markers
- provider/external invocation normalized outcomes and retry-decision provenance

Stage-A overflow handling:
- On pressure, OR-02 MUST deterministically drop/summarize non-critical detail first and emit `recording_level_downgraded` when fidelity is reduced.
- OR-02 MUST NOT stall RK-03 commit/close progression.
- If replay-critical baseline evidence append cannot be guaranteed, runtime MUST deterministically emit `abort(reason=recording_evidence_unavailable)` followed by `close`.
- Stage-A reserved capacity MUST always include terminal outcome markers so the deterministic `abort(reason=recording_evidence_unavailable)` and `close` signals can still be appended.

### OR-03 Replay Engine

Supports replay modes:
- re-simulate node execution
- playback recorded provider outputs
- replay-decisions (authoritative recorded adaptive decisions)
- recompute-decisions (allowed only when deterministic inputs are sufficient and recorded)

Access control rule:
- OR-03 MUST enforce tenant-scoped authorization and policy-aligned redaction constraints when serving replay results.

Must preserve lane priorities, arbitration semantics, and deterministic stepping rules.

Divergence classification (mandatory output):
- `ORDERING_DIVERGENCE`: event ordering differs from recorded deterministic markers.
- `PLAN_DIVERGENCE`: plan hash, frozen policy/buffer fields, or plan-included snapshot provenance references differ.
- `OUTCOME_DIVERGENCE`: terminal outcome or provider/external normalized outcomes differ.
- `TIMING_DIVERGENCE`: budget/cancel timing crosses declared deterministic tolerance bounds.
- `AUTHORITY_DIVERGENCE`: lease epoch or migration marker mismatch.

### 6.1 Recording levels and replay guarantees

Level 0 (default):
- Record ControlLane + replay-critical summaries (final ASR/LLM/TTS metadata and terminal markers).
- Raw audio and high-volume partial streams may be omitted, sampled, or represented by hashes.
- Supports control-flow replay and outcome-forensics; full content re-simulation is limited.
- Level 0 minimum evidence: plan hash, determinism markers, authority markers, admission/policy decisions, terminal outcomes.
- Overload behavior: never block runtime hot path; drop non-critical detail first while preserving baseline evidence. If baseline evidence cannot be preserved, deterministic terminal policy is `abort(reason=recording_evidence_unavailable)`.

Level 1 (debug):
- Record partials/deltas with bounded sampling and selected per-chunk media captures.
- Supports deeper timing/order diagnostics and broader replay of streaming behavior.
- Level 1 minimum evidence: Level 0 plus sampled partial/delta evidence.
- Overload behavior: sampling/downgrade allowed; must emit deterministic control markers when fidelity is reduced. If baseline evidence cannot be preserved, deterministic terminal policy is `abort(reason=recording_evidence_unavailable)`.

Level 2 (forensics/eval):
- Full-fidelity timeline capture for enabled scopes under strict security and retention controls.
- Supports highest-fidelity replay modes and detailed divergence localization.
- Level 2 minimum evidence: Level 1 plus full-fidelity enabled streams.
- Overload behavior: MUST emit deterministic `recording_level_downgraded` marker and fall back to configured lower level when capacity limits are exceeded. If fallback still cannot preserve baseline evidence, deterministic terminal policy is `abort(reason=recording_evidence_unavailable)`.

Profile requirements:
- ExecutionProfile MUST declare recording level and allowed replay modes.
- Cost ceilings and security posture (retention/redaction/access) MUST be declared per recording level.
- Timeline append buffers MUST define explicit bounds and overflow policy to prevent unbounded memory growth.

## 7. Developer tooling module set

### DX-01 Local Runner

Single-node runtime + minimal control-plane stub for quickstart integration testing.

### DX-02 Spec Lint and Contract Test Harness

Runs schema/contract validation, profile-default expansion checks, and static policy/admission/security checks.

### DX-03 Replay Regression Harness

Runs deterministic replay tests and divergence reports in CI.

### DX-04 Release and Rollout CLI

Publishes artifacts, configures rollout policies, and gates releases on validation status.

### DX-05 Operations SLO Pack

Standard dashboards and alerts for p95/p99, cancel latency, drops, queue pressure, provider error rate, and admission/shedding patterns.

### DX-06 Node Authoring Kit

Contract-first node authoring toolkit (not an agent SDK).

Provides:
- stable node interface templates and lifecycle hook scaffolds
- context-handle bindings (`CancelToken`, `BudgetHandle`, `StateHandle`, `TelemetryHandle`)
- contract conformance tests for node authors
- local-runner integration helpers

### 7.1 Developer lifecycle gates (normative)

Gate A: Quickstart readiness (DX-01 + DX-02 + DX-06)
- Local runner boots with selected profile and validates minimal artifact set.
- ABI/schema and contract checks pass with no blocking errors.

Gate B: Integration readiness (DX-01 + RK-22/RK-23)
- Transport mapping and connection normalization tests pass.
- Turn boundary and cancellation signals are observed end-to-end.

Gate C: Release readiness (DX-02 + DX-03 + DX-04)
- Spec validation passes.
- Replay regression passes with no unexplained divergence classes.
- Rollout policy has explicit failure rollback posture.

Gate D: Operational readiness (DX-05)
- Dashboards/alerts wired for p95/p99 latency, cancel latency, drops, provider errors, admission shed rate.
- On-call runbook includes migration/failover and stale-epoch rejection diagnostics.

## 8. Module interface surfaces (contract-level)

The design stays implementation-agnostic, but each plane exposes stable interface shapes.

Control-plane orchestration interfaces (authoring/boundary-time, not runtime hot-path):
- `ResolvePipelineVersion(session_ctx, requested_pipeline) -> pipeline_version`
- `EvaluatePolicy(session_ctx, turn_ctx, normalized_spec) -> policy_outcome`
- `EvaluateSessionAdmission(session_ctx, demand) -> admit|defer|reject` (explicit boundary-time admission only; not enqueue/dequeue/dispatch hot-path)
- `IssueOrValidateLease(session_id, runtime_id, epoch_hint) -> placement_lease`

Runtime-consumed distribution interfaces (snapshot/artifact path):
- `GetRoutingViewSnapshot(session_id) -> routing_snapshot`
- `GetAdmissionPolicySnapshot(scope) -> admission_policy_snapshot`
- `GetAbiCompatibilitySnapshot() -> abi_compatibility_snapshot`
- `GetProviderHealthSnapshot(scope) -> provider_health_snapshot`
- `GetVersionResolutionSnapshot(session_ctx) -> version_resolution_snapshot`
- `GetPolicyResolutionSnapshot(session_ctx, turn_ctx) -> policy_resolution_snapshot`

Runtime-kernel interfaces:
- `ResolveTurnPlan(turn_open_proposed_event, normalized_spec, local_snapshots) -> ResolvedTurnPlan`
- `DispatchEvent(event) -> lane_routed_event`
- `ExecuteNode(node_id, input_events, NodeExecutionContext) -> output_events`
- `ApplyBufferPolicy(edge_id, lane, event) -> enqueue|drop|merge|block`
- `ApplyLocalAdmissionAndScheduling(policy_snapshot, scheduling_point, context) -> allow|defer|reject|shed`
- `AllocateExecutionSlot(node_id, budget_ctx, qos_ctx) -> slot|queued|rejected`
- `FenceOutputs(scope, reason) -> fencing_applied`
- `PropagateCancel(scope, reason) -> cancel_ack_set`
- `RecordBudgetOutcome(scope, outcome) -> control_signal`

Replay and telemetry interfaces:
- `AppendTimeline(event) -> replay_cursor`
- `ReplayFrom(cursor, mode) -> replay_result` (includes divergence classes and evidence references)
- `EmitTelemetry(sample|span|log) -> accepted|sampled|dropped`

Tooling interfaces:
- `RunLocal(spec_ref, profile_ref, provider_cfg) -> local_endpoint`
- `ValidateSpec(spec_ref) -> validation_report`
- `RunReplayRegression(test_suite) -> pass|fail + divergence_report`
- `PublishRelease(spec_ref, rollout_cfg) -> release_id`

### 8.1 Signal ownership and emission matrix

DataLane signal families:
- audio/text/semantic outputs: produced by RK-07 and RK-08, shaped by RK-06 and RK-12.
- final assistant audio chunks: produced by RK-07/RK-08, delivery acknowledgements handled on ControlLane by RK-22/RK-23.

ControlLane signal families:
- turn intent (`turn_open_proposed` runtime-internal, non-authoritative intent from RK-02).
- turn boundaries (authoritative `turn_open`, `commit`/`abort`, `close` from RK-03).
- interruption/cancellation (`barge-in`, `stop`, `cancel(scope)`): ingress via RK-06, propagated by RK-16, enforced by RK-08/RK-11.
- pressure/budget (`watermark`, budget warnings/exhaustion, degrade/fallback outcomes): emitted by RK-13 and RK-17.
- discontinuity and sync recovery: emitted by RK-15.
- deterministic drop/merge lineage markers (`drop_notice`, merge lineage metadata): emitted by RK-12/RK-13.
- flow-control (`flow_xoff`, `flow_xon`, `credit_grant`): emitted/honored by RK-14.
- provider outcomes/circuit and provider switch decisions: emitted by RK-11 and governed by CP-04.
- routing/migration (`lease issued/rotated`, migration markers, handoff markers): emitted by CP-07/CP-08 and enforced by RK-24.
- admission/capacity outcomes (`admit/reject/defer`, shedding): emitted authoritatively by RK-25 under CP-05-authored policy snapshots; CP-05 emits policy/snapshot version decisions (plus optional boundary-time session admission decisions). RK-03 consumes these outcomes to open/defer/reject turns.
- snapshot validity outcomes for turn start (for example stale/missing/incompatible snapshot) are emitted via the same deterministic admission/capacity outcome path.
- authority validation outcomes (`stale_epoch_reject`, `deauthorized_drain`): emitted by RK-24 and consumed by RK-03/RK-22 for authority-safe lifecycle and delivery behavior.
- connection lifecycle (`connected`, `reconnecting`, `disconnected`, `ended`, silence/stall): emitted by RK-23.
- replay-critical markers (`plan hash`, determinism seed, ordering markers): emitted by RK-04/RK-19 and recorded by OR-02.
- observability/degrade markers (`recording_level_downgraded`): emitted by OR-02 on deterministic fidelity downgrade.
- output delivery signals (`output_accepted`, `playback_started`, `playback_completed`, `playback_cancelled`): emitted at transport boundary by RK-22/RK-23.

TelemetryLane signal families:
- metrics/traces/logs/debug snapshots: emitted through OR-01 with best-effort shedding guarantees.

Rules:
- RK-06 and RK-12 MUST preserve ControlLane preemption under overload.
- OR-01 MUST never block ControlLane delivery.
- Signal ownership must remain transport-agnostic; transport-specific payload shapes are mapped only in RK-22/RK-23.
- `turn_open_proposed` is runtime-internal and not guaranteed to be surfaced outside runtime boundaries.

### 8.2 Lease epoch propagation rules

Authority propagation contract:
- CP-07 issues lease token + epoch; CP-08 distributes routing snapshots with epoch visibility.
- RK-21 stamps current lease epoch into correlation context for all authority-relevant events (turn-scoped events plus session-scoped control events that can affect execution/output).
- RK-22 attaches authority context to transport-bound outputs and validates authority context on ingress.
- RK-24 rejects stale-epoch events/outputs deterministically and emits diagnostics control signals for auditability.

Boundary requirements:
- Epoch validation MUST happen on both ingress and egress paths.
- Epoch mismatch MUST not be auto-healed by silent retry; it must trigger explicit migration/reauthorization flow.
- During migration windows, only one authority epoch is writable for a session at any instant.
- Lease renewal target is before TTL/2; beyond TTL without renewal the runtime MUST self-transition to de-authorized drain-only behavior.
- Epoch values are control-plane-issued fencing tokens and MUST be monotonic for a session; they are not wall-clock-derived.
- If a transport cannot carry epoch metadata end-to-end, RK-22 MUST deterministically enrich authority context at ingress using routing-view validation before event acceptance; after ingress, all authority-relevant events/outputs MUST carry current lease/epoch.

### 8.3 Ordering and cancellation sequencing norms

Ordering guarantees:
- Edge-lane order: for each `(session_id, edge_id, lane)`, runtime emits and records events in strict `runtime_sequence` order.
- Node output order: if one node emits to multiple edges, each edge preserves local order; cross-edge order is defined only by timeline merge rules.
- Turn-global replay order: OR-02 constructs deterministic timeline order from lane priority + `runtime_sequence` + (`merge_rule_id`, `merge_rule_version`) from DeterminismContext.

Sequence fields:
- `transport_sequence`: ingress/media-local sequencing marker from transport/provider domains.
- `runtime_sequence`: authoritative runtime enqueue/emit sequencing marker used for replay determinism.

Merge/drop lineage requirements:
- Merge/coalesce operations MUST include lineage evidence (`merge_group_id`, bounded `merged_from_event_ids`, or equivalent deterministic span metadata).
- Deterministic drops MUST emit `drop_notice` with reason and affected sequence range so replay can distinguish drop/merge from upstream absence.

Cancellation ordering requirements:
- After cancellation is accepted for a scope, subsequent non-fenced DataLane outputs for that scope are invalid.
- Race-arriving outputs after cancellation MUST be marked `late_after_cancel=true` and handled via deterministic late-event policy (drop/fence/diagnostics).
- ControlLane cancellation events MUST appear in timeline order before any later accepted DataLane output for the same scope.

## 9. Runtime correctness contract (module-integrated)

Invariant C1. `ResolvedTurnPlan` immutability per turn.

Invariant C2. Turn boundaries are authoritative and consistent: `turn_open` -> (`commit` | `abort`) -> `close`.

Invariant C3. ControlLane preemption over DataLane/TelemetryLane in all load states.

Invariant C4. Bounded buffering at every edge with deterministic pressure behavior.

Invariant C5. Explicit cancellation scope propagation to node/provider/buffer layers.

Invariant C6. Placement lease epoch checks prevent split-brain outputs/events.

Invariant C7. Replayability is guaranteed by timeline + determinism context + plan hash.

Invariant C8. Stateless runtime claim is preserved by separating disposable runtime memory from declared durable state.

### 9.1 Turn arbitration and lifecycle state model (authoritative in RK-03)

Canonical states:
- `Idle` (no active turn authority in session).
- `Opening` (pre-turn arbitration after `turn_open_proposed`; admission/authority checks running, but no authoritative turn lifecycle has started).
- `Active` (turn accepted and `ResolvedTurnPlan` frozen).
- `Terminal` (`commit` or `abort(reason)` emitted).
- `Closed` (`close` emitted; late-event policy applies).

Boundary rules:
- `turn_open_proposed` transitions session arbitration into `Opening` only; it does not create authoritative turn lifecycle state.
- `Opening` is a session-arbitration state (no authoritative `turn_id` lifecycle yet); authoritative turn lifecycle begins only at `turn_open`.
- A turn is considered started only after RK-03 accepts `turn_open_proposed`, RK-04 emits `ResolvedTurnPlan`, and then RK-03 emits authoritative `turn_open`.
- If admission rejects or defers before acceptance, runtime emits admission/capacity outcomes without creating an active turn and without emitting `abort`/`close`.
- If a turn has been accepted and later rejected by hard authority loss or policy boundary, runtime emits `abort(reason)` then `close`.
- Exactly one terminal outcome is legal per accepted turn.
- Late events after `Closed` are handled by Turn Arbitration Rules (drop, map to next turn, or diagnostics-only) and MUST be deterministic per session policy.

## 10. State and failure model

### 10.1 State classes and module owners

- Turn-ephemeral state: RK-07/RK-08; disposable on abort or migration.
- Session-hot state: RK-20; may reset on migration/failover.
- Session-durable state: RK-20 with external persistence; reattached by RK-01.
- Control-plane config state: CP-01/CP-04/CP-05/CP-06/CP-09.
- Fleet-scoped provider health/circuit state: CP-10; published as versioned snapshots for turn-start freezing and runtime fast-path consumption.

### 10.2 Failure classes and deterministic handling owner

F1. Admission overload:
- Owner: CP-05 + RK-25.
- Outcome: reject/defer/shed with explicit control signal at the applicable scheduling point.

F2. Node timeout/failure:
- Owner: RK-08 + RK-17 + RK-03.
- Outcome: degrade/fallback/abort from plan.

F3. Provider failure/overload:
- Owner: RK-11 + CP-04 + CP-10.
- Outcome: retry/switch/fallback/abort from plan.

F4. Edge pressure overflow:
- Owner: RK-12/RK-13/RK-14.
- Outcome: deterministic shedding or bounded blocking per plan.

F5. Sync-coupled partial loss:
- Owner: RK-15.
- Outcome: atomic drop or discontinuity-based recovery.

F6. Transport disconnect/stall:
- Owner: RK-23 + RK-03.
- Outcome: abort/drain/cleanup rules.

F7. Placement authority conflict:
- Owner: CP-07 + RK-24.
- Outcome: deterministic authority outcomes (`stale_epoch_reject`) and, when authority is revoked mid-session, de-authorized drain behavior (`deauthorized_drain`).

F8. Region failover:
- Owner: CP-07/CP-08 + RK-01/RK-20.
- Outcome: authority migration, durable reattach, hot-state reset allowed.

## 11. Section 4 abstraction ownership matrix

1. PipelineSpec -> CP-01/CP-02.
2. ExecutionProfile -> CP-01/CP-02.
3. GraphDefinition -> CP-03.
4. ResolvedTurnPlan -> RK-04.
5. Session -> RK-01.
6. Turn -> RK-03.
7. Turn Arbitration Rules -> RK-03.
8. Event ABI (Schema + Envelope) Contract -> RK-05.
9. Event -> RK-05/RK-06.
10. Lane -> RK-06.
11. Node -> RK-08.
12. NodeExecutionContext -> RK-08.
13. Preemption Hooks Contract -> RK-08/RK-16.
14. Edge -> RK-07/RK-12.
15. BufferSpec -> RK-12.
16. Watermarks -> RK-13.
17. Timebase Contract -> RK-18.
18. CancellationScope + CancelToken -> RK-16.
19. Budget -> RK-17.
20. Determinism Contract -> RK-19.
21. DeterminismContext -> RK-19.
22. Replay & Timeline Contract -> OR-02/OR-03.
23. Provider Contract -> RK-10.
24. ProviderInvocation -> RK-11.
25. External Node Boundary Contract -> RK-09.
26. Control Plane Contract -> CP-01..CP-10.
27. PlacementLease (Epoch / Lease Token) -> CP-07/RK-24.
28. Routing View / Registry Handle -> CP-08.
29. State Contract (Hot vs Durable) -> RK-20.
30. Identity / Correlation / Idempotency Contract -> RK-21.
31. Transport Boundary Contract -> RK-22.
32. ConnectionAdapter -> RK-23.
33. Runtime Contract -> RK-24 (enforced) + all runtime modules including RK-25/RK-26 (implemented).
34. Policy Contract -> CP-04.
35. Resource & Admission Contract -> CP-05 (policy authority) + RK-25 (runtime enforcement).
36. Security & Tenant Isolation Contract -> CP-06 (policy authority) + RK-05/RK-08/RK-09/RK-22/OR-01/OR-02/OR-03 (runtime and observability-path enforcement).

### 11.1 Derived artifact ownership matrix

- `AllowedAdaptiveActions` -> CP-04 (definition), RK-04 (freeze), OR-02 (recording)
- `RecordingLevelPolicy` -> CP-01/CP-02 (governance/defaulting), OR-02/OR-03 (enforcement)
- `AbiPolicyRegistry` -> CP-01 (publication/deprecation), RK-05 (validation)
- `AbiCompatibilitySnapshot` -> CP-01/CP-08 (distribution), RK-05 (local enforcement), RK-04 (turn-start provenance freeze), OR-02 (recording)
- `AdmissionPolicySnapshot` -> CP-05/CP-08 (publication), RK-25 (hot-path enforcement), RK-04 (turn-start provenance freeze), OR-02 (recording)
- `RoutingViewSnapshot` -> CP-08 (publication/distribution), RK-04/RK-24 (turn-start and authority consumption), OR-02 (recording)
- `VersionResolutionSnapshot` -> CP-09/CP-08 (publication/distribution), RK-04 (turn-start consumption), OR-02 (recording)
- `PolicyResolutionSnapshot` -> CP-04/CP-08 (publication/distribution), RK-04 (turn-start consumption), OR-02 (recording)
- `ProviderHealthSnapshot` -> CP-10 (aggregation/publication), CP-04/RK-11/RK-04 (consumption and turn-start provenance freeze), OR-02 (recording)
- `MergeRuleIdentity` -> RK-19 (issuance), OR-02 (evidence), OR-03 (divergence evaluation)

## 12. Requirement and non-goal traceability matrix

R1 -> CP-03, RK-07, RK-08, RK-19.

R2 -> RK-05, RK-06, RK-21, RK-22.

R3 -> RK-03, RK-08, RK-11, RK-16.

R4 -> RK-12, RK-13, RK-14, RK-17, RK-25.

R5 -> CP-04, RK-10, RK-11.

R6 -> OR-01, OR-02, OR-03, DX-03.

R7 -> RK-05 + RK-24 + OR-01 + DX-05 (overhead guardrails, invariant enforcement, runtime telemetry, and SLO operations).

R8 -> RK-02 + CP-04 + RK-04 (policy-selected turn-detection primitives frozen into turn execution).

R9 -> CP-02 + RK-12/RK-14 defaults.

R10 -> RK-01/RK-20/RK-26 with disposable runtime boundaries.

R11 -> CP-07/CP-08 + RK-24 epoch enforcement.

R12 -> RK-22/RK-23.

R13 -> CP-01/CP-05/CP-07/CP-08/CP-09/CP-10 + RK-09 + RK-25.

R14 -> DX-01..DX-06.

NG1 -> RK-22/RK-23 explicit no-ownership boundary.

NG2 -> No planner/memory/tool orchestration module in core; business orchestration remains node-level.

NG3 -> RK-10/RK-11 normalize runtime behavior only, not model quality guarantees.

NG4 -> Business integrations remain custom node responsibility in RK-08.

NG5 -> Provider adapters integrate external model APIs; they do not replace them.

### 12.1 ExecutionProfile behavior by mode

Simple mode (default onboarding path):
- Required inputs: minimal `PipelineSpec`, node references, provider configuration.
- CP-02 expands omitted fields using profile defaults for buffer policy, budgets, flow-control mode, fallback posture, and lane handling.
- RK-04 freezes those defaults into `ResolvedTurnPlan` so runtime behavior is explicit and replayable.

Advanced mode (fine-grained control path):
- Developers can override per-edge BufferSpec, SyncDropPolicy, Watermarks, FlowControlMode, and lane behavior within contract limits.
- Policy/admission/security constraints remain mandatory and are still validated by CP-02/CP-04/CP-05/CP-06.
- RK-24 rejects turn execution if overrides would violate lane priority, determinism, or non-goal boundaries.

### 12.2 Buffer and flow-control defaults by lane/profile

| Lane | Simple default (profile-applied) | Advanced override envelope |
| --- | --- | --- |
| ControlLane | Minimal buffering, non-blocking priority path, deterministic delivery intent | Buffer tuning allowed only if control preemption guarantees are preserved |
| DataLane | Bounded queue, `signal` flow-control mode, deterministic shedding before indefinite blocking | Can select `signal`/`credit`/`hybrid`, custom bounds/strategies, but must set `max_block_time` if blocking is allowed |
| TelemetryLane | Best-effort sampling/drop under pressure | Can tune sampling/drop rates; cannot block control/data critical paths |

Hard profile rules:
- Every edge MUST have effective BufferSpec (explicit or profile-derived).
- Low-latency profiles MUST enforce no-hidden-blocking (`max_block_time` + deterministic shedding path).
- Watermark thresholds and resulting actions MUST be frozen into `ResolvedTurnPlan` at turn start.

### 12.3 Modality-aware buffering guidance

Audio frame paths:
- Prefer coalesce/merge behavior over frame drop under pressure.
- If drop is unavoidable, runtime MUST emit discontinuity for affected audio sync domains and downstream reset behavior is required.

ASR interim text paths:
- `latest-only` is acceptable for interim deltas when explicitly declared.

TTS output paths:
- Barge-in/cancel must flush buffered audio and emit delivery-side cancellation signals (`playback_cancelled`).

## 13. Integration sequence (normative)

### 13.1 Turn start

1. RK-02 proposes non-authoritative `turn_open_proposed`.
2. Runtime consumes latest local snapshots/artifacts (`RoutingViewSnapshot`, `AdmissionPolicySnapshot`, `AbiCompatibilitySnapshot`, `VersionResolutionSnapshot`, `PolicyResolutionSnapshot`, `ProviderHealthSnapshot`) distributed asynchronously by CP-08 (routing/admission/ABI/version/policy snapshots) and CP-10 (provider-health snapshots).
3. RK-25 applies local admission/scheduling enforcement and RK-24 performs local authority epoch validation.
4. If admission/snapshot data are stale, missing, unavailable, or version-incompatible, RK-25 emits deterministic `defer`/`reject` outcomes (per configured policy) and RK-03 does not open a turn.
5. If authority validation fails (stale/invalid epoch), RK-24 emits deterministic authority-reject outcomes and RK-03 does not open a turn.
6. If step 3 yields an admissible and authority-valid candidate, RK-04 emits immutable `ResolvedTurnPlan` with plan hash and snapshot provenance references from normalized spec + local snapshots.
7. RK-03 emits authoritative `turn_open` only after successful step 6 materialization.
8. RK-07/RK-26 start turn-scoped graph execution.

### 13.2 Barge-in/cancel

1. RK-06 prioritizes cancel signal in ControlLane.
2. RK-16 plus RK-22/RK-23 apply immediate output fencing and egress queue flush for the cancelled scope.
3. RK-16 propagates cancellation by scope to RK-08/RK-11/RK-09.
4. RK-11 and RK-09 attempt provider/external cancellation and emit observability markers (`cancel_sent_at`, `cancel_ack_at`, `late_provider_output_dropped_count`).
5. RK-03 emits `abort(reason)` and `close`.

### 13.3 Commit and delivery

1. RK-07/OR-02 append terminal turn evidence to OR-02 Stage-A local append log: final DataLane output if present, otherwise a control-only terminal marker.
2. RK-03 emits `commit` only after step 1 completes.
3. No further DataLane outputs are emitted for that turn.
4. Delivery completion is tracked independently through control signals (`output_accepted`, `playback_started`, `playback_completed`, `playback_cancelled`).
5. RK-03 emits `close` after lifecycle finalization.

### 13.4 Migration and failover

1. CP-07 rotates lease epoch and CP-08 publishes updated routing view.
2. Old placement enters drain-only mode and stops accepting new turn activation.
3. New placement validates authority epoch, reattaches session-durable state through RK-20, and reconstructs required session context via RK-01.
4. Session-hot and turn-ephemeral state are allowed to reset; policy/runtime may trigger deterministic degrade/fallback on first turn after migration.
5. RK-24 rejects stale-epoch outputs from the old placement and records migration markers for replay and audits.

### 13.5 External node execution boundary

1. RK-07 dispatches an external-node step to RK-09 under `ResolvedTurnPlan` constraints.
2. RK-09 injects identity/authz/tenant context and least-privilege credentials from CP-06 policy.
3. RK-09 enforces timeout/resource/cancel contracts and emits normalized outcome classes compatible with RK-11.
4. RK-16 cancellation propagates through RK-09 with observable markers; output fencing remains enforced by RK-22/RK-23.
5. OR-02 records external-boundary invocation lineage and outcomes for replay parity with provider invocations.

## 14. Acceptance gate for section 5 insertion

A section-5 integration is accepted only if:

1. All 36 section-4 abstractions map to explicit module ownership.
2. All section-1/2/3 requirements map to modules and sequences.
3. All non-goals are enforced as explicit module boundaries.
4. `ResolvedTurnPlan` is the only term for turn-frozen plan.
5. Turn intent/boundary naming is consistent (`turn_open_proposed` non-authoritative; `turn_open`, `commit/abort`, `close` authoritative).
6. Simple mode defaults are explicit and deterministic.
7. Stateless runtime wording is reconciled with durable state externalization.
8. Multi-region single-authority semantics are enforced by lease epoch checks.
9. ControlLane/DataLane/TelemetryLane signal ownership is explicitly mapped to modules.
10. Replay outputs include divergence classes with evidence references.
11. Ordering guarantees and sequence field semantics are explicitly defined.
12. Cancellation hard guarantee is output fencing, with provider cancel attempts fully observable.
13. Recording level policy (L0/L1/L2) is declared by ExecutionProfile with replay/cost/security implications.
14. Derived artifacts (`AllowedAdaptiveActions`, `RecordingLevelPolicy`, `AbiPolicyRegistry`, `RoutingViewSnapshot`, `AbiCompatibilitySnapshot`, `AdmissionPolicySnapshot`, `VersionResolutionSnapshot`, `PolicyResolutionSnapshot`, `ProviderHealthSnapshot`, `MergeRuleIdentity`) have explicit ownership and replay visibility.
15. Control-plane policy evaluation is snapshot-distributed; runtime hot-path enforcement is local (no per-event control-plane dependency).
16. `turn_open_proposed` is runtime-internal intent and not a stable external client-facing signal.
17. External node execution boundaries are represented in both ownership mapping and integration sequences (RK-09 with CP-06 policy constraints).

## 15. Reference repository layout (non-normative)

```text
RealtimeSpeechPipeline/
├── cmd/
│   ├── rspp-control-plane/              # CP service entrypoint
│   ├── rspp-runtime/                    # RK service entrypoint
│   ├── rspp-local-runner/               # DX-01 local runner entrypoint
│   └── rspp-cli/                        # DX-04 release/rollout CLI
├── api/
│   ├── eventabi/                        # Event ABI schemas and versioning contracts
│   ├── controlplane/                    # Admission/placement/policy service APIs
│   └── observability/                   # Telemetry/replay APIs
├── pkg/
│   └── contracts/                       # Stable public contract types (spec/event/plan/identity)
├── internal/
│   ├── controlplane/                    # CP-01..CP-10
│   │   ├── registry/                    # CP-01 Pipeline Registry
│   │   ├── normalizer/                  # CP-02 Spec Normalizer and Validator
│   │   ├── graphcompiler/               # CP-03 GraphDefinition compiler
│   │   ├── policy/                      # CP-04 Policy Evaluation
│   │   ├── admission/                   # CP-05 Resource & Admission
│   │   ├── security/                    # CP-06 Security & Tenant Isolation
│   │   ├── lease/                       # CP-07 Placement Lease Authority
│   │   ├── routingview/                 # CP-08 Routing View Publisher
│   │   ├── rollout/                     # CP-09 Rollout and Version Resolver
│   │   └── providerhealth/              # CP-10 Provider Health Aggregator
│   ├── runtime/                         # RK-01..RK-26
│   │   ├── session/                     # RK-01 Session Lifecycle
│   │   ├── prelude/                     # RK-02 Session Prelude
│   │   ├── turnarbiter/                 # RK-03 Turn Arbiter
│   │   ├── planresolver/                # RK-04 ResolvedTurnPlan resolver
│   │   ├── eventabi/                    # RK-05 Event ABI Gateway
│   │   ├── lanes/                       # RK-06 Lane Router
│   │   ├── executor/                    # RK-07 Graph Runtime Executor
│   │   ├── nodehost/                    # RK-08 Node Runtime Host
│   │   ├── externalnode/                # RK-09 External Node Boundary Gateway
│   │   ├── provider/
│   │   │   ├── contracts/               # RK-10 provider capability/error normalization contracts
│   │   │   ├── registry/                # RK-10 adapter discovery/binding (no provider-specific logic)
│   │   │   └── invocation/              # RK-11 Provider Invocation Controller
│   │   ├── buffering/                   # RK-12 Edge Buffer + RK-13 Watermark
│   │   ├── flowcontrol/                 # RK-14 Flow Control Engine
│   │   ├── sync/                        # RK-15 Sync Integrity Engine
│   │   ├── cancellation/                # RK-16 Cancellation Manager
│   │   ├── budget/                      # RK-17 Budget Manager
│   │   ├── timebase/                    # RK-18 Timebase Service
│   │   ├── determinism/                 # RK-19 Determinism Service
│   │   ├── state/                       # RK-20 State Access Layer
│   │   ├── identity/                    # RK-21 Identity/Correlation/Idempotency
│   │   ├── transport/                   # RK-22/RK-23 boundary orchestration contracts (no transport-specific implementation)
│   │   ├── guard/                       # RK-24 Runtime Contract Guard
│   │   ├── localadmission/              # RK-25 Local Admission and Scheduling Enforcer
│   │   └── executionpool/               # RK-26 Execution Pool Manager
│   ├── observability/                   # OR-01..OR-03
│   │   ├── telemetry/                   # OR-01 Telemetry Pipeline
│   │   ├── timeline/                    # OR-02 Event Timeline Recorder
│   │   └── replay/                      # OR-03 Replay Engine
│   ├── tooling/                         # DX-02..DX-06 runtime-adjacent tooling
│   │   ├── validation/                  # DX-02 Spec lint/contract tests
│   │   ├── regression/                  # DX-03 Replay regression harness
│   │   ├── release/                     # DX-04 Release and rollout operations
│   │   ├── ops/                         # DX-05 SLO dashboards/alerts integration
│   │   └── authoring/                   # DX-06 Node Authoring Kit scaffolds/tests
│   └── shared/
│       ├── config/                      # Profile/default loading and configuration guards
│       ├── errors/                      # Normalized error/outcome taxonomy
│       ├── types/                       # Internal type aliases and envelopes
│       └── util/                        # Non-domain utility helpers
├── providers/                           # Provider-specific adapter implementations
│   ├── stt/
│   ├── llm/
│   └── tts/
├── transports/                          # Transport-specific mapping implementations
│   ├── livekit/
│   ├── websocket/
│   └── telephony/
├── pipelines/                           # Versioned PipelineSpec + ExecutionProfile artifacts
│   ├── specs/
│   └── profiles/
├── deploy/
│   ├── k8s/
│   ├── helm/
│   └── terraform/
├── test/
│   ├── contract/                        # Contract conformance tests
│   ├── integration/                     # End-to-end runtime/control-plane/transport tests
│   ├── replay/                          # Determinism and replay suites
│   ├── load/                            # Latency/backpressure/cancel stress suites
│   └── failover/                        # Lease epoch and migration/failover tests
└── docs/
    ├── rspp_SystemDesign.md
    ├── ModularDesign.md
    └── runbooks/
```

## 16. Appendices (implementation guardrails)

### Appendix A: Event ordering and cancellation norms

Normative source sections:
- Ordering levels and sequence fields: section `8.3`.
- Cancellation tiers and observability: section `5`, RK-16.
- Late-event marking and deterministic handling: sections `8.3` and `9.1`.

### Appendix B: Recording levels and replay guarantees

Normative source sections:
- Recording levels and supported replay guarantees: section `6.1`.
- Divergence classes and evidence model: section `6`, OR-02/OR-03.
- Cost/security constraints and profile declaration requirements: sections `6.1` and `12.1`.
