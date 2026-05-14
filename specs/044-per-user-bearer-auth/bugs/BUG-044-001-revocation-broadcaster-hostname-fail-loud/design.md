# Design: BUG-044-001 — auth revocation broadcaster falls back to literal `"smackerel-core"` when HOSTNAME is empty

## Approach

Replace the inline silent-fallback env-read at `cmd/core/wiring.go` line 243 with a fail-loud `(value, error)`-returning helper `resolveBroadcasterInstanceID()`. The helper reads `HOSTNAME` via `os.Getenv` and returns `("", error)` when the value is empty, with an error message that names `HOSTNAME`, `HL-RESCAN-008`, `Gate G028`, `spec 044`, and `deduplication`. The broadcaster construction block consumes the helper, refuses to wire the broadcaster (and emits a loud `slog.Error` with the error and the NATS subject) when the helper returns an error, and proceeds normally otherwise. The error path matches the existing non-fatal-but-loud handling in the same code block (where `revocation.NewBroadcaster` construction errors and `Subscribe` errors are also non-fatal). Add a new test file `cmd/core/wiring_revocation_test.go` with three test methods (positive / empty-set / unset) that lock the fail-loud contract and would FAIL RED if reverted to the silent-fallback form.

## Design Decisions

### DD-1: Extract a helper, do not inline the fail-loud check

**Decision:** Extract the HOSTNAME resolution into a package-private helper `func resolveBroadcasterInstanceID() (string, error)` placed near the top of `cmd/core/wiring.go` (immediately before `buildAPIDeps`). The broadcaster construction block calls the helper and consumes its `(value, error)` return.

**Rationale:** The pre-fix block was 4 lines inline. A helper extraction is justified because (a) it makes the read directly unit-testable without mocking the full `buildDeps` dependency tree (which has ~50 dependencies), (b) the helper is the unit that can be RED→GREEN proven by toggling its body, and (c) the helper's error message centralizes the HL-RESCAN-008 / Gate G028 / spec 044 attribution in a single source location. Without the extraction the test would have to either spin up a real `coreServices` fixture (over-engineering for a 4-line check) or skip the adversarial RED→GREEN proof entirely. The helper is 6 lines including the doc comment; this is not over-engineering.

**Alternatives rejected:**
- Inline check + log.Fatal on empty: rejected because (a) `log.Fatal` aborts the entire process, which is harsher than the existing non-fatal-but-loud pattern in the same block (where `NewBroadcaster` errors are non-fatal), and (b) inline is not unit-testable without mocking the full wiring tree.
- Inline check + slog.Error + skip-construction: rejected because the inline form is not directly unit-testable; only the integration test would catch a regression to the silent-fallback form.
- Move the helper to `cmd/core/helpers.go`: rejected because `helpers.go` is targeted by HL-RESCAN-014 for cleanup of unused fail-soft helpers (`parseFloatEnv` etc.), and adding a new fail-loud helper there would muddy the HL-RESCAN-014 scope. Co-locating the helper with its sole caller in `wiring.go` is the cleaner design.

### DD-2: Fail-loud-but-non-fatal at the broadcaster site

**Decision:** When `resolveBroadcasterInstanceID()` returns an error, the broadcaster block emits `slog.Error("auth revocation broadcaster construction refused", "error", hostnameErr, "subject", cfg.Auth.RevocationNATSSubject)` AND skips broadcaster construction. The service continues to start; `svc.authRevocationBroadcaster` remains `nil` (its zero value); downstream consumers (`api.NewAuthAdminHandlers` line 281) already accept a `nil` broadcaster gracefully (the spec 044 audit verified this fall-through path).

**Rationale:** The existing `revocation.NewBroadcaster` construction errors and `Subscribe` errors in the SAME block are non-fatal (`slog.Error` + continue). Treating the HOSTNAME-empty case as fatal would inconsistently escalate one specific failure mode over the others. The broadcaster is gated on `cfg.Auth.Enabled && svc.nc != nil && svc.nc.Conn != nil && cfg.Auth.RevocationNATSSubject != ""` — if the operator explicitly opted in to revocation broadcast, they get a loud error in the logs explaining exactly which env var to set; the rest of the service continues to function (auth itself is unaffected). This is consistent with the spec 044 design philosophy: the broadcaster is an optional cross-replica deduplication enhancement, not a load-bearing auth dependency.

**Alternatives rejected:**
- `log.Fatal` / `os.Exit(1)` on empty HOSTNAME: rejected per the consistency argument above.
- Return an error from `buildAPIDeps`: rejected because that would break the spec 044 Scope 02 contract (the function returns an error only on fail-fast validation paths, and this is an optional-feature configuration error, not a fail-fast validation error).
- Silently skip with no log: rejected — that would be a different kind of silent default. The whole point of HL-RESCAN-008 is that the operator must be able to learn from logs why broadcaster wiring was refused.

### DD-3: Switch+default+switch instead of nested if/else

**Decision:** The post-fix broadcaster block uses two nested `switch { case ...: ... default: ... }` blocks instead of `if .. else if .. else`. The outer switch on `hostnameErr != nil`; the inner switch on `err != nil` from `NewBroadcaster`.

**Rationale:** The Go style guide and `gocritic` linter prefer `switch` over `if/else if/else` chains when the conditions are mutually exclusive. The nested-switch form makes the three states (HOSTNAME error, NewBroadcaster error, success) visually obvious. The pre-fix block already used `if .. else if .. else` which would have been flagged by `gocritic ifElseChain` on the next lint pass.

**Alternatives rejected:**
- Keep `if .. else if .. else`: rejected per the linter argument.
- Early-return pattern (`if hostnameErr != nil { slog.Error(...); return }`): rejected because the broadcaster block is INSIDE `buildAPIDeps` and an early return would skip the rest of the function (the auth verifier wiring at lines 280+).

### DD-4: Error message must name multiple anchors

**Decision:** The helper's error message is a single `fmt.Errorf` string that names: the env var (`HOSTNAME`), the finding ID (`HL-RESCAN-008`), the policy gate (`Gate G028`), the parent spec (`spec 044`), and the consequence (`deduplication`). The test asserts ALL FIVE tokens are present in the error message via a `strings.Contains` loop.

**Rationale:** A regression to the silent-fallback form OR a partial revert that drops the attribution would be caught immediately by the missing-token assertion. The 5 anchor tokens are deliberately chosen to cover (a) what variable is missing, (b) which finding tracks this, (c) which policy is violated, (d) which spec owns the broader surface, and (e) what behavioral consequence is at stake. A future maintainer reading the error message gets the full context without needing to grep the codebase.

**Alternatives rejected:**
- Short error message ("HOSTNAME is empty"): rejected because the operator gets no actionable context (which finding? which policy? what's at stake?).
- Long error message naming individual replica counts or NATS subject: rejected because the broadcaster block already logs the subject separately via `slog.Error("..., "subject", ...)`; the helper error message stays bounded to the env-read concern.

### DD-5: Three test methods (positive + 2 adversarial)

**Decision:** `cmd/core/wiring_revocation_test.go` declares three test methods: `TestResolveBroadcasterInstanceID_NonEmpty` (positive guard rail), `TestResolveBroadcasterInstanceID_Empty_FailsLoud` (adversarial — empty-string set), and `TestResolveBroadcasterInstanceID_UnsetEnv` (adversarial — env var unset entirely). The empty and unset cases both target the same branch (`os.Getenv` returns `""` for both unset and empty-set) but exercise the input shape distinctly.

**Rationale:** `os.Getenv` returns `""` for both unset and empty-set, so the helper's behavior is identical for both cases. But the two test methods exercise the input shape distinctly: `t.Setenv(KEY, "")` proves the helper handles "set to empty string", and `os.Unsetenv(KEY)` proves it handles "actually unset". A future refactor that switched from `os.Getenv` to `os.LookupEnv` (which DOES distinguish unset from empty-set) would need to satisfy BOTH test methods to remain compliant. The positive `NonEmpty` test guards against an over-zealous rewrite that mistakenly errors on every read.

**Alternatives rejected:**
- Single empty test: rejected because it would not catch a future `os.LookupEnv` refactor that mishandles unset.
- Add a "whitespace-only" test (`HOSTNAME="   "`): rejected because the helper does NOT trim — `os.Getenv` returns whitespace verbatim, and a whitespace-only hostname would be treated as a valid (if useless) instance ID. Adding trim + reject-whitespace would be a separate behavioral change outside HL-RESCAN-008's scope.
- Test the broadcaster construction block end-to-end: rejected because that requires mocking NATS, the bearer store, the revocation cache, and the full `coreServices` fixture — over-engineering for a 4-line check.

### DD-6: RED proof via temporary helper-body revert + restore

**Decision:** Capture the RED→GREEN proof by temporarily reverting the body of `resolveBroadcasterInstanceID` to a silent-fallback form (`return "smackerel-core", nil` on empty) via `replace_string_in_file` — keeping the test file intact. Re-run the targeted Go test selector. Observe the empty-set and unset cases FAIL with the explicit `id="smackerel-core" err=nil` mismatch message. Restore via `replace_string_in_file` and re-run to confirm GREEN.

**Rationale:** This isolates the proof to the helper body only. The test file is untouched throughout the proof. The revert form is deliberately the EXACT pre-fix behavior (literal `"smackerel-core"` string, nil error), so the FAIL message proves the test catches the exact regression HL-RESCAN-008 calls out. The IDE `replace_string_in_file` tool is the user-blessed mechanism for this kind of targeted toggle.

**Alternatives rejected:**
- `git stash push -p cmd/core/wiring.go`: rejected because interactive patch staging cannot be scripted reliably and is error-prone.
- Whole-file stash of `cmd/core/wiring.go` + `git apply`: rejected because the diff would conflict with the parallel session's edits to other files in the working tree.
- Adversarial mutation harness (e.g. go-mutesting): rejected as over-engineering for a 4-line helper.

## Trade-offs

- The helper is package-private (`resolveBroadcasterInstanceID`, lowercase first letter), which keeps the API surface bounded but means external packages cannot reuse it. This is intentional: there is exactly one production caller (the broadcaster wiring block), and a future caller for a different env var should use a parametrized helper or its own fail-loud read pattern, not coerce this one.
- The fail-loud-but-non-fatal stance (DD-2) means an operator who sets `cfg.Auth.RevocationNATSSubject` non-empty AND forgets to ensure `HOSTNAME` is injected gets a degraded service (auth works, revocation broadcast is silent). The slog.Error in the logs is the operator's signal. An alternative that crashed the process would be more aggressive but would also crash on every restart until the operator notices — the slog.Error gives the operator a chance to repair the env without taking the service down.
- The helper signature `() (string, error)` does not accept a parameter for the env var name. This is intentional: the helper is single-purpose for HOSTNAME-resolution-for-broadcaster-instance-ID. A general-purpose `requireEnv(key string) (string, error)` would be a different abstraction that should be designed deliberately if/when a second caller appears (the project's existing pattern is per-call-site fail-loud reads, not a shared helper, per Gate G028).
