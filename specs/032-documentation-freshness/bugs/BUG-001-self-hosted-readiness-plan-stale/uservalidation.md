# User Validation: BUG-001 Self-Hosted Readiness Docs Belong Outside Product Repo

## Checklist

- [x] Planning packet records the corrected target-adapter ownership and does not move adapter implementation or self-hosted checklist ownership into Smackerel.
- [x] Planning packet includes V-006, V-010, V-020, V-004, DOC-001, and the D-001 correction.
- [x] Planning packet scopes removal/migration of `docs/Self_Hosted_Deployment_Plan.md` target-specific content and generic `docs/Deployment.md` alignment without editing docs in planning mode.
- [x] Planning packet requires auth and secret provisioning to appear as generic deployment prerequisites, while self-hosted values and paths belong in knb.
- [x] Planning packet requires obsolete OPS rows to be removed or replaced with real planning links.

## Implementation Acknowledgement (2026-05-13)

| Item | Acknowledgement |
|------|-----------------|
| `docs/Self_Hosted_Deployment_Plan.md` replaced with a 60-line migration-pointer stub naming knb spec `003-smackerel-self-hosted-adapter-readiness`; zero product-side self-hosted adapter scripts asked for | Acknowledged |
| `docs/Deployment.md` extended with §"Generic Pre-Apply Prerequisites (Product Contract)" listing all five product-required keys (`auth.signing.active_private_key`, `auth.signing.active_key_id`, `auth.at_rest_hashing_key`, `auth.bootstrap_token`, non-default `infrastructure.postgres.password`) in dotted YAML form, anchored to Spec 044 OQ-8 + Spec 051 FR-051-004/005 | Acknowledged |
| `docs/Deployment.md` extended with §"Connector Live-Stack Evidence Caveat" tabling unit/static vs integration vs live-stack evidence classes; live-stack evidence assigned to the deploy-adapter overlay, not Smackerel CI | Acknowledged |
| Obsolete OPS-SELFHOSTED-1xx rows: zero matches in `docs/` after the rewrite | Acknowledged |
| D-001 ownership correction preserved (Smackerel does NOT host product-side `deploy/self-hosted/` adapter scripts) | Acknowledged |
| No runtime, source, config, CI workflow, compose, or adapter-overlay file modified | Acknowledged |
