## 4) Core Abstractions (Revised, Contract-First)

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

4) **ResolvedTurnPlan** (a.k.a. **CompiledGraph**, turn-scoped)  
A *turn‑scoped*, immutable plan produced at turn start by combining:  
- PipelineSpec (normalized)  
- Control-plane decisions (placement, resolved pipeline version, admission outcome)  
- Policy evaluation results (provider binding, degrade posture)  
- Provider/transport capability snapshots (as of turn start)  
ResolvedTurnPlan freezes provider bindings, budgets, edge buffering policies, and determinism rules **for the duration of the turn**.

---

#### 4.1.2 Runtime scopes

5) **Session**  
A logical conversation context with stable identity and policy scope (tenant/region/language). A session may migrate across runtimes/regions, but remains logically continuous via correlation identities and leases/epochs (see 4.1.8).

6) **Turn**  
The smallest conversational work unit from user-intent intake to assistant output completion (or cancellation). Turns have explicit lifecycle and a single terminal outcome:  
- `commit`: successful completion for that turn  
- `abort`: terminated before commit (reason: cancel, timeout, error, admission reject, disconnect, etc.)  
A `close` marker is emitted after terminal outcome to finalize lifecycle.

7) **Turn Arbitration Rules**  
A contract defining whether turns may overlap and how preemption behaves (e.g., barge‑in creates a new turn that can preempt an in‑flight turn). It defines late-event handling (events arriving after abort), grace windows, and which terminal semantics are authoritative within a session.

---

#### 4.1.3 Event model and lanes

8) **Event ABI (Schema + Envelope) Contract**  
The versioned ABI for all RSPP events. It defines:  
- Envelope fields (session_id, turn_id, pipeline_version, edge_id/node_id, event_id, causal_parent_id, idempotency_key, lane, sequence numbers)  
- Timestamp fields (see Timebase in 4.1.5)  
- Compatibility rules (schema evolution, version negotiation, deprecation)  
- Ordering and dedupe expectations (per lane/per edge)  

9) **Event**  
The universal, immutable exchange unit. Events carry **data** (audio/text), **control** (cancel/budget/turn boundaries), and **telemetry** (metrics/traces/debug) but all share the same ABI envelope for correlation and replay.

10) **Lane** (QoS class for events)  
RSPP classifies events into lanes with explicit priority and buffering semantics:  
- **DataLane**: audio frames, text deltas, semantic content outputs  
- **ControlLane**: turn boundaries, cancellation, budgets, admission, routing/migration, connection state  
- **TelemetryLane**: metrics, traces, logs, debug snapshots, replay markers  
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

12) **NodeExecutionContext** (turn-scoped)  
A runtime-provided context injected into every node invocation, containing:  
- `CancelToken` and cancel scope (see 4.1.5)  
- `BudgetHandle` (remaining time/capacity, degrade triggers)  
- `TimebaseHandle` (timestamping and latency measurement)  
- `StateHandle` (turn-ephemeral, session-hot, session-durable state access)  
- `TelemetryHandle` (non-blocking emission to TelemetryLane)  
- `DeterminismContext` (random seed, ordering markers; see 4.1.6)

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

16) **Watermarks** (a BufferSpec property)  
High/low watermark thresholds for queue depth and/or time-in-queue. Crossing watermarks emits pressure signals and may deterministically trigger policy outcomes (drop/merge/degrade/admission actions).

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
Defines ordering, merge semantics, fan-in resolution, and fallback terminal-outcome semantics such that execution and replay remain consistent within a turn under a ResolvedTurnPlan.

21) **DeterminismContext**  
A turn-scoped context containing:  
- a canonical **random seed** (and any allowed PRNG stream identifiers)  
- stable ordering markers  
- explicit declarations of allowed nondeterministic inputs (e.g., external calls)  
This context is recorded so replay can reproduce node-internal nondeterminism when re-simulating.

22) **Replay & Timeline Contract**  
Defines how to persist and replay event streams:  
- **EventTimeline**: ordered event log per session/turn/lane  
- **ReplayCursor**: a position in the timeline with deterministic stepping rules  
- **ReplayMode**: re-simulate nodes vs “playback recorded provider outputs” (provider determinism is not assumed)  
Replay must honor lane priorities and turn arbitration rules.

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

27) **PlacementLease (Epoch / Lease Token)**  
A control-plane-issued token proving current authority to execute a session/turn on a specific runtime placement. Leases have epochs and expiration. Runtime/transport must reject events and outputs that do not carry a valid current lease/epoch to prevent split-brain duplicates.

28) **Routing View / Registry Handle**  
A runtime-usable, read-optimized view of routing/placement decisions (derived from the control plane) used by transport adapters to locate the correct runtime endpoint and to detect/honor migrations quickly.

29) **State Contract (Hot vs Durable)**  
Defines state classes and ownership boundaries:  
- **Turn-Ephemeral State**: per-turn transient runtime state (always disposable)  
- **Session Hot State**: pipeline-internal hot state (e.g., VAD windows, caches, KV-cache) that is *not required* to survive failover and is not cross-region replicated  
- **Session Durable State**: session context state (identity, consent flags, intent slots, conversation memory pointers) that must survive failover and has defined consistency requirements  
- **Control-Plane Configuration State**: pipeline specs, rollouts, policy configs  
The contract explicitly declares which state may be lost/reset on migration to protect low latency. This reconciles with the **stateless runtime** model: runtime instances remain disposable, while only declared durable state is externalized and reattached on placement changes.

30) **Identity / Correlation / Idempotency Contract**  
Defines canonical identities and keys: session_id, turn_id, pipeline_version, event_id, provider_invocation_id, plus idempotency rules and dedupe semantics across retries, reconnects, and failovers. Lease epoch is part of correlation to disambiguate concurrent placements.

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
The invariant set RSPP guarantees: lane-prioritized control preemption, deterministic turn execution under a resolved plan, bounded buffering via BufferSpec/Watermarks, cancellation propagation with explicit scope, and end-to-end observability/replay correlation.

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
- turn boundaries: open, commit/abort(reason), close  
- interruption/cancellation: barge-in, stop, cancel(scope)  
- pressure/budget: watermark crossing, budget warnings/exhaustion, runtime-selected degrade/fallback outcomes  
- provider outcomes: normalized errors, circuit-break events, provider switch decisions (as allowed by plan)  
- routing/migration: placement lease issued/rotated, migration start/finish, session handoff markers  
- admission/capacity: admit/reject/defer outcomes, load-shedding decisions  
- connection lifecycle: connected/reconnecting/disconnected/ended, transport silence/stall

**Rule:** ControlLane events have preemptive priority over DataLane and TelemetryLane; runtime must deliver them with minimal buffering and must not allow telemetry/data backpressure to block control delivery.

#### 4.2.3 TelemetryLane signals (non-blocking observability)
- metrics samples (queue depth, drops, cancel latency, provider RTT, p95/p99)  
- traces/spans and correlation markers  
- debug snapshots (selected, rate-limited)  
- replay markers and evaluation annotations

**Rule:** TelemetryLane must be safe-by-default under overload (aggressive dropping/sampling) so it cannot add tail latency to DataLane or delay ControlLane.

---

### 4.3 Minimal rule set (what “runtime semantics” means)

- **Plan freezing:** A ResolvedTurnPlan is immutable within a turn; policy/control-plane changes apply only at explicit boundaries (turn boundary or lease epoch change).  
- **One terminal outcome per turn:** exactly one of `commit` or `abort` is emitted, followed by `close`.  
- **Lane priority:** ControlLane preempts everything; DataLane is latency-protected; TelemetryLane is best-effort.  
- **Bounded buffering:** Every Edge must have BufferSpec and watermarks (explicit or via profile defaults) so latency accumulation is always observable and controllable.  
- **Cancellation scope:** cancel propagation is explicit and must cancel provider invocations and buffered output generation where applicable.  
- **Failover safety:** PlacementLease epochs prevent split-brain outputs; HotState may reset; DurableState survives with declared consistency.  
- **Replayability:** EventTimeline + plan + determinism context define what can be replayed; provider determinism is not assumed unless recorded outputs are used.

---

## Change Notes & Rationales (one reason per modified area)

1) **Added “ResolvedTurnPlan / CompiledGraph” (turn-scoped freezing).**  
Reason: Without a frozen turn plan, dynamic policy/provider decisions can change mid-turn and break determinism, replay, and debuggability.

2) **Promoted “Event ABI (Schema + Envelope) Contract” to a first-class abstraction.**  
Reason: Replay, cross-transport compatibility, and end-to-end correlation require an explicit, versioned ABI—not just a generic “Event” concept.

3) **Introduced “Lane” (DataLane / ControlLane / TelemetryLane) while keeping a single Event model.**  
Reason: Separating QoS semantics prevents high-frequency telemetry or bulk data from blocking cancellation/turn control while preserving a unified event contract.

4) **Expanded “Edge” from a generic channel to a buffering boundary with explicit semantics.**  
Reason: In real-time voice pipelines, latency is dominated by queueing; Edge must therefore express buffering and ordering semantics directly.

5) **Added “BufferSpec” as an Edge property.**  
Reason: Drop/merge/block strategies and maximum in-queue latency must be declared and auditable to enforce backpressure and protect low latency, while still allowing Simple-mode profile defaults to keep required fields minimal.

6) **Added “Watermarks” as an explicit buffering construct.**  
Reason: Watermarks create deterministic triggers for pressure signals and runtime actions, making backpressure measurable and policy-driven rather than implicit.

7) **Added a “Timebase Contract” (monotonic, wall-clock, media time, mapping).**  
Reason: You cannot specify or measure real-time latency—and you cannot build reliable timelines for replay—without a unified session time model across transports.

8) **Added “NodeExecutionContext” as a required runtime injection.**  
Reason: Budgets, cancellation, timebase, state access, and telemetry must be enforceable and consistent across nodes, not left to convention.

9) **Added “Preemption Hooks Contract” (on_cancel/on_barge_in/on_budget_exhausted).**  
Reason: In streaming TTS/LLM/ASR nodes, rapid interruption requires explicit cooperative preemption semantics beyond merely sending a signal event.

10) **Refined Session/Turn semantics by adding “Turn Arbitration Rules.”**  
Reason: Barge-in and overlapping turns are common in voice UX; without explicit arbitration rules, commit/abort and late-event behavior becomes ambiguous.

11) **Split “State Contract” into Turn-Ephemeral, Session Hot, and Session Durable state.**  
Reason: Cross-region replication of hot pipeline state can destroy latency; explicitly allowing hot-state loss on failover reconciles multi-region resilience with low-latency goals while preserving the stateless-runtime deployment model.

12) **Added “PlacementLease (Epoch / Lease Token)” to the control-plane boundary.**  
Reason: Multi-region failover and reconnects can cause split-brain execution; epochs/leases are the minimal abstraction to prevent duplicated outputs and ambiguous authority.

13) **Added “Routing View / Registry Handle” as a read-optimized routing surface.**  
Reason: Transport adapters need a fast, runtime-usable representation of placement decisions to honor migrations without embedding control-plane logic inside the runtime.

14) **Added “ProviderInvocation” as a first-class unit.**  
Reason: Normalized observability, cancellation, idempotency, and replay often need to reference provider calls explicitly rather than treating them as opaque node internals.

15) **Added “DeterminismContext” including a canonical random seed.**  
Reason: Deterministic replay of node-internal behavior requires capturing allowed nondeterminism (like randomness) as part of turn-scoped recorded context.

16) **Expanded “Replay & Timeline” into explicit objects (EventTimeline, ReplayCursor, ReplayMode).**  
Reason: “Replay by default” is only meaningful if the system defines what is persisted, how it is stepped, and whether provider outputs are re-executed or played back.

17) **Extended “Transport Boundary Contract” with “ConnectionAdapter” and normalized liveness/disconnect semantics.**  
Reason: WebRTC/WebSocket/SIP have different failure semantics; normalizing them into control events is required for consistent cancellation, turn termination, and cleanup behavior without expanding RSPP into transport-stack ownership.

18) **Reorganized Section 4 into grouped primitives + cross-cutting contracts.**  
Reason: Treating principles (e.g., determinism, resource admission) as peers to concrete runtime objects obscures the stable surface area and makes implementation responsibilities unclear.
