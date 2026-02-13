# RSPP Voice Runtime Framework — Report (Transport-Agnostic, with LiveKit as an Optional Integration)

> Goal: step beyond the current implementation constraints and redefine RSPP as a **foundational runtime framework for voice agents**—the way **NVIDIA Triton** provides a kernel and contract for inference.
> Developers implement business logic (Nodes / Policies / custom operators); the framework guarantees **low latency, controllability, observability, and swap-ability**.
> LiveKit is an optional integration path, not a platform dependency.

---

## 1) Framework Positioning & Value Proposition

### 1.1 One-liner
**RSPP is the infrastructure framework for real-time voice AI teams to ship agents without rebuilding streaming, scaling, and reliability primitives.**

RSPP = Real-time Speech Pipeline Platform.
Definition: "RSPP is the infrastructure runtime that executes real-time speech pipelines (ASR/LLM/TTS) with low latency, cancellation, and observability built in."

**Value proposition (for product + development teams)**
- Saves teams weeks of infra work for real-time voice AI.
- Latency-first runtime semantics (budgets, backpressure, cancellation).
- Provider/model swap-ability without rewriting business logic.
- Observability and replay built in (traces/metrics/logs/events).
- Kubernetes-native: stateless runtime, horizontal scaling, portable deployment.
- Multi-region routing and failover as a platform capability.

### 1.2 Positioning in the Stack (vendor-agnostic)
- **Not a voice SDK** (e.g., Vapi/Retell) and **not an RTC platform** (e.g., LiveKit/Twilio).
- **Not a model API** (e.g., OpenAI/Google/Anthropic).
- **RSPP sits between** transport and models: it is the runtime kernel that executes streaming speech graphs with guarantees and operational tooling.
- LiveKit can be one transport integration, but the same runtime contract applies over WebSocket/telephony and other transport layers.

### 1.3 Core User Journey (minimal example)
- Journey: "A client streams audio → ASR → LLM → TTS → audio back."
- Where RSPP sits: "RSPP runs the pipeline between the transport layer (LiveKit/WebSocket/telephony) and model providers, handling routing, budgets, and cancellation."

---

## 2) What the Framework Can / Cannot Do

### 2.1 What RSPP *can* do (capability boundary)

1) **Executable real-time speech graphs**
- Streaming partial/final events, multi-output, fan-out/fan-in (fan-out = one->many, fan-in = many->one (merge/select)), and fallback paths
- Deterministic routing/ordering as part of the runtime contract

2) **Unified event-driven ABI**
- All nodes exchange a stable Event stream (audio/text/control/metrics/debug)
- Versioned schema for compatibility and replay

3) **Cancellation-first runtime**
- Barge-in/interrupt propagates end-to-end: stop downstream generation/output, cancel in-flight calls, flush queues

4) **Backpressure & budgets**
- Bounded queues with explicit strategies (block/drop/merge)
- Node-level budgets with timeout/fallback/degrade behaviors

5) **Pluggable providers**
- Policy-driven adapters for STT/LLM/TTS/translation
- Swap providers/models without rewriting business nodes

6) **Observability + replay by default**
- Per node/edge latency, queue depth, drops, cancels, provider error rates
- Session/turn/pipeline correlation; event timelines for replay/evals

7) **Low-overhead runtime**
- Low-overhead Event ABI designed to add minimal latency; overhead is measured and budgeted

8) **Turn-detection primitives**
- VAD and turn-detection are first-class and swappable
- Kernel-managed primitive with a default implementation, represented as a node type and swappable via policy

9) **Simple mode**
- Opinionated defaults and templates to reduce developer cognitive load
- Configuration design is a first-class concern (concise, readable, and scalable)
- Minimal required fields; advanced knobs are opt-in
- In practice, Simple mode is a predefined execution profile: developers provide minimal pipeline artifacts while the framework supplies default budgets, queue behavior, and fallback posture.

10) **K8s-native, stateless runtime**
- Horizontal scaling with pooled/multiplexed execution
- Adaptive scheduling / shared execution pools (dynamic batching as a possible optimization)

11) **Multi-region routing and failover**
- Active-active at the fleet level (sessions distributed across regions), with single-authority execution per session enforced via placement leases/epochs.
- Multi-region routing means the control plane assigns sessions to the lowest-latency healthy region based on policy, with failover to a secondary region if capacity or health degrades.

12) **Transport-agnostic orchestration**
- Same graph runs over RTC platforms, WebSockets, or telephony gateways without rewriting orchestration logic

13) **Control-plane basics + external nodes**
- Pipeline versioning + rollout policies
- External node execution supported via isolation boundaries (sandbox/WASM later)

### 2.2 What RSPP *does not* do (non-goals / explicit boundaries)

1) **Not a real-time media transport stack**
- Does not implement SFU/WebRTC/ICE/TURN, rooms/participants, or track routing
- Use RTC platforms such as LiveKit, Mediasoup, or equivalent transport layers

2) **Not a voice SDK / agent framework**
- RSPP enables shipping agents, but is not an agent SDK
- RSPP is a runtime kernel, not a code-first agent SDK (e.g., Pipecat, LiveKit Agents)
- Planner/memory/tool orchestration can be built as nodes or upstream logic

3) **Does not guarantee model quality**
- Guarantees runtime semantics and operational properties, not accuracy

4) **Does not own business-system integrations**
- CRM/ticketing/payments/KB integrations belong to application nodes

5) **Not a model API**
- Works with model providers rather than replacing them
- RSPP is not a model provider (for example OpenAI, Anthropic, Google)

---

## 3) How Developers Use It (Developer Experience)

RSPP is **spec-first**: developers define pipelines as versioned artifacts, while the runtime owns the hard parts (latency, cancellation, backpressure, observability). The control plane manages pipeline registry/versions/rollouts and session routing; the runtime kernel executes session graphs.

### 3.1 What developers deliver

1) **PipelineSpec (primary artifact)**
- Declaratively defines nodes, edges, budgets, queue strategies, and fallbacks
- Auditable, versioned, and portable across environments

2) **Nodes**
- Implement business logic: input Event stream → output Event stream
- Examples: VAD, ASR, LLM, Segmenter, TTS, Translator, Compliance, Custom tool bridge

3) **Policies (optional)**
- Provider selection, degradation rules, circuit-breakers, cost controls, tenant/region/language routing

Minimal artifact set to run a pipeline: PipelineSpec + referenced Nodes (built-in or custom) + provider configuration (API keys/endpoints).

Ownership split (who defines what):
- Developers define: PipelineSpec, custom nodes, and optional policy intent.
- Platform/control plane defines: version resolution, rollout decisions, admission, placement/routing, and migration.
- Runtime enforces: cancellation, backpressure, budgets, and observability semantics over the combined inputs.

### 3.2 How developers run and ship

1) **Quickstart (spec-first)**: pick a template PipelineSpec → configure providers → run the **local runner** (CLI that starts a single-node runtime plus a minimal control-plane stub) → connect via LiveKit/RTC/WebSocket transport → see streaming outputs + metrics  
2) **Integration**: connect your client to a transport endpoint (LiveKit room/WebSocket/telephony gateway) using a session token/route from the control plane; the transport adapter maps audio/events into the runtime, and RSPP executes the PipelineSpec.  
3) **Release**: publish PipelineSpec + nodes → apply rollout policies (short mention)  
4) **CI/CD**: lint/validate specs + replay tests before deploy (short mention)  
5) **Operate**: observe p95/p99, cancel latency, drops, provider error rates → tune budgets & strategies  

### 3.3 What developers care about (the promises you must keep)

- **“I only write business nodes—everything else is handled by the framework.”**
  - budgets/backpressure/cancellation/resource limits
  - observability by default
  - provider swap-ability
  - toolchain for debugging/regression/evals
  - scale and deploy voice AI agents reliably using the framework

---

## 4) Core Abstractions (no implementation details)

RSPP is defined by a small set of **stable primitives** (what application teams build against) plus **cross‑cutting contracts** (what the runtime must guarantee). This section is intentionally transport‑agnostic and provider‑agnostic.

---

### 4.1 Stable objects and contracts

#### 4.1.1 Specs, plans, and execution surfaces

1) **PipelineSpec**  
A versioned, declarative artifact that defines: graph topology, node types, edge types, buffering/queue policy, budgets, fallback/degradation intent, and observability hooks. PipelineSpec is the *source of truth* for what “can run.”  

2) **ExecutionProfile**  
A named profile that defines default runtime semantics (e.g., **Simple** vs **Advanced**) including default budgets, default buffering strategies, and which knobs are opt‑in. ExecutionProfile is applied during spec normalization so behavior is explicit and replayable.

3) **GraphDefinition**  
The canonical directed streaming dataflow implied by a normalized PipelineSpec for a given pipeline version. It defines legal paths and declared semantics (fan‑out/fan‑in, merge rules, fallback graph, terminal outcomes).

4) **ResolvedTurnPlan** (turn-scoped)  
A *turn‑scoped*, immutable plan produced at turn start by combining:  
- PipelineSpec (normalized)  
- Control-plane decisions (placement, resolved pipeline version, admission outcome)  
- Policy evaluation results (provider binding, degrade posture)  
- Provider/transport capability snapshots (as of turn start)  
ResolvedTurnPlan freezes provider bindings, budgets, edge buffering policies, determinism rules, and snapshot provenance references **for the duration of the turn**.

SessionPreludeGraph (optional)  
A session-scoped subgraph that runs continuously and may emit runtime-internal turn boundary intent signals (for example `turn_open_proposed`) from continuous inputs (for example VAD + turn detection). Its outputs feed Turn Arbitration Rules and are outside turn-scoped work accounting.

---

#### 4.1.2 Runtime scopes

5) **Session**  
A logical conversation context with stable identity and policy scope (tenant/region/language). A session may migrate across runtimes/regions, but remains logically continuous via correlation identities and leases/epochs (see 4.1.8).

6) **Turn**  
The smallest conversational work unit from user-intent intake to assistant output completion (or cancellation). Turns have explicit lifecycle and a single terminal outcome:  
- `commit`: successful completion for that turn  
- `abort`: terminated before commit (reason: cancel, timeout, runtime error, authority loss, disconnect, policy termination, etc.)  
A `close` marker is emitted after terminal outcome to finalize lifecycle.
`commit` indicates the runtime has completed generation for the turn and will emit no further DataLane outputs for that turn. Delivery completion (e.g., playback finished / transport accepted) is represented separately via ControlLane delivery signals. `commit` is also valid for control-only turns (no DataLane output) when terminal control outcomes are recorded and no further DataLane output will follow. In this context, "recorded" means appended to the runtime's non-blocking local timeline append path; durable export/persistence remains asynchronous.
A turn begins when the runtime accepts a `turn_open` ControlLane event, typically emitted after a non-authoritative `turn_open_proposed` intent from session-scoped detection and/or transport boundary signals.
`turn_open_proposed` MAY be emitted as a non-authoritative runtime-internal intent signal; it does not create turn lifecycle state and is not a stable external client-facing signal.
`turn_open` acceptance is valid only when a ResolvedTurnPlan has been successfully materialized for that turn.
`turn_open` acceptance also requires current authority validation (valid lease/epoch); stale or de-authorized authority outcomes at this pre-turn gate MUST remain pre-turn and MUST NOT emit `abort` or `close`.
If authority is revoked after a turn is already `Active`, runtime emits `deauthorized_drain`, then `abort(reason=authority_loss)`, then `close`.
Admission outcomes (`admit/reject/defer`) before `turn_open` are pre-turn ControlLane outcomes and MUST NOT emit `abort` or `close`.

7) **Turn Arbitration Rules**  
A contract defining whether turns may overlap and how preemption behaves (e.g., barge‑in creates a new turn that can preempt an in‑flight turn). It defines late-event handling (events arriving after abort), grace windows, and which terminal semantics are authoritative within a session.

---

#### 4.1.3 Event model and lanes

8) **Event ABI (Schema + Envelope) Contract**  
The versioned ABI for all RSPP events. It defines:  
- Envelope fields (session_id, turn_id (optional; required for turn-scoped events), pipeline_version, edge_id/node_id, event_id, causal_parent_id, idempotency_key, lane, sequence markers including transport_sequence/runtime_sequence, sync_id (optional), sync_domain (optional), discontinuity_id (optional), merge_group_id (optional), merged_from_event_ids (optional, bounded), late_after_cancel (optional))  
- Timestamp fields (see Timebase in 4.1.5)  
- Compatibility rules (schema evolution, version negotiation, deprecation)  
- Ordering and dedupe expectations (per lane/per edge)  

9) **Event**  
The universal, immutable exchange unit. Events carry **data** (audio/text), **control** (cancel/budget/turn boundaries), and **telemetry** (metrics/traces/debug) but all share the same ABI envelope for correlation and replay.

10) **Lane** (QoS class for events)  
RSPP classifies events into lanes with explicit priority and buffering semantics:  
- **DataLane**: audio frames, text deltas, semantic content outputs  
- **ControlLane**: turn intent/boundaries, cancellation, budgets, admission, routing/migration, connection state  
- **TelemetryLane**: metrics, traces, logs, debug snapshots (best-effort).  
Lanes share the same Event ABI but may use different buffering strategies and drop policies to protect low latency.

---

#### 4.1.4 Graph building blocks: Nodes, edges, buffers

11) **Node**  
A typed event processor: `in(EventStream) → out(EventStream)` subject to runtime constraints. Business logic lives in nodes. Nodes must declare:  
- supported input/output event types  
- streaming behavior (incremental/final)  
- cancel responsiveness (preemptible vs cooperative)  
- resource characteristics (CPU/GPU/network)  
- state usage (hot vs durable)  
- execution scope (session-scoped or turn-scoped)
Session-scoped nodes run continuously within a session (including outside active turns) and may emit ControlLane turn boundary signals. Turn-scoped nodes run only within an active turn and MUST assume a valid `turn_id`.

12) **NodeExecutionContext** (session- or turn-scoped)  
A runtime-provided context injected into every node invocation, containing:  
- `turn_id` (optional): absent in session-scoped execution  
- `CancelToken` and cancel scope (see 4.1.5)  
- `BudgetHandle` (remaining time/capacity, degrade triggers)  
- `TimebaseHandle` (timestamping and latency measurement)  
- `StateHandle` (turn-ephemeral, session-hot, session-durable state access)  
- `TelemetryHandle` (non-blocking emission to TelemetryLane)  
- `DeterminismContext` (required for turn-scoped execution; optional in session scope; see 4.1.6)
If `turn_id` is absent, the context is session-scoped and MUST NOT allow access to turn-ephemeral state. StateHandle MUST enforce this scope boundary. If `DeterminismContext` is present in session scope, it is session-level only and MUST NOT claim turn-level replay guarantees.

13) **Preemption Hooks Contract**  
Nodes must implement lifecycle hooks that enable fast interruption when required by policy/runtime:  
- `on_cancel()` for immediate stop + cleanup  
- `on_barge_in()` for flushing buffered output (e.g., TTS audio)  
- `on_budget_exhausted(outcome)` for deterministic fallback/degrade behaviors  
This is a contract requirement for nodes that buffer or generate long-running streams.

14) **Edge**  
A typed stream channel connecting nodes with explicit **ordering**, **buffering**, and **QoS** semantics. Edges are the *buffer boundaries* where latency can accumulate; therefore edges must declare buffering policy and watermarks (below).

15) **BufferSpec** (an Edge property)  
A declarative buffer/queue policy attached to an edge, defining:  
- bounds (max items / max bytes / max time in-queue / max latency contribution)  
- strategy on pressure (block, drop, merge/coalesce, sample, latest-only)  
- lane handling (how DataLane vs ControlLane vs TelemetryLane behave on this edge)  
- fairness key (e.g., session/turn/tenant) if edge participates in shared pools  
- defaulting source (explicit edge config vs ExecutionProfile defaults)
In **Simple** mode, an edge may omit explicit BufferSpec fields when the selected ExecutionProfile supplies deterministic defaults; **Advanced** mode can override per edge.

SyncDropPolicy (optional)  
Declares how drops behave for synchronized data streams:
- `group_by`: envelope field used for atomic grouping (default: `sync_id`)
- `policy`: `atomic_drop`, `drop_with_discontinuity`, or `no_sync`
- `scope`: `edge_local` or `plan_wide`
If `policy=atomic_drop`, dropping any event in a group on an edge MUST drop all events in that group for that edge. If `policy=drop_with_discontinuity`, runtime MUST emit `discontinuity(sync_domain, discontinuity_id, reason)` ControlLane signals so downstream can reset and recover.
If cross-edge synchronization is required, the ResolvedTurnPlan MUST declare `scope=plan_wide` and propagate discontinuity markers to all affected edges in the sync domain.

FlowControlMode
Declares how this edge applies flow control under load:
- `signal`: producer may send until receiving `flow_xoff`; resumes on `flow_xon`
- `credit`: producer MUST hold credits to send; downstream grants credits explicitly
- `hybrid`: signal for coarse control plus optional credit for burst control
ExecutionProfile MUST define default FlowControlMode per lane. Low-latency profiles SHOULD default DataLane to `signal` with deterministic shedding (for example leaky-bucket, merge, or latest-only) rather than unbounded blocking.

16) **Watermarks** (a BufferSpec property)  
High/low watermark thresholds for queue depth and/or time-in-queue. Crossing watermarks emits pressure signals. The resulting actions (block/drop/merge/degrade/fallback) MUST be pre-resolved into the ResolvedTurnPlan at turn start, so runtime behavior remains deterministic and replayable.

---

#### 4.1.5 Time, latency, and cancellation

17) **Timebase Contract**  
A session-scoped contract that unifies time across transports and providers, enabling latency measurement and replay timelines. It defines:  
- **Monotonic runtime time** (for latency accounting and budgets)  
- **Wall-clock time** (for cross-region correlation)  
- **Media time** (audio PTS/DTS or sample-index time)  
- mapping rules between transport timestamps and session timeline, including jitter handling responsibilities at the transport boundary.

18) **CancellationScope + CancelToken**  
A first-class contract for cancellation propagation, with explicit scopes: session-scope, turn-scope, node-scope, and provider-invocation-scope. Cancel tokens must preempt lower-priority work (see lane rules) and define bounded “cancel-latency” expectations for compliant nodes/providers.

19) **Budget**  
A bounded execution contract across time and capacity scopes (turn/node/path/edge). Budget outcomes are explicit and must be recorded as control events: continue, degrade, fallback, terminate (abort).

---

#### 4.1.6 Determinism, replay, and “sources of nondeterminism”

20) **Determinism Contract**  
Defines ordering, merge semantics, fan-in resolution, and fallback terminal-outcome semantics such that execution and replay remain consistent within a turn under a ResolvedTurnPlan. Determinism includes canonical fan-in merge-rule identity/version, resolved into the turn plan and used by replay.

21) **DeterminismContext**  
A turn-scoped context containing:  
- a canonical **random seed** (and any allowed PRNG stream identifiers)  
- stable ordering markers  
- merge_rule_id and merge_rule_version for replay-stable fan-in semantics  
- nondeterministic_inputs[] entries for captured runtime decisions that cannot be recomputed deterministically  
- explicit declarations of allowed nondeterministic inputs (e.g., external calls)  
This context is recorded so replay can reproduce node-internal nondeterminism when re-simulating.

22) **Replay & Timeline Contract**  
Defines how to persist and replay event streams:  
- **EventTimeline**: ordered event log per session/turn/lane  
- **ReplayCursor**: a position in the timeline with deterministic stepping rules  
- **ReplayMode**: re-simulate nodes vs “playback recorded provider outputs” (provider determinism is not assumed), with decision handling modes (replay-decisions or recompute-decisions when deterministic inputs are sufficient)  
Replay must honor lane priorities and turn arbitration rules.
ExecutionProfile MUST declare a recording level (L0/L1/L2) that defines capture fidelity and supported replay modes. All levels MUST record replay-critical control evidence (plan hash, determinism markers, authority markers, terminal outcomes, turn-start snapshot provenance references, and admission/policy/authority decision outcomes that gate turn opening/execution); higher levels add partial/full payload capture under declared cost and security constraints.
Replay/timeline recording semantics MUST separate low-latency local append from asynchronous durable export so timeline persistence cannot block ControlLane progression, cancellation propagation, or turn terminalization.
If timeline pressure forces fidelity reduction, runtime MUST apply deterministic recording-level downgrade behavior while preserving replay-critical control evidence.

---

#### 4.1.7 Providers and external execution boundaries

23) **Provider Contract**  
A normalized capability and behavior contract for STT/LLM/TTS/etc., including:  
- streaming modes (partial/final, chunking expectations)  
- cancellation semantics (what “cancel” means and expected timing)  
- error taxonomy and retryability  
- limits (rate, concurrency, payload sizes)  
- outcome normalization (success, timeout, overload, safety block, etc.)

24) **ProviderInvocation**  
A single provider call with explicit identity, idempotency semantics, cancellation scope, and outcome events. ProviderInvocation is the unit recorded for observability and replay (as recorded outputs or as a stubbed outcome in simulation).

25) **External Node Boundary Contract**  
Defines isolation and trust boundaries for externally executed nodes (process/VM/WASM later), including: cancellation propagation, terminal outcomes, timeouts, resource limits, and mandatory observability injection.

---

#### 4.1.8 Control plane, routing, multi-region, and state

26) **Control Plane Contract**  
The authoritative contract for: pipeline version resolution, rollout decisions, session admission, placement/routing, and migration actions. It emits explicit decisions the runtime must execute and record.
Control-plane decisions are distributed as versioned snapshots/artifacts to runtime; hot-path enforcement remains runtime-local.

27) **PlacementLease (Epoch / Lease Token)**  
A control-plane-issued token proving current authority to execute a session/turn on a specific runtime placement. Leases have epochs and expiration. Runtime/transport must reject events and outputs that do not carry a valid current lease/epoch to prevent split-brain duplicates. If transport metadata is missing, authority context must be deterministically enriched at ingress before acceptance; after ingress, all authority-relevant events/outputs MUST carry the current lease/epoch.

28) **Routing View / Registry Handle**  
A runtime-usable, read-optimized view of routing/placement decisions (derived from the control plane) used by transport adapters to locate the correct runtime endpoint and to detect/honor migrations quickly.

29) **State Contract (Hot vs Durable)**  
Defines state classes and ownership boundaries:  
- **Turn-Ephemeral State**: per-turn transient runtime state (always disposable)  
- **Session Hot State**: pipeline-internal hot state (e.g., VAD windows, caches, KV-cache) that is *not required* to survive failover and is not cross-region replicated  
- **Session Durable State**: session context state (identity, consent flags, intent slots, conversation memory pointers) that must survive failover and has defined consistency requirements  
- **Control-Plane Configuration State**: pipeline specs, rollouts, policy configs  
- **Fleet-Scoped Provider Health/Circuit State**: control-plane aggregated health/circuit snapshots used by policy and turn-start resolution  
The contract explicitly declares which state may be lost/reset on migration to protect low latency. This reconciles with the **stateless runtime** model: runtime instances remain disposable, while only declared durable state is externalized and reattached on placement changes.

30) **Identity / Correlation / Idempotency Contract**  
Defines canonical identities and keys: session_id, turn_id (required for turn-scoped events), pipeline_version, event_id, provider_invocation_id, plus idempotency rules and dedupe semantics across retries, reconnects, and failovers. Lease epoch is part of correlation to disambiguate concurrent placements.

31) **Transport Boundary Contract**  
Maps transport-domain inputs/outputs to RSPP Events (and back) while preserving non-goals (RSPP is not the transport). It defines required mappings for audio frames, playback control, and connection lifecycle.

32) **ConnectionAdapter**  
A transport-specific adapter that normalizes connection semantics into RSPP control events, including:  
- liveness probes / keepalive interpretation  
- normalized states: connected / reconnecting / disconnected / ended  
- normalized silence vs user-muted vs transport-stalled signals  
- required behavior on disconnect (turn abort rules, drain rules, cleanup)
ConnectionAdapter is a normalization surface only; it does **not** implement transport-stack media responsibilities (SFU/WebRTC/ICE/TURN, rooms/participants, or track routing).

33) **Runtime Contract**  
The invariant set RSPP guarantees: lane-prioritized control preemption, deterministic turn execution under a ResolvedTurnPlan, bounded buffering via BufferSpec/Watermarks, cancellation propagation with explicit scope, and end-to-end observability/replay correlation.

---

#### 4.1.9 Governance, admission, and security contracts

34) **Policy Contract**  
Defines declarative policy intent and evaluation boundaries for provider selection, degradation posture, routing preferences, circuit-break behavior, and cost controls. Policy evaluation may consume fleet-scoped provider health/circuit snapshots. Policy is resolved into the ResolvedTurnPlan at turn start; policy changes apply only at explicit boundaries.

35) **Resource & Admission Contract**  
Defines admission control, concurrency quotas, fairness, shared-pool scheduling, and load-shedding behavior across tenant/session/turn scopes. It specifies authoritative admit/reject/defer outcomes and overload actions that feed ControlLane signals. At minimum, the contract MUST define: (a) the fairness key(s) used for scheduling (tenant/session/turn), (b) overload precedence rules across lanes (ControlLane protected, TelemetryLane best-effort), and (c) the scheduling points where admission and shedding decisions may be applied (edge enqueue, edge dequeue, node dispatch). Control plane publishes the authoritative policy/snapshot; runtime enforces locally at the scheduling points. Direct control-plane admit/defer/reject decisions, when used, are explicit boundary-time outcomes (for example session bootstrap) rather than required hot-path checks. The runtime MUST also define deterministic outcomes for stale/missing/incompatible turn-start snapshots (`defer` or `reject`) and record those outcomes.

36) **Security & Tenant Isolation Contract**  
Defines tenant isolation and security responsibilities across runtime, control plane, and external-node execution: identity/authentication/authorization context propagation, secret handling, data-access constraints, and per-tenant execution plus telemetry isolation. Control plane defines security policy and tenancy constraints; runtime, transport-boundary adapters, external-node gateways, and observability paths enforce those constraints inline. This complements (not replaces) the External Node Boundary Contract and transport non-goals.

---

### 4.2 Core signals (by lane) and rules

All signals are Events that follow the Event ABI, but are classified by lane for priority and buffering semantics.

#### 4.2.1 DataLane signals (progress and content)
- audio frames (with media timestamps)  
- text deltas and final text segments  
- semantic outputs (tool results, structured intents)  
- final assistant audio chunks

**Rule:** DataLane events may be dropped/merged under pressure only if BufferSpec declares a deterministic strategy (e.g., “latest-only” for interim ASR).

#### 4.2.2 ControlLane signals (preemptive)
- turn intent (non-authoritative, runtime-internal): turn_open_proposed  
- turn boundaries: turn_open, commit/abort(reason), close  
- interruption/cancellation: barge-in, stop, cancel(scope)  
- pressure/budget: watermark crossing, budget warnings/exhaustion, runtime-selected degrade/fallback outcomes  
- discontinuity(sync_domain, discontinuity_id, reason): indicates a break in continuity; downstream MUST reset stateful decoding/presentation pipelines for that domain
- drop_notice(edge_id, lane, reason, seq_range, sync_domain?, discontinuity_id?): deterministic drop/merge lineage marker for replay and diagnostics
- flow control: flow_xoff(edge_id, lane, reason), flow_xon(edge_id, lane), credit_grant(edge_id, lane, amount) (credit mode only)
- provider outcomes: normalized errors, circuit-break events, provider switch decisions (as allowed by plan)  
- routing/migration: placement lease issued/rotated, migration start/finish, session handoff markers  
- admission/capacity: admit/reject/defer outcomes, load-shedding decisions  
- authority validation outcomes: stale_epoch_reject, deauthorized_drain  
- connection lifecycle: connected/reconnecting/disconnected/ended, transport silence/stall  
- replay-critical markers (plan hash, determinism seed, ordering markers)
- output delivery signals: output_accepted, playback_started, playback_completed, playback_cancelled
After `cancel(scope)` acceptance, runtime and transport egress queues for that scope MUST be fenced/cleared; new `output_accepted` or `playback_started` signals for that scope are invalid.
`turn_open_proposed` MAY be recorded for replay/debug but is not guaranteed to be surfaced outside runtime boundaries.

**Rule:** ControlLane events have preemptive priority over DataLane and TelemetryLane; runtime must deliver them with minimal buffering and must not allow telemetry/data backpressure to block control delivery.

#### 4.2.3 TelemetryLane signals (non-blocking observability)
- metrics samples (queue depth, drops, cancel latency, provider RTT, p95/p99)  
- traces/spans and correlation markers  
- debug snapshots (selected, rate-limited)  
- non-critical replay annotations and evaluation metadata

**Rule:** TelemetryLane must be safe-by-default under overload (aggressive dropping/sampling) so it cannot add tail latency to DataLane or delay ControlLane.

---

### 4.3 Minimal rule set (what “runtime semantics” means)

- **Plan freezing:** A ResolvedTurnPlan is immutable within a turn; policy/control-plane changes apply only at explicit boundaries (turn boundary or lease epoch change). Flow-control mode and thresholds (watermarks, XON/XOFF boundaries, shedding strategy) MUST be resolved into the ResolvedTurnPlan at turn start.  
- **One terminal outcome per turn:** exactly one of `commit` or `abort` is emitted, followed by `close`.  
- **Lane priority:** ControlLane preempts everything; DataLane is latency-protected; TelemetryLane is best-effort.  
- **Bounded buffering:** Every Edge must have BufferSpec and watermarks (explicit or via profile defaults) so latency accumulation is always observable and controllable.  
- **No hidden blocking:** In low-latency profiles, DataLane edges MUST NOT block indefinitely. If blocking is permitted, BufferSpec MUST declare a bounded `max_block_time` after which deterministic shedding (drop/merge/degrade) occurs.  
- **Cancellation scope:** cancel propagation is explicit and must cancel provider invocations and buffered output generation where applicable. This guarantee includes transport-bound egress buffers (output fencing/flush).  
- **Failover safety:** PlacementLease epochs prevent split-brain outputs; HotState may reset; DurableState survives with declared consistency.  
- **Replayability:** EventTimeline + plan + determinism context define what can be replayed; provider determinism is not assumed unless recorded outputs are used.

## 5) MVP standards
### 5.1 MVP quality gates (decided SLO/SLI targets)

#### 1. Turn-open decision latency:
   - p95 <= 120 ms from `turn_open_proposed` acceptance point to `turn_open` emission.

#### 2. First assistant output latency:
   - p95 <= 1500 ms from `turn_open` to first DataLane output chunk on the happy path.

#### 3. Cancellation fence latency:
   - p95 <= 150 ms from cancel acceptance to egress output fencing.

#### 4. Authority safety:
   - 0 accepted stale-epoch outputs in failover/lease-rotation tests.

#### 5. Replay baseline completeness:
   - 100% of accepted turns include required OR-02 baseline evidence fields.

#### 6. Terminal lifecycle correctness:
   - 100% of accepted turns emit exactly one terminal (`commit` or `abort`) followed by `close`.

#### 7. End to end latency
   - P50 <= 1.5s
   - P95 <= 2s

### 5.2 Fixed decisions for MVP
- Transport path: LiveKit only
- Provider set: deterministic multi-provider catalog per modality (3-5 STT, 3-5 LLM, 3-5 TTS) with pre-authorized retry/switch behavior.
  - STT: Deepgram, Google Speech-to-Text, AssemblyAI
  - LLM: Anthropic, Google Gemini, Cohere
  - TTS: ElevenLabs, Google Cloud Text-to-Speech, Amazon Polly
- Mode: Simple mode only (ExecutionProfile defaults required; advanced overrides deferred).
- Authority model: single-region execution authority with lease/epoch checks.
- Replay scope: OR-02 baseline replay evidence (L0 baseline contract) is mandatory.
