# Test Suite Guide

This document defines the target architecture and module breakdown for framework test and conformance suites, aligned to `docs/PRD.md`, `docs/RSPP_features_framework.json`, and `docs/rspp_SystemDesign.md`.

## Evidence Baseline

- PRD semantics that drive suite design:
  - turn lifecycle and pre-turn outcomes: `§4.1.2`, `§4.3`
  - lanes, buffering, and flow control: `§4.1.3`, `§4.1.4`, `§4.3`
  - cancellation and output fencing: `§4.1.5`, `§4.2.2`, `§4.3`
  - determinism, replay, OR-02 evidence: `§4.1.6`
  - authority/lease epoch and migration safety: `§4.1.8`
  - MVP quality gates: `§5.1`
- System design boundaries: test/conformance module role in repository architecture (`docs/rspp_SystemDesign.md` sections `3`, `4`, `5`).
- Feature catalog anchors: `F-001..F-176`, `NF-009`, `NF-024..NF-029`, `NF-038` (referenced per module below).

## Target Architecture (Test + Conformance)

```text
PRD + Feature Catalog
        |
        v
+------------------------------+
| M1 Traceability Catalog      |
+--------------+---------------+
               |
               v
+--------------+---------------+       +------------------------------+
| M2 Contract Conformance      |<----->| M9 Replay + Provenance Lab  |
+--------------+---------------+       +------------------------------+
               |
               v
+--------------+---------------+       +------------------------------+
| M3 Lifecycle + Arbitration   |<----->| M6 Authority + Migration     |
+--------------+---------------+       +------------------------------+
               |
               v
+--------------+---------------+       +------------------------------+
| M4 Lanes/Buffer/Flow/Budget  |<----->| M5 Cancel + Egress Fence     |
+--------------+---------------+       +------------------------------+
               |
               v
+--------------+---------------+       +------------------------------+
| M7 Provider/Policy           |<----->| M8 Transport Profiles        |
+--------------+---------------+       +------------------------------+
               |
               v
+--------------+---------------+
| M10 Fault/Failover Matrix    |
+--------------+---------------+
               |
               v
+--------------+---------------+
| M11 SLO + Gate Reporter      |
+--------------+---------------+
               |
               v
CI gates (`verify-quick`, `verify-full`, `verify-mvp`)
```

## Current Codebase Snapshot (Verified 2026-02-16)

Progress summary from `test/` code audit:
- Strongly implemented: contract conformance (`CT-*`), replay/provenance baseline (`RD-*`), failover matrix baseline (`FX-*`), and LiveKit transport integration tests (`TP-*` MVP scope) (`F-001`, `F-083`, `F-086..F-091`, `F-149..F-154`, `NF-009`, `NF-028`).
- Partially implemented and currently scattered: lifecycle/arbitration, lanes/buffering, cancellation scope/fencing, authority/migration, provider/policy conformance (`F-033..F-048`, `F-055..F-066`, `F-138..F-148`, `F-155..F-160`, `F-175`).
- Missing as dedicated modules: traceability catalog (`M1`), transport conformance profiles directory (`M8` profile artifacts), gate-reporter tests (`M11`), extensibility SDK (`M12`), and runnable load suite (only scaffold guide exists) (`F-164`, `NF-024..NF-029`, `NF-038`).

Current suite evidence paths:
- `test/contract/fixtures_test.go`, `test/contract/schema_validation_test.go`
- `test/replay/rd001_replay_smoke_test.go`, `test/replay/rd002_rd003_rd004_test.go`
- `test/integration/quick_conformance_test.go`, `test/integration/cf_full_conformance_test.go`, `test/integration/ae004_enrichment_test.go`
- `test/integration/runtime_chain_test.go`, `test/integration/ml_conformance_test.go`, `test/integration/replay_audit_backend_integration_test.go`
- `test/integration/livekit_transport_integration_test.go`, `test/integration/livekit_live_smoke_test.go`, `test/integration/provider_live_smoke_test.go`
- `test/failover/failure_smoke_test.go`, `test/failover/failure_full_test.go`
- `test/load/load_test_suite_guide.md` (scaffold only)

## 1) Module List with Responsibilities

| Module | Target paths | Current status | Current code evidence | Responsibilities | Evidence / feature refs |
| --- | --- | --- | --- | --- | --- |
| `M1 Traceability Catalog` | `test/framework/spectrace` (planned) | Missing | None yet in `test/` | Maintain canonical mapping `feature_id -> case_id -> evidence artifact -> gate`; fail on orphaned tests, uncited features, or phase mismatch. | PRD `§4.4.6`, `§5.3`; `F-126`, `F-127`, `NF-009` |
| `M2 Contract Conformance` | `test/contract` | Implemented | `test/contract/fixtures_test.go`, `test/contract/schema_validation_test.go` | Schema/ABI/type validation, fixture validity/invalidity, lifecycle legality, and `ResolvedTurnPlan` provenance shape checks. | `F-001`, `F-003`, `F-008`, `F-020..F-032`, `F-138`, `F-141`, `F-149` |
| `M3 Lifecycle + Arbitration Conformance` | `test/integration/lifecycle` (planned), `test/integration/quick_conformance_test.go` | Partial | `test/integration/quick_conformance_test.go`, `test/integration/runtime_chain_test.go` | Validate turn lifecycle, overlap/preemption behavior, pre-turn decisions, and terminal sequencing. | `F-014`, `F-138..F-141`, `F-175`, `NF-029`; PRD `§4.1.2`, `§4.3` |
| `M4 Lanes/Buffer/Flow/Budget Conformance` | `test/integration/lanes` (planned), `test/load` | Partial | `test/integration/ml_conformance_test.go`, `test/failover/failure_full_test.go`, `test/load/load_test_suite_guide.md` | Validate lane priority, bounded buffering, watermarks, flow-control signals, deterministic shedding, and budget outcomes. | `F-043..F-048`, `F-142..F-148`, `F-052`, `NF-001`, `NF-004`, `NF-006` |
| `M5 Cancellation + Egress Fence Conformance` | `test/integration/cancellation` (planned), `test/integration/cf_full_conformance_test.go` | Partial | `test/integration/cf_full_conformance_test.go`, `test/integration/quick_conformance_test.go`, `test/integration/runtime_chain_test.go` | Validate cancel scope propagation, cooperative/forced cancel behavior, queue flush/reset, and post-cancel output rejection at runtime and transport boundaries. | `F-033..F-041`, `F-175`, `NF-003`, `NF-026`; PRD `§4.2.2` |
| `M6 Authority, State, Migration Conformance` | `test/integration/authority` (planned), `test/failover`, `test/integration/ae004_enrichment_test.go` | Partial | `test/integration/ae004_enrichment_test.go`, `test/integration/quick_conformance_test.go`, `test/failover/failure_smoke_test.go`, `test/failover/failure_full_test.go` | Validate lease-epoch ingress/egress rejection, migration markers, idempotency/dedupe, and state-class behavior under failover. | `F-095`, `F-096`, `F-155..F-159`, `NF-027`; PRD `§4.1.8` |
| `M7 Provider + Policy Conformance` | `test/integration/provider_*`, `test/integration/runtime_chain_test.go` | Partial | `test/integration/runtime_chain_test.go`, `test/integration/provider_live_smoke_test.go` | Validate provider contract normalization, retries/fallback/degrade policy actions, streaming/non-streaming parity, and error taxonomy behavior. | `F-055..F-066`, `F-131..F-137`, `F-160`, `NF-017` |
| `M8 Transport Adapter Conformance Profiles` | `test/integration/livekit_*`, `transports/*/*_test.go`, `test/conformance_profiles/transport` (planned) | Partial | `test/integration/livekit_transport_integration_test.go`, `test/integration/livekit_live_smoke_test.go`, `transports/livekit/*_test.go` | Define adapter profile requirements for session/auth/audio/control/lifecycle mapping; MVP profile for LiveKit and post-MVP profiles for WebSocket/telephony. | `F-083`, `F-086..F-091`, `F-084`, `F-085`, `F-164`, `NF-019` |
| `M9 Replay + Provenance Lab` | `test/replay`, `test/replay/fixtures/metadata.json` | Implemented (baseline) | `test/replay/rd001_replay_smoke_test.go`, `test/replay/rd002_rd003_rd004_test.go`, `test/integration/replay_audit_backend_integration_test.go` | Validate D1/D2 replay behavior, OR-02 completeness, divergence classification policy, and deterministic recording-level downgrade semantics. | `F-071..F-077`, `F-149..F-154`, `F-176`, `NF-009`, `NF-028`; PRD `§4.1.6` |
| `M10 Fault Injection + Resilience Matrix` | `test/failover`, `test/integration/replay_audit_backend_integration_test.go` | Implemented (baseline) | `test/failover/failure_smoke_test.go`, `test/failover/failure_full_test.go`, `test/integration/replay_audit_backend_integration_test.go` | Execute failure classes across cancellation, provider outage, admission stress, and authority rotation; assert deterministic outcomes and replay evidence. | `F-096`, `F-133`, `F-134`, `F-137`, `F-167`, `NF-030`, `NF-031` |
| `M11 SLO + Gate Reporter` | `internal/tooling/*`, `cmd/rspp-cli`, `.codex/*` artifacts | Partial (no dedicated test coverage) | Gate/report artifact generation is referenced in integration/live smoke paths, but no dedicated `test/*` suite validates gate computation end-to-end. | Compute and enforce gate metrics, publish operator-readable reports, and provide fixed-decision artifact outputs for CI. | `NF-024..NF-029`, `NF-038`, `F-127`; PRD `§5.1` |
| `M12 Extensibility Conformance SDK` | `test/framework/extensions` (planned), `test/integration` hooks | Missing | None yet in `test/` | Framework for plugin-provided tests covering custom nodes, external nodes, and custom runtime targets with minimum required assertions. | `F-011`, `F-121..F-124`, `F-128`, `F-164`, `NF-036`, `NF-037` |

## 2) Key Interfaces (Pseudocode)

```go
type FeatureID string
type CaseID string
type GateID string

type ConformanceCase struct {
	ID               CaseID
	Title            string
	FeatureRefs      []FeatureID // mandatory traceability coverage (F-126, F-127)
	ReleasePhase     string // mvp | post_mvp | future
	DeterminismLevel string // D0 | D1 | D2 (NF-009, F-149..F-153)
	NegativePath     bool   // required for active-phase coverage (PRD §4.4.6)
	FixtureRef       string
	RunnerProfile    string
	Assertions       []AssertionSpec
	RequiredEvidence []EvidenceKey // includes OR-02 keys for accepted turns (NF-028, F-153)
	RequiredGates    []GateID      // MVP gate checks (NF-024..NF-029, NF-038)
}

type SuiteModule interface {
	Name() string
	Register(reg *CaseRegistry) error
}

type RuntimeTarget interface {
	Name() string
	Start(run RunRequest) (RunHandle, error)
	CollectEvidence(runID string) (EvidenceBundle, error)
	Stop(runID string) error
	Capabilities() TargetCapabilities // transport/profile capability disclosure (F-083..F-091, F-164)
}

type ConformancePlugin interface {
	ID() string
	Profiles() []string // e.g. "transport/livekit/v1", "node/custom/v1"
	RegisterCases(reg *CaseRegistry) error
}
```

```go
type AssertionSpec struct {
	Key      string
	Operator string // ==, <=, contains, count_eq
	Expect   any
}

type EvidenceBundle struct {
	RunID         string
	CaseID        CaseID
	EventTimeline []Event
	Or02Manifest  map[string]any // required for accepted turns (NF-028, F-153)
	Metrics       map[string]float64
	Artifacts     []string
}
```

## 3) Data Model: Core Entities and Relations

| Entity | Core fields | Relations |
| --- | --- | --- |
| `FeatureRequirement` | `feature_id`, `release_phase`, `priority`, `source_refs` | `1:N` to `ConformanceCase` |
| `ConformanceCase` | `case_id`, `family`, `determinism_level`, `negative_path` | `N:1` to `SuiteModule`; `1:N` to `ScenarioRun` |
| `Fixture` | `fixture_id`, `schema_version`, `kind`, `seed` | `1:N` to `ConformanceCase` |
| `ScenarioRun` | `run_id`, `case_id`, `runtime_target`, `started_at`, `status` | `1:1` to `EvidenceBundle`; `1:N` to `DivergenceRecord` |
| `EvidenceBundle` | `timeline_ref`, `or02_manifest_ref`, `metrics_ref`, `artifact_refs` | `1:1` to `ScenarioRun`; consumed by `GateResult` |
| `GateResult` | `gate_id`, `status`, `computed_metric`, `threshold`, `reason` | `N:1` to `ScenarioRun` |
| `ConformanceProfile` | `profile_id`, `domain`, `required_cases`, `required_features` | `N:M` with `RuntimeTarget` and `ConformancePlugin` |
| `PluginDescriptor` | `plugin_id`, `type`, `version`, `declared_capabilities` | `1:N` to contributed `ConformanceCase` |

Relation rules:
1. Every `ConformanceCase` MUST reference at least one `FeatureRequirement` (`F-126`, `F-127`, `F-164`).
2. Every `ScenarioRun` that reaches accepted-turn execution MUST produce OR-02 evidence fields required by `NF-028`.
3. `GateResult` derivation must only use canonical anchor metrics defined by PRD `§4.4.2` (`NF-024..NF-029`, `NF-038`).

## 4) Extension Points: Plugins, Custom Nodes, Custom Runtimes

### 4.1 Plugins

- Plugin type: `ConformancePlugin`.
- Use cases:
  - adapter profile packs (`transport/livekit/v1`, future `transport/websocket/v1`)
  - provider policy packs
  - resilience/fault packs
- Rule: plugin-contributed cases must declare `FeatureRefs` and `DeterminismLevel` before registration (`F-164`, `NF-032`).

### 4.2 Custom Nodes

- Custom node authors provide:
  - node conformance manifest (`node_type`, `in/out event types`, `cancel_class`, `resource_limits`)
  - minimal case bundle: lifecycle hooks, cancel responsiveness, output semantics.
- Mandatory coverage: Node API compatibility and lifecycle (`F-011`, `F-012`), external boundary/resource limits when out-of-process (`F-122`, `F-123`, `F-124`), unit harness support (`F-128`).

### 4.3 Custom Runtimes

- Runtime vendors implement `RuntimeTarget` adapter and publish capability profile (`execution_profile`, `transport_profiles`, `replay_modes`).
- Required minimum conformance to join CI gate set:
  - contract suite pass (`M2`) for schema/ABI and lifecycle contract baseline (`F-001`, `F-020..F-032`, `F-138`)
  - lifecycle/cancel/authority suite pass (`M3`, `M5`, `M6`) (`F-139..F-141`, `F-033..F-041`, `F-156`, `F-175`)
  - replay OR-02 completeness pass (`M9`) (`F-149..F-154`, `NF-009`, `NF-028`)
- Post-MVP: conformance-profile governance becomes mandatory (`F-162`, `F-164`).

## 5) Design Invariants (Implementation Must Preserve)

1. Every case is traceable to at least one feature ID and one PRD semantic anchor (`F-126`, `F-127`).
2. Every active feature has at least one negative-path assertion in conformance coverage (PRD `§4.4.6`, `F-126`, `F-127`).
3. Accepted turns emit exactly one terminal outcome (`commit` or `abort`) followed by `close` (`F-139`, `NF-029`).
4. Pre-turn outcomes (`admit/reject/defer`) never emit turn terminal events (`F-141`).
5. `ResolvedTurnPlan` is immutable inside a turn and replay-visible (`F-149`, PRD `§4.3`).
6. ControlLane preemption over DataLane/TelemetryLane is always asserted under pressure (`F-142`, PRD `§4.2.2`).
7. DataLane must not block indefinitely in low-latency profiles; bounded shedding behavior must be deterministic (`F-148`).
8. Cancellation propagates by declared scope and fences runtime + transport egress (`F-034`, `F-175`, `NF-026`).
9. Post-cancel `output_accepted` and `playback_started` for canceled scope are invalid and rejected (`F-175`, PRD `§4.2.2`).
10. Stale-epoch ingress/egress acceptance count must remain zero (`F-156`, `NF-027`).
11. OR-02 replay baseline evidence is mandatory for accepted turns (`F-153`, `NF-028`).
12. Replay D1 ordering stability is mandatory for deterministic suites; D2 is required for decision-bearing replay suites (`F-149`, `F-150`, `F-151`, `NF-009`, PRD `§4.4.1`).
13. Timeline persistence cannot block ControlLane progression or cancel propagation (`F-154`, PRD `§4.1.6`).
14. Correlation fields (`session_id`, `turn_id`, plan/version identifiers) must be present across logs/metrics/traces (`F-067`, `NF-008`).
15. MVP gates are hard-fail in CI: `NF-024..NF-029` and `NF-038`.
16. Transport conformance profiles must preserve transport non-goals while validating adapter mapping correctness (`F-086`, `NF-019`).

## 6) Tradeoffs (Major Decisions and Alternatives)

| Decision | Chosen approach | Alternative | Why chosen |
| --- | --- | --- | --- |
| Determinism vs realism | Fixture-first deterministic suites as gate-critical; live suites as supplementary | Make live provider tests gate-critical | Live systems are noisy; deterministic gates are needed to enforce `NF-009`, `NF-028`, and repeatable CI behavior. |
| Module layout | Domain-specific modules (`M2..M12`) with explicit contracts | Single monolithic integration suite | Improves ownership and traceability to feature groups; reduces ambiguous failures (`F-126`, `F-127`, `F-164`). |
| Replay validation style | OR-02 manifest + divergence taxonomy + tolerances | Golden-output-only replay checks | Manifest-driven checks catch provenance/authority gaps that output-only checks miss (`F-149..F-154`). |
| Transport coverage model | Profile-based adapter conformance (`transport/<name>/vN`) | Ad hoc per-adapter tests | Supports MVP LiveKit now and extensible WebSocket/telephony later (`F-083`, `F-084`, `F-085`, `F-164`). |
| Gate strategy | Hard-fail on normative MVP gates, soft-fail diagnostics for optional live matrices | Treat all suites equally blocking | Maintains deterministic release quality while still collecting signal from optional live coverage (`NF-024..NF-029`, `NF-038`). |
| Assertion model | Property/invariant assertions over timelines plus targeted snapshots | Snapshot-only testing | Property checks are more robust for ordering/cancel/authority semantics; snapshots remain useful for specific fixtures (`F-008`, `F-140`, `F-142`, `F-175`, `NF-009`). |
| Extensibility enforcement | Plugin registration requires feature refs + deterministic level + mandatory base cases | Allow unstructured custom tests | Prevents extension sprawl and protects baseline conformance guarantees for custom nodes/runtimes (`F-121..F-124`, `F-164`, `NF-036`, `NF-037`). |

## Conformance ID Families (Target)

- `CT-*`: contracts and schema/typed fixtures (`F-001`, `F-020..F-032`, `F-138`)
- `LF-*`: lifecycle and turn arbitration (`F-139`, `F-140`, `F-141`, `NF-029`)
- `CF-*`: cancellation and output fencing (`F-033..F-041`, `F-175`, `NF-026`)
- `LN-*`: lane, buffering, flow control, and budget behavior (`F-043..F-048`, `F-142..F-148`)
- `AE-*`: authority epoch and migration safety (`F-095`, `F-156..F-159`, `NF-027`)
- `PV-*`: provider/policy behavior and degradation/fallback (`F-055..F-066`, `F-131..F-137`)
- `TP-*`: transport profile conformance (`F-083`, `F-086..F-091`, `F-164`)
- `RD-*`: replay determinism and provenance (`F-071..F-077`, `F-149..F-154`, `F-176`, `NF-009`, `NF-028`)
- `FX-*`: fault injection and resilience (`F-096`, `F-133`, `F-134`, `F-137`, `F-167`, `NF-030`, `NF-031`)
- `GQ-*`: gate quality metric conformance (`NF-024..NF-029`, `NF-038`)

## Gate Mapping

Current (codebase-verified) gate coverage:
- `make verify-quick`: contract (`CT`), integration baseline (`LF/CF/AE` partial), replay smoke (`RD` subset), failover smoke (`FX` subset), plus baseline report generation (`NF-024`, `NF-025`, `NF-026`, `NF-027`, `NF-028`, `NF-029`).
- `make verify-full`: broad `go test ./...` plus replay regression and artifact checks (current `LN/PV/TP/GQ` assertions remain partial in test topology) (`NF-009`, `F-149..F-154`, `F-176`).
- `make verify-mvp`: baseline gates plus live comparison artifact paths used for MVP evidence (`NF-038`, `F-127`).
- Optional non-blocking: live-provider smoke, LiveKit live smoke, A2 runtime live matrix.

Target (after moduleization) gate coverage:
- `make verify-quick`: deterministic `CT/LF/CF/AE/RD(smoke)/FX(smoke)/GQ(core)`.
- `make verify-full`: deterministic `CT/LF/CF/LN/AE/PV/TP/RD/FX/GQ` plus full replay regression.
- `make verify-mvp`: `verify-full` plus fixed-decision live comparison artifacts.

## Implementation-Ready Plan

Phase 1: Traceability and current-suite normalization (`M1`, `M2`, `M9`)
1. Create `test/framework/spectrace/` with a source-of-truth mapping file (`feature_id -> case_id -> evidence path -> gate`) (`F-126`, `F-127`, `NF-009`).
2. Add a lightweight conformance check test in `test/framework/spectrace` that fails on unmapped tests or unsupported release-phase references.
3. Wire existing cases from `test/contract`, `test/replay`, `test/failover`, and `test/integration` into this catalog.

Phase 2: Consolidate scattered integration coverage (`M3`, `M4`, `M5`, `M6`)
1. Introduce `test/integration/lifecycle/` and migrate lifecycle/arbitration checks from `test/integration/quick_conformance_test.go` and `test/integration/runtime_chain_test.go` (`F-138..F-141`, `NF-029`).
2. Introduce `test/integration/lanes/` and migrate lane/buffering checks from `test/integration/ml_conformance_test.go` and `test/failover/failure_full_test.go` (`F-043..F-048`, `F-142..F-148`).
3. Introduce `test/integration/cancellation/` and consolidate cancellation/fence coverage from `test/integration/cf_full_conformance_test.go` and `test/integration/runtime_chain_test.go` (`F-033..F-041`, `F-175`, `NF-026`).
4. Introduce `test/integration/authority/` and consolidate authority/migration checks from `test/integration/ae004_enrichment_test.go` and failover suites (`F-155..F-159`, `NF-027`).

Phase 3: Complete provider/transport/gate conformance (`M7`, `M8`, `M11`)
1. Promote provider policy checks into stable `test/integration/provider_*` deterministic suites, separating them from live-only smoke paths (`F-055..F-066`, `F-131..F-137`, `F-160`).
2. Create `test/conformance_profiles/transport/` and codify profile contracts for LiveKit (MVP) and stubs for WebSocket/telephony (post-MVP) (`F-083`, `F-084`, `F-085`, `F-164`).
3. Add dedicated `test` coverage that validates gate-metric computation/report generation against canonical anchors (`NF-024..NF-029`, `NF-038`).

Phase 4: Extensibility and load completion (`M12`, `M4`)
1. Implement `test/framework/extensions/` with plugin/custom-node/custom-runtime test registration primitives (`F-011`, `F-121..F-124`, `F-128`, `NF-036`, `NF-037`).
2. Convert `test/load/load_test_suite_guide.md` from scaffold to runnable load tests aligned with lane/budget invariants (`F-043..F-048`, `F-142..F-148`, `NF-001`, `NF-004`, `NF-006`).

Definition of done for this document to stay implementation-ready:
1. Every planned module row must include status (`Implemented`, `Partial`, or `Missing`) and at least one concrete evidence file path.
2. Any new claim in this file must include inline feature IDs (`F-*`/`NF-*`) where relevant.
3. Gate mapping must explicitly distinguish “current” and “target” coverage whenever they differ.
4. New `test/` suites must be added to this guide in the same change set that introduces them.

## Recommended Local Commands

- `go test ./test/contract ./test/integration ./test/replay ./test/failover`
- `make verify-quick`
- `make verify-full`

## Related Docs

- `docs/PRD.md`
- `docs/RSPP_features_framework.json`
- `docs/rspp_SystemDesign.md`
- `docs/CIValidationGates.md`
