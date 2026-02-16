# RSPP System Design (Repository-Level)

## 1. Purpose

This document is the repository-level architecture source of truth for RSPP.

It consolidates:
- `docs/PRD.md` (product and runtime semantics)
- `docs/RSPP_features_framework.json` (feature inventory and release phasing)
- `docs/repository_docs_index.md` (canonical guide locations and governance rules)

RSPP is a transport-agnostic real-time speech runtime framework that sits between transport layers and model providers.

## 2. Architectural Positioning

RSPP is:
- a deterministic runtime and contract layer for streaming speech pipelines
- a control-plane + runtime + observability + tooling system with explicit operational gates
- spec-first: `PipelineSpec` + `ExecutionProfile` define behavior; runtime enforces semantics

RSPP is not:
- an RTC media stack implementation (SFU/WebRTC/ICE/TURN)
- a voice agent SDK/planner framework
- a model provider API

## 3. Target Architecture (PRD-Aligned)

### 3.1 Core principles

1. Turn execution is frozen by immutable `ResolvedTurnPlan` at turn start.
2. `ControlLane` preempts `DataLane`; `TelemetryLane` is best-effort and non-blocking.
3. Cancellation is first-class and includes output fencing semantics.
4. Authority is lease/epoch enforced at ingress and egress boundaries.
5. Replay-critical OR-02 evidence is mandatory for accepted turns.
6. MVP execution profile is `simple/v1`; additional profiles are post-MVP.
7. Runtime is stateless-by-design for compute; durable session state is externalized.

### 3.2 High-level architecture diagram

```text
                                     +-------------------------------------------+
                                     | Authoring + Operations Plane              |
                                     | rspp-cli | rspp-local-runner | CI gates   |
                                     +-------------------+-----------------------+
                                                         |
                                                         | publish/validate specs
                                                         v
+------------------------+            +------------------+----------------------------------+
| Client Channels        |            | Control Plane                                     |
| RTC / WebSocket / PSTN |            | registry | normalizer | rollout | policy | lease |
+------------+-----------+            | routing view | provider health | admission         |
             |                        +------------------+----------------------------------+
             | ingress audio/events                      |
             v                                           | turn-start bundle
+------------+-------------------------------------------v----------------------------------+
| Transport Boundary Adapters                                                                |
| livekit (MVP), websocket (post-MVP), telephony (post-MVP)                                 |
+------------+------------------------------------------------------------------------------+
             |
             v
+------------+---------------------------------------------------------------------------------------------+
| Runtime Kernel (authority holder for accepted turn)                                                      |
| turnarbiter | planresolver | lanes | executor | nodehost | buffering | flowcontrol | budget | cancel   |
| determinism | identity | state-handle | localadmission | runtime guard                                  |
+--------+-------------------------------+---------------------------------------+--------------------------+
         |                               |                                       |
         | provider invocation path      | state path                            | telemetry/replay path
         v                               v                                       v
+--------+----------------------+   +----+--------------------------+   +-------+-------------------------+
| Provider Adapters             |   | State Services                |   | Observability + Replay          |
| STT/LLM/TTS + retries/switch  |   | turn-ephemeral (runtime)      |   | telemetry | timeline | replay   |
| (`providers/*`, `runtime/*`)  |   | session-hot (resettable)      |   | divergence | audit | retention  |
+-------------------------------+   | session-durable (external)    |   +----------------+---------------+
                                     +-------------------------------+                    |
                                                                                         v
                                                                       +-----------------+----------------+
                                                                       | Tooling + Gates                  |
                                                                       | verify-quick/full/security/mvp   |
                                                                       +----------------------------------+
```

### 3.3 Turn execution path (normative)

1. Transport normalizes ingress to Event ABI and authority context.
2. Control plane resolves pipeline version, policy, routing view, lease epoch, and admission outcomes.
3. Runtime materializes immutable `ResolvedTurnPlan` and accepts `turn_open` only on successful pre-turn checks.
4. Runtime executes graph with lane priority, bounded buffering, and cancellation-first semantics.
5. Provider calls run through normalized invocation outcomes and policy-driven retry/switch behavior.
6. Observability records OR-02 baseline evidence and replay markers without blocking control progression.

## 4. Target Module Breakdown

Feature-catalog baseline (`docs/RSPP_features_framework.json`): 214 entries total (`189` MVP, `25` post-MVP).

| Target module | Primary repository paths | Responsibilities | Primary feature groups |
| --- | --- | --- | --- |
| Contract surfaces | `api/controlplane`, `api/eventabi`, `api/observability` | Shared types for plans, events, replay/audit semantics, compatibility rules | `Unified Event ABI & Schema`, `Determinism, Replay & Provenance`, `Control Plane (Registry, Routing, Rollouts)` |
| Control-plane core | `internal/controlplane/*` | Registry, spec normalization, graph compile, policy/admission, lease authority, routing view, rollout decisions | `Control Plane (Registry, Routing, Rollouts)`, `State, Authority & Migration`, `Governance, Compatibility & Change Management`, `Multi-Region Routing & Failover` |
| Runtime session control | `internal/runtime/session`, `internal/runtime/prelude`, `internal/runtime/turnarbiter`, `internal/runtime/planresolver`, `internal/runtime/guard`, `internal/runtime/localadmission` | Turn lifecycle enforcement, pre-turn gating, plan freeze, deterministic terminal sequencing | `Runtime Semantics & Lifecycle`, `Cancellation & Turn Interrupts`, `Simple Mode & Configuration` |
| Runtime data plane | `internal/runtime/eventabi`, `internal/runtime/lanes`, `internal/runtime/executor`, `internal/runtime/nodehost`, `internal/runtime/buffering`, `internal/runtime/flowcontrol`, `internal/runtime/budget`, `internal/runtime/cancellation`, `internal/runtime/timebase`, `internal/runtime/sync`, `internal/runtime/executionpool` | Graph execution, lane routing, buffering/watermarks, flow control, budgets, cancellation, timebase | `PipelineSpec & Graph Execution`, `Backpressure, Budgets & Degradation`, `Edge Buffering, Lanes & Flow Control`, `Kubernetes Runtime, Scaling & Scheduling` |
| Provider subsystem | `internal/runtime/provider/*`, `providers/*` | Provider registry/invocation, normalized outcomes, streaming behavior, policy-aware retry/switch | `Provider Adapters & Policies`, `Error Handling & Recovery`, `Latency & Performance` |
| Transport subsystem | `internal/runtime/transport`, `transports/livekit`, `transports/websocket`, `transports/telephony` | Transport ingress/egress normalization, connection lifecycle mapping, authority-aware output fencing | `Transport Adapters (LiveKit/WebSocket/Telephony)`, `Cancellation & Turn Interrupts`, `State, Authority & Migration` |
| State and identity | `internal/runtime/state`, `internal/runtime/identity`, `internal/controlplane/lease`, `internal/controlplane/routingview` | Correlation/idempotency keys, hot vs durable state boundaries, lease epoch correctness | `State, Authority & Migration`, `Reliability & Resilience`, `Correctness` |
| Observability and replay | `internal/observability/telemetry`, `internal/observability/timeline`, `internal/observability/replay` | Non-blocking telemetry, OR-02 timeline evidence, replay divergence and audit access | `Observability, Recording & Replay`, `Determinism, Replay & Provenance`, `MVP Quality Gates` |
| Security and tenancy | `internal/security/*`, `internal/controlplane/security`, `internal/observability/replay` | Tenant isolation, authn/authz context, redaction/access controls, trust boundaries | `Security, Tenancy & Compliance`, `Data Governance, Key Management & Trust`, `Security Supply Chain`, `Security` |
| Developer tooling and ops | `internal/tooling/*`, `cmd/*` | Lint/validation, replay regression, SLO reports, release gating, local-runner workflows | `Tooling: Local Runner, CI/CD & Testing`, `MVP Quality Gates`, `Operability`, `Usability` |
| Test and conformance | `test/contract`, `test/integration`, `test/replay`, `test/failover`, `test/load` | Contract conformance, runtime integration, replay determinism, failover and stress coverage | `Testability`, `Reliability & Resilience`, `MVP Quality Gates` |
| Delivery scaffolds | `pipelines/*`, `deploy/*`, `pkg/contracts` | Pipeline/profile templates, deployment artifacts, future public package promotion surfaces | `Portability`, `Governance, Compatibility & Change Management`, `Capacity Planning & Fairness` |

## 5. MVP Baseline vs Post-MVP Target

| Capability area | MVP baseline | Post-MVP target |
| --- | --- | --- |
| Transport adapters | LiveKit path in production/runtime guides | WebSocket and telephony adapters as first-class implementations |
| Execution profiles | `simple/v1` only | Additional named profiles with deterministic defaults |
| Authority and routing | Single-region authority with lease/epoch safety | Active-active multi-region routing/failover |
| Replay recording | OR-02 baseline evidence (`L0`) mandatory | Expanded L1/L2 fidelity policy and broader replay modes |
| Governance and compatibility | Baseline schema compatibility checks | Explicit skew-window and conformance profile governance |
| Capacity and fairness | Baseline admission and scheduling controls | Broader fleet-level capacity planning/fairness controls |

Post-MVP feature concentration in catalog (25 total):
- `Data Governance, Key Management & Trust` (6)
- `Governance, Compatibility & Change Management` (5)
- `Multi-Region Routing & Failover` (4)
- `Capacity Planning & Fairness` (4)
- `Transport Adapters (LiveKit/WebSocket/Telephony)` (2)
- `Security Supply Chain` (2)
- `Resilience, DR & Chaos` (2)

## 6. Canonical Implementation Guides (Merged)

`docs/` remains repository-level policy/boundary documentation. Detailed module behavior is canonical in folder guides.

| Area | Canonical file |
| --- | --- |
| Command entrypoints | `cmd/command_entrypoints_guide.md` |
| Control-plane internals | `internal/controlplane/controlplane_module_guide.md` |
| Runtime kernel internals | `internal/runtime/runtime_kernel_guide.md` |
| Observability and replay internals | `internal/observability/observability_module_guide.md` |
| Tooling and release gates internals | `internal/tooling/tooling_and_gates_guide.md` |
| Internal shared helpers | `internal/shared/internal_shared_guide.md` |
| Provider adapters | `providers/provider_adapter_guide.md` |
| Transport adapters | `transports/transport_adapter_guide.md` |
| Test and conformance suites | `test/test_suite_guide.md` |
| Pipeline artifact scaffolds | `pipelines/pipeline_scaffolds_guide.md` |
| Deployment scaffolds | `deploy/deployment_scaffolds_guide.md` |
| Public package contract scaffolds | `pkg/public_package_scaffold_guide.md` |
| Control-plane API contracts | `api/controlplane/controlplane_api_guide.md` |
| Event ABI API contracts | `api/eventabi/eventabi_api_guide.md` |
| Observability API contracts | `api/observability/observability_api_guide.md` |

## 7. Documentation and Ownership Operating Rules

Documentation policy:
- `docs/` is for cross-repository architecture boundaries, CI/security governance, migration/ownership governance, and shared contract artifacts.
- Module-level implementation/design details must be updated in canonical folder guides first.

Ownership operating rules:
1. One primary owner per module path.
2. Cross-domain changes require review from all impacted owners.
3. Contract changes under `api/` require control-plane and runtime cross-review.
4. Security-sensitive paths require Security-Team review.

Scaffold/layout change-control requirements:
1. Declare impacted contracts and module boundaries.
2. Declare explicit ownership remap.
3. Declare CI gate impact (`verify-quick`, `verify-full`, `security-baseline`).
4. Preserve non-goal boundaries from this system design.

## 8. Repository-Level Guarantees and Non-Goals

Repository-level guarantees:
- deterministic turn lifecycle boundaries and terminal sequencing
- cancellation-first behavior with explicit output fencing
- bounded buffering and explicit pressure-handling strategies
- authority epoch safety for single-writer execution semantics
- replayable evidence and divergence classification support
- policy/security-aware data handling and replay access controls

Repository-level non-goals:
- RTC transport internals (SFU/WebRTC/ICE/TURN)
- platform-owned business integrations (CRM/payments/KB)
- model quality guarantees

## 9. Streaming Overlap Progress Snapshot (2026-02-13)

1. Mode correctness path is implemented end-to-end (effective handoff policy + effective provider-streaming posture in artifacts).
2. Dual-metric comparison artifacts are implemented:
   - `first_assistant_audio_e2e_latency_ms`
   - `turn_completion_e2e_latency_ms`
3. Latest paired live sample (`stt-assemblyai|llm-anthropic|tts-elevenlabs`, combo cap `1`) still showed streaming slower:
   - first-audio delta (streaming - non-streaming): `+3969ms`
   - completion delta (streaming - non-streaming): `+4925ms`
4. Dominant current contributor in this sample: STT attempt latency.

Progress snapshot pointers:
1. `docs/PRD.md` section `5.4 Execution progress snapshot (2026-02-13)`
2. `docs/rspp_SystemDesign.md` section `9. Streaming Overlap Progress Snapshot (2026-02-13)`
3. `docs/CIValidationGates.md` section `5.2 Implementation progress snapshot (2026-02-13)`
4. `internal/tooling/tooling_and_gates_guide.md` section `Latest implementation progress snapshot (2026-02-13)`
