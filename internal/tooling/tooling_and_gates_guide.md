# Tooling and Release Gates Guide (Code-Verified + Implementation-Ready)

This guide is both:
- the target architecture for tooling/release-gate internals
- the current-state progress/divergence report for what is already in code

As-of snapshot date: `2026-02-16`.

Primary evidence sources:
- `docs/PRD.md` section `3.2`, `4.1.6`, `5.1`, `5.2`
- `docs/RSPP_features_framework.json`
- `docs/rspp_SystemDesign.md`
- code under `internal/tooling/*`, `cmd/rspp-cli`, `cmd/rspp-local-runner`, `Makefile`, `scripts/*`

Code-level verification run in this pass:
- `GOCACHE=/tmp/go-build go test ./internal/tooling/... ./cmd/rspp-cli ./cmd/rspp-local-runner` passed.

## 0) Current codebase progress and divergence snapshot

| Area | Current implementation (code-verified) | Progress | Divergence from target | Evidence |
| --- | --- | --- | --- | --- |
| Gate orchestration | Gate sequencing is implemented in CLI subcommands + Makefile chains (`verify-quick/full/mvp`), not as a reusable internal orchestrator package. | partial | No `internal/tooling/gates` package or shared gate registry yet. | `cmd/rspp-cli/main.go`, `Makefile` ([F-126], [F-127], [NF-016]) |
| Contract validation | Strict typed + schema validation for `event`, `control_signal`, `turn_transition`, `resolved_turn_plan`, `decision_outcome`, plus CLI `validate-spec`/`validate-policy` validation paths with strict/relaxed mode. | implemented baseline | Spec/policy validation is file-scoped; cross-artifact compatibility lint is still pending. | `internal/tooling/validation/contracts.go`, `internal/tooling/validation/spec_policy.go`, `cmd/rspp-cli/main.go` ([F-001], [F-126], [NF-015], [NF-010]) |
| Replay regression | Divergence policy engine exists; quick/full fixture selection, per-fixture artifacts, expected divergence policy, and invocation-latency threshold checks are wired. | implemented baseline | Regression input is fixture-builder driven; no first-class replay execution ingestion path in tooling module yet. | `internal/tooling/regression/divergence.go`, `cmd/rspp-cli/main.go` ([F-127], [F-149], [F-151], [F-152], [NF-009]) |
| SLO gates | All seven MVP gate formulas are implemented and report violations deterministically. | implemented baseline | Primary default input path uses generated runtime baseline artifact; production trace ingestion is not yet the default gate path. | `internal/tooling/ops/slo.go`, `cmd/rspp-cli/main.go` ([NF-024], [NF-025], [NF-026], [NF-027], [NF-028], [NF-029], [NF-038]) |
| MVP live gates | `slo-gates-mvp-report` enforces baseline + live end-to-end + fixed-decision checks (simple mode, provider policy, parity markers, LiveKit smoke). | implemented baseline | Live end-to-end includes waiver after multiple attempts (explicitly recorded), so strict threshold failure can be waived by policy. | `cmd/rspp-cli/main.go` ([NF-038], [F-078], [F-095], [F-100]) |
| Release readiness | Release publish fails closed on missing/stale/failing contracts/replay/SLO artifacts; rollout config is validated; deterministic manifest emitted with source hashes. | implemented baseline | Readiness currently uses three artifacts only; no built-in readiness check for `security-baseline`, `codex-artifact-policy`, or `verify-mvp` artifacts in publish path. | `internal/tooling/release/release.go`, `cmd/rspp-cli/main.go` ([F-093], [F-100], [NF-016], [NF-032]) |
| Local runner | `rspp-local-runner` now supports both LiveKit dry-run and explicit offline `loopback` mode (synthetic/WAV input) with Event ABI parity validation of generated reports. | implemented baseline | Dedicated `internal/tooling/runner` abstraction is still not introduced; command-level orchestration remains in entrypoint code. | `cmd/rspp-local-runner/main.go`, `cmd/rspp-local-runner/main_test.go` ([F-125], [F-130]) |
| Authoring kit | Authoring area exists only as scaffold guide. | scaffold only | No node SDK test harness implementation in `internal/tooling/authoring` yet. | `internal/tooling/authoring/node_authoring_kit_guide.md` ([F-128], [F-121], [F-122], [F-123], [F-124]) |
| Conformance/governance gates | Dedicated governance checks now exist under `internal/tooling/conformance` and are wired into `rspp-cli conformance-governance-report` + `verify-quick/full/mvp` chains (version-skew matrix, deprecation lifecycle, mandatory profile categories, feature-status stewardship checks). | implemented baseline | Scope is currently UB-T05-focused and policy-file driven; broader plugin/extensibility governance remains separate work. | `internal/tooling/conformance/governance.go`, `cmd/rspp-cli/main.go`, `Makefile` ([F-162], [F-163], [F-164], [NF-010], [NF-032]) |
| Plugin/extension host | No gate plugin host in tooling internals. | not started | Extensibility path exists at architecture level only. | folder inventory ([F-121], [F-122], [F-123], [F-124], [F-164]) |

## 1) Module list with responsibilities (target architecture)

| Target module | Path (target) | Responsibility | Current status | Next implementation slice | Feature evidence |
| --- | --- | --- | --- | --- | --- |
| Gate Orchestrator | `internal/tooling/gates` | Central gate profile execution and deterministic aggregation (`quick/full/mvp/security`). | not implemented | Extract orchestration from `cmd/rspp-cli`/Makefile into reusable package. | [F-126], [F-127], [NF-016] |
| Spec and Contract Validation | `internal/tooling/validation` | Contract + spec lint/validation with actionable diagnostics. | implemented baseline | Add cross-artifact compatibility lint (bundle/spec/policy coherence) while keeping strict per-file validators stable. | [F-001], [F-126], [F-028], [NF-015], [NF-010] |
| Replay Regression Engine | `internal/tooling/regression` | Deterministic divergence policy and replay regression blocking rules. | implemented baseline | Add replay input adapters (recorded timeline/provider playback modes) beyond fixture builders. | [F-127], [F-149], [F-150], [F-151], [F-152], [NF-009] |
| SLO Gate Engine | `internal/tooling/ops` | Canonical metric gate computation and threshold enforcement. | implemented baseline | Add explicit source typing (synthetic vs recorded vs live) and stricter source policy controls. | [NF-024], [NF-025], [NF-026], [NF-027], [NF-028], [NF-029], [NF-038] |
| Evidence and Artifact Ledger | `internal/tooling/evidence` | Artifact schema/version/hash/freshness/provenance management. | partial (logic spread across CLI + release) | Consolidate artifact IO into one module and normalize schema/version metadata. | [F-149], [F-153], [F-154], [F-176] |
| Release Readiness and Publish | `internal/tooling/release` | Fail-closed readiness + deterministic release manifest generation. | implemented baseline | Extend readiness policy inputs (optional mvp/security artifact checks) without breaking current publish path. | [F-093], [F-100], [NF-016], [NF-032], [F-165] |
| Local Runner Harness | `internal/tooling/runner` + `cmd/rspp-local-runner` | Local single-node runtime execution + simulation harness integration. | implemented baseline | Consolidate loopback/livekit orchestration into reusable runner abstraction to reduce command-surface duplication. | [F-125], [F-130], [F-078] |
| Authoring and Node Test Kit | `internal/tooling/authoring` | Node authoring scaffolds + deterministic node stream harness. | scaffold only | Implement harness APIs and fixture-driven node conformance tests. | [F-128], [F-121], [F-122], [F-123], [F-124] |
| Conformance and Compatibility Profiles | `internal/tooling/conformance` | Adapter/node/runtime conformance packs + skew/deprecation gate rules. | implemented baseline | Expand beyond MVP UB-T05 checks to additional post-MVP compatibility/deprecation governance flows. | [NF-032], [F-162], [F-163], [F-164] |
| Extension Host | `internal/tooling/extensions` | Plugin registration, capability declaration, guardrails, and execution budgets. | not implemented | Define plugin manifest + registration API, keep default no-plugin runtime. | [F-121], [F-122], [F-123], [F-124], [F-164] |

## 2) Key interfaces

### 2.1 Current concrete interfaces (already in code)

```go
// validation
ValidateContractFixtures(root string) (ContractValidationSummary, error)
ValidateContractFixturesWithSchema(schemaPath, root string) (ContractValidationSummary, error)

// regression
EvaluateDivergences(divergences []ReplayDivergence, policy DivergencePolicy) DivergenceEvaluation

// ops
EvaluateMVPSLOGates(samples []TurnMetrics, thresholds MVPSLOThresholds) MVPSLOGateReport
DefaultMVPSLOThresholds() MVPSLOThresholds

// release
LoadRolloutConfig(path string) (RolloutConfig, ArtifactSource, error)
EvaluateReadiness(in ReadinessInput) (ReadinessResult, map[string]ArtifactSource)
BuildReleaseManifest(specRef string, cfg RolloutConfig, readiness ReadinessResult, sources map[string]ArtifactSource, now time.Time) (ReleaseManifest, error)
```

### 2.2 Target interfaces (to add)

```go
type Gate interface {
    ID() string
    Stage() string // quick|full|mvp|security|publish
    Evaluate(ctx context.Context, in GateInput) (GateResult, error)
}

type GateOrchestrator interface {
    RunProfile(ctx context.Context, profile string, candidate ReleaseCandidate) (GateRun, error)
}

type EvidenceStore interface {
    Put(ctx context.Context, artifact EvidenceArtifact) error
    Get(ctx context.Context, id string) (EvidenceArtifact, error)
    VerifyFresh(ctx context.Context, id string, maxAge time.Duration) error
}

type GatePlugin interface {
    Manifest() PluginManifest
    Register(reg GateRegistry) error
}

type RuntimeAdapter interface {
    Name() string
    Start(ctx context.Context, specRef string, opts RunnerOptions) (RuntimeHandle, error)
    Stop(ctx context.Context, h RuntimeHandle) error
    CollectEvidence(ctx context.Context, h RuntimeHandle) ([]EvidenceArtifact, error)
}
```

## 3) Data model: core entities and relations

### 3.1 Implemented artifacts today

| Artifact | Purpose | Default path | Defined in |
| --- | --- | --- | --- |
| `contractsReportArtifact` | Contract fixture validation result | `.codex/ops/contracts-report.json` | `cmd/rspp-cli/main.go` |
| `replayRegressionReport` | Replay gate aggregate + fixture outcomes | `.codex/replay/regression-report.json` | `cmd/rspp-cli/main.go` |
| `sloGateArtifact` | Baseline SLO gate report | `.codex/ops/slo-gates-report.json` | `cmd/rspp-cli/main.go` |
| `mvpSLOGateArtifact` | Baseline + live + fixed-decision gate report | `.codex/ops/slo-gates-mvp-report.json` | `cmd/rspp-cli/main.go` |
| `liveLatencyCompareArtifact` | Streaming/non-streaming paired comparison | `.codex/providers/live-latency-compare.json` | `cmd/rspp-cli/main.go` |
| `ReleaseManifest` | Release handoff manifest | `.codex/release/release-manifest.json` | `internal/tooling/release/release.go` |

### 3.2 Target entities to standardize

| Entity | Purpose | Key fields |
| --- | --- | --- |
| `ReleaseCandidate` | Immutable input to gate/publish flow | `spec_ref`, `pipeline_version`, `execution_profile`, `rollout_config_ref`, `commit_sha` |
| `GateDefinition` | Declarative gate rule | `gate_id`, `metric_formula`, `threshold`, `scope`, `stage`, `required` |
| `GateRun` | One orchestrated execution | `run_id`, `profile`, `candidate_ref`, `status`, `started_at`, `ended_at` |
| `GateResult` | One gate result | `gate_id`, `status`, `violations[]`, `metrics`, `evidence_refs[]` |
| `EvidenceArtifact` | Versioned evidence blob | `artifact_id`, `kind`, `schema_version`, `sha256`, `generated_at_utc`, `payload_ref` |
| `ConformanceProfile` | Required checks by runtime/adapter class | `profile_id`, `required_gates[]`, `supported_versions[]` |
| `PluginManifest` | Plugin compatibility + safety contract | `plugin_id`, `api_version`, `capabilities[]`, `resource_limits`, `signature_ref` |

Core relations:
- `ReleaseCandidate -> GateRun -> GateResult -> EvidenceArtifact`
- `ReadinessDecision` is derived from `GateRun` + freshness policy
- `ReleaseManifest` is emitted only when readiness passes
- `ConformanceProfile` injects additional required gate definitions
- `PluginManifest` contributes optional gate/validator registration

## 4) Extension points: plugins, custom nodes, custom runtimes

### 4.1 Current extension seams
- Replay fixture metadata policy (`test/replay/fixtures/metadata.json`) controls gate membership, tolerances, and expected divergences.
- CLI artifact path overrides allow integration into alternate CI layouts.
- Live provider chain artifacts are consumed as external evidence for MVP/fairness gates.

### 4.2 Target extension seams
- Plugins: `GatePlugin` registration in `internal/tooling/extensions` with explicit compatibility and resource budgets ([F-164], [F-162], [F-163], [NF-032]).
- Custom nodes: authoring harness + conformance packs for in-process and external-node paths ([F-011], [F-121], [F-122], [F-123], [F-124], [F-128]).
- Custom runtimes: `RuntimeAdapter` boundary to run gates against local, cluster, or external execution backends while preserving canonical metrics/evidence ([F-125], [F-130], [F-078], [NF-032], [F-162]).

## 5) Design invariants (must preserve)

1. Required release artifacts fail closed on missing/read/decode/freshness failure ([F-100], [NF-016]).
2. Gate formulas for MVP metrics use canonical PRD anchors only ([NF-024], [NF-025], [NF-026], [NF-038]).
3. OR-02 completeness gate remains `100%` for accepted turns ([NF-028], [F-153]).
4. Stale-epoch accepted output count remains `0` ([NF-027], [F-095]).
5. Terminal lifecycle gate remains exactly one terminal then `close` ([NF-029]).
6. Replay divergence policy stays deterministic for equal inputs/policies ([NF-009], [F-150], [F-152]).
7. Invocation-latency threshold breaches are always replay-gate failures regardless of generic timing tolerance ([F-127], [NF-009]).
8. Timeline/export pressure cannot block control progression; replay-critical evidence remains preserved ([F-154], [F-176]).
9. MVP profile enforcement remains `simple`-only in gate policy ([F-078]).
10. Fixed-decision provider policy remains explicit and audited in reports ([F-100], [F-165]).
11. If live E2E waiver policy is used, waiver reason and attempt evidence must be recorded in artifact output ([NF-038], [NF-016]).
12. Release manifest must include source artifact integrity metadata (path + hash) ([F-100], [F-149]).
13. Validation errors must remain actionable and field-specific ([F-001], [F-126], [NF-015]).
14. New extension execution paths must enforce timeout/resource bounds ([F-123], [F-124]).
15. Version-skew/deprecation checks must be explicit and testable once introduced ([NF-032], [F-162], [F-163], [F-164], [F-165]).

## 6) Tradeoffs (major)

| Tradeoff | Current choice | Alternative | Why |
| --- | --- | --- | --- |
| Gate orchestration location | CLI + Makefile chains | Central in-module orchestrator | Current setup is simple and works, but duplicates orchestration logic and limits extension injection ([F-126], [F-127]). |
| Replay source model | Fixture-builder dominated | Full replay execution ingestion | Current model is deterministic and fast; real replay ingestion is needed for richer production fidelity ([F-127], [F-151], [NF-009]). |
| SLO baseline source | Generated runtime baseline defaults | Live/recorded-only baseline input | Synthetic baseline stabilizes CI, but can hide drift if not paired with recorded evidence policies ([NF-024], [NF-028], [NF-038]). |
| Live E2E policy | Threshold + explicit waiver path | Hard fail with no waiver | Waiver supports unstable live envs but weakens strict gate semantics if overused ([NF-038], [NF-016]). |
| Release readiness inputs | 3 required artifacts | Broader artifact set (security/mvp/compare) | Minimal readiness is robust today; broader policy gives stronger release confidence but adds coupling ([F-100], [NF-016], [F-165]). |
| Extensibility path | No plugin host yet | Early plugin-first architecture | Defers complexity now; delays tenant/domain-specific gate innovation ([F-121], [F-164]). |
| Local runner scope | LiveKit dry-run plus command-level loopback mode | Runtime adapter + loopback simulation suite | Current implementation satisfies MVP loopback intent with Event ABI parity checks, but still duplicates orchestration logic in command code ([F-125], [F-130]). |

## 7) Divergence matrix: current vs target

| Divergence | Current evidence | Impact | Implementation action |
| --- | --- | --- | --- |
| No reusable gate orchestrator module | `cmd/rspp-cli/main.go`, `Makefile` | Harder reuse/composition/plugin loading | Introduce `internal/tooling/gates` and move profile wiring into package API. |
| Spec/policy validation is file-scoped | `internal/tooling/validation/spec_policy.go`, `cmd/rspp-cli/main.go` | Cross-file policy/spec/bundle compatibility checks are not yet codified in one validation command. | Add cross-artifact compatibility validators and bundle-level lint commands. |
| Replay regression relies on fixture builders | `replayFixtureBuilders` in `cmd/rspp-cli/main.go` | Limited fidelity to recorded-session replay pathways | Add replay artifact ingestion adapter and keep fixture mode for deterministic tests. |
| Evidence logic is scattered | CLI artifact structs + release readers | Schema/version/freshness policy is duplicated | Create `internal/tooling/evidence` with normalized read/write/validate APIs. |
| Publish readiness omits mvp/security/codex policy artifacts | `internal/tooling/release/release.go` | Promotion readiness can be narrower than CI policy intent | Add optional readiness profile inputs and policy mode flags. |
| Authoring harness is not implemented | `internal/tooling/authoring/node_authoring_kit_guide.md` | Custom-node developer workflow is incomplete | Implement node stream harness + baseline fixtures + contract hooks. |
| Governance checks are policy-file driven | `internal/tooling/conformance` + `rspp-cli conformance-governance-report` | Post-MVP governance depth can still drift without richer runtime/plugin hooks | Extend governance inputs beyond static artifacts once extension host and broader compatibility layers are introduced. |
| No extension host | no `internal/tooling/extensions` | Cannot register custom gate plugins safely | Add plugin manifest, registration API, and bounded execution runner. |
| Runner abstraction missing | `cmd/rspp-local-runner` now includes livekit + loopback mode directly | Unified custom-runtime gate targeting remains harder to reuse | Add `internal/tooling/runner` interfaces and migrate command orchestration into shared adapter layer. |

## 8) Implementation-ready execution plan

### Phase P0-A: Gate orchestration extraction
- Scope:
  - create `internal/tooling/gates` (`orchestrator.go`, `profiles.go`, `types.go`)
  - move profile composition from CLI/Makefile into orchestrator package
  - keep CLI command UX stable
- Files:
  - `internal/tooling/gates/*` (new)
  - `cmd/rspp-cli/main.go`
  - `cmd/rspp-cli/main_test.go`
  - `Makefile` (only if command chain simplification is adopted)
- Exit criteria:
  - `verify-quick/full/mvp` behavior is unchanged
  - output artifact paths remain backward-compatible

### Phase P0-B: Validation completion for spec-first CI
- Scope:
  - add PipelineSpec/policy validation entrypoints under `internal/tooling/validation`
  - add CLI commands for lint/validate with actionable diagnostics
- Files:
  - `internal/tooling/validation/*`
  - `cmd/rspp-cli/main.go`
  - contract/spec fixtures under `test/contract` (and new spec fixture folder)
- Exit criteria:
  - invalid spec/policy returns field-path diagnostics
  - valid spec/policy passes strict mode

### Phase P0-C: Evidence normalization
- Scope:
  - add `internal/tooling/evidence` with typed artifact IO + schema/version metadata
  - refactor CLI report writers to use shared evidence package
- Files:
  - `internal/tooling/evidence/*` (new)
  - `cmd/rspp-cli/main.go`
  - `internal/tooling/release/release.go`
- Exit criteria:
  - no duplicated freshness/hash/version parsing logic
  - release readiness consumes shared evidence interfaces

### Phase P1-A: Replay ingestion upgrade
- Scope:
  - keep fixture mode for deterministic unit tests
  - add replay-result ingestion mode for regression reporting
- Files:
  - `internal/tooling/regression/*`
  - `cmd/rspp-cli/main.go`
  - `test/replay/*`
- Exit criteria:
  - replay report can be generated from either fixture mode or replay artifact mode
  - divergence policy behavior remains deterministic

### Phase P1-B: Conformance + extension foundations
- Scope:
  - add `internal/tooling/conformance` (`ConformanceProfile`, skew/deprecation checks)
  - add `internal/tooling/extensions` plugin manifest and guarded loader
- Files:
  - `internal/tooling/conformance/*` (new)
  - `internal/tooling/extensions/*` (new)
  - `cmd/rspp-cli/main.go` (registration/wiring)
- Exit criteria:
  - base conformance profile can run and fail deterministically
  - plugin registration is explicit and bounded

### Phase P1-C: Runner abstraction + loopback simulation
- Scope:
  - add `internal/tooling/runner` adapter contract
  - keep `rspp-local-runner` LiveKit path and add loopback simulation path
- Files:
  - `internal/tooling/runner/*` (new)
  - `cmd/rspp-local-runner/main.go`
  - `cmd/rspp-local-runner/main_test.go`
- Exit criteria:
  - offline loopback mode emits Event ABI-compatible artifacts
  - LiveKit mode behavior remains unchanged

### Common validation commands for each phase
- `GOCACHE=/tmp/go-build go test ./internal/tooling/... ./cmd/rspp-cli ./cmd/rspp-local-runner`
- `make verify-quick`
- `make verify-full`
- `make verify-mvp` (for changes touching live/mvp gate logic)
