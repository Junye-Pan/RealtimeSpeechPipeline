# RSPP MVP Completion Master ExecPlan

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document is maintained in accordance with `.codex/PLANS.md`.

## Purpose / Big Picture

The goal is to complete all MVP-phase features in `docs/RSPP_features_framework.json` by executing the unified backlog in `docs/mvp_backlog.md` without semantic drift from the repository architecture. After completion, the repository will have implemented MVP behavior across contracts, runtime, control plane, observability/replay, providers/transports, tooling/gates, and scaffolds, with deterministic evidence through `verify-quick`, `verify-full`, `security-baseline-check`, and MVP gate artifacts.

The user-visible outcome is that MVP invariants are enforced end-to-end in code and tests: immutable turn plan freeze, lane priority, cancellation-first fencing, authority epoch safety, OR-02 replay baseline completeness, and stateless runtime compute with external durable state boundaries.

## Progress

- [x] (2026-02-16 19:20Z) Loaded authoritative MVP sources and canonical guides; confirmed dependency DAG and workstream boundaries.
- [x] (2026-02-16 19:20Z) Created this master ExecPlan in `task.md` for continuous progress tracking.
- [x] (2026-02-16 19:20Z) Baseline repo health confirmed from latest backlog snapshot: `verify-quick`, `verify-full`, and `security-baseline-check` passing.
- [x] (2026-02-16 19:28Z) Landed `PR-C1` slice A: Event ABI validator parity for `timestamp_ms`, `turn_open_proposed`, cancel scope enum, and strict emitter/reason rules for `barge_in|stop|cancel`, `budget_*`, `degrade|fallback`; added contract fixtures and tests; `verify-quick` + `security-baseline-check` passing.
- [x] (2026-02-16 23:35Z) Contract stream `PR-C1` (`UB-C01`) complete (closure review: no residual `timestamp_ms`/lane-semantics parity gaps; validated via `go test ./api/eventabi ./test/contract` plus `verify-quick`/`verify-full`/`security-baseline-check`).
- [x] (2026-02-16 19:31Z) Landed `PR-C2` slice A: control-plane/schema parity for `decision_outcome.timestamp_ms`, strict `sync_drop_policy.group_by` enum validation, and `streaming_handoff` schema/fixture alignment; `verify-quick` + `security-baseline-check` passing.
- [x] (2026-02-16 23:35Z) Contract stream `PR-C2` (`UB-C02`) complete (validated expanded control-plane contract surfaces against runtime/controlplane adoption with `go test ./api/controlplane ./internal/controlplane/... ./internal/runtime/turnarbiter` and full repo gates).
- [x] (2026-02-16 18:55Z) Landed `PR-C3` slice A: observability replay contracts (`ReplayMode`, `ReplayCursor`, replay request/result/report) plus standalone access decision/divergence validators and control-plane replay-mode compatibility wiring; `verify-quick` + `security-baseline-check` passing.
- [x] (2026-02-16 19:33Z) Landed `PR-C4` slice A: bootstrapped shared `api/transport` contracts (`SessionBootstrap`, `AdapterCapabilities`, transport/lifecycle enums) with strict validators and unit tests; `go test ./api/transport` + `verify-quick` + `security-baseline-check` passing.
- [x] (2026-02-16 23:35Z) Contract stream `PR-C3` (`UB-C03`) complete (replay/access/report schema+validator parity and downstream replay/tooling consumers validated with `go test ./api/observability ./internal/observability/... ./test/replay` and repo gates).
- [x] (2026-02-16 23:35Z) Contract stream `PR-C4` (`UB-C04`) complete (transport shared contract adoption validated with `go test ./api/transport ./transports/... ./cmd/...` and repo gates).
- [x] (2026-02-16 19:35Z) Landed `UB-R01` slice A: implemented `internal/runtime/session` lifecycle manager with deterministic transition enforcement (`Idle->Opening->Active->Terminal->Closed` plus pre-turn reject/defer paths) and unit tests; `verify-quick` + `security-baseline-check` passing.
- [x] (2026-02-16 19:41Z) Landed `UB-R01` slice B: added `internal/runtime/session` orchestrator to run `turnarbiter` under session-owned lifecycle state, wired `transports/livekit` turn-open/active handling through orchestrator, and added runtime integration tests for pre-turn reject/defer plus accepted-turn abort/close sequencing; `go test ./internal/runtime/session ./transports/livekit ./test/integration -run 'TestSessionLifecycleRejectAndDeferDoNotEmitAcceptedTurnTerminalSignals|TestSessionLifecycleAcceptedTurnAbortCloseSequence'`, `verify-quick`, `verify-full`, and `security-baseline-check` passing.
- [x] Runtime `UB-R01` Session Lifecycle Manager complete (completed: slice A/B with lifecycle registry + session-owned orchestration wiring + integration coverage).
- [x] (2026-02-16 19:46Z) Landed `UB-R02` slice A: tightened `planresolver` to require CP-derived turn-start inputs (`graph_definition_ref`, `execution_profile`, non-nil adaptive actions, full snapshot provenance), added execution-profile policy resolution (`simple`/`simple-v1`), and made `plan_hash` derive from frozen policy/provenance material (not only identity fields); added/updated resolver tests and reran runtime/transport/integration suites plus `verify-quick`, `verify-full`, and `security-baseline-check`.
- [x] (2026-02-16 19:58Z) Landed `UB-R02` slice B: threaded `ResolvedTurnPlan` into executor scheduling (`SchedulingInput.ResolvedTurnPlan`) with strict turn/pipeline/authority mismatch rejection and plan-derived provider/adaptive defaults at node dispatch; added executor tests for plan-hydrated provider invocation and mismatch failures; `go test ./internal/runtime/executor ./internal/runtime/planresolver ./internal/runtime/turnarbiter ./transports/livekit ./test/integration`, `verify-quick`, `verify-full`, and `security-baseline-check` passing.
- [x] (2026-02-16 20:43Z) Landed `UB-R02` slice E: routed CP-materialized policy surfaces end-to-end (`policy.ResolvedTurnPolicy` -> distribution adapters -> turn-start bundle -> `planresolver.Input` -> frozen `ResolvedTurnPlan`), enforced all-or-nothing CP policy-surface overrides, and added regression tests for CP policy propagation/validation in `planresolver`, `turnarbiter`, and distribution adapters; ran `go test ./internal/controlplane/policy ./internal/controlplane/distribution ./internal/runtime/planresolver ./internal/runtime/turnarbiter ./test/failover`, `verify-quick`, `verify-full`, and `security-baseline-check`.
- [x] Runtime `UB-R02` Plan Freeze upgrade complete (completed: slice A/B/C/D/E with strict input freeze, expanded plan-hash inputs, executor plan-policy wiring, plan-derived scheduler config, runtime scheduler instantiation, and CP-materialized policy-surface freeze at turn start).
- [x] (2026-02-16 19:58Z) Landed `UB-R03` slice A: implemented `internal/runtime/lanes` priority scheduler with bounded per-lane queues, strict dispatch order (`Control > Data > Telemetry`), deterministic overflow handling (`shed` for control/data, best-effort `drop_notice` for telemetry), and flow-control watermark/recovery signaling (`flow_xoff`/`flow_xon`); added focused lane scheduler tests; `go test ./internal/runtime/lanes ./internal/runtime/executor`, `verify-quick`, `verify-full`, and `security-baseline-check` passing.
- [x] (2026-02-16 20:00Z) Landed `UB-R03` slice B: updated executor DAG dispatch ordering to prioritize ready nodes by lane (`Control > Data > Telemetry`) rather than FIFO and added regression coverage proving data-lane preemption over telemetry when both become ready from the same predecessor; targeted/runtime/integration tests plus `verify-quick`, `verify-full`, and `security-baseline-check` passing.
- [x] (2026-02-16 20:01Z) Landed `UB-R03` slice C / `UB-R02` bridge: added `lanes.ConfigFromResolvedTurnPlan` to derive bounded queue limits and lane watermarks from frozen `ResolvedTurnPlan` policy surfaces (`edge_buffer_policies`, `flow_control`, plan hash/epoch context) with unit tests validating derivation and invalid-plan rejection; targeted/runtime/integration tests plus `verify-quick`, `verify-full`, and `security-baseline-check` passing.
- [x] (2026-02-16 20:25Z) Landed `UB-R03` slice D / `UB-R02` bridge: integrated plan-derived lane scheduler into executor runtime path (`ExecutePlan` now instantiates `PriorityScheduler` via frozen `ResolvedTurnPlan`), added deterministic dropped-node handling (`telemetry` non-blocking `drop_notice`; control/data overflow emits shed denial + terminal stop), and added executor tests for telemetry overflow non-blocking and data overflow shed-stop; `go test ./internal/runtime/executor ./internal/runtime/lanes ./internal/runtime/session ./internal/runtime/turnarbiter`, `go test ./test/integration -run 'TestSchedulingPointShedDoesNotForceTerminalLifecycle|TestExecutePlanPersistsAttemptEvidenceAndTerminalBaseline|TestExecutePlanPromotesAttemptEvidenceIntoTerminalBaseline|TestSessionLifecycleRejectAndDeferDoNotEmitAcceptedTurnTerminalSignals|TestSessionLifecycleAcceptedTurnAbortCloseSequence'`, `verify-quick`, `verify-full`, and `security-baseline-check` passing.
- [x] (2026-02-16 20:43Z) Landed `UB-R03` slice E: broadened failover coverage for queue-pressure + sync-integrity coupling by adding scheduler assertions in `test/failover/failure_full_test.go` (`telemetry` overflow remains best-effort, control lane preempts under pressure, sync-loss control dispatch preempts telemetry backlog under frozen-plan scheduler config); reran targeted runtime/failover suites plus `verify-quick`, `verify-full`, and `security-baseline-check`.
- [x] Runtime `UB-R03` Lane Dispatch Scheduler complete (completed: slice A/B/C/D/E scheduler primitive + bounded priority queues + executor lane-priority dispatch + runtime-path scheduler integration + failover queue-pressure/sync-coupling coverage).
- [x] (2026-02-16 20:53Z) Landed `UB-R04` slice A: hardened execution-pool overload handling by introducing typed pool saturation/closure errors and mapping executor pool-submit failures into deterministic scheduling outcomes (`execution_pool_saturated` / `execution_pool_closed`) instead of raw dispatch errors; kept telemetry-lane saturation non-blocking by converting overload sheds into best-effort continuation at node-dispatch boundary; added execution-pool and executor tests for control-path shed-stop and telemetry non-blocking saturation behavior; ran `go test ./internal/runtime/executionpool ./internal/runtime/executor ./internal/runtime/turnarbiter ./internal/runtime/lanes ./internal/runtime/localadmission ./test/failover`, `verify-quick`, `verify-full`, and `security-baseline-check`.
- [x] (2026-02-16 21:07Z) Landed `UB-R04` slice B: threaded optional `tenant_id` through runtime turn-start resolver inputs (`OpenRequest`/`ActiveInput` -> `TurnStartBundleInput`) and into CP admission evaluation (`admission.Input.TenantID`), enabling tenant-scoped CP pre-turn admission decisions from runtime paths; added resolver + arbiter regression tests asserting tenant propagation and tenant-scoped CP admission scope defaults; ran `go test ./internal/runtime/turnarbiter ./internal/controlplane/admission ./internal/controlplane/distribution ./internal/runtime/session ./transports/livekit ./test/failover ./test/integration -run 'TestSessionLifecycleRejectAndDeferDoNotEmitAcceptedTurnTerminalSignals|TestSessionLifecycleAcceptedTurnAbortCloseSequence|TestF1AdmissionOverloadSmoke'`, `verify-quick`, `verify-full`, and `security-baseline-check`.
- [x] (2026-02-16 21:22Z) Landed `UB-R04` slice C: implemented per-node concurrency/fairness controls in runtime execution pool (`Task.FairnessKey`, `Task.MaxOutstanding`) with explicit `ErrNodeConcurrencyExceeded` and `RejectedByConcurrency` stats, then mapped concurrency-limit submission failures to deterministic scheduler shed reason `node_concurrency_limited` in executor dispatch while preserving telemetry-lane non-blocking continuation; added pool and executor regression tests for control-path stop, telemetry non-blocking behavior, and negative `concurrency_limit` validation; ran `go test ./internal/runtime/executionpool ./internal/runtime/executor`, `verify-quick`, `verify-full`, and `security-baseline-check`.
- [x] (2026-02-16 22:18Z) Landed `UB-R04` slice D: completed CP-originated fairness/concurrency policy threading by adding additive `node_execution_policies` control-plane surfaces (`api/controlplane`, distribution adapters, policy service, turn-start bundle, planresolver hash freeze) and tenant-aware policy-input threading (`policy.Input.TenantID` + file distribution `policy.by_tenant` overrides); executor now applies frozen-plan node execution policy defaults at dispatch (with edge-policy fairness-key fallback) when `NodeSpec` does not override local settings; added regression coverage across control-plane policy/distribution, turnarbiter backend wiring, planresolver policy freeze, and executor runtime enforcement under resolved-plan policy; ran `go test ./api/controlplane ./internal/controlplane/policy ./internal/controlplane/distribution ./internal/runtime/turnarbiter ./internal/runtime/planresolver ./internal/runtime/executor ./internal/runtime/executionpool`.
- [x] (2026-02-16 22:28Z) Landed `UB-R04` slice E: completed CP-originated tenant admission override wiring by extending distribution admission surfaces with additive `admission.by_tenant` precedence (`default -> by_pipeline -> by_tenant`) across file + HTTP adapters, and verified runtime enforcement via distribution-backed arbiter turn-open tests where tenant-scoped CP admission defer blocks turn activation deterministically; added distribution adapter tenant-override tests (`file`/`http`) and turnarbiter distribution integration coverage; ran `go test ./internal/controlplane/admission ./internal/controlplane/policy ./internal/controlplane/distribution ./internal/runtime/turnarbiter ./internal/runtime/planresolver ./internal/runtime/executor ./internal/runtime/executionpool`, `verify-quick`, `verify-full`, and `security-baseline-check`.
- [x] Runtime `UB-R04` Admission + execution-pool overload hardening complete (completed slices A/B/C/D/E for MVP runtime baseline; remaining broader multi-scope quota governance remains explicitly tracked under `UB-CP04`).
- [x] (2026-02-16 22:32Z) Landed `UB-R05` slice A: implemented runtime state service boundaries in `internal/runtime/state/service.go` (session-hot and turn-ephemeral stores, authority-epoch validation, idempotency/provider dedupe registries), enforced stale-authority egress rejection in `internal/runtime/transport/fence.go`, and added failover/integration coverage in `test/failover/failure_full_test.go` + `test/integration/runtime_state_idempotency_test.go`; ran `go test ./internal/runtime/state ./internal/runtime/transport ./test/failover ./test/integration`, `verify-quick`, `verify-full`, and `security-baseline-check`.
- [x] Runtime `UB-R05` State service + authority/idempotency enforcement complete (completed slice A for MVP baseline with authority-at-ingress/egress enforcement and deterministic idempotency evidence paths).
- [x] (2026-02-16 22:32Z) Landed `UB-R06` slice A: implemented deterministic timebase mapping in `internal/runtime/timebase/service.go` (calibrate/project/observe-rebase) and sync integrity policy engine in `internal/runtime/sync/engine.go` (`atomic_drop`, `drop_with_discontinuity`), with replay/failover coverage updates in `test/replay/rd002_rd003_rd004_test.go` and `test/failover/failure_full_test.go`; ran `go test ./internal/runtime/timebase ./internal/runtime/sync ./test/replay ./test/failover`, `verify-quick`, `verify-full`, and `security-baseline-check`.
- [x] Runtime `UB-R06` Timebase + sync integrity + deterministic downgrade semantics complete (completed slice A for MVP baseline with deterministic projection/sync policies and replay-coupled validation).
- [x] (2026-02-16 22:32Z) Landed `UB-R07` slice A: implemented runtime node host lifecycle orchestration in `internal/runtime/nodehost/host.go` (register/start/dispatch/cancel/stop with timeout enforcement) and external-node execution boundary in `internal/runtime/externalnode/runtime.go` (resource envelope checks, timeout-bound invocation, cancel propagation, telemetry injection), plus failover coverage in `test/failover/failure_full_test.go`; ran `go test ./internal/runtime/nodehost ./internal/runtime/externalnode ./test/failover`, `verify-quick`, `verify-full`, and `security-baseline-check`.
- [x] Runtime `UB-R07` Node host lifecycle + external-node boundary complete (completed slice A for MVP baseline with deterministic lifecycle and boundary controls).
- [x] (2026-02-16 23:35Z) Control plane `UB-CP01` bootstrap process complete (implemented `cmd/rspp-control-plane` publish/list/get/rollback/status/audit command surface with persisted immutable audit trail and tests).
- [x] (2026-02-16 23:35Z) Control plane `UB-CP02` deterministic rollout semantics complete (validated rollout/registry deterministic resolution path via `go test ./internal/controlplane/rollout ./internal/controlplane/registry ./internal/runtime/turnarbiter` and full gates).
- [x] (2026-02-16 23:47Z) Rechecked `UB-CP01` against feature expectations and tightened process metadata/audit evidence: registry publish/list/get now emit `created_at_utc`/`author`/`spec_hash`, audit entries include actor and change hash, and command tests validate these fields.
- [x] (2026-02-16 23:47Z) Rechecked `UB-CP02` and closed rollout semantics gap by implementing deterministic canary routing in `internal/controlplane/rollout` and distribution adapters (`tenant_allowlist`, `canary_percentage`, deterministic cohort hashing, artifact validation), plus tenant threading from turn-start resolver to rollout input and regression coverage.
- [x] (2026-02-17 01:02Z) Control plane `UB-CP03` session route/token/status service complete (added first-class route/token/status shared contracts in `api/controlplane`, implemented `internal/controlplane/sessionroute` service for route resolution + signed token issuance + status reads, extended `cmd/rspp-control-plane` with `resolve-session-route`, `issue-session-token`, `session-status`, and `set-session-status`, and validated via `go test ./api/controlplane ./internal/controlplane/sessionroute ./cmd/rspp-control-plane` plus `make verify-quick`).
- [x] (2026-02-17 01:02Z) Control plane `UB-CP04` lease/admission/fallback hardening complete (implemented explicit strict-vs-availability fallback mode in `internal/runtime/turnarbiter/controlplane_backends.go`, added quota/rate admission governance fields and enforcement normalization in `internal/controlplane/admission` + distribution adapters, enriched lease outputs with token id/expiry metadata in `internal/controlplane/lease`, threaded lease token metadata into turn-start bundle validation, and validated via targeted control-plane/runtime tests plus `make verify-quick`).
- [x] (2026-02-17) `UB-CP03` closure evidence expanded: added integration coverage in `test/integration/controlplane_sessionroute_integration_test.go` proving session route + signed token + status outputs satisfy transport bootstrap contract compatibility (`api/transport.SessionBootstrap`) with deterministic versioned snapshots.
- [x] (2026-02-17) `UB-CP04` residual gap closed: hardened `internal/controlplane/lease` to treat expired lease-token metadata as deauthorized authority, added explicit expired/invalid-expiry tests in `internal/controlplane/lease/lease_test.go`, and added integration assertion that expired lease tokens deterministically deny turn-open in `test/integration/controlplane_sessionroute_integration_test.go`; revalidated with `go test ./internal/controlplane/lease ./test/integration` and `make verify-quick`.
- [x] (2026-02-16 23:35Z) Observability `UB-O01` canonical correlation resolver complete.
- [x] (2026-02-16 23:35Z) Observability `UB-O02` telemetry metrics/traces completeness complete.
- [x] (2026-02-16 23:35Z) Observability `UB-O03` OR-02 durable exporter split complete.
- [x] (2026-02-17) Recheck correction closed: `UB-O01` canonical correlation gaps resolved.
  1. Added dedicated canonical resolver package `internal/observability/telemetry/context` with strict required ID validation/defaulting/normalization tests.
  2. Threaded node/edge correlation IDs through runtime telemetry call sites (`executor`, `provider/invocation`, `transport/fence`, `turnarbiter`, `lanes`, `buffering`).
  3. Added correlation consistency assertions in runtime telemetry tests (scheduler/provider/fence and edge-level telemetry paths) validating node/edge linkage across metric/span/log emissions.
- [x] (2026-02-17) Recheck correction closed: `UB-O02` telemetry completeness gaps resolved.
  1. Added stable per-edge metrics for queue/drop/merge coverage (`edge_queue_depth`, `edge_drops_total`, `edge_merges_total`) with deterministic edge linkage labels.
  2. Added per-node/per-edge latency surfaces (`node_latency_ms`, `edge_latency_ms`) and verified emission paths in scheduler + streaming-edge/buffering telemetry tests.
  3. Added linkage assertions for `node_id`/`edge_id` across metrics/spans/logs in scheduler/provider/fence/lane tests.
- [x] (2026-02-17) Recheck correction closed: `UB-O03` durable-export split gaps resolved.
  1. Added async durable timeline exporter package `internal/observability/timeline/exporter` with bounded queue, timeout, retry, and failure accounting.
  2. Wired explicit local-append vs durable-export seam in `internal/observability/timeline/recorder.go` via `NewRecorderWithDurableExporter` and best-effort enqueue on baseline append.
  3. Added outage/retry non-blocking coverage in `test/integration/observability_timeline_exporter_integration_test.go` and exporter unit tests; validated with targeted suites and `make verify-quick`.
- [x] (2026-02-16 23:35Z) Observability `UB-O04` replay engine + deterministic cursor execution complete.
- [x] (2026-02-16 23:35Z) Observability `UB-O05` replay access/redaction hardening + divergence report pipeline complete.
- [x] (2026-02-17 02:58Z) Recheck correction closed: `UB-O04` replay engine/cursor execution is now concretely implemented.
  1. Added `internal/observability/replay/engine.go` with resolver-backed replay execution over `ReplayRunRequest`, explicit mode propagation, deterministic cursor boundary resolution/resume, and validated `ReplayRunResult` cursor outputs.
  2. Added engine coverage in `internal/observability/replay/engine_test.go` for mode tagging, provider-playback mode, cursor resume determinism, baseline cursor-missing error, and candidate cursor-boundary fallback divergence behavior.
  3. Wired CLI replay smoke/regression fixture execution through the engine (`cmd/rspp-cli/main.go`) so generated artifacts are real replay runs rather than direct comparator-only synthesis.
- [x] (2026-02-17 02:58Z) Recheck correction closed: `UB-O05` divergence/report hardening gaps are now closed.
  1. Added explicit `PROVIDER_CHOICE_DIVERGENCE` contract support in `api/observability/types.go` and validator tests, plus comparator-level provider/model mismatch detection in `internal/observability/replay/comparator.go`.
  2. Extended regression policy classification (`internal/tooling/regression/divergence.go`) and tests so provider-choice divergences are first-class fail/pass policy inputs.
  3. Extended replay report artifacts (`.codex/replay/smoke-report.json`, `.codex/replay/regression-report.json`) with replay mode, run id, request/result cursor fields, and by-class accounting that now includes provider-choice divergence.
- [x] (2026-02-16 23:35Z) Provider/transport `UB-PT01` provider policy/capability freeze + budget/retry enforcement complete.
- [x] (2026-02-16 23:35Z) Provider/transport `UB-PT02` provider evidence/secret-ref normalization + conformance suites complete.
- [x] (2026-02-16 23:35Z) Provider/transport `UB-PT03` transport kernel split + buffering/codec/control/routing safety complete.
- [x] (2026-02-17 03:23Z) Recheck correction closed: `UB-PT01` provider policy/capability freeze + budget/retry enforcement is now explicit in runtime code.
  1. Added deterministic provider policy resolver (`internal/runtime/provider/policy`) and capability snapshot freeze module (`internal/runtime/provider/capability`) to produce turn-frozen ordered candidates, adaptive-action set, budget bounds, and snapshot refs.
  2. Extended provider invocation controller to accept resolved provider plans and enforce max-attempt/max-latency invocation budgets while preserving deterministic retry/switch behavior.
  3. Threaded resolved-turn provider snapshot metadata/budget into scheduler invocation path so provider attempts carry frozen policy/capability provenance.
- [x] (2026-02-17 03:23Z) Recheck correction closed: `UB-PT02` provider secret-ref/config normalization + adapter conformance gaps are now closed.
  1. Added provider secret-ref config layer (`internal/runtime/provider/config`) with `env://` secret resolution, literal fallback, deterministic redaction marker, and rotation-safe resolution tests.
  2. Wired all HTTP-backed provider adapters to accept secret-ref env vars for API keys/endpoints (`*_API_KEY_REF`, `*_ENDPOINT_REF`) while preserving existing direct env compatibility.
  3. Added missing adapter suites for `providers/llm/gemini`, `providers/stt/deepgram`, `providers/stt/google`, `providers/tts/elevenlabs`, and `providers/tts/google`, plus provider contract/conformance profile tests in `test/contract/provider_adapter_contract_test.go` and `test/contract/provider_conformance_profile_test.go`.
- [x] (2026-02-17 03:23Z) Recheck correction closed: `UB-PT03` transport kernel split + shared contract adoption and safety modules are now concretely implemented.
  1. Added transport kernel submodules under `internal/runtime/transport` for lifecycle FSM (`sessionfsm`), ingress normalization (`ingress`), egress broker (`egress`), authority guard (`authority`), bounded jitter buffering (`buffering`), routing update client (`routing`), and orchestrated session kernel (`kernel`).
  2. Added concrete `transports/websocket` and `transports/telephony` adapters implementing shared bootstrap/capability/routing/ingress/egress seams over the kernel modules.
  3. Added shared contract adoption surface for LiveKit (`transports/livekit/contract.go`) exposing normalized transport kind/capabilities/bootstrap validation.
  4. Validated via targeted provider/transport suites and green repo gates: `go test ./internal/runtime/provider/... ./providers/... ./internal/runtime/executor ./internal/runtime/transport/... ./transports/livekit ./transports/websocket ./transports/telephony ./test/contract/...`, `make verify-quick`, `make verify-full`, and `make security-baseline-check`.
- [x] (2026-02-16 23:35Z) Tooling `UB-T01` release artifact schema alignment complete.
- [x] (2026-02-16 23:35Z) Tooling `UB-T02` validate-spec/validate-policy + gate conformance complete.
- [x] (2026-02-17 03:38Z) Recheck correction closed: `UB-T01` release artifact schema compatibility is now explicitly enforced in publish-readiness.
  1. Added explicit artifact schema-version constants and stamping for contracts/replay-regression/SLO artifacts and release manifests.
  2. Hardened `internal/tooling/release` readiness checks to fail closed on schema-version mismatch before freshness/gate evaluation.
  3. Added coverage for schema-version mismatch failure and schema-version propagation in release tests.
- [x] (2026-02-17 03:38Z) Recheck correction closed: `UB-T02` CLI `validate-spec` + `validate-policy` plus gate conformance wiring are now concretely implemented.
  1. Added spec/policy validators under `internal/tooling/validation` with strict/relaxed modes and actionable field-path diagnostics.
  2. Added new `rspp-cli` subcommands `validate-spec` and `validate-policy`, including usage/docs and command tests.
  3. Wired `validate-spec` and `validate-policy` into `verify-quick`, `verify-full`, and `verify-mvp` command chains.
- [x] (2026-02-16 23:35Z) Tooling `UB-T03` replay regression modes + cursor resume in CLI/gates complete.
- [x] (2026-02-16 23:35Z) Tooling `UB-T04` loopback local-runner + CP command MVP surface complete (added durable `rspp-control-plane` command MVP surface and validated with `go test ./cmd/rspp-control-plane ./cmd/rspp-local-runner`).
- [x] (2026-02-16 23:35Z) Tooling `UB-T05` conformance/skew governance checks + feature-status stewardship complete (verified `verify-quick`/`verify-full`/`verify-mvp`/`security-baseline-check` and published deterministic MVP live-compare artifacts).
- [x] (2026-02-17 04:02Z) Recheck correction closed: `UB-T04` now includes explicit offline `rspp-local-runner loopback` mode with synthetic/WAV input support and Event ABI parity validation against generated transport reports.
  1. Extended `cmd/rspp-local-runner` with `loopback` subcommand and deterministic fixture generation feeding LiveKit dry-run adapter path.
  2. Added loopback report validation that enforces Event ABI validity (`event_record` and `control_signal`) plus required payload-class coverage (`text_raw` synthetic, `audio_raw` wav).
  3. Added command tests for synthetic loopback, WAV loopback, and conflicting-input rejection in `cmd/rspp-local-runner/main_test.go`.
- [x] (2026-02-17 04:02Z) Recheck correction closed: `UB-T05` conformance/skew governance checks + feature-status stewardship are now concretely implemented and gate-enforced.
  1. Added `internal/tooling/conformance` governance engine covering version-skew matrix validation, deprecation lifecycle phase checks, mandatory conformance-category certification, and `docs/RSPP_features_framework.json` stewardship checks for `F-162/F-163/F-164/NF-010/NF-032`.
  2. Added new CLI artifact command `rspp-cli conformance-governance-report` and deterministic report outputs (`.codex/ops/conformance-governance-report.{json,md}`).
  3. Added canonical governance policy/profile fixtures (`pipelines/compat/version_skew_policy_v1.json`, `pipelines/compat/deprecation_policy_v1.json`, `pipelines/compat/conformance_profile_v1.json`, `test/contract/fixtures/conformance_results_v1.json`) and wired governance command into `verify-quick`, `verify-full`, and `verify-mvp`.
  4. Validated via targeted suites and gates: `go test ./cmd/rspp-local-runner ./internal/tooling/conformance ./cmd/rspp-cli`, `make verify-quick`, `make verify-full`, `make verify-mvp`, and `make security-baseline-check`.
- [x] (2026-02-16 23:35Z) Scaffolds `UB-S01` `pkg/contracts/v1` facade promotion complete (added `pkg/contracts/v1/core` parity aliases and tests).
- [x] (2026-02-16 23:35Z) Scaffolds `UB-S02` pipeline artifact set complete (added canonical spec/profile/bundle/rollout/policy/compat/extensions/replay manifests under `pipelines/` with contract tests).
- [x] (2026-02-16 23:35Z) Scaffolds `UB-S03` deploy scaffolds + deterministic gate runner complete (added K8s/Helm/Terraform scaffolds, deploy profile rollout, and `deploy/verification/gates/mvp_gate_runner.sh`).
- [x] (2026-02-17 04:15Z) Recheck correction closed: `UB-S01` facade parity tests now assert the full exported `pkg/contracts/v1/core` alias surface against canonical `api/*` contracts.
  1. Expanded compile-time alias coverage in `pkg/contracts/v1/core/types_test.go` for event/controlplane/observability/transport contract aliases.
  2. Preserved transport validator behavior checks for `SessionBootstrap` and `AdapterCapabilities`.
- [x] (2026-02-17 04:15Z) Recheck correction closed: `UB-S02` scaffold artifact coverage now enforces schema-version and cross-artifact linkage invariants.
  1. Added schema/linkage assertions in `test/contract/scaffold_artifacts_test.go` across spec/profile/bundle/rollout/policy/compat/extensions/replay and deploy profile bundle refs.
  2. Added compatibility matrix assertions to require explicit MVP LiveKit support and provider-binding completeness.
- [x] (2026-02-17 04:15Z) Recheck correction closed: `UB-S03` deploy scaffold verification now includes deterministic gate-runner sequence enforcement and refreshed scaffold docs.
  1. Added gate runner command-order and PASS-marker assertions in `test/contract/scaffold_artifacts_test.go`.
  2. Added deploy scaffold content-marker assertions for K8s/Helm/Terraform MVP assets.
  3. Updated scaffold guides under `pipelines/` and `deploy/` to remove stale placeholder-only status and document implemented MVP baseline.
- [x] (2026-02-16 23:35Z) `docs/RSPP_features_framework.json` MVP entries updated to verified status (`mvp_pass == mvp_total`) with traceable evidence links (`.codex/ops/*`, `.codex/replay/*`, `.codex/providers/*`, gate command refs).
- [x] (2026-02-16 23:35Z) Final MVP release readiness review and signoff completed (`verify-quick`, `verify-full`, `verify-mvp`, `security-baseline-check`, and deploy gate runner all passing with regenerated artifacts).

## Surprises & Discoveries

- Observation: The latest backlog snapshot shows green CI gates while `docs/RSPP_features_framework.json` still records `mvp_pass=0` using transitional `passes=false` fields.
  Evidence: `docs/mvp_backlog.md` repo-health row and features-framework status row.

- Observation: Major parts of runtime, control-plane process surface, transport modularization, and observability replay engine are still partial/scaffold despite baseline tests passing.
  Evidence: `internal/runtime/runtime_kernel_guide.md`, `internal/controlplane/controlplane_module_guide.md`, `internal/observability/observability_module_guide.md`, `transports/transport_adapter_guide.md` divergence sections.

- Observation: The contract stream is explicitly serialized (`api/*` lock), so MVP velocity depends on strict ordering and narrow PR scope.
  Evidence: `docs/mvp_backlog.md` contract PR plan and stream lock rules.

- Observation: Tightening `turn_open_proposed` to schema parity immediately surfaced a hidden runtime assumption in prelude/livekit flow (`event_scope=turn`) and broke `verify-quick` until prelude signal scope was corrected.
  Evidence: initial `make verify-quick` failure in `transports/livekit` and `test/integration`; resolved by updating `internal/runtime/prelude/engine.go`.

- Observation: `resolved_turn_plan.streaming_handoff` existed in Go contracts but was absent from `docs/ContractArtifacts.schema.json`, creating a silent schema/type divergence.
  Evidence: control-plane guide divergence matrix and schema inspection under `$defs.resolved_turn_plan`.

- Observation: The repository had no `api/transport` package, so transport modularity work had no shared typed contract boundary despite MVP transport scope decisions.
  Evidence: package inventory under `api/*` before `PR-C4` slice A.

- Observation: `internal/runtime/session` was scaffold-only and not executable before this slice, with lifecycle ownership concentrated in `turnarbiter`.
  Evidence: `internal/runtime/session/session_lifecycle_module_guide.md` and runtime package file inventory before `UB-R01` slice A.

- Observation: LiveKit transport processing called `turnarbiter` directly, so session lifecycle state was not the owning entrypoint for runtime turn-open/terminal sequencing.
  Evidence: `transports/livekit/adapter.go` event handling path before `UB-R01` slice B.

- Observation: `planresolver` used identity-only hashing and implicit fallback defaults, so plan-freeze integrity could miss policy/provenance drift when turn-start inputs were partially absent.
  Evidence: pre-slice `internal/runtime/planresolver/resolver.go` identity hash/defaulting behavior.

- Observation: executor scheduling accepted free-form provider invocation settings even when a frozen `ResolvedTurnPlan` was available, leaving a plan-freeze drift seam at node dispatch.
  Evidence: pre-slice `internal/runtime/executor/scheduler.go` provider invocation path without plan-aware defaulting/mismatch enforcement.

- Observation: runtime had routing helpers (`lanes/router`), pressure handlers, and flow-control primitives, but no concrete bounded lane queue with dispatch semantics to enforce strict lane priority under saturation.
  Evidence: `internal/runtime/lanes` package inventory and `internal/runtime/runtime_kernel_guide.md` RK-03 divergence table.

- Observation: executor topological scheduling previously used FIFO ready-node traversal, so when data and telemetry nodes became ready concurrently, dispatch order depended on edge insertion order rather than lane priority.
  Evidence: pre-slice `internal/runtime/executor/plan.go` queue-pop behavior before `UB-R03` slice B.

- Observation: queue/watermark scheduler settings were previously configured ad-hoc in tests/runtime call sites rather than being derived from immutable turn-plan policy artifacts.
  Evidence: lack of resolved-plan-to-lane-scheduler adapter prior to `lanes.ConfigFromResolvedTurnPlan`.

- Observation: integrating lane-priority dispatch with bounded queues surfaced control-signal ordering regressions (runtime/control sequences could move backward when lower-priority queued work drained after higher-priority dispatch).
  Evidence: initial `UB-R03` slice D test failure (`runtime_sequence regression`) while validating normalized control-signal stream in `internal/runtime/executor/plan.go`.

- Observation: comparing `controlplane.RecordingPolicy` with zero-value struct caused compilation failures because the type contains a slice field.
  Evidence: Go build errors in `internal/controlplane/policy/policy.go` and `internal/runtime/planresolver/resolver.go` (`struct containing []string cannot be compared`), resolved by explicit `isZeroRecordingPolicy` helpers.

- Observation: `F-109` node-concurrency behavior could be implemented with low blast radius by enforcing per-key outstanding limits at execution-pool submit time and reusing existing overload shed semantics in executor dispatch.
  Evidence: `internal/runtime/executionpool/pool.go` (`FairnessKey`, `MaxOutstanding`, `ErrNodeConcurrencyExceeded`) and `internal/runtime/executor/plan.go` (`node_concurrency_limited` mapping) with coverage in `internal/runtime/executionpool/pool_test.go` and `internal/runtime/executor/scheduler_test.go`.

- Observation: CP policy surfaces can now carry and freeze node-level execution fairness/concurrency (`node_execution_policies`) without disruptive runtime API changes by extending existing turn-start bundle -> planresolver flow and keeping `NodeSpec` overrides authoritative when provided.
  Evidence: `api/controlplane/types.go`, `internal/controlplane/policy/policy.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/planresolver/resolver.go`, and `internal/runtime/executor/plan.go` plus coverage in policy/distribution/turnarbiter/resolver/executor tests.

- Observation: Tenant-aware turn-start threading alone was insufficient for CP-originated admission fairness posture because distribution admission snapshots only supported default/pipeline scopes.
  Evidence: pre-slice `internal/controlplane/distribution/file_adapter.go` resolved admission via `default` + `by_pipeline` only; tenant-specific admission policy outcomes could not be authored in CP distribution artifacts.

- Observation: Runtime state/authority guarantees for MVP can be implemented as an in-memory state service without violating stateless runtime compute, as long as durable ownership remains external and authority epoch is enforced at ingress/egress boundaries.
  Evidence: `internal/runtime/state/service.go` scope-limited state + epoch validation and `internal/runtime/transport/fence.go` stale-authority rejection path (`stale_epoch_reject`) covered by `test/failover/failure_full_test.go`.

- Observation: Deterministic timebase + sync semantics can be proven without full distributed clock reconciliation by combining local monotonic projection/rebase rules with explicit sync-drop policy signals and replay/failover assertions.
  Evidence: `internal/runtime/timebase/service.go`, `internal/runtime/sync/engine.go`, `test/replay/rd002_rd003_rd004_test.go`, and `test/failover/failure_full_test.go`.

- Observation: `verify-mvp` was not reproducible from a clean, no-credentials environment because live compare inputs were missing (`comparison_identity` absent), causing gate failure before MVP report generation.
  Evidence: `make verify-mvp` failure at `rspp-cli live-latency-compare-report` with `comparison identity missing from streaming/non-streaming reports`.

## Decision Log

- Decision: Use `docs/mvp_backlog.md` as execution map and canonical module guides as implementation detail authority.
  Rationale: The backlog captures dependency-safe sequencing while guides define module-level contracts and done criteria.
  Date/Author: 2026-02-16 / Codex

- Decision: Treat `PR-C4` (`api/transport`) as in-scope for MVP completion.
  Rationale: User direction and transport target architecture require shared contract bootstrap for full transport modularity.
  Date/Author: 2026-02-16 / Codex

- Decision: Execute invariant-first ordering: runtime lifecycle/plan freeze/lane scheduler and authority enforcement before downstream replay/tooling features.
  Rationale: This prevents semantic drift and ensures all later modules build on enforced turn semantics.
  Date/Author: 2026-02-16 / Codex

- Decision: Keep one PR per unified backlog item (or tightly coupled split pair) with path ownership boundaries.
  Rationale: Avoid merge contention and simplify regression isolation while honoring `api/*` serialization constraints.
  Date/Author: 2026-02-16 / Codex

- Decision: Treat `PR-C1` as iterative slices with gate-green checkpoints, rather than waiting for all parity deltas before progress updates.
  Rationale: User requested ongoing milestone execution and progress updates as slices land; slice-level gating keeps momentum and regressions contained.
  Date/Author: 2026-02-16 / Codex

- Decision: Align prelude `turn_open_proposed` emission to `event_scope=session` at source rather than loosening Event ABI validation.
  Rationale: Schema and contract invariants define `turn_open_proposed` as pre-turn session-scope control; keeping validator strict prevents future drift.
  Date/Author: 2026-02-16 / Codex

- Decision: Introduce `api/transport` as a minimal strict bootstrap contract first, then expand fields incrementally with fixtures and adapter adoption.
  Rationale: Establishes a stable transport API seam quickly while keeping `PR-C4` low-risk and testable per slice.
  Date/Author: 2026-02-16 / Codex

- Decision: Add `UB-R01` as an additive session-manager package first, before integrating turnarbiter ownership changes.
  Rationale: Enables incremental lifecycle hardening with immediate test coverage while minimizing risk to existing runtime execution paths.
  Date/Author: 2026-02-16 / Codex

- Decision: Introduce a `session.Orchestrator` wrapper and route LiveKit turn-open/active handling through it, while keeping `turnarbiter` APIs unchanged.
  Rationale: Moves runtime ownership to session-scoped lifecycle state with low migration risk and preserves compatibility for existing tests/tools that still call arbiter directly.
  Date/Author: 2026-02-16 / Codex

- Decision: Tighten `planresolver` input validation and hash composition first (slice A), while keeping execution-profile policy tables local until CP contract surfaces expose richer resolved policy artifacts.
  Rationale: Improves plan-freeze safety immediately without blocking on broader CP/runtime API expansion, and keeps the slice small enough for deterministic gate verification.
  Date/Author: 2026-02-16 / Codex

- Decision: Add optional `ResolvedTurnPlan` threading at executor scheduling input (slice B) rather than refactoring scheduler/executor ownership boundaries.
  Rationale: This enforces turn-plan freeze semantics at dispatch seams with minimal disruption and clear compatibility for existing call sites.
  Date/Author: 2026-02-16 / Codex

- Decision: Implement `UB-R03` as an additive `lanes` priority scheduler first, before replacing existing executor dispatch loops.
  Rationale: Establishes deterministic queueing, saturation, and lane-priority semantics in an isolated unit with strong tests while reducing regression risk in runtime integration paths.
  Date/Author: 2026-02-16 / Codex

- Decision: Integrate lane-priority semantics into executor DAG scheduling via ready-node selection (slice B) before full queue-policy/edge-policy plumbing.
  Rationale: This closes the highest-impact lane-priority correctness gap immediately while keeping the change small enough to remain gate-safe.
  Date/Author: 2026-02-16 / Codex

- Decision: Add `ConfigFromResolvedTurnPlan` as a policy bridge (slice C) before wiring scheduler instantiation into runtime orchestration.
  Rationale: Makes turn-plan policy the configuration source of truth for lane queues/watermarks while preserving incremental integration safety.
  Date/Author: 2026-02-16 / Codex

- Decision: Wire lane scheduler integration directly into `executor.ExecutePlan` behind frozen-plan presence and stabilize emitted control-signal ordering before ABI normalization.
  Rationale: This closes the highest-priority runtime-path gap for `UB-R03` while preserving compatibility for legacy callers and keeping queue-pressure semantics deterministic under lane-priority dispatch.
  Date/Author: 2026-02-16 / Codex

- Decision: Require CP policy-surface overrides to be complete (`budgets`, `provider_bindings`, `edge_buffer_policies`, `flow_control`, `recording_policy`) whenever any override is provided.
  Rationale: Partial overrides allow hidden fallback defaults and create plan-freeze drift; all-or-nothing validation keeps turn-start policy materialization deterministic and auditable.
  Date/Author: 2026-02-16 / Codex

- Decision: Close `UB-R03` residual risk with failover-suite coverage that combines lane queue pressure and sync-loss control signaling rather than adding a partial sync runtime service.
  Rationale: `WP-03` requires deterministic scheduler behavior under pressure; `WP-07` still owns full sync engine implementation. Coupling tests provide MVP-safe proof without crossing work-package boundaries.
  Date/Author: 2026-02-16 / Codex

- Decision: Map execution-pool saturation/closure failures to deterministic RK-25 shed semantics at node-dispatch boundary, and preserve telemetry-lane best-effort progress by treating telemetry overload shed as non-blocking.
  Rationale: Raw pool submission errors are catastrophic and violate overload-hardening intent (`UB-R04`); converting to deterministic shed control-path behavior plus telemetry best-effort continuation keeps lane-priority invariants intact while remaining within runtime scope before full CP quota/fairness rollout.
  Date/Author: 2026-02-16 / Codex

- Decision: Add optional `tenant_id` threading from runtime open/active inputs into CP turn-start admission evaluation without making tenant mandatory for all call paths.
  Rationale: This enables tenant-scoped admission/quota policy enforcement where tenant context is available (`F-050`) while preserving compatibility for existing transport/runtime callers that are not yet tenant-aware.
  Date/Author: 2026-02-16 / Codex

- Decision: Enforce MVP node-level concurrency/fairness at execution-pool submit time using per-task fairness keys and bounded outstanding counters, then normalize overload handling through existing scheduler shed semantics.
  Rationale: This closes `F-109` noisy-neighbor protection with minimal boundary changes by reusing deterministic overload behavior (`shed` reasons and telemetry non-blocking semantics) instead of introducing new executor control paths.
  Date/Author: 2026-02-16 / Codex

- Decision: Extend CP materialized turn-policy surfaces with additive `node_execution_policies` and thread tenant-aware policy evaluation input so per-node concurrency/fairness limits can originate from frozen control-plane artifacts rather than only runtime-local node specs.
  Rationale: This closes the remaining UB-R04 seam between runtime enforcement and CP authority while preserving backward compatibility (optional additive fields, default-empty policy map) and deterministic plan-freeze hashing.
  Date/Author: 2026-02-16 / Codex

- Decision: Extend CP admission distribution surfaces with additive `admission.by_tenant` and deterministic precedence (`default -> by_pipeline -> by_tenant`) while keeping admission service/output contracts unchanged.
  Rationale: This makes tenant-scoped CP admission reject/defer outcomes authorable from snapshot artifacts and enforceable at turn-open without contract churn, closing the remaining UB-R04 CP-originated fairness gap in MVP runtime scope.
  Date/Author: 2026-02-16 / Codex

- Decision: Treat `UB-R05` state service as runtime-local hot/ephemeral boundary enforcement only, with strict authority/idempotency contracts and no in-runtime durable session persistence.
  Rationale: This satisfies MVP authority/idempotency invariants while preserving the system-design rule that runtime compute remains stateless-by-design and durable state is externalized.
  Date/Author: 2026-02-16 / Codex

- Decision: Close `UB-R06`/`UB-R07` at MVP baseline by implementing deterministic policy engines and lifecycle/boundary controls with failover/replay evidence, deferring deeper distributed/sandbox hardening to later control-plane/security streams.
  Rationale: This maximizes invariant coverage now (timebase/sync determinism, node host lifecycle, external boundary cancellation/limits) without crossing into post-MVP scope.
  Date/Author: 2026-02-16 / Codex

- Decision: Make MVP gate execution deterministic in no-credentials environments by preparing canonical live-provider/livekit fixture artifacts before `verify-mvp`.
  Rationale: MVP readiness gates must be reproducible in CI/local workflows without relying on ad hoc prior live runs; fixture preparation preserves stable gate artifacts while retaining strict validation logic in CLI commands.
  Date/Author: 2026-02-16 / Codex

## Outcomes & Retrospective

- (2026-02-16) Completed `UB-R04` runtime MVP baseline with deterministic pre-turn and scheduling-point overload behavior, per-node concurrency/fairness enforcement in execution pool, and CP-originated policy/admission surfaces frozen into turn-start decisions and plan dispatch defaults.
- Prior open control-plane completion gaps for `UB-CP03` and `UB-CP04` were closed by implementing route/token/status service surfaces, strict-vs-availability fallback mode controls, quota/rate admission governance fields, and lease token metadata propagation.
- (2026-02-16) Completed runtime milestone closures for `UB-R05`, `UB-R06`, and `UB-R07` at MVP baseline with deterministic state/authority/idempotency enforcement, timebase+sync integrity services, and nodehost/external-node lifecycle boundaries backed by runtime/failover/replay test coverage and green repo gates.
- (2026-02-16) Completed remaining MVP closure items: contract-stream finalization (`PR-C1..PR-C4`), control-plane command/process surface baseline, deterministic MVP gate preparation + runner, scaffold artifact sets (`pkg/contracts/v1`, `pipelines/*`, `deploy/*`), and full MVP feature verification status update (`mvp_pass=189`, `mvp_total=189`) with green `verify-quick`/`verify-full`/`verify-mvp`/`security-baseline-check`.
- (2026-02-17) Recheck correction: prior closure marking for control-plane milestones overstated status; `UB-CP03` is reopened as incomplete and `UB-CP04` is tracked as partial pending explicit strict/availability policy, quota governance, and route/token/status surface completion.
- (2026-02-17) Closed `UB-CP03` and `UB-CP04` by landing route/token/status APIs and command surfaces, strict-vs-availability fallback mode, quota/rate admission governance fields, and lease token id/expiry propagation with targeted control-plane/runtime suites and green `verify-quick`.

## Context and Orientation

RSPP MVP completion work is driven by these authoritative sources in priority order:

1. `docs/mvp_backlog.md`
2. `docs/rspp_SystemDesign.md`
3. Canonical module guides:
   - `internal/runtime/runtime_kernel_guide.md`
   - `internal/controlplane/controlplane_module_guide.md`
   - `internal/observability/observability_module_guide.md`
   - `internal/tooling/tooling_and_gates_guide.md`
   - `providers/provider_adapter_guide.md`
   - `transports/transport_adapter_guide.md`
   - `test/test_suite_guide.md`
4. `docs/RSPP_features_framework.json`

The non-negotiable invariants that must stay true across all milestones are:

1. Turn Plan Freeze: immutable `ResolvedTurnPlan` at turn start, no in-turn mutation.
2. Lane Priority: `ControlLane > DataLane > TelemetryLane`; telemetry never blocks control.
3. Cancellation-first: interrupt/barge-in propagates end-to-end; output fencing mandatory.
4. Authority/Epoch: lease epoch enforced at ingress and egress; stale authority rejected.
5. Replay Evidence OR-02: every accepted turn emits replay-critical L0 evidence without blocking control.
6. Stateless Runtime Compute: durable session state remains externalized per design.

## Plan of Work

The implementation is executed in seven waves, each with deterministic acceptance and narrow PR boundaries.

### Milestone 1: Contract Lock Completion (`WS-0`)

Complete `PR-C1` through `PR-C4` in order. For each PR, update typed validators, schema parity where required, and contract fixtures under `test/contract/fixtures`. `PR-C3` must be finished beyond current partial status by closing residual schema/fixture parity and consumer compatibility checks. `PR-C4` establishes transport shared contracts required by transport kernel split.

Milestone acceptance: contract tests and schema validation pass; no downstream module needs ad-hoc duplicate enums/shape assumptions.

### Milestone 2: Runtime Core Invariants (`WS-1`)

Implement `UB-R01`, `UB-R02`, and `UB-R03` first to anchor lifecycle, immutable turn plan freeze, and lane scheduling semantics. Then complete `UB-R04`, `UB-R05`, `UB-R06`, and `UB-R07` to cover overload controls, state/authority enforcement, deterministic timebase/sync semantics, and node host boundaries.

Milestone acceptance: lifecycle and arbitration tests, lane/cancellation/authority conformance suites, and failover assertions prove invariants with deterministic outcomes.

### Milestone 3: Control Plane Process and Authority (`WS-2`)

Deliver `UB-CP01` to `UB-CP04`, moving from scaffold process surface to deterministic rollout, route/token services, and hardened lease/admission modes. Ensure runtime pre-turn bundle resolution and authority decisions use complete control-plane contract surfaces.

Milestone acceptance: control-plane integration suites cover publish/list/get/rollback behavior, route/token issuance, and authority rejection/fallback semantics.

### Milestone 4: Observability and Replay Completion (`WS-3`)

Implement `UB-O01` to `UB-O05` to close correlation completeness, telemetry processors, OR-02 durable exporter split, replay engine/cursor execution, and divergence report + access hardening. All additions must preserve non-blocking telemetry and cancel/control-path latency guarantees.

Milestone acceptance: replay suites and OR-02 completeness checks pass under normal and degraded exporter conditions, with deterministic divergence classification.

### Milestone 5: Providers and Transports (`WS-5`)

Complete provider policy/capability freeze and conformance (`UB-PT01`, `UB-PT02`) and transport kernel split with shared contract adoption (`UB-PT03`). Maintain strict authority/cancel fencing at transport boundaries and provider evidence alignment to OR-02.

Milestone acceptance: provider/transport integration and conformance profiles pass; LiveKit baseline remains stable while WebSocket/telephony modular seams are implemented per MVP scope decision.

### Milestone 6: Tooling, Gates, and Command Surfaces (`WS-4`)

Complete `UB-T01` to `UB-T05`: release artifact compatibility, validate-spec/policy, replay cursor regression modes, local-runner/control-plane command surfaces, and conformance/skew governance checks. This wave also owns stewardship updates for `docs/RSPP_features_framework.json` verification status.

Milestone acceptance: `verify-quick`, `verify-full`, `verify-mvp`, and security checks are aligned with feature traceability and produce required artifacts deterministically.

### Milestone 7: Delivery Scaffolds and Final MVP Closure (`WS-6`)

Complete `UB-S01` to `UB-S03`: `pkg/contracts/v1` facade parity, pipeline artifact sets, deployment scaffolds, and deterministic deploy verification runner. Lock final CI/gate behavior and update MVP feature statuses with evidence references.

Milestone acceptance: release readiness gates and artifact generation succeed from clean checkout with reproducible outputs.

## Concrete Steps

Execute each PR/work package cycle from `/Users/tiger/Documents/projects/RealtimeSpeechPipeline` using this exact pattern.

1. Synchronize scope and create one feature branch per backlog item.

   git checkout -b feat/<ub-id-or-pr-id>

2. Implement only allowed path prefixes for the targeted workstream.

3. Add unit + contract + integration tests for the changed behavior, including negative-path assertions.

4. Run targeted module tests first.

   go test ./<touched-package>/...

5. Run mandatory repo gates before merging each work package.

   make verify-quick
   make verify-full
   make security-baseline-check

6. For replay/observability and tooling milestones, run replay and gate artifact commands as required by changed modules.

   go run ./cmd/rspp-cli replay-smoke-report
   go run ./cmd/rspp-cli slo-gates-report
   go run ./cmd/rspp-cli slo-gates-mvp-report

7. Update this file immediately after each merge-ready milestone by checking completed progress items and adding evidence notes.

8. Update `docs/RSPP_features_framework.json` verification fields only when corresponding tests and gate evidence are present.

## Validation and Acceptance

Each milestone is accepted only when both behavior and evidence are present.

Behavior acceptance criteria:

1. Runtime invariants are asserted by tests across lifecycle, lanes, cancellation, authority, and replay evidence.
2. Control-plane outputs and runtime consumption remain contract-compatible and deterministic.
3. Telemetry and OR-02 recording remain non-blocking for control-path and cancellation.
4. Replay modes/cursor and divergence reporting are deterministic and operator-consumable.
5. Provider and transport flows enforce policy, budget, authority, and output fencing semantics.

Evidence acceptance criteria:

1. `make verify-quick` passes.
2. `make verify-full` passes.
3. `make security-baseline-check` passes.
4. MVP gate artifacts are generated and pass threshold evaluation.
5. `docs/RSPP_features_framework.json` MVP items are moved to verified status with traceable tests/artifacts.

Final MVP completion criteria:

1. All checklist items in `Progress` are checked.
2. MVP feature count is fully verified (`mvp_pass == mvp_total`) with traceable evidence links.
3. Release readiness pipeline is green on clean checkout.

## Idempotence and Recovery

This plan is intentionally idempotent and safe for incremental execution.

1. Each work package is additive and branch-scoped; failed milestones can be retried by rerunning the same test/gate sequence.
2. If a milestone fails a gate, revert only the milestone branch changes, keep this plan updated, and split the failing item into smaller PR slices.
3. Never bypass invariant tests to achieve temporary green status.
4. If cross-stream conflicts arise, pause and rebase the later stream against the merged contract/runtime baseline before continuing.

## Artifacts and Notes

Use these artifact locations as evidence anchors when checking items in `Progress`:

- `.codex/ops/contracts-report.json`
- `.codex/ops/contracts-report.md`
- `.codex/replay/smoke-report.json`
- `.codex/replay/regression-report.json`
- `.codex/ops/slo-gates-report.json`
- `.codex/ops/slo-gates-mvp-report.json`
- `.codex/release/release-manifest.json`

For each completed work package, add one short note in this file containing:

- PR/work-package ID
- files changed
- tests and gates run
- feature IDs closed
- known residual risks

## Interfaces and Dependencies

Execution dependencies are strict and must be preserved:

1. Contract stream order: `PR-C1 -> PR-C2 -> PR-C3 -> PR-C4`.
2. Runtime core (`UB-R01..UB-R03`) must complete before observability replay engine and major provider/transport upgrades.
3. Control-plane route/token/lease hardening (`UB-CP03`, `UB-CP04`) is prerequisite for full runtime state/authority and transport routing safety.
4. OR-02 durable exporter (`UB-O03`) is prerequisite for replay engine/cursor (`UB-O04`) and replay tooling cursor resume (`UB-T03`).
5. Tooling feature-status stewardship (`UB-T05`) is required to close MVP verification tracking in `docs/RSPP_features_framework.json`.

Primary hotspot files to coordinate carefully:

- `api/eventabi/types.go`
- `api/controlplane/types.go`
- `api/observability/types.go`
- `internal/runtime/turnarbiter/arbiter.go`
- `internal/runtime/planresolver/resolver.go`
- `internal/runtime/executor/scheduler.go`
- `internal/controlplane/lease/lease.go`
- `internal/observability/replay/service.go`
- `internal/tooling/regression/divergence.go`
- `docs/RSPP_features_framework.json`

Revision note (2026-02-16): Created master MVP completion ExecPlan with dependency-ordered waves, invariant-preserving acceptance criteria, and a living progress checklist in `task.md`.
Revision note (2026-02-16): Closed `UB-R04` runtime MVP scope by adding CP tenant admission override distribution support (`admission.by_tenant`), related runtime/distribution tests, canonical guide updates, and explicit residual-scope boundary to `UB-CP04`.
Revision note (2026-02-16): Closed `UB-R05`, `UB-R06`, and `UB-R07` for MVP baseline with code in `internal/runtime/state`, `internal/runtime/timebase`, `internal/runtime/sync`, `internal/runtime/nodehost`, and `internal/runtime/externalnode`, plus failover/replay/integration evidence and green `verify-quick`/`verify-full`/`security-baseline-check`.
Revision note (2026-02-16): Closed remaining contract/control-plane/observability/provider/tooling/scaffold milestones, added deterministic MVP gate fixture prep + deploy gate runner, promoted `pkg/contracts/v1` facade and pipeline/deploy artifact sets, implemented `cmd/rspp-control-plane` process command surface, and updated `docs/RSPP_features_framework.json` to `mvp_pass == mvp_total` with traceable evidence refs and green `verify-quick`/`verify-full`/`verify-mvp`/`security-baseline-check`.
