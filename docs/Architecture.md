# Smackerel — Architecture

This document holds short, focused architecture notes that complement
[`docs/smackerel.md`](smackerel.md) (the full design document) and
[`docs/Deployment.md`](Deployment.md) (operator workflows). It is the
landing page for trust-boundary diagrams, security perimeters, and
cross-cutting architectural contracts that span both the runtime and
the deployment surface.

For the full system architecture (data flow, modules, storage, NATS
topology, ML sidecar boundaries), see [`docs/smackerel.md`](smackerel.md)
sections 3 (System Architecture), 8 (Storage), 17 (Trust), 18 (Privacy),
and 23 (Implementation reality).

---

## Secret Boundary (spec 052)

Smackerel's secret pipeline crosses three trust boundaries between three
distinct hosts running in three distinct security postures. The contract is
defined in
[`specs/052-bundle-secret-injection-contract/`](../specs/052-bundle-secret-injection-contract/);
this section is the operator-facing summary.

```
┌──────────────────────────────────────────────────────────────────────┐
│ L1: SST LOADER (build time, in CI or operator workstation)            │
│ scripts/commands/config.sh + internal/config/secret_keys.go (mirror)  │
│                                                                       │
│   for KEY in infrastructure.secret_keys:                              │
│     if TARGET_ENV in infrastructure.production_class_targets:         │
│       app.env: KEY=__SECRET_PLACEHOLDER__<KEY>__                      │
│       (skip FR-051-005 dev-default check for this key)                │
│     else:                                                             │
│       app.env: KEY=<literal yaml value>                               │
│       (FR-051-005 dev-default check still fires for actual literals)  │
│                                                                       │
│   bundle ships sibling: secret-keys.yaml (enumerates declared keys)   │
└─────────────────────────────────┬────────────────────────────────────┘
                                  │ tar.gz, deterministic
                                  │ cosign-signed, sha256-pinned
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│ L2: KNB ADAPTER (apply time, on target host with sops + age key)      │
│ <knb-repo>/smackerel/home-lab/apply.sh                                │
│                                                                       │
│   1. Verify bundle cosign signature (existing)                        │
│   2. Verify bundle sha256 against build manifest (existing)           │
│   3. Extract bundle → COMPOSE_DIR (existing)                          │
│   4. NEW: parse secret-keys.yaml from extracted bundle                │
│   5. NEW: assert every declared key has placeholder in app.env        │
│   6. sops -d secrets file → tmpfile chmod 0600 (existing)             │
│   7. NEW: assert every declared key has real value in tmpfile         │
│           (non-empty AND not equal to its placeholder marker)         │
│   8. docker compose --env-file app.env --env-file tmpfile up          │
│      (existing — Compose's "later wins" override does the substitution)│
│   9. NEW: docker compose --env-file ... config | grep __SECRET_       │
│           → MUST find zero placeholder markers in resolved env        │
│  10. Audit log: secrets_substituted=N placeholders_remaining=0 (NEW)  │
└─────────────────────────────────┬────────────────────────────────────┘
                                  │ docker compose up -d
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│ L3: GO RUNTIME (startup time, inside smackerel-core container)        │
│ internal/config/config.go::Validate()                                 │
│ internal/auth/startup.go::ValidateRuntimeAuthStartup()                │
│                                                                       │
│   for KEY in internal/config/secret_keys.go::SecretKeys():            │
│     if env[KEY] == __SECRET_PLACEHOLDER__<KEY>__:                     │
│       return fmt.Errorf("KEY still equals placeholder marker          │
│                          (spec 052 FR-052-007)")                      │
│       (FR-051-007 redaction: name KEY, never echo placeholder/value)  │
└──────────────────────────────────────────────────────────────────────┘
```

### Trust boundaries

| Layer | Host | Privilege | Secret access |
|-------|------|-----------|---------------|
| L1 SST loader | CI runner OR operator workstation | Build-time only; runs `./smackerel.sh config generate --env <env> --bundle` | **None** for production-class targets — emits placeholder marker only |
| L2 knb adapter | Target host (e.g. home-lab box) | Operator-trusted; runs `<knb-repo>/smackerel/<target>/apply.sh` | sops + age private key (`/etc/sops/age/keys.txt` or operator-mounted) |
| L3 Go runtime | Inside `smackerel-core` container on target host | Container-scoped; runs Smackerel process | Process env vars only — no key material on disk inside the container |

### Defense-in-depth invariants

Each layer assumes the layer below it may be compromised. Each layer fails
loud independently. **Compromising any one layer does not leak production
secrets nor allow a placeholder-as-credential boot:**

- L1 compromise (e.g. malicious CI step) → still cannot exfiltrate secrets
  because L1 has no secret access; the bundle ships placeholders.
- L2 compromise (e.g. operator machine compromise) → leaks the four
  bootstrap secrets in `secrets/<target>.enc.env`, which can be rotated
  without code changes; L3 still rejects any process started without
  substitution.
- L3 compromise (e.g. container escape) → process env vars are reachable,
  but the bundle and the operator secret store are not on the container
  filesystem.

The contract is enforced by the canonical secret-key manifest
(`config/smackerel.yaml::infrastructure.secret_keys`, mirrored in
`internal/config/secret_keys.go::secretKeys` and
`scripts/commands/config.sh`; drift caught by
`internal/deploy/bundle_secret_contract_test.go`).

For the full operator workflow (adding a new managed secret, rotating a
secret, auditor inspection), see:

- [`docs/Deployment.md` → Bundle Secret Injection (spec 052)](Deployment.md#bundle-secret-injection-spec-052)
- [`docs/Operations.md` → Bundle Secret Substitution (spec 052)](Operations.md#bundle-secret-substitution-spec-052)
- [`README.md` → Managed Secrets & Bundle Substitution (spec 052) — 3-Layer Defense](../README.md#managed-secrets--bundle-substitution-spec-052--3-layer-defense)
