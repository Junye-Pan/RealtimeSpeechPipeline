# Transport Boundary Guide

This folder contains transport adapters that translate external transport events into runtime-safe contract signals. It owns transport-boundary behavior, not RTC media-stack internals.

## Ownership and scope

- Primary owner: `Transport-Team`
- Cross-review: `Runtime-Team` (event/authority semantics), `Security-Team` (transport auth/config handling)

Module alignment:
- `RK-22`: ingress classification and output fence integration
- `RK-23`: connection lifecycle normalization

## Implementations in this folder

- `transports/livekit` (implemented baseline)
- `transports/websocket` (scaffold)
- `transports/telephony` (scaffold)

## `transports/livekit` package map

| File | Responsibility |
| --- | --- |
| `config.go` | env/flag config resolution and validation |
| `adapter.go` | deterministic adapter processing and report generation |
| `command.go` | shared CLI runner (`RunCLI`) for runtime/local-runner paths |
| `probe.go` | optional RoomService connectivity/auth probe |

## Adapter API surface

Implemented LiveKit adapter entrypoints:
1. `ConfigFromEnv()` and `Config.Validate()`
2. `NewAdapter(...)`
3. `Adapter.Process(...)`
4. `DefaultScenario(...)`
5. `RunCLI(...)`
6. `ProbeRoomService(...)`

## LiveKit closure acceptance criteria

1. Deterministic ingress mapping and ABI normalization are preserved.
2. Runtime output fence behavior is preserved through adapter integration.
3. Lifecycle signal normalization remains schema-valid (`connected`, `reconnecting`, `disconnected`, `ended`, `silence`, `user_muted`, `stall`).
4. Runtime and local-runner command surfaces produce deterministic report artifacts.

## Transport boundary contract alignment

Authority and lease/epoch requirements (PRD alignment):
1. Ingress acceptance requires current authority context (valid lease/epoch); if transport metadata is missing, authority context must be deterministically enriched before acceptance.
2. Stale/missing/incompatible authority context must produce deterministic pre-turn rejection outcomes (for example `stale_epoch_reject`) and must not emit turn `abort`/`close` for pre-turn rejects.
3. After ingress acceptance, authority-relevant transport outputs and lifecycle/control signals must carry current lease/epoch context for split-brain safety.
4. If authority is revoked during an active turn, adapter behavior must cooperate with runtime `deauthorized_drain` and output fencing semantics.

Ingress mapping and connection-behavior requirements (PRD alignment):
1. Transport audio frames (with media timestamps) must map deterministically to runtime DataLane events with preserved ordering/continuity markers.
2. Connection normalization must cover liveness/keepalive interpretation and schema-valid lifecycle signals (`connected`, `reconnecting`, `disconnected`, `ended`, `silence`, `user_muted`, `stall`).
3. Disconnect handling must be explicit: on disconnect during an active turn, adapter behavior must cooperate with runtime turn abort rules (`abort(reason=disconnect)`), drain rules, and cleanup semantics.

Playback, cancellation, and output-delivery requirements (PRD alignment):
1. Transport adapters map playback/output lifecycle to ControlLane delivery signals (`output_accepted`, `playback_started`, `playback_completed`, `playback_cancelled`).
2. After `cancel(scope)` acceptance, transport egress queues for that scope must be fenced/cleared; emitting new `output_accepted` or `playback_started` signals for that scope is invalid.
3. Deterministic artifacts should preserve enough signal lineage to diagnose cancel fences, playback completion, and dropped/fenced output behavior.

Timebase and sequencing requirements (PRD alignment):
1. Transport timestamps must be mapped into runtime timebase semantics (monotonic runtime time, wall-clock correlation, media time).
2. Adapters must preserve deterministic sequence progression (`transport_sequence` ingress ordering and normalized runtime sequencing markers) needed for replay/debug correlation.
3. Jitter/discontinuity handling responsibilities at the transport boundary must be explicit and deterministic for replayability.

Routing and migration requirements (PRD alignment):
1. Adapters must consume routing/placement view updates to locate the current runtime endpoint and honor migration/handoff decisions quickly.
2. During lease rotation or migration, adapter routing behavior must prevent duplicate split-brain egress across concurrent placements/epochs.

## Operator runbook

Runtime command path:
- `go run ./cmd/rspp-runtime livekit -dry-run=true -probe=false`

Probe-enabled path:
- `go run ./cmd/rspp-runtime livekit -dry-run=false -probe=true -room <room>`

Local runner path:
- `go run ./cmd/rspp-local-runner`

Common env inputs:
- `RSPP_LIVEKIT_URL`
- `RSPP_LIVEKIT_API_KEY`
- `RSPP_LIVEKIT_API_SECRET`
- optional room/session/turn/pipeline/epoch fields

Deterministic artifacts:
- `.codex/transports/livekit-report.json`
- `.codex/transports/livekit-local-runner-report.json`

## Gate evidence map

Blocking deterministic evidence:
- `transports/livekit/*_test.go`
- `test/integration/livekit_transport_integration_test.go`
- `cmd/rspp-runtime/main_test.go`
- `cmd/rspp-local-runner/main_test.go`

Non-blocking live evidence:
- `make livekit-smoke`
- CI job `livekit-smoke`
- artifacts under `.codex/providers/`

PRD MVP standards tie-in:
1. Turn-open decision latency target: p95 <= 120 ms from `turn_open_proposed` acceptance to `turn_open`.
2. First assistant output latency target: p95 <= 1500 ms from `turn_open` to first DataLane output chunk.
3. Cancellation fence latency target: p95 <= 150 ms from cancel acceptance to egress output fencing.
4. Authority safety target: 0 accepted stale-epoch outputs in failover/lease-rotation coverage.
5. MVP mode is `Simple` only: transport integration behavior must remain compatible with Simple-profile runtime defaults.
6. OR-02 baseline replay evidence is mandatory for accepted turns: transport artifacts/signals must preserve replay-critical authority, lifecycle, and delivery markers required by the baseline contract.

## Conformance touchpoints

- `LK-001`: adapter mapping + integration assertions
- `LK-002`: runtime/local-runner command behavior and deterministic artifacts
- `LK-003`: optional live smoke path

## Change checklist

1. Keep signal names/emitter ownership schema-valid.
2. Preserve deterministic report structure and error behavior.
3. Update runtime/local-runner command tests for CLI changes.
4. Re-run LiveKit package and integration tests.

## Related repo-level docs

- `docs/repository_docs_index.md`
- `docs/rspp_SystemDesign.md`
- `docs/CIValidationGates.md`
- `docs/SecurityDataHandlingBaseline.md`
