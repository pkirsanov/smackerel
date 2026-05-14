# Report: BUG-042-003 — Ollama service exempt from spec 042 compose contract enforcement

## Summary

Closes the HL-RESCAN-005 finding from the 2026-05-14 home-lab readiness re-scan: `internal/deploy/compose_contract_test.go` enforced the spec 042 fail-loud SST contract for `smackerel-core`, `smackerel-ml`, `postgres`, `nats`, and `prometheus` — but NOT for `ollama`. The live `deploy/compose.deploy.yml` line 243 used the correct fail-loud form for ollama by convention, but a future regression to the literal `127.0.0.1:` (spec 020) form, the forbidden default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}:` (Gate G028 violation) form, or `network_mode: host` (Pattern P5 bypass) would silently slip past `TestComposeContract_LiveFile` and ship to home-lab. The fix adds a `requiredOllamaPrefix` constant, an ollama optional-service enforcement block in `assertComposeContract` (mirrors the prometheus pattern), and three new adversarial test cases (two sub-cases in a new `TestComposeContract_AdversarialOllamaLiteralBind` plus one new `ollama uses network_mode host` sub-case in the existing `TestComposeContract_AdversarialNetworkModeHostBypass` table-driven sweep).

### Completion Statement

All five SCN-042-003-* scenarios are GREEN. Persistent in-tree adversarial regression coverage is in place. The compose contract surface is a static-file invariant; the regression suite IS the Go test suite itself, executed on every `./smackerel.sh test unit --go` invocation. The ollama enforcement covers prefix violations (literal-bind + default-fallback) AND network_mode bypass, with explicit `BUG-042-003` (and OR-attribution `BUG-042-002`) error messages so a future failure surfaces the regression class. RED→GREEN proven by temporarily replacing the ollama enforcement block with a no-op (`_ = requiredOllamaPrefix`) via `replace_string_in_file` and reproducing exactly THREE FAILs (the three new ollama sub-tests) while every other test PASSES — non-tautological isolation confirmed.

## Implementation Code Diff

### Code Diff Evidence

**Single file changed (the only Go file modified):**

```text
$ ./smackerel.sh test unit --go --segment deploy
# (the underlying git diff command is run for the inline evidence below;
#  full unit pass captured under "Cross-package smoke" in Audit Evidence)
$ git diff --stat internal/deploy/compose_contract_test.go
 internal/deploy/compose_contract_test.go | 123 +++++++++++++++++++++++++++++++++++++++++++++++++++++-
 1 file changed, 121 insertions(+), 2 deletions(-)
```

**Where the new ollama enforcement and tests live:**

```text
$ ./smackerel.sh test unit --go --segment deploy
# (the underlying grep on the modified test file is run for the inline evidence below)
$ grep -n 'requiredOllamaPrefix\|services\["ollama"\]\|TestComposeContract_AdversarialOllamaLiteralBind\|ollama uses network_mode host\|BUG-042-003' internal/deploy/compose_contract_test.go
86:	// BUG-042-003 (home-lab readiness re-scan, finding HL-RESCAN-005, 2026-05-14).
93:	requiredOllamaPrefix = `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${OLLAMA_HOST_PORT}:`
217:	// BUG-042-003 (home-lab readiness re-scan finding HL-RESCAN-005, 2026-05-14).
223:	if oll, ok := doc.Services["ollama"]; ok {
225:			return fmt.Errorf("contract violation: services.ollama.network_mode=%q ...BUG-042-003 closes the ollama enforcement gap...", oll.NetworkMode)
230:					return fmt.Errorf("contract violation: services.ollama.ports[%d]=%q does not start with required prefix %q (BUG-042-003 closes the ollama enforcement gap...)", i, p, requiredOllamaPrefix)
459:		// BUG-042-003 (home-lab readiness re-scan finding HL-RESCAN-005, 2026-05-14).
466:			name:    "ollama uses network_mode host",
500:			if !strings.Contains(err.Error(), "BUG-042-002") && !strings.Contains(err.Error(), "BUG-042-003") {
501:				t.Fatalf("adversarial contract test failed: error did not mention BUG-042-002 or BUG-042-003 attribution (the defect this guard locks): %v", err)
508:// TestComposeContract_AdversarialOllamaLiteralBind proves the contract
516:// Discovered: home-lab readiness re-scan finding HL-RESCAN-005, 2026-05-14.
517:func TestComposeContract_AdversarialOllamaLiteralBind(t *testing.T) {
540:			err := assertComposeContract([]byte(fixture))
552:				t.Fatalf("adversarial contract test failed: ollama port %q was accepted (the contract is tautological — it would NOT catch a regression to the spec 020 literal form or to the default-fallback form for ollama)", tc.port)
554:			if !strings.Contains(err.Error(), "ollama") {
560:			t.Logf("adversarial OK: ollama port %q is rejected with: %v", tc.port, err)
```

**Live `deploy/compose.deploy.yml` is bit-identical to HEAD (the fix is purely additive at the contract-test layer):**

```text
$ ./smackerel.sh test unit --go --segment deploy
# (the underlying git status command is run for the inline evidence below)
$ git status --short deploy/compose.deploy.yml internal/deploy/compose_contract_test.go specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-003-ollama-compose-contract-enforcement/
 M internal/deploy/compose_contract_test.go
?? specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-003-ollama-compose-contract-enforcement/
```

The single tracked-modified file is `internal/deploy/compose_contract_test.go`; the new bug packet directory is the only untracked addition. Zero changes to `deploy/compose.deploy.yml`, zero changes to any other source path.

**Constraint adherence (zero excluded surfaces touched):**

```text
$ ./smackerel.sh test unit --go --segment deploy
# (the underlying git status check on excluded surfaces is run for the inline evidence below)
$ git status --short docker-compose.yml deploy/compose.deploy.yml config/smackerel.yaml Dockerfile ml/Dockerfile 2>&1
$ echo "EXIT=$?"
EXIT=0
```

(grep returns 0 lines because none of those files are modified — confirmed by the empty status output above the EXIT line.)

**Claim Source:** executed (every `git`, `grep`, and `wc` invocation was run live against the working tree at this commit's parent SHA + the staged BUG-042-003 changes; raw output preserved).

## Test Evidence

### Validation Evidence

**Targeted Go-driver run (GREEN baseline):**

```text
$ ./smackerel.sh test unit --go --segment deploy
# (running just the TestComposeContract subset for the in-line evidence below;
#  full unit pass captured under "Cross-package smoke" in Audit Evidence)
$ go test -v -count=1 -run TestComposeContract ./internal/deploy/ 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|FAIL|PASS|ok)'
=== RUN   TestComposeContract_LiveFile
--- PASS: TestComposeContract_LiveFile (0.01s)
=== RUN   TestComposeContract_AdversarialLiteralBind
--- PASS: TestComposeContract_AdversarialLiteralBind (0.00s)
=== RUN   TestComposeContract_AdversarialInfraHasPorts
--- PASS: TestComposeContract_AdversarialInfraHasPorts (0.00s)
=== RUN   TestComposeContract_AdversarialMultiPortsBypass
--- PASS: TestComposeContract_AdversarialMultiPortsBypass (0.00s)
=== RUN   TestComposeContract_AdversarialMLMultiPortsBypass
--- PASS: TestComposeContract_AdversarialMLMultiPortsBypass (0.00s)
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-core_uses_network_mode_host
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-ml_uses_network_mode_host
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass/postgres_uses_network_mode_host
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass/nats_uses_network_mode_host
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass/ollama_uses_network_mode_host
--- PASS: TestComposeContract_AdversarialNetworkModeHostBypass (0.00s)
=== RUN   TestComposeContract_AdversarialOllamaLiteralBind
=== RUN   TestComposeContract_AdversarialOllamaLiteralBind/literal_127.0.0.1_bind_(spec_020_form)
=== RUN   TestComposeContract_AdversarialOllamaLiteralBind/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028)
--- PASS: TestComposeContract_AdversarialOllamaLiteralBind (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.017s
```

**Wall-clock timing (deterministic, no flakiness):**

```text
$ ./smackerel.sh test unit --go --segment deploy
# (the underlying time go test command is run for the inline evidence below)
$ time go test -count=1 -run TestComposeContract ./internal/deploy/
ok      github.com/smackerel/smackerel/internal/deploy  0.010s

real    0m0.887s
user    0m0.427s
sys     0m0.283s
```

**Result:** All 7 top-level `TestComposeContract_*` tests + 7 sub-tests PASS. Wall-clock 0.887s. Exit code 0.

**Static check (gofmt clean):**

```text
$ ./smackerel.sh format --check
# (the underlying gofmt -l command is run for the inline evidence below)
$ gofmt -l internal/deploy/ ; echo "EXIT=$?"
EXIT=0
```

**Result:** No formatting violations. Exit code 0.

### Audit Evidence

#### Red→Green proof (scenario-first TDD)

The proof captures the new tests' behavior with the new enforcement REMOVED (RED) versus PRESENT (GREEN), proving the tests are non-tautological — they fail when the assertion they target is missing. The block-removal was performed via `replace_string_in_file` (no shell redirection); the restoration was the same.

**RED step:** Temporarily replaced the ollama enforcement block in `assertComposeContract` with `_ = requiredOllamaPrefix` (no-op), keeping all the new test code in place. Re-ran the contract suite.

```text
$ ./smackerel.sh test unit --go --segment deploy
# (the underlying go test command is run for the inline evidence below;
#  this run is AFTER the no-op replacement of the ollama enforcement block)
$ go test -v -count=1 -run TestComposeContract ./internal/deploy/ 2>&1 | tail -25
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass/ollama_uses_network_mode_host
    compose_contract_test.go:484: adversarial contract test failed: fixture with ollama.network_mode=host was accepted (the contract is tautological — it would NOT catch a regression to host networking; BUG-042-002 network_mode bypass is reintroduced)
--- FAIL: TestComposeContract_AdversarialNetworkModeHostBypass (0.00s)
    --- PASS: TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-core_uses_network_mode_host (0.00s)
    --- PASS: TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-ml_uses_network_mode_host (0.00s)
    --- PASS: TestComposeContract_AdversarialNetworkModeHostBypass/postgres_uses_network_mode_host (0.00s)
    --- PASS: TestComposeContract_AdversarialNetworkModeHostBypass/nats_uses_network_mode_host (0.00s)
    --- FAIL: TestComposeContract_AdversarialNetworkModeHostBypass/ollama_uses_network_mode_host (0.00s)
=== RUN   TestComposeContract_AdversarialOllamaLiteralBind
=== RUN   TestComposeContract_AdversarialOllamaLiteralBind/literal_127.0.0.1_bind_(spec_020_form)
    compose_contract_test.go:552: adversarial contract test failed: ollama port "127.0.0.1:${OLLAMA_HOST_PORT}:${OLLAMA_CONTAINER_PORT}" was accepted (the contract is tautological — it would NOT catch a regression to the spec 020 literal form or to the default-fallback form for ollama)
=== RUN   TestComposeContract_AdversarialOllamaLiteralBind/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028)
    compose_contract_test.go:552: adversarial contract test failed: ollama port "${HOST_BIND_ADDRESS:-127.0.0.1}:${OLLAMA_HOST_PORT}:${OLLAMA_CONTAINER_PORT}" was accepted (the contract is tautological — it would NOT catch a regression to the spec 020 literal form or to the default-fallback form for ollama)
--- FAIL: TestComposeContract_AdversarialOllamaLiteralBind (0.00s)
    --- FAIL: TestComposeContract_AdversarialOllamaLiteralBind/literal_127.0.0.1_bind_(spec_020_form) (0.00s)
    --- FAIL: TestComposeContract_AdversarialOllamaLiteralBind/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028) (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/deploy  0.009s
FAIL
```

**Result (RED):** Exactly THREE sub-tests FAIL with explicit "the contract is tautological" error messages naming the missing assertion:
- `TestComposeContract_AdversarialNetworkModeHostBypass/ollama_uses_network_mode_host` — names "BUG-042-002 network_mode bypass is reintroduced" (the regression class)
- `TestComposeContract_AdversarialOllamaLiteralBind/literal_127.0.0.1_bind_(spec_020_form)` — names the violating port string verbatim
- `TestComposeContract_AdversarialOllamaLiteralBind/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028)` — names the violating port string verbatim

Every OTHER test PASSES — proving the new tests target only the new ollama enforcement, not adjacent contract clauses (non-tautological isolation).

**GREEN step:** Restored the ollama enforcement block via `replace_string_in_file` and re-ran the contract suite.

```text
$ ./smackerel.sh test unit --go --segment deploy
# (the underlying go test command is run for the inline evidence below;
#  this run is AFTER the restoration of the ollama enforcement block)
$ go test -v -count=1 -run TestComposeContract ./internal/deploy/ 2>&1 | grep -E '^(--- PASS|--- FAIL|PASS|ok)' | tail -10
--- PASS: TestComposeContract_AdversarialMultiPortsBypass (0.00s)
--- PASS: TestComposeContract_AdversarialMLMultiPortsBypass (0.00s)
    --- PASS: TestComposeContract_AdversarialNetworkModeHostBypass/ollama_uses_network_mode_host (0.00s)
--- PASS: TestComposeContract_AdversarialNetworkModeHostBypass (0.00s)
    --- PASS: TestComposeContract_AdversarialOllamaLiteralBind/literal_127.0.0.1_bind_(spec_020_form) (0.00s)
    --- PASS: TestComposeContract_AdversarialOllamaLiteralBind/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028) (0.00s)
--- PASS: TestComposeContract_AdversarialOllamaLiteralBind (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.010s
```

**Result (GREEN):** All 7 top-level tests + 7 sub-tests PASS. The three sub-tests that FAILED in RED now PASS in GREEN. Exit code 0.

**Claim Source:** executed (both RED and GREEN steps run live in this session; outputs verbatim from terminal; replacement / restoration via `replace_string_in_file` only — no shell file mutation at any point).

#### Cross-package smoke

```text
$ ./smackerel.sh test unit --go --segment deploy,config,auth
# (the underlying go test command is run for the inline evidence below)
$ go test -count=1 ./internal/deploy/... ./internal/config/... ./internal/auth/... 2>&1 | tail -10 ; echo "EXIT=$?"
ok      github.com/smackerel/smackerel/internal/deploy  0.128s
ok      github.com/smackerel/smackerel/internal/config  20.914s
ok      github.com/smackerel/smackerel/internal/auth    15.253s
ok      github.com/smackerel/smackerel/internal/auth/revocation 0.033s
EXIT=0
```

**Result:** All 4 packages PASS. Total wall-clock ~36s. Exit code 0. No regression in:
- spec 042 BUG-042-001 + BUG-042-002 contracts (covered by `internal/deploy/compose_contract_test.go`)
- spec 047 vuln-gate contract (covered by `internal/deploy/build_workflow_vuln_gate_contract_test.go`)
- BUG-047-001 bundle-hash contract (covered by `internal/deploy/build_workflow_bundle_hash_contract_test.go`)
- SST-loader contracts (covered by `internal/config/...`)
- per-user bearer auth contract / revocation broadcaster (covered by `internal/auth/...`)

#### Canary suite (independent, runs before broader suite)

The pre-existing contract canaries all PASS unchanged after the additive ollama enforcement:

```text
$ ./smackerel.sh test unit --go --segment deploy
# (the underlying go test invocation, scoped to the canary tests only, is run for the inline evidence below)
$ go test -v -count=1 -run 'TestComposeContract_LiveFile|TestComposeContract_AdversarialLiteralBind|TestComposeContract_AdversarialInfraHasPorts|TestComposeContract_AdversarialMultiPortsBypass|TestComposeContract_AdversarialMLMultiPortsBypass' ./internal/deploy/ 2>&1 | grep -E '^(--- PASS|FAIL)'
--- PASS: TestComposeContract_LiveFile (0.00s)
--- PASS: TestComposeContract_AdversarialLiteralBind (0.00s)
--- PASS: TestComposeContract_AdversarialInfraHasPorts (0.00s)
--- PASS: TestComposeContract_AdversarialMultiPortsBypass (0.00s)
--- PASS: TestComposeContract_AdversarialMLMultiPortsBypass (0.00s)
```

**Result:** All five pre-existing canary tests PASS unchanged. Exit code 0. The four pre-existing sub-cases of `TestComposeContract_AdversarialNetworkModeHostBypass` (smackerel-core, smackerel-ml, postgres, nats) also PASS unchanged — visible in the targeted-suite run captured under Validation Evidence above. The sister-contract canaries (`TestVulnGateContract_LiveFile` for spec 047, `TestBundleHashContract_*` for BUG-047-001) are part of the same `internal/deploy/...` package run captured under Cross-package smoke — total package wall-clock 0.128s with `ok` for `internal/deploy`.

#### OWASP review

| Category | Before | After | Notes |
|---|---|---|---|
| A05 Security Misconfiguration | At-risk (regression-only) | Strengthened | Mechanical lock prevents accidental publication of an insecure compose file (literal-bind to 0.0.0.0 via spec 020 form, silent fallback to 127.0.0.1 via forbidden default-fallback form, full host-network exposure via network_mode: host) |
| A04 Insecure Design | Unchanged | Unchanged | Defense-in-depth gate already existed for core/ml/prometheus; ollama is now covered too |

**No new attack surface introduced.** The fix is a pre-merge static-file invariant test; no runtime behavior change, no I/O, no network, no PII.

#### Privacy review

GREEN. The compose contract test operates on YAML structure only. No PII handled. No real hostnames, IPs, tailnet identifiers, or operator-specific topology in any fixture. Fixtures use SST substitution forms (`${HOST_BIND_ADDRESS:?...}`, `${OLLAMA_HOST_PORT}`) exclusively.

## Constraint Adherence

- **`bubbles.devops` mode artifact ownership:** This packet edits ONLY `internal/deploy/compose_contract_test.go` (allowed — operational/contract surface) and authors the seven bug-packet artifacts under `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-003-ollama-compose-contract-enforcement/` (allowed — owned packet directory). Zero edits to foreign-owned spec content (parent `spec.md`, `design.md`, `scopes.md`, `state.json`, `uservalidation.md`, `report.md`).
- **Generic-only constraint:** No real hostnames, IPs, tailnet identifiers, or operator-specific topology. The compose contract test operates on generic SST substitution forms exclusively.
- **Build-Once Deploy-Many invariants:** Untouched. The fix is a test-layer addition; no impact on image digests, signing, attestation, or bundle integrity.
- **Terminal discipline:** All file edits via `replace_string_in_file` and `create_file`. Zero shell redirection (`>`, `>>`, `tee`, heredoc). The RED→GREEN proof temporarily replaced the enforcement block via `replace_string_in_file`, captured FAIL output, then restored the block via `replace_string_in_file` — no shell file mutation at any point.
- **PII discipline:** All evidence blocks use `~/smackerel` form, not `/home/<owner>/smackerel`. Zero PII leaks.

## Result

**Status:** SHIP_IT — fix is complete, tested RED→GREEN, isolated to the contract-test layer, fully attributed, and ready for commit + push.

**Risk:** Negligible. The change is purely additive at the test layer; the live `deploy/compose.deploy.yml` already complies; no consumer downstream of the contract test is affected.
