# CI Validation Gates (Implemented Baseline)

## 1. Purpose and status

Define the currently implemented quick/full validation gates for this repository, plus optional non-blocking live-provider smoke checks.

Status snapshot:
- Baseline reflects repository behavior as of `2026-02-09`.
- Verification runs through `scripts/verify.sh` and command chains in `Makefile`.
- Replay divergence and SLO gating behavior are enforced by `cmd/rspp-cli`.
- This documentation pass closes MVP item `10.2.3` from `docs/MVP_ImplementationSlice.md`.

## 2. Source of truth files

1. `Makefile`
2. `scripts/verify.sh`
3. `cmd/rspp-cli/main.go`
4. `internal/tooling/regression/divergence.go`
5. `internal/tooling/ops/slo.go`
6. `test/replay/fixtures/metadata.json`

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
go test ./api/controlplane ./api/eventabi ./internal/runtime/planresolver ./internal/runtime/turnarbiter ./internal/runtime/executor ./internal/runtime/buffering ./internal/runtime/guard ./internal/runtime/transport ./internal/observability/replay ./internal/observability/timeline ./internal/tooling/regression ./internal/tooling/ops ./test/contract ./test/integration ./test/replay &&
go test ./test/failover -run 'TestF[137]'
```

Coverage summary:
1. Contract validation and schema-backed fixture checks.
2. Replay smoke artifact generation and replay divergence enforcement for fixture `rd-001-smoke`.
3. Runtime baseline artifact generation and MVP SLO gate evaluation.
4. Conformance package tests in `test/contract`, `test/integration`, and `test/replay`.
5. Failure smoke subset `F1`, `F3`, `F7`.

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

## 6. Artifact outputs and paths

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

## 7. CI boundary status and remaining work

Implemented now:
1. Quick and full command chains return non-zero on validation/test/divergence/SLO failures.
2. Replay and SLO artifacts are generated locally under `.codex/`.
3. `.github/workflows/verify.yml` uploads quick/full artifacts with `if-no-files-found: error` so missing expected artifacts fail CI.
4. `.github/workflows/verify.yml` includes a non-blocking `live-provider-smoke` job for real-provider integration checks.

Repository policy action (outside repo code):
1. Configure branch protection required checks:
   - `verify-quick` required for PR merge.
   - `verify-full` required for protected `main`/release promotion path.
2. Keep `live-provider-smoke` non-required until flake/security posture is production-hardened.

## 8. Consistency references

1. `docs/ConformanceTestPlan.md` maps conformance IDs and suite coverage to concrete tests/fixtures.
2. `docs/MVP_ImplementationSlice.md` section `10.2` tracks remaining closure items.
