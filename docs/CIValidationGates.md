# CI Validation Gates (Immediate Baseline)

## 1. Purpose

Define CI gates that must run through `scripts/verify.sh` with explicit command targets for:
- quick validation
- full validation
- replay divergence fail behavior

This file is the source of truth for pre-implementation CI behavior.

## 2. `scripts/verify.sh` command targets

`scripts/verify.sh` supports env overrides. Set these in CI now:

```bash
export VERIFY_QUICK_CMD="make verify-quick"
export VERIFY_FULL_CMD="make verify-full"
```

Required Make targets (to add/maintain in repo root):

- `verify-quick`:
  1. schema and contract checks
  2. quick conformance subset
  3. replay smoke divergence check

- `verify-full`:
  1. full conformance suites
  2. integration/failure-injection matrix
  3. full replay regression with divergence report

## 3. Expected gate composition

## 3.1 Quick gate (`verify.sh quick`)

Minimum required checks:
1. Validate `docs/ContractArtifacts.schema.json` fixtures.
2. Run quick conformance subset from `docs/ConformanceTestPlan.md`.
3. Run smoke failure set (`F1`, `F3`, `F7`) from `docs/FailureInjectionMatrix.md`.
4. Produce replay divergence summary artifact.

Quick gate objective:
- fast merge protection for contract/lifecycle/authority regressions.

## 3.2 Full gate (`verify.sh full`)

Minimum required checks:
1. Run all conformance suites (`CT`, `RD`, `CF`, `AE`, `ML`).
2. Run full failure matrix (`F1`-`F8`).
3. Run replay regression on full fixture set with divergence report artifact.
4. Run race/soak checks where available.

Full gate objective:
- release readiness and deterministic behavior confidence.

## 4. Required pass criteria

## 4.1 Quick pass criteria

A quick run passes only when all are true:
1. 100% pass for selected quick tests.
2. No schema violations in contract artifacts.
3. No unexplained replay divergence in:
   - `PLAN_DIVERGENCE`
   - `OUTCOME_DIVERGENCE`
   - `AUTHORITY_DIVERGENCE`
4. `ORDERING_DIVERGENCE` count == 0 for quick fixtures.

`TIMING_DIVERGENCE` handling in quick gate:
- allowed only when within configured deterministic tolerance band.
- outside tolerance => fail.

## 4.2 Full pass criteria

A full run passes only when all are true:
1. 100% pass for all conformance and failure-injection tests.
2. No unexplained divergence classes in replay output.
3. Terminal lifecycle correctness is 100% (`commit` or `abort`, then `close`).
4. Authority safety invariant holds (zero accepted stale-epoch outputs).
5. OR-02 baseline evidence completeness is 100% for accepted turns.

## 5. Replay divergence fail conditions (normative)

CI MUST fail immediately on:
1. Any unexplained `PLAN_DIVERGENCE`.
2. Any unexplained `OUTCOME_DIVERGENCE`.
3. Any `AUTHORITY_DIVERGENCE`.
4. Any `ORDERING_DIVERGENCE` unless explicitly approved test scenario expects it.
5. `TIMING_DIVERGENCE` exceeding configured tolerance policy.

"Unexplained" means:
- no matching expected divergence annotation in fixture metadata, or
- mismatch between expected and observed divergence class/scope.

## 6. Required CI artifacts

Every CI run must upload:
1. conformance test summary (suite and case-level status)
2. failure injection run summary
3. replay divergence report (machine-readable + human-readable)
4. OR-02 baseline evidence completeness report
5. lifecycle correctness report (terminal sequence and pre-turn outcome checks)

## 7. Suggested implementation checklist

- [ ] Define `make verify-quick` and `make verify-full`.
- [ ] Export `VERIFY_QUICK_CMD` and `VERIFY_FULL_CMD` in CI pipeline.
- [ ] Add divergence report parser that returns non-zero on fail conditions.
- [ ] Block merge on quick gate failure.
- [ ] Block release on full gate failure.
