# Build Executable MVP Backlog From Canonical Guide Map

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This repository contains `.codex/PLANS.md`; this document must be maintained in accordance with `.codex/PLANS.md`.

## Purpose / Big Picture

The goal is to produce an executable MVP backlog for the full RSPP repository that a contributor can execute without guessing intent. After this work, a contributor should be able to open one backlog artifact, see every MVP feature mapped to canonical ownership paths, run the listed validation commands, and execute tightly scoped PRs in parallel without semantic drift.

The work is complete when the backlog includes all `release_phase == "mvp"` features, links each item to canonical module paths and dependency edges, defines testable done criteria, and includes a current repo health snapshot with owner attribution.

## Progress

- [x] (2026-02-16 08:25Z) Read `prompt.md` and captured required deliverables (A-E), constraints, and Phase A/B/C workflow.
- [x] (2026-02-16 08:27Z) Read `.codex/PLANS.md` and extracted non-negotiable ExecPlan requirements.
- [x] (2026-02-16 08:31Z) Verified all canonical guide files listed in `prompt.md` exist in the repository.
- [x] (2026-02-16 08:32Z) Validated feature inventory shape and baseline counts (`214` total, `189` MVP, `25` post-MVP).
- [x] (2026-02-16 08:36Z) Created `.codex/ops/mvp-backlog/spec_map.json` from repository-level docs and feature inventory.
- [x] (2026-02-16 08:37Z) Created `.codex/ops/mvp-backlog/guide_recon.json` by extracting each canonical guide and validating against code paths.
- [x] (2026-02-16 08:38Z) Captured current repo health by running `make verify-quick`; gate returned exit `0` (no fallback needed).
- [x] (2026-02-16 08:42Z) Synthesized backlog items with feature IDs, path prefixes, dependencies, PR boundaries, done criteria, and risk flags in `docs/mvp_backlog.md`.
- [x] (2026-02-16 08:42Z) Defined parallel workstreams with strict allowed path prefixes and single-writer hotspots in `docs/mvp_backlog.md`.
- [x] (2026-02-16 08:42Z) Completed final acceptance checks (required sections present; MVP coverage `189/189`).
- [x] (2026-02-16 08:42Z) Created `docs/mvp_backlog.md` with final deliverables A-E (grouped backlog, workstreams, contract PR candidates, repo health, assumptions).

## Surprises & Discoveries

- Observation: `docs/RSPP_features_framework.json` is a top-level JSON list, not an object with a `features` key.
  Evidence: `python3` inventory probe returned `container_type list`.
- Observation: Priority metadata is sparse; only 4 entries include explicit `P0` in `priority`, so high-risk classification cannot rely on a single field.
  Evidence: inventory probe returned `priority_counts` with `P0: 4` and `<missing>: 210`.
- Observation: The canonical guide set from `docs/rspp_SystemDesign.md` is fully present, so no fallback guide discovery is needed.
  Evidence: existence check returned `OK` for all 15 canonical guide paths.
- Observation: MVP feature IDs include both `F-*` and `NF-*`; restricting coverage checks to `F-*` undercounts completion.
  Evidence: MVP inventory contains `189` IDs total, including `33` `NF-*` IDs.
- Observation: `make verify-quick` passed fully in the current branch snapshot.
  Evidence: `.codex/ops/mvp-backlog/repo_health.txt` ends with `VERIFY_QUICK_EXIT=0`.

## Decision Log

- Decision: Use `task.md` as the authoritative ExecPlan and keep implementation outputs in `docs/mvp_backlog.md` plus `.codex/ops/mvp-backlog/*`.
  Rationale: `task.md` holds the living execution specification; separate artifacts keep source data, extraction output, and final backlog auditable.
  Date/Author: 2026-02-16 / Codex.
- Decision: Treat high-risk MVP features as the union of explicit priority signals and invariants from `prompt.md`/system design (turn-plan freeze, lane priority, cancellation-first, authority/epoch, replay evidence, MVP simple profile).
  Rationale: feature inventory priority coverage is incomplete; invariant-driven risk labeling avoids false negatives.
  Date/Author: 2026-02-16 / Codex.
- Decision: Keep this task documentation-first with no sweeping refactors; permit only isolated, minimal changes if required to run health checks.
  Rationale: the user task is backlog synthesis and repo health reporting, not architectural code changes.
  Date/Author: 2026-02-16 / Codex.
- Decision: Represent backlog as one item per MVP feature group, then add guide-alignment items for canonical areas without direct primary feature-group assignment.
  Rationale: this keeps feature coverage complete (`189/189`) while preserving full canonical-area coverage (`15/15`) and PR-scoped executability.
  Date/Author: 2026-02-16 / Codex.
- Decision: Use deterministic primary-area assignment with explicit mapping assumptions when a feature group maps to multiple target modules.
  Rationale: avoids ambiguous ownership while still documenting multi-area touchpoints.
  Date/Author: 2026-02-16 / Codex.

## Outcomes & Retrospective

Milestones completed end-to-end for this ExecPlan.

Achieved outcomes:
- Built the Spec Map artifact at `.codex/ops/mvp-backlog/spec_map.json` with repository guarantees/non-goals, governance rules, canonical guide map, MVP counts, group mapping, and high-risk invariant-linked IDs.
- Built canonical-guide recon artifact at `.codex/ops/mvp-backlog/guide_recon.json` across all 15 guide files with status extraction, code evidence, contract surfaces, and recommended gates/tests.
- Captured repo health in `.codex/ops/mvp-backlog/repo_health.txt` via `make verify-quick` (exit `0`).
- Delivered `docs/mvp_backlog.md` containing required deliverables A-E:
  - A) MVP backlog grouped by canonical area with item-level metadata.
  - B) Parallel workstreams (6) with strict allowed path prefixes.
  - C) Contract PR candidates (`CPR-1..CPR-4`).
  - D) Current repo health snapshot with owner mapping logic.
  - E) Assumptions and open questions.

Validation summary:
- Required backlog sections present.
- MVP ID coverage reached `189/189`.
- Canonical area representation reached `15/15`.

## Context and Orientation

RSPP (Realtime Speech Pipeline Platform) is organized around canonical module guides and repository-level architecture documents. For this task, a “canonical guide map” means the mapping in `docs/rspp_SystemDesign.md` section `6. Canonical Implementation Guides (Merged)` that ties each architecture area to one guide file. A “Spec Map” means a structured artifact combining repository guarantees/non-goals, governance rules, canonical area mappings, and MVP feature-group mappings to code path prefixes. A “backlog item” means one executable unit of work linked to feature IDs or work package IDs, module boundaries, dependencies, test commands, and risk labels.

The authoritative inputs are:
`docs/rspp_SystemDesign.md`, `docs/RSPP_features_framework.json`, and canonical guides under `cmd/`, `internal/`, `providers/`, `transports/`, `test/`, `pipelines/`, `deploy/`, `pkg/`, and `api/`.

The required output is not just text summary. It must be an actionable MVP backlog with explicit PR boundaries, dependency edges, and verification instructions, plus a repo health snapshot that identifies failure owners.

## Plan of Work

Milestone 1 produces the Spec Map without code changes. Parse repository-level guarantees, non-goals, governance constraints, canonical guide mapping, and MVP feature distribution. Build a deterministic group-to-area mapping by anchoring each feature group to the closest canonical module boundaries in `docs/rspp_SystemDesign.md`. Where mapping is ambiguous, record a deterministic assumption with rationale in the final backlog.

Milestone 2 performs canonical guide recon for every guide file. Extract each guide’s responsibilities, prohibited scope, MVP invariants, implementation status, work packages or divergence items, contract surfaces, and required tests/gates. Validate each extracted claim against code path existence and current package/test coverage in the corresponding directories.

Milestone 3 synthesizes the executable backlog. Convert feature acceptance steps and guide work packages into itemized backlog entries grouped by canonical area. Every item must include feature IDs, path prefixes, dependency edges, PR boundary recommendation, done criteria, tests to add/run, and risk flags. Build at least three parallel workstreams with strict allowed path prefixes and explicit single-writer hotspots (`api/*`, `docs/RSPP_features_framework.json`, `docs/rspp_SystemDesign.md`).

Milestone 4 captures repo health and closes acceptance. Run gate commands to capture current failures and map each failure to owning module area. Write final deliverables A-E into `docs/mvp_backlog.md`, include assumptions/open questions, and record concise evidence snippets showing coverage and command outcomes.

## Concrete Steps

All commands below run from `/Users/tiger/Documents/projects/RealtimeSpeechPipeline`.

1. Create artifact directories and initialize output files.

    mkdir -p .codex/ops/mvp-backlog
    : > .codex/ops/mvp-backlog/spec_map.json
    : > .codex/ops/mvp-backlog/guide_recon.json
    : > .codex/ops/mvp-backlog/repo_health.txt

2. Build the Spec Map from authoritative docs and feature inventory.

    python3 - <<'PY'
    import json, pathlib
    from collections import Counter, defaultdict
    root = pathlib.Path('.')
    features = json.loads((root / 'docs/RSPP_features_framework.json').read_text())
    mvp = [f for f in features if f.get('release_phase') == 'mvp']
    by_group = Counter(f.get('group', '<missing>') for f in mvp)
    out = {
        "source_docs": [
            "docs/rspp_SystemDesign.md",
            "docs/RSPP_features_framework.json"
        ],
        "feature_counts": {
            "total": len(features),
            "mvp": len(mvp),
            "post_mvp": len(features) - len(mvp),
        },
        "mvp_group_counts": dict(sorted(by_group.items())),
        "invariant_focus": [
            "Turn Plan Freeze",
            "Lane Priority",
            "Cancellation-first",
            "Authority/Epoch",
            "Replay Evidence OR-02",
            "MVP simple/v1-only profile",
            "Runtime stateless compute with external durable state"
        ],
    }
    (root / '.codex/ops/mvp-backlog/spec_map.json').write_text(json.dumps(out, indent=2) + '\n')
    print("wrote .codex/ops/mvp-backlog/spec_map.json")
    PY

3. Extract canonical guide recon fields for each guide and store structured JSON.

    python3 - <<'PY'
    import json, pathlib, re
    guides = [
      "cmd/command_entrypoints_guide.md",
      "internal/controlplane/controlplane_module_guide.md",
      "internal/runtime/runtime_kernel_guide.md",
      "internal/observability/observability_module_guide.md",
      "internal/tooling/tooling_and_gates_guide.md",
      "internal/shared/internal_shared_guide.md",
      "providers/provider_adapter_guide.md",
      "transports/transport_adapter_guide.md",
      "test/test_suite_guide.md",
      "pipelines/pipeline_scaffolds_guide.md",
      "deploy/deployment_scaffolds_guide.md",
      "pkg/public_package_scaffold_guide.md",
      "api/controlplane/controlplane_api_guide.md",
      "api/eventabi/eventabi_api_guide.md",
      "api/observability/observability_api_guide.md",
    ]
    data = []
    for rel in guides:
        text = pathlib.Path(rel).read_text()
        headings = [ln.strip() for ln in text.splitlines() if ln.startswith('#')]
        status_hits = sorted(set(re.findall(r'\\b(implemented|partial|scaffold|divergence|backlog)\\b', text, flags=re.I)))
        data.append({
          "guide_path": rel,
          "headings": headings,
          "status_keywords": status_hits,
          "code_prefix_hint": rel.split('/')[0] + '/',
        })
    pathlib.Path('.codex/ops/mvp-backlog/guide_recon.json').write_text(json.dumps(data, indent=2) + '\\n')
    print("wrote .codex/ops/mvp-backlog/guide_recon.json")
    PY

4. Run repository health checks and capture failures for owner mapping.

    make verify-quick > .codex/ops/mvp-backlog/repo_health.txt 2>&1 || true
    if ! grep -q "PASS" .codex/ops/mvp-backlog/repo_health.txt; then go test ./... >> .codex/ops/mvp-backlog/repo_health.txt 2>&1 || true; fi

5. Author final backlog deliverable with required sections A-E.

    Edit docs/mvp_backlog.md and include:
    - A) MVP backlog grouped by canonical area with item metadata.
    - B) Parallel workstreams (>=3) with strict allowed path prefixes.
    - C) Contract PR candidates for `api/*` dependencies.
    - D) Repo health snapshot with failure list and owner mapping.
    - E) Assumptions and open questions that do not block execution.

6. Validate final coverage and formatting before completion.

    python3 - <<'PY'
    import json, pathlib
    spec = json.loads(pathlib.Path('.codex/ops/mvp-backlog/spec_map.json').read_text())
    text = pathlib.Path('docs/mvp_backlog.md').read_text()
    required = ['MVP Backlog', 'Parallel Workstreams', 'Contract PR Candidates', 'Current Repo Health Snapshot', 'Assumptions', 'Open Questions']
    missing = [k for k in required if k not in text]
    print('mvp_count', spec['feature_counts']['mvp'])
    print('missing_sections', missing)
    raise SystemExit(1 if missing else 0)
    PY

Expected command evidence to record in this plan and `docs/mvp_backlog.md`:

    feature_count: 214
    mvp: 189
    post_mvp: 25
    canonical_guides_found: 15/15
    verify-quick: <pass|fail with failing package list>

## Validation and Acceptance

Acceptance is behavior-based and must be demonstrated with artifacts and command output. A reader is successful when they can run the extraction commands, produce `spec_map.json` and `guide_recon.json`, and open `docs/mvp_backlog.md` to find complete deliverables A-E.

`docs/mvp_backlog.md` must show every MVP backlog item grouped by canonical area, with each item explicitly containing feature IDs or work package IDs, target path prefixes, dependency edges, PR boundaries, done criteria, test commands, and risk flags. The document must also include at least three non-overlapping workstreams with strict allowed prefixes and explicit single-writer hotspots.

`repo_health.txt` must include the latest gate run evidence. Any failures must be mapped to owners in the backlog document using the canonical ownership model from `docs/rspp_SystemDesign.md` section `7. Documentation and Ownership Operating Rules`.

## Idempotence and Recovery

This process is designed to be rerun safely. The JSON/text artifacts in `.codex/ops/mvp-backlog/` are regenerated in place, so rerunning updates state rather than duplicating it. If `make verify-quick` fails early, fallback to `go test ./...` and record partial coverage explicitly as a known gap.

If extraction scripts fail due to schema drift, update the parser logic in-place, rerun from Step 2, and append the schema change in `Decision Log` and `Surprises & Discoveries`. Do not modify runtime/control-plane code to complete this task unless a minimal fix is strictly required for executing health checks; if such a fix is required, isolate it into a separate commit and record why.

## Artifacts and Notes

Current baseline evidence captured during planning:

    docs/RSPP_features_framework.json container_type: list
    total features: 214
    MVP features: 189
    post-MVP features: 25
    canonical guide files present: 15/15

Artifacts to maintain during implementation:
`docs/mvp_backlog.md`, `.codex/ops/mvp-backlog/spec_map.json`, `.codex/ops/mvp-backlog/guide_recon.json`, `.codex/ops/mvp-backlog/repo_health.txt`.

## Interfaces and Dependencies

This task depends on the consistency of three authoritative interfaces:
the repository architecture interface in `docs/rspp_SystemDesign.md`, the feature inventory interface in `docs/RSPP_features_framework.json`, and the module contract interfaces in each canonical guide. “Interface” here means the stable shape of definitions and obligations that backlog items must trace to.

The structured artifacts must expose stable keys so downstream automation can consume them. `spec_map.json` must provide `authoritative_sources`, `repo_level`, `governance`, `canonical_guide_map`, `feature_inventory`, and `feature_group_to_area_mapping`. `guide_recon.json` must provide one record per canonical guide with at least `guide_path`, `headings`, `status_keywords`, and `current_status` evidence. `docs/mvp_backlog.md` must retain fixed section names for deliverables A-E so reviewers and agents can verify completeness deterministically.

Revision Note (2026-02-16 08:35Z): Initial ExecPlan created to implement `prompt.md` backlog task with PLANS-compliant living sections, concrete command flow, and acceptance criteria.
Revision Note (2026-02-16 08:42Z): Updated plan to completed state after executing all milestones; recorded produced artifacts, verification evidence (`verify-quick` exit `0`), full MVP coverage (`189/189`), and canonical-area coverage (`15/15`) so the next contributor can resume from current reality.
