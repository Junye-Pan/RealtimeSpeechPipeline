# RSPP MVP First Implementation Slice

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

## 10. Execution sequence to start coding

1. Freeze MVP artifacts and schemas (section 6).
2. Scaffold Go module boundaries to match the MVP subset (section 5).
3. Implement turn-start gating path first:
   - RK-02 -> RK-25/RK-24 -> RK-04 -> RK-03
4. Implement minimal execution path:
   - LiveKit ingress -> STT -> LLM -> TTS -> LiveKit egress
5. Add OR-02 baseline recorder and enforce commit/abort-close ordering.
6. Add cancellation fencing and authority validation enforcement.
7. Enable DX/CI gates and replay regression checks before adding new scope.
