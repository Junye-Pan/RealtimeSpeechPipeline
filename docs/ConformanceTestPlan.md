# Conformance Test Plan (Pre-Implementation)

## 1. Purpose

Define mandatory conformance tests before feature implementation to enforce:
- contract correctness
- deterministic replay behavior
- cancellation fencing guarantees
- authority epoch safety
- merge/drop lineage integrity

## 2. Test suite structure

| Suite ID | Suite name | Primary contracts validated |
| --- | --- | --- |
| `CT` | Contract tests | Event ABI, ControlLane signal registry, Turn lifecycle, ResolvedTurnPlan schema, Decision outcome schema |
| `RD` | Replay determinism tests | OR-02 baseline evidence, OR-03 divergence rules, determinism markers |
| `CF` | Cancellation fencing tests | Cancel propagation and egress fencing invariants |
| `AE` | Authority epoch tests | Lease/epoch validation and stale-writer rejection |
| `ML` | Merge/drop lineage tests | deterministic `drop_notice`, merge lineage completeness |

## 3. Required test cases

## 3.1 Contract tests (`CT`)

| Test ID | Description | Pass criteria |
| --- | --- | --- |
| `CT-001` | Validate sample event envelopes against `docs/ContractArtifacts.schema.json` | 100% schema pass for valid fixtures; 100% rejection for invalid fixtures |
| `CT-002` | Validate ControlLane signals and emitting module ownership constraints | all known signals map to allowed emitters; unknown signal rejected |
| `CT-003` | Validate turn transition legality | only allowed transitions accepted; illegal transitions rejected; pre-turn plan-materialization failure remains pre-turn (`Opening -> Idle` via deterministic `defer`/`reject`) with no `turn_open`/`abort`/`close` |
| `CT-004` | Validate `ResolvedTurnPlan` frozen fields and provenance refs | all required frozen fields present and immutable in-turn |
| `CT-005` | Validate decision outcome schema and authority/admission emitter mapping | authority outcomes only from RK-24; admission outcomes only from RK-25/CP-05; `shed` constrained to scheduling-point phase; phase/scope constraints and `scope=turn -> turn_id` requirement enforced |

## 3.2 Replay determinism tests (`RD`)

| Test ID | Description | Pass criteria |
| --- | --- | --- |
| `RD-001` | Replay same trace with replay-decisions mode | no unexplained `PLAN_DIVERGENCE`/`OUTCOME_DIVERGENCE`; zero `ORDERING_DIVERGENCE`; zero `AUTHORITY_DIVERGENCE`; `TIMING_DIVERGENCE` only within configured tolerance policy |
| `RD-002` | Replay with recompute-decisions when deterministic inputs are complete | deterministic outputs match baseline or produce explained `TIMING_DIVERGENCE` only within tolerance policy |
| `RD-003` | Verify OR-02 baseline evidence completeness at L0 | required baseline evidence fields present for every accepted turn |
| `RD-004` | Validate snapshot provenance influence on divergence classification | modified snapshot provenance must produce `PLAN_DIVERGENCE` |

## 3.3 Cancellation fencing tests (`CF`)

| Test ID | Description | Pass criteria |
| --- | --- | --- |
| `CF-001` | Cancel during active generation | no accepted DataLane output after fencing point for canceled scope |
| `CF-002` | Provider/external late output after cancel | late output marked and dropped/fenced deterministically |
| `CF-003` | Turn terminalization after cancel | when no same-point hard authority revoke is present, `abort(reason=cancelled)` followed by `close` exactly once |
| `CF-004` | Cancel observability markers | `cancel_sent_at` and related markers recorded in OR-02 evidence |

## 3.4 Authority epoch tests (`AE`)

| Test ID | Description | Pass criteria |
| --- | --- | --- |
| `AE-001` | Pre-turn stale epoch injection | emit `stale_epoch_reject`; no `turn_open`; no `abort/close` |
| `AE-002` | In-turn authority revoke | emit `deauthorized_drain`, then `abort(authority_loss)`, then `close`; if cancel and revoke are simultaneous at the same arbitration point, authority-loss path wins deterministically |
| `AE-003` | Old placement stale output during migration | emit `stale_epoch_reject` at scheduling point; no split-brain accepted output; no authoritative turn-state mutation from stale diagnostic alone |
| `AE-004` | Ingress authority enrichment fallback path | if transport metadata missing, deterministic enrichment occurs before acceptance |
| `AE-005` | Pre-turn authority deauthorization before open | emit `deauthorized_drain`; no `turn_open`; no `abort/close` |

## 3.5 Merge/drop lineage tests (`ML`)

| Test ID | Description | Pass criteria |
| --- | --- | --- |
| `ML-001` | Deterministic drop under pressure | `drop_notice` emitted with valid seq range and reason |
| `ML-002` | Deterministic merge/coalesce | merged event contains lineage metadata (`merge_group_id` and source refs/span) |
| `ML-003` | Replay explainability for drop vs absence | replay distinguishes dropped/merged events from upstream absence |
| `ML-004` | Sync-domain discontinuity under drop_with_discontinuity | discontinuity signal present and downstream reset path deterministic |

## 4. Fixture requirements

1. Provide canonical fixture pack:
   - valid/invalid ABI events
   - valid/invalid turn transitions
   - provider timeout/overload traces
   - lease/epoch rotation traces
   - merge/drop pressure traces

2. Fixture metadata must include:
   - seed
   - pipeline version
   - snapshot provenance ids
   - expected divergence annotations (if any), including divergence class and scope
   - explicit approval marker for any fixture that expects `ORDERING_DIVERGENCE`
   - timing tolerance policy reference for fixtures that expect `TIMING_DIVERGENCE`
   - no fixture may declare expected `AUTHORITY_DIVERGENCE`

## 5. Gate criteria

Quick conformance gate:
- run `CT-001..005`, `RD-001`, `CF-001`, `AE-001`, `AE-005`, `ML-001`
- required: 100% pass, zero `AUTHORITY_DIVERGENCE`, zero `ORDERING_DIVERGENCE` for quick fixtures, and `TIMING_DIVERGENCE` only within configured tolerance policy
- quick fixture metadata must not declare expected `ORDERING_DIVERGENCE`

Full conformance gate:
- run all suites and all cases
- required: 100% pass with no unexplained divergence classes, zero `AUTHORITY_DIVERGENCE`, `ORDERING_DIVERGENCE` only for explicitly approved scenarios in fixture metadata, and `TIMING_DIVERGENCE` only within configured tolerance policy
- release pipelines may extend this conformance gate with failure-injection and race/soak checks where available

## 6. Exit criteria before feature expansion

1. All quick and full conformance gates pass in CI.
2. Failures produce deterministic diagnostics with replay evidence links.
3. No open contract ambiguity remains for section-4 baseline artifacts.
