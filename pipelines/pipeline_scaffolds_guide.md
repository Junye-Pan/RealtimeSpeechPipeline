# Pipeline Artifact Scaffolds Guide

## Scope and Evidence

This guide documents the implemented MVP scaffold baseline for pipeline artifacts.

Primary evidence:
- `docs/PRD.md` sections `4.1`, `4.2`, `4.3`, and `5.1`.
- `docs/rspp_SystemDesign.md` sections `3`, `4`, and `6`.
- `docs/mvp_backlog.md` milestone `UB-S02`.
- Contract tests in `test/contract/scaffold_artifacts_test.go`.

## Implemented Scaffold Layout

```text
pipelines/
  specs/
  profiles/
  bundles/
  rollouts/
  policies/
  compat/
  extensions/
  replay/
```

The current repository includes concrete versioned MVP artifacts (not placeholder-only guides).

## Canonical MVP Artifacts

| Path | Purpose |
| --- | --- |
| `pipelines/specs/voice_assistant_spec_v1.json` | Canonical MVP pipeline graph specification (`pipeline-v1`). |
| `pipelines/profiles/simple_profile_v1.json` | Canonical MVP `simple` execution profile defaults. |
| `pipelines/bundles/voice_assistant_bundle_v1.json` | Immutable bundle pinning spec/profile/rollout/policy/compat/extensions/replay refs. |
| `pipelines/rollouts/voice_assistant_rollout_v1.json` | MVP rollout strategy and default pipeline version pin. |
| `pipelines/policies/voice_assistant_policy_v1.json` | Provider bindings + adaptive action policy baseline. |
| `pipelines/compat/voice_assistant_compat_v1.json` | Runtime/transport compatibility envelope for MVP artifacts. |
| `pipelines/compat/version_skew_policy_v1.json` | Version skew governance policy used by tooling gates. |
| `pipelines/compat/deprecation_policy_v1.json` | Deprecation lifecycle governance policy used by tooling gates. |
| `pipelines/compat/conformance_profile_v1.json` | Mandatory conformance category profile for governance checks. |
| `pipelines/extensions/voice_assistant_extensions_v1.json` | Extension/plugin reference map for the canonical pipeline nodes. |
| `pipelines/replay/voice_assistant_replay_manifest_v1.json` | Replay/cursor manifest for deterministic regression workflows. |

## Linkage and Invariants

The scaffold baseline enforces these relationships:

1. `pipelines/bundles/voice_assistant_bundle_v1.json` references existing spec/profile/rollout/policy/compat/extensions/replay artifacts.
2. `pipeline_version` is pinned consistently across spec, bundle, rollout, compatibility, and replay manifests.
3. The simple profile is explicitly `execution_profile="simple"`.
4. Compatibility matrix includes explicit LiveKit support posture for MVP.
5. Governance policies under `pipelines/compat/` are consumed by `rspp-cli conformance-governance-report`.

These invariants are validated by `test/contract/scaffold_artifacts_test.go` and the verify gates.

## Validation Commands

Run from repository root:

```bash
go test ./test/contract -run 'TestPipelineAndDeployScaffoldArtifactsExistAndLink|TestScaffoldArtifactsHaveExpectedSchemaAndCrossLinks'
go run ./cmd/rspp-cli validate-spec pipelines/specs/voice_assistant_spec_v1.json strict
go run ./cmd/rspp-cli validate-policy pipelines/policies/voice_assistant_policy_v1.json strict
make verify-quick
```

## Post-MVP Expansion Surface

The following paths are intentionally deferred beyond MVP baseline and can be added without changing the canonical MVP artifact set:

- `pipelines/modules/`
- `pipelines/examples/`
- `pipelines/tests/`
- Additional execution profiles beyond `simple_profile_v1.json`
