# Deterministic Turn Lifecycle Truth Table

## 1. Purpose

Define the authoritative, deterministic transition contract for:

`turn_open_proposed -> turn_open -> commit/abort -> close`

Including:
- pre-turn `reject` / `defer`
- authority outcomes `stale_epoch_reject` / `deauthorized_drain`
- replay-critical evidence expectations

This table is normative for MVP behavior and test assertions.

## 2. Canonical states

- `Idle`: no active authoritative turn.
- `Opening`: pre-turn arbitration only (intent received, no authoritative turn started).
- `Active`: authoritative turn open, `ResolvedTurnPlan` frozen.
- `Terminal`: terminal outcome emitted (`commit` or `abort`).
- `Closed`: `close` emitted; turn lifecycle finalized.

## 3. Deterministic precedence rules

1. Pre-turn authority/admission failures always win over opening a turn.
2. `turn_open` is legal only after:
   - admission/snapshot checks pass
   - authority validation passes
   - `ResolvedTurnPlan` materialization succeeds
3. Exactly one terminal outcome is legal for an accepted turn.
4. For accepted turns, terminal emission order is:
   - `commit` or `abort(reason)`
   - then `close`
5. If cancellation is accepted before `commit`, `abort(cancelled)` path wins.
6. If authority is revoked during `Active`, emit `deauthorized_drain` and terminate with `abort(authority_loss)` then `close`.

## 4. Lifecycle truth table

| Case | Current state | Trigger/input | Guard condition | Required outputs (in order) | Required OR-02 evidence | Next state |
| --- | --- | --- | --- | --- | --- | --- |
| T1 | `Idle` | `turn_open_proposed` received | always | none (enter arbitration only) | intent marker (optional per recording level policy) | `Opening` |
| T2 | `Opening` | admission/snapshot evaluation | stale/missing/incompatible snapshot | `defer` or `reject` | admission/snapshot outcome + snapshot provenance refs used | `Idle` |
| T3 | `Opening` | authority validation | stale/invalid authority epoch | `stale_epoch_reject` | authority outcome + epoch marker | `Idle` |
| T4 | `Opening` | authority validation | authority revoked/de-authorized before open | `deauthorized_drain` | authority outcome + epoch marker | `Idle` |
| T5 | `Opening` | plan resolution | admission valid + authority valid + plan materialized | `turn_open` | `ResolvedTurnPlan` hash + determinism markers + start provenance refs | `Active` |
| T6 | `Active` | normal generation complete | terminal evidence append succeeded | `commit`, `close` | terminal markers + ordering markers | `Closed` |
| T7 | `Active` | cancel accepted | before terminal emitted | `abort(reason=cancelled)`, `close` | cancel markers (`cancel_sent_at`, optional `cancel_ack_at`) + terminal markers | `Closed` |
| T8 | `Active` | authority revoked in-turn | hard authority loss | `deauthorized_drain`, `abort(reason=authority_loss)`, `close` | authority outcome + terminal markers + epoch marker | `Closed` |
| T9 | `Active` | OR-02 baseline append failure | baseline evidence cannot be preserved | `abort(reason=recording_evidence_unavailable)`, `close` | attempted append failure marker + terminal markers | `Closed` |
| T10 | `Active` | budget/provider/runtime terminal failure | no legal continue/degrade/fallback path | `abort(reason=<deterministic_reason>)`, `close` | failure outcome class + terminal markers | `Closed` |
| T11 | `Closed` | late DataLane/control event for same turn | always | deterministic late handling (drop/diagnostic only, no reopen) | late-event diagnostics (if enabled) | `Closed` |
| T12 | any | stale output/event from non-authoritative placement | epoch mismatch on ingress/egress | `stale_epoch_reject` diagnostic; no state mutation for authoritative turn | authority divergence evidence | unchanged |

## 5. State-transition notes for pre-turn outcomes

1. `reject`/`defer` are pre-turn outcomes:
   - they do not create authoritative turn state
   - they MUST NOT emit `abort` or `close`

2. `stale_epoch_reject` or `deauthorized_drain` before `turn_open`:
   - remain pre-turn outcomes
   - MUST NOT emit `abort` or `close`

3. `deauthorized_drain` during `Active`:
   - must transition accepted turn to terminal via `abort(authority_loss)` and `close`

## 6. Deterministic pseudocode order

```text
on turn_open_proposed:
  state = Opening

  if snapshot_invalid_or_missing:
    emit defer_or_reject
    state = Idle
    return

  if authority_invalid_epoch:
    emit stale_epoch_reject
    state = Idle
    return

  if authority_deauthorized:
    emit deauthorized_drain
    state = Idle
    return

  plan = materialize_resolved_turn_plan()
  emit turn_open
  state = Active
```

```text
while state == Active:
  if authority_revoked:
    emit deauthorized_drain
    emit abort(authority_loss)
    emit close
    state = Closed
    break

  if cancel_accepted and terminal_not_emitted:
    emit abort(cancelled)
    emit close
    state = Closed
    break

  if terminal_success_ready and terminal_evidence_appended:
    emit commit
    emit close
    state = Closed
    break
```

## 7. Test assertions derived from this table

1. No path may emit both `commit` and `abort` for one turn.
2. No accepted turn may end without `close`.
3. Pre-turn failure paths never emit `abort` or `close`.
4. `turn_open` cannot occur without valid authority and successful `ResolvedTurnPlan` materialization.
5. `stale_epoch_reject` never mutates an already authoritative turn state unless accompanied by explicit in-turn authority-loss handling.
6. Mapping to `decision_outcome` phase/scope: stale diagnostics during active execution are represented as `scheduling_point` outcomes, while hard authority revoke uses `deauthorized_drain` at `active_turn` scope.
7. Any authority/admission `decision_outcome` emitted with `scope=turn` includes `turn_id` for deterministic turn correlation.
