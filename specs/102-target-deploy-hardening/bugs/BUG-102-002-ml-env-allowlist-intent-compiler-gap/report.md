# Report — BUG-102-002

## Summary

The `ml.env` projection staged into every `self-hosted` config bundle omitted the
seven `ASSISTANT_INTENT_COMPILER_*` keys that
`ml/app/main.py::_check_required_config` requires unconditionally. `smackerel-ml`
therefore called `sys.exit(1)` about one second after start on every deploy,
`smackerel-core` never cleared ML readiness, `/api/health?strict=true` never
returned 200, and the knb adapter's `ollama-pre-retirement-proof` apply gate
refused. The defect is latent rather than new — it blocked the earlier `02697bfd`
build from landing as well, not only `cf005ed4`.

Two changes shipped together:

1. **`config/smackerel.yaml`** — one PREFIX entry, `"ASSISTANT_INTENT_COMPILER_*"`,
   added to `services.ml.env_allowlist`. A prefix rather than the seven literal
   keys: the sidecar's required set grows in batches (BUG-069-005 added all seven
   at once), so an exact list is the same drift trap that produced this bug.
2. **`internal/deploy/bundle_secret_contract_test.go`** — the hand-maintained
   `required` slice was replaced with a set DERIVED from the sidecar sources (the
   `keys = [...]` literal in `_check_required_config`, plus every
   `os.environ["KEY"]` subscript read under `ml/app/`). The hand-maintained list
   was the reason the existing boot-safety guard stayed silent through the
   outage, so removing it is the durable half of the fix.

**This packet is NOT complete.** The fix is verified in-repo; the live deploy that
SCN-102-002-A describes has not been performed. See
[Evidence 7](#evidence-7--live-deploy) and [Honest gaps](#honest-gaps).

## Completion Statement

Six of the seven SCOPE-01 DoD items are satisfied with first-hand evidence
collected in this session and recorded below. The seventh — build, promote, and
verify the fixed SST on the live `<deploy-target>` target — is **unsatisfied and left
unchecked**, because no build, promotion, or host interaction was performed. The
scope status stays **In Progress** and `state.json` status stays `in_progress`.

Scenario coverage:

| Scenario | Status | Where |
|---|---|---|
| SCN-102-002-B — projection carries the intent-compiler contract | PASS | [Evidence 2](#evidence-2--bundle-projection-carries-the-seven-keys), [Evidence 3](#evidence-3--derived-guard--adversarial-bite) |
| SCN-102-002-C — secret isolation survives the fix | PASS | [Evidence 2](#evidence-2--bundle-projection-carries-the-seven-keys), [Evidence 3](#evidence-3--derived-guard--adversarial-bite) |
| SCN-102-002-D — new boot requirement without allowlist coverage fails | PASS | [Evidence 3](#evidence-3--derived-guard--adversarial-bite) |
| SCN-102-002-E — allowlist regression fails | PASS | [Evidence 3](#evidence-3--derived-guard--adversarial-bite) |
| SCN-102-002-F — broken derivation fails loudly | PASS (structural) | [Evidence 3](#evidence-3--derived-guard--adversarial-bite) |
| SCN-102-002-A — sidecar boots against the projected env on the live target | **NOT VERIFIED** | [Evidence 7](#evidence-7--live-deploy) |

### Evidence redaction notice

Every block below is verbatim command output from this session with exactly three
mechanical substitutions applied, required by the repo's no-env-specific-content
policy and enforced by the pre-commit PII scan. Nothing else was altered, no line
was removed, and no line was reordered:

| Real value in the terminal | Written here |
|---|---|
| the operator's absolute home path | `<operator-home>` |
| the deploy host's tailnet CGNAT IPv4 | `<host-tailnet-ip>` |
| the deploy host's short name | `<deploy-host>` |

The tailnet IP is injected at generate time and is **not** present in the
committed tree; `git grep` over all tracked non-`specs/` files returned no hit.
`dist/` and `config/generated/` are both gitignored, so no generated artifact
carrying it is committed.

## Test Evidence

### Evidence 1 — Bundle regeneration

Command run twice, back to back, to check that the bundle digest is deterministic
for a fixed `(env, sourceSha)` — a Build-Once Deploy-Many requirement, not just a
convenience. Both runs produced the same `sha256`.

```
$ DISK_PREFLIGHT_OVERRIDE=1 ./smackerel.sh config generate --env self-hosted --bundle --source-sha 9b155650f02e66c1bf1d22cb8d9f87ddbd205613
config-validate: skipped for production-class target env=self-hosted (placeholder mode; runtime check enforces at container start)
Generated <operator-home>/smackerel/config/generated/self-hosted.env
Generated <operator-home>/smackerel/config/generated/nats.conf
Generated <operator-home>/smackerel/config/generated/prometheus.yml
Generated <operator-home>/smackerel/internal/experience/catalog.gen.json
Generated <operator-home>/smackerel/dist/config-bundles/config-bundle-self-hosted-9b155650f02e66c1bf1d22cb8d9f87ddbd205613.tar.gz
  sha256: f20e30ab31abdb6209d396a1ae2c8d9efd5a934be495be070632160a9f210c56
  sourceSha: 9b155650f02e66c1bf1d22cb8d9f87ddbd205613
  environment: self-hosted
GEN_EXIT=0
```

Second run:

```
$ DISK_PREFLIGHT_OVERRIDE=1 ./smackerel.sh config generate --env self-hosted --bundle --source-sha 9b155650f02e66c1bf1d22cb8d9f87ddbd205613
config-validate: skipped for production-class target env=self-hosted (placeholder mode; runtime check enforces at container start)
Generated <operator-home>/smackerel/config/generated/self-hosted.env
Generated <operator-home>/smackerel/config/generated/nats.conf
Generated <operator-home>/smackerel/config/generated/prometheus.yml
Generated <operator-home>/smackerel/internal/experience/catalog.gen.json
Generated <operator-home>/smackerel/dist/config-bundles/config-bundle-self-hosted-9b155650f02e66c1bf1d22cb8d9f87ddbd205613.tar.gz
  sha256: f20e30ab31abdb6209d396a1ae2c8d9efd5a934be495be070632160a9f210c56
  sourceSha: 9b155650f02e66c1bf1d22cb8d9f87ddbd205613
  environment: self-hosted
GEN2_EXIT=0
```

`f20e30ab…` is the digest of the bundle produced from the tree as it stands in
this session. It is deliberately **not** the digest recorded during the original
triage session, because the tree has changed since (the packet artifacts and the
regenerated `catalog.gen.json` are inside the generated set). Reporting the
earlier digest here would have been transcription, not measurement.

### Evidence 2 — Bundle projection carries the seven keys

The bundle from Evidence 1 was extracted and its `ml.env` member read directly.
This is the artifact the sidecar actually consumes: `deploy/compose.deploy.yml`
gives `smackerel-ml` exactly `env_file: [./ml.env]`.

```
$ tar -xzf dist/config-bundles/config-bundle-self-hosted-9b155650f02e66c1bf1d22cb8d9f87ddbd205613.tar.gz -C "$WORK"
=== bundle members ===
alertmanager.yml
alertmanager_ntfy_url
alerts.yml
app.env
assistant
bundle-manifest.yaml
config
docker-compose.yml
ml.env
nats.conf
nats_contract.json
prometheus.yml
prompt_contracts
secret-keys.yaml

=== ml.env : ASSISTANT_INTENT_COMPILER_* ===
ASSISTANT_INTENT_COMPILER_ENABLED=true
ASSISTANT_INTENT_COMPILER_MODEL_ROLE=assistant_intent_compiler
ASSISTANT_INTENT_COMPILER_PROVIDER_NAME=ollama
ASSISTANT_INTENT_COMPILER_PROVIDER_URL=http://<host-tailnet-ip>:11434
ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION=intent-compiler-v1
ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION=v1
ASSISTANT_INTENT_COMPILER_TIMEOUT_MS=5000
ASSISTANT_INTENT_COMPILER_CONFIDENCE_FLOOR=0.6
ASSISTANT_INTENT_COMPILER_MAX_CONTEXT_TURNS=8
ASSISTANT_INTENT_COMPILER_MAX_OUTPUT_BYTES=16384
ASSISTANT_INTENT_COMPILER_RETRY_BUDGET=1
grep_exit=0

count=11

=== 7 boot-required keys present? ===
  PRESENT ASSISTANT_INTENT_COMPILER_ENABLED
  PRESENT ASSISTANT_INTENT_COMPILER_MODEL_ROLE
  PRESENT ASSISTANT_INTENT_COMPILER_PROVIDER_NAME
  PRESENT ASSISTANT_INTENT_COMPILER_PROVIDER_URL
  PRESENT ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION
  PRESENT ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION
  PRESENT ASSISTANT_INTENT_COMPILER_TIMEOUT_MS

=== isolated-ML-sidecar boundary scan (expect NO matches) ===
boundary_scan_exit=1 (1 = no matches = clean)

=== ml.env total line count ===
108
```

Reading of this block:

- All **seven** boot-required keys are present. Pre-fix this count was zero, which
  is what `_check_required_config` reported before calling `sys.exit(1)`.
- **Eleven** keys are projected, not seven, because the allowlist entry is a
  prefix. The four extra keys (`_CONFIDENCE_FLOOR`, `_MAX_CONTEXT_TURNS`,
  `_MAX_OUTPUT_BYTES`, `_RETRY_BUDGET`) are non-secret members of the same
  compiler contract; spec.md scopes them out of any separate claim.
- The boundary scan for `DATABASE_URL|POSTGRES_*|REDIS_*|RABBITMQ_*` exited **1**,
  i.e. **no matches**. The isolated-ML-sidecar boundary is intact: the Python tier
  still receives no database, cache, or queue credential. Note that a `grep` exit
  of 1 is the clean result here and an exit of 0 would have been the failure.

The secret-exclusion property is additionally asserted by
`TestMLEnv_ExcludesManagedSecretsAndPostgres_Spec102` in Evidence 3, which checks
every member of `SHELL_SECRET_KEYS` rather than the four families scanned here.

### Evidence 3 — Derived guard + adversarial bite

```
$ DISK_PREFLIGHT_OVERRIDE=1 ./smackerel.sh test unit --go --go-run 'TestMLEnv' --verbose
...
--- PASS: TestMLEnv_ExcludesManagedSecretsAndPostgres_Spec102 (17.40s)
=== RUN   TestMLEnv_ContainsEveryComputeKey_Spec102
    bundle_secret_contract_test.go:1076: SCN-102-C1-02 OK — all 30 derived fail-loud sidecar keys are projected into ml.env (104 keys).
--- PASS: TestMLEnv_ContainsEveryComputeKey_Spec102 (6.64s)
=== RUN   TestMLEnv_DerivedComputeKeySet_HasBite_BUG102002
=== RUN   TestMLEnv_DerivedComputeKeySet_HasBite_BUG102002/sidecar_requires_an_unprojected_key
=== RUN   TestMLEnv_DerivedComputeKeySet_HasBite_BUG102002/allowlist_drops_the_intent-compiler_prefix
    bundle_secret_contract_test.go:1164: BUG-102-002 bite confirmed — dropping the allowlist entry strips 7 boot-required keys: [ASSISTANT_INTENT_COMPILER_ENABLED ASSISTANT_INTENT_COMPILER_MODEL_ROLE ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION ASSISTANT_INTENT_COMPILER_PROVIDER_NAME ASSISTANT_INTENT_COMPILER_PROVIDER_URL ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION ASSISTANT_INTENT_COMPILER_TIMEOUT_MS]
--- PASS: TestMLEnv_DerivedComputeKeySet_HasBite_BUG102002 (13.94s)
    --- PASS: TestMLEnv_DerivedComputeKeySet_HasBite_BUG102002/sidecar_requires_an_unprojected_key (0.00s)
    --- PASS: TestMLEnv_DerivedComputeKeySet_HasBite_BUG102002/allowlist_drops_the_intent-compiler_prefix (6.81s)
=== RUN   TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102
=== RUN   TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102/exact_secret_key
=== RUN   TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102/prefix_glob_covering_a_secret
--- PASS: TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102 (9.99s)
    --- PASS: TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102/exact_secret_key (5.27s)
    --- PASS: TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102/prefix_glob_covering_a_secret (4.72s)
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102/uppercase_suffix
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102/mixed-case_suffix
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102/sanctioned_LLM_API_key_remains_projected
--- PASS: TestMLEnv_CredentialSuffixRequiresSanction_Spec102 (18.89s)
    --- PASS: TestMLEnv_CredentialSuffixRequiresSanction_Spec102/uppercase_suffix (5.16s)
    --- PASS: TestMLEnv_CredentialSuffixRequiresSanction_Spec102/mixed-case_suffix (5.26s)
    --- PASS: TestMLEnv_CredentialSuffixRequiresSanction_Spec102/sanctioned_LLM_API_key_remains_projected (8.47s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  66.892s
...
[go-unit] go test ./... finished OK
FOCUSED_EXIT=0
```

Reading of this block:

- **The derivation is live, not decorative.** The guard reports `30 derived
  fail-loud sidecar keys` against `104 keys` projected. The previous
  hand-maintained expectation was 15 literals, so the derived set is twice the
  size of the list that drifted. That count is emitted by the test itself; it is
  not a claim added here.
- **The bite test names the exact seven keys.** The
  `allowlist_drops_the_intent-compiler_prefix` sub-case removes the new allowlist
  entry from a tampered copy of `config/smackerel.yaml`, regenerates a real bundle
  in a sandbox root, and reports precisely the seven boot-required keys as
  missing. This is the outage reproduced under test: had this guard existed in its
  derived form, BUG-069-005 could not have shipped the gap.
- **The other direction is covered too.** `sidecar_requires_an_unprojected_key`
  injects a sentinel into an in-memory copy of the boot list and asserts it is
  reported missing, so a future eighth key that the prefix does not cover still
  fails.
- **SCN-102-002-F is structural.** The `DERIVATION BROKEN` floors (≥12 boot keys,
  ≥8 subscript reads) sit inside `deriveMLBootRequiredKeys`, which every test
  above calls. A parse that silently matched nothing would fail all five tests
  rather than make them vacuous, so it has no separate runner row.
- **Nothing was weakened.** `TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102`
  and `TestMLEnv_CredentialSuffixRequiresSanction_Spec102` are pre-existing
  tripwires and both still pass, including the `prefix_glob_covering_a_secret`
  sub-case — which is the one that would catch a future prefix entry that swept a
  secret into the least-trusted tier.

The full 534-line captured run is summarised by `FOCUSED_EXIT=0`; every package
outside `internal/deploy` reported `[no tests to run]` under the `-run` filter, as
expected.

### Evidence 4 — check

```
# BUG-102-002 smackerel.sh check
$ ./smackerel.sh check
exit: 0
lines: 6
sha256: 2fca374455f8ccf7221dd50272883e8d0b41aab89af81b65e2ac99ba07548e68
--- output ---
config-validate: <operator-home>/smackerel/config/generated/dev.env.tmp.3497633 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
```

`Config is in sync with SST` and `env_file drift guard: OK` are the two lines that
matter here: the allowlist edit did not leave a generated artifact stale relative
to `config/smackerel.yaml`.

### Evidence 5 — lint

```
# BUG-102-002 smackerel.sh lint
$ ./smackerel.sh lint
exit: 0
lines: 157
sha256: 59895a9c0fc99392e1dbff1bc7031bc95af3ce0b10605a52b9cc9e9c29f232e4
--- last 20 ---
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)

=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
  OK: web/extension/lib/queue.js
  OK: web/extension/lib/browser-polyfill.js

=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)

Web validation passed
```

The lint lane's 157 lines are bounded above by the recorded `sha256`, which
`evidence-capture.sh --verify` can re-derive. Because the tail of that lane is web
validation rather than the guard this bug turns on, the
`python-compute-only-guard` was additionally run on its own so its verdict is
legible rather than buried:

```
# BUG-102-002 python-compute-only-guard (isolated-ML-sidecar boundary)
$ bash scripts/lint/python-compute-only-guard.sh
exit: 0
lines: 4
sha256: 0297c14ef915f5b9813baec1d9d38f59d22a4d46719fa4cc87a49f6fbb89f206
--- output ---
python-compute-only-guard: OK - dependency scan: no forbidden datastore driver in 2 dependency file(s) under ml (nats-py is the sanctioned transport)
python-compute-only-guard: OK - infra-URL scan: no direct DATABASE_URL/POSTGRES_URL/REDIS_URL/RABBITMQ_URL read in *.py (NATS_URL is the sanctioned wire)
python-compute-only-guard: OK - env-delivery shape: smackerel-ml loads the projected ./ml.env (not ./app.env); services.ml.env_allowlist is declared
python-compute-only-guard: clean - smackerel Python (ml) is compute-only: no datastore driver, no direct datastore-URL read, secret-free projected env delivery.
```

The third line is the one this bug could plausibly have broken: the fix widens
what `ml.env` carries, and the guard confirms `smackerel-ml` still loads the
projected `./ml.env` rather than the full-secret `./app.env`.

### Evidence 6 — Full unit suite

```
# BUG-102-002 full unit lane
$ ./smackerel.sh test unit
exit: 0
lines: 442
sha256: f60a5e02faab4eebcca94fc8d5178b995cb238bac319fa9e04474e10ff3a7ff0
--- first 20 ---
oom-preflight: OK — 27509 MB available (need 6000 MB; swap used 2938 MB).
disk-preflight: OVERRIDE set — skipping disk gate.
++ dirname /workspace/scripts/runtime/go-unit.sh
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
[go-unit] envsubst missing — installing gettext-base
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
+ apt-get install -y --no-install-recommends gettext-base
Reading package lists...
Building dependency tree...
Reading state information...
The following NEW packages will be installed:
  gettext-base
0 upgraded, 1 newly installed, 0 to remove and 20 not upgraded.
Need to get 160 kB of archives.
After this operation, 660 kB of additional disk space will be used.
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
--- omitted 402 line(s); sha256 above covers the full output ---
--- last 20 ---
  ...
1..2
# tests 2
# suites 0
# pass 2
# fail 0
# cancelled 0
# skipped 0
# todo 0
# duration_ms 176.961756
PASS: bug_077_002_login_session_reuse_test (SCN-077-BUG-002-01 / SCN-077-BUG-002-02)
[test unit] -> bash <operator-home>/smackerel/tests/unit/web/spec_077_discovery_convention_test.sh
PASS: spec_077_discovery_convention_test (TP-077-02-01 / SCN-077-A02)
[test unit] -> bash <operator-home>/smackerel/tests/unit/web/spec_077_no_stub_bodies_test.sh
PASS: spec_077_no_stub_bodies_test (TP-077-03-06 / SCN-077-A08)
[test unit] shell unit tests in tests/unit/web/ finished OK
[test unit] running 1 shell unit test(s) from tests/unit/docs/
[test unit] -> bash <operator-home>/smackerel/tests/unit/docs/spec_077_test_category_parity_test.sh
PASS: spec_077_test_category_parity_test (TP-077-02-03 / SCN-077-A06)
[test unit] shell unit tests in tests/unit/docs/ finished OK
```

Exit 0 across the whole lane. No collateral regression from the SST edit or the
guard rewrite.

### Evidence 7 — Live deploy

**NOT PERFORMED. This section records an absence, not a result.**

No build, no promotion, no host interaction was carried out in this session.
Nothing below is claimed:

- the ML sidecar reaching `Application startup complete` against the fixed `ml.env`
- 8/8 containers healthy on the target
- running core and ml digests matching a new manifest
- `/api/health?strict=true` returning 200
- the knb adapter's `ollama-pre-retirement-proof` gate passing

SCN-102-002-A is therefore **unverified**, and its DoD item is left unchecked in
[scopes.md](scopes.md). What the in-repo evidence does establish is the necessary
precondition: the artifact the sidecar reads now contains every key whose absence
caused the exit. Whether the sidecar then boots cleanly on the target is a
separate claim that only a real deploy can settle.

## Honest gaps

1. **Live deploy outstanding (the packet's only open DoD item).** The fix is
   proven against a real generated bundle and against the derived guard, but the
   end-to-end assertion in SCN-102-002-A requires a build and promote on the
   `<deploy-target>` target. Until that runs, `status` stays `in_progress`. Treating a
   green local lane as proof that the deploy now lands would be exactly the
   inference this bug punished — the previous guard was also green while the
   deploy was broken.

2. **Parent `uservalidation.md` left unchecked, deliberately.**
   `specs/102-target-deploy-hardening/uservalidation.md` carries, under
   SCN-102-C1, the item:

   > `smackerel-ml` still starts cleanly against `ml.env` — no missing-env
   > fail-loud error (it has every compute key it actually reads).

   That is this exact failure mode, and this bug is what will close it. It is
   **not** checked here. The item is phrased as something the operator observes
   about a running sidecar, and no sidecar was run; the parent spec's own
   anti-fabrication convention reserves those boxes for confirmation against the
   live apply. It flips only after Evidence 7 exists. `uservalidation.md` is
   human-owned and was left byte-identical.

3. **Bundle digest differs from the triage session's.** Explained in
   [Evidence 1](#evidence-1--bundle-regeneration): the recorded `f20e30ab…` is
   what this tree produces now, measured twice. The earlier value was not carried
   forward, because copying a digest is not measuring one.

4. **Four extra keys ride the prefix.** `_CONFIDENCE_FLOOR`,
   `_MAX_CONTEXT_TURNS`, `_MAX_OUTPUT_BYTES` and `_RETRY_BUDGET` are now projected
   into the ML tier as a consequence of choosing a prefix over seven literals.
   They are non-secret and carry no credential suffix, and the
   credential-shape tripwire (Evidence 3) still refuses a future
   `ASSISTANT_INTENT_COMPILER_*` key that looks like a credential. This is a
   recorded consequence of the design decision, not an unnoticed side effect;
   spec.md scopes it out of any separate claim.
