# Failure Injection Matrix (F1-F8)

## 1. Purpose

Map documented failure classes (`F1`-`F8`) to deterministic runtime behavior:
- where to inject failure
- expected ControlLane outcomes
- required replay evidence in OR-02

Source alignment:
- `docs/ModularDesign.md` sections 8.1 and 10.2
- `docs/rspp_SystemDesign.md` sections 4.1.2, 4.2.2, and 4.3
- `docs/DeterministicLifecycleTruthTable.md` sections 3-5 and section 7 assertions
- `docs/ContractArtifacts.schema.json` (`control_signal`, `decision_outcome`)
- `docs/ConformanceTestPlan.md` section 3

## 2. Matrix

| Failure class | Injection method | Expected ControlLane outcomes | Expected terminal behavior | Required OR-02 replay evidence |
| --- | --- | --- | --- | --- |
| `F1` Admission overload | force quota exhaustion at `edge_enqueue`, `edge_dequeue`, `node_dispatch` | deterministic admission/capacity outcomes: `reject`/`defer` and scheduling-point `shed` when policy applies | pre-turn outcomes (`reject`/`defer`) MUST not emit `turn_open`, `abort`, or `close`; if overload occurs after acceptance, preserve single terminal sequence (`commit` or `abort`, then `close`) | admission policy snapshot refs, scheduling point (`edge_enqueue`/`edge_dequeue`/`node_dispatch`), fairness key, emitted outcome + phase |
| `F2` Node timeout/failure | simulate node handler timeout/panic/error in RK-08 | `budget_warning`/`budget_exhausted`; optional `degrade`/`fallback` | if no legal continue/degrade/fallback path, emit `abort(reason=node_timeout_or_failure)` then `close` | node id, timeout budget values, chosen degrade/fallback decision provenance, terminal markers |
| `F3` Provider failure/overload | inject provider timeout/rate-limit/provider disconnect from RK-11 | `provider_error` and/or `circuit_event`; optional `provider_switch`/`fallback` | if configured retry/switch/fallback resolution fails, emit `abort(reason=provider_failure)` then `close` | provider invocation id, normalized outcome class, retry/switch decision provenance, terminal markers |
| `F4` Edge pressure overflow | force queue depth/time beyond watermark in RK-12/13/14 | `watermark`, flow control (`flow_xoff`/`flow_xon`), `drop_notice`; optional `shed` when admission layer applies deterministic shedding | non-terminal unless plan forces terminal failure; terminal path, when taken, is `abort` then `close` | edge id, watermark crossings, queue depth/time evidence, buffer policy ref, dropped/merged lineage, resulting action |
| `F5` Sync-coupled partial loss | drop one stream in a sync domain under pressure | `discontinuity` (with `sync_domain`, `discontinuity_id`, `reason`) or deterministic atomic drop markers (`drop_notice`) | continue with deterministic sync recovery; if unrecoverable, emit `abort` then `close` | `sync_id`/`sync_domain`, discontinuity marker, affected sequence range, reason, `drop_notice` evidence (`edge_id`, `target_lane`, `seq_range`) when drop markers are used, recovery decision |
| `F6` Transport disconnect/stall | force transport disconnect/stall and silence/stall edge behavior | connection lifecycle signals (`disconnected`/`stall`, optionally `silence`, `reconnecting`, `ended`) | for accepted turns requiring teardown: `abort(reason=transport_disconnect_or_stall)` then `close` | transport state markers, cancellation fencing evidence, deterministic cleanup evidence, terminal markers |
| `F7` Placement authority conflict | inject stale epoch event/output or revoke active authority | `stale_epoch_reject` for pre-turn/scheduling-point stale authority; `deauthorized_drain` for pre-turn deauthorization before open or in-turn authority revoke | `stale_epoch_reject` does not force terminalization by itself; pre-turn authority outcomes MUST NOT emit `abort`/`close`; in-turn authority loss emits `abort(reason=authority_loss)` then `close`; if cancel and revoke are simultaneous at the same arbitration point, authority-loss path wins | lease epoch markers, authority outcome signal, ingress/egress mismatch evidence, migration/handoff markers, terminal markers |
| `F8` Region failover | emulate authority handoff and runtime migration | `lease_rotated`, `migration_start`/`migration_finish` (and `session_handoff` when used), stale old-writer output rejection (`stale_epoch_reject`) | authoritative continuation on new placement; old placement must not emit authoritative output; active-turn revoke path on old placement is `deauthorized_drain`, `abort(reason=authority_loss)`, `close` | old/new epoch markers, routing snapshot refs, durable reattach marker, migration markers, divergence-safe continuity evidence |

## 3. Injection sequencing rules

1. Inject one primary failure per run; keep all other dimensions stable.
2. For authority and migration tests, always include both ingress and egress epoch mismatch checks.
3. For pressure tests, include queue-depth and time-in-queue variants.
4. For cancel-sensitive tests, verify post-cancel output fencing before terminal outcome assertions.
5. Use only schema-defined ControlLane and decision outcome vocabulary (`docs/ContractArtifacts.schema.json`) and valid emitter ownership.

## 4. Required assertions per run

1. ControlLane outcomes are deterministic for identical seeds/inputs/snapshots.
2. Terminal sequence rule holds (`commit` or `abort`, then `close`) for accepted turns.
3. Pre-turn outcomes (`reject`, `defer`, `stale_epoch_reject`, `deauthorized_drain`) do not emit `abort`/`close`.
4. OR-02 includes plan hash, determinism markers, authority markers, and failure-specific evidence from matrix row.
5. Replay of the run does not produce unexplained `PLAN_DIVERGENCE`/`OUTCOME_DIVERGENCE`; `AUTHORITY_DIVERGENCE` is always zero.
6. `ORDERING_DIVERGENCE` is zero unless an explicitly approved test scenario declares it in fixture metadata.
7. `TIMING_DIVERGENCE` is allowed only within configured deterministic tolerance.
8. Authority outcome phase mapping remains schema-valid:
   - `stale_epoch_reject`: `pre_turn` or `scheduling_point`
   - `deauthorized_drain`: `pre_turn` (before open) or `active_turn` (accepted turn revoke)
9. Authority outcome scope mapping remains schema-valid:
   - `stale_epoch_reject` with `pre_turn` uses `scope=session|turn`; with `scheduling_point` uses `scope=edge_enqueue|edge_dequeue|node_dispatch`
   - `deauthorized_drain` with `active_turn` uses `scope=turn` and includes `turn_id`
   - `deauthorized_drain` with `pre_turn` uses `scope=session|turn`
10. Any `decision_outcome` with `scope=turn` includes `turn_id` for unambiguous turn correlation.
11. `shed` outcomes are scheduling-point decisions (`phase=scheduling_point`) emitted by RK-25.
12. For same-point cancel+authority-revoke tie cases, terminal reason is deterministically `authority_loss` (never `cancelled`).

## 5. Minimum execution set for CI

- Smoke set (quick): `F1`, `F3`, `F7`
- Full set (nightly/release): `F1`-`F8`
