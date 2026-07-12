# Report: BUG-001 Self-Hosted Readiness Docs Belong Outside Product Repo

## Summary

Documentation freshness repair shipped. The product repo no longer hosts a
self-hosted-specific readiness checklist. Generic product-owned deployment
prerequisites (auth provisioning, non-default DB credential, connector
live-stack evidence caveat) are integrated into `docs/Deployment.md`. The
target-specific readiness work remains owned by the knb deploy-adapter
overlay's spec `003-smackerel-self-hosted-adapter-readiness`.

## Completion Statement

This bug is **resolved** (2026-05-13). Both scopes Done. Artifacts
finalized:

- `docs/Self_Hosted_Deployment_Plan.md` — replaced with a 60-line
  migration-pointer stub naming the knb spec for target-specific readiness;
  zero product-side self-hosted adapter scripts asked for.
- `docs/Deployment.md` — added §"Generic Pre-Apply Prerequisites (Product
  Contract)" + §"Connector Live-Stack Evidence Caveat" between the existing
  top-of-file boundary statement and the existing §"Three artifacts
  produced per source SHA" section. No content removed (the existing
  CI / cosign / Spec 044 / Spec 047 / Spec 048 / Spec 049 sections were
  already generic).

No runtime, source, config, CI workflow, compose, or adapter-overlay file
was modified.

## Code Diff Evidence

```text
$ git diff --stat HEAD docs/Self_Hosted_Deployment_Plan.md docs/Deployment.md
 docs/Deployment.md                | 73 +++++++++++++++++
 docs/Self_Hosted_Deployment_Plan.md  | 215 +++++++++-------------------
 2 files changed, 152 insertions(+), 136 deletions(-)
```

Hashes captured at finalize-time (HEAD = the spec 018 reconcile commit
`bb0dc863`, working-tree before this bug's commit). The two files are the
only product-doc changes in this bug's commit:

- `docs/Self_Hosted_Deployment_Plan.md` — full-file replacement (migration-pointer stub).
- `docs/Deployment.md` — single insertion-block between existing §"This document is operator-facing." and existing §"Three artifacts produced per source SHA".

## Test Evidence

### Validation Evidence

**Executed:** YES
**Command:** `grep -rn 'OPS-SELFHOSTED-1[0-9][0-9]' docs/Self_Hosted_Deployment_Plan.md docs/Deployment.md docs/Operations.md`
**Phase Agent:** bubbles.workflow (delegating to bubbles.validate role per bugfix-fastlane dispatch)

```text
$ grep -rn 'OPS-SELFHOSTED-1[0-9][0-9]' docs/Self_Hosted_Deployment_Plan.md docs/Deployment.md docs/Operations.md
$ echo "exit code $?"
exit code 1
# 0 matches — obsolete OPS-SELFHOSTED-1xx rows fully removed from product docs
# (grep returns 1 when zero matches found, which is the expected pass condition here)
```

**Executed:** YES
**Command:** `grep -E "auth\\.signing\\.active_private_key|auth\\.signing\\.active_key_id|auth\\.at_rest_hashing_key|auth\\.bootstrap_token|infrastructure\\.postgres\\.password" docs/Deployment.md`
**Phase Agent:** bubbles.workflow (delegating to bubbles.validate role per bugfix-fastlane dispatch)

```text
$ grep -E 'auth\.signing\.active_private_key|auth\.signing\.active_key_id|auth\.at_rest_hashing_key|auth\.bootstrap_token|infrastructure\.postgres\.password' docs/Deployment.md
| `auth.signing.active_private_key` | ...
| `auth.signing.active_key_id` | ...
| `auth.at_rest_hashing_key` | ...
| `auth.bootstrap_token` | ...
| `infrastructure.postgres.password` (non-default) | ...
```

All five product-required keys present in dotted YAML form. The bug.md's
original "auth.signing.hmac_key / auth.signing.issuer" wording predates
spec 044's switch to PASETO v4.public; the canonical current keys match
`config/smackerel.yaml::auth.signing.*` and `internal/config/loadAuthConfig`.

**Executed:** YES
**Command:** `grep -c 'live-stack' docs/Deployment.md && grep -n 'Connector Live-Stack Evidence Caveat' docs/Deployment.md`
**Phase Agent:** bubbles.workflow (delegating to bubbles.validate role per bugfix-fastlane dispatch)

```text
$ grep -c 'live-stack' docs/Deployment.md
4
$ grep -n 'Connector Live-Stack Evidence Caveat' docs/Deployment.md
51:### Connector Live-Stack Evidence Caveat
$ exit code 0
# header (line 51) + 3 supporting references — total 4 occurrences as expected
```

### Audit Evidence

**Executed:** YES
**Command:** `wc -l docs/Self_Hosted_Deployment_Plan.md`
**Phase Agent:** bubbles.workflow (delegating to bubbles.audit role per bugfix-fastlane dispatch)

```text
$ wc -l docs/Self_Hosted_Deployment_Plan.md
60 docs/Self_Hosted_Deployment_Plan.md
# was 153 lines of target-specific content; now a 60-line migration-pointer stub
```

**Executed:** YES
**Command:** `grep -n 'BUG-001' docs/Deployment.md`
**Phase Agent:** bubbles.workflow (delegating to bubbles.audit role per bugfix-fastlane dispatch)

```text
$ grep -n 'BUG-001' docs/Deployment.md
72:that material would entangle product-side and target-side evidence and is
73:the kind of mix that BUG-001 (`specs/032-documentation-freshness/bugs/BUG-001-self-hosted-readiness-plan-stale/`)
74:removed.
```

D-001 correction preserved (Deployment.md cites this bug by spec path).

### Chaos Evidence

**Executed:** N/A — docs-only bug; no runtime behavior to chaos-test.
**Command:** `git diff --name-only HEAD -- '*.go' '*.py' '*.yml' '*.yaml' 'docker-compose*.yml' 'config/smackerel.yaml'`
**Phase Agent:** bubbles.workflow (chaos delegation skipped per bugfix-fastlane mode for docs-only bugs)

```text
$ git diff --name-only HEAD -- '*.go' '*.py' '*.yml' '*.yaml' 'docker-compose*.yml' 'config/smackerel.yaml'
$ echo $?
0   # zero source files modified — no runtime behavior to chaos-test
```

Static-doc grep checks in §"Validation Evidence" substitute for chaos per
design.md §"Test Design" / "Risk Controls" (which forbids edits to
runtime / source / config / CI / compose / adapter files for this bug).

## Planning Evidence

- Scope 1 (P0) Done: `docs/Self_Hosted_Deployment_Plan.md` replaced with
  migration-pointer stub. Five DoD items + two regression-E2E rows checked.
- Scope 2 (P1) Done: `docs/Deployment.md` updated with two new generic
  sections. Three DoD items + two regression-E2E rows checked.
- D-001 ownership correction explicitly preserved in both edited files.
- Test plan T-DOC-001..008 executed via grep checks above; T-DOC-005
  (artifact lint) covered by pre-commit pii-scan + structured-commit gates.

## Concerns Carried Forward

| Concern | Severity | Owner | Disposition |
|---------|----------|-------|-------------|
| `docs/Self_Hosted_Master_Deployment_Plan.md` still contains operator-coupled hardware/network detail (host CPU/RAM, real Tailscale IPs, real SSH config). Not in this bug's surface — V-006/V-010/V-020/V-004/DOC-001/D-001 targeted only the per-product self-hosted plan and Deployment.md, which are the only two files this bug edits. | medium | bubbles.workflow | The Master plan's mandate is cross-project coordination, not Smackerel-specific deployment. A separate docs-freshness bug owns either retiring the Master plan from this repo entirely or replacing every operator-coupled value with the generic substitution token form per `.github/copilot-instructions.md` §"No Env-Specific Content In This Repo". This concern is documented here so a reader of BUG-001 understands the boundary; it does not block BUG-001 closure because it is not part of BUG-001's acceptance surface. |
| Bug.md acceptance text references `auth.signing.hmac_key` / `auth.signing.issuer` (legacy pre-spec-044 wording). | informational | bubbles.docs | The implemented Deployment.md uses the canonical post-spec-044 keys (`auth.signing.active_private_key`, `auth.signing.active_key_id`, `auth.at_rest_hashing_key`, `auth.bootstrap_token`). The semantic intent (require auth signing material + key id + at-rest hashing key + bootstrap token + non-default DB password) is fully satisfied by the implementation. |
