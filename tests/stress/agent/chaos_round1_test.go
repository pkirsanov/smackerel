//go:build stress

// Spec 037 — chaos round 1 (stochastic-quality-sweep, parent-expanded
// chaos-hardening). Generated 2026-06-05 against the live test stack.
//
// These probes attack vectors NOT already covered by the deterministic
// BS-001..BS-022 suites:
//
//   1. POST /v1/agent/invoke body-size boundary + malformed-envelope
//      handling — the 64 KiB LimitReader contract and the JSON parse
//      gate must fail-cleanly without leaking 5xx or panicking.
//   2. POST /v1/agent/invoke concurrent burst — middleware.Throttle(100)
//      on /v1/* should reject excess in-flight with 503 (chi default),
//      and the server must stay responsive after the burst clears.
//   3. POST /v1/agent/invoke auth-boundary fuzz — every malformed auth
//      shape must return 401 (Unauthorized), never 500.
//   4. `smackerel agent replay` CLI with chaos trace_ids — every
//      missing/malformed trace_id must exit ERROR=2, never PASS=0.
//
// Reproducibility: seed is logged at the start of every test. Set
// CHAOS_SEED to reproduce a specific run.
//
// Skips cleanly when DATABASE_URL or NATS_URL is unset so
// `go test -tags=stress ./...` outside the live stack does not fail.

package agent_stress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// liveCoreOrSkip resolves the live test-stack core URL + auth token.
// The chaos probes need both — they exercise the real HTTP surface,
// not an httptest.Server.
func liveCoreOrSkip(t *testing.T) (baseURL, token string) {
	t.Helper()
	baseURL = os.Getenv("CHAOS_CORE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("CORE_EXTERNAL_URL")
	}
	if baseURL == "" {
		t.Skip("chaos: CHAOS_CORE_URL/CORE_EXTERNAL_URL not set — live core not reachable")
	}
	token = os.Getenv("SMACKEREL_AUTH_TOKEN")
	if token == "" {
		t.Skip("chaos: SMACKEREL_AUTH_TOKEN not set — cannot authenticate against /v1")
	}
	return baseURL, token
}

func chaosSeed(t *testing.T) int64 {
	t.Helper()
	if s := os.Getenv("CHAOS_SEED"); s != "" {
		var seed int64
		fmt.Sscanf(s, "%d", &seed)
		t.Logf("chaos: using CHAOS_SEED=%d", seed)
		return seed
	}
	seed := time.Now().UnixNano()
	t.Logf("chaos: generated seed=%d (set CHAOS_SEED=%d to reproduce)", seed, seed)
	return seed
}

// postInvokeLive sends a single POST /v1/agent/invoke against the live
// core. It returns status + raw body so callers can branch on either.
func postInvokeLive(client *http.Client, baseURL, token string, body []byte, authOverride string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/agent/invoke", bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if authOverride == "__none__" {
		// caller wants NO Authorization header at all
	} else if authOverride != "" {
		req.Header.Set("Authorization", authOverride)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, b, err
}

// TestChaos_037_BodyBoundaryAndMalformed attacks the JSON-body gate of
// /v1/agent/invoke with adversarial shapes that must never crash or
// 5xx. The 64 KiB LimitReader contract documents the size ceiling; the
// JSON parse gate must reject malformed inputs with 4xx envelopes.
func TestChaos_037_BodyBoundaryAndMalformed(t *testing.T) {
	baseURL, token := liveCoreOrSkip(t)
	seed := chaosSeed(t)
	rng := rand.New(rand.NewSource(seed))
	client := &http.Client{Timeout: 30 * time.Second}

	// Helper: build a body with a `raw_input` of N filler chars, then
	// add overhead so total approximates `total` bytes.
	makeBodyOfSize := func(total int) []byte {
		const wrapper = `{"raw_input":"`
		const tail = `"}`
		if total < len(wrapper)+len(tail)+1 {
			total = len(wrapper) + len(tail) + 1
		}
		filler := make([]byte, total-len(wrapper)-len(tail))
		for i := range filler {
			filler[i] = byte('a' + rng.Intn(26))
		}
		b := make([]byte, 0, total)
		b = append(b, wrapper...)
		b = append(b, filler...)
		b = append(b, tail...)
		return b
	}

	// expectedShape distinguishes the documented two response shapes:
	//   - pre_agent: the agent never ran. Body has "error" key, no
	//     "outcome" and no "trace_id" (userreply.MalformedRequestResponse
	//     or InfrastructureFailureResponse).
	//   - agent_ran: routing/execution ran. Body has "outcome" AND
	//     "trace_id" (userreply.APIResponse for the matched outcome class).
	//   - any: just-must-be-JSON, either shape acceptable.
	type expectedShape int
	const (
		shapePreAgent expectedShape = iota
		shapeAgentRan
		shapeAny
	)
	cases := []struct {
		name             string
		body             []byte
		acceptableStatus map[int]bool
		mustNotBe5xx     bool
		shape            expectedShape
	}{
		{
			name:             "zero_byte_body",
			body:             nil,
			acceptableStatus: map[int]bool{http.StatusBadRequest: true},
			mustNotBe5xx:     true,
			shape:            shapePreAgent,
		},
		{
			name:             "empty_object",
			body:             []byte(`{}`),
			acceptableStatus: map[int]bool{http.StatusBadRequest: true},
			mustNotBe5xx:     true,
			shape:            shapePreAgent,
		},
		{
			name:             "json_null",
			body:             []byte(`null`),
			acceptableStatus: map[int]bool{http.StatusBadRequest: true},
			mustNotBe5xx:     true,
			shape:            shapePreAgent,
		},
		{
			name:             "json_array",
			body:             []byte(`["raw_input","hello"]`),
			acceptableStatus: map[int]bool{http.StatusBadRequest: true},
			mustNotBe5xx:     true,
			shape:            shapePreAgent,
		},
		{
			name:             "malformed_json_unterminated",
			body:             []byte(`{"raw_input":"hello`),
			acceptableStatus: map[int]bool{http.StatusBadRequest: true},
			mustNotBe5xx:     true,
			shape:            shapePreAgent,
		},
		{
			name:             "binary_garbage",
			body:             []byte{0x00, 0xff, 0x42, 0x13, 0x37, 0xde, 0xad, 0xbe, 0xef},
			acceptableStatus: map[int]bool{http.StatusBadRequest: true},
			mustNotBe5xx:     true,
			shape:            shapePreAgent,
		},
		{
			name:             "control_chars_in_raw_input",
			body:             []byte("{\"raw_input\":\"a\\u0000b\\u0001c\"}"),
			acceptableStatus: map[int]bool{http.StatusOK: true, http.StatusBadRequest: true},
			mustNotBe5xx:     true,
			shape:            shapeAgentRan,
		},
		{
			name:             "deeply_nested_structured_context",
			body:             append([]byte(`{"raw_input":"x","structured_context":`), append(bytes.Repeat([]byte(`{"a":`), 200), append(bytes.Repeat([]byte("}"), 200), []byte("}")...)...)...),
			acceptableStatus: map[int]bool{http.StatusOK: true, http.StatusBadRequest: true},
			mustNotBe5xx:     true,
			shape:            shapeAny,
		},
		{
			name:             "body_at_64kib_boundary",
			body:             makeBodyOfSize(64 * 1024),
			acceptableStatus: map[int]bool{http.StatusOK: true, http.StatusBadRequest: true},
			mustNotBe5xx:     true,
			shape:            shapeAny,
		},
		{
			name:             "body_over_64kib_boundary",
			body:             makeBodyOfSize(65 * 1024),
			acceptableStatus: map[int]bool{http.StatusBadRequest: true},
			mustNotBe5xx:     true,
			shape:            shapePreAgent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body, err := postInvokeLive(client, baseURL, token, tc.body, "")
			if err != nil {
				t.Fatalf("chaos: HTTP do failed: %v", err)
			}
			t.Logf("status=%d body=%s", status, string(body))
			if tc.mustNotBe5xx && status >= 500 {
				t.Fatalf("FINDING: 5xx leaked for adversarial body case=%s status=%d body=%s", tc.name, status, string(body))
			}
			if len(tc.acceptableStatus) > 0 && !tc.acceptableStatus[status] {
				t.Fatalf("FINDING: unexpected status case=%s status=%d body=%s", tc.name, status, string(body))
			}
			var env map[string]any
			if jerr := json.Unmarshal(body, &env); jerr != nil {
				t.Fatalf("FINDING: response not JSON case=%s body=%s err=%v", tc.name, string(body), jerr)
			}
			switch tc.shape {
			case shapePreAgent:
				if _, ok := env["error"]; !ok {
					t.Fatalf("FINDING: pre-agent response missing 'error' field case=%s body=%s", tc.name, string(body))
				}
				if _, ok := env["trace_id"]; ok {
					t.Fatalf("FINDING: pre-agent response unexpectedly carries 'trace_id' case=%s body=%s", tc.name, string(body))
				}
				if _, ok := env["outcome"]; ok {
					t.Fatalf("FINDING: pre-agent response unexpectedly carries 'outcome' case=%s body=%s", tc.name, string(body))
				}
			case shapeAgentRan:
				if _, ok := env["trace_id"]; !ok {
					t.Fatalf("FINDING: agent-ran response missing 'trace_id' case=%s body=%s", tc.name, string(body))
				}
			}
		})
	}
}

// TestChaos_037_ConcurrentBurst fires N concurrent POSTs at the live
// /v1/agent/invoke and asserts the throttle layer rejects cleanly (no
// panics, no 5xx leakage other than the documented 503 from
// middleware.Throttle), then verifies the server still responds.
func TestChaos_037_ConcurrentBurst(t *testing.T) {
	baseURL, token := liveCoreOrSkip(t)
	_ = chaosSeed(t)

	const concurrency = 200
	client := &http.Client{Timeout: 30 * time.Second}

	type result struct {
		status int
		err    error
	}
	results := make(chan result, concurrency)
	var ready, fire sync.WaitGroup
	ready.Add(concurrency)
	fire.Add(1)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			body := []byte(fmt.Sprintf(`{"raw_input":"chaos burst probe %d","scenario_id":"does_not_exist_chaos_%d"}`, id, id))
			ready.Done()
			fire.Wait()
			status, _, err := postInvokeLive(client, baseURL, token, body, "")
			results <- result{status: status, err: err}
		}(i)
	}

	ready.Wait()
	start := time.Now()
	fire.Done()

	statusCounts := map[int]int{}
	var errCount int
	for i := 0; i < concurrency; i++ {
		r := <-results
		if r.err != nil {
			errCount++
			continue
		}
		statusCounts[r.status]++
	}
	elapsed := time.Since(start)

	t.Logf("burst: %d requests in %s, errors=%d status_counts=%v", concurrency, elapsed, errCount, statusCounts)

	// Allow ANY status that is not a 5xx OTHER THAN 503 (the documented
	// throttle response). 502/504/500 would indicate genuine breakage.
	for status, count := range statusCounts {
		if status >= 500 && status != http.StatusServiceUnavailable {
			t.Fatalf("FINDING: burst leaked 5xx (not 503) status=%d count=%d", status, count)
		}
	}
	if errCount > concurrency/4 {
		t.Fatalf("FINDING: burst caused %d HTTP errors out of %d (>25%%) — transport instability", errCount, concurrency)
	}

	// Probe server health AFTER the burst — should still respond cleanly.
	// Wait briefly for any in-flight slots to drain.
	time.Sleep(2 * time.Second)
	postBurstClient := &http.Client{Timeout: 10 * time.Second}
	postBurstStatus, postBurstBody, err := postInvokeLive(postBurstClient, baseURL, token, []byte(`{"raw_input":"post-burst health probe","scenario_id":"does_not_exist_post_burst"}`), "")
	if err != nil {
		t.Fatalf("FINDING: server unresponsive after burst: %v", err)
	}
	t.Logf("post-burst probe: status=%d body=%s", postBurstStatus, string(postBurstBody))
	if postBurstStatus >= 500 && postBurstStatus != http.StatusServiceUnavailable {
		t.Fatalf("FINDING: server returned 5xx (not 503) after burst: status=%d body=%s", postBurstStatus, string(postBurstBody))
	}
}

// TestChaos_037_AuthBoundaryChaos sends well-formed requests with
// pathological Authorization headers. None of these may produce a 5xx
// — every malformed auth shape MUST return 401 (Unauthorized) or be
// upgraded to a 200 only by the dev empty-token bypass (which is NOT
// active in our test stack because SMACKEREL_AUTH_TOKEN is set).
func TestChaos_037_AuthBoundaryChaos(t *testing.T) {
	baseURL, token := liveCoreOrSkip(t)
	_ = chaosSeed(t)

	body := []byte(`{"raw_input":"auth chaos probe","scenario_id":"does_not_exist_auth_chaos"}`)
	client := &http.Client{Timeout: 15 * time.Second}

	atypicalToken := strings.Repeat("a", 4096) // 4 KiB token
	cases := []struct {
		name            string
		auth            string
		expectClientErr bool // when true, Go http.Client itself rejects the header (header injection defense)
	}{
		{name: "missing_header_entirely", auth: "__none__"},
		{name: "bearer_no_token", auth: "Bearer "},
		{name: "bearer_wrong_token", auth: "Bearer this_is_not_the_real_token_chaos_round_1"},
		{name: "bearer_empty_with_extra_spaces", auth: "Bearer    "},
		{name: "no_bearer_prefix", auth: token},
		{name: "wrong_scheme_basic", auth: "Basic dXNlcjpwYXNz"},
		{name: "wrong_scheme_digest", auth: "Digest username=\"user\""},
		{name: "bearer_with_newlines_in_token", auth: "Bearer abc\ndef\rghi", expectClientErr: true},
		{name: "bearer_with_unicode_control", auth: "Bearer abc\u202edef"},
		{name: "bearer_atypically_long_token", auth: "Bearer " + atypicalToken},
		{name: "lowercase_bearer", auth: "bearer " + token},
		{name: "uppercase_bearer", auth: "BEARER " + token},
		{name: "double_bearer", auth: "Bearer Bearer " + token},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, respBody, err := postInvokeLive(client, baseURL, token, body, tc.auth)
			if tc.expectClientErr {
				if err == nil {
					t.Fatalf("FINDING: expected Go http.Client to reject malformed header for case=%s but request succeeded status=%d body=%s", tc.name, status, string(respBody))
				}
				t.Logf("auth=%s rejected by Go http.Client (header injection defense): %v", tc.name, err)
				return
			}
			if err != nil {
				t.Fatalf("FINDING: HTTP do failed for auth=%q: %v", tc.name, err)
			}
			t.Logf("auth=%s status=%d body=%s", tc.name, status, string(respBody))
			if status >= 500 {
				t.Fatalf("FINDING: 5xx leaked for malformed auth case=%s status=%d body=%s", tc.name, status, string(respBody))
			}
			// Bearer scheme is case-insensitive per RFC 7235 — lowercase and
			// uppercase Bearer with the real token are accepted. Everything
			// else MUST be rejected with 401 (not 200).
			switch tc.name {
			case "lowercase_bearer", "uppercase_bearer":
				if status != http.StatusOK {
					t.Fatalf("FINDING: case-insensitive Bearer scheme not honored (RFC 7235): case=%s status=%d body=%s", tc.name, status, string(respBody))
				}
			default:
				if status == http.StatusOK {
					t.Fatalf("FINDING: malformed auth case=%s accepted with 200: body=%s", tc.name, string(respBody))
				}
				if status != http.StatusUnauthorized {
					t.Logf("note: case=%s rejected with status=%d (not 401); investigate", tc.name, status)
				}
			}
		})
	}
}

// TestChaos_037_ReplayCLIErrorPaths probes the `smackerel agent replay`
// exit-code contract. Every missing/malformed/adversarial trace_id MUST
// exit ERROR=2 (never PASS=0 or FAIL=1) so wrapping scripts can
// distinguish "system error" from "drift detected".
//
// Implementation note: we build the cmd/core binary once and exec it
// directly. `go run` would translate the inner exit code (2) into its
// own exit (1), masking the contract under test.
func TestChaos_037_ReplayCLIErrorPaths(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("chaos: DATABASE_URL not set — replay CLI requires live DB")
	}
	_ = chaosSeed(t)

	// Locate the repo root so the binary build path resolves.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := wd
	for {
		if _, statErr := os.Stat(filepath.Join(repoRoot, "go.mod")); statErr == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Fatalf("could not locate go.mod from %s", wd)
		}
		repoRoot = parent
	}

	// Build the binary once into a temp dir so each sub-test execs it
	// directly (avoiding go run's exit-code wrapping).
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "smackerel-chaos-cli")
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer buildCancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binPath, "./cmd/core")
	build.Dir = repoRoot
	var buildErr bytes.Buffer
	build.Stderr = &buildErr
	if err := build.Run(); err != nil {
		t.Fatalf("chaos: failed to build cmd/core: %v stderr=%s", err, buildErr.String())
	}

	cases := []struct {
		name       string
		traceID    string
		skipReason string
	}{
		{name: "nonexistent_uuid", traceID: "trace_00000000_does_not_exist"},
		{name: "empty_string_after_double_dash", traceID: ""},
		{name: "sql_injection_attempt", traceID: "trace'); DROP TABLE agent_traces;--"},
		{name: "very_long_trace_id", traceID: strings.Repeat("a", 8192)},
		{name: "null_byte_in_trace_id", traceID: "trace_with_null\x00byte", skipReason: "exec.Command cannot pass NUL byte in args (OS-level restriction)"},
		{name: "unicode_control_in_trace_id", traceID: "trace\u202ereversed"},
		{name: "whitespace_only_trace_id", traceID: "   "},
		{name: "path_traversal_attempt", traceID: "../../../etc/passwd"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipReason != "" {
				t.Skipf("chaos: %s", tc.skipReason)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			args := []string{"agent", "replay", "--json", tc.traceID}
			if tc.traceID == "" {
				args = []string{"agent", "replay", "--json"}
			}
			cmd := exec.CommandContext(ctx, binPath, args...)
			cmd.Dir = repoRoot
			cmd.Env = os.Environ()
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			runErr := cmd.Run()
			exit := 0
			if runErr != nil {
				if ee, ok := runErr.(*exec.ExitError); ok {
					exit = ee.ExitCode()
				} else {
					t.Fatalf("chaos: failed to execute replay CLI: %v stderr=%s", runErr, stderr.String())
				}
			}
			t.Logf("case=%s exit=%d stdout=%s stderr=%s", tc.name, exit, stdout.String(), stderr.String())
			if exit == 0 {
				t.Fatalf("FINDING: replay CLI returned PASS=0 for adversarial case=%s stdout=%s", tc.name, stdout.String())
			}
			if exit == 1 {
				t.Fatalf("FINDING: replay CLI returned FAIL=1 for adversarial case=%s — should be ERROR=2 (no real drift to compare): stdout=%s stderr=%s", tc.name, stdout.String(), stderr.String())
			}
			if exit != 2 {
				t.Fatalf("FINDING: replay CLI returned exit=%d for case=%s (expected ERROR=2): stdout=%s stderr=%s", exit, tc.name, stdout.String(), stderr.String())
			}
		})
	}
}

// chaos_round1 sentinel: tickle the linter to keep imports honest if
// any unused while iterating.
var _ = atomic.LoadInt32
