# RSPP System Design (Repository-Level)

## 1. Purpose

This document defines RSPP at the repository level:

- what the framework is
- what boundaries it enforces
- what repository-wide guarantees it provides

Implementation details for modules live in folder-level guide markdown files.

## 2. Positioning

RSPP is a real-time speech runtime framework that sits between transport layers and model providers.

RSPP is not:

- an RTC media stack implementation
- a voice agent SDK/planner framework
- a model provider API

RSPP is:

- a deterministic runtime and contract layer for streaming speech pipelines
- a control-plane + runtime + observability + tooling system with explicit operational gates

## 3. Cross-repo architecture model

RSPP is organized into four repository planes:

1. Control plane: versioning, rollout, policy, admission, authority/routing decisions
2. Runtime kernel: turn lifecycle, execution semantics, buffering, cancellation, authority enforcement
3. Observability/replay: timeline evidence, divergence analysis, replay access/audit
4. Developer tooling: contract validation, replay regression, SLO gates, release readiness

### Plane ownership map

| Plane | Canonical implementation guides |
| --- | --- |
| Control plane | `internal/controlplane/controlplane_module_guide.md`, `api/controlplane/controlplane_api_guide.md` |
| Runtime kernel | `internal/runtime/runtime_kernel_guide.md`, `api/eventabi/eventabi_api_guide.md` |
| Observability/replay | `internal/observability/observability_module_guide.md`, `api/observability/observability_api_guide.md` |
| Tooling | `internal/tooling/tooling_and_gates_guide.md`, `cmd/command_entrypoints_guide.md`, `test/test_suite_guide.md` |

## 4. Repository-level guarantees

RSPP guarantees at repository level:

- deterministic turn lifecycle boundaries and terminal sequencing
- cancellation-first behavior with explicit output fencing
- bounded buffering and explicit pressure-handling policy targets
- authority epoch safety for single-writer execution semantics
- replayable evidence and divergence classification
- policy/security-aware data handling and replay access controls

Detailed module-level behavior for these guarantees is defined in domain READMEs.

## 5. Repository-level non-goals

RSPP explicitly does not own:

- RTC transport internals (SFU/WebRTC/ICE/TURN)
- business-system integrations (CRM/payments/KB) as platform-owned modules
- model quality guarantees

## 6. Engineering rationale (repo-level)

### Latency mechanics

RSPP treats latency as a runtime-architecture problem. Repository design includes overlap-capable streaming paths and low-overhead contract boundaries. Default runtime execution remains deterministic stage-by-stage unless streaming handoff policy is enabled. At a system level, perceived latency can be reasoned as:

`L_total = L_net + L_vad + L_asr + L_llm + L_tts + L_client`

The architecture goal is to overlap compatible stages where possible rather than enforce hard stage-by-stage waits.

### Orchestration-level streaming overlap model

Target overlap path:
1. STT partial output begins while user audio is still arriving.
2. LLM starts on eligible STT partial content before STT final.
3. TTS starts on eligible LLM partial content before LLM final.

Current status:
- baseline overlap executor exists in runtime (`internal/runtime/executor/streaming_handoff.go`).
- live provider chain smoke invokes streaming-chain execution and records chain artifacts with effective-mode markers:
  - streaming mode uses overlap-friendly handoff defaults when explicit handoff env policy is unset
  - non-streaming mode explicitly disables provider-native streaming in scheduler->RK-11 invocation input
- rollout is policy-gated; sequential stage-by-stage fallback remains the safe default when overlap policy is disabled.
- compare and gate workflow now includes deterministic paired artifact generation via `rspp-cli live-latency-compare-report`; `verify-mvp` fails closed if compare evidence is missing/invalid.

Implementation progress snapshot (2026-02-13):
1. Mode-correctness path is implemented end-to-end (effective handoff policy + effective provider streaming posture captured in live artifacts).
2. Dual-metric comparison artifacts are implemented (`first_assistant_audio_e2e_latency_ms`, `turn_completion_e2e_latency_ms`).
3. Latest paired live sample (`stt-assemblyai|llm-anthropic|tts-elevenlabs`, combo cap 1) still showed streaming slower:
   - first-audio delta (streaming - non-streaming): `+3969ms`
   - completion delta (streaming - non-streaming): `+4925ms`
   - dominant contributor in this sample: STT attempt latency.

Transport independence note:
1. LiveKit usage at transport boundary does not block provider-native streaming upgrades.
2. LiveKit handles client transport/session boundary concerns; provider adapters can independently use native streaming protocols (including websocket-based provider streams) between runtime and provider backends.

Expected first-audio behavior with overlap:
- perceived first assistant audio is driven by overlapped partial-path timing, not sum of full-stage final latencies.
- deterministic sequencing, cancellation fences, and authority epoch rules still apply.

Slow-stage policy at architecture level:
1. Architecture target: any slower downstream stage should trigger bounded queues plus explicit flow-control markers. Current runtime evidence includes handoff actions (`forward`, `coalesce`, `supersede`, `final_fallback`) and backpressure signals (`xoff`/`xon`).
2. Current implementation status: pressure decision primitives are implemented in `internal/runtime/buffering/pressure.go` and validated in runtime/failover suites.
3. If bounded mitigation cannot preserve legal progress, runtime emits deterministic terminal abort semantics, and observability captures both timing and mitigation actions so replay can explain latency outliers.

### Barge-in and cancellation correctness

RSPP treats interruption as a first-class control concern. The runtime contract requires cancellation-first output fencing semantics so accepted cancellation is immediately reflected at transport output boundaries and in replay evidence.

### Stateless scaling posture

RSPP is designed for horizontally scalable runtime workers with durable state externalization. Authority and epoch controls preserve single-writer behavior while allowing migration/failover continuity. This aligns with dispatcher/worker deployment patterns where compute is replaceable while authoritative state and epoch progression remain durable.

## 7. Operational entrypoints and lifecycle

Repository lifecycle is anchored by:

- runtime and local-runner entrypoints: `cmd/command_entrypoints_guide.md`
- contract/replay/SLO/release tooling: `internal/tooling/tooling_and_gates_guide.md`
- test and conformance suites: `test/test_suite_guide.md`
- CI gate policy: `docs/CIValidationGates.md`

## 8. Documentation policy link

For module implementation/design details, use corresponding folder guide files indexed in `docs/repository_docs_index.md`.
