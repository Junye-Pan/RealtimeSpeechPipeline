# RSPP MVP First Implementation Slice

## Status snapshot (2026-02-10, post retention-sweep operational hardening + replay-latency + CP distribution hardening)

This document remains the normative MVP contract and now includes an implementation snapshot.

Current milestone state (from current local workspace on `main` at commit `4d9bf2e`):
- Contract/schema validation, runtime arbiter/guard path, full failure matrix slices (`F1`-`F8`), and replay divergence gating paths are implemented.
- `RK-10`/`RK-11` provider contracts plus deterministic invocation/retry-switch/fallback behavior are implemented, including startup+CI provider-count enforcement (3-5 per modality), non-terminal attempt evidence promotion into terminal OR-02 baseline evidence, per-attempt/invocation latency fields in OR-02 invocation outcomes, optional non-terminal invocation snapshot append support (config-gated), and live-provider switch/fallback smoke coverage.
- Replay invocation-latency threshold gating is implemented with runtime-baseline-artifact extraction in replay regression (`cmd/rspp-cli replay-regression-report`), replacing synthetic threshold sample inputs.
- CP turn-start bundle seam is implemented for deterministic CP-derived plan inputs and snapshot provenance (`CP-01/02/03/04/05/07/08/09/10` services + runtime seam wiring in arbiter), including backend bootstrap wiring (`turnarbiter.NewWithControlPlaneBackends`), file/env/http-backed distribution adapter loading, per-service partial-backend fallback defaults, stale-snapshot error classification, CP-03 graph compile output threading, CP-05 pre-turn admission decision shaping, CP-07 lease authority gating, and integration coverage for custom/partial/stale pipeline-version/snapshot propagation.
- Security/data-handling baseline is implemented, including replay access controls, immutable replay-audit durable backend resolver paths (HTTP backend with ordered retry/failover plus JSONL fallback), tenant policy-resolver-backed retention enforcement seams, retention/deletion contract coverage with backend resolver wiring, distributed CP snapshot-sourced retention policy loading for `retention-sweep` (with deterministic fallback defaults), deterministic per-run/per-tenant/per-class sweep counters, and fail-fast policy-artifact validation with stable error taxonomy.
- `make verify-quick` and `make verify-full` invoke `scripts/verify.sh` with replay and SLO artifacts.
- CI enforces blocking `codex-artifact-policy`, `verify-quick`, `verify-full`, and `security-baseline` jobs; `live-provider-smoke` and `a2-runtime-live` remain non-blocking.

Normative requirements in sections 1-10 remain in force unless explicitly superseded in this file.

## 1. Objective

Define the first code-shippable slice of RSPP that is fully aligned with:
- `docs/rspp_SystemDesign.md`
- `docs/ModularDesign.md`

This MVP is intentionally narrow: it proves runtime correctness contracts, deterministic turn lifecycle semantics, and replay-critical evidence capture before broader feature expansion.

## 2. Fixed decisions for MVP

- Implementation language: Go.
- Transport path: LiveKit only (through RK-22/RK-23 boundaries).
- Provider set: deterministic multi-provider catalog per modality (3-5 STT, 3-5 LLM, 3-5 TTS) with pre-authorized retry/switch behavior.
  - STT: Deepgram, Google Speech-to-Text, AssemblyAI
  - LLM: Anthropic, Google Gemini, Cohere
  - TTS: ElevenLabs, Google Cloud Text-to-Speech, Amazon Polly
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
- Dynamic provider-routing policies beyond pre-authorized deterministic retry/switch/fallback actions.
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

## 10. Execution sequence and closure log

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
   - `main` branch protection requires `codex-artifact-policy`, `verify-quick`, and `verify-full` (strict mode, admins enforced).
   - `release/*` branch-protection rule requires `verify-full` (strict mode, admins enforced).

11. Implement RK-10/RK-11 provider manager + invocation slice:
   - `internal/runtime/provider/contracts/contracts.go`, `internal/runtime/provider/contracts/contracts_test.go`
   - `internal/runtime/provider/registry/registry.go`, `internal/runtime/provider/registry/registry_test.go`
   - `internal/runtime/provider/invocation/controller.go`, `internal/runtime/provider/invocation/controller_test.go`
   - `internal/runtime/executor/scheduler.go`, `internal/runtime/executor/scheduler_test.go` (scheduler/provider integration)

12. Add real-provider adapter bootstrap and non-blocking live smoke checks:
   - `providers/stt/*`, `providers/llm/*`, `providers/tts/*` (3 providers per modality)
   - `internal/runtime/provider/bootstrap/bootstrap.go`, `internal/runtime/provider/bootstrap/bootstrap_test.go`
   - `test/integration/provider_live_smoke_test.go` (build tag `liveproviders`)
   - `Makefile` target `live-provider-smoke`
   - `.github/workflows/verify.yml` non-blocking `live-provider-smoke` job

13. Add A.2 runtime real-provider validation matrix:
   - `test/integration/a2_runtime_live_test.go` (build tag `liveproviders`)
   - `Makefile` target `a2-runtime-live`
   - `.github/workflows/verify.yml` non-blocking `a2-runtime-live` job
   - artifacts: `.codex/providers/a2-runtime-live-report.json|.md`

14. Close security/data-handling baseline checklist:
   - `internal/runtime/transport/classification.go`, `internal/runtime/transport/classification_test.go`
   - `internal/security/policy/policy.go`, `internal/security/policy/policy_test.go`
   - `api/observability/types.go`, `internal/observability/replay/access.go`
   - `internal/observability/timeline/redaction.go`, `internal/observability/timeline/recorder.go`
   - `scripts/security-check.sh`, `Makefile` target `security-baseline-check`
   - `.github/workflows/verify.yml` blocking `security-baseline` job with artifacts

15. Enforce `.codex` generated artifact tracking policy:
   - `.gitignore` tracks only `.codex/skills/**` and `.codex/rules/**`.
   - `scripts/check-codex-artifact-policy.sh` and `Makefile` target `codex-artifact-policy-check`.
   - `.github/workflows/verify.yml` blocking `codex-artifact-policy` job, required before quick/full/security/live jobs.

16. Close prior 10.2 follow-up bundle (sync pass 2026-02-10):
   - Provider follow-up closure items were completed and reflected in A.2/A.3 evidence mappings.
   - CP turn-start seam/backend bootstrap plus file/env/http distribution adapter support were completed with deterministic fallback + stale-snapshot handling coverage.
   - Replay retention sweep entrypoint and replay invocation-latency threshold gating were completed and documented.

17. Harden service-client CP distribution beyond baseline adapters (2026-02-10):
   - added authenticated HTTP fetch path with bearer-token and client-identity headers (`RSPP_CP_DISTRIBUTION_HTTP_AUTH_BEARER_TOKEN`, `RSPP_CP_DISTRIBUTION_HTTP_CLIENT_ID`).
   - added ordered multi-endpoint failover (`RSPP_CP_DISTRIBUTION_HTTP_URLS`) with deterministic per-endpoint retry/backoff controls.
   - added on-demand TTL cache refresh with bounded stale-serving fallback (`cache_ttl` + `max_staleness`) and failover orchestration coverage for degraded/stale endpoint chains.

18. Productionize replay durability and policy distribution backends (2026-02-10):
   - added distributed replay-audit durable HTTP backend path with auth headers, deterministic retry/backoff, and ordered endpoint failover.
   - added JSONL tenant-scoped replay-audit fallback resolver composition for deterministic degraded operation when HTTP backends are unavailable.
   - moved retention policy sourcing for `rspp-runtime retention-sweep` from static artifact-first to CP distribution snapshot-first (`file`/`env`/`http`) with deterministic default-policy fallback and run-level source/fallback reporting.
   - added outage/recovery tests covering CP distribution snapshot fallback on backend outage and deterministic source recovery on subsequent runs.

19. Expand replay invocation-latency evidence coverage across runtime chains (2026-02-10):
   - extended replay fixture latency scope resolution beyond fixture-id derivation by adding metadata-driven scope overrides (`invocation_latency_scopes`) with deterministic normalization/dedup/sort fallback behavior.
   - expanded runtime-baseline artifact latency evidence generation beyond the prior `turn-rd-003` only scope by adding deterministic invocation outcomes for `turn-rd-002`, `turn-ae-001`, `turn-cf-001`, `turn-ml-001`, and `turn-ordering-approved-1`.
   - expanded replay fixture threshold assertions across more runtime chains (`rd-002`, `ae-001`, `cf-001`, `ml-001`) while keeping CI deterministic pass defaults.
   - added deterministic threshold behavior coverage for metadata-scope precedence, ordered multi-scope threshold divergence output, and explicit missing-evidence failure paths.

20. Promote CP modules `CP-01/03/04/05/07/08/09/10` toward MVP `implemented` gate (2026-02-10):
   - defined and satisfied promotion criteria requiring module-behavior correctness, service-client backend parity (`file`/`env`/`http`), deterministic failure-injection handling (backend-error fallback + stale-snapshot classification), and synchronized conformance evidence mappings.
   - expanded CP failure-injection integration coverage for rollout/policy/provider-health fallback defaults under backend outage (`TestCPBackendRolloutPolicyProviderHealthFailuresFallBackDeterministically`).
   - revalidated CP turn-start seam coverage for graph compile propagation, admission reject/defer shaping, lease-authority gating, partial-backend fallback behavior, and distribution adapter failover/retry/stale handling.
   - updated A.1 status rows and CI/conformance docs to reflect MVP-scope `implemented` status for promoted modules (advanced discovery/rotation/publisher/live-health capabilities remain explicitly deferred outside this MVP gate).

21. Promote CP module `CP-02` to MVP `implemented` gate (2026-02-11):
   - defined and satisfied promotion criteria requiring deterministic turn-start defaulting plus explicit MVP simple-mode profile validation (`execution_profile == simple`) in CP-02 normalization.
   - added CP-02 resolver/integration failure-path coverage for unsupported profiles and deterministic pre-turn outcome handling (`turn_start_bundle_resolution_failed`) under default defer and explicit reject policies.
   - revalidated CP turn-start backend parity coverage (`file`/`env`/`http`) with CP-02 normalization semantics in bundle resolver tests and end-to-end runtime chain assertions.
   - updated A.1 status row and CI/conformance docs to reflect MVP-scope `implemented` status for CP-02 while advanced profile customizations remain explicitly deferred outside this MVP gate.

22. Implement OR-01 telemetry pipeline and runtime instrumentation baseline (2026-02-11):
   - implemented bounded non-blocking telemetry pipeline (`internal/observability/telemetry/pipeline.go`) with deterministic debug-log sampling, in-memory sink test harness, and OTLP/HTTP exporter path.
   - wired runtime env configuration (`RSPP_TELEMETRY_*`) plus `rspp-runtime` startup integration for strict telemetry config parsing and lifecycle-safe default-emitter setup/teardown.
   - instrumented deterministic runtime chains (`turnarbiter`, `scheduler`, `provider invocation`, `transport fence`) for OTel-friendly `turn_span -> node_span -> provider_invocation_span`, plus stable metrics/log payload emission (`cancel_latency_ms`, `provider_rtt_ms`, `shed_rate`).
   - added focused telemetry coverage tests in `internal/observability/telemetry/*_test.go`, `internal/runtime/*/*_test.go`, and `cmd/rspp-runtime/main_test.go`, and promoted OR-01 into quick-gate package coverage (`Makefile`).

### 10.2 Remaining (open, ordered as of 2026-02-11)

Status update:
- This section tracks unfinished/partially finished/unstarted next-step work only.
- Completed closure items were moved to section `10.1`.

1. No open section-10 follow-up items remain after `10.1.22`; future post-MVP work should be tracked as new entries when scoped.

## Appendix A. MVP module status map (CP/RK/OR/DX)

Status values:
- `implemented`: concrete module code + tests for MVP behavior are present.
- `partial`: some MVP behavior is implemented, but full module boundary/coverage is incomplete.
- `scaffold-only`: directory/entrypoint exists with no substantive module logic.
- `deferred`: intentionally not implemented in this MVP slice.

### A.1 Control plane

| Module | Status | Evidence | Notes/Gap |
| --- | --- | --- | --- |
| CP-01 | implemented | `internal/controlplane/registry/registry.go`, `internal/controlplane/registry/registry_test.go`, `internal/controlplane/distribution/file_adapter.go`, `internal/controlplane/distribution/file_adapter_test.go`, `internal/controlplane/distribution/http_adapter.go`, `internal/controlplane/distribution/http_adapter_test.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/controlplane_backends.go`, `internal/runtime/turnarbiter/controlplane_backends_test.go`, `test/integration/runtime_chain_test.go` | Deterministic pipeline registry resolver behavior, file/env/http parity, and fallback/failure handling satisfy MVP promotion criteria; advanced endpoint discovery/rotation and push invalidation remain deferred outside this slice. |
| CP-02 | implemented | `internal/controlplane/normalizer/normalizer.go`, `internal/controlplane/normalizer/normalizer_test.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/controlplane_bundle_test.go`, `internal/runtime/turnarbiter/controlplane_backends.go`, `internal/runtime/turnarbiter/controlplane_backends_test.go`, `test/integration/runtime_chain_test.go` | Deterministic turn-start normalization/defaulting plus MVP simple-mode profile enforcement are implemented through runtime seam with deterministic unsupported-profile pre-turn handling and backend parity coverage; advanced profile customizations remain deferred outside this slice. |
| CP-03 | implemented | `internal/controlplane/graphcompiler/graphcompiler.go`, `internal/controlplane/graphcompiler/graphcompiler_test.go`, `internal/controlplane/distribution/file_adapter.go`, `internal/controlplane/distribution/http_adapter.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/controlplane_backends.go`, `internal/runtime/turnarbiter/controlplane_backends_test.go`, `test/integration/runtime_chain_test.go` | Deterministic graph-compile output propagation, distribution parity, and deterministic failure handling satisfy MVP promotion criteria; production distributed compiler hardening remains deferred outside this slice. |
| CP-04 | implemented | `internal/controlplane/policy/policy.go`, `internal/controlplane/policy/policy_test.go`, `internal/controlplane/distribution/file_adapter.go`, `internal/controlplane/distribution/file_adapter_test.go`, `internal/controlplane/distribution/http_adapter.go`, `internal/controlplane/distribution/http_adapter_test.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/controlplane_backends.go`, `internal/runtime/turnarbiter/controlplane_backends_test.go`, `test/integration/runtime_chain_test.go` | Deterministic policy snapshot/action defaults, distribution parity, and backend-outage fallback behavior satisfy MVP promotion criteria; dynamic policy rollout controls remain deferred outside this slice. |
| CP-05 | implemented | `internal/controlplane/admission/admission.go`, `internal/controlplane/admission/admission_test.go`, `internal/controlplane/distribution/file_adapter.go`, `internal/controlplane/distribution/http_adapter.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/arbiter.go`, `internal/runtime/turnarbiter/arbiter_test.go`, `test/integration/runtime_chain_test.go` | Deterministic CP admission decision shaping (`CP-05` emitter paths), distribution parity, and failure-path determinism satisfy MVP promotion criteria; dynamic distributed policy controls remain deferred outside this slice. |
| CP-07 | implemented | `internal/controlplane/lease/lease.go`, `internal/controlplane/lease/lease_test.go`, `internal/controlplane/distribution/file_adapter.go`, `internal/controlplane/distribution/http_adapter.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/arbiter.go`, `internal/runtime/turnarbiter/arbiter_test.go`, `test/integration/runtime_chain_test.go` | Deterministic lease-authority gating integration, distribution parity, and deterministic failure-path handling satisfy MVP promotion criteria; distributed lease orchestration hardening remains deferred outside this slice. |
| CP-08 | implemented | `internal/controlplane/routingview/routingview.go`, `internal/controlplane/routingview/routingview_test.go`, `internal/controlplane/distribution/file_adapter.go`, `internal/controlplane/distribution/file_adapter_test.go`, `internal/controlplane/distribution/http_adapter.go`, `internal/controlplane/distribution/http_adapter_test.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/controlplane_backends.go`, `internal/runtime/turnarbiter/controlplane_backends_test.go`, `test/integration/runtime_chain_test.go` | Deterministic routing/admission/ABI snapshot threading, distribution parity, and stale/fallback handling satisfy MVP promotion criteria; live snapshot publisher integration remains deferred outside this slice. |
| CP-09 | implemented | `internal/controlplane/rollout/rollout.go`, `internal/controlplane/rollout/rollout_test.go`, `internal/controlplane/distribution/file_adapter.go`, `internal/controlplane/distribution/file_adapter_test.go`, `internal/controlplane/distribution/http_adapter.go`, `internal/controlplane/distribution/http_adapter_test.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/controlplane_backends.go`, `internal/runtime/turnarbiter/controlplane_backends_test.go`, `test/integration/runtime_chain_test.go` | Deterministic turn-start version resolution, distribution parity, and backend-outage fallback determinism satisfy MVP promotion criteria; rollout policy/canary controls remain deferred outside this slice. |
| CP-10 | implemented | `internal/controlplane/providerhealth/providerhealth.go`, `internal/controlplane/providerhealth/providerhealth_test.go`, `internal/controlplane/distribution/file_adapter.go`, `internal/controlplane/distribution/file_adapter_test.go`, `internal/controlplane/distribution/http_adapter.go`, `internal/controlplane/distribution/http_adapter_test.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/controlplane_backends.go`, `internal/runtime/turnarbiter/controlplane_backends_test.go`, `test/integration/runtime_chain_test.go` | Deterministic provider-health snapshot resolution, distribution parity, and backend-outage fallback determinism satisfy MVP promotion criteria; live health aggregation backend remains deferred outside this slice. |

### A.2 Runtime

Real-provider validation coverage:
- `make a2-runtime-live` executes `TestLiveProviderSmoke` plus `TestA2RuntimeLiveScenarios`.
- `TestA2RuntimeLiveScenarios` maps all A.2 runtime modules (`RK-02` through `RK-26`) to explicit scenario assertions and emits `.codex/providers/a2-runtime-live-report.json|.md`.

| Module | Status | Evidence | Notes/Gap |
| --- | --- | --- | --- |
| RK-02 | implemented | `internal/runtime/prelude/engine.go`, `internal/runtime/prelude/engine_test.go`, `test/integration/runtime_chain_test.go` | Session prelude emits deterministic non-authoritative `turn_open_proposed` intents for arbiter turn-open gating. |
| RK-03 | implemented | `internal/runtime/turnarbiter/arbiter.go`, `internal/runtime/turnarbiter/arbiter_test.go` | Deterministic lifecycle path is present. |
| RK-04 | implemented | `internal/runtime/planresolver/resolver.go`, `internal/runtime/planresolver/resolver_test.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/controlplane_bundle_test.go` | Turn-plan materialization checks are present and now consume CP-resolved turn-start bundle defaults/provenance through the arbiter seam. |
| RK-05 | implemented | `api/eventabi/types.go`, `api/eventabi/types_test.go`, `internal/runtime/eventabi/gateway.go`, `internal/runtime/eventabi/gateway_test.go`, `internal/runtime/transport/fence.go`, `internal/runtime/nodehost/failure.go` | Runtime-side EventRecord/ControlSignal normalization and sequencing validation gateway is implemented and enforces payload-class presence at ABI boundary. |
| RK-06 | implemented | `internal/runtime/lanes/router.go`, `internal/runtime/lanes/router_test.go` | Deterministic lane router and route validation are implemented. |
| RK-07 | implemented | `internal/runtime/executor/scheduler.go`, `internal/runtime/executor/plan.go`, `internal/runtime/executor/scheduler_test.go`, `test/integration/runtime_chain_test.go` | Deterministic multi-node execution-plan ordering, lane dispatch, terminal reasoning, and failure-shaped continuation/stop behavior are implemented. |
| RK-08 | implemented | `internal/runtime/nodehost/failure.go`, `internal/runtime/nodehost/failure_test.go`, `internal/runtime/executor/plan.go`, `internal/runtime/executor/scheduler_test.go` | Node failure shaping is implemented and integrated into execution-plan flow with deterministic degrade/fallback/terminal control-signal outcomes. |
| RK-10 | implemented | `internal/runtime/provider/contracts/contracts.go`, `internal/runtime/provider/contracts/contracts_test.go`, `internal/runtime/provider/registry/registry.go`, `internal/runtime/provider/registry/registry_test.go`, `internal/runtime/provider/bootstrap/bootstrap.go`, `internal/runtime/provider/bootstrap/bootstrap_test.go`, `providers/stt/*`, `providers/llm/*`, `providers/tts/*`, `test/integration/provider_live_smoke_test.go` | Deterministic provider contracts, registry/bootstrap, and request-policy envelope validation (adaptive actions/retry budget/candidate count) are implemented. |
| RK-11 | implemented | `internal/runtime/provider/invocation/controller.go`, `internal/runtime/provider/invocation/controller_test.go`, `internal/runtime/executor/scheduler.go`, `internal/runtime/executor/scheduler_test.go`, `internal/observability/timeline/recorder.go`, `internal/observability/timeline/recorder_test.go`, `test/integration/provider_live_smoke_test.go`, `test/integration/runtime_chain_test.go` | Invocation attempt/retry/switch/fallback policy gating and deterministic signal emission are implemented with attempt-level timeline persistence and integration coverage. |
| RK-12 | implemented | `internal/runtime/buffering/drop_notice.go`, `internal/runtime/buffering/drop_notice_test.go`, `internal/runtime/buffering/merge.go`, `internal/runtime/buffering/merge_test.go`, `test/failover/failure_full_test.go` | Deterministic buffering/lineage behavior present. |
| RK-13 | implemented | `internal/runtime/buffering/pressure.go`, `internal/runtime/buffering/pressure_test.go`, `test/failover/failure_full_test.go` | Watermark/pressure behavior covered. |
| RK-14 | implemented | `internal/runtime/flowcontrol/controller.go`, `internal/runtime/flowcontrol/controller_test.go`, `internal/runtime/buffering/pressure.go`, `internal/runtime/buffering/pressure_test.go` | Dedicated RK-14 flow-control controller emits deterministic `flow_xoff`/`flow_xon`/`credit_grant` signals and is integrated with pressure handling. |
| RK-16 | implemented | `internal/runtime/transport/fence.go`, `internal/runtime/transport/fence_test.go`, `internal/runtime/cancellation/fence.go`, `internal/runtime/cancellation/fence_test.go`, `test/integration/runtime_chain_test.go` | Cancellation module and transport fence integration are implemented with deterministic post-cancel output suppression coverage. |
| RK-17 | implemented | `internal/runtime/budget/manager.go`, `internal/runtime/budget/manager_test.go`, `internal/runtime/nodehost/failure.go`, `internal/runtime/nodehost/failure_test.go` | Budget manager provides deterministic continue/degrade/fallback/terminate decisions and is integrated into node-failure shaping. |
| RK-19 | implemented | `internal/runtime/determinism/service.go`, `internal/runtime/determinism/service_test.go`, `internal/runtime/planresolver/resolver.go`, `internal/runtime/planresolver/resolver_test.go` | Determinism service issues and validates deterministic context (seed/order markers/merge rule) for resolved turn plans. |
| RK-21 | implemented | `internal/runtime/identity/context.go`, `internal/runtime/identity/context_test.go`, `internal/runtime/executor/scheduler.go`, `internal/runtime/executor/scheduler_test.go` | Identity/correlation/idempotency context service is implemented and used for deterministic event-id generation in scheduler paths. |
| RK-22 | implemented | `internal/runtime/transport/fence.go`, `internal/runtime/transport/fence_test.go`, `internal/runtime/transport/classification.go`, `internal/runtime/transport/classification_test.go`, `test/integration/cf_full_conformance_test.go`, `test/integration/runtime_chain_test.go` | Transport boundary behavior includes deterministic ingress payload classification tagging plus output fencing guarantees. |
| RK-23 | implemented | `internal/runtime/transport/signals.go`, `internal/runtime/transport/signals_test.go`, `test/integration/cf_full_conformance_test.go`, `test/integration/ml_conformance_test.go` | Connection and transport signal handling present. |
| RK-24 | implemented | `internal/runtime/guard/guard.go`, `internal/runtime/guard/enrichment.go`, `internal/runtime/guard/enrichment_test.go`, `internal/runtime/guard/migration.go`, `internal/runtime/guard/migration_test.go`, `test/integration/runtime_chain_test.go` | Authority checks and migration guard behavior present. |
| RK-25 | implemented | `internal/runtime/localadmission/localadmission.go`, `internal/runtime/localadmission/localadmission_test.go`, `internal/runtime/executor/scheduler_test.go`, `test/integration/runtime_chain_test.go` | Deterministic local admission outcomes are implemented. |
| RK-26 | implemented | `internal/runtime/executionpool/pool.go`, `internal/runtime/executionpool/pool_test.go`, `internal/runtime/executor/plan.go`, `internal/runtime/executor/scheduler_test.go` | Deterministic bounded FIFO execution pool manager is implemented with optional executor dispatch integration. |

### A.3 Observability and replay

| Module | Status | Evidence | Notes/Gap |
| --- | --- | --- | --- |
| OR-01 | implemented | `internal/observability/telemetry/pipeline.go`, `internal/observability/telemetry/pipeline_test.go`, `internal/observability/telemetry/env.go`, `internal/observability/telemetry/env_test.go`, `internal/observability/telemetry/otlp_http.go`, `internal/observability/telemetry/otlp_http_test.go`, `internal/observability/telemetry/memory_sink.go`, `internal/runtime/turnarbiter/arbiter.go`, `internal/runtime/turnarbiter/arbiter_telemetry_test.go`, `internal/runtime/executor/scheduler.go`, `internal/runtime/executor/scheduler_test.go`, `internal/runtime/provider/invocation/controller.go`, `internal/runtime/provider/invocation/controller_test.go`, `internal/runtime/transport/fence.go`, `internal/runtime/transport/fence_test.go`, `cmd/rspp-runtime/main.go`, `cmd/rspp-runtime/main_test.go`, `Makefile` | Bounded non-blocking telemetry pipeline is implemented with deterministic debug-log sampling, OTLP/HTTP + in-memory sink paths, runtime env wiring, and OTel-friendly runtime instrumentation (`turn_span`, `node_span`, `provider_invocation_span`) including stable metric/log emission for scheduling, provider invocation, and cancellation fence paths. |
| OR-02 | implemented | `internal/observability/timeline/recorder.go`, `internal/observability/timeline/recorder_test.go`, `internal/observability/timeline/redaction.go`, `internal/observability/timeline/redaction_test.go`, `internal/observability/timeline/artifact.go`, `internal/runtime/turnarbiter/arbiter.go`, `internal/runtime/turnarbiter/arbiter_test.go`, `internal/runtime/executor/scheduler.go`, `internal/runtime/executor/scheduler_test.go`, `test/replay/rd002_rd003_rd004_test.go` | Baseline timeline recording includes payload classification tags, persisted redaction decisions, terminal baseline promotion of invocation outcomes synthesized from non-terminal provider attempt evidence, deterministic invocation latency fields (`final_attempt_latency_ms`, `total_invocation_latency_ms`), and optional non-terminal invocation snapshot append path (config-gated). |
| OR-03 | implemented | `internal/observability/replay/comparator.go`, `internal/observability/replay/access.go`, `internal/observability/replay/access_test.go`, `internal/observability/replay/retention.go`, `internal/observability/replay/retention_backend.go`, `internal/observability/replay/retention_backend_test.go`, `internal/observability/replay/service.go`, `internal/observability/replay/service_test.go`, `internal/observability/replay/audit_backend.go`, `internal/observability/replay/audit_backend_http.go`, `internal/observability/replay/audit_backend_http_test.go`, `internal/controlplane/distribution/retention_snapshot.go`, `api/observability/types.go`, `test/replay/*`, `cmd/rspp-cli/main.go`, `cmd/rspp-cli/main_test.go`, `cmd/rspp-runtime/main.go`, `cmd/rspp-runtime/main_test.go` | Replay divergence comparison/reporting is implemented, with deny-by-default replay access schema, immutable audit sink durable backend resolver paths (HTTP + JSONL fallback), backend-policy resolver seams for retention enforcement, CP distribution snapshot-first retention policy resolution with deterministic fallback defaults in `retention-sweep`, artifact-derived invocation-latency threshold gating in replay regression, and concrete scheduled retention sweep operational enforcement. |

### A.4 Tooling and DevEx

| Module | Status | Evidence | Notes/Gap |
| --- | --- | --- | --- |
| DX-01 | implemented | `cmd/rspp-local-runner/main.go` | Local runner entrypoint exists for MVP workflow. |
| DX-02 | implemented | `internal/tooling/validation/contracts.go`, `test/contract/*`, `cmd/rspp-cli validate-contracts` | Contract validation harness is active. |
| DX-03 | implemented | `internal/tooling/regression/divergence.go`, `test/replay/*`, `cmd/rspp-cli replay-*` | Replay regression harness is active. |
| DX-04 | partial | `cmd/rspp-cli/main.go`, `internal/tooling/release/.gitkeep` | CLI release/report commands exist; release module remains scaffold. |
| DX-05 | implemented | `internal/tooling/ops/slo.go`, `cmd/rspp-cli slo-gates-report`, `Makefile` verify targets | SLO report generation is present and wired into quick/full verify flows. |

## Appendix B. Follow-up references (mapped to section 10)

1. Keep `docs/CIValidationGates.md` and `docs/ConformanceTestPlan.md` synchronized with `Makefile`, `.github/workflows/verify.yml`, and `.codex` artifact policy changes as section-10 scope evolves.
2. Replay durability/policy distribution backend productionization scope is closed in `10.1.18` (distributed durable sink + distributed policy snapshots + outage/recovery fallback coverage).
3. Replay invocation-latency evidence expansion scope is closed in `10.1.19` (artifact-derived extraction coverage growth beyond baseline fixture scope + deterministic threshold behavior across expanded fixture sets).
4. CP module promotion-to-implemented gate scope is closed in `10.1.20` + `10.1.21` (CP-01/02/03/04/05/07/08/09/10: defined promotion criteria + backend parity + deterministic failure-path coverage + conformance evidence synchronization).
5. OR-01 telemetry pipeline implementation scope is closed in `10.1.22` (bounded non-blocking telemetry pipeline + runtime instrumentation + env-wired runtime bootstrap + targeted coverage and gate synchronization).
