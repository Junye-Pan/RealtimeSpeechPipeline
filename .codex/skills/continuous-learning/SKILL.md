---
name: continuous-learning
description: Extract instincts from session checkpoint logs, maintain confidence-scored behavior files, and optionally evolve strong instinct clusters into reusable Codex skills. Use after session-checkpoints or when asked to learn from recent work.
---

# continuous-learning

## Purpose
Turn Codex work sessions into reusable knowledge using **instincts**:
- atomic trigger → action rules
- evidence-backed
- confidence-weighted (0.3–0.9)

This Codex adaptation does **not** rely on tool-call hooks.  
Instead, it learns from **session checkpoint logs** created by `$session-checkpoint`.

## Required inputs
- A session log produced by `$session-checkpoint`
  - Prefer: `.codex/sessions/LATEST` points to the active session log
- Optional: git context (branch, diffs) for evidence

## Storage layout (repo-scoped by default)
- `.codex/homunculus/observations.jsonl`
- `.codex/homunculus/instincts/personal/*.md`
- `.codex/homunculus/instincts/inherited/*.md`
- `.codex/homunculus/evolved/skills/<skill-name>/SKILL.md`

If the user explicitly asks for user-scoped storage and permissions allow it, mirror this under `~/.codex/homunculus/`.
Create directories if missing.

## Instinct format (write exactly)
Each instinct is a markdown file with YAML front matter + sections:

```
---
id: <kebab-case-id>
trigger: "<when this applies>"
confidence: <0.3-0.9 float>
domain: "<one of: code-style|testing|git|debugging|workflow|docs|perf|security>"
source: "session-checkpoint"
---

# <Human title>

## Action
- What to do when triggered (imperative, 1-5 bullets)

## Evidence
- Bullet list of specific observations with dates + references.
- Prefer: file paths, command outputs, test names, error excerpts.

## Notes (optional)
- Edge cases / exceptions
```

## How to run (workflow)
### Step 1: Locate the session log
1. If `.codex/sessions/LATEST` exists, read it and use the pointed file.
2. Otherwise, ask the user for the session log path.

### Step 2: Extract observations
Scan the latest checkpoint section(s) and extract candidate observations in these categories:
- user_corrections (user had to re-steer behavior)
- error_resolutions (how a bug was fixed)
- repeated_workflows (multi-step sequences repeated)
- tool_preferences (preferred commands/tools/approvals)

For each observation, capture:
- category
- short description
- timestamp (or date)
- evidence pointer (file/command/test/error)

Append extracted observations as JSONL lines to:
`.codex/homunculus/observations.jsonl`

JSONL schema (one per line):
```
{"timestamp":"YYYY-MM-DD","category":"user_corrections|error_resolutions|repeated_workflows|tool_preferences","summary":"...","evidence":"file/command/test/error"}
```

### Step 3: Convert observations → instincts
For each observation:
- Propose a candidate instinct:
  - id, trigger, domain
  - action (concrete)
  - evidence list (include this observation)
  - initial confidence:
    - 0.5 if strongly evidenced in this session
    - 0.3 if tentative / weak evidence
- Deduplicate:
  - If an instinct with the same intent already exists (match by `id` OR normalized `trigger + action + domain`), update:
    - evidence (append)
    - confidence (see next section)
  - Do NOT create near-duplicates.

### Step 4: Confidence update rules
Maintain confidence with a simple, explainable policy:
- +0.1 when the pattern appears again in a new checkpoint AND user did not correct it
- -0.2 when user explicitly corrects/rejects it
- -0.05 decay if not observed for a long time (only if you have dates)

Clamp to [0.3, 0.9].
Only apply at most one confidence increase per checkpoint per instinct.

Before writing changes, show the user:
- which instincts will be created/updated
- the confidence deltas
Ask for edits if needed, then write files.

If no observations are found, report "nothing to update" and stop.

### Step 5 (optional): Evolve strong instincts into a Codex skill
If there are >= 3 related instincts in the same domain with confidence >= 0.7:
- Propose an evolved skill:
  - name: <kebab-case>
  - description: when to use it
  - instructions: a workflow that applies those instincts together
- Ask for explicit confirmation before writing the evolved skill.
- Write to: `.codex/homunculus/evolved/skills/<skill-name>/SKILL.md`
- Also print: how to invoke in Codex:
  - `$<skill-name>` or `/skills` then select it

## Safety & review
- Never modify source code as part of this learning step unless explicitly asked.
- Always prefer writing/updating the homunculus files only.
- No speculation: if the evidence is weak, keep confidence low (0.3–0.5).
