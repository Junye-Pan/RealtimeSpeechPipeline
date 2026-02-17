# RK-01 Session Lifecycle Module Guide

Module: `RK-01` Session Lifecycle Manager.

Paths:
- `internal/runtime/session/manager.go`
- `internal/runtime/session/orchestrator.go`
- `internal/runtime/prelude/engine.go` (turn intent proposal seam)
- `internal/runtime/turnarbiter/arbiter.go` (deterministic policy/lifecycle outcomes)
- `transports/livekit/adapter.go` (session-owned runtime entrypoint wiring)

Role:
- own session-scoped turn lifecycle state (`Idle -> Opening -> Active -> Terminal -> Closed`),
- run pre-turn and active-turn arbiter logic under session-owned state transitions,
- ensure pre-turn `reject|defer|stale_epoch_reject|deauthorized` outcomes return to `Idle` without accepted-turn terminal events,
- enforce deterministic accepted-turn terminal sequencing (`commit|abort` followed by `close`).

Current status (2026-02-16):
- implemented baseline.

Implemented behavior:
- `Manager` provides deterministic turn-state transitions with single-active-turn enforcement.
- `Orchestrator` wraps `turnarbiter` and owns lifecycle transitions for turn-open and terminal-close sequencing.
- LiveKit adapter turn-open and active handling routes through `session.Orchestrator` rather than calling `turnarbiter` directly.
- Unit/integration coverage validates pre-turn reject/defer behavior and accepted-turn abort/close flow.

Primary evidence:
- `internal/runtime/session/manager_test.go`
- `internal/runtime/session/orchestrator_test.go`
- `test/integration/runtime_session_lifecycle_test.go`

Known remaining scope (outside RK-01 baseline):
- durable/session-hot state externalization and failover reattach semantics (`RK-05`/`RK-09`),
- equivalent session-orchestrator wiring for non-LiveKit transport adapters as they are brought in-scope.
