//go:build integration

// Spec 077 SCOPE-1c — TP-077-01-05 (SCN-077-A07).
//
// Static contract test: the `./smackerel.sh test e2e-ui` lane MUST
// declare and use a dedicated Compose project name
// `smackerel-test-e2e-ui` that is distinct from every other
// `./smackerel.sh test <category>` lane. This is the mechanical
// invariant that guarantees container/volume/port isolation between
// the PWA e2e-ui harness and the persistent dev stack
// (`smackerel`) + the Go integration/e2e/stress lanes
// (`smackerel-test`).
//
// The check is intentionally static (file-level): the integration
// runner executes inside a sandboxed `golang:1.25.10-bookworm`
// container with no docker socket, so it cannot observe live
// container lifecycle on the host. The static contract is
// adversarial — if a future edit silently swaps the project name to
// `smackerel-test` (collision with the existing test stack) or drops
// the `--project-name` override, this test fails immediately.
package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const spec077ExpectedProjectName = "smackerel-test-e2e-ui"

func TestSpec077TestStackUsesDedicatedComposeProject(t *testing.T) {
	wrapper := spec077ReadFile(t, "scripts/runtime/web-e2e-ui.sh")

	t.Run("declares the dedicated Compose project constant", func(t *testing.T) {
		want := `SMACKEREL_E2E_UI_COMPOSE_PROJECT="` + spec077ExpectedProjectName + `"`
		if !strings.Contains(wrapper, want) {
			t.Fatalf("scripts/runtime/web-e2e-ui.sh must declare %s; not found", want)
		}
	})

	t.Run("project name is NOT the integration/e2e/stress lane name", func(t *testing.T) {
		// Adversarial: a regression that drops the `-e2e-ui` suffix
		// would collide with the existing `smackerel-test` project
		// owned by `./smackerel.sh test integration|e2e|stress`. The
		// `--env-file` would then point at the same env file and
		// `down --volumes` would wipe the other lane's state.
		bad := `SMACKEREL_E2E_UI_COMPOSE_PROJECT="smackerel-test"`
		if strings.Contains(wrapper, bad) {
			t.Fatalf("e2e-ui Compose project must NOT be `smackerel-test` (collides with the integration/e2e/stress lane); found: %s", bad)
		}
		badDev := `SMACKEREL_E2E_UI_COMPOSE_PROJECT="smackerel"`
		if strings.Contains(wrapper, badDev) {
			t.Fatalf("e2e-ui Compose project must NOT be `smackerel` (collides with the persistent dev stack); found: %s", badDev)
		}
	})

	t.Run("docker compose invocations pass --project-name with the dedicated value", func(t *testing.T) {
		// docker compose's `--project-name` flag overrides the env-file
		// `COMPOSE_PROJECT` value. If the wrapper omits the flag, the
		// stack comes up under `smackerel-test` (from
		// `config/generated/test.env`) and collides with the
		// integration/e2e/stress lane — silently breaking isolation.
		pattern := regexp.MustCompile(`(?s)docker compose\s*\\\s*\n\s*--project-name\s+"\$SMACKEREL_E2E_UI_COMPOSE_PROJECT"`)
		if !pattern.MatchString(wrapper) {
			t.Fatalf("e2e_ui_compose helper must invoke `docker compose --project-name \"$SMACKEREL_E2E_UI_COMPOSE_PROJECT\"`; pattern not found in wrapper")
		}
	})

	t.Run("--print-compose-project introspection returns the dedicated value", func(t *testing.T) {
		// The dispatcher canary (TP-077-01-04, SCOPE-1a) already
		// asserts this end-to-end via `./smackerel.sh test e2e-ui
		// --print-compose-project`. Here we pin the static contract
		// inside the wrapper so a regression in the introspection arm
		// is caught at integration time too.
		if !strings.Contains(wrapper, `printf '%s\n' "$SMACKEREL_E2E_UI_COMPOSE_PROJECT"`) {
			t.Fatalf("wrapper must short-circuit `--print-compose-project` and print SMACKEREL_E2E_UI_COMPOSE_PROJECT")
		}
	})

	t.Run("dispatcher arm routes e2e-ui to the lane wrapper", func(t *testing.T) {
		dispatcher := spec077ReadFile(t, "smackerel.sh")
		if !strings.Contains(dispatcher, `exec bash "$SCRIPT_DIR/scripts/runtime/web-e2e-ui.sh"`) {
			t.Fatalf("smackerel.sh e2e-ui arm must exec scripts/runtime/web-e2e-ui.sh (lane delegation)")
		}
	})
}

func spec077ReadFile(t *testing.T, repoRelPath string) string {
	t.Helper()
	root := spec077RepoRoot(t)
	path := filepath.Join(root, repoRelPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func spec077RepoRoot(t *testing.T) string {
	t.Helper()
	// The integration runner mounts the repo at /workspace and sets
	// `-w /workspace`; per-package test CWD is the package dir, so
	// climb until docker-compose.yml appears (works whether the test
	// is run via `./smackerel.sh test integration` inside the runner
	// or directly on the host).
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root (no docker-compose.yml found ascending from %q)", dir)
	return ""
}
