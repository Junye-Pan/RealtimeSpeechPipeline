# RSPP MVP First Implementation Slice

## Status snapshot (2026-02-08)

This document remains the normative MVP contract and now includes an implementation snapshot.

Current milestone state (from `.codex/sessions/2026-02-08-scaffold-contract-verify-20260208.tmp` and current repository state):
- Contract/schema validation, verify wiring, and runtime arbiter/guard path were completed.
- Full failure matrix slices (`F1`-`F8`) and replay divergence gating paths were completed.
- `make verify-quick` and `make verify-full` invoke `scripts/verify.sh` with replay and SLO artifacts.

Normative requirements in sections 1-10 remain in force unless explicitly superseded in this file.

## 1. Objective

Define the first code-shippable slice of RSPP that is fully aligned with:
- `docs/rspp_SystemDesign.md`
- `docs/ModularDesign.md`

This MVP is intentionally narrow: it proves runtime correctness contracts, deterministic turn lifecycle semantics, and replay-critical evidence capture before broader feature expansion.

## 2. Fixed decisions for MVP

- Implementation language: Go.
- Transport path: LiveKit only (through RK-22/RK-23 boundaries).
- Provider set: one STT, one LLM, one TTS adapter path.
- Mode: Simple mode only (ExecutionProfile defaults required; advanced overrides deferred).
- Authority model: single-region execution authority with lease/epoch checks.
- Replay scope: OR-02 baseline replay evidence (L0 baseline contract) is mandatory.

## 3. In scope

1. Spec-first runtime flow for one pipeline version:
   - `PipelineSpec` normalization
   - `GraphDefinition` compile/load
   - turn-start `ResolvedTurnPlan` freeze

2. Deterministic turn lifecycle with authoritative boundaries:
   - `turn_open_proposed` (internal intent)
   - `turn_open`
   - exactly one terminal `commit` or `abort(reason)`
   - `close`

3. Runtime-local hot-path admission and authority checks:
   - RK-25 deterministic `admit/reject/defer`
   - RK-24 deterministic `stale_epoch_reject` and `deauthorized_drain`

4. ControlLane preemption and cancellation fencing:
   - cancel propagation through runtime/provider/transport boundary
   - post-cancel output fencing on transport egress

5. OR-02 baseline evidence capture with non-blocking Stage-A append:
   - baseline evidence requirements from `docs/ModularDesign.md` section 6
   - deterministic failure policy when baseline evidence cannot be preserved

6. OR-03 minimal replay mode support:
   - replay from OR-02 baseline with divergence classification output

## 4. Out of scope (deferred)

- Multi-region active-active runtime routing.
- Advanced mode per-edge overrides and profile customizations.
- Multiple providers per modality with in-turn adaptive switching logic beyond pre-authorized defaults.
- Full L1/L2 recording fidelity as production requirement.
- External node execution isolation hardening (sandbox/WASM depth).
- Transport implementations beyond LiveKit.

## 5. MVP module subset (must implement first)

Control plane:
- CP-01, CP-02, CP-03, CP-04, CP-05, CP-07, CP-08, CP-09, CP-10

Runtime:
- RK-02, RK-03, RK-04, RK-05, RK-06, RK-07, RK-08, RK-10, RK-11, RK-12, RK-13, RK-14, RK-16, RK-17, RK-19, RK-21, RK-22, RK-23, RK-24, RK-25, RK-26

Observability/replay:
- OR-01 (best-effort), OR-02 (baseline required), OR-03 (minimum replay and divergence output)

Tooling:
- DX-01, DX-02, DX-03, DX-04, DX-05 (minimum CI/ops readiness)

## 6. Required contract artifacts before coding

1. Event ABI schema and envelope contract (with lane, sequencing, idempotency, authority markers).
2. ControlLane signal registry with owner module mapping.
3. Turn lifecycle state machine and deterministic transition guards.
4. `ResolvedTurnPlan` schema including frozen snapshot provenance references.
5. Admission and authority outcome schema (`admit/reject/defer`, `stale_epoch_reject`, `deauthorized_drain`).
6. OR-02 baseline evidence schema and append semantics (Stage-A/Stage-B split).

Status note:
- Core artifacts above now exist in-repo and are exercised by contract fixtures/tests (`test/contract/*`) and CLI validation (`cmd/rspp-cli validate-contracts`).

## 7. Simple mode defaults for MVP

Simple mode defaults MUST be explicit and deterministic:
- Effective BufferSpec for every edge (profile-derived allowed).
- DataLane default flow-control mode: `signal`.
- No hidden blocking: bounded `max_block_time` and deterministic shedding path.
- Turn-start snapshot validity behavior: deterministic `defer` or `reject`.
- Recording level default: L0 baseline replay evidence.

## 8. MVP quality gates (decided SLO/SLI targets)

1. Turn-open decision latency:
   - p95 <= 120 ms from `turn_open_proposed` acceptance point to `turn_open` emission.

2. First assistant output latency:
   - p95 <= 1500 ms from `turn_open` to first DataLane output chunk on the happy path.

3. Cancellation fence latency:
   - p95 <= 150 ms from cancel acceptance to egress output fencing.

4. Authority safety:
   - 0 accepted stale-epoch outputs in failover/lease-rotation tests.

5. Replay baseline completeness:
   - 100% of accepted turns include required OR-02 baseline evidence fields.

6. Terminal lifecycle correctness:
   - 100% of accepted turns emit exactly one terminal (`commit` or `abort`) followed by `close`.

Status note:
- Gate reports are generated via CLI targets wired in `Makefile` (`replay-smoke-report`, `replay-regression-report`, `generate-runtime-baseline`, `slo-gates-report`).

## 9. MVP test matrix (minimum)

1. Contract tests:
   - ABI validation, signal registry, state machine transition legality.

2. Turn lifecycle tests:
   - happy path commit
   - pre-turn `defer`/`reject`
   - pre-turn `stale_epoch_reject`
   - active-turn authority loss with `deauthorized_drain`

3. Cancellation tests:
   - barge-in/cancel propagation and egress fencing guarantees

4. Replay tests:
   - OR-02 baseline evidence presence
   - OR-03 divergence classes (`ORDERING`, `PLAN`, `OUTCOME`, `TIMING`, `AUTHORITY`)

5. Failure-injection tests:
   - provider timeout/overload
   - OR-02 baseline evidence pressure/failure path
   - lease epoch rotation and stale output rejection

Status note:
- Smoke and full failure matrices are covered by `test/failover/failure_smoke_test.go` and `test/failover/failure_full_test.go`.

## 10. Execution sequence for remaining MVP closure

### 10.1 Completed (evidence-linked)

1. Freeze MVP artifacts and schemas:
   - `docs/ContractArtifacts.schema.json`
   - `internal/tooling/validation/contracts.go`
   - `test/contract/schema_validation_test.go`

2. Scaffold Go module boundaries for MVP subset:
   - `cmd/rspp-control-plane/main.go`
   - `cmd/rspp-runtime/main.go`
   - `cmd/rspp-local-runner/main.go`
   - `internal/controlplane/*`
   - `internal/runtime/*`
   - `internal/observability/*`

3. Implement turn-start gating path:
   - `internal/runtime/localadmission/localadmission.go` (RK-25)
   - `internal/runtime/guard/guard.go` and `internal/runtime/guard/migration.go` (RK-24)
   - `internal/runtime/planresolver/resolver.go` (RK-04)
   - `internal/runtime/turnarbiter/arbiter.go` (RK-03)

4. Add replay and gate-report generation in CLI:
   - `cmd/rspp-cli/main.go`
   - `.codex/replay/*.json|*.md` generated by CLI commands

5. Wire verify gates:
   - `Makefile` targets `verify-quick` and `verify-full`
   - `scripts/verify.sh` invocation with explicit command chains

6. Wire CI artifact publication and gate-failure propagation:
   - `.github/workflows/verify.yml` uploads replay and SLO artifacts for both `verify-quick` and `verify-full`.
   - `scripts/verify.sh` and verify command chains fail fast on non-zero exit.
   - `cmd/rspp-cli/main.go` returns non-zero on replay/SLO gate violations.

7. Expand failure matrix coverage:
   - `internal/runtime/nodehost/failure.go` (F2)
   - `internal/runtime/buffering/pressure.go` (F4/F5)
   - `internal/runtime/transport/signals.go` (F6)
   - `internal/runtime/guard/migration.go` (F8)
   - `test/failover/failure_full_test.go` (F1-F8)

8. Harden CI artifact publication:
   - `.github/workflows/verify.yml` uses strict artifact checks (`if-no-files-found: error`).
   - quick/full jobs publish artifact sets aligned to files produced by each gate.

9. Expand replay regression artifacts for release diagnostics:
   - `replay-regression-report` emits per-fixture reports under `.codex/replay/fixtures/*.json|*.md`.
   - full CI uploads per-fixture replay artifacts.

10. Enforce repository required checks (external settings):
   - `main` branch protection requires `verify-quick` and `verify-full` (strict mode, admins enforced).
   - `release/*` branch-protection rule requires `verify-full` (strict mode, admins enforced).

### 10.2 Remaining (ordered, post doc-sync 2026-02-09)

Status update:
- Documentation synchronization follow-up is complete and tracked in Appendix B.
- No open remaining items in section 10.2 as of the 2026-02-09 closure pass.

## Appendix A. MVP module status map (CP/RK/OR/DX)

Status values:
- `implemented`: concrete module code + tests for MVP behavior are present.
- `partial`: some MVP behavior is implemented, but full module boundary/coverage is incomplete.
- `scaffold-only`: directory/entrypoint exists with no substantive module logic.
- `deferred`: intentionally not implemented in this MVP slice.

### A.1 Control plane

| Module | Status | Evidence | Notes/Gap |
| --- | --- | --- | --- |
| CP-01 | scaffold-only | `internal/controlplane/registry/.gitkeep` | Contract/types exist under `api/`, but registry implementation is not built out. |
| CP-02 | scaffold-only | `internal/controlplane/normalizer/.gitkeep` | Normalization behavior is represented in docs/tests, not full CP service implementation. |
| CP-03 | scaffold-only | `internal/controlplane/graphcompiler/.gitkeep` | Compiler module remains scaffold. |
| CP-04 | scaffold-only | `internal/controlplane/policy/.gitkeep` | Policy engine runtime stubs only. |
| CP-05 | scaffold-only | `internal/controlplane/admission/.gitkeep` | Runtime local admission (RK-25) exists; CP policy service remains scaffold. |
| CP-07 | scaffold-only | `internal/controlplane/lease/.gitkeep` | Lease authority service remains scaffold. |
| CP-08 | scaffold-only | `internal/controlplane/routingview/.gitkeep` | Routing view publisher remains scaffold. |
| CP-09 | scaffold-only | `internal/controlplane/rollout/.gitkeep` | Rollout/version resolver remains scaffold. |
| CP-10 | scaffold-only | `internal/controlplane/providerhealth/.gitkeep` | Provider health aggregator remains scaffold. |

### A.2 Runtime

| Module | Status | Evidence | Notes/Gap |
| --- | --- | --- | --- |
| RK-02 | scaffold-only | `internal/runtime/prelude/.gitkeep` | Session prelude engine remains scaffold. |
| RK-03 | implemented | `internal/runtime/turnarbiter/arbiter.go`, `internal/runtime/turnarbiter/arbiter_test.go` | Deterministic lifecycle path is present. |
| RK-04 | implemented | `internal/runtime/planresolver/resolver.go`, `internal/runtime/planresolver/resolver_test.go` | Turn-plan materialization checks present. |
| RK-05 | partial | `api/eventabi/types.go`, `api/eventabi/types_test.go`, `internal/runtime/eventabi/.gitkeep` | ABI contracts/validation exist; runtime gateway module is not fully fleshed out. |
| RK-06 | scaffold-only | `internal/runtime/lanes/.gitkeep` | Lane router remains scaffold. |
| RK-07 | partial | `internal/runtime/executor/scheduler.go`, `internal/runtime/executor/scheduler_test.go` | Execution scheduler subset exists; full graph runtime execution path remains incomplete. |
| RK-08 | partial | `internal/runtime/nodehost/failure.go`, `internal/runtime/nodehost/failure_test.go` | Failure handling present; broader node host surface remains limited. |
| RK-10 | scaffold-only | `internal/runtime/provider/registry/.gitkeep`, `internal/runtime/provider/contracts/.gitkeep` | Provider manager/contracts remain scaffold. |
| RK-11 | scaffold-only | `internal/runtime/provider/invocation/.gitkeep` | Invocation controller not yet implemented. |
| RK-12 | implemented | `internal/runtime/buffering/drop_notice.go`, `internal/runtime/buffering/merge.go`, tests | Deterministic buffering/lineage behavior present. |
| RK-13 | implemented | `internal/runtime/buffering/pressure.go`, tests | Watermark/pressure behavior covered. |
| RK-14 | scaffold-only | `internal/runtime/flowcontrol/.gitkeep` | Dedicated flow-control engine not implemented beyond signal contracts. |
| RK-16 | partial | `internal/runtime/transport/fence.go`, `internal/runtime/transport/fence_test.go`, `internal/runtime/cancellation/.gitkeep` | Cancellation fencing exists at transport boundary; standalone cancellation module remains scaffold. |
| RK-17 | scaffold-only | `internal/runtime/budget/.gitkeep` | Budget manager remains scaffold. |
| RK-19 | scaffold-only | `internal/runtime/determinism/.gitkeep` | Determinism service remains scaffold; replay tooling exists separately (OR-03/DX-03). |
| RK-21 | scaffold-only | `internal/runtime/identity/.gitkeep` | Identity/correlation service remains scaffold. |
| RK-22 | implemented | `internal/runtime/transport/fence.go`, tests | Transport output fencing and contract-safe boundary behavior present. |
| RK-23 | implemented | `internal/runtime/transport/signals.go`, tests | Connection and transport signal handling present. |
| RK-24 | implemented | `internal/runtime/guard/guard.go`, `internal/runtime/guard/migration.go`, tests | Authority checks and migration guard behavior present. |
| RK-25 | implemented | `internal/runtime/localadmission/localadmission.go`, tests | Deterministic local admission outcomes are implemented. |
| RK-26 | scaffold-only | `internal/runtime/executionpool/.gitkeep` | Execution pool manager remains scaffold. |

### A.3 Observability and replay

| Module | Status | Evidence | Notes/Gap |
| --- | --- | --- | --- |
| OR-01 | scaffold-only | `internal/observability/telemetry/.gitkeep` | Telemetry pipeline module is not implemented in this slice. |
| OR-02 | implemented | `internal/observability/timeline/recorder.go`, `internal/observability/timeline/artifact.go`, tests | Baseline timeline recording and artifact modeling are present. |
| OR-03 | implemented | `internal/observability/replay/comparator.go`, `test/replay/*`, `cmd/rspp-cli/main.go` | Replay divergence comparison and report generation are implemented. |

### A.4 Tooling and DevEx

| Module | Status | Evidence | Notes/Gap |
| --- | --- | --- | --- |
| DX-01 | implemented | `cmd/rspp-local-runner/main.go` | Local runner entrypoint exists for MVP workflow. |
| DX-02 | implemented | `internal/tooling/validation/contracts.go`, `test/contract/*`, `cmd/rspp-cli validate-contracts` | Contract validation harness is active. |
| DX-03 | implemented | `internal/tooling/regression/divergence.go`, `test/replay/*`, `cmd/rspp-cli replay-*` | Replay regression harness is active. |
| DX-04 | partial | `cmd/rspp-cli/main.go`, `internal/tooling/release/.gitkeep` | CLI release/report commands exist; release module remains scaffold. |
| DX-05 | implemented | `internal/tooling/ops/slo.go`, `cmd/rspp-cli slo-gates-report`, `Makefile` verify targets | SLO report generation is present and wired into quick/full verify flows. |

## Appendix B. Known follow-ups outside this pass

1. `docs/CIValidationGates.md` and `docs/ConformanceTestPlan.md` were synchronized in the 2026-02-09 doc pass; keep them aligned with command/test changes in future slices.
2. `docs/SecurityDataHandlingBaseline.md` checklist items remain open and should be planned as a separate security-focused slice.
