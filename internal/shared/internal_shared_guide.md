# Internal Shared Guide

This folder contains internal cross-domain helpers that are not domain owners by themselves.

PRD alignment scope:
- intended to support framework-wide consistency for configuration and shared helper behavior once implemented
- intended to support normalized internal error mapping across domains once implemented
- does not own runtime/control-plane/observability/tooling semantics

## Subfolder map and current status

| Subfolder | Purpose | Current status |
| --- | --- | --- |
| `config` | configuration parsing and guardrails | scaffold only (not implemented yet) |
| `errors` | normalized internal error taxonomy | scaffold only (not implemented yet) |
| `types` | shared internal envelopes/aliases | scaffold only (not implemented yet) |
| `util` | non-domain utility helpers | scaffold only (not implemented yet) |

## PRD-relevant responsibilities (target state)

1. `config` should provide deterministic shared parsing/defaulting helpers used by multiple internal domains.
2. `errors` should provide shared error classes/mappings for cross-domain handling and reporting surfaces.
3. `types` should provide small internal envelopes/aliases reused by multiple internal modules.
4. `util` should provide deterministic generic helpers with no domain policy semantics.

Rule:
- Do not move domain semantics from `internal/controlplane`, `internal/runtime`, `internal/observability`, or `internal/tooling` into this folder.
- If a helper starts encoding domain policy or contract decisions, keep that logic in the owning domain module instead.
