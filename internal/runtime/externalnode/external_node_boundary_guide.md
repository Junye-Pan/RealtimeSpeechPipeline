# RK-09 External Node Boundary Guide

Module: `RK-09` External Node Boundary Gateway.

Paths:
- `internal/runtime/externalnode/runtime.go`
- `internal/runtime/externalnode/runtime_test.go`
- `test/failover/failure_full_test.go` (`TestF2ExternalNodeTimeoutAndCancelFull`)

Role:
- provide deterministic out-of-process node invocation boundary for runtime,
- enforce per-invocation timeout and resource envelope checks,
- propagate cancel requests to external adapters and inject boundary telemetry markers.

Current status (2026-02-16):
- implemented baseline.

Implemented behavior:
- `Runtime.Invoke` validates identity/authority/resource fields before dispatch.
- resource ceilings (`cpu_units`, `memory_mb`) are enforced before external execution and reject with deterministic `ErrResourceLimitExceeded`.
- external execution timeout is bounded by runtime config and returns deterministic `ErrExecutionTimeout`.
- responses are normalized with boundary telemetry fields (`boundary`, `node_id`, resource markers).
- `Runtime.Cancel` propagates explicit cancellation to the external adapter with the same timeout envelope.

Primary evidence:
- `internal/runtime/externalnode/runtime_test.go`
- `test/failover/failure_full_test.go`

Known remaining scope:
- deeper process sandboxing/isolation policy (OS/container-level hardening) remains outside MVP baseline and is tracked in security/extensibility streams.
