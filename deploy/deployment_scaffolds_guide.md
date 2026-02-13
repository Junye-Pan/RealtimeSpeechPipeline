# Deployment Assets Guide

This folder currently contains placeholder documentation stubs for deployment artifacts and infrastructure scaffolds.

## PRD deployment commitments (documentation scope only)

The following production commitments come from `docs/PRD.md` and are tracked here as deployment targets:

- Kubernetes-native stateless runtime with horizontal scaling.
- Multi-region routing/failover with single-authority execution enforced by lease/epoch checks.
- Deployment support for transport-agnostic orchestration paths (RTC/WebSocket/telephony), with MVP transport path fixed to LiveKit.
- Operational gate alignment for latency/cancellation/authority/replay objectives.

Current coding status for this folder: unimplemented scaffolds (guides only).

Subfolders (current status: placeholders only):
- `deploy/k8s`: Placeholder guide only (Kubernetes manifests/scaffolds not yet added)
- `deploy/helm`: Placeholder guide only (Helm chart scaffolds not yet added)
- `deploy/terraform`: Placeholder guide only (Terraform scaffolds not yet added)
