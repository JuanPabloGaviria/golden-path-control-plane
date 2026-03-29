# Kubernetes Proof Surface

Back to the flagship documents:

- [Main README](../../README.md)
- [Verification matrix](../../docs/verification-matrix.md)

This directory exists to support one honest Kubernetes claim: the repository has a verified local cluster deployment shape, and that claim is limited to the overlay that is actually exercised.

## Verified Overlay

- `overlays/local-kind` is the only Kubernetes deployment shape this repository claims today.
- It deploys PostgreSQL, the local OIDC proof issuer, the migration job, the API, and the worker.
- It is exercised by `make smoke-kind`.

## What The Layout Means

- `base` is a reusable workload definition, not a production claim.
- `overlays/local-kind` is the concrete proof surface.
- The local OIDC issuer exists to validate JWT/JWKS control-plane behavior, not to imitate an enterprise identity estate.

## How To Read This Directory

If you want the strongest proof, use this order:

1. [README.md](../../README.md)
2. [docs/verification-matrix.md](../../docs/verification-matrix.md)
3. `kubectl kustomize deployments/kubernetes/overlays/local-kind`
4. `make smoke-kind`

## Non-Claims

- No AWS or EKS proof is claimed here.
- No production multi-environment rollout process is claimed here.
- No static manifest in this directory should be read as stronger evidence than the smoke it is paired with.
