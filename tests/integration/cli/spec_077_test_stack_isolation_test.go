//go:build integration

// Spec 077 SCOPE-1c — TP-077-01-02 (SCN-077-A07).
//
// Static contract test: the `./smackerel.sh test e2e-ui` lane wrapper
// MUST scope every lifecycle operation to the dedicated Compose
// project (`smackerel-test-e2e-ui`) and MUST install a teardown trap
// that fires on success, failure, AND signal interruption. Together
// these invariants prove the persistent dev stack
// (default Compose project `smackerel`) is never touched by the
// e2e-ui harness.
//
// The check is static because the integration runner has no docker
// socket (see `spec_077_compose_project_test.go` comment). It is
// adversarial: a regression that drops the trap, removes the
// `--project-name` flag, or runs the dev `--env dev down` sequence
// would all fail this test.
package cli

import (
	"regexp"
	"strings"
	"testing"
)

func TestSpec077TestStackIsolation_DevStackUntouched(t *testing.T) {
	wrapper := spec077ReadFile(t, "scripts/runtime/web-e2e-ui.sh")

	t.Run("EXIT trap installed for teardown", func(t *testing.T) {
		if !regexp.MustCompile(`trap\s+'tear_down_test_stack'\s+EXIT`).MatchString(wrapper) {
			t.Fatalf("wrapper must install `trap 'tear_down_test_stack' EXIT` so teardown runs on success and failure")
		}
	})

	t.Run("INT + TERM traps installed for signal teardown", func(t *testing.T) {
		if !regexp.MustCompile(`trap\s+'[^']*tear_down_test_stack[^']*'\s+INT`).MatchString(wrapper) {
			t.Fatalf("wrapper must install an INT trap that calls tear_down_test_stack")
		}
		if !regexp.MustCompile(`trap\s+'[^']*tear_down_test_stack[^']*'\s+TERM`).MatchString(wrapper) {
			t.Fatalf("wrapper must install a TERM trap that calls tear_down_test_stack")
		}
	})

	t.Run("teardown is scoped to the dedicated project via e2e_ui_compose", func(t *testing.T) {
		// tear_down_test_stack MUST go through e2e_ui_compose (which
		// pins `--project-name smackerel-test-e2e-ui`). A direct
		// `docker compose down` without --project-name would inherit
		// the env-file `COMPOSE_PROJECT=smackerel-test` and wipe the
		// integration/e2e lane's state.
		fnBody := spec077extractFunction(t, wrapper, "tear_down_test_stack")
		if !strings.Contains(fnBody, "e2e_ui_compose down") {
			t.Fatalf("tear_down_test_stack body must invoke `e2e_ui_compose down` (scoped to --project-name); body:\n%s", fnBody)
		}
		if strings.Contains(fnBody, "smackerel.sh --env dev") {
			t.Fatalf("tear_down_test_stack must NOT touch the dev stack (`./smackerel.sh --env dev ...`); body:\n%s", fnBody)
		}
		if strings.Contains(fnBody, "smackerel.sh --env test down") {
			t.Fatalf("tear_down_test_stack must NOT invoke `./smackerel.sh --env test down` (would tear down the integration/e2e lane under `smackerel-test`); body:\n%s", fnBody)
		}
	})

	t.Run("bring_up does NOT invoke the integration/e2e lane lifecycle", func(t *testing.T) {
		// Adversarial: a wrapper that delegated bring-up to
		// `./smackerel.sh --env test up` would inherit the
		// `smackerel-test` project and collide with the
		// integration/e2e lane.
		fnBody := spec077extractFunction(t, wrapper, "bring_up_test_stack")
		if strings.Contains(fnBody, "smackerel.sh --env test up") {
			t.Fatalf("bring_up_test_stack must NOT invoke `./smackerel.sh --env test up` (would inherit the `smackerel-test` project); body:\n%s", fnBody)
		}
		if strings.Contains(fnBody, "smackerel.sh --env dev") {
			t.Fatalf("bring_up_test_stack must NOT touch the dev stack; body:\n%s", fnBody)
		}
		if !strings.Contains(fnBody, "e2e_ui_compose up") {
			t.Fatalf("bring_up_test_stack must invoke `e2e_ui_compose up` (scoped to --project-name); body:\n%s", fnBody)
		}
	})

	t.Run("SMACKEREL_BASE_URL is derived from SST CORE_EXTERNAL_URL (no hardcoded default)", func(t *testing.T) {
		fnBody := spec077extractFunction(t, wrapper, "bring_up_test_stack")
		if !strings.Contains(fnBody, `smackerel_env_value "$SMACKEREL_E2E_UI_ENV_FILE" "CORE_EXTERNAL_URL"`) {
			t.Fatalf("bring_up must source CORE_EXTERNAL_URL from the SST env file via smackerel_env_value; body:\n%s", fnBody)
		}
		if !strings.Contains(fnBody, `export SMACKEREL_BASE_URL="$core_url"`) {
			t.Fatalf("bring_up must export SMACKEREL_BASE_URL=$core_url (Playwright config consumer); body:\n%s", fnBody)
		}
		// Adversarial: NO `??`, NO `||`, NO hardcoded localhost.
		if regexp.MustCompile(`SMACKEREL_BASE_URL\s*=\s*"?(http://localhost|http://127\.0\.0\.1)`).MatchString(fnBody) {
			t.Fatalf("bring_up must NOT hardcode SMACKEREL_BASE_URL to localhost; body:\n%s", fnBody)
		}
		if regexp.MustCompile(`SMACKEREL_BASE_URL[^=]*:-`).MatchString(fnBody) {
			t.Fatalf("bring_up must NOT use `${SMACKEREL_BASE_URL:-...}` fallback (NO-DEFAULTS SST policy); body:\n%s", fnBody)
		}
	})
}

// spec077extractFunction returns the body of a top-level bash
// function definition `name() { ... }` from the wrapper source, by
// brace-balancing. Crude but sufficient for the lane wrapper which
// uses unbalanced-quote-free bodies.
func spec077extractFunction(t *testing.T, src, name string) string {
	t.Helper()
	idx := strings.Index(src, name+"() {")
	if idx < 0 {
		t.Fatalf("function %s not found in wrapper source", name)
	}
	depth := 0
	start := -1
	for i := idx; i < len(src); i++ {
		switch src[i] {
		case '{':
			if start < 0 {
				start = i + 1
			}
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start:i]
			}
		}
	}
	t.Fatalf("function %s: unbalanced braces in wrapper source", name)
	return ""
}
