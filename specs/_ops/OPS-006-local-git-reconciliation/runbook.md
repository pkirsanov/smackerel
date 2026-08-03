# OPS-006 Local Git Reconciliation Runbook

## Purpose

Use this runbook to reconcile Smackerel-local Git state without losing unique
work or publishing sensitive material.

Execute each phase in order. Stop on contradictory evidence, a failed backup,
or an unresolved secret or PII finding.

**Claim Source:** not-run

The original runbook was planning-only. The durable ledger below now records
current-session verification and the completed semantic classification.

## Durable Execution Ledger

**Claim Source:** interpreted

The execution ledger records semantic classifications supplied by the completed
review. Current-session commands must reverify every checked Scope 1 item.
Evidence must remain generic and must not expose local identity or topology.

No source item, ref, tag, ignored file, or recovery artifact is deleted by this
ledger update.

### Recovery Material

| Artifact | Verification | SHA-256 | Security disposition |
|---|---|---|---|
| Git recovery bundle | Complete history; 62 special refs | `255402a84fd3f42316c545abdd1a9a45188e3dbf6901bebbe9d71e3eb287f1df` | Retain in the private recovery directory |
| Ignored-work archive | Readable, 67 entries | `523d8c30e8a936bfb9620b99ac4e3e8fb0ebd1a4af28c15095fd40fffd635eea` | Retain in the private recovery directory |

The private directory mode is `0700`. Both recovery-file modes are `0600`.
The bundle records complete history and contains all 62 special refs: 11 tags,
9 dangling-commit refs, and 42 unreachable-commit refs. Every special ref
resolves to the expected object in both the repository and the bundle.

**Claim Source:** executed. See `report.md#recovery-and-ref-verification`.

### Protected Commit Dispositions

The completed semantic classification found no protected source work that
requires integration. Each protected source is `REPRESENTED` by reachable
rewrite counterpart work on `main`.

| Protected source | Final class | Rewrite counterpart |
|---|---|---|
| `a1ed9f91` | `REPRESENTED` | `0aa7c315` |
| `15ee91c2` | `REPRESENTED` | `4f99827e` |
| `afeedf7b` | `REPRESENTED` | `49a05656` |
| `e2f0b997` | `REPRESENTED` | `2351ed76` |
| `1ef1cd24` | `REPRESENTED` | `e1924a90` |
| `58f3655e` | `REPRESENTED` | `d2691fba`, `216cec66` |
| `73f3434d` | `REPRESENTED` | `116dae3a` |
| `e4fb11ae` | `REPRESENTED` | `732eafbc` |
| `34fd9bae` | `REPRESENTED` | `ac1cf705` |

**Claim Source:** interpreted classification plus executed object and ancestry
checks. See `report.md#protected-commit-representation`.

### Protected Unreachable Commit Dispositions

The completed semantic review classified all 42 commits left after mechanical
comparison. `stash snapshot` means a stash metadata, index, or untracked-support
commit retained only for recovery. `packet snapshot` means historical packet
evidence for which current tracked history remains authoritative. Neither class
contains unique product source that requires integration.

| Protected object | Final class | Safe reason or reachable counterpart |
|---|---|---|
| `094dd1fd087d6f4857ca09e3087af7630e6759e7` | `ARCHIVED` | Stash snapshot; no standalone file delta. |
| `09b22e558b2d6a2e5e7a84bd6c8c59c0f3ee0456` | `ARCHIVED` | Environment-specific model/config snapshot; recovery only. |
| `130fe2908e8a46ca02422f06da89111869b9a8cd` | `ARCHIVED` | Stash index snapshot; no standalone file delta. |
| `134b32e56d60ecd617cdec5357320e91bb1a5cfe` | `ARCHIVED` | Stash snapshot; no standalone file delta. |
| `13cba34b2aeeb6490eeeac58cacfe6506aa98c70` | `ARCHIVED` | Historical framework-install snapshot; recovery only. |
| `14775e35c52e96c1175deb7f8176e5e060d84dca` | `ARCHIVED` | Stash snapshot; recovery-only temporary gate work. |
| `1536947312085c1ef2286b5cb658d2674e04f085` | `ARCHIVED` | Historical certification packet snapshot. |
| `1c9dc462206af6fc32ec8ee3b67e80e07415c126` | `ARCHIVED` | Stash untracked snapshot; no standalone file delta. |
| `220c9359fa32625d6bb284e46074a34d5a3cd21f` | `ARCHIVED` | Historical reconciliation packet snapshot. |
| `22174c97003252dbec68b6ece33f8df61ae46f62` | `ARCHIVED` | Stash untracked snapshot; no standalone file delta. |
| `315f0253524cabff1f3cc6cb0ab04876ded592ff` | `ARCHIVED` | Stash snapshot; recovery-only pre-deploy work. |
| `323a3a3546516f2d05ff95fcf39b8dbb9b5b46f7` | `SUPERSEDED` | Current tracked planning artifacts on `origin/main` supersede this historical planning-truth packet snapshot. |
| `40fade815ef03e1cd44edd54e28e5a9c7a93f240` | `ARCHIVED` | Stash index snapshot; no standalone file delta. |
| `4736286db4da7082aa85318e1557f591fe7e2b7a` | `ARCHIVED` | Stash untracked snapshot; no standalone file delta. |
| `4c1b204385a4ea01b825f6c185b781c87b42d868` | `ARCHIVED` | Stash untracked product-WIP snapshot. |
| `529374a946dc615df4142e9346b88b612efa97a9` | `ARCHIVED` | Environment-specific untracked snapshot; recovery only. |
| `5d3640537588a6b83cad5a7800ade246a9cf5db1` | `ARCHIVED` | Stash snapshot; recovery-only bug work. |
| `5d40d407a96077b809804085ff7fd325d4e2c751` | `ARCHIVED` | Environment-specific untracked snapshot; recovery only. |
| `722c54366cc7a53ff6acb5353f07c5d45c02785b` | `ARCHIVED` | Contaminated WIP stash snapshot; recovery only. |
| `72acce5c85cb6b9a4e7877aea2dcad7836ec1d95` | `ARCHIVED` | Stash untracked spec-WIP snapshot. |
| `765adddbd0fbc4dbae23443f519d80cfd1247364` | `ARCHIVED` | Historical fail-loud NATS-auth snapshot; recovery only. |
| `7669a19787ea38921380f6448efb92755cd3ebe7` | `ARCHIVED` | Stash snapshot; recovery-only dependency work. |
| `7abefb9c46e5fa1c6fd42345e20209742cc25c19` | `ARCHIVED` | Stash snapshot; recovery-only assistant bug work. |
| `863be27863b155e6f37155220d102d19ac725080` | `ARCHIVED` | Stash snapshot; recovery-only documentation work. |
| `89ba4d7793f64d14b02322106764c99c85b7fe5e` | `ARCHIVED` | Stash snapshot; recovery-only SST work. |
| `94597d313fdfe4e8a86b1012d87f432bef8a3531` | `ARCHIVED` | Parked notification WIP stash snapshot. |
| `9db442e1a7f93bd70af9943726ec4716bf70f47f` | `ARCHIVED` | Stash untracked scenario-seam snapshot. |
| `9ea7fefcd5bd82ccd153328f0f6ad3c42d2027ac` | `ARCHIVED` | Stash untracked spec-WIP snapshot. |
| `a5d1bb0a536587419c5c369985c7fa8a416f72f0` | `ARCHIVED` | Stash snapshot; recovery-only config WIP. |
| `ac120c8c2b9825446b57c7eed578084f65125b6d` | `ARCHIVED` | Stash snapshot; recovery-only spec WIP. |
| `b07649af11f14c82dde60aedb538ab6c74af8475` | `ARCHIVED` | Stash untracked readiness-document snapshot. |
| `b18bef747994e7466b06b4fb84ac95aba97e1704` | `REPRESENTED` | Malformed-response capture fix is reachable as `0a24f5cd0f8c648232230d14f8474898ea254cca`. |
| `b63dd09796871771f7f8010d448960fb3ed49706` | `ARCHIVED` | Stash untracked spec-WIP snapshot. |
| `b85d37cc1044820f31ad810490abd06779853723` | `ARCHIVED` | Stash snapshot; recovery-only vulnerability-gate work. |
| `c2260fff2162530e983f9f2db0befce76726f895` | `ARCHIVED` | Stash snapshot; recovery-only promotion work. |
| `d15da76f3b80baa06d5ecc8b44c43a657441215e` | `ARCHIVED` | Stash snapshot; no standalone file delta. |
| `e0d57e257ee254988d5b3491c7b0cb37431adecd` | `ARCHIVED` | Stash snapshot; recovery-only isolation work. |
| `e5e8b591e65bebb114abc5c94cfcf1ad40022f94` | `ARCHIVED` | Stash snapshot; recovery-only vulnerability-gate work. |
| `f5fdca43a8f30fa3e16a0e7d9a03044a8dd2f7c4` | `ARCHIVED` | Stash snapshot; recovery-only spec WIP. |
| `fa75c312fe650d2f64d266c3bb978e6c104fbcfa` | `ARCHIVED` | Stash untracked framework-refresh snapshot. |
| `fba3a25c6014b0438bec63e2bc2781a71c9ae3fd` | `ARCHIVED` | Stash snapshot; recovery-only test-stub work. |
| `fc9cd7ba43cf76b929a0d97c582d54def6e94646` | `REPRESENTED` | Later reachable certification history represents this packet snapshot. |

Disposition totals are `ARCHIVED=39`, `REPRESENTED=2`, `SUPERSEDED=1`, and
`ROUTE_REQUIRED=0`. No protected object contains unique product source that
requires integration.

**Claim Source:** interpreted semantic review plus executed preservation,
counterpart, and classification-set checks. See
`report.md#unreachable-preservation-and-classification`.

### Tag Dispositions

| Local tag | Final class |
|---|---|
| `archive/bug-spec069-BUG-069-005-IN-PROGRESS` | `REPRESENTED` |
| `archive/feat-spec-021-064-084-edits` | `REPRESENTED` |
| `archive/feat-spec-021-staged-edits` | `SUPERSEDED` |
| `archive/parking-041-scope2-QF063-DONE-READY-TO-COMPLETE` | `SUPERSEDED` |
| `archive/release-deploy-20260721-dup` | `REPRESENTED` |
| `archive/spec-079-wip-20260727` | `ROUTE_REQUIRED` |
| `archive/stash-preserve-20260614-spec083-wip` | `SUPERSEDED` |
| `archive/stash-reconcile-20260612` | `SUPERSEDED` |
| `contributor-scrub-backup-pre-20260727` | `ARCHIVED` |
| `pii-scrub-backup-pre-20260614` | `ARCHIVED` |
| `pii-scrub-backup-pre2-20260614` | `ARCHIVED` |

All 11 tags resolve and occur in the verified recovery bundle. Never merge any
pre-scrub tag. It may contain historical identity material and exists only for
recovery or audit.

**Claim Source:** interpreted classification plus executed tag and bundle
checks. See `report.md#tag-verification`.

### Tagged Handoff Guidance

The unique handoff blob is
`26359fdd5cebf01343d271bba6d35b00fdc81751`. Its final disposition is
`ARCHIVED/ROUTE_REQUIRED`.

- Route `BUG-083-002` to `bubbles.plan` for scope-DAG replanning.
- Keep Spec 105 gated on `BUG-080-001`.
- Do not import the historical handoff file.

**Claim Source:** executed marker verification plus interpreted routing. See
`report.md#tagged-handoff-verification`.

### Ignored Python Dispositions

Spec 040 excludes provider migration. No ignored migration utility enters the
product repository under that feature.

| Ignored file | SHA-256 | Final security disposition |
|---|---|---|
| `.copilot-temp/flickr_pipeline_reference.py` | `72b6971c85554f64b1819caad00d2a877596ac5a3ff9558877943af18bafe15c` | `ARCHIVED` operator reference |
| `.copilot-temp/shutterfly_archive_probe.py` | `2b6188c9984098b228b5199caa63a0b66a8184e1381aaaba9c7f9bbe053c43f3` | `SUPERSEDED` after backup |
| `.copilot-temp/shutterfly_immich_pipeline.py` | `f495a1c4eaeff54b78dc852b6674ec7381987d775e9dd757975b8abd92b4fe3b` | `ARCHIVED` operator reference |
| `.copilot-temp/shutterfly_probe_download.py` | `da4fce54023ac3fc5daae3e800fd1adc7fa580e6a99d062bd25a07da0b381e42` | `SUPERSEDED` after backup |
| `.copilot-temp/shutterfly_probe_download_v2.py` | `d1b22bda608a2a247ecf160cafeacfee4adc7b2b6f457e15e8cc4ba88e537fa9` | `SUPERSEDED` after backup |
| `.copilot-temp/shutterfly_probe_download_v3.py` | `4479720a6b55be1b35a58e2f093ef041797708d099ddf8fd104d8df1b3bf9eb9` | `DISCARDED` after backup |
| `.copilot-temp/shutterfly_probe_download_v4.py` | `08c93b96b22c33eaa54fd279f5446d5f262fae6b119a45724942873ca4b07bf9` | `DISCARDED`; never integrate because TLS verification is disabled |

The Flickr reference and robust Shutterfly pipeline remain operator references
only. Reviving either requires a separately planned, hardened capability.

**Claim Source:** executed hashes and TLS predicate plus interpreted security
classification. See `report.md#temporary-python-security-dispositions`.

### Other Ignored-Work Dispositions

The 67-entry archive has a closed classification with zero unclassified paths.

| Category | Count | Final disposition |
|---|---:|---|
| Temporary Python files | 7 | Per-file dispositions above |
| Local environment and generated config | 5 | `DISCARDED` after backup because concrete values are already knb-owned |
| Build and release output, including `dist` | 33 | `ARCHIVED` output, then disposable locally |
| Stale runtime and session files | 22 | `DISCARDED` after backup |

No ignored path is deleted by this update.

**Claim Source:** executed archive-manifest classification. See
`report.md#ignored-archive-completeness`.

### Completed Unreachable Classification

The reconstructed pre-protection inventory contains 175 commits not reachable
from `main`. Comparison against latest `origin/main` classifies 29 by exact tree,
56 by stable patch, and 48 by exact subject plus changed-path set. The remaining
42 commits exactly match the 42 protected unreachable refs listed above.

Semantic review closes that remaining set at `ARCHIVED=39`, `REPRESENTED=2`,
`SUPERSEDED=1`, and `ROUTE_REQUIRED=0`. No object remains unclassified, and no
unique product source requires integration.

**Claim Source:** executed mechanical reconstruction and preservation checks,
plus interpreted semantic review. See
`report.md#unreachable-preservation-and-classification`.

### Mode-Aware Nonterminal Handoff

The committed helper reports exactly 27 nonterminal paths. The genericization
repair discovered four further open items that the helper cannot see because
they carry no `state.json`. Scope 3 execution added one further row for the
validation chain that was not completed before the push. This table therefore
records 32 paths. The first 27 rows keep the helper's path order. The four
discovered rows are appended next and sorted among themselves. The Scope 3
validation-chain row is appended last. This table records current state without
mutating any existing packet.

| Path | Status | Mode | Current owner | Phase | Exact safe next action |
|---|---|---|---|---|---|
| `specs/002-phase1-foundation/bugs/BUG-002-006-search-htmx-sri-blocks-submit` | `blocked` | `bugfix-fastlane` | `bubbles.implement` | `implement` | Operator vendors source-locked HTMX bytes, then `bubbles.implement` resumes `SCOPE-01`. |
| `specs/002-phase1-foundation/bugs/BUG-002-007-digest-date-scan-false-empty` | `blocked` | `bugfix-fastlane` | `bubbles.implement` | `implement` | Obtain the owner decision on digest grant enforcement, then resume the affected scope under that decision. |
| `specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count` | `in_progress` | `bugfix-fastlane` | `bubbles.test` | `test` | Route the recorded audit rework to `bubbles.implement` before validation. |
| `specs/004-phase3-intelligence/bugs/BUG-004-004-synthesis-persistence-and-health-truth` | `blocked` | `bugfix-fastlane` | `bubbles.implement` | `implement` | Implement `SCOPE-01` durable synthesis persistence before later scopes. |
| `specs/032-documentation-freshness/bugs/BUG-004-production-readiness-claims-runtime-drift` | `in_progress` | `bugfix-fastlane` | `bubbles.design` | `bootstrap` | Route `SCOPE-01` capability-catalog implementation through `bugfix-fastlane`. |
| `specs/039-recommendations-engine/bugs/BUG-039-005-enabled-with-zero-providers-false-ready` | `blocked` | `bugfix-fastlane` | `bubbles.implement` | `implement` | Implement `SCOPE-01` provider-contract foundation before adapter work. |
| `specs/058-chrome-extension-bridge/bugs/BUG-058-EXTERNAL-INFRA-MISSING` | `open` | `bugfix-fastlane` | `unassigned` | `validate` | Route to `bubbles.plan` to add the missing standard packet scaffolding before terminal validation. |
| `specs/058-chrome-extension-bridge` | `blocked` | `full-delivery` | `unassigned` | `implement` | Run a tagged CI release and verify keyless OIDC and Rekor identity binding. |
| `specs/061-conversational-assistant/bugs/BUG-061-006-duplicate-contradictory-capture-ack` | `in_progress` | `bugfix-fastlane` | `bubbles.goal` | `unassigned` | Operator runs the two Telegram acknowledgement smoke cases, then routes certification. |
| `specs/061-conversational-assistant/bugs/BUG-061-007-weather-shortcut-masked-as-saved-as-idea` | `in_progress` | `bugfix-fastlane` | `bubbles.goal` | `unassigned` | Operator runs the Telegram weather smoke case, then routes certification. |
| `specs/061-conversational-assistant/bugs/BUG-061-008-execution-errors-masked-as-saved-as-idea` | `in_progress` | `bugfix-fastlane` | `bubbles.goal` | `unassigned` | Operator confirms a failed Telegram scenario renders an honest error. |
| `specs/061-conversational-assistant/bugs/BUG-061-009-high-band-refusal-masked-as-saved-as-idea` | `blocked` | `bugfix-fastlane` | `bubbles.goal` | `unassigned` | Operator compares an ungrounded `/ask` refusal with a genuine low-band capture. |
| `specs/061-conversational-assistant/bugs/BUG-061-010-open-knowledge-grounding-gap` | `blocked` | `bugfix-fastlane` | `bubbles.goal` | `unassigned` | Operator performs the shared live `/ask` deployment confirmation, then routes certification. |
| `specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup` | `in_progress` | `bugfix-fastlane` | `bubbles.bug` | `governance-reconciliation` | `bubbles.audit` appends candidate-attributed G053 and G093 diff evidence. |
| `specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green` | `in_progress` | `bugfix-fastlane` | `bubbles.implement` | `implement` | `bubbles.plan` reconciles concrete paths before the separate upstream guard route continues. |
| `specs/070-web-username-password-login/bugs/BUG-070-001-production-credential-session-paseto-split` | `blocked` | `bugfix-fastlane` | `bubbles.implement` | `implement` | Start `SCOPE-01` bound browser-account policy and role/grant work. |
| `specs/073-web-mobile-assistant-frontend/bugs/BUG-073-006-auth-rejection-blank-assistant-response` | `in_progress` | `bugfix-fastlane` | `bubbles.plan` | `plan` | `bubbles.implement` consumes the BUG-102 fault profiles in `SCOPE-01`. |
| `specs/080-knowledge-graph-public-api/bugs/BUG-080-001-graph-api-fail-soft-runtime-disable` | `blocked` | `bugfix-fastlane` | `bubbles.implement` | `implement` | Resume `SCOPE-03` product-read synthetic and readiness truth. |
| `specs/083-card-rewards-companion/bugs/BUG-083-002-ccmanager-parity-runtime-drift` | `in_progress` | `bugfix-fastlane` | `bubbles.design` | `bootstrap` | `bubbles.plan` replans the scope DAG before any implementation pickup. |
| `specs/096-multi-provider-model-connections/bugs/BUG-096-001-discovery-probe-compose-dns` | `blocked` | `bugfix-fastlane` | `bubbles.devops` | `implement` | On a stable build host, deploy the fix through the adapter and probe model discovery. |
| `specs/096-multi-provider-model-connections` | `blocked` | `full-delivery` | `bubbles.implement` | `implement` | `bubbles.devops` runs the dependency deployment handoff and Spec 096 C7 live legs. |
| `specs/102-target-deploy-hardening/bugs/BUG-102-001-product-journey-acceptance-gap` | `blocked` | `bugfix-fastlane` | `bubbles.implement` | `implement` | `bubbles.devops` provisions an off-traffic candidate for `SCOPE-02` acceptance. |
| `specs/104-universal-ask-self-knowledge` | `blocked` | `full-delivery` | `bubbles.goal` | `unassigned` | Operator performs the final live Telegram self-knowledge smoke test. |
| `specs/105-connected-knowledge-graph-explorer` | `in_progress` | `full-delivery` | `bubbles.plan` | `harden` | Keep gated until `BUG-080-001` is complete, then pick up `SCOPE-01`. |
| `specs/106-coherent-product-experience` | `in_progress` | `full-delivery` | `bubbles.implement` | `implement` | Finish and evidence `SCOPE-106-01` before advancing shell cutover. |
| `specs/107-proactive-correlated-experience` | `blocked` | `full-delivery` | `bubbles.implement` | `implement` | After Specs 105 and 106 ship, resume `SCOPE-03B2`. |
| `specs/_ops/OPS-006-local-git-reconciliation` | `in_progress` | `stabilize-to-doc` | `bubbles.test` | `test` | Commit, rebase, push, and cleanup are already executed. Complete the outstanding validation chain in the row below, then re-verify both recovery artifacts after cleanup and resolve the two `unassigned` owners before any terminal claim. |
| `cmd/config-validate :: TestRun_OversizedModel_ExitsOne` | `open` | `unassigned` | `bubbles.test` | `test` | Under Spec 045, extend the fixture env-override list to cover the `ASSISTANT_OPEN_KNOWLEDGE_*` model keys, or add those models to the fixture profile set. |
| `internal/assistant :: TestFacadeResolvedCompiledWeatherSourceFailuresCaptureSafely` | `open` | `unassigned` | `bubbles.test` | `test` | With `bubbles.plan` under the BUG-061-008 and BUG-061-009 lineage, reconcile the stale pre-honesty expectation in `assertCompiledWeatherCapture` without weakening the honesty behavior. |
| `specs/031-live-stack-testing` | `open` | `unassigned` | `bubbles.workflow` | `validate` | With `bubbles.validate`, record the missing `gaps` and `harden` phase records, then rerun artifact lint. |
| `specs/069-assistant-http-transport` | `open` | `unassigned` | `bubbles.workflow` | `validate` | With `bubbles.validate`, record the missing `gaps` and `harden` phase records and repair the two `report.md` evidence blocks, then rerun artifact lint. |
| `specs/_ops/OPS-006-local-git-reconciliation :: unrun validation chain` | `open` | `stabilize-to-doc` | `bubbles.test` | `test` | Run, in this order, `./smackerel.sh lint`, `./smackerel.sh test unit --python`, `./smackerel.sh build`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, `./smackerel.sh test stress`, and `bash .github/bubbles/scripts/cli.sh framework-validate`, each with full unfiltered output, then record the results against the Scope 2 and Scope 3 test-evidence items. `./smackerel.sh test unit --go` must still exit `1` on exactly the two documented pre-existing failures and on nothing else. |

**Claim Source:** executed mode-aware inventory for the 27 helper-derived rows,
executed repair verification for the four discovered rows, plus executed Scope 3
validation-gap observation for the final row, plus interpreted state-bound
actions. See `report.md#mode-aware-nonterminal-inventory`,
`report.md#genericization-repair-verification`, and
`report.md#integration-push-cleanup-and-final-invariants`.

The validation-chain row is scoped to OPS-006 and is not a helper-visible path,
so rerunning the mode-aware helper will continue to report 27. The Scope 2
diff, ledger, and handoff completeness check must be rerun against a refreshed
inventory because this row changes the table.

### Discovered Open Work Detail

These four items were found while repairing genericity. None of them is caused
by the repair. Each was verified against `HEAD` before being recorded.

#### `cmd/config-validate :: TestRun_OversizedModel_ExitsOne`

The failure is pre-existing at `HEAD`. `cmd/config-validate/`, `internal/config/`,
and the tracked `config/` tree are all byte-identical to `HEAD`. The only
modified non-test Go files are
`internal/agent/tools/microtools/location_normalize.go` and
`internal/agent/tools/microtools/location_normalize_preprocess.go`, and neither
is imported by `cmd/config-validate`.

The binary does exit `1`, but for a different reason than the test asserts, so
the three expected stderr substrings are absent. Actual stderr reports
`model envelope validation failed (spec 045 FR-045-002): missing model memory
profile(s)` for `assistant.open_knowledge.switchable_models`,
`assistant.open_knowledge.synthesis_model_id`, and
`assistant.open_knowledge.tool_capable_gather_models`.

The root cause is fixture drift. The fixture's env-key override list predates
the `assistant.open_knowledge.*` model keys, so validation aborts on the
missing-profile check before it reaches the oversize check.

#### `internal/assistant :: TestFacadeResolvedCompiledWeatherSourceFailuresCaptureSafely`

The failure is pre-existing at `HEAD`. All 42 depth-1 files of package
`assistant` are byte-identical to `HEAD`. The imported `contracts` and
`agent/tools/weather` packages are also identical. The test injects its own
resolver and forecast closures.

Both subtests fail, at `compiled_weather_test.go:112` and
`compiled_weather_test.go:142`, with failure status and capture reported as
`"unavailable"`/`false` where the test wants `saved_as_idea`/`true`. The run log
shows `scenario_id=weather_query band=high status=unavailable
error_cause=no_grounded_answer`.

The root cause is a stale test. The helper `assertCompiledWeatherCapture` still
encodes the pre-honesty-fix contract. Shipped behavior now follows the ratified
Assistant Response Honesty invariant recorded in BUG-061-008, BUG-061-009, and
`.github/copilot-instructions.md`, under which a band-high turn must never
render as "saved as an idea". The test is stale, not the code.

Reconcile the stale expectation deliberately. This item MUST NOT be resolved by
weakening the honesty behavior.

#### `specs/069-assistant-http-transport`

Artifact lint fails with 7 issues, and the failure is pre-existing at `HEAD`.
All four lint inputs, `state.json`, `report.md`, `scopes.md`, and
`uservalidation.md`, are byte-identical to `HEAD`. Only `design.md` was edited
during genericization, and artifact lint does not read it for these checks.

Gate G022 reports `gaps` and `harden` absent from the phase records. Two
evidence blocks in `report.md` also lack terminal-output signals.

#### `specs/031-live-stack-testing`

Artifact lint fails with 5 issues, and the failure is pre-existing at `HEAD`.
All four lint inputs are byte-identical to `HEAD`. Only a file under
`bugs/BUG-031-008-integration-job-stabilization/` was edited during
genericization.

Gate G022 reports the same missing `gaps` and `harden` phase records.

**Claim Source:** executed repair verification plus interpreted root-cause
review. See `report.md#genericization-repair-verification`.

## Entry Conditions

1. Obtain a fresh actionable binding for the Smackerel repository root.
2. Confirm the active agent owns operational Git reconciliation.
3. Run from the repository root.
4. Confirm no other process is changing this checkout.
5. Set an explicit secure backup directory outside the repository.
6. Keep the backup directory private and excluded from synchronization services.

If current `origin/main` moved beyond `29e46260`, record the new base. Rebase
the reconciliation work onto that base before integration.

Never reset a newer remote state back to the supplied inventory baseline.

Load the repository's portable timeout helper once before running commands:

```bash
source .github/bubbles/scripts/guard-lib.sh
```

## Non-Negotiable Safety Rules

- Back up first. Review second. Integrate third. Delete last.
- Never delete a tag or recovery ref before its disposition is pushed.
- Never remove an ignored file before reviewing its content and provenance.
- Never run `git clean -fdx`, `git gc`, `git prune`, or `git reflog expire`.
- Never force-push.
- Never display a secret-bearing file or environment value in terminal output.
- Never paste unredacted candidate content into chat, evidence, or this runbook.
- Never weaken tests to admit recovered code.
- Never edit foreign-owned spec, scope, or certification fields.
- Use the Smackerel CLI for runtime validation.

## Disposition Classes

Use exactly one class for every inventoried item.

| Class | Meaning | Required result |
|---|---|---|
| `INTEGRATED` | Unique work remains valid | Traceable commit on pushed `main` |
| `REPRESENTED` | Current `main` already carries the same intent | Equivalence evidence and pushed disposition |
| `SUPERSEDED` | A newer implementation or decision replaces the item | Superseding path or commit and pushed reason |
| `ARCHIVED` | The item has durable historical value but no runtime role | Verified secure backup and pushed recovery metadata |
| `DISCARDED` | The item is generated, stale, or invalid | Semantic reason, recovery proof, and pushed disposition |
| `ROUTE_REQUIRED` | Work remains valid but another owner must act | Next owner, exact action, and preserved source |

Do not classify an item from its filename, timestamp, or commit subject alone.

## Closed Review Ledger

Every intake row below has a final disposition. No active row carries an
unresolved state. Evidence records omit secret and PII values.

### Dangling Commits

| Object | Required evidence | Final disposition |
|---|---|---|
| `a1ed9f91` | Commit metadata, changed paths, parent graph, semantic diff, main overlap | `REPRESENTED` by `0aa7c315` |
| `15ee91c2` | Commit metadata, changed paths, parent graph, semantic diff, main overlap | `REPRESENTED` by `4f99827e` |
| `afeedf7b` | Commit metadata, changed paths, parent graph, semantic diff, main overlap | `REPRESENTED` by `49a05656` |
| `e2f0b997` | Commit metadata, changed paths, parent graph, semantic diff, main overlap | `REPRESENTED` by `2351ed76` |
| `1ef1cd24` | Commit metadata, changed paths, parent graph, semantic diff, main overlap | `REPRESENTED` by `e1924a90` |
| `58f3655e` | Commit metadata, changed paths, parent graph, semantic diff, main overlap | `REPRESENTED` by `d2691fba` and `216cec66` |
| `73f3434d` | Commit metadata, changed paths, parent graph, semantic diff, main overlap | `REPRESENTED` by `116dae3a` |
| `e4fb11ae` | Commit metadata, changed paths, parent graph, semantic diff, main overlap | `REPRESENTED` by `732eafbc` |
| `34fd9bae` | Commit metadata, changed paths, parent graph, semantic diff, main overlap | `REPRESENTED` by `ac1cf705` |

### Tags And Tagged-Only Content

| Item set | Required evidence | Final disposition |
|---|---|---|
| 11 local-only archive or backup tags | Exact names, target objects, unique reachable commits, semantic purpose | Final classes recorded in `Tag Dispositions` |
| `HANDOFF-second-brain-coordinator.md` | Owning tag, content review, current relevance, superseding artifact | `ARCHIVED/ROUTE_REQUIRED`; do not import |

### Ignored Work

| Item set | Required evidence | Final disposition |
|---|---|---|
| Seven `.copilot-temp` Python files | Exact paths, source comparison, tests, secret and PII scan | Per-file classes recorded in `Ignored Python Dispositions` |
| Ignored `dist` artifacts | Exact paths, generating source, reproducibility through repo CLI | `ARCHIVED`, then disposable locally |
| Stale session files | Exact paths, owning tool, current references, safe removal method | `DISCARDED` after backup |
| Stale runtime files | Exact paths, active process check, generating command, recovery need | `DISCARDED` after backup |

## Phase 1: Revalidate The Inventory

Fetch remote state without fetching tags. Then capture the complete local
inventory before creating recovery refs.

```bash
bubbles_run_with_timeout 120 git fetch --prune --no-tags origin
bubbles_run_with_timeout 30 git status --short --branch
bubbles_run_with_timeout 30 git rev-parse main
bubbles_run_with_timeout 30 git rev-parse origin/main
bubbles_run_with_timeout 30 git for-each-ref --format='%(refname) %(objectname)' refs/heads refs/tags
bubbles_run_with_timeout 30 git worktree list --porcelain
bubbles_run_with_timeout 30 git stash list
bubbles_run_with_timeout 30 git fsck --full --no-reflogs --unreachable
bubbles_run_with_timeout 30 git ls-files --others --ignored --exclude-standard
```

Record all output needed for later comparison. Do not truncate or filter
command output.

Stop if tracked files are dirty, an unknown worktree exists, or an unknown
branch or stash appears. Add each new item to the review ledger first.

Confirm every supplied dangling ID still resolves as a commit:

```bash
bubbles_run_with_timeout 30 git cat-file -e 'a1ed9f91^{commit}'
bubbles_run_with_timeout 30 git cat-file -e '15ee91c2^{commit}'
bubbles_run_with_timeout 30 git cat-file -e 'afeedf7b^{commit}'
bubbles_run_with_timeout 30 git cat-file -e 'e2f0b997^{commit}'
bubbles_run_with_timeout 30 git cat-file -e '1ef1cd24^{commit}'
bubbles_run_with_timeout 30 git cat-file -e '58f3655e^{commit}'
bubbles_run_with_timeout 30 git cat-file -e '73f3434d^{commit}'
bubbles_run_with_timeout 30 git cat-file -e 'e4fb11ae^{commit}'
bubbles_run_with_timeout 30 git cat-file -e '34fd9bae^{commit}'
```

## Phase 2: Create Verified Recovery Material

Set `OPS006_BACKUP_DIR` explicitly. Keep it outside every workspace repository.

```bash
: "${OPS006_BACKUP_DIR:?Set OPS006_BACKUP_DIR to a secure directory outside the repository}"
umask 077
bubbles_run_with_timeout 30 mkdir -p "$OPS006_BACKUP_DIR"
```

Create temporary refs before any operation that could make dangling commits
harder to recover.

```bash
bubbles_run_with_timeout 30 git update-ref refs/ops/OPS-006/dangling/a1ed9f91 a1ed9f91
bubbles_run_with_timeout 30 git update-ref refs/ops/OPS-006/dangling/15ee91c2 15ee91c2
bubbles_run_with_timeout 30 git update-ref refs/ops/OPS-006/dangling/afeedf7b afeedf7b
bubbles_run_with_timeout 30 git update-ref refs/ops/OPS-006/dangling/e2f0b997 e2f0b997
bubbles_run_with_timeout 30 git update-ref refs/ops/OPS-006/dangling/1ef1cd24 1ef1cd24
bubbles_run_with_timeout 30 git update-ref refs/ops/OPS-006/dangling/58f3655e 58f3655e
bubbles_run_with_timeout 30 git update-ref refs/ops/OPS-006/dangling/73f3434d 73f3434d
bubbles_run_with_timeout 30 git update-ref refs/ops/OPS-006/dangling/e4fb11ae e4fb11ae
bubbles_run_with_timeout 30 git update-ref refs/ops/OPS-006/dangling/34fd9bae 34fd9bae
```

Create a bundle containing all current refs, including the temporary recovery
refs and local tags.

```bash
bubbles_run_with_timeout 300 git bundle create "$OPS006_BACKUP_DIR/smackerel-OPS-006.bundle" --all
bubbles_run_with_timeout 120 git bundle verify "$OPS006_BACKUP_DIR/smackerel-OPS-006.bundle"
bubbles_run_with_timeout 30 git bundle list-heads "$OPS006_BACKUP_DIR/smackerel-OPS-006.bundle"
bubbles_run_with_timeout 30 openssl dgst -sha256 "$OPS006_BACKUP_DIR/smackerel-OPS-006.bundle"
```

Record the bundle path, SHA-256, creation time, and access owner. Record no
secret content.

Inventory exact ignored paths before copying them. Review names before opening
any file body.

Copy the explicit reviewed path list into a private archive. Pass each exact
path to `tar -czf` as a separate argument.

Do not use a glob, the repository root, or `git clean` as a substitute. Then
verify the archive without displaying file contents:

```bash
bubbles_run_with_timeout 30 tar -tzf "$OPS006_BACKUP_DIR/smackerel-OPS-006-ignored.tar.gz"
bubbles_run_with_timeout 30 openssl dgst -sha256 "$OPS006_BACKUP_DIR/smackerel-OPS-006-ignored.tar.gz"
```

Stop if either archive fails verification. Do not change refs or ignored files
after a failed backup.

## Phase 3: Apply Secret And PII Safeguards

Scan candidate paths before staging or displaying their contents. Use the
repository's committed lint and secret controls.

Do not use `cat`, unrestricted `git show`, `env`, `printenv`, or shell tracing
on a suspected secret-bearing path.

Review sensitive content locally with the smallest necessary view. Record only
paths, finding classes, and redacted remediation notes.

If a secret is present:

1. Stop integration.
2. Preserve the file only in the protected backup.
3. Notify the operator without quoting the value.
4. Rotate or revoke the credential.
5. Remove the value from any candidate commit.
6. Re-run the repository secret checks.

If PII is present, keep it out of the generic product repository. Record a
redacted disposition and route environment-specific material to its owner.

## Phase 4: Review Tags And Tagged-Only Content

Review all 11 tag names, target objects, and reachable commits. Do not infer a
tag's value from an `archive` or `backup` prefix.

For each tag:

1. Record the full tag name and object ID.
2. Determine whether it is annotated or lightweight.
3. List commits reachable from the tag but not from current `main`.
4. Compare changed paths and behavior with current `main`.
5. Assign one disposition class.
6. Record the integrating or superseding commit when applicable.

Review `HANDOFF-second-brain-coordinator.md` before deleting its owning tag.

If its guidance remains actionable, transfer the action into the next-session
handoff table with provenance. Integrate the file itself only when its location
and ownership remain current.

If newer tracked artifacts supersede it, record those paths and the semantic
reason. The verified bundle remains the historical recovery source.

## Phase 5: Review Dangling Commits

Review parents before children. Preserve dependency order when several commits
form one change series.

For each supplied commit:

1. Inspect commit metadata without exposing sensitive values.
2. Inventory changed paths.
3. Identify its parent graph and related supplied commits.
4. Compare its semantic behavior with current `main`.
5. Check current specs, tests, and architecture for continuing validity.
6. Assign one disposition class.
7. Record exact evidence in the review ledger.

Use `INTEGRATED` only when the behavior remains valid and tested. Use
`REPRESENTED` only with concrete current-main evidence.

Use `SUPERSEDED` only when a named tracked artifact replaces the work. Use
`DISCARDED` only after recording why the work is invalid or generated.

Preserve original provenance when integrating a commit. Prefer a traceable
cherry-pick with origin metadata over copying code by hand.

## Phase 6: Review Ignored Work

### `.copilot-temp` Python Files

Review all seven files separately. Compare each file with tracked source and
tests.

Integrate valid unique behavior into the correct owned module. Add tests that
prove the behavior before deleting the temporary copy.

Classify a file as discarded only when it is generated, duplicate, invalid, or
superseded. Record the exact reason and recovery archive digest.

### `dist` Artifacts

Identify the tracked source and repository command that generates each
artifact. Rebuild through `./smackerel.sh` before claiming reproducibility.

Compare behavior or checksums as appropriate. Remove the ignored artifact only
after confirming that tracked sources reproduce it.

### Session And Runtime Residue

Identify the owning process or committed lifecycle command for every file.
Confirm no active process still uses it.

Use the owning tool's cleanup path when one exists. Do not hand-edit
framework-managed session state.

Remove residue by explicit path only after review. Never use a broad clean
command.

## Phase 7: Preserve Nonterminal Work

Inventory every nonterminal spec and bug already tracked on `main`. Evaluate
terminal state with the committed mode-aware helper.

Do not change execution or certification state during Git reconciliation.
Route stale or contradictory artifacts to their owning agent.

Complete this table before integration:

| Path | Current status | Workflow mode | Next owner | Exact next action |
|---|---|---|---|---|
| See `Mode-Aware Nonterminal Handoff` | 32 current paths | Recorded per path | Recorded per path | Recorded per path |

Include valid actions recovered from tagged handoff content. Cite the source tag
or bundle object ID without copying sensitive text.

## Phase 8: Integrate On A Temporary Branch

Create one temporary branch from the current remote base. Do not work from the
old `29e46260` baseline when `origin/main` has advanced.

```bash
bubbles_run_with_timeout 30 git switch --create ops/OPS-006-reconcile origin/main
```

Integrate approved commits in dependency order. Use `-x` when cherry-picking a
recovered commit.

Move approved ignored work into its correct tracked ownership boundary. Keep
generated outputs ignored.

Update every review ledger row and the next-session handoff. Include recovery
hashes and integrating commits.

Commit only coherent reviewed units. The final packet update must contain all
durable dispositions.

## Phase 9: Validate Through The Repository CLI

Run the narrowest relevant command after each integration. Run this complete
chain before advancing `main`:

```bash
bubbles_run_with_timeout 60 ./smackerel.sh config generate
bubbles_run_with_timeout 120 ./smackerel.sh check
bubbles_run_with_timeout 600 ./smackerel.sh lint
bubbles_run_with_timeout 600 ./smackerel.sh format --check
bubbles_run_with_timeout 600 ./smackerel.sh test unit --go
bubbles_run_with_timeout 600 ./smackerel.sh test unit --python
bubbles_run_with_timeout 600 ./smackerel.sh test integration
bubbles_run_with_timeout 900 ./smackerel.sh test e2e
bubbles_run_with_timeout 600 ./smackerel.sh test stress
bubbles_run_with_timeout 1200 ./smackerel.sh build
bubbles_run_with_timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate
bubbles_run_with_timeout 30 git diff --check origin/main...HEAD
```

Capture full output and exit codes. Do not claim success from inspection or
from an earlier session.

If recovered work changes a test category, add and run that category before
continuing. A failure blocks integration until its cause is resolved.

## Phase 10: Reconcile Remote Movement

Fetch remote state again before advancing local `main`.

```bash
bubbles_run_with_timeout 120 git fetch --prune --no-tags origin
bubbles_run_with_timeout 30 git rev-parse origin/main
```

If `origin/main` changed, rebase the temporary branch onto the new base. Review
every conflict semantically, then rerun the full validation chain.

Never overwrite a newer remote commit. Never use a force-push.

## Phase 11: Advance Main And Push

Confirm the temporary branch is based on current `origin/main`. Then advance
`main` by fast-forward only.

```bash
bubbles_run_with_timeout 30 git switch main
bubbles_run_with_timeout 30 git merge --ff-only ops/OPS-006-reconcile
bubbles_run_with_timeout 30 git status --short --branch
bubbles_run_with_timeout 120 git push --porcelain origin main:main
```

Do not push local archive tags. Do not use `--tags` or `--force`.

If the push is rejected, fetch remote state and recreate the temporary branch
from the new `origin/main`. Revalidate after reconciliation.

Keep the temporary branch and all recovery refs until push verification passes.

## Phase 12: Verify The Push

Verify local and remote identity after the push.

```bash
bubbles_run_with_timeout 120 git fetch --prune --no-tags origin
bubbles_run_with_timeout 30 git rev-parse main
bubbles_run_with_timeout 30 git rev-parse origin/main
bubbles_run_with_timeout 30 git ls-remote --heads origin refs/heads/main
bubbles_run_with_timeout 30 git status --short --branch
```

The three reported main commit IDs must match. Record the pushed commit ID and
the full validation command outcomes.

## Phase 13: Remove Reviewed Local Residue

> **ALREADY EXECUTED ON 2026-08-03. DO NOT RERUN THIS PHASE.**
>
> This phase completed after the verified push of
> `8a4e553d2b41bfc63bf82cb34ddb8423025fcb1a`. Both protected recovery artifacts
> were re-verified immediately BEFORE cleanup and are deliberately RETAINED, not
> deleted. Exactly 51 refs under `refs/ops/OPS-006/` were deleted, being 9
> dangling and 42 unreachable recovery refs, and exactly 11 local tags were
> deleted, being the 8 `archive/*` tags plus
> `contributor-scrub-backup-pre-20260727`,
> `pii-scrub-backup-pre-20260614`, and `pii-scrub-backup-pre2-20260614`. Every
> deletion named its full ref or tag. No wildcard, no `git gc`, no `git prune`,
> and no reflog expiry was used.
>
> The repository now reports zero `refs/ops` refs and zero local tags, so
> rerunning the commands below would fail on missing refs and would prove
> nothing. A future session MUST NOT re-create or re-delete any of these refs or
> tags. The commands are kept verbatim as the executed record. See
> `report.md#integration-push-cleanup-and-final-invariants`.

Do not begin this phase until every disposition exists on verified
`origin/main`. Both recovery archives must also verify successfully.

Delete only the exact 11 reviewed local tag names. Do not use a wildcard that
could delete a newly fetched or newly created tag.

Delete each explicit ignored path only after its ledger row is complete. Review
any ignored output created by validation before removing it.

Delete the nine dangling and 42 unreachable temporary recovery refs only after
the bundle lists each one. Their ledger rows must also exist on verified
`origin/main`.

```bash
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/dangling/a1ed9f91
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/dangling/15ee91c2
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/dangling/afeedf7b
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/dangling/e2f0b997
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/dangling/1ef1cd24
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/dangling/58f3655e
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/dangling/73f3434d
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/dangling/e4fb11ae
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/dangling/34fd9bae
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/094dd1fd087d6f4857ca09e3087af7630e6759e7
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/09b22e558b2d6a2e5e7a84bd6c8c59c0f3ee0456
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/130fe2908e8a46ca02422f06da89111869b9a8cd
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/134b32e56d60ecd617cdec5357320e91bb1a5cfe
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/13cba34b2aeeb6490eeeac58cacfe6506aa98c70
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/14775e35c52e96c1175deb7f8176e5e060d84dca
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/1536947312085c1ef2286b5cb658d2674e04f085
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/1c9dc462206af6fc32ec8ee3b67e80e07415c126
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/220c9359fa32625d6bb284e46074a34d5a3cd21f
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/22174c97003252dbec68b6ece33f8df61ae46f62
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/315f0253524cabff1f3cc6cb0ab04876ded592ff
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/323a3a3546516f2d05ff95fcf39b8dbb9b5b46f7
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/40fade815ef03e1cd44edd54e28e5a9c7a93f240
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/4736286db4da7082aa85318e1557f591fe7e2b7a
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/4c1b204385a4ea01b825f6c185b781c87b42d868
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/529374a946dc615df4142e9346b88b612efa97a9
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/5d3640537588a6b83cad5a7800ade246a9cf5db1
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/5d40d407a96077b809804085ff7fd325d4e2c751
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/722c54366cc7a53ff6acb5353f07c5d45c02785b
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/72acce5c85cb6b9a4e7877aea2dcad7836ec1d95
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/765adddbd0fbc4dbae23443f519d80cfd1247364
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/7669a19787ea38921380f6448efb92755cd3ebe7
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/7abefb9c46e5fa1c6fd42345e20209742cc25c19
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/863be27863b155e6f37155220d102d19ac725080
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/89ba4d7793f64d14b02322106764c99c85b7fe5e
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/94597d313fdfe4e8a86b1012d87f432bef8a3531
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/9db442e1a7f93bd70af9943726ec4716bf70f47f
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/9ea7fefcd5bd82ccd153328f0f6ad3c42d2027ac
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/a5d1bb0a536587419c5c369985c7fa8a416f72f0
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/ac120c8c2b9825446b57c7eed578084f65125b6d
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/b07649af11f14c82dde60aedb538ab6c74af8475
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/b18bef747994e7466b06b4fb84ac95aba97e1704
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/b63dd09796871771f7f8010d448960fb3ed49706
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/b85d37cc1044820f31ad810490abd06779853723
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/c2260fff2162530e983f9f2db0befce76726f895
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/d15da76f3b80baa06d5ecc8b44c43a657441215e
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/e0d57e257ee254988d5b3491c7b0cb37431adecd
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/e5e8b591e65bebb114abc5c94cfcf1ad40022f94
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/f5fdca43a8f30fa3e16a0e7d9a03044a8dd2f7c4
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/fa75c312fe650d2f64d266c3bb978e6c104fbcfa
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/fba3a25c6014b0438bec63e2bc2781a71c9ae3fd
bubbles_run_with_timeout 30 git update-ref -d refs/ops/OPS-006/unreachable/fc9cd7ba43cf76b929a0d97c582d54def6e94646
bubbles_run_with_timeout 30 git branch -d ops/OPS-006-reconcile
```

Do not prune the now-unreachable objects. The verified bundle and pushed
dispositions provide recovery and auditability.

## Final Invariant Check

Run these commands without output filters:

```bash
bubbles_run_with_timeout 30 git for-each-ref --format='%(refname:short)' refs/heads
bubbles_run_with_timeout 30 git worktree list --porcelain
bubbles_run_with_timeout 30 git stash list
bubbles_run_with_timeout 30 git tag --list
bubbles_run_with_timeout 30 git status --porcelain=v1
bubbles_run_with_timeout 30 git rev-parse main
bubbles_run_with_timeout 30 git rev-parse origin/main
```

Interpret the output exactly:

- The branch command prints only `main`.
- The worktree command reports exactly one worktree record.
- The stash command prints nothing.
- The tag command prints nothing.
- The status command prints nothing.
- Both revision commands print the same full commit ID.

Also confirm every review ledger row has a final class and evidence. Confirm
the next-session handoff has no unresolved owner or vague action.

## Rollback

### Before Main Advances

Abort the current cherry-pick or rebase. Return to `main`. Keep the verified
bundle and ignored-file archive.

Delete the temporary branch only after confirming it adds no recovery value
beyond those archives.

Restore an ignored file from the private archive when a deletion was mistaken.
Do not restore unreviewed files into the repository.

### After Local Main Advances But Before Push

Stop and obtain operator approval before moving local `main` backward. Confirm
the worktree is clean and the bundle still verifies.

Restore local `main` to `origin/main` only before any reconciliation commit has
reached the remote. Keep a branch or bundle reference to the rejected work.

Rerun the final invariant checks after rollback.

### After Push

Never rewrite remote history. Revert faulty integration commits with new
commits, run the complete validation chain, and push normally.

Restore a deleted local tag or dangling commit from the verified bundle only
when further review requires it. Do not republish archive tags by default.

## Closeout Record

Complete these fields during execution and push the result:

| Field | Required value |
|---|---|
| Starting `origin/main` | Full observed commit ID |
| Final `origin/main` | Full verified commit ID |
| Git bundle | Protected external path and SHA-256, without sensitive content |
| Ignored-file archive | Protected external path and SHA-256, without sensitive content |
| Integrated objects | Source object IDs and resulting main commit IDs |
| Non-integrated objects | Final disposition classes and evidence |
| Tags approved for local removal | Exact reviewed names |
| Ignored paths approved for removal | Exact reviewed paths |
| Validation | Current-session commands, exit codes, and evidence location |
| Push verification | Matching local, remote-tracking, and remote commit IDs |
| Next-session handoff | Nonterminal paths, next owners, and exact actions |

Close the packet only after every final invariant passes in the current
session.