# Kubernetes Assets

This directory now uses Kustomize instead of a single flat manifest.

## Verified overlay

- `overlays/local-kind` is the only Kubernetes deployment shape exercised by the repository today.
- It deploys PostgreSQL, the local OIDC proof issuer, the migration job, the API, and the worker.
- It is proven by `make smoke-kind`.

## Truthfulness rules

- `base` is a reusable workload definition, not a claim of production readiness on its own.
- No AWS or EKS runtime proof is claimed from these files.
- The local OIDC issuer is a proof asset for JWT/JWKS validation, not a production identity provider.
