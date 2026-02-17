# RK-18 Timebase Service Guide

Module: `RK-18` Timebase Service.

Paths:
- `internal/runtime/timebase/service.go`
- `internal/runtime/timebase/service_test.go`
- `test/replay/rd002_rd003_rd004_test.go` (timebase determinism coupling check)

Role:
- provide deterministic mapping across monotonic runtime time, wall-clock time, and media time,
- enforce monotonic projection ordering within a session,
- support bounded skew observation with deterministic rebase semantics.

Current status (2026-02-16):
- implemented baseline.

Implemented behavior:
- `Service.Calibrate` sets per-session anchors (`monotonic_ms`, `wall_clock_ms`, `media_time_ms`) with monotonic anchor progression checks.
- `Service.Project` maps monotonic timestamps to wall/media timestamps deterministically and rejects non-monotonic regressions.
- `Service.Observe` compares observed wall/media values against projected values, computes skew, and deterministically re-calibrates when skew exceeds a caller-provided threshold.

Primary evidence:
- `internal/runtime/timebase/service_test.go`
- `test/replay/rd002_rd003_rd004_test.go` (`TestRD002TimebaseProjectionDeterminism`)

Known remaining scope:
- global/distributed clock reconciliation and cross-runtime drift arbitration remain out of MVP baseline scope.
