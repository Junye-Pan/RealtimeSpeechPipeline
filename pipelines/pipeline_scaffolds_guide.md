# Pipeline Artifact Scaffolds Guide

## Scope and Evidence

This guide defines the target architecture and module breakdown for pipeline artifact scaffolds.

Evidence baseline:
- `docs/PRD.md` sections `3.1`, `4.1`, `4.2`, `4.3`, `5.1`, and `5.3`.
- `docs/rspp_SystemDesign.md` sections `2`, `3`, `4`, and `6`.
- `docs/RSPP_features_framework.json` feature IDs cited inline.

## Target Scaffold Layout

```text
pipelines/
  specs/
  profiles/
  modules/
  policies/
  bundles/
  rollouts/
  extensions/
  replay/
  compat/
  examples/
  tests/
```

Current repo status: only guide stubs exist under `pipelines/specs` and `pipelines/profiles`.  
Target status: all paths above become scaffolded artifact modules.

Observed repository snapshot (current):

```text
pipelines/
  pipeline_scaffolds_guide.md
  specs/pipeline_specs_guide.md
  profiles/pipeline_profiles_guide.md
```

Progress note:
- `pipelines/specs/pipeline_specs_guide.md` and `pipelines/profiles/pipeline_profiles_guide.md` are placeholder-only guides (no concrete versioned artifacts yet).
- Target directories `modules`, `policies`, `bundles`, `rollouts`, `extensions`, `replay`, `compat`, `examples`, and `tests` are not created yet.

## 1) Module List With Responsibilities

| Module | Path | Responsibilities | Feature trace |
| --- | --- | --- | --- |
| Pipeline specs | `pipelines/specs` | Versioned `PipelineSpec` source-of-truth artifacts: node/edge graph, typed IO, fallback/degrade intent, conditional routing, reusable graph references. | `F-001` to `F-019`, `F-015`, `F-016` |
| Execution profiles | `pipelines/profiles` | Named default packs (`simple/v1` MVP) for budgets, buffering, lane behavior, and deterministic defaults with explicit defaulting sources. | `F-078` to `F-082`, `F-143`, `F-148` |
| Reusable graph modules | `pipelines/modules` | Shareable subgraph artifacts imported by multiple specs; versioned independently but compatibility-checked against parent specs. | `F-019`, `F-003`, `F-005`, `F-006`, `F-018` |
| Policy overlays | `pipelines/policies` | Provider selection, circuit-break, cost, degrade/fallback, and optional runtime graph inserts resolved at turn boundaries. | `F-056` to `F-062`, `F-066`, `F-102` |
| Release bundles | `pipelines/bundles` | Immutable release unit that pins spec/profile/policy/node package digests and compatibility envelope for rollout and replay. | `F-015`, `F-092`, `F-100`, `F-149`, `NF-010` |
| Rollout manifests | `pipelines/rollouts` | Canary/percentage/tenant rollout plans, rollback targets, and admission constraints bound to bundle refs. | `F-093`, `F-098`, `F-100`, `F-165` |
| Extension manifests | `pipelines/extensions` | Registration manifests for custom nodes, external nodes, and runtime adapter plugins with capability and trust metadata. | `F-011`, `F-121` to `F-124`, `F-164`, `F-171` |
| Replay manifests | `pipelines/replay` | OR-02 baseline evidence schema refs, replay mode configs, fixtures, and divergence policy for regression. | `F-029`, `F-071`, `F-072`, `F-149` to `F-154`, `F-176`, `NF-028` |
| Compatibility contracts | `pipelines/compat` | Schema compatibility rules, version skew policy, deprecation windows, and conformance profile IDs. | `F-028`, `F-032`, `F-162`, `F-163`, `NF-032` |
| Examples/templates | `pipelines/examples` | Minimal quickstart and reference artifacts for simple mode and transport simulation workflows. | `F-079`, `F-125`, `F-126`, `F-130` |
| Conformance scaffolds | `pipelines/tests` | Artifact-level contract tests, replay regression suites, and gate assertions for latency/cancel/authority/replay lifecycle correctness. | `F-127`, `F-128`, `NF-024` to `NF-029`, `NF-038` |

### 1.1 Progress and Divergence Matrix

| Target module path | Current repo state | Divergence | First implementation artifact (minimum) |
| --- | --- | --- | --- |
| `pipelines/specs` | Exists, guide-only placeholder | Missing versioned spec artifacts and validation wiring | `pipelines/specs/simple_v1.yaml` + `pipelines/specs/README.md` (`F-001`, `F-079`) |
| `pipelines/profiles` | Exists, guide-only placeholder | Missing concrete `ExecutionProfile` defaults artifact | `pipelines/profiles/simple_v1.yaml` + `pipelines/profiles/README.md` (`F-078`, `F-081`) |
| `pipelines/modules` | Missing | No reusable subgraph scaffold | `pipelines/modules/README.md` + `pipelines/modules/transcription_normalize_v1.yaml` (`F-019`) |
| `pipelines/policies` | Missing | No policy overlay scaffold for provider/degrade behavior | `pipelines/policies/README.md` + `pipelines/policies/default_voice_policy_v1.yaml` (`F-056`, `F-057`, `F-102`) |
| `pipelines/bundles` | Missing | No immutable release bundle scaffold | `pipelines/bundles/README.md` + `pipelines/bundles/voice_assistant_bundle_v1.json` (`F-015`, `F-092`) |
| `pipelines/rollouts` | Missing | No rollout manifest scaffold | `pipelines/rollouts/README.md` + `pipelines/rollouts/canary_10pct_v1.yaml` (`F-093`, `F-100`) |
| `pipelines/extensions` | Missing | No extension/plugin manifest scaffold | `pipelines/extensions/README.md` + `pipelines/extensions/custom_sentiment_node_v1.yaml` (`F-011`, `F-121`, `F-122`) |
| `pipelines/replay` | Missing | No replay evidence + mode manifest scaffold | `pipelines/replay/README.md` + `pipelines/replay/or02_baseline_v1.yaml` (`F-153`, `F-154`, `NF-028`) |
| `pipelines/compat` | Missing | No compatibility/deprecation contract scaffold | `pipelines/compat/README.md` + `pipelines/compat/version_skew_policy_v1.yaml` (`F-028`, `F-162`, `NF-032`) |
| `pipelines/examples` | Missing | No end-to-end authoring examples | `pipelines/examples/README.md` + `pipelines/examples/simple_livekit_voice_v1/` (`F-079`, `F-125`, `F-130`) |
| `pipelines/tests` | Missing | No artifact-level gate/conformance scaffolds | `pipelines/tests/README.md` + `pipelines/tests/mvp_gates_manifest_v1.yaml` (`F-127`, `NF-024` to `NF-029`, `NF-038`) |

## 2) Key Interfaces (Pseudocode)

Interface intent mapping:
- `ArtifactStore` anchors spec/profile/policy retrieval + immutable bundle publication semantics (`F-001`, `F-015`, `F-092`, `F-100`).
- `ScaffoldCompiler` covers schema/semantic validation, normalization, and effective resolved-view generation (`F-001`, `F-003`, `F-081`, `F-126`).
- `TurnPlanResolver` freezes turn-time execution decisions with authority/routing context (`F-095`, `F-149`, `F-156`, `F-159`).
- `ExtensionRegistry` and `RuntimeAdapterPlugin` are extension control points for custom/external nodes and adapter contracts (`F-011`, `F-121`, `F-122`, `F-164`).

```go
type ArtifactStore interface {
    GetPipelineSpec(ref SpecRef) (PipelineSpec, error)
    GetExecutionProfile(ref ProfileRef) (ExecutionProfile, error)
    GetPolicyOverlay(ref PolicyRef) (PolicyOverlay, error)
    PutBundle(bundle PipelineBundle) error // immutable write
}

type ScaffoldCompiler interface {
    ValidateSpec(spec PipelineSpec) ([]ValidationIssue, error) // schema + semantic checks
    Normalize(spec PipelineSpec, profile ExecutionProfile, overlays []PolicyOverlay) (ResolvedSpec, error)
    BuildBundle(input BundleInput) (PipelineBundle, error)
}

type TurnPlanResolver interface {
    ResolveTurnPlan(
        session SessionIdentity,
        turn TurnOpenRequest,
        bundle PipelineBundle,
        lease PlacementLease,
        routing RoutingSnapshot,
        capabilities CapabilitySnapshot, // post-MVP expansion
    ) (ResolvedTurnPlan, error)
}

type ExtensionRegistry interface {
    RegisterNodePlugin(plugin NodePlugin) error
    RegisterExternalNodeBoundary(boundary ExternalNodeBoundary) error
    RegisterRuntimeAdapter(adapter RuntimeAdapterPlugin) error
}

type RuntimeAdapterPlugin interface {
    ID() string
    ValidateContract(TransportContract, EventABIContract) error
    BuildAdapter(cfg map[string]any) (RuntimeAdapter, error)
}
```

## 3) Data Model: Core Entities and Relations

| Entity | Core fields | Relations |
| --- | --- | --- |
| `PipelineSpec` | `pipeline_id`, `version`, `nodes[]`, `edges[]`, `subgraph_refs[]`, `metadata` | Referenced by `PipelineBundle`; compiled into `ResolvedSpec` (`F-001`, `F-002`, `F-019`). |
| `ExecutionProfile` | `profile_id`, `version`, `lane_defaults`, `budget_defaults`, `buffer_defaults` | Applied during normalization; resolved defaults become replay-visible (`F-078`, `F-079`, `F-081`). |
| `PolicyOverlay` | `policy_id`, `version`, `provider_rules`, `degrade_rules`, `insert_rules` | Applied at turn boundaries; contributes to `ResolvedTurnPlan` (`F-056`, `F-057`, `F-102`). |
| `NodePackageRef` | `node_type`, `impl_ref`, `digest`, `execution_mode` | Referenced by specs and extensions; supports built-in/custom/external node classes (`F-010`, `F-011`, `F-121`, `F-122`). |
| `PipelineBundle` | `bundle_id`, `spec_ref`, `profile_ref`, `policy_refs[]`, `node_digests[]`, `compat_window` | Immutable release artifact consumed by rollout and runtime resolution (`F-015`, `F-092`, `F-100`, `NF-010`). |
| `RolloutManifest` | `rollout_id`, `bundle_ref`, `strategy`, `cohorts`, `rollback_ref` | Control-plane consumes this for version resolution and traffic shaping (`F-093`, `F-098`, `F-100`). |
| `ResolvedSpec` | `resolved_spec_id`, `bundle_id`, `effective_nodes`, `effective_edges`, `defaulting_sources` | Materialized effective graph used to build per-turn plans (`F-081`, `F-143`, `F-148`). |
| `ResolvedTurnPlan` | `resolved_turn_plan_id`, `plan_hash`, `bundle_id`, `lease_epoch`, `routing_snapshot_ref`, `determinism_context` | Frozen per accepted turn; source for execution + replay truth (`F-149`, `F-150`, `F-156`). |
| `ReplayEvidenceManifest` | `turn_ref`, `or02_evidence_schema_version`, `required_fields`, `recording_level` | Linked to turn outcomes and gate validation (`F-153`, `F-154`, `F-176`, `NF-028`). |
| `ExtensionManifest` | `extension_id`, `type`, `api_version`, `trust_level`, `resource_limits` | Governs plugin loading and safety checks before activation (`F-121`, `F-123`, `F-164`, `F-171`). |

Core relations:
- `PipelineBundle` pins exactly one `PipelineSpec` version and one `ExecutionProfile` version, plus zero or more `PolicyOverlay` refs (`F-015`, `F-078`, `F-102`).
- `RolloutManifest` points to `PipelineBundle` and is the only mutable traffic-control surface (`F-093`, `F-100`).
- `ResolvedTurnPlan` is derived from `PipelineBundle` plus control-plane snapshots and lease authority at turn open (`F-095`, `F-149`, `F-159`).
- `ReplayEvidenceManifest` references `ResolvedTurnPlan` and terminal outcomes for gate enforcement (`F-153`, `F-154`, `NF-028`).

## 4) Extension Points: Plugins, Custom Nodes, Custom Runtimes

Plugin classes:
- `NodePlugin` (in-process custom node): stable Event stream API, lifecycle hooks, correlation propagation (`F-011`, `F-012`, `F-013`).
- `ExternalNodePlugin` (out-of-process): explicit boundary for cancellation, timeout, observability injection, and isolation policy (`F-122`, `F-123`, `F-124`).
- `PolicyPlugin` (optional): constrained policy evaluators producing deterministic decisions at turn boundaries only (`F-056`, `F-057`, `F-062`, `F-066`).
- `RuntimeAdapterPlugin`: custom transport/runtime bridge implementing the shared transport + Event ABI contract (`F-086`, `F-020`, `F-025`).

Custom nodes:
- Must declare IO schema, streaming mode, cancel behavior, and state class usage (`F-003`, `F-009`, `F-036`, `F-155`).
- Must be package-versioned independently from specs and referenced by digest in bundles (`F-121`, `F-015`).
- External nodes must provide resource/time limits and trust metadata before admission (`F-123`, `F-171`).

Custom runtimes:
- Must preserve lane priority, lease/epoch authority checks, cancellation fencing, and replay evidence compatibility (`F-142`, `F-156`, `F-175`, `NF-028`).
- Must not bypass Event ABI compatibility checks or emit non-conformant lifecycle sequences (`F-021`, `F-028`, `F-139`).
- Must advertise capability snapshots explicitly (post-MVP contract path) (`F-161`).

## 5) Design Invariants (Implementation MUST Preserve)

1. Every `PipelineSpec` and profile artifact must pass schema and semantic validation before publish or rollout. (`F-001`, `F-126`)
2. Published pipeline versions are immutable; changes require new versions and explicit rollout references. (`F-015`, `F-092`, `F-100`)
3. Session execution is pinned to a resolved pipeline version; cross-session leakage is forbidden. (`F-016`)
4. A `ResolvedTurnPlan` is frozen at turn start and is immutable for the turn duration. (`F-149`, `F-078`)
5. Accepted turn lifecycle is exactly `OPEN -> ACTIVE -> (COMMIT|ABORT) -> CLOSE`; no alternate terminal ordering. (`F-139`, `NF-029`)
6. `admit/reject/defer` remain pre-turn outcomes and do not emit turn terminal markers. (`F-141`)
7. Lane priority is strict: `ControlLane > DataLane > TelemetryLane` under pressure. (`F-142`)
8. Each edge has bounded buffering with explicit bounds/watermarks/defaulting source. (`F-043`, `F-143`, `F-144`)
9. Low-latency mode must never permit indefinite DataLane blocking; deterministic shedding path is required. (`F-044`, `F-148`)
10. Cancellation must propagate by scope and enforce runtime plus transport egress fencing. (`F-033`, `F-034`, `F-175`, `NF-026`)
11. After cancel acceptance, new `output_accepted` and `playback_started` for that scope are invalid. (`F-175`)
12. Lease epoch authority must be validated on ingress and egress; stale outputs/events are rejected. (`F-095`, `F-156`, `NF-027`)
13. Correlation identities must be present end-to-end in events, logs, and traces. (`F-013`, `F-067`)
14. Replay baseline evidence completeness is mandatory for all accepted turns, independent of recording level. (`F-153`, `F-154`, `F-176`, `NF-028`)
15. Event ABI/schema compatibility and version-skew policy are gate-enforced; incompatible combos are rejected. (`F-021`, `F-028`, `F-162`, `NF-032`)
16. External/custom node execution must respect declared isolation and resource controls. (`F-122`, `F-123`, `F-124`)
17. `simple/v1` defaults must resolve deterministically and be inspectable in resolved views. (`F-078`, `F-079`, `F-081`)
18. Artifact-level conformance suites must enforce latency, cancellation, authority, replay, and lifecycle gate targets. (`F-127`, `NF-024` to `NF-029`, `NF-038`)

## 6) Tradeoffs: Major Decisions and Alternatives

| Decision area | Chosen direction | Alternative | Tradeoff summary | Feature trace |
| --- | --- | --- | --- | --- |
| Authoring model | Declarative, spec-first artifacts | Code-first graph construction SDK | Declarative artifacts improve auditability/replay/governance; code-first is more flexible but harder to normalize and gate consistently. | `F-001`, `F-002`, `F-081`, `F-126` |
| Version management | Immutable bundles + explicit rollouts | Mutable "latest" spec/profile pointers | Immutability improves rollback safety and traceability; mutable pointers reduce operational overhead but increase drift risk. | `F-015`, `F-092`, `F-093`, `F-100` |
| Turn-time decisioning | Freeze plan at turn start | Allow policy/provider rebinding mid-turn | Plan freeze preserves determinism and replayability; mid-turn adaptation may improve resilience but complicates correctness proofs. | `F-149`, `F-150`, `F-151`, `F-176` |
| Queue strategy | Bounded queues + deterministic shedding/degrade | Lossless queues or indefinite blocking | Bounded queues protect latency and memory; lossless behavior improves fidelity but can violate latency and stability SLOs. | `F-043`, `F-044`, `F-048`, `F-143`, `NF-004`, `NF-024`, `NF-025` |
| Node isolation default | In-process for trusted nodes, boundary for external/untrusted nodes | Out-of-process for all nodes | Mixed mode reduces latency overhead while preserving security boundaries where needed; all-external maximizes isolation but increases overhead and complexity. | `F-121`, `F-122`, `F-123`, `F-124`, `F-171` |
| Replay fidelity baseline | Mandatory L0 baseline, opt-in L1/L2 | Always-on full-fidelity recording | L0 baseline controls cost and latency overhead; always-on L2 improves forensic depth but raises storage/privacy/perf costs. | `F-153`, `F-154`, `F-176`, `NF-028` |
| Authority topology (MVP) | Single-region lease/epoch authority | Active-active multi-region from day one | MVP path simplifies split-brain prevention and rollout risk; active-active improves availability but adds major coordination complexity. | `F-095`, `F-156`, `F-110` to `F-113` |

## 7) Implementation-Ready Rollout Plan

Phase 0: baseline scaffolds (make target layout real)
- Create missing directories and `README.md` stubs for: `modules`, `policies`, `bundles`, `rollouts`, `extensions`, `replay`, `compat`, `examples`, `tests`.
- Definition of done: every target module path exists and describes artifact purpose + validation owner (`F-126`, `F-129`).

Phase 1: minimal runnable authoring path
- Add `pipelines/specs/simple_v1.yaml` (minimal valid `PipelineSpec`) and `pipelines/profiles/simple_v1.yaml` (`simple/v1` defaults).
- Add `pipelines/bundles/voice_assistant_bundle_v1.json` that pins spec/profile references and node digests.
- Definition of done: spec/profile can be validated and composed into a bundle deterministically (`F-001`, `F-078`, `F-081`, `F-015`).

Phase 2: control-plane-facing rollout scaffolds
- Add rollout manifest + policy overlay samples tied to the bundle (`pipelines/rollouts/canary_10pct_v1.yaml`, `pipelines/policies/default_voice_policy_v1.yaml`).
- Add compatibility policy scaffold (`pipelines/compat/version_skew_policy_v1.yaml`).
- Definition of done: rollout/policy/compat references are internally consistent and versioned (`F-093`, `F-100`, `F-102`, `F-162`, `NF-032`).

Phase 3: extension and replay scaffolds
- Add extension manifest examples for custom and external node paths.
- Add OR-02 replay manifest and gate-oriented evidence requirements.
- Definition of done: extension/replay artifacts define enforceable requirements for isolation and evidence completeness (`F-121`, `F-122`, `F-123`, `F-153`, `F-154`, `NF-028`).

Phase 4: conformance and examples
- Add `pipelines/examples/simple_livekit_voice_v1/` with reference artifacts.
- Add `pipelines/tests/mvp_gates_manifest_v1.yaml` covering gate assertions (latency, cancel fence, authority safety, replay completeness, lifecycle correctness).
- Definition of done: artifact-level test manifests map directly to MVP gates and can be wired to CI (`F-127`, `NF-024` to `NF-029`, `NF-038`).

Recommended implementation order:
1. `specs` + `profiles`
2. `bundles`
3. `policies` + `rollouts` + `compat`
4. `extensions` + `replay`
5. `examples` + `tests`

## Notes on MVP vs Post-MVP

- MVP scaffold baseline should fully implement the modules above for `simple/v1`, LiveKit transport path, and single-region lease authority (`F-078`, `F-083`, `F-095`).
- Post-MVP expansion should add capability discovery freeze, explicit version-skew governance, conformance profile publication, and fairness/data-governance overlays (`F-161`, `F-162`, `F-164`, `F-172`, `F-168`).
