# Kubernetes Deployment Scaffold

MVP scaffolds are implemented under `deploy/k8s/mvp-single-region/`:

- `runtime-deployment.yaml`
- `control-plane-deployment.yaml`

These manifests are the canonical Kubernetes baseline for `mvp-single-region` and are validated by contract tests in `test/contract/scaffold_artifacts_test.go`.
