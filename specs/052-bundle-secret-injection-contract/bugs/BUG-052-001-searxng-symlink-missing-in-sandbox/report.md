# BUG-052-001 Report — Bundle-secret sandbox searxng symlink

- **Parent spec:** 052-bundle-secret-injection-contract
- **Workflow:** bugfix-fastlane (post-sweep remediation of finding F-047-R17-A)
- **Discovered:** 2026-06-06 (stochastic sweep Round 17)
- **Resolved:** 2026-06-06
- **Baseline HEAD:** d76cb034

## Summary

`scripts/commands/config.sh --bundle` requires `config/searxng/settings.yml`
(added by commit `a7774fff`). The spec 052 sandbox in
`internal/deploy/bundle_secret_contract_test.go` symlinks `prometheus`,
`prompt_contracts`, `assistant`, and `nats_contract.json` but not `searxng`, so
bundle generation exits 1 with `searxng settings file not found` and all five
sub-tests fail. Fix: symlink `config/searxng` into the sandbox `REPO_ROOT`.

## Implementation Code Diff Evidence

### Code Diff Evidence

`internal/deploy/bundle_secret_contract_test.go` — added one symlink + doc-comment update:

<!-- bubbles:evidence-legitimacy-skip-begin -->
```diff
 	symlink(filepath.Join(repoRoot, "config", "assistant"),
 		filepath.Join(tmpRoot, "config", "assistant"))
+	// config/searxng/settings.yml is required by config.sh --bundle (the bundle
+	// stage copies it into <stage>/config/searxng/settings.yml; its absence makes
+	// the loader exit 1 with "searxng settings file not found"). Symlink it like
+	// the other config dirs so the sandbox bundle generation succeeds.
+	symlink(filepath.Join(repoRoot, "config", "searxng"),
+		filepath.Join(tmpRoot, "config", "searxng"))
 	symlink(filepath.Join(repoRoot, "config", "nats_contract.json"),
 		filepath.Join(tmpRoot, "config", "nats_contract.json"))
```

Doc-comment hunk (enumerates symlinked config inputs):

```diff
-// (`config/prometheus/`, `config/prompt_contracts/`, `config/assistant/`,
-// `config/nats_contract.json`, `deploy/`) are symlinked to the live repo so
-// the loader's other inputs remain unchanged.
+// (`config/prometheus/`, `config/prompt_contracts/`, `config/assistant/`,
+// `config/searxng/`, `config/nats_contract.json`, `deploy/`) are symlinked to
+// the live repo so the loader's other inputs remain unchanged.
```
<!-- bubbles:evidence-legitimacy-skip-end -->

## Test Evidence (Red→Green Proof)

### RED — before the symlink was added

```text
$ go test -count=1 -run 'TestBundleSecretContract' ./internal/deploy/...
--- FAIL: TestBundleSecretContract_AdversarialA1_DriftDetector (7.54s)
    ERROR: searxng settings file not found: /tmp/TestBundleSecretContract_AdversarialA1_DriftDetector762436033/001/config/searxng/settings.yml
--- FAIL: TestBundleSecretContract_AdversarialA2_LeakageDetector (7.54s)
    bundle_secret_contract_test.go:541: tampered loader exited 1 (expected 0)
    ERROR: searxng settings file not found: /tmp/.../config/searxng/settings.yml
--- FAIL: TestBundleSecretContract_AdversarialA3_DeterminismDetector (7.24s)
    bundle_secret_contract_test.go:585: first run exited 1
--- FAIL: TestBundleSecretContract_AdversarialA4_OptOutDetector (7.49s)
    bundle_secret_contract_test.go:706: tampered loader exited 1 (expected 0)
FAIL
FAIL    github.com/smackerel/smackerel/internal/deploy  43.019s
FAIL
RC=1
```

### GREEN — after the symlink was added

```text
$ go test -count=1 -run 'TestBundleSecretContract' ./internal/deploy/...
ok      github.com/smackerel/smackerel/internal/deploy  56.014s
RC=0
```

### Red→Green Phase Summary

<!-- bubbles:evidence-legitimacy-skip-begin -->
```text
RED  : 5 sub-tests FAIL ("searxng settings file not found"), package FAIL, RC=1
FIX  : symlink config/searxng -> tmpRoot/config/searxng in sandbox helper
GREEN: all 5 sub-tests PASS, package ok 56.014s, RC=0
```
<!-- bubbles:evidence-legitimacy-skip-end -->

## Verification

### Validation Evidence

#### Build + vet clean after the fix

```text
$ go build ./...
BUILD_RC=0
$ go vet ./internal/deploy/...
VET_RC=0
```

No assertion lines were modified — the A1 drift / A2 leakage / A3 determinism /
A4 opt-out expectations are unchanged; only the sandbox now provides the
searxng input the loader requires.

### Audit Evidence

#### Change Boundary — exactly one source file changed

```text
$ git diff --name-only -- internal/deploy/
internal/deploy/bundle_secret_contract_test.go
$ git diff --name-only -- scripts/ config/searxng/
(empty — config.sh and config/searxng/settings.yml unchanged)
RC=0
```

`config.sh` searxng requirement, `config/searxng/settings.yml`, and spec 052
certification fields are all unchanged. The adversarial guarantees are
preserved (no weakened assertion).

## Completion Statement

Finding F-047-R17-A (spec 052 bundle-secret test failures, surfaced in
stochastic sweep Round 17) is CLOSED. The `internal/deploy` package is GREEN
(`ok ... 56.014s`), unblocking code pushes. The fix is test-scaffolding only;
production bundle generation and every spec 052 secret guarantee are unchanged.
All Scope 1 DoD items are checked with red→green evidence.
