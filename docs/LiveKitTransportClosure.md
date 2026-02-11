# LiveKit Transport Closure (RK-22/RK-23 + DX-01)

## 1. Purpose

Define objective-level acceptance criteria to close the MVP LiveKit transport-path gap, and provide operator run instructions that are executable outside test harnesses.

This document is normative for transport-path closure evidence and is synchronized with:
- `docs/MVP_ImplementationSlice.md` section `10` and Appendix `A.2/A.4`
- `docs/ConformanceTestPlan.md`
- `docs/CIValidationGates.md`

## 2. Acceptance Criteria

All criteria below must be met for transport-path closure to move from `partial` to `met`:

1. Adapter API and behavior:
   - `transports/livekit` exposes a concrete adapter (`Config`, `Event`, `Adapter`, `Report`) and command runner (`RunCLI`).
   - RK-22 ingress mapping/classification is applied through `internal/runtime/transport.TagIngressEventRecord` and ABI normalization.
   - RK-22 egress fencing is applied through `internal/runtime/transport.OutputFence`.
   - RK-23 connection lifecycle normalization emits valid `connected/reconnecting/disconnected/ended/silence/stall` signals.

2. Runtime wiring:
   - `cmd/rspp-runtime` exposes a production/operator command surface `rspp-runtime livekit ...`.
   - Command supports deterministic report generation and optional RoomService probe for connectivity/auth validation.

3. Local runner wiring (DX-01):
   - `cmd/rspp-local-runner` is no longer scaffold-only and starts a single-node local LiveKit-backed runtime path by delegating to the same LiveKit command implementation.

4. Operator runbook:
   - Required/optional environment and startup commands are documented and executable.
   - Failure mode guidance exists for auth/connectivity probe errors.

5. Gate evidence:
   - Blocking deterministic tests cover adapter mapping + arbiter/control integration paths.
   - Non-blocking LiveKit smoke job exists in CI and captures artifacts/log evidence.

## 3. Adapter API (Implemented Surface)

Primary package:
- `transports/livekit`

Contracted surfaces:
1. `ConfigFromEnv() (Config, error)` and `Config.Validate()`.
2. `NewAdapter(cfg Config, deps Dependencies) (Adapter, error)`.
3. `Adapter.Process(ctx context.Context, events []Event) (Report, error)`.
4. `DefaultScenario(cfg Config) []Event`.
5. `RunCLI(args []string, stdout io.Writer, now func() time.Time) error`.
6. `ProbeRoomService(ctx context.Context, cfg Config, client roomServiceClient, now func() time.Time) (ProbeResult, error)`.

## 4. Runtime / EntryPoint Wiring

1. Runtime entrypoint:
```bash
go run ./cmd/rspp-runtime livekit -dry-run=true -probe=false
```

2. Optional RoomService probe path (auth + endpoint validation):
```bash
RSPP_LIVEKIT_URL=https://<livekit-host> \
RSPP_LIVEKIT_API_KEY=<key> \
RSPP_LIVEKIT_API_SECRET=<secret> \
go run ./cmd/rspp-runtime livekit -dry-run=false -probe=true -room rspp-demo
```

3. Local runner path (DX-01):
```bash
go run ./cmd/rspp-local-runner
```

## 5. Operator Runbook

Required for probe-enabled or non-dry-run execution:
- `RSPP_LIVEKIT_URL`
- `RSPP_LIVEKIT_API_KEY`
- `RSPP_LIVEKIT_API_SECRET`

Common optional settings:
- `RSPP_LIVEKIT_ROOM`
- `RSPP_LIVEKIT_SESSION_ID`
- `RSPP_LIVEKIT_TURN_ID`
- `RSPP_LIVEKIT_PIPELINE_VERSION`
- `RSPP_LIVEKIT_AUTHORITY_EPOCH`
- `RSPP_LIVEKIT_DEFAULT_DATA_CLASS`
- `RSPP_LIVEKIT_DRY_RUN`
- `RSPP_LIVEKIT_PROBE`
- `RSPP_LIVEKIT_REQUEST_TIMEOUT_MS`

Artifacts:
- Runtime command writes deterministic transport evidence by default:
  - `.codex/transports/livekit-report.json`
- Local runner default report:
  - `.codex/transports/livekit-local-runner-report.json`

Failure handling:
1. Probe failures return non-zero and include HTTP status/body summary in command error output.
2. Keep `-dry-run=true -probe=false` for deterministic local validation when remote LiveKit credentials/endpoints are unavailable.

## 6. Gate Evidence Map

Blocking deterministic evidence:
1. `transports/livekit/*_test.go`
2. `test/integration/livekit_transport_integration_test.go`
3. `cmd/rspp-runtime/main_test.go` (livekit subcommand coverage)
4. `cmd/rspp-local-runner/main_test.go`

Non-blocking real LiveKit evidence:
1. Make target:
   - `make livekit-smoke`
2. CI job:
   - `.github/workflows/verify.yml` job `livekit-smoke` (`continue-on-error: true`)
3. Artifacts:
   - `.codex/providers/livekit-smoke.log`
   - `.codex/providers/livekit-smoke-report.json`
