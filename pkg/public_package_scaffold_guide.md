# Package Contracts Guide

This folder defines public package scaffolds and future promotion rules for stable import-safe surfaces.

Current scope:
- `pkg/contracts`: scaffold-only (no promoted public Go API yet).

## PRD alignment for public package contracts

`docs/PRD.md` defines versioned/stable framework contracts and developer-facing primitives. Public package promotion from this folder must preserve those guarantees for external Go consumers.

Canonical contract sources remain:
- `api/controlplane/*`
- `api/eventabi/*`
- `api/observability/*`

Public `pkg/*` promotion policy:
- Promote only contract surfaces that must be importable by external consumers.
- Preserve backward compatibility and version notes for promoted types.
- Keep replay/determinism-relevant fields stable when surfaced publicly.
- Keep `pkg/*` API stability stricter than `internal/*`.

Current coding status:
- no promoted public Go API yet; this folder remains scaffold-first.
