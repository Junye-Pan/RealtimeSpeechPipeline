# RSPP MVP Backlog (Unified Snapshot Merge)

_Generated: 2026-02-16 09:52:18Z_

## Inputs Rechecked
- `mvp_snapshots/runtime_module_snapshot_2026-02-16.md`
- `mvp_snapshots/control_plane_api_module_snapshot_2026-02-16.md`
- `mvp_snapshots/observability_api_module_snapshot_2026-02-16.md`
- `mvp_snapshots/providers_transports_module_snapshot_2026-02-16.md`
- `mvp_snapshots/tooling_test_cmd_module_snapshot_2026-02-16.md`
- `mvp_snapshots/scaffolds_module_snapshot_2026-02-16.md`
- `docs/rspp_SystemDesign.md`
- `docs/RSPP_features_framework.json`
- `docs/mvp_backlog.md` (v0 baseline)

## Merge Rules Applied
1. De-duplicated overlapping module items into unified backlog items.
2. Split cross-module items into:
   - contract PRs (`api/*`) when shared schema/types are required
   - module implementation PRs (`internal/*`, `providers/*`, `transports/*`, `tooling/*`, `cmd/*`, `test/*`).
3. Enforced conflict avoidance:
   - one active `api/*` stream at a time (serial contract lock)
   - one progress steward for `docs/RSPP_features_framework.json`
   - module path boundaries as primary ownership rule.

## PR Boundary Rules (Tight Scope)
- One PR should target one backlog item and one primary module prefix (plus directly related tests).
- No mega refactors; defer unrelated cleanup.
- Cross-module behavior changes require explicit split PRs (contract first, implementation second).

## Dependency DAG (Execution Order)

```text
PR-C1 Event ABI contract parity
  -> PR-C2 ControlPlane API expansion
    -> PR-R01 Session lifecycle manager
    -> PR-R02 Plan freeze upgrade
    -> PR-CP01 Control-plane process bootstrap
    -> PR-CP03 Session route/token service
    -> PR-CP04 Lease/admission/failure-mode hardening
  -> PR-C3 Observability API replay/access contracts
    -> PR-O04 Replay engine + cursor flow
    -> PR-T03 Replay regression CLI mode/cursor

PR-R01 + PR-R02 + PR-R03 (lane scheduler)
  -> PR-O01/O02/O03 (correlation, telemetry, OR-02 durable exporter)
  -> PR-PT01/PT02 (provider freeze/evidence/conformance)
  -> PR-T02 (gate conformance + release artifact alignment)

PR-O03 + PR-O04 + PR-T02
  -> PR-S03 (deploy verification harness)

Optional:
PR-C4 api/transport bootstrap
  -> PR-PT03 transport kernel split + reusable transport seams
```

## Contract PR Plan (Serial `api/*` Lock)

| PR | Scope | Allowed path prefixes | Unblocks |
| --- | --- | --- | --- |
| `PR-C1` | Event ABI parity (`timestamp_ms`, lane semantics, `sync_drop_policy.group_by`, `streaming_handoff` parity) | `api/eventabi`, `docs/ContractArtifacts.schema.json`, `test/contract/fixtures` | `PR-R03`, `PR-O01`, `PR-O02`, `PR-PT03`, transport/runtime lane safety work |
| `PR-C2` | ControlPlane API expansion (route/token/status/version/lease/state-idempotency fields) | `api/controlplane`, `docs/ContractArtifacts.schema.json`, `test/contract/fixtures` | `PR-R01`, `PR-R02`, `PR-R05`, `PR-CP01..PR-CP04`, provider freeze alignment |
| `PR-C3` | Observability API expansion (`ReplayMode`, `ReplayCursor`, replay request/result/report, decision validators) | `api/observability`, `docs/ContractArtifacts.schema.json`, `test/contract/fixtures` | `PR-O04`, `PR-O05`, `PR-T03`, replay/tooling cursor resume |
| `PR-C4` (optional) | Shared transport contract bootstrap | `api/transport`, `test/contract/fixtures` | `PR-PT03` reusable transport kernel split |

## Unified MVP Backlog (Grouped By Canonical Areas)

### 1) Contract Surfaces

| ID | Unified item | Feature/WP IDs | Split plan | Depends on |
| --- | --- | --- | --- | --- |
| `UB-C01` | Event ABI schema parity + validator completeness | `F-043`, `F-044`, `F-081`, `F-149`, `F-153` | Contract: `PR-C1`; Impl: `PR-R03`, `PR-O02`, `PR-PT03` | none |
| `UB-C02` | ControlPlane shared types for routing/lease/version/state/idempotency | `F-094`, `F-095`, `F-099`, `F-101`, `F-141`, `F-150`, `F-155..F-160` | Contract: `PR-C2`; Impl: `PR-R01`, `PR-R02`, `PR-R05`, `PR-CP03`, `PR-CP04` | `PR-C1` (serial API lock) |
| `UB-C03` | Observability replay/access/report contracts | `F-072`, `F-073`, `F-077`, `F-116`, `F-151`, `F-152`, `NF-009` | Contract: `PR-C3`; Impl: `PR-O04`, `PR-O05`, `PR-T03` | `PR-C2` |
| `UB-C04` | Shared transport API (optional MVP, required for full transport modularity) | `F-083`, `F-086`, `F-088`, `F-091` | Contract: `PR-C4`; Impl: `PR-PT03` | `PR-C1` |

### 2) Runtime Kernel

| ID | Unified item | Feature/WP IDs | Split plan | Depends on |
| --- | --- | --- | --- | --- |
| `UB-R01` | Session Lifecycle Manager (`OPEN -> ACTIVE -> (COMMIT|ABORT) -> CLOSE`) | `WP-01`, `F-138..F-141`, `F-175`, `NF-029` | `PR-R01` | `PR-C2` |
| `UB-R02` | Plan Freeze upgrade (remove resolver defaults; CP-derived immutable plan) | `WP-02`, `F-149`, `F-150`, `F-102`, `F-078`, `F-079`, `NF-009` | `PR-R02` | `PR-R01` |
| `UB-R03` | Lane Dispatch Scheduler (strict priority + bounded queues) | `WP-03`, `F-043`, `F-044`, `F-142..F-148`, `NF-004` | `PR-R03` | `PR-C1`, `PR-R02` |
| `UB-R04` | Admission + execution-pool overload hardening | `F-049`, `F-050`, `F-103`, `F-105`, `F-109`, `NF-024`, `NF-038` | `PR-R04` | `PR-R03`, `PR-CP04` |
| `UB-R05` | State service + authority/idempotency enforcement | `WP-05`, `F-155`, `F-156`, `F-158..F-160`, `NF-018`, `NF-027` | `PR-R05` | `PR-C2`, `PR-CP04` |
| `UB-R06` | Timebase + sync integrity engine + deterministic downgrade semantics | `WP-06`, `WP-07`, `F-146`, `F-150`, `F-152..F-154`, `F-176`, `NF-009`, `NF-018` | `PR-R06` | `PR-R02`, `PR-R03` |
| `UB-R07` | Node host lifecycle completion + external-node runtime boundary | `WP-04`, `WP-08`, `F-011`, `F-012`, `F-036`, `F-037`, `F-121..F-124`, `F-134`, `F-137` | `PR-R07a` + `PR-R07b` | `PR-R02`, `PR-C2`, (`PR-C4` optional) |

### 3) Control Plane

| ID | Unified item | Feature/WP IDs | Split plan | Depends on |
| --- | --- | --- | --- | --- |
| `UB-CP01` | Control-plane process bootstrap (publish/list/get/rollback + immutable audit) | `F-092`, `F-097`, `F-100`, `F-101`, `F-102`, `CP-13` | `PR-CP01` | `PR-C2` |
| `UB-CP02` | Deterministic rollout semantics (canary/percentage/allowlist/rollback) | `F-093`, `F-100`, `CP-04` | `PR-CP02` | `PR-CP01` |
| `UB-CP03` | Session route + token + status service | `F-094`, `F-099`, `F-101`, `CPAPI-03`, `CPAPI-04` | `PR-CP03` | `PR-C2`, `PR-CP01` |
| `UB-CP04` | Lease/admission/fallback hardening (strict-vs-availability mode, tenant quotas) | `F-095`, `F-096`, `F-098`, `F-156`, `F-157`, `F-159` | `PR-CP04` | `PR-C2`, `PR-CP02` |

### 4) Observability & Replay

| ID | Unified item | Feature/WP IDs | Split plan | Depends on |
| --- | --- | --- | --- | --- |
| `UB-O01` | Canonical correlation resolver (`session_id`, `turn_id`, `pipeline_version`, node/edge IDs) | `F-067`, `NF-008`, `OBS-01` | `PR-O01` | `PR-C1`, `PR-R01` |
| `UB-O02` | Telemetry metrics/traces completeness (latency, queue/drop/merge, linkage labels) | `F-068`, `F-069`, `F-070`, `OBS-02`, `OBS-03` | `PR-O02` | `PR-O01`, `PR-R03` |
| `UB-O03` | OR-02 durable exporter separation (non-blocking path + retention closure) | `F-071`, `F-154`, `F-176`, `NF-028`, `OBS-05`, `OBS-06` | `PR-O03` | `PR-R02`, `PR-R06` |
| `UB-O04` | Replay engine + deterministic cursor execution | `F-072`, `F-151`, `F-152`, `NF-009`, `OBS-07` | `PR-O04` | `PR-C3`, `PR-O03` |
| `UB-O05` | Replay access/redaction hardening + divergence report artifact pipeline | `F-073`, `F-077`, `F-116`, `F-127`, `NF-011`, `NF-014`, `OBS-08`, `OBS-09`, `OBS-10` | `PR-O05a` + `PR-O05b` | `PR-C3`, `PR-O04` |

### 5) Providers & Transports

| ID | Unified item | Feature/WP IDs | Split plan | Depends on |
| --- | --- | --- | --- | --- |
| `UB-PT01` | Provider policy/capability freeze + budget/retry enforcement | `F-056`, `F-057`, `F-058`, `F-059`, `F-060`, `F-062`, `F-066` | `PR-PT01` | `PR-R02`, `PR-C2` |
| `UB-PT02` | Provider evidence/secret-ref/config normalization + adapter conformance suites | `F-055`, `F-061`, `F-063`, `F-064`, `F-065`, `F-149`, `F-153`, `F-154`, `F-160`, `F-164`, `F-176`, `NF-013` | `PR-PT02` | `PR-PT01`, `PR-O03` |
| `UB-PT03` | Transport kernel split + buffering/codec/control/routing safety | `F-083`, `F-086..F-091`, `NF-031` | `PR-PT03a` + `PR-PT03b` | `PR-C1`, (`PR-C4` if adopted), `PR-R03`, `PR-CP03` |

### 6) Tooling, Gates, and Cmd

| ID | Unified item | Feature/WP IDs | Split plan | Depends on |
| --- | --- | --- | --- | --- |
| `UB-T01` | Release artifact schema compatibility alignment (`publish-release` chain) | `F-093`, `F-100`, `NF-016`, `NF-032` | `PR-T01` | `PR-O03` |
| `UB-T02` | CLI `validate-spec` + `validate-policy` and gate conformance tests | `F-001`, `F-126`, `NF-010`, `NF-024..NF-029`, `NF-038` | `PR-T02` | `PR-T01` |
| `UB-T03` | Replay regression modes + cursor resume in CLI/gates | `F-127`, `F-151`, `F-152`, `NF-009` | `PR-T03` | `PR-C3`, `PR-O04`, `PR-T01` |
| `UB-T04` | Loopback local-runner + control-plane command MVP surface | `F-092`, `F-095`, `F-099`, `F-101`, `F-119`, `F-125`, `F-130` | `PR-T04a` + `PR-T04b` | `PR-CP01`, `PR-CP03` |
| `UB-T05` | Conformance/skew governance checks + feature-status stewardship | `F-162`, `F-163`, `F-164`, `NF-010`, `NF-032` | `PR-T05` | `PR-T02`, `PR-T03` |

### 7) Delivery Scaffolds

| ID | Unified item | Feature/WP IDs | Split plan | Depends on |
| --- | --- | --- | --- | --- |
| `UB-S01` | `pkg/contracts/v1` facade promotion with parity tests | `BL-034`, `F-020`, `F-021`, `F-025`, `F-028`, `F-032`, `F-149`, `F-153`, `NF-010`, `NF-032` | `PR-S01` | `PR-C1`, `PR-C2`, `PR-C3` |
| `UB-S02` | Pipeline artifact set (spec/profile/bundle/rollout/policy/compat/extensions/replay manifests) | `BL-032`, `F-001`, `F-015`, `F-019`, `F-078`, `F-081`, `F-092`, `F-093`, `F-100`, `F-102`, `F-121..F-123`, `F-127`, `F-129`, `F-153`, `F-154`, `F-176`, `NF-024..NF-029`, `NF-038` | `PR-S02a` + `PR-S02b` | `PR-CP02`, `PR-R02`, `PR-O03` |
| `UB-S03` | Deploy scaffolds (K8s/Helm/Terraform) + deterministic gate runner | `BL-033`, `F-103`, `F-106`, `F-108`, `F-114`, `F-119`, `F-120`, `F-125`, `F-126`, `F-127`, `F-129`, `NF-007`, `NF-011`, `NF-012`, `NF-016`, `NF-031` | `PR-S03a` + `PR-S03b` | `PR-T01`, `PR-S02`, `PR-CP04` |

## Parallel Workstream Plan (Strict Prefix Ownership)

### WS-0: Contracts (serial lock stream)
- Allowed path prefixes:
  - `api/eventabi`, `api/controlplane`, `api/observability`, `api/transport` (optional)
  - `docs/ContractArtifacts.schema.json`
  - `test/contract/fixtures`
- Execution order:
  1. `PR-C1`
  2. `PR-C2`
  3. `PR-C3`
  4. `PR-C4` (optional)
- Hotspot files (exclusive while stream is active):
  - `api/eventabi/types.go`
  - `api/controlplane/types.go`
  - `api/observability/types.go`
  - `docs/ContractArtifacts.schema.json`

### WS-1: Runtime Kernel
- Allowed path prefixes:
  - `internal/runtime/session`
  - `internal/runtime/prelude`
  - `internal/runtime/turnarbiter`
  - `internal/runtime/planresolver`
  - `internal/runtime/lanes`
  - `internal/runtime/buffering`
  - `internal/runtime/flowcontrol`
  - `internal/runtime/localadmission`
  - `internal/runtime/executionpool`
  - `internal/runtime/state`
  - `internal/runtime/nodehost`
  - `internal/runtime/timebase`
  - `internal/runtime/sync`
  - `test/integration/runtime_*`
  - `test/failover/*`
- Execution order:
  1. `UB-R01`
  2. `UB-R02`
  3. `UB-R03`
  4. `UB-R04`
  5. `UB-R05`
  6. `UB-R06`
  7. `UB-R07`
- Hotspot files (do not edit from other streams):
  - `internal/runtime/turnarbiter/arbiter.go`
  - `internal/runtime/planresolver/resolver.go`
  - `internal/runtime/executor/scheduler.go`
  - `internal/runtime/prelude/engine.go`

### WS-2: Control Plane
- Allowed path prefixes:
  - `internal/controlplane`
  - `cmd/rspp-control-plane`
  - `test/integration/controlplane_*`
- Execution order:
  1. `UB-CP01`
  2. `UB-CP02`
  3. `UB-CP03`
  4. `UB-CP04`
- Hotspot files:
  - `internal/controlplane/rollout/rollout.go`
  - `internal/controlplane/lease/lease.go`
  - `internal/controlplane/admission/admission.go`
  - `cmd/rspp-control-plane/main.go`

### WS-3: Observability & Replay
- Allowed path prefixes:
  - `internal/observability/telemetry`
  - `internal/observability/timeline`
  - `internal/observability/replay`
  - `test/replay`
  - `test/integration/observability_*`
- Execution order:
  1. `UB-O01`
  2. `UB-O02`
  3. `UB-O03`
  4. `UB-O04`
  5. `UB-O05`
- Hotspot files:
  - `internal/observability/telemetry/pipeline.go`
  - `internal/observability/timeline/recorder.go`
  - `internal/observability/replay/service.go`
  - `internal/observability/replay/comparator.go`

### WS-4: Tooling & Gates (also progress steward)
- Allowed path prefixes:
  - `internal/tooling`
  - `cmd/rspp-cli`
  - `cmd/rspp-local-runner`
  - `test/integration/gq_*`
  - `test/framework`
  - `docs/RSPP_features_framework.json` (exclusive steward lock)
- Execution order:
  1. `UB-T01`
  2. `UB-T02`
  3. `UB-T03`
  4. `UB-T04`
  5. `UB-T05`
- Hotspot files:
  - `internal/tooling/release/release.go`
  - `internal/tooling/ops/slo.go`
  - `internal/tooling/regression/divergence.go`
  - `cmd/rspp-cli/main.go`
  - `docs/RSPP_features_framework.json`

### WS-5: Providers & Transports
- Allowed path prefixes:
  - `internal/runtime/provider`
  - `providers`
  - `internal/runtime/transport`
  - `transports`
  - `test/contract/provider`
  - `test/integration/livekit_*`
- Execution order:
  1. `UB-PT01`
  2. `UB-PT02`
  3. `UB-PT03`
- Hotspot files:
  - `internal/runtime/provider/invocation/controller.go`
  - `internal/runtime/provider/contracts/contracts.go`
  - `internal/runtime/transport/fence.go`
  - `transports/livekit/adapter.go`

### WS-6: Scaffolds (optional parallel stream)
- Allowed path prefixes:
  - `pkg/contracts/v1`
  - `pipelines`
  - `deploy`
  - `test/contract/pipeline_*`
  - `deploy/verification`
- Execution order:
  1. `UB-S01`
  2. `UB-S02`
  3. `UB-S03`
- Hotspot files:
  - `pkg/contracts/v1/core/types.go`
  - `pipelines/bundles/voice_assistant_bundle_v1.json`
  - `deploy/profiles/mvp-single-region/rollout.json`
  - `deploy/verification/gates/mvp_gate_runner.sh`

## Repo Health Snapshot (Rechecked 2026-02-16)

| Check | Result | Evidence | Assigned owner |
| --- | --- | --- | --- |
| `make verify-quick` | Pass | contracts/replay-smoke/runtime-baseline/slo reports generated; targeted suites pass | All streams (baseline green) |
| `make security-baseline-check` | Pass | focused security suites pass (`api/eventabi`, `api/observability`, security + replay/timeline paths) | WS-3 + Security reviewers |
| `make verify-full` | Pass | full `go test ./...` + contract/replay regression reports pass | All streams |
| `make a2-runtime-live` | **Fail** (optional gate) | `TestA2RuntimeLiveScenarios`: strict mode failure due no enabled providers (`S5 skipped in strict mode`) | WS-5 primary, WS-4 secondary (strict-mode policy/CI gating) |
| `docs/RSPP_features_framework.json` status | At risk | `mvp_total=189`, `mvp_pass=0` tracked as pass=false | WS-4 exclusive progress steward |

## Known Assumptions
- Required CI gates for MVP merge readiness are `verify-quick`, `verify-full`, and `security-baseline`; live-provider and A2-live remain environment-gated.
- `api/*` contract changes are merged in the listed serial order and not batched together.
- Runtime P0 (`UB-R01`/`UB-R02`/`UB-R03`) is treated as the main unblocker for downstream observability/tooling/provider milestones.
- Cross-stream test edits must stay in stream-specific test prefixes to avoid merge contention.

## Open Questions
1. Should `api/transport` (`PR-C4`) be in MVP scope or deferred while keeping LiveKit-only implementation paths?
Let `PR-C4` be in MVP scope.

2. Should `make a2-runtime-live` strict mode remain fail-on-skip, or be moved behind an explicit credentials-ready gate in CI?
In CI.

3. Who is the named human owner for WS-4 progress stewardship of `docs/RSPP_features_framework.json` updates?
4. Are lease token semantics in `UB-C02` expected to be signed artifacts in MVP, or reference-only with expiry metadata?
MVP
