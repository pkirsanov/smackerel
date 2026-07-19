//go:build e2e

// Spec 075 SCOPE-1 — TP-075-04 / SCN-075-A11.
//
// Live-stack e2e proof that the residual telemetry surface is shaped
// so user identity can ONLY appear as the HMAC user bucket, never as
// a raw user identifier. The test scrapes /metrics from the running
// test-stack core, locates every line of the
// `smackerel_legacy_command_residual_total` family (HELP, TYPE, and
// any sample lines), and asserts:
//
//   1. The metric is registered (HELP / TYPE lines present).
//   2. The label set declared in the HELP/sample lines is exactly
//      {command, user_bucket} — no raw-id-shaped labels.
//   3. If any samples exist, every user_bucket value is the
//      64-character lowercase hex shape produced by HMAC-SHA256;
//      no raw id format (e.g. integer telegram chat id, "u_*"
//      strings) is permitted as a value.
//
// Scope split: this Scope 1 test asserts the privacy *shape* of the
// metric. Scope 3 wires the actual increment path, at which point
// the same assertions will run against real residual usage.

package assistant_e2e

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func legacyRetirementE2EBaseURL(t *testing.T) string {
	t.Helper()
	base := os.Getenv("CORE_EXTERNAL_URL")
	if base == "" {
		t.Fatal("spec 075 e2e test requires CORE_EXTERNAL_URL — run via `./smackerel.sh test e2e` which brings up the live test stack and exports CORE_EXTERNAL_URL")
	}
	return strings.TrimRight(base, "/")
}

func scrapeMetrics(t *testing.T, base string) string {
	t.Helper()
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/metrics returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /metrics body: %v", err)
	}
	return string(body)
}

// TestLegacyResidualTelemetry_LiveMetricsExposeBucketsOnly is the
// SCN-075-A11 live-stack regression. It is non-tautological: it
// fails if (a) the metric family is not registered, (b) any
// raw-id-shaped label name appears, or (c) any sample carries a
// user_bucket value that is not the 64-char hex HMAC shape.
func TestLegacyResidualTelemetry_LiveMetricsExposeBucketsOnly(t *testing.T) {
	stack := loadLegacyRetirementNoticeLiveStack(t)
	if stack.WindowState != "open" {
		t.Fatalf("LEGACY_RETIREMENT_WINDOW_STATE=%q, want explicit test capability open", stack.WindowState)
	}
	waitLegacyRetirementNoticeReady(t, stack)
	turnID := fmt.Sprintf("bug-075-001-residual-metric-%d", time.Now().UnixNano())
	resp, responseBody := postNoticeAssistantTurn(t, stack, stack.RetiredCmd, turnID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("retired-command turn status=%d, want 200; body=%s", resp.StatusCode, responseBody)
	}

	body := scrapeMetrics(t, legacyRetirementE2EBaseURL(t))

	const metricName = "smackerel_legacy_command_residual_total"

	helpRE := regexp.MustCompile(`(?m)^# HELP ` + regexp.QuoteMeta(metricName) + `\b`)
	typeRE := regexp.MustCompile(`(?m)^# TYPE ` + regexp.QuoteMeta(metricName) + `\b`)
	if !helpRE.MatchString(body) {
		t.Fatalf("/metrics is missing the HELP line for %q; metric is not registered. A regression that removed the init() in internal/assistant/legacyretirement/telemetry.go would trip this.", metricName)
	}
	if !typeRE.MatchString(body) {
		t.Fatalf("/metrics is missing the TYPE line for %q", metricName)
	}

	forbiddenLabelNames := []string{
		`user_id="`,
		`user="`,
		`telegram_chat_id="`,
		`chat_id="`,
		`raw_id="`,
		`username="`,
	}
	for _, label := range forbiddenLabelNames {
		// Confine the search to lines that begin with the metric
		// name so we do not false-fire on unrelated metric series.
		for _, line := range strings.Split(body, "\n") {
			if !strings.HasPrefix(line, metricName) {
				continue
			}
			if strings.Contains(line, label) {
				t.Errorf("metric %q line carries forbidden raw-id label %q; privacy invariant violated: %s", metricName, label, line)
			}
		}
	}

	// The live turn above must materialize at least one sample. Every
	// sample has exactly the closed {command,user_bucket} label set,
	// and every user_bucket is the 64-char lowercase-hex HMAC-SHA256
	// digest for the authenticated test actor.
	bucketRE := regexp.MustCompile(`user_bucket="([^"]*)"`)
	hexRE := regexp.MustCompile(`^[0-9a-f]{64}$`)
	sampleCount := 0
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, metricName) {
			continue
		}
		sampleCount++
		openBrace := strings.IndexByte(line, '{')
		closeBrace := strings.IndexByte(line, '}')
		if openBrace < 0 || closeBrace <= openBrace {
			t.Errorf("metric %q sample has no label set: %s", metricName, line)
			continue
		}
		labels := strings.Split(line[openBrace+1:closeBrace], ",")
		if len(labels) != 2 || !strings.HasPrefix(labels[0], `command="`) || !strings.HasPrefix(labels[1], `user_bucket="`) {
			t.Errorf("metric %q sample labels=%q, want exactly command,user_bucket: %s", metricName, labels, line)
		}
		matches := bucketRE.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			val := m[1]
			if !hexRE.MatchString(val) {
				t.Errorf("metric %q sample carries non-HMAC user_bucket value %q; only 64-char lowercase hex (HMAC-SHA256) is permitted: %s", metricName, val, line)
			}
		}
	}
	if sampleCount == 0 {
		t.Fatalf("metric %q has no samples after a successful retired-command turn", metricName)
	}
}
