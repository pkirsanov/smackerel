# User Validation: BUG-042-003 — Ollama service exempt from spec 042 compose contract enforcement

## Acceptance Checklist

- [x] **AC-1 verified:** `internal/deploy/compose_contract_test.go` declares `requiredOllamaPrefix = `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${OLLAMA_HOST_PORT}:`` matching `deploy/compose.deploy.yml` line 243 character-for-character. (See report.md > Code Diff Evidence.)
- [x] **AC-2 verified:** `assertComposeContract` enforces the ollama port-mapping prefix (every entry in `oll.Ports` MUST start with `requiredOllamaPrefix`) AND `oll.NetworkMode != "host"`, both as an optional-service block (`if oll, ok := doc.Services["ollama"]; ok { ... }`). (See report.md > Code Diff Evidence.)
- [x] **AC-3 verified:** `TestComposeContract_AdversarialOllamaLiteralBind` exists with two table-driven sub-cases: literal `127.0.0.1:` (spec 020 form) and default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}:` (Gate G028 violation). Both PASS in GREEN state and FAIL in RED state. (See report.md > Audit Evidence > Red→Green proof.)
- [x] **AC-4 verified:** `TestComposeContract_AdversarialNetworkModeHostBypass` table-driven sweep gained an `ollama uses network_mode host` sub-case as the fifth entry. The attribution check accepts `BUG-042-002` OR `BUG-042-003`. (See report.md > Code Diff Evidence + Validation Evidence.)
- [x] **AC-5 verified:** `TestComposeContract_LiveFile` continues to PASS GREEN against the unchanged `deploy/compose.deploy.yml`. The live file already complies; the addition only locks compliance against future drift. (See report.md > Validation Evidence > Targeted contract suite.)
- [x] **AC-6 verified:** RED proof captured by temporarily replacing the ollama enforcement block with `_ = requiredOllamaPrefix` (no-op): exactly THREE sub-tests FAIL (`TestComposeContract_AdversarialOllamaLiteralBind/literal_127.0.0.1_bind_(spec_020_form)`, `TestComposeContract_AdversarialOllamaLiteralBind/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028)`, `TestComposeContract_AdversarialNetworkModeHostBypass/ollama_uses_network_mode_host`); every other test PASSES. Restoring the block returns the suite to all-PASS GREEN. (See report.md > Audit Evidence > Red→Green proof (scenario-first TDD).)

## Bounded-Scope Validation

- [x] **Single-file code change:** the only Go file modified is `internal/deploy/compose_contract_test.go`. No production runtime code, no compose files, no `config/smackerel.yaml`, no CI workflow, no scripts, no Dockerfile, no `ml/...`. (See report.md > Code Diff Evidence > git status output.)
- [x] **Foreign-owned spec content untouched:** zero edits to `specs/042-tailnet-edge-bind-pattern/spec.md`, `specs/042-tailnet-edge-bind-pattern/design.md`, `specs/042-tailnet-edge-bind-pattern/scopes.md`, `specs/042-tailnet-edge-bind-pattern/state.json`, `specs/042-tailnet-edge-bind-pattern/uservalidation.md`, `specs/042-tailnet-edge-bind-pattern/report.md`. The `bubbles.devops` mode artifact-ownership boundary is respected.
- [x] **Live file unchanged:** `deploy/compose.deploy.yml` is bit-identical to HEAD. The fix locks compliance against future drift; it does not require the live file to change.
- [x] **Cross-package smoke clean:** `internal/deploy/...`, `internal/config/...`, `internal/auth/...`, `internal/auth/revocation` all PASS with `ok` exit. No regression in spec 042 BUG-042-001 + BUG-042-002 contracts, the SST-loader contract (spec 044), or the per-user bearer auth contract (spec 044). (See report.md > Validation Evidence > Cross-package smoke.)
- [x] **Static checks clean:** `gofmt -l internal/deploy/` returns no output. (See report.md > Validation Evidence > Static checks.)
- [x] **Generic-only constraint preserved:** zero real hostnames, IPs, tailnet identifiers, operator-specific topology, or PII. All references use SST substitution forms or generic placeholders. (See report.md > Constraint Adherence.)
- [x] **Terminal discipline preserved:** all file edits via IDE tools (`replace_string_in_file`, `create_file`). Zero shell redirection at any point in the implementation or in the RED→GREEN proof. (See report.md > Constraint Adherence.)

## Sign-off

**Status: SHIP_IT**

The fix closes home-lab readiness re-scan finding HL-RESCAN-005 (P2). The ollama compose service is now mechanically locked to the spec 042 fail-loud SST contract; a regression to the literal `127.0.0.1:` form (spec 020), the forbidden default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}` form (Gate G028 violation), or `network_mode: host` (Pattern P5 bypass) would be caught at pre-merge by the new adversarial sub-tests. The live `deploy/compose.deploy.yml` already complies, so the fix is risk-free to ship.
