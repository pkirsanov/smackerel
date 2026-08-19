// spec 052 chaos-finding follow-up — Envsubst Test-Wrapper Contract.
//
// Long-lived contract test asserting that every Go test wrapper under
// scripts/runtime/ which delegates to scripts/commands/config.sh
// (directly or transitively, via the SST-loader assertions in any test
// layer — unit, integration, e2e, stress) explicitly ensures envsubst
// is available before invoking `go test`. The contract is satisfied by
// sourcing the shared helper `scripts/runtime/_ensure_envsubst.sh` and
// calling `ensure_envsubst <tag>` early in the wrapper, BEFORE the
// `cd /workspace` line and BEFORE any `go test` invocation.
//
// Rationale (long-term, not a workaround):
//
//   The base test image `golang:1.25.10-bookworm` does NOT include
//   `gettext-base`. Without this contract, only `scripts/runtime/go-unit.sh`
//   carried the install logic (per spec-047 R2R-CI). The spec-052
//   chaos phase surfaced that integration/e2e/stress wrappers shelling
//   into scripts/commands/config.sh would fail with exit 127
//   `envsubst: command not found`, blocking:
//
//     - spec-041 Scope 2 live integration/e2e tests (per the runtime
//       note at tests/e2e/qf_decisions_connector_api_test.go:580)
//     - tests/integration/config_validate_test.go,
//       ollama_config_contract_test.go,
//       drive/drive_config_contract_test.go
//     - tests/e2e/drive/drive_foundation_e2e_test.go
//
//   The structural fix promotes the envsubst-install path to a shared
//   helper so all four wrappers share one implementation. This contract
//   test prevents regression — any new wrapper or any wrapper that
//   loses the source line will fail this contract.
//
// Adversarial sub-tests prove the assertion has bite:
//   - A wrapper missing the source line is REJECTED.
//   - A wrapper sourcing the helper but never calling ensure_envsubst
//     is REJECTED.
//   - A wrapper calling ensure_envsubst AFTER `go test` is REJECTED.
//   - A wrapper whose `go test` invocation the matcher cannot LOCATE is
//     REJECTED (BUG-061-013). A zero match is a locator failure, never
//     compliance: membership in envsubstTrackedWrappers is itself the
//     assertion that the wrapper runs `go test`.
//   - A wrapper using the conditional-and-piped invocation form with
//     ensure_envsubst AFTER it is REJECTED with the ORDERING error, which
//     proves the widened matcher landed on the real invocation rather than
//     on something inert (BUG-061-013).

package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// envsubstTrackedWrappers is the canonical list of go-test wrappers
// that delegate to scripts/commands/config.sh (directly or via tests
// they invoke). Adding a new go-* wrapper that runs `go test` MUST be
// accompanied by appending it here.
var envsubstTrackedWrappers = []string{
	"go-unit.sh",
	"go-integration.sh",
	"go-e2e.sh",
	"go-stress.sh",
}

// envsubstHelperRelpath is the shared helper that all tracked wrappers
// must source. It lives next to the wrappers under scripts/runtime/.
const envsubstHelperRelpath = "scripts/runtime/_ensure_envsubst.sh"

// envsubstSourceLineRE matches a non-comment `source` (or `.`) line
// that pulls in _ensure_envsubst.sh. The (?m)^\s* prefix anchors to
// line start so the rationale comment above does NOT match; the
// `(?:source|\.)\s` enforces the source-statement shape; the rest of
// the line is unconstrained so any path-construction idiom is
// accepted as long as it ends in `_ensure_envsubst.sh`.
var envsubstSourceLineRE = regexp.MustCompile(`(?m)^[^\S\n]*(?:source|\.)\s+\S.*_ensure_envsubst\.sh`)

// envsubstCallRE matches a non-comment call to the ensure_envsubst
// function. The (?m)^\s* prefix anchors the call to a line start so a
// comment-mention (e.g., the rationale comment above) does NOT match.
var envsubstCallRE = regexp.MustCompile(`(?m)^\s*ensure_envsubst\s+`)

// envsubstGoTestRE matches an actual `go test` invocation. This must
// appear AFTER the ensure_envsubst call.
//
// The token is anchored to a line start, but whitespace is NOT the only
// permitted prefix. Real wrappers put shell syntax in front of the command:
// scripts/runtime/go-integration.sh runs it as `if ! go test … | tee …`, a
// form the eval gate requires. A pattern that allowed only leading
// whitespace matched nothing there, and before BUG-061-013 a zero match
// returned nil — so the ordering check silently asserted nothing.
//
// The permitted prefixes are therefore enumerated: `if`, `elif`, `then`,
// `while`, `until`, `&&`, `||`, `|`, and `!`, repeatable in any order. The
// enumeration is deliberately bounded rather than open. Forms outside it
// (`env VAR=x go test`, `timeout N go test`, `x=$(go test …)`) still do not
// match — and that is safe now only because the zero-match branch below
// makes such a miss LOUD. Widening a blind matcher buys one round; the
// absence branch is what makes the next miss visible.
var envsubstGoTestRE = regexp.MustCompile(`(?m)^[^\S\n]*(?:(?:if|elif|then|while|until|&&|\|\||\||!)[^\S\n]+)*go[^\S\n]+test\b`)

// assertEnvsubstWrapperContract reads the wrapper bytes, asserts the
// presence + order invariants, and returns a descriptive error on
// violation. Used by both the live test and adversarial sub-tests.
func assertEnvsubstWrapperContract(wrapperName string, raw []byte) error {
	src := string(raw)

	srcIdx := envsubstSourceLineRE.FindStringIndex(src)
	if srcIdx == nil {
		return fmt.Errorf("%s: missing source line for %s; add `source \"$(dirname \"${BASH_SOURCE[0]}\")/_ensure_envsubst.sh\"` near the top of the wrapper",
			wrapperName, envsubstHelperRelpath)
	}

	callIdx := envsubstCallRE.FindStringIndex(src)
	if callIdx == nil {
		return fmt.Errorf("%s: sources %s but never calls `ensure_envsubst <tag>`; add the call immediately after the source line",
			wrapperName, envsubstHelperRelpath)
	}

	if callIdx[0] < srcIdx[0] {
		return fmt.Errorf("%s: `ensure_envsubst` call (offset %d) appears BEFORE the source line (offset %d); the helper must be sourced first",
			wrapperName, callIdx[0], srcIdx[0])
	}

	goTestIdx := envsubstGoTestRE.FindStringIndex(src)
	if goTestIdx == nil {
		// Absence is a LOCATOR failure, not compliance. Every wrapper reaching
		// here is in envsubstTrackedWrappers, and the documented entry
		// condition for that list is "runs `go test`" — so the list and the
		// matcher contradict each other, and it is the matcher that is
		// fallible. Returning nil here is the BUG-061-013 silent pass.
		return fmt.Errorf("%s: could not LOCATE a `go test` invocation. This wrapper is in envsubstTrackedWrappers precisely because it runs `go test`, so a zero match means the matcher is blind — the wrapper is NOT necessarily wrong and MUST NOT be rewritten to satisfy the pattern. The matcher may need widening: extend envsubstGoTestRE to recognise the invocation form actually in use",
			wrapperName)
	}
	if goTestIdx[0] < callIdx[0] {
		return fmt.Errorf("%s: `go test` invocation (offset %d) appears BEFORE the `ensure_envsubst` call (offset %d); envsubst must be ensured BEFORE any go test runs that may shell out to scripts/commands/config.sh",
			wrapperName, goTestIdx[0], callIdx[0])
	}

	return nil
}

func envsubstWrapperRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) returned !ok")
	}
	// internal/deploy/envsubst_wrapper_contract_test.go → two parents up.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// TestEnvsubstWrapperContract_HelperExistsAndIsExecutable proves the
// shared helper is present and marked executable. A missing or
// non-executable helper would silently no-op for all sourcing wrappers,
// re-introducing the spec-052 chaos finding.
func TestEnvsubstWrapperContract_HelperExistsAndIsExecutable(t *testing.T) {
	root := envsubstWrapperRepoRoot(t)
	helperPath := filepath.Join(root, envsubstHelperRelpath)

	info, err := os.Stat(helperPath)
	if err != nil {
		t.Fatalf("shared helper missing at %s: %v", envsubstHelperRelpath, err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("shared helper at %s is not executable (mode=%v); chmod +x required so sourcing wrappers can run it consistently",
			envsubstHelperRelpath, info.Mode())
	}

	body, err := os.ReadFile(helperPath)
	if err != nil {
		t.Fatalf("read helper: %v", err)
	}
	if !strings.Contains(string(body), "ensure_envsubst()") {
		t.Fatalf("helper at %s must define the function `ensure_envsubst()`; not found",
			envsubstHelperRelpath)
	}
	if !strings.Contains(string(body), "apt-get install -y --no-install-recommends gettext-base") {
		t.Fatalf("helper at %s must install gettext-base via apt-get (the package providing envsubst on Debian/Ubuntu); not found",
			envsubstHelperRelpath)
	}
}

// TestEnvsubstWrapperContract_LiveWrappers asserts every tracked
// wrapper sources the helper and calls ensure_envsubst BEFORE any
// `go test` invocation.
func TestEnvsubstWrapperContract_LiveWrappers(t *testing.T) {
	root := envsubstWrapperRepoRoot(t)
	for _, name := range envsubstTrackedWrappers {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, "scripts", "runtime", name)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			if err := assertEnvsubstWrapperContract(name, raw); err != nil {
				t.Fatalf("envsubst wrapper contract violated: %v", err)
			}
		})
	}
}

// TestEnvsubstWrapperContract_AdversarialRejectsMissingSource proves
// the guard catches a wrapper that forgets to source the helper. If
// this fixture passes the guard silently, the guard would not detect
// a regression where a new wrapper is added without the source line.
func TestEnvsubstWrapperContract_AdversarialRejectsMissingSource(t *testing.T) {
	const fixture = `#!/usr/bin/env bash
set -euo pipefail
cd /workspace
go test ./...
`
	err := assertEnvsubstWrapperContract("synthetic-missing-source.sh", []byte(fixture))
	if err == nil {
		t.Fatalf("guard FALSE NEGATIVE: assertEnvsubstWrapperContract returned nil for a wrapper that never sources _ensure_envsubst.sh; this would re-introduce the spec-052 chaos finding")
	}
	if !strings.Contains(err.Error(), "missing source line") {
		t.Fatalf("guard error must say `missing source line`; got: %q", err.Error())
	}
}

// TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall
// proves the guard catches a wrapper that sources the helper but
// never invokes the function. A sourced-but-never-called helper has
// the same runtime effect as no helper at all.
func TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall(t *testing.T) {
	const fixture = `#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
cd /workspace
go test ./...
`
	err := assertEnvsubstWrapperContract("synthetic-source-only.sh", []byte(fixture))
	if err == nil {
		t.Fatalf("guard FALSE NEGATIVE: assertEnvsubstWrapperContract returned nil for a wrapper that sources but never calls ensure_envsubst; the helper would never run")
	}
	if !strings.Contains(err.Error(), "never calls") {
		t.Fatalf("guard error must say `never calls`; got: %q", err.Error())
	}
}

// TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest proves
// the guard catches a wrapper that calls ensure_envsubst AFTER `go test`.
// Calling the helper after the test already ran provides zero
// protection.
func TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest(t *testing.T) {
	const fixture = `#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
cd /workspace
go test ./...
ensure_envsubst "go-late"
`
	err := assertEnvsubstWrapperContract("synthetic-call-after-go-test.sh", []byte(fixture))
	if err == nil {
		t.Fatalf("guard FALSE NEGATIVE: assertEnvsubstWrapperContract returned nil for a wrapper that calls ensure_envsubst AFTER go test; envsubst would never be installed before the SST-loader assertions run")
	}
	if !strings.Contains(err.Error(), "BEFORE the `ensure_envsubst` call") {
		t.Fatalf("guard error must say `BEFORE the ensure_envsubst call`; got: %q", err.Error())
	}
}

// TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation proves
// the guard fails when it cannot LOCATE the invocation it is contracted to
// order against (BUG-061-013, SCN-01/SCN-02).
//
// The fixture is well-formed: it sources the helper and calls ensure_envsubst
// first. Only the invocation is written in a shape the matcher does not
// enumerate — an assignment plus command substitution. Before BUG-061-013 the
// zero match fell through to `return nil`, so the guard reported compliance
// having compared nothing. This fixture is the regression that keeps the
// absence branch honest; without it, the branch could be removed again as
// invisibly as the ordering check was.
func TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation(t *testing.T) {
	const fixture = `#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
ensure_envsubst "go-unlocatable"
cd /workspace
output=$(go test ./... 2>&1)
echo "$output"
`
	err := assertEnvsubstWrapperContract("synthetic-unlocatable-invocation.sh", []byte(fixture))
	if err == nil {
		t.Fatalf("guard FALSE NEGATIVE: assertEnvsubstWrapperContract returned nil for a tracked wrapper whose `go test` invocation the matcher cannot locate; a zero match is a LOCATOR failure, not compliance, and returning nil is exactly the BUG-061-013 silent pass")
	}
	if !strings.Contains(err.Error(), "could not LOCATE") {
		t.Fatalf("guard error must say the invocation `could not LOCATE`d, so the reader knows the matcher failed; got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "matcher may need widening") {
		t.Fatalf("guard error must say the `matcher may need widening` rather than blaming the wrapper, so a reader does not waste time auditing a correct wrapper; got: %q", err.Error())
	}
}

// TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest
// proves the restored ordering check has teeth on the invocation form that was
// previously invisible (BUG-061-013, SCN-04).
//
// The fixture is scripts/runtime/go-integration.sh:76's shape — `if ! go test
// … | tee …` — with ensure_envsubst moved AFTER it. It asserts the ORDERING
// error specifically, not merely that some error was returned: if the matcher
// were widened onto something inert, this would surface the locator error
// instead and the test would fail. That distinction is the whole point of the
// assertion.
func TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest(t *testing.T) {
	const fixture = `#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
cd /workspace
if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then
  exit 1
fi
ensure_envsubst "go-late"
`
	err := assertEnvsubstWrapperContract("synthetic-conditional-call-after-go-test.sh", []byte(fixture))
	if err == nil {
		t.Fatalf("guard FALSE NEGATIVE: assertEnvsubstWrapperContract returned nil for a wrapper using the conditional-and-piped invocation form with ensure_envsubst AFTER it; this is the exact shape at scripts/runtime/go-integration.sh:76 whose ordering went unchecked under BUG-061-013")
	}
	if !strings.Contains(err.Error(), "BEFORE the `ensure_envsubst` call") {
		t.Fatalf("guard must reject this fixture with the ORDERING error, proving the matcher located the real `if ! go test` invocation; a locator error here would mean the widening landed on nothing. got: %q", err.Error())
	}
}
