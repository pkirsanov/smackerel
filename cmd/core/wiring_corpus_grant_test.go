package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// Spec 108 Scope 02 — TP-02-01 / TP-02-02.
//
// These are the adversarial proof for SCN-108-C03 and SCN-108-C05. The defect
// class they exist to prevent is a resolver that answers a missing or
// malformed SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT by silently picking a
// stage — the classic `os.Getenv(...)` + `if v == "" { v = "false" }` shape
// banned by .github/instructions/smackerel-no-defaults.instructions.md and by
// R-108-FL5. Restoring any such fallback makes the fail-loud tests below
// return (false, nil) and FAIL.
//
// Each failure assertion checks that the error NAMES the offending variable or
// value. An error that does not tell the operator which key is wrong is the
// second defect these requirements exist to prevent, so "an error occurred" is
// deliberately not sufficient here.

// TestResolveCorpusGrantEnforcement_Observe proves the OBSERVE value resolves
// to false with no error (SCN-108-O01/O02 precondition).
func TestResolveCorpusGrantEnforcement_Observe(t *testing.T) {
	enforce, err := resolveCorpusGrantEnforcement(map[string]string{
		corpusGrantEnforcementEnvVar: "false",
	})
	if err != nil {
		t.Fatalf("expected nil error for %q, got %v", "false", err)
	}
	if enforce {
		t.Fatalf("expected enforce=false (OBSERVE) for %q, got true", "false")
	}
	if got := corpusGrantEnforcementStage(enforce); got != corpusGrantStageObserve {
		t.Fatalf("expected stage %q, got %q", corpusGrantStageObserve, got)
	}
}

// TestResolveCorpusGrantEnforcement_Enforce proves the ENFORCE value resolves
// to true with no error.
func TestResolveCorpusGrantEnforcement_Enforce(t *testing.T) {
	enforce, err := resolveCorpusGrantEnforcement(map[string]string{
		corpusGrantEnforcementEnvVar: "true",
	})
	if err != nil {
		t.Fatalf("expected nil error for %q, got %v", "true", err)
	}
	if !enforce {
		t.Fatalf("expected enforce=true (ENFORCE) for %q, got false", "true")
	}
	if got := corpusGrantEnforcementStage(enforce); got != corpusGrantStageEnforce {
		t.Fatalf("expected stage %q, got %q", corpusGrantStageEnforce, got)
	}
}

// TestResolveCorpusGrantEnforcement_Absent_FailsLoud is TP-02-01: the env var
// is entirely absent from the resolved environment. The error MUST name
// SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT so the operator knows which SST key
// to set.
func TestResolveCorpusGrantEnforcement_Absent_FailsLoud(t *testing.T) {
	enforce, err := resolveCorpusGrantEnforcement(map[string]string{})
	if err == nil {
		t.Fatalf("expected non-nil error when %s is absent (silent-default regression), got enforce=%v err=nil",
			corpusGrantEnforcementEnvVar, enforce)
	}
	assertNamesEnvVar(t, err.Error())
}

// TestResolveCorpusGrantEnforcement_Empty_FailsLoud is the second half of
// TP-02-01: the key is present but empty (the shape a generated env file would
// carry if the fail-loud ${VAR:?...} emission were downgraded to ${VAR}).
func TestResolveCorpusGrantEnforcement_Empty_FailsLoud(t *testing.T) {
	enforce, err := resolveCorpusGrantEnforcement(map[string]string{
		corpusGrantEnforcementEnvVar: "",
	})
	if err == nil {
		t.Fatalf("expected non-nil error when %s is empty (silent-default regression), got enforce=%v err=nil",
			corpusGrantEnforcementEnvVar, enforce)
	}
	assertNamesEnvVar(t, err.Error())
}

// TestResolveCorpusGrantEnforcement_Malformed_FailsLoud is TP-02-02: any value
// outside the two accepted booleans aborts, and the error names the offending
// value.
//
// Adversarial: the first six cases are values strconv.ParseBool ACCEPTS. If a
// future refactor swaps the exact match for ParseBool, those cases would
// resolve to a stage instead of refusing, silently widening the closed
// two-value vocabulary — and this test fails.
func TestResolveCorpusGrantEnforcement_Malformed_FailsLoud(t *testing.T) {
	parseBoolAccepts := []string{"1", "0", "t", "f", "TRUE", "False"}
	otherMalformed := []string{"observe", "ENFORCE", "yes", "no", "on", "off", "shadow", "dry-run", " true", "true "}

	for _, value := range append(parseBoolAccepts, otherMalformed...) {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			enforce, err := resolveCorpusGrantEnforcement(map[string]string{
				corpusGrantEnforcementEnvVar: value,
			})
			if err == nil {
				t.Fatalf("expected non-nil error for malformed value %q, got enforce=%v err=nil", value, enforce)
			}

			msg := err.Error()
			assertNamesEnvVar(t, msg)
			if !strings.Contains(msg, strconv.Quote(value)) {
				t.Errorf("error message must NAME the offending value %s so the operator can correct it; got: %s",
					strconv.Quote(value), msg)
			}
			for _, stage := range []string{corpusGrantStageObserve, corpusGrantStageEnforce} {
				if !strings.Contains(msg, stage) {
					t.Errorf("error message missing accepted stage %q; got: %s", stage, msg)
				}
			}
		})
	}
}

// assertNamesEnvVar fails the test when the fail-loud message does not name
// the env var or does not carry its spec provenance.
func assertNamesEnvVar(t *testing.T, msg string) {
	t.Helper()
	for _, want := range []string{corpusGrantEnforcementEnvVar, "R-108-FL5", "refusing to start"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing required token %q; got: %s", want, msg)
		}
	}
}

// TestCorpusGrantEnforcementEnv_Absent proves the process-env snapshot omits
// the key entirely when the variable is unset, so the resolver sees a genuine
// absence rather than a collapsed empty string.
func TestCorpusGrantEnforcementEnv_Absent(t *testing.T) {
	prev, hadPrev := os.LookupEnv(corpusGrantEnforcementEnvVar)
	if err := os.Unsetenv(corpusGrantEnforcementEnvVar); err != nil {
		t.Fatalf("failed to unset %s: %v", corpusGrantEnforcementEnvVar, err)
	}
	t.Cleanup(func() {
		if hadPrev {
			if err := os.Setenv(corpusGrantEnforcementEnvVar, prev); err != nil {
				t.Fatalf("failed to restore %s: %v", corpusGrantEnforcementEnvVar, err)
			}
		}
	})

	if _, present := corpusGrantEnforcementEnv()[corpusGrantEnforcementEnvVar]; present {
		t.Fatalf("expected %s to be absent from the snapshot when unset", corpusGrantEnforcementEnvVar)
	}
	if _, err := resolveCorpusGrantEnforcement(corpusGrantEnforcementEnv()); err == nil {
		t.Fatalf("expected startup resolution to refuse when %s is unset", corpusGrantEnforcementEnvVar)
	}
}

// TestCorpusGrantEnforcementEnv_Empty proves an explicitly empty variable is
// captured as present-but-empty and still refuses.
func TestCorpusGrantEnforcementEnv_Empty(t *testing.T) {
	t.Setenv(corpusGrantEnforcementEnvVar, "")

	raw, present := corpusGrantEnforcementEnv()[corpusGrantEnforcementEnvVar]
	if !present || raw != "" {
		t.Fatalf("expected present-but-empty snapshot entry, got present=%v raw=%q", present, raw)
	}
	if _, err := resolveCorpusGrantEnforcement(corpusGrantEnforcementEnv()); err == nil {
		t.Fatalf("expected startup resolution to refuse when %s is empty", corpusGrantEnforcementEnvVar)
	}
}

// TestCorpusGrantEnforcementEnv_Set proves a legal value round-trips from the
// process environment through the snapshot into a resolved stage.
func TestCorpusGrantEnforcementEnv_Set(t *testing.T) {
	t.Setenv(corpusGrantEnforcementEnvVar, "true")

	enforce, err := resolveCorpusGrantEnforcement(corpusGrantEnforcementEnv())
	if err != nil {
		t.Fatalf("expected nil error for a legal process-env value, got %v", err)
	}
	if !enforce {
		t.Fatal("expected enforce=true (ENFORCE) to round-trip from the process environment")
	}
}

// TestCorpusGrantEnforcement_SingleResolutionPointBeforeListenerBind pins the
// R-108-FL6 wiring contract by source scan: the stage is resolved exactly once,
// its error aborts run(), and both happen before the HTTP server is
// constructed or bound. A future refactor that moves the call after the
// listener — or adds a second per-route resolution — fails here.
func TestCorpusGrantEnforcement_SingleResolutionPointBeforeListenerBind(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(src)

	const call = "resolveCorpusGrantEnforcement(corpusGrantEnforcementEnv())"
	if got := strings.Count(body, call); got != 1 {
		t.Fatalf("expected exactly ONE corpus-grant resolution point in main.go (R-108-FL6: no per-route override), found %d", got)
	}

	callIdx := strings.Index(body, call)
	abortIdx := strings.Index(body, `return fmt.Errorf("corpus grant enforcement configuration: %w", err)`)
	if abortIdx < 0 {
		t.Fatal("resolution error is not returned from run() — an unchecked error would let startup continue with no stage selected (SCN-108-C03)")
	}
	if abortIdx < callIdx {
		t.Fatalf("abort (offset %d) must follow the resolution call (offset %d)", abortIdx, callIdx)
	}

	for _, marker := range []string{"srv := &http.Server{", "srv.ListenAndServe()"} {
		markerIdx := strings.Index(body, marker)
		if markerIdx < 0 {
			t.Fatalf("listener marker %q not found in main.go — regression-test target moved; update the test", marker)
		}
		if callIdx >= markerIdx {
			t.Fatalf("corpus-grant resolution (offset %d) MUST precede %q (offset %d): SCN-108-C03 requires that no HTTP listener is bound when the stage cannot be resolved",
				callIdx, marker, markerIdx)
		}
	}
}

// TestCorpusGrantEnforcement_ResolverHasNoDefaultShape is the mechanical
// no-defaults guard for the resolution path. os.Getenv collapses unset and
// empty and is the usual carrier of a hidden fallback; strconv.ParseBool would
// widen the closed two-value vocabulary. Neither belongs in this file.
//
// Comments are stripped before scanning: prose that NAMES a banned shape in
// order to explain why it is banned is documentation, not a use.
func TestCorpusGrantEnforcement_ResolverHasNoDefaultShape(t *testing.T) {
	src, err := os.ReadFile("wiring_corpus_grant.go")
	if err != nil {
		t.Fatalf("read wiring_corpus_grant.go: %v", err)
	}

	var code strings.Builder
	for _, line := range strings.Split(string(src), "\n") {
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		code.WriteString(line)
		code.WriteString("\n")
	}
	body := code.String()

	for _, banned := range []string{"os.Getenv(", "strconv.ParseBool"} {
		if strings.Contains(body, banned) {
			t.Errorf("%q must not appear in the corpus-grant resolution path (smackerel-no-defaults / R-108-FL5)", banned)
		}
	}
	if !strings.Contains(body, "os.LookupEnv(") {
		t.Error("the resolution path must read the environment with os.LookupEnv so absent and empty stay distinguishable")
	}
}
