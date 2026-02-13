# Command Entrypoints Guide

This folder contains operator and developer binaries for running the platform, producing gate artifacts, and local transport-path validation.

## Ownership

- `DX-01` local runner flow: `rspp-local-runner` (DevEx-Team + Transport-Team)
- `DX-04` release/gate CLI: `rspp-cli` (DevEx-Team)
- Runtime service command surface: `rspp-runtime` (Runtime-Team)
- Control-plane command surface (planned contract, unimplemented): `rspp-control-plane` (CP-Team)

## Entrypoints and responsibilities

| Entrypoint | Responsibility | Typical outputs |
| --- | --- | --- |
| `rspp-runtime` | Runtime process commands: provider bootstrap, LiveKit adapter command, retention sweep | provider bootstrap summary, LiveKit report JSON, retention sweep report JSON |
| `rspp-cli` | Contract validation, replay reports, SLO reports, release publish workflow | `.codex/ops/*`, `.codex/replay/*`, `.codex/release/*` |
| `rspp-local-runner` | `DX-01` single-node local runtime + minimal CP stub via shared LiveKit command path | `.codex/transports/livekit-local-runner-report.json` |
| `rspp-control-plane` | Planned session token/route contract for client integration (current scaffold is unimplemented) | planned: session token JSON, route resolution JSON |

## Command surfaces

### `rspp-runtime`

- Subcommands:
  - `bootstrap-providers`
  - `livekit`
  - `retention-sweep`
- `retention-sweep` behavior:
  - requires `-store` and `-tenants`
  - policy precedence: explicit artifact (`-policy`) -> CP distribution snapshot (`file`/`http`) -> default fallback
  - deterministic failure codes for policy read/decode/validation/tenant mismatch cases

### `rspp-cli`

- Contracts:
  - `validate-contracts`
  - `validate-contracts-report`
- Replay:
  - `replay-smoke-report`
  - `replay-regression-report`
- Runtime baseline and SLO:
  - `generate-runtime-baseline`
  - `slo-gates-report`
  - report contract coverage for PRD "Operate" expectations:
    - latency percentiles (p95/p99)
    - cancellation fence latency
    - drop counts/rates
    - provider error rates
- Release:
  - `publish-release <spec_ref> <rollout_cfg_path> ...`
  - fails closed if required gate artifacts are missing/stale/failing

### `rspp-local-runner` (`DX-01`)

- Delegates to `transports/livekit.RunCLI`.
- Default mode is deterministic dry-run for local validation.
- Operator path supports probe-enabled mode with explicit LiveKit credentials.

### `rspp-control-plane` (planned contract, unimplemented)

- Planned subcommands:
  - `issue-session-token`: emits control-plane-issued session token JSON for transport/session bootstrap.
  - `resolve-session-route`: emits route/placement JSON for transport endpoint selection.
- Planned output contract fields:
  - session identity (`session_id`, `tenant_id`)
  - pipeline and authority context (`pipeline_version`, `lease_epoch`, `expires_at`)
  - runtime route target (`runtime_endpoint`, `routing_view_snapshot`)

## CI and gate integration

- Blocking gate workflows rely on `rspp-cli` outputs:
  - `make verify-quick`
  - `make verify-full`
  - `make security-baseline-check`
  - `make codex-artifact-policy-check`
- Optional live checks:
  - `make live-provider-smoke`
  - `make livekit-smoke`
  - `make a2-runtime-live`

## LiveKit command-path requirements

- Runtime command path: `rspp-runtime livekit ...`
- Local runner path: `rspp-local-runner`
- Both must emit deterministic report artifacts and fail closed on invalid probe/runtime config.

## Change checklist

1. Keep command outputs deterministic and artifact-path stable unless intentionally versioned.
2. Update tests with command-surface changes:
   - `cmd/rspp-runtime/main_test.go`
   - `cmd/rspp-cli/main_test.go`
   - `cmd/rspp-local-runner/main_test.go`
3. Re-run:
   - `go test ./cmd/...`
   - `make verify-quick`

## Related docs

- `docs/repository_docs_index.md`
- `docs/CIValidationGates.md`
- `docs/SecurityDataHandlingBaseline.md`
- `docs/rspp_SystemDesign.md`
