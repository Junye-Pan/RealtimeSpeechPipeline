# RK-15 Sync Integrity Engine Guide

Module: `RK-15` Sync Integrity Engine.

Paths:
- `internal/runtime/sync/engine.go`
- `internal/runtime/sync/engine_test.go`
- `test/failover/failure_full_test.go` (`TestF5SyncEngineDropWithDiscontinuityFull`)

Role:
- execute deterministic sync-loss policies for synchronized streams,
- emit canonical control markers for atomic drop and discontinuity handling,
- preserve lane-priority safety by using control-lane metadata markers rather than blocking dispatch.

Current status (2026-02-16):
- implemented baseline.

Implemented behavior:
- `Engine.Evaluate` supports `atomic_drop` and `drop_with_discontinuity` policies.
- `atomic_drop` emits deterministic `drop_notice` metadata through buffering-compatible shape.
- `drop_with_discontinuity` emits ordered `drop_notice` + `discontinuity` markers with required sync-domain metadata.

Primary evidence:
- `internal/runtime/sync/engine_test.go`
- `test/failover/failure_full_test.go`

Known remaining scope:
- unified executor-owned sync orchestration loop across all data-plane edges remains future work; current MVP uses policy-engine outputs composed with existing buffering/scheduler paths.
