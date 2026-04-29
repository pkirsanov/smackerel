# Execution Reports — [BUG-015-001] Twitter API Polling Path Deprecation

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Document the deprecation across all 015-twitter-connector artifacts — Done

### Summary
On 2026-04-26 `bubbles.validate` flagged that Twitter Scope 6 (API Client / Opt-In) DoD claimed an implemented API polling path that does not exist in `internal/connector/twitter/twitter.go`. After review, the team selected option (b) — formally deprecate the API path — because (1) the X v2 free tier no longer reliably supports the bookmarks/likes endpoints described in the original spec, (2) the archive-import path covers the real user value (127/127 unit tests, security/chaos/devops hardening complete), (3) implementing untested paid-tier API integration would carry ongoing maintenance burden for marginal benefit, and (4) the spec already labelled the API path as "Optional".

This bug applies the deprecation across all 015 artifacts: `spec.md`, `scopes.md`, `uservalidation.md`, `state.json`, `report.md`, plus a new `scenario-manifest.json` (which fixes a pre-existing traceability-guard finding). No production code under `internal/connector/twitter/` was modified.

### Completion Statement
All 21 DoD items in `scopes.md` (17 Core + 4 Build Quality) are checked with inline `**Evidence:**` blocks captured this session from real terminal output. Scope 1 status promoted from `In Progress` to `Done`. State promoted from `in_progress` to `done`.

### Test Evidence

**Command:** `./smackerel.sh test unit` (Go unit sweep, captured 2026-04-26)

```
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/connector/twitter       (cached)
... (full sweep, 41 Go packages green; Python tests green)
Exit Code: 0
```

(See parent `specs/015-twitter-connector/report.md` BUG-015-001 Deprecation Resolution section for the full transcript captured in the parent run.)

**Command:** `git diff --name-only HEAD -- internal/connector/twitter/`

```
$ git diff --name-only HEAD -- internal/connector/twitter/
$ echo "exit=$?"
exit=0
```

Empty output proves no production code under the connector package was touched.

### Validation Evidence

**Command:** `jq -r '.status, .certification.status' specs/015-twitter-connector/state.json`

```
$ jq -r '.status, .certification.status' specs/015-twitter-connector/state.json
done
done
Exit Code: 0
```

**Command:** `jq -r '.certification.scopeProgress[] | select(.scope==6) | .status' specs/015-twitter-connector/state.json`

```
$ jq -r '.certification.scopeProgress[] | select(.scope==6) | .status' specs/015-twitter-connector/state.json
Deferred
Exit Code: 0
```

**Command:** `jq -r '.resolvedBugs[].bugId' specs/015-twitter-connector/state.json`

```
$ jq -r '.resolvedBugs[].bugId' specs/015-twitter-connector/state.json
BUG-015-001
Exit Code: 0
```

**Command:** `grep -nE '\[ \] VERIFIED FAIL' specs/015-twitter-connector/scopes.md specs/015-twitter-connector/uservalidation.md`

```
$ grep -nE '\[ \] VERIFIED FAIL' specs/015-twitter-connector/scopes.md specs/015-twitter-connector/uservalidation.md || echo "no VERIFIED FAIL markers remain"
no VERIFIED FAIL markers remain
Exit Code: 0
```

### Audit Evidence

**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector`

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Detected state.json status: done
✅ DoD completion gate passed for status 'done' (all DoD checkboxes are checked)
... (full pass)
Artifact lint PASSED.
Exit Code: 0
```

**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/015-twitter-connector`

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/015-twitter-connector
✅ Scenario manifest cross-check passed
... (full pass)
Exit Code: 0
```

### Verification Notes
- Heavy integration / e2e / stress suites were intentionally NOT re-run because this is a documentation-only deprecation — there is no production code change that could regress the live stack. The unit-test sweep is the appropriate regression gate per the option-(b) decision and the user-supplied boundary "documentation/spec-amendment only".
- The unused `bearer_token` and `api_enabled` fields in `config/smackerel.yaml` and `parseTwitterConfig` are intentionally left in place — config-field cleanup is out of scope per the user-supplied boundary and would be tracked as a separate spec if pursued.
