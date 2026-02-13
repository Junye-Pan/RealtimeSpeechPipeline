# Repository Guidelines

## Project Structure & Module Organization
- `cmd/` contains executable entrypoints (`rspp-runtime`, `rspp-control-plane`, `rspp-cli`, `rspp-local-runner`).
- `api/` defines shared contracts (`controlplane`, `eventabi`, `observability`).
- `internal/` holds core implementation by domain (`controlplane`, `runtime`, `observability`, `security`, `tooling`).
- `providers/` contains STT/LLM/TTS adapters; `transports/livekit/` contains transport integration.
- `test/` contains cross-package suites: `contract/`, `integration/`, `replay/`, `failover/`.
- `docs/` contains system design, CI gate definitions, and security/data-handling baselines.

## Build, Test, and Development Commands
- `make test`: run the full Go test suite (`go test ./...`).
- `make verify-quick`: run the fast validation gate (contracts, replay smoke, runtime baseline, SLO checks, targeted tests).
- `make verify-full`: run full regression gates (replay regression + all tests).
- `make security-baseline-check`: run focused security/data-handling tests.
- `make live-provider-smoke`: run optional live-provider smoke tests (`-tags=liveproviders`).
- `make a2-runtime-live`: run extended live runtime matrix checks.

## Coding Style & Naming Conventions
- Language baseline is Go `1.25` (`go.mod`).
- Format code with `gofmt` (or `go fmt ./...`) before committing.
- Follow idiomatic Go naming: lowercase package names, `PascalCase` exports, `camelCase` internals.
- Use descriptive snake_case file names; test files end with `_test.go`.
- Keep changes scoped to domain boundaries (e.g., runtime logic in `internal/runtime/*`, contracts in `api/*`).

## Testing Guidelines
- Use Go’s standard `testing` package and table-driven tests where practical.
- Keep unit tests next to implementation files; place scenario/conformance suites under `test/integration` and `test/replay`.
- Name tests as `Test<Behavior>`; conformance tests may include IDs (for example, `TestAE001...`, `TestCF001...`).
- Before opening a PR, run at least `make verify-quick`; run `make verify-full` for broad changes.

## Commit & Pull Request Guidelines
- Follow the repository’s commit style: `<type>: <summary>` (e.g., `feat: ...`, `test: ...`, `security: ...`, `chore: ...`).
- Do not commit directly to `main` or push changes straight to `main`; create a feature branch (for example, `feat/<topic>`), push that branch, and merge through a pull request.
- Keep commits focused and explain behavior changes in commit/PR text.
- PRs should include: purpose, key files touched, and commands run to validate.
- CI-required checks are `codex-artifact-policy`, `verify-quick`, `verify-full`, and `security-baseline`; live-provider jobs are optional and can be triggered with the `run-live-provider-smoke` label.

## Security & Configuration Tips
- Never commit credentials; provide provider keys via `RSPP_*` environment variables for live tests.
- Do not track generated `.codex` artifacts. Only `.codex/skills/**` and `.codex/rules/**` are allowed as tracked `.codex` paths.

# ExecPlans
When writing complex features or significant refactors, use an ExecPlan (as described in .codex/PLANS.md) from design to implementation.