# Bug: BUG-020-003 — `cmd/core/helpers.go` env-reading helpers (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`) silently fall back to `0` / `nil` on missing or malformed env vars, codifying the FORBIDDEN fail-soft pattern under Gate G028 (NO-DEFAULTS / fail-loud SST policy)

## Classification

- **Type:** SST contract gap — fail-soft env-reading helpers exist in `cmd/core/helpers.go` and are tested in `cmd/core/main_test.go`, but have ZERO production callers. Their continued presence advertises a code pattern that the canonical Smackerel SST policy explicitly forbids (`.github/copilot-instructions.md` "SST Zero-Defaults Enforcement" + `.github/instructions/smackerel-no-defaults.instructions.md` Gate G028: Go MUST use `os.Getenv("KEY")` followed by an explicit empty-value error, never a silent fallback).
- **Severity:** P3 — LOW (no production runtime risk today: the three `Env`-suffixed helpers (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`) have ZERO production callers — their fail-soft `0` / `nil` return values never reach a live code path. The risk is documentation/audit drift: a future maintainer who copy-pastes the helper signature into a new connector wiring would re-introduce a Gate G028 violation in production. The two parse-string-form helpers (`parseJSONArray`, `parseJSONArrayVal`) DO have production callers (see Detection table below) and are out-of-scope for deletion — design specialist decides whether they need fail-loud conversion or remain as legitimate string parsers.)
- **Parent Spec:** 020 — Security Hardening (the parent spec that owns Gate G028 / NO-DEFAULTS / fail-loud SST policy enforcement under "SST Zero-Defaults Enforcement (NON-NEGOTIABLE)" in `.github/copilot-instructions.md` and `.github/instructions/smackerel-no-defaults.instructions.md`)
- **Workflow Mode:** bugfix-fastlane (parent-expanded from `bubbles.goal` runtime because `bubbles.workflow` runtime lacks `runSubagent`)
- **Status:** Reported
- **Discovered By:** 2026-05-14 self-hosted readiness re-scan (finding HL-RESCAN-014)

## Problem Statement

The Smackerel runtime contract per `.github/copilot-instructions.md` "SST Zero-Defaults Enforcement (NON-NEGOTIABLE)" is:

> ALL configuration values MUST originate from `config/smackerel.yaml`. Zero hardcoded ports, URLs, hostnames, or fallback defaults anywhere in the codebase.
>
> | Language | FORBIDDEN | REQUIRED |
> |----------|-----------|----------|
> | **Go** | `getEnv("KEY", "fallback")` | `os.Getenv("KEY")` + empty check → fatal |

`.github/instructions/smackerel-no-defaults.instructions.md` Gate G028 reinforces:

> For SST-managed runtime values, the following are forbidden in source, Compose, deploy specs, scripts, examples, and docs unless the text explicitly labels the form as **FORBIDDEN**:
> - `os.getenv("KEY", "default")`
> - `process.env.KEY || "default"`
> - `unwrap_or(...)` / `unwrap_or_default()` for required config
> - Any helper that silently supplies a runtime fallback value

`cmd/core/helpers.go` declares 7 helpers, three of which are env-reading wrappers that codify exactly the pattern Gate G028 forbids — silent fallback of UNSET / malformed env vars to a usable "default-ish" value with only a `slog.Warn` (not an error return, not a `log.Fatal`):

```go
// cmd/core/helpers.go:65-83 (post-fix line numbers will differ)
func parseFloatEnv(key string) float64 {
    s := os.Getenv(key)
    if s == "" {
        return 0                                           // ← FORBIDDEN: silent fallback to 0
    }
    f, err := strconv.ParseFloat(s, 64)
    if err != nil {
        slog.Warn("failed to parse float from env var — using 0", ...)
        return 0                                           // ← FORBIDDEN: silent fallback to 0
    }
    if math.IsNaN(f) || math.IsInf(f, 0) {
        slog.Warn("non-finite float value in env var — using 0", ...)
        return 0                                           // ← FORBIDDEN: silent fallback to 0
    }
    return f
}

// cmd/core/helpers.go:19-22
func parseJSONArrayEnv(key string) []interface{} {
    s := os.Getenv(key)
    return parseJSONArrayVal(key, s)                       // ← parseJSONArrayVal returns nil on empty / parse error
}

// cmd/core/helpers.go:45-48
func parseJSONObjectEnv(key string) map[string]interface{} {
    s := os.Getenv(key)
    return parseJSONObjectVal(key, s)                      // ← parseJSONObjectVal returns nil on empty / parse error
}
```

The runtime risk is bounded today because the three `Env`-suffixed helpers have ZERO production callers (verified evidence in the Detection table below). The only callers are the test suite in `cmd/core/main_test.go`, which exercises the fail-soft branches to lock their current behaviour. Their continued presence in the codebase produces three concrete harms:

1. **Code-search audit drift.** A future agent grepping for Gate G028 violations (`grep -rn 'os\.Getenv(' cmd/core/`) finds the helpers and has to decide whether they are exempt. The helpers are exempt today only because no production code calls them; the moment any new wiring code calls `parseFloatEnv("FOO")` to read a numeric config, Gate G028 is silently violated in production.
2. **Copy-paste hazard.** The helper signatures (`parseFloatEnv(key string) float64`, `parseJSONArrayEnv(key string) []interface{}`, `parseJSONObjectEnv(key string) map[string]interface{}`) are convenient one-call ergonomics. A maintainer adding a new connector who needs a numeric env value is reasonably likely to reach for `parseFloatEnv("WEATHER_FORECAST_DAYS")` before discovering that the canonical fail-loud pattern requires a hand-rolled `os.Getenv` + `strconv.ParseFloat` + empty-or-error → `log.Fatal` block.
3. **Test suite codifies forbidden behaviour.** The 16 + 5 + 3 = 24 test cases in `cmd/core/main_test.go` (lines 229–329, 407–447, 570–658) lock the silent-fallback semantics — `parseFloatEnv("UNSET") == 0`, `parseJSONArrayEnv("INVALID_JSON") == nil`, etc. These tests would block any future fail-loud conversion of the `Env`-suffixed helpers without coordinated test deletion or rewrite. Removing the helpers and their tests in the same change unblocks the audit-cleanliness goal.

The fix is a deletion (or fail-loud conversion — design specialist decides) bounded to the three `Env`-suffixed helpers AND their corresponding test cases in `cmd/core/main_test.go`. The string-form parse helpers (`parseJSONArray`, `parseJSONArrayVal`) and the helpers' fall-through targets (`parseJSONObjectVal`) require a separate determination — see "Discrepancy with Discovery Brief" and "Out of Scope" sections below.

## Detection

| Aspect | Detail |
|---|---|
| Trigger | self-hosted readiness re-scan (system review session 2026-05-14) |
| Finding | HL-RESCAN-014 |
| Severity | P3 (zero production callers; risk is documentation/audit drift + copy-paste hazard, not live runtime fail-soft) |
| Lens | SST defaults / Gate G028 NO-DEFAULTS fail-loud policy |
| Surface | `cmd/core/helpers.go` (7 helper functions; 3 confirmed-dead, 2 live, 2 transitively-live-or-dead pending design decision) |
| Audit method | Cross-package `grep_search` for each helper symbol across all `*.go` files in the repo, restricted by package. The `Env`-suffixed helpers (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`) had ZERO production hits across `cmd/`, `internal/`, and `tests/` — only test references in `cmd/core/main_test.go`. The string-form `parseJSONArray` had 2 production hits in `cmd/core/connectors.go` lines 76 and 103 (called from connector wiring blocks for `BookmarksExcludeDomains` and `BrowserHistoryCustomSkipDomains`), so `parseJSONArray` AND its delegate `parseJSONArrayVal` are LIVE. The string-form `parseJSONObject` had ZERO production hits — only test references. `parseJSONObjectVal` is reachable only via `parseJSONObject` (dead) + `parseJSONObjectEnv` (dead), so it is transitively dead pending design confirmation. Cross-referenced `.github/copilot-instructions.md` "SST Zero-Defaults Enforcement" table and `.github/instructions/smackerel-no-defaults.instructions.md` Gate G028 wording. |

### Verified Call-Site Inventory (as of HEAD on 2026-05-14)

| Helper | Production callers | Test callers | Status | Evidence |
|---|---|---|---|---|
| `parseFloatEnv` | 0 | 16 (`cmd/core/main_test.go` lines 229, 237, 245, 254, 262, 270, 278, 286, 296, 304, 312, 320, 329, 570, 578, 590) | UNUSED IN PRODUCTION — confirmed candidate for deletion | `grep_search` across `cmd/`, `internal/`, `tests/` found zero non-test hits |
| `parseJSONArrayEnv` | 0 | 5 (`cmd/core/main_test.go` lines 407, 415, 423, 645, 658) | UNUSED IN PRODUCTION — confirmed candidate for deletion | `grep_search` across `cmd/`, `internal/`, `tests/` found zero non-test hits |
| `parseJSONObjectEnv` | 0 | 3 (`cmd/core/main_test.go` lines 431, 439, 447) | UNUSED IN PRODUCTION — confirmed candidate for deletion | `grep_search` across `cmd/`, `internal/`, `tests/` found zero non-test hits |
| `parseJSONObject` | 0 | 7 (`cmd/core/main_test.go` lines 167, 180, 187, 197, 205, 212, 463) | UNUSED IN PRODUCTION — candidate for deletion (transitively unblocks `parseJSONObjectVal`) | `grep_search` across `cmd/`, `internal/`, `tests/` found zero non-test hits |
| `parseJSONObjectVal` | 0 (called only by `parseJSONObject` + `parseJSONObjectEnv`, both dead) | 0 (called only transitively from the test cases above) | TRANSITIVELY UNUSED — candidate for deletion if both callers above are removed | `grep_search` for direct callers found only `parseJSONObject` + `parseJSONObjectEnv` in `cmd/core/helpers.go` |
| `parseJSONArray` | 2 (`cmd/core/connectors.go` lines 76, 103: `parseJSONArray(cfg.BookmarksExcludeDomains)`, `parseJSONArray(cfg.BrowserHistoryCustomSkipDomains)`) | 7 (`cmd/core/main_test.go` lines 109, 119, 126, 136, 143, 150, 158) | LIVE IN PRODUCTION — out-of-scope for this packet (string parser, not env reader) | `grep_search` `parseJSONArray\(` |
| `parseJSONArrayVal` | 0 (called only by `parseJSONArray` (live) + `parseJSONArrayEnv` (dead)) | 0 (called only transitively) | LIVE IN PRODUCTION via `parseJSONArray` — out-of-scope for deletion | `grep_search` for direct callers found only `parseJSONArray` + `parseJSONArrayEnv` in `cmd/core/helpers.go` |

### Discrepancy with Discovery Brief

The discovery brief from the parent agent listed all 7 helpers as candidates for deletion. The honest in-spec evidence above shows that **`parseJSONArray` has 2 production callers** in `cmd/core/connectors.go` and is therefore NOT a candidate for deletion in this packet. Its delegate `parseJSONArrayVal` is reachable from production via `parseJSONArray` and is therefore also NOT a candidate for deletion.

The corrected dead-set candidates for this packet are:

1. `parseFloatEnv` — 0 production callers, 16 test callers
2. `parseJSONArrayEnv` — 0 production callers, 5 test callers
3. `parseJSONObjectEnv` — 0 production callers, 3 test callers
4. `parseJSONObject` — 0 production callers, 7 test callers
5. `parseJSONObjectVal` — transitively dead (called only by 1, 2, and 3 above, all dead)

Note that the discovery brief was **directionally correct on the FORBIDDEN-pattern claim** (the three `Env`-suffixed helpers do codify the forbidden silent-fallback shape) and **directionally correct on the dead-set claim for the `Env`-suffixed helpers** (they truly have zero production callers). The discrepancy is bounded to two of the parse-string-form helpers (`parseJSONArray`, `parseJSONArrayVal`), which are LIVE because of the `cfg.BookmarksExcludeDomains` / `cfg.BrowserHistoryCustomSkipDomains` connector wiring sites.

The design specialist receives this honest evidence and may choose any of:

- **(A) Pure deletion of the 5 dead helpers + their tests.** Simplest scope; closes the audit-drift / copy-paste hazard. Leaves `parseJSONArray` + `parseJSONArrayVal` in place because they have legitimate production callers.
- **(B) Pure deletion of the 5 dead helpers + their tests + fail-loud refactor of `parseJSONArray` callers.** Larger scope; converts `cmd/core/connectors.go:76,103` to inline fail-loud `json.Unmarshal` blocks if Gate G028 also applies to JSON-shaped config (design specialist's call). Out of scope for this packet's `spec.md` per the user's bounded acceptance criteria; tracked as a sequel finding if needed.
- **(C) Convert all 3 `Env`-suffixed helpers to fail-loud read pattern instead of deleting them.** The signatures would change (`parseFloatEnv(key string) (float64, error)` etc.) and callers — there are none in production — would need to handle errors. The test cases would also need to be rewritten. Likely strictly worse than (A) because the signatures advertise a "default value on missing" ergonomic that fail-loud reads should not encourage; deleting the helpers and forcing future wiring code to write the canonical pattern inline matches the existing precedent set by `cmd/core/wiring.go` HOSTNAME read after BUG-044-001.

## Acceptance Criteria

- AC-1: A single-scope bug packet exists at `specs/020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/` with all 7 standard artifacts (`spec.md`, `design.md`, `scopes.md`, `report.md`, `uservalidation.md`, `state.json`, `scenario-manifest.json`) authored. The packet's `state.json` declares `parentWorkflow.mode = "self-hosted-readiness-rescan-2026-05-14"`, `workflowMode = "bugfix-fastlane"`, and `discoveryRef.source` matching the sister packets exactly.
- AC-2: After the fix lands, `grep_search` of all `*.go` files for the dead-set symbols (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`, `parseJSONObject`, `parseJSONObjectVal`) returns ZERO matches anywhere in the repo (no source definitions, no test references, no comment mentions). Equivalent for any helpers the design specialist confirms are transitively dead after the deletion.
- AC-3: `cmd/core/helpers.go` does not contain any `os.Getenv("KEY")` followed by a `return <literal>` / `return nil` silent-fallback path. Any env reads remaining in `cmd/core/helpers.go` (e.g., from helpers transitively kept alive by live string-form callers) MUST follow the Gate G028 canonical Go pattern: `os.Getenv("KEY")` + empty/parse-error → `log.Fatal` (or, for connector-side parse failures, propagate the error up to the wiring block which decides whether to refuse construction with `slog.Error`).
- AC-4: `cmd/core/main_test.go` does not contain any test function that locks the silent-fallback semantics for the deleted helpers. Tests for the deleted helpers are removed in the same change set; tests for any kept helpers are rewritten to assert the fail-loud / error-returning contract, not the legacy silent-fallback contract.
- AC-5: An adversarial regression guard exists that mechanically catches future re-introduction of the FORBIDDEN pattern. The guard MUST live in a persistent in-tree test (Go test, build tag check, or `go vet` analyzer) that runs on every `./smackerel.sh test unit --go` invocation and fails loud if any new helper in `cmd/core/` matches the silent-fallback signature shape (`func ...(key string) ...` whose body contains `os.Getenv(key)` followed by a return-on-empty path that does not propagate an error). The guard's RED→GREEN proof is captured in `report.md` per `evidence-rules.md`.
- AC-6: All existing Go unit tests pass after the deletion. `./smackerel.sh test unit --go` returns exit 0. The pre-existing `cmd/core/connectors.go:76,103` `parseJSONArray(...)` call sites continue to compile and behave identically (deletion is bounded to dead helpers; live callers are not touched).
- AC-7: Generic-only constraint preserved: zero real hostnames, IPs, tailnet identifiers, owner-username tokens, real geographic locations, real Tailscale identifiers, or real systemd unit names introduced into any source or evidence file. The error-message tokens `Gate G028` and `HL-RESCAN-014` are policy / finding identifiers and are explicitly ALLOWED. PII paths (`/home/<user>/...`) in any evidence block MUST be redacted to `~/smackerel` per repo policy.

## Out of Scope

- Editing `cmd/core/connectors.go` lines 76 and 103 (`parseJSONArray(cfg.BookmarksExcludeDomains)` / `parseJSONArray(cfg.BrowserHistoryCustomSkipDomains)`). Those are LIVE production callers of `parseJSONArray` (the string-form parser, not the env-form parser). Their fail-soft `nil`-on-parse-error behaviour is a legitimate connector concern (handing `nil` to the connector params map produces an empty exclusion list, which is a sensible default for a JSON-encoded list of domains). Whether Gate G028 applies to JSON-shaped config values stored in struct fields rather than read from env is a separate question for a sequel packet — out-of-scope here to keep this fix minimum-touch and to avoid colliding with other connector-wiring sessions.
- Editing `parseJSONArray` and `parseJSONArrayVal` in `cmd/core/helpers.go`. Both are live in production via the connector wiring sites above; their continued presence is required for `cmd/core` to build. If the design specialist later determines that the JSON-shaped config values themselves should be converted to fail-loud reads, that work belongs in a separate packet that touches both the parser AND the connector wiring sites in one coherent change.
- Editing `internal/config/`, `scripts/commands/config.sh`, or any other SST loader. The SST emission of numeric / JSON-shaped env values is already correct; the bug is purely about dead helper code that codifies a forbidden read shape. The SST source-of-truth boundary is unchanged.
- Editing `.github/copilot-instructions.md`, `.github/instructions/smackerel-no-defaults.instructions.md`, or `.github/skills/smackerel-no-defaults/SKILL.md`. The policy text already correctly forbids the silent-fallback pattern; this packet aligns the codebase to the existing policy, not the other way around.
- Editing `specs/020-security-hardening/spec.md`, `design.md`, `scopes.md`, `state.json`, `report.md`, or `uservalidation.md` (foreign-owned parent-spec content; outside the bug packet's edit scope). Cross-references to those files are read-only.
- Editing other `cmd/core/` source files (`cmd/core/main.go`, `cmd/core/wiring.go`, `cmd/core/connectors.go`, `cmd/core/api_init.go`, etc.) for unrelated NO-DEFAULTS audit cleanup. HL-RESCAN-008 already closed the `HOSTNAME` read site (BUG-044-001); other wiring-site reads are tracked separately.
- Editing the ML sidecar (`ml/`). The Python equivalent of this finding is HL-RESCAN-013 (BUG-020-002, already CLOSED at commit `eec1437c`). This packet is bounded to the Go core surface in `cmd/core/`.
- Implementing the fix itself. This packet's scope is strictly the BUG-phase artifact creation (spec.md authored by `bubbles.bug`, design.md by `bubbles.design`, scopes.md by `bubbles.plan`, source/test changes by `bubbles.implement`, validation by `bubbles.test` / `bubbles.validate`, audit by `bubbles.audit`).
- Editing parallel-session WIP under `specs/041-qf-companion-connector/` and `specs/052-bundle-secret-injection-contract/` (out-of-scope per discovery-brief constraints).

## Sister Packets (Same Discovery Mode `self-hosted-readiness-rescan-2026-05-14`)

These packets share the same discovery source and `parentWorkflow.mode` value. They are referenced for shape-template consistency and to prevent duplicate work; this packet does not modify any of their content.

| Sister | Path | Status | Lens | Surface |
|---|---|---|---|---|
| BUG-020-002 | [`specs/020-security-hardening/bugs/BUG-020-002-ml-auth-token-module-import-fail-loud/`](../BUG-020-002-ml-auth-token-module-import-fail-loud/) | done | SST defaults (Python) | `ml/app/auth.py` module-import-time `os.environ.get` → fail-loud read (HL-RESCAN-013) |
| BUG-029-003 | [`specs/029-devops-pipeline/bugs/BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/`](../../../029-devops-pipeline/bugs/BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/) | done | SST defaults (Compose) | `${VAR:-default}` substitution sweep across compose files |
| BUG-042-003 | `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-003-*` | (sister, not modified here) | SST defaults | tailnet-edge bind pattern |
| BUG-042-004 | `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-004-*` | (sister, not modified here) | SST defaults | tailnet-edge bind pattern |
| BUG-042-005 | `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-005-*` | (sister, not modified here) | SST defaults | tailnet-edge bind pattern |
| BUG-043-003 | `specs/043-*/bugs/BUG-043-003-*` | (sister, not modified here) | SST defaults | (sister-packet context) |
| BUG-044-001 | [`specs/044-per-user-bearer-auth/bugs/BUG-044-001-revocation-broadcaster-hostname-fail-loud/`](../../../044-per-user-bearer-auth/bugs/BUG-044-001-revocation-broadcaster-hostname-fail-loud/) | done | SST defaults (Go) | `cmd/core/wiring.go` HOSTNAME → fail-loud `resolveBroadcasterInstanceID()` helper (HL-RESCAN-008) |

### Cross-References Already Filed in Sister Packets

Three sister packets explicitly carve out HL-RESCAN-014 (this packet) as out-of-scope and defer to the sequel:

- [`specs/020-security-hardening/bugs/BUG-020-002-ml-auth-token-module-import-fail-loud/uservalidation.md`](../BUG-020-002-ml-auth-token-module-import-fail-loud/uservalidation.md) line 107 — "HL-RESCAN-014 (next packet candidate) — `cmd/core/helpers.go` unused-helper cleanup (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`)."
- [`specs/044-per-user-bearer-auth/bugs/BUG-044-001-revocation-broadcaster-hostname-fail-loud/spec.md`](../../../044-per-user-bearer-auth/bugs/BUG-044-001-revocation-broadcaster-hostname-fail-loud/spec.md) line 62, design.md line 18, scopes.md line 209 — "those are out-of-scope unused fail-soft helpers tracked separately by HL-RESCAN-014."
- [`specs/029-devops-pipeline/bugs/BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/report.md`](../../../029-devops-pipeline/bugs/BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/report.md) line 347 — "HL-RESCAN-014 — cmd/core/helpers.go dead helper functions" listed as remaining work.

This packet is the explicit follow-up referenced from each of those locations.

## Policy Anchors

| Anchor | Where | What It Says |
|---|---|---|
| Gate G028 (NO-DEFAULTS / fail-loud SST) | `.github/instructions/smackerel-no-defaults.instructions.md` | Forbids `os.getenv("KEY", "default")`, `${VAR:-default}`, `unwrap_or(...)` for required config; mandates `os.Getenv("KEY")` + empty check → fatal/refuse. |
| SST Zero-Defaults Enforcement | `.github/copilot-instructions.md` (Required Runtime Standards section) | "ALL configuration values MUST originate from `config/smackerel.yaml`. Zero hardcoded ports, URLs, hostnames, or fallback defaults anywhere in the codebase." Provides the FORBIDDEN / REQUIRED table per language. |
| Smackerel NO-DEFAULTS Skill | `.github/skills/smackerel-no-defaults/SKILL.md` | Operational guidance on detecting and fixing fail-soft helpers. |
| Sister precedent (Go) | BUG-044-001 closure at commit `7482fb24` | `cmd/core/wiring.go` `os.Getenv("HOSTNAME")` + literal-string fallback → `resolveBroadcasterInstanceID() (string, error)` helper that returns error on empty. |
| Sister precedent (Python) | BUG-020-002 closure at commit `eec1437c` | `ml/app/auth.py` `os.environ.get(KEY, "")` → `os.environ[KEY]` wrapped in `try/except KeyError → raise RuntimeError` with breadcrumb tokens. |
