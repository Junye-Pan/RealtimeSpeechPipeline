# Runtime Kernel Guide

This folder is the execution kernel for deterministic turn lifecycle, scheduling, provider invocation, buffering/flow-control, cancellation, authority safety, and boundary orchestration.

## Ownership and module map

| Module | Path | Primary owner | Status |
| --- | --- | --- | --- |
| `RK-01` Session Lifecycle Manager | `internal/runtime/session` | `Runtime-Team` | scaffolded |
| `RK-02` Session Prelude | `internal/runtime/prelude` | `Runtime-Team` | implemented baseline |
| `RK-03` Turn Arbiter | `internal/runtime/turnarbiter` | `Runtime-Team` | implemented baseline |
| `RK-04` ResolvedTurnPlan Resolver | `internal/runtime/planresolver` | `Runtime-Team` | implemented baseline |
| `RK-05` Event ABI Gateway | `internal/runtime/eventabi` | `Runtime-Team` | implemented baseline |
| `RK-06` Lane Router | `internal/runtime/lanes` | `Runtime-Team` | implemented baseline |
| `RK-07` Graph Runtime Executor | `internal/runtime/executor` | `Runtime-Team` | implemented baseline |
| `RK-08` Node Runtime Host | `internal/runtime/nodehost` | `Runtime-Team` | implemented baseline |
| `RK-09` External Node Boundary Gateway | `internal/runtime/externalnode` | `Runtime-Team` | scaffolded |
| `RK-10` Provider Adapter Manager | `internal/runtime/provider/registry` | `Provider-Team` | implemented baseline |
| `RK-11` Provider Invocation Controller | `internal/runtime/provider/invocation` | `Provider-Team` | implemented baseline |
| `RK-12`/`RK-13` Buffering + Watermarks | `internal/runtime/buffering` | `Runtime-Team` | implemented baseline |
| `RK-14` Flow Control | `internal/runtime/flowcontrol` | `Runtime-Team` | implemented baseline |
| `RK-15` Sync Integrity Engine | `internal/runtime/sync` | `Runtime-Team` | scaffolded |
| `RK-16` Cancellation | `internal/runtime/cancellation` | `Runtime-Team` | implemented baseline |
| `RK-17` Budget | `internal/runtime/budget` | `Runtime-Team` | implemented baseline |
| `RK-18` Timebase Service | `internal/runtime/timebase` | `Runtime-Team` | scaffolded |
| `RK-19` Determinism | `internal/runtime/determinism` | `Runtime-Team` | implemented baseline |
| `RK-20` State Access | `internal/runtime/state` | `Runtime-Team` | scaffolded |
| `RK-21` Identity/Correlation | `internal/runtime/identity` | `Runtime-Team` | implemented baseline |
| `RK-22`/`RK-23` Transport Boundary | `internal/runtime/transport` | `Transport-Team` | implemented baseline |
| `RK-24` Runtime Contract Guard | `internal/runtime/guard` | `Runtime-Team` | implemented baseline |
| `RK-25` Local Admission Enforcer | `internal/runtime/localadmission` | `Runtime-Team` | implemented baseline |
| `RK-26` Execution Pool Manager | `internal/runtime/executionpool` | `Runtime-Team` | implemented baseline |
| Provider bootstrap wiring | `internal/runtime/provider/bootstrap` | `Provider-Team` | implemented baseline |

## Lifecycle state model

Canonical states:
- `Idle`
- `Opening`
- `Active`
- `Terminal`
- `Closed`

Deterministic precedence rules:
1. Pre-turn authority/admission failures win over opening a turn.
2. `turn_open` requires successful admission/authority checks and successful `ResolvedTurnPlan` materialization.
3. Accepted turns emit exactly one terminal outcome, then `close`.
4. Pre-turn outcomes (`reject`, `defer`, `stale_epoch_reject`, pre-turn `deauthorized_drain`) never emit `abort`/`close`.
5. Same-point cancel vs hard authority revoke chooses authority-loss path deterministically.
6. Plan materialization failure before open remains pre-turn and does not create accepted-turn lifecycle transitions.

## Truth-table summary (migrated from lifecycle table)

| Case class | Required behavior |
| --- | --- |
| Pre-turn snapshot/policy failure | emit deterministic `reject` or `defer`; no `turn_open`, `abort`, `close` |
| Pre-turn stale authority | emit `stale_epoch_reject`; no accepted-turn transition |
| Pre-turn deauthorization | emit `deauthorized_drain`; no accepted-turn transition |
| Accepted-turn success | emit `commit` then `close` |
| Accepted-turn cancel | emit `abort(cancelled)` then `close`, unless same-point hard authority revoke exists |
| Accepted-turn authority loss | emit `deauthorized_drain`, `abort(authority_loss)`, `close` |
| Baseline evidence append unavailable | emit `abort(recording_evidence_unavailable)` then `close` |
| Late stale output/event from non-authoritative placement | emit stale diagnostic outcome without reopening closed turn |

## Deterministic pseudocode order

```text
on turn_open_proposed:
  if snapshot_or_policy_invalid -> emit reject/defer and return pre-turn
  if authority_epoch_invalid -> emit stale_epoch_reject and return pre-turn
  if pre_turn_deauthorized -> emit deauthorized_drain and return pre-turn
  if plan_materialization_failed -> emit reject/defer and return pre-turn
  emit turn_open and enter Active
```

```text
while Active:
  if hard_authority_revoke -> deauthorized_drain, abort(authority_loss), close
  else if cancel_accepted -> abort(cancelled), close
  else if no_legal_continue_path -> abort(deterministic_reason), close
  else if success_ready -> commit, close
```

## Integration sequence owned here

1. Turn start: prelude -> CP bundle resolution -> plan freeze -> `turn_open`.
2. Barge-in/cancel: cancellation accepted -> output fencing -> deterministic terminalization.
3. Commit and delivery: terminal success path emits `commit` then `close`.
4. Migration/failover: old placement output is stale-rejected; new placement continues under new epoch.
5. External node boundary (`RK-09`): future extension point for policy-constrained external execution.

## PRD MVP fixed-scope alignment

1. Transport path for MVP validation is LiveKit-only (`RK-22`/`RK-23` runtime transport boundary path).
2. MVP execution mode is Simple-mode only: execution defaults come from `ExecutionProfile`; advanced per-edge/per-node override behavior is deferred.
3. MVP authority model is single-region execution authority with lease/epoch enforcement.
4. Replay scope dependency for runtime MVP gates is OR-02 baseline (`L0`) evidence completeness.

## ExecutionProfile/Simple-mode runtime contract (PRD-aligned)

1. `ExecutionProfile` selection is part of turn-start plan freeze and must be replay-visible through deterministic turn evidence.
2. In MVP Simple mode, runtime resolves default budgets, buffering/watermarks, flow-control posture, and fallback/degrade posture when explicit knobs are absent.
3. Runtime deterministic behavior for a turn is evaluated against resolved profile defaults plus explicit plan values, not implicit runtime-only defaults.

## Replay-mode and decision-handling distinctions (PRD-aligned)

1. Replay modes supported by contract:
   - re-simulation mode: runtime replays timeline inputs and re-executes deterministic runtime/node logic.
   - recorded-provider-playback mode: runtime reuses recorded provider outputs; provider determinism is not assumed.
2. Decision-handling modes:
   - `replay-decisions`: replay consumes recorded runtime decisions (for example admission, degrade/fallback, provider retry/switch) from timeline evidence.
   - `recompute-decisions`: replay recomputes decisions only when required deterministic inputs are present and schema-valid for that decision scope.
3. Safety rule:
   - if deterministic inputs for recomputation are incomplete/invalid, replay must not claim recompute equivalence and must use recorded decisions (or fail comparison validity at tooling layer).
4. Replay mode and decision mode used for evaluation must be explicitly surfaced in replay artifacts so result interpretation remains deterministic.

## Orchestration-level streaming handoff (implemented baseline, rollout in progress)

Status:
- runtime overlap executor is implemented in `internal/runtime/executor/streaming_handoff.go` with unit coverage in `internal/runtime/executor/streaming_handoff_test.go`
- live-provider chain smoke uses this path in `test/integration/provider_live_smoke_test.go` (`TestLiveProviderSmokeChainedWorkflow`)
- runtime policy is currently environment-driven via `StreamingHandoffPolicyFromEnv` (`RSPP_ORCH_STREAM_HANDOFF_*`) with deterministic sequential fallback when disabled
- `ResolvedTurnPlan.streaming_handoff` contract types exist in `api/controlplane/types.go`; planresolver/runtime wiring to consume those per-turn values is still pending
- current trigger implementation uses `min_partial_chars` and punctuation boundaries; `max_partial_age_ms`, per-edge `min_tts_chars`, and explicit max-wait timers are not yet implemented in runtime executor

Handoff contract:
1. STT emits `start` and ordered `delta` chunks with monotonic sequence numbers.
2. Runtime opens an `STT->LLM` handoff window when partial trigger policy is met (`min_partial_chars` or punctuation boundary).
3. LLM invocation starts on eligible STT partial content and is tagged with `handoff_id` + `upstream_revision`.
4. STT `final` seals a revision. If final text invalidates prior prefix, runtime deterministically supersedes downstream work and restarts from the latest stable prefix.
5. LLM emits partial tokens; runtime opens an `LLM->TTS` handoff window on segment boundaries (`min_partial_chars` or punctuation boundary).
6. TTS starts synthesis before LLM final and streams audio chunks while preserving output fence ordering.

Deterministic consistency rules:
1. Every downstream chunk is linked to exactly one upstream revision.
2. Supersede/restart decisions are deterministic for identical input, policy snapshot, and sequence history.
3. Output already fenced to transport is immutable; corrections are emitted as new segments, never in-place rewrites.
4. Cancel and authority-loss signals fence all active handoff windows before terminalization.

Slow-stage and backpressure policy:

| Condition | Runtime action | Expected outcome |
| --- | --- | --- |
| STT slower than downstream demand | keep downstream idle until partial trigger or STT completion; emit final-only handoff on STT completion (`final_fallback`) | continue unless upstream failure or turn budget/policy exhaustion forces terminalization |
| LLM slower than STT partial ingress | bound STT-partial queue, coalesce to latest revision, emit `flow_xoff` at high watermark | retry/switch/degrade before terminal abort |
| TTS slower than LLM partial egress | bound token-to-audio queue, throttle `LLM->TTS` forwarding | degrade to text-only or deterministic abort on no legal path |
| Stage hard-failure | apply modality retry/provider-switch/fallback policy | accepted turn still emits one terminal outcome then `close` |

## Latency improvement backlog (planned execution order)

These items are planned and prioritized for implementation; this section is documentation-only planning guidance.

1. Handoff duplicate-start suppression:
   - Goal: prevent unnecessary `final_fallback` plus `supersede` cascades that cause duplicate downstream invocations.
   - Runtime scope: `STT->LLM` and `LLM->TTS` trigger state machines.
   - Expected impact: lower `turn_completion_e2e` and lower excess provider usage.
2. Text-only partial trigger gating:
   - Goal: partial forwarding decisions should be driven only by text-bearing chunks (`delta`/`final`), not metadata-only chunks.
   - Runtime scope: chunk eligibility logic in streaming handoff hooks.
   - Expected impact: fewer false handoff openings and more stable handoff latency.
3. STT streaming transport latency tuning:
   - Goal: reduce provider-side partial wait intervals while preserving deterministic behavior.
   - Status: bounded AssemblyAI poll cadence env knob is implemented (`RSPP_STT_ASSEMBLYAI_POLL_INTERVAL_MS`, clamped to safe limits).
   - Runtime scope: STT adapter-level cadence knobs surfaced through provider config.
   - Expected impact: improved `stt_first_partial_latency_ms` and downstream start latency.
4. Native STT streaming transport rollout for all STT adapters:
   - Goal: eliminate pseudo-streaming built from unary responses and use provider-native incremental streams.
   - Runtime scope: `providers/stt/assemblyai`, `providers/stt/deepgram`, `providers/stt/google`.
   - Expected impact: strongest improvement in first-partial and first-audio latency.
5. Fair streaming vs non-streaming semantic parity:
   - Goal: enforce equivalent success semantics when comparing modes (avoid early-success bias in non-streaming paths).
   - Status: implemented baseline:
     - explicit per-invocation non-streaming control (`DisableProviderStreaming`) through scheduler -> RK-11
     - live chain mode-proof checks (streaming overlap evidence + non-streaming `streaming_used=false`)
     - compare artifact generation (`live-latency-compare-report`)
   - Runtime scope: provider outcome completion criteria and live compare workflow.
   - Expected impact: trustworthy A/B latency conclusions and valid gate decisions.

## Failure-class map (`F1`-`F8`)

| Class | Runtime ownership path | Expected control/terminal behavior |
| --- | --- | --- |
| `F1` admission overload | `localadmission`, `executor` | deterministic pre-turn `reject`/`defer` or scheduling-point `shed`; accepted turns keep single terminal sequence |
| `F2` node timeout/failure | `nodehost`, `budget` | budget/degrade/fallback decisions; no-legal-path ends in deterministic `abort(...)->close` |
| `F3` provider failure/overload | `provider/invocation` | normalized provider errors plus retry/switch/fallback; terminal abort on exhausted policy |
| `F4` edge pressure overflow | `buffering`, `flowcontrol` | watermark/flow signals and drop lineage; terminal only when no legal continue path |
| `F5` sync-coupled partial loss | `buffering`, `sync` | deterministic discontinuity or atomic drop lineage; recover or deterministic terminal abort |
| `F6` transport disconnect/stall | `transport`, `turnarbiter` | connection lifecycle signals and cleanup; abort/close when active-turn teardown required |
| `F7` authority conflict | `guard`, `turnarbiter` | stale epoch diagnostics or `deauthorized_drain`; in-turn authority loss aborts then closes |
| `F8` migration/failover | `guard`, `turnarbiter`, `state` | old writer deauthorizes; authoritative continuation on new placement with epoch-safe handling |

## Conformance areas supported by this folder

- Contract correctness: `CT-*`
- Cancellation fencing: `CF-*`
- Authority epoch safety: `AE-*`
- Merge/drop lineage: `ML-*`
- Replay determinism support markers: `RD-*`

Primary evidence lives in `test/integration`, `test/failover`, and `test/replay` plus runtime package tests.

## Assertions derived from lifecycle contract

1. No accepted turn emits both `commit` and `abort`.
2. Every accepted turn emits `close` after terminal outcome.
3. Pre-turn failure paths never emit accepted-turn terminalization.
4. `turn_open` cannot be emitted before valid authority and plan freeze.
5. Same-point cancel + hard authority revoke resolves to authority-loss path.

## PRD MVP quality-gate targets (runtime semantics)

Runtime kernel behavior is evaluated against these PRD targets (measurement emitted through OR-01/OR-02):
1. Turn-open decision latency: `p95 <= 120 ms` from `turn_open_proposed` acceptance point to `turn_open` emission.
2. First assistant output latency: `p95 <= 1500 ms` from `turn_open` to first DataLane output chunk on happy path.
3. Cancellation fence latency: `p95 <= 150 ms` from cancel acceptance to egress output fencing.
4. Authority safety: `0` accepted stale-epoch outputs in failover/lease-rotation tests.
5. Replay baseline completeness (shared runtime + observability gate): `100%` of accepted turns include required OR-02 baseline evidence.
6. Terminal lifecycle correctness: `100%` of accepted turns emit exactly one terminal (`commit` or `abort`) followed by `close`.
7. End-to-end latency target: `P50 <= 1.5s` and `P95 <= 2s`.

## Test and gate touchpoints

- `go test ./internal/runtime/...`
- `go test ./test/integration ./test/failover ./test/replay`
- `make verify-quick`
- `make verify-full`
- `make verify-mvp`

## Change checklist

1. Preserve lifecycle ordering and authority precedence invariants.
2. Keep control-signal and decision-outcome emissions schema-valid.
3. Keep replay-critical OR-02 evidence append behavior deterministic.
4. For pre-turn paths, assert no accidental accepted-turn terminal signals.
5. Re-run runtime + integration/failover/replay suites for behavior changes.

## Related repo-level docs

- `docs/repository_docs_index.md`
- `docs/rspp_SystemDesign.md`
- `docs/CIValidationGates.md`
- `docs/SecurityDataHandlingBaseline.md`
- `docs/ContractArtifacts.schema.json`
