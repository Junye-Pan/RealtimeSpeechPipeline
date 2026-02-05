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
- Policy-driven active-active session routing across regions
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

RSPP is defined by a small set of contracts. These abstractions are the stable surface that application teams build against, independent of transport vendors and model providers.

### 4.1 Fundamental objects

1) **PipelineSpec**
A versioned, declarative contract that defines the execution graph, budgets, queue behavior, and fallback intent for a voice pipeline.

2) **Session**
A runtime conversation context with policy scope (tenant/region/language). PipelineSpec version is resolved by the control plane and is stable within a turn; version changes occur only at explicit turn boundaries. A session is logically continuous even if execution location changes.

3) **Turn**
The smallest conversational work unit from user intent intake to assistant output completion (or cancellation). Turns are the primary unit for latency, cancellation, and quality accounting.

4) **Graph**
A directed streaming dataflow for one pipeline version. The graph defines legal paths for normal flow, degradation, and fallback.

5) **Node**
A typed event processor that transforms input Event streams into output Event streams under declared runtime constraints. Business logic lives here.

6) **Edge**
A typed channel between nodes with explicit ordering and pressure semantics. Edges define how work moves and where buffering boundaries exist.

7) **Event**
The universal, immutable exchange unit in RSPP. Data, control, and observability all travel as Events so behavior is replayable and auditable.

8) **Policy**
A declarative intent layer for provider selection, fallback/degradation behavior, and routing preferences, evaluated within control-plane constraints.

9) **Provider Contract**
A normalized contract for provider capabilities and behavior (streaming modes, errors, limits, and outcomes) so providers/models can be swapped without rewriting business nodes.

10) **Transport Boundary Contract**
A contract that maps transport-domain inputs/outputs to runtime Events and back, keeping orchestration transport-agnostic while preserving non-goal boundaries (RSPP is not RTC transport).

11) **Control Plane Contract**
The authoritative contract for pipeline version resolution, rollout decisions, session admission, placement/routing, and migration decisions that the runtime executes.

12) **Determinism Contract**
A contract that defines ordering, merge, and terminal outcome semantics for fan-out/fan-in/fallback paths so execution and replay remain consistent. Determinism applies within a turn for a selected pipeline version and declared graph rules.

13) **Budget**
A contract for bounded execution across time and capacity scopes (turn/node/path). Budget outcomes are explicit: continue, fallback, degrade, or terminate.

14) **Resource & Admission Contract**
A contract that defines admission, concurrency, fairness, and overload behavior across session/turn scopes so the runtime protects latency and stability under load.

15) **External Node Boundary Contract**
A contract that defines trust/isolation boundaries and event/control interaction semantics for external node execution, including cancellation and terminal outcomes.

16) **Execution Profile**
A named behavior profile (for example, simple versus advanced) that defines default runtime semantics and which controls are opt-in. Simple mode is the profile with minimal required fields and opinionated defaults; advanced mode exposes additional controls.

17) **Runtime Contract**
The invariant set the framework guarantees: deterministic graph semantics, cancellation-first behavior, bounded resource behavior, and end-to-end observability.

### 4.2 Core signals

1) **Data progress signals**
Represent incremental and final media/text progress so stages can overlap instead of waiting for full completion.

2) **Turn boundary signals**
Mark turn open/close and carry one explicit terminal outcome marker per turn: `commit` (successful completion) or `abort` (terminated before commit, with reason such as cancel, timeout, or error), so the runtime has explicit lifecycle checkpoints.

3) **Interruption and cancellation signals**
Represent barge-in and stop intent; these signals have preemptive priority over ongoing generation and playback.

4) **Pressure and budget signals**
Represent queue pressure, budget warning/exhaustion, and runtime-selected fallback/degrade outcomes.

5) **Provider capability and outcome signals**
Represent provider selection context, normalized provider outcomes, and swap/fallback triggers under policy control.

6) **Failure and recovery signals**
Represent node/provider/edge failures and corresponding recovery actions as first-class runtime events.

7) **Routing, rollout, and migration signals**
Represent session assignment, pipeline version selection, handoff, and continuity across execution locations and regions.

8) **Admission and capacity signals**
Represent admit/reject/defer outcomes, capacity pressure, and load-shedding decisions.

9) **External boundary signals**
Represent external-node start/stop/cancel/failure/completion outcomes at the isolation boundary.

10) **Determinism and ordering signals**
Represent ordering markers, merge intent, and terminal path outcomes needed for stable fan-in/fallback semantics.

11) **Trace and replay signals**
Represent correlation and timeline markers required for debugging, evaluation, and deterministic replay.

Core signal rules:
- Every signal is correlated to session, turn, and pipeline version.
- Control signals can preempt data-path work.
- Control-plane decision signals are explicit inputs to runtime behavior and are auditable.
- Authority rule: control-plane decisions (admission, version, placement) are authoritative; policy refines behavior within those decisions.
- Terminal outcome per turn: `commit` (successful completion) or `abort` (terminated before commit, with reason such as cancel, timeout, or error). 
- `close` marks lifecycle completion and occurs after the terminal outcome.
- Signal semantics are transport-agnostic and provider-agnostic.

---
