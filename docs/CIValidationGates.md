# CI Validation Gates (Implemented Baseline)

## 1. Purpose and status

Define the currently implemented quick/full validation gates for this repository, plus optional non-blocking live-provider smoke checks and `.codex` artifact policy enforcement.

Status snapshot:
- Baseline reflects repository behavior as of `2026-02-11`.
- Verification runs through `scripts/verify.sh` and command chains in `Makefile`.
- Replay divergence and SLO gating behavior are enforced by `cmd/rspp-cli`.
- Replay fixture metadata invocation-latency thresholds are now evaluated against runtime-baseline-artifact OR-02 invocation evidence in full replay regression.
- Runtime CP backend bootstrap integration is now hardened with file/env/http-backed distribution adapters, authenticated HTTP fetch (`Authorization` + client identity), ordered endpoint failover, deterministic retry/backoff, on-demand TTL refresh with bounded stale-serving fallback, per-service partial-backend fallback, and stale-snapshot deterministic handling coverage.
- CP promotion-to-implemented gate scope for `CP-01/02/03/04/05/07/08/09/10` is closed at MVP scope with evidence for module behavior, service-client backend parity (`file`/`env`/`http`), backend-failure deterministic fallback handling, and synchronized conformance mappings.
- `.codex` generated artifact tracking policy is finalized and enforced in CI.
- This document is synchronized with the current MVP section-10 closure state in `docs/MVP_ImplementationSlice.md` (`10.1.22` closed; `10.2` currently has no open items).

## 2. Source of truth files

1. `Makefile`
2. `scripts/verify.sh`
3. `scripts/security-check.sh`
4. `scripts/check-codex-artifact-policy.sh`
5. `.gitignore`
6. `.github/workflows/verify.yml`
7. `cmd/rspp-cli/main.go`
8. `internal/tooling/regression/divergence.go`
9. `internal/tooling/ops/slo.go`
10. `test/replay/fixtures/metadata.json`
11. `internal/runtime/turnarbiter/controlplane_backends.go`
12. `internal/observability/replay/audit_backend.go`
13. `internal/observability/replay/audit_backend_http.go`
14. `internal/controlplane/distribution/retention_snapshot.go`
15. `internal/controlplane/distribution/file_adapter.go`
16. `internal/controlplane/distribution/http_adapter.go`
17. `internal/controlplane/graphcompiler/graphcompiler.go`
18. `internal/controlplane/admission/admission.go`
19. `internal/controlplane/lease/lease.go`
20. `internal/runtime/turnarbiter/controlplane_bundle.go`
21. `internal/runtime/turnarbiter/arbiter.go`

## 3. Verify entrypoint behavior (`scripts/verify.sh`)

`scripts/verify.sh` executes one command string selected by mode:

1. `quick` mode uses `VERIFY_QUICK_CMD` when set.
2. `full` mode uses `VERIFY_FULL_CMD` when set.
3. If unset, the script falls back to `make verify-$MODE` when available.

Repository convention:
- CI should invoke `make verify-quick` and `make verify-full`.
- Each Make target sets `VERIFY_*_CMD` explicitly before calling `bash scripts/verify.sh ...`.

## 4. Implemented gate composition

## 4.1 Quick gate (`make verify-quick`)

Implemented command chain:

```bash
go run ./cmd/rspp-cli validate-contracts &&
go run ./cmd/rspp-cli replay-smoke-report &&
go run ./cmd/rspp-cli generate-runtime-baseline &&
go run ./cmd/rspp-cli slo-gates-report &&
go test ./api/controlplane ./api/eventabi ./internal/runtime/planresolver ./internal/runtime/turnarbiter ./internal/runtime/executor ./internal/runtime/buffering ./internal/runtime/guard ./internal/runtime/transport ./internal/observability/replay ./internal/observability/timeline ./internal/observability/telemetry ./internal/tooling/regression ./internal/tooling/ops ./test/contract ./test/integration ./test/replay &&
go test ./test/failover -run 'TestF[137]'
```

Coverage summary:
1. Contract validation and schema-backed fixture checks.
2. Replay smoke artifact generation and replay divergence enforcement for fixture `rd-001-smoke`.
3. Runtime baseline artifact generation and MVP SLO gate evaluation.
4. Conformance package tests in `test/contract`, `test/integration`, and `test/replay`.
5. Failure smoke subset `F1`, `F3`, `F7`.
6. CP turn-start service integration checks for promoted modules `CP-01/02/03/04/05/07/08/09/10` through `turnarbiter` and distribution-backed resolver tests, including CP-02 simple-mode profile enforcement with deterministic unsupported-profile pre-turn handling and rollout/policy/provider-health fallback defaults under backend failure.
7. OR-01 telemetry module behavior coverage via targeted telemetry package tests and runtime instrumentation-path assertions.

## 4.2 Full gate (`make verify-full`)

Implemented command chain:

```bash
go run ./cmd/rspp-cli validate-contracts &&
go run ./cmd/rspp-cli replay-regression-report &&
go run ./cmd/rspp-cli generate-runtime-baseline &&
go run ./cmd/rspp-cli slo-gates-report &&
go test ./...
```

Coverage summary:
1. Full repository test suite (`go test ./...`), including failure matrix coverage.
2. Replay regression artifact generation and divergence enforcement for fixtures enabled for gate `full`.
3. Runtime baseline + SLO gate evaluation.
4. Full conformance coverage for CP turn-start resolver seams, including CP-01/02/03/04/05/07/08/09/10 parity and failure-path determinism (CP-02 unsupported-profile pre-turn handling, CP-03 compile output propagation, CP-05 reject/defer shaping, CP-07 lease-authority gating, CP-09/04/10 fallback defaults under backend outage).

## 4.3 Live provider smoke (`make live-provider-smoke`)

Implemented command:

```bash
go test -tags=liveproviders ./test/integration -run TestLiveProviderSmoke -v
```

Execution policy:
1. Runs in CI as a non-blocking job (`live-provider-smoke`) in `.github/workflows/verify.yml`.
2. Triggered on `schedule`, `workflow_dispatch`, and pull requests explicitly labeled `run-live-provider-smoke`.
3. Uses provider secrets/env when present; individual provider checks are skipped when disabled via env flags.
4. Current CI config enables real TTS smoke only for ElevenLabs (`RSPP_TTS_GOOGLE_ENABLE=0`, `RSPP_TTS_POLLY_ENABLE=0`).
5. Current CI config keeps Gemini disabled (`RSPP_LLM_GEMINI_ENABLE=0`), pins Anthropic to `claude-3-5-haiku-latest`, and uses OpenRouter-compatible Cohere settings.
6. Does not replace required merge gates `verify-quick` and `verify-full`.

## 4.4 A.2 runtime live matrix (`make a2-runtime-live`)

Implemented command chain:

```bash
RSPP_LIVE_PROVIDER_SMOKE=1 go test -tags=liveproviders ./test/integration -run TestLiveProviderSmoke -v &&
RSPP_A2_RUNTIME_LIVE=1 RSPP_A2_RUNTIME_LIVE_STRICT=1 go test -tags=liveproviders ./test/integration -run TestA2RuntimeLiveScenarios -v
```

Execution policy:
1. Runs in CI as a non-blocking job (`a2-runtime-live`) in `.github/workflows/verify.yml`.
2. Triggered on `schedule`, `workflow_dispatch`, and pull requests explicitly labeled `run-live-provider-smoke`.
3. Produces real-provider runtime matrix artifacts:
   - `.codex/providers/a2-runtime-live-report.json`
   - `.codex/providers/a2-runtime-live-report.md`
   - `.codex/providers/a2-runtime-live.log`
4. Uses strict mode in CI (`RSPP_A2_RUNTIME_LIVE_STRICT=1`) so skipped A.2 scenarios are treated as failures for that non-blocking job.

## 4.5 Security baseline gate (`make security-baseline-check`)

Implemented command:

```bash
make security-baseline-check
```

Execution policy:
1. Runs in CI as a blocking job (`security-baseline`) in `.github/workflows/verify.yml`.
2. Triggered on pull requests, scheduled runs, manual dispatch, and `main` pushes.
3. Executes focused security suites for classification enforcement, redaction policy, replay access authorization, and OR-02 redaction evidence validation.
4. Publishes machine/human-readable artifacts:
   - `.codex/ops/security-baseline-check.log`
   - `.codex/ops/security-baseline-report.json`
   - `.codex/ops/security-baseline-report.md`

## 4.6 `.codex` artifact policy gate (`make codex-artifact-policy-check`)

Implemented command:

```bash
make codex-artifact-policy-check
```

Execution policy:
1. Runs in CI as a blocking job (`codex-artifact-policy`) in `.github/workflows/verify.yml`.
2. Allows tracked `.codex` files only under:
   - `.codex/skills/**`
   - `.codex/rules/**`
3. Fails when any tracked `.codex` path is outside the allowlist.
4. Enforces generated artifact policy:
   - `.codex/replay/**`, `.codex/ops/**`, `.codex/providers/**`, `.codex/checkpoints/**`, `.codex/checkpoints.log`, and `.codex/sessions/**` are CI-only outputs and must remain untracked.
5. `verify-quick`, `verify-full`, `security-baseline`, `live-provider-smoke`, and `a2-runtime-live` depend on this policy gate.

## 5. Replay divergence fail policy (normative, implemented)

`replay-smoke-report` and `replay-regression-report` both fail when `FailingCount > 0`.

`internal/tooling/regression.EvaluateDivergences` marks a run failing when any of the following occurs:
1. `PLAN_DIVERGENCE` without matching expected metadata entry.
2. `OUTCOME_DIVERGENCE` without matching expected metadata entry.
3. Any `AUTHORITY_DIVERGENCE`.
4. `ORDERING_DIVERGENCE` without metadata expectation, or with expectation lacking `approved: true`.
5. `TIMING_DIVERGENCE` where `diff_ms` is missing or exceeds fixture timing tolerance.
6. Any expected divergence declared in metadata but not observed (`MissingExpected`).
7. Any unknown divergence class.
8. Invocation-latency threshold timing scopes (`invocation_latency_final:*`, `invocation_latency_total:*`) generated by threshold checks regardless of timing tolerance.

Fixture metadata source:
- `test/replay/fixtures/metadata.json`

Gate filtering:
1. `quick`: fixtures with `"gate": "quick"` or `"gate": "both"`.
2. `full`: fixtures with `"gate": "full"` or `"gate": "both"`.
3. If `gate` is omitted, it defaults to `full`.

Current expected-divergence annotations in metadata:
1. `rd-004-snapshot-provenance-plan`: expected `PLAN_DIVERGENCE`.
2. `rd-ordering-approved-1`: expected `ORDERING_DIVERGENCE` with `approved: true`.
3. `ml-003-replay-absence-classification`: expected `OUTCOME_DIVERGENCE`.

Current invocation-latency threshold annotations in metadata:
1. `rd-002-recompute-within-tolerance`, `rd-003-baseline-completeness`, `rd-ordering-approved-1`, `ae-001-preturn-stale-epoch`, `cf-001-cancel-fence`, and `ml-001-drop-under-pressure` define latency thresholds.
2. `rd-ordering-approved-1` sets `invocation_latency_scopes` to decouple threshold scope from fixture-id derivation.
3. Threshold checks are evaluated from runtime baseline artifact invocation outcomes (not synthetic fixture constants).

## 6. Artifact outputs and paths

Tracked `.codex` policy:
1. Source-controlled `.codex` paths are limited to `.codex/skills/**` and `.codex/rules/**`.
2. Other `.codex` paths are generated CI/local outputs and are not tracked in git.

Quick run artifacts:
1. `.codex/replay/smoke-report.json`
2. `.codex/replay/smoke-report.md`
3. `.codex/replay/runtime-baseline.json`
4. `.codex/ops/slo-gates-report.json`
5. `.codex/ops/slo-gates-report.md`

Full run artifacts:
1. `.codex/replay/regression-report.json`
2. `.codex/replay/regression-report.md`
3. `.codex/replay/fixtures/*.json`
4. `.codex/replay/fixtures/*.md`
5. `.codex/replay/runtime-baseline.json`
6. `.codex/ops/slo-gates-report.json`
7. `.codex/ops/slo-gates-report.md`

Security baseline artifacts:
1. `.codex/ops/security-baseline-check.log`
2. `.codex/ops/security-baseline-report.json`
3. `.codex/ops/security-baseline-report.md`

## 7. CI boundary status and remaining work

Implemented now:
1. `.github/workflows/verify.yml` runs blocking `codex-artifact-policy` before all other verification jobs.
2. Quick and full command chains return non-zero on validation/test/divergence/SLO failures.
3. Replay and SLO artifacts are generated locally under `.codex/`.
4. `.github/workflows/verify.yml` uploads quick/full artifacts with `if-no-files-found: error` so missing expected artifacts fail CI.
5. `.github/workflows/verify.yml` includes a non-blocking `live-provider-smoke` job for real-provider integration checks.
6. `.github/workflows/verify.yml` includes a non-blocking `a2-runtime-live` job with per-module A.2 runtime evidence artifacts.
7. `.github/workflows/verify.yml` includes a blocking `security-baseline` job for security/data-handling baseline enforcement.

Repository policy action (outside repo code):
1. Configure branch protection required checks:
   - `codex-artifact-policy` required for PR merge.
   - `verify-quick` required for PR merge.
   - `verify-full` required for protected `main`/release promotion path.
2. Keep `live-provider-smoke` non-required until flake/security posture is production-hardened.

## 8. Consistency references

1. `docs/ConformanceTestPlan.md` maps conformance IDs and suite coverage to concrete tests/fixtures.
2. `docs/MVP_ImplementationSlice.md` section `10` records closure state and post-MVP follow-ups (`10.1.22` closure + no current open `10.2` items).
