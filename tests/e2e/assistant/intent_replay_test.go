//go:build e2e

// Spec 071 SCOPE-03 — Replay CLI E2E (SCN-071-A04).
//
// Drives the LIVE `smackerel-core assistant replay-intent <trace_id>`
// CLI binary against the live test-stack Postgres. Asserts:
//
//   1. A seeded full v1 trace round-trips through the CLI: exit
//      code 0 and stdout is a JSON ReplayComparison with
//      Match.RouteDecision = Match.ToolCalls = true and
//      side_effects_invoked = false.
//   2. An unknown trace_id returns exit code 2
//      (`intent_trace_not_found`) and prints the canonical
//      error vocabulary to stderr.
//   3. The CLI never mutates the live store (row count for the test
//      namespace is unchanged after the invocation).
//
// Skip policy: legitimate "no live stack" skip when
// SMACKEREL_TEST_ENV_FILE and DATABASE_URL are both unset (matches
// the test-stack harness contract used by spec 065's micro-tools
// fail-loud e2e). Per NO-DEFAULTS: a partial environment (one of
// SMACKEREL_TEST_ENV_FILE / DATABASE_URL set but not the other) is
// a wiring bug and fails the test.

package assistant_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/intenttrace"
)

func intentReplayRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("go.mod not found walking up from %s", file)
	return ""
}

func intentReplayResolveLiveEnv(t *testing.T) (envFile, dbURL string) {
	t.Helper()
	envFile = os.Getenv("SMACKEREL_TEST_ENV_FILE")
	dbURL = os.Getenv("DATABASE_URL")
	if envFile == "" && dbURL == "" {
		t.Skip("e2e: neither SMACKEREL_TEST_ENV_FILE nor DATABASE_URL set — live test stack not available")
	}
	if envFile == "" || dbURL == "" {
		t.Fatalf("e2e: partial test env — SMACKEREL_TEST_ENV_FILE=%q DATABASE_URL=%q (must be both set or both unset)", envFile, dbURL)
	}
	return envFile, dbURL
}

func intentReplaySeedRow(t *testing.T, pool *pgxpool.Pool, traceID, turnID string) {
	t.Helper()
	store := intenttrace.NewPostgresStore(pool)
	recorder := intenttrace.NewStoreRecorder(store, 24*time.Hour).WithExporter(intenttrace.NopExporter{})
	conf := 0.91
	in := intenttrace.TurnTraceInput{
		TraceID:             traceID,
		TurnID:              turnID,
		UserIDHash:          "deadbeefdeadbeef",
		Transport:           intenttrace.TransportWeb,
		TransportMessageID:  "e2e-replay",
		CompilerInvoked:     true,
		Sampled:             true,
		ActionClass:         "external_lookup",
		SideEffectClass:     "external_read",
		Confidence:          &conf,
		RouteDecision:       "scenarios/weather",
		ToolCalls:           []intenttrace.ToolCallSummary{{Name: "weather.lookup", ArgumentsRedacted: true, Outcome: "ok"}},
		FinalResponseStatus: intenttrace.StatusCheckingWeather,
		SlotsRedactionSummary: intenttrace.SlotsRedactionSummary{
			RawText:     "absent",
			SlotClasses: map[string]string{"location": "safe"},
		},
		EmittedAt: time.Now().UTC(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := recorder.Record(ctx, in); err != nil {
		t.Fatalf("seed Record: %v", err)
	}
}

// runReplayCLI builds and runs `smackerel-core assistant replay-intent
// <traceID>` with the test-stack env file loaded. Executing the built
// binary preserves the command's closed exit-code contract; `go run`
// collapses every non-zero program exit to wrapper exit 1.
// Returns (exitCode, stdout, stderr). The CLI exit codes mirror the
// design.md §"CLI Contract" table.
func runReplayCLI(t *testing.T, repoRoot, envFile, traceID string) (int, string, string) {
	t.Helper()
	envLines, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env file %s: %v", envFile, err)
	}
	env := os.Environ()
	for _, line := range strings.Split(string(envLines), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		env = append(env, line)
	}
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer buildCancel()
	binaryPath := filepath.Join(t.TempDir(), "smackerel-core")
	// -buildvcs=false: this is a throwaway test-harness build of the CLI,
	// not a release artifact, so VCS stamping is unnecessary. Disabling it
	// keeps the build deterministic and avoids "error obtaining VCS status:
	// exit status 128 / Use -buildvcs=false to disable VCS stamping." when
	// git refuses to stamp the container-mounted repo tree (dubious
	// ownership: the mounted checkout is owned by a different uid than the
	// e2e container user). The replay contract under test is exit-code and
	// route/tool behavior, none of which depends on embedded VCS metadata.
	build := exec.CommandContext(buildCtx, "go", "build", "-buildvcs=false", "-o", binaryPath, "./cmd/core")
	build.Dir = repoRoot
	var buildStderr bytes.Buffer
	build.Stderr = &buildStderr
	if err := build.Run(); err != nil {
		t.Fatalf("build replay CLI: %v\nstderr: %s", err, buildStderr.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "assistant", "replay-intent", traceID)
	cmd.Dir = repoRoot
	cmd.Env = env
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("replay CLI failed (non-exit error): %v\nstderr: %s", err, errBuf.String())
		}
	}
	return code, outBuf.String(), errBuf.String()
}

// TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects —
// SCN-071-A04 (e2e-api row).
func TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects(t *testing.T) {
	envFile, dbURL := intentReplayResolveLiveEnv(t)
	repoRoot := intentReplayRepoRoot(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	ns := fmt.Sprintf("spec071-a04-e2e-%d", time.Now().UnixNano())
	traceID := ns + "-trace"
	turnID := ns + "-turn"
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%")
	})
	intentReplaySeedRow(t, pool, traceID, turnID)

	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%").Scan(&before); err != nil {
		t.Fatalf("count before: %v", err)
	}

	code, stdout, stderr := runReplayCLI(t, repoRoot, envFile, traceID)
	if code != 0 {
		t.Fatalf("replay-intent exit=%d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	var got intenttrace.ReplayComparison
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\nstdout: %s", err, stdout)
	}
	if got.TraceID != traceID {
		t.Fatalf("trace_id = %q, want %q", got.TraceID, traceID)
	}
	if !got.ReadOnly {
		t.Fatalf("read_only=false")
	}
	if got.SideEffectsInvoked {
		t.Fatalf("side_effects_invoked=true (must be false)")
	}
	if !got.Match.RouteDecision || !got.Match.ToolCalls {
		t.Fatalf("match=%+v, want true/true", got.Match)
	}

	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%").Scan(&after); err != nil {
		t.Fatalf("count after: %v", err)
	}
	if before != after {
		t.Fatalf("replay-intent mutated row count: before=%d after=%d (must be equal)", before, after)
	}
}

// TestIntentReplayE2E_UnknownTraceIDExits2 — guards the design.md
// CLI exit-code contract row for `intent_trace_not_found`.
func TestIntentReplayE2E_UnknownTraceIDExits2(t *testing.T) {
	envFile, _ := intentReplayResolveLiveEnv(t)
	repoRoot := intentReplayRepoRoot(t)

	traceID := fmt.Sprintf("spec071-a04-e2e-missing-%d", time.Now().UnixNano())
	code, stdout, stderr := runReplayCLI(t, repoRoot, envFile, traceID)
	if code != 2 {
		t.Fatalf("missing trace exit=%d, want 2\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "intent_trace_not_found") {
		t.Fatalf("stderr did not name intent_trace_not_found vocabulary:\n%s", stderr)
	}
}
