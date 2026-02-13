# Public Contracts Scaffold

This folder is reserved for stable public contract types when external consumers require importable RSPP contracts.

Current status:
- scaffold only; canonical runtime contracts currently remain under `api/*`.

If promoted here, keep strict PRD-aligned contract guarantees:
- versioned compatibility rules for public types
- stable identity/correlation fields required for replay/operations
- explicit backward-compatibility and version notes for every breaking/non-breaking change
