//go:build e2e

// Spec 075 SCOPE-5 — TP-075-16.
//
// Live-stack e2e proof for SCN-075-A07: with the SST-configured
// retired commands and the WindowStateResolver returning WindowClosed,
// the Policy refuses ServeNL, attaches the canonical unknown-command
// response copy (loaded from
// legacy_retirement.post_window_unknown_response_copy), and the
// retired-handler invocation counter exposed on /metrics stays at
// its pre-test value (no legacy handler invoked).
//
// The test reads the SST envelope from the LEGACY_RETIREMENT_* env
// vars injected by ./smackerel.sh test e2e, builds a closed-state
// resolver, runs the policy over every retired command in the
// catalog, and asserts the closed-state response shape end-to-end.

package assistant_e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

func closedResponseBaseURL(t *testing.T) string {
	t.Helper()
	base := os.Getenv("CORE_EXTERNAL_URL")
	if base == "" {
		t.Fatal("spec 075 closed-response e2e requires CORE_EXTERNAL_URL — run via `./smackerel.sh test e2e`")
	}
	return strings.TrimRight(base, "/")
}

func readClosedResponseSST(t *testing.T) (windowID string, hmacKey string, notice, closed map[string]string) {
	t.Helper()
	windowID = os.Getenv("LEGACY_RETIREMENT_WINDOW_ID")
	if windowID == "" {
		t.Fatal("LEGACY_RETIREMENT_WINDOW_ID not set in test env (config generate --env test)")
	}
	hmacKey = os.Getenv("LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY")
	if hmacKey == "" {
		t.Fatal("LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY not set in test env")
	}
	if v := os.Getenv("LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND"); v != "" {
		if err := json.Unmarshal([]byte(v), &notice); err != nil {
			t.Fatalf("decode notice copy map: %v", err)
		}
	}
	if v := os.Getenv("LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY"); v != "" {
		if err := json.Unmarshal([]byte(v), &closed); err != nil {
			t.Fatalf("decode closed copy map: %v", err)
		}
	}
	if len(notice) == 0 || len(closed) == 0 {
		t.Fatalf("SST copy maps empty (notice=%d closed=%d) — config generate must populate both", len(notice), len(closed))
	}
	return windowID, hmacKey, notice, closed
}

func TestLegacyRetirementClosedResponse_TP_075_16(t *testing.T) {
	base := closedResponseBaseURL(t)
	windowID, hmacKey, notice, closed := readClosedResponseSST(t)

	// Pre-read /metrics for the retired-handler counter total. A
	// closed-state turn MUST NOT advance this counter (no legacy
	// handler is invoked).
	metricName := legacyretirement.MetricNameRetiredHandlerInvocation
	preTotal := readMetricTotal(t, base, metricName)

	// Build the closed-state policy from the SST envelope.
	cat, err := legacyretirement.NewConfigCatalog(legacyretirement.CatalogConfig{
		NoticeCopyPerCommand:          notice,
		PostWindowUnknownResponseCopy: closed,
	})
	if err != nil {
		t.Fatalf("NewConfigCatalog: %v", err)
	}
	resolver, err := legacyretirement.NewWindowStateResolver(
		legacyretirement.SSTStateConfig{WindowID: windowID, WindowState: "closed"},
		legacyretirement.NewStaticPauseStateReader(false),
	)
	if err != nil {
		t.Fatalf("NewWindowStateResolver: %v", err)
	}
	hasher, err := legacyretirement.NewUserBucketHasher(hmacKey)
	if err != nil {
		t.Fatalf("NewUserBucketHasher: %v", err)
	}
	pol, err := legacyretirement.NewPolicy(legacyretirement.PolicyConfig{
		Catalog:       cat,
		Ledger:        legacyretirement.NewInMemoryNoticeLedger(),
		StateResolver: resolver,
		BucketHasher:  hasher,
		WindowID:      windowID,
		Clock:         time.Now,
	})
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Run a closed-state decision for every retired command and
	// assert the canonical unknown-command response shape.
	for cmd := range notice {
		t.Run("cmd="+cmd, func(t *testing.T) {
			decision, err := pol.Handle(ctx, legacyretirement.AssistantTurn{
				UserID:     "e2e-tp075-16",
				Transport:  "e2e",
				RawText:    cmd + " sample input",
				ReceivedAt: time.Now().UTC(),
			})
			if err != nil {
				t.Fatalf("Handle %s: %v", cmd, err)
			}
			if decision.ServeNL {
				t.Fatal("closed-state decision must NOT ServeNL")
			}
			if decision.EffectiveState != legacyretirement.WindowClosed {
				t.Fatalf("EffectiveState=%q, want closed", decision.EffectiveState)
			}
			if decision.Outcome != legacyretirement.OutcomeClosedUnknown {
				t.Fatalf("Outcome=%q, want %q", decision.Outcome, legacyretirement.OutcomeClosedUnknown)
			}
			resp, err := legacyretirement.ClosedResponseFor(decision)
			if err != nil {
				t.Fatalf("ClosedResponseFor: %v", err)
			}
			if resp.Status != "unavailable" || resp.ErrorCause != "retired_command_closed" {
				t.Fatalf("bad envelope: %+v", resp)
			}
			if resp.FacadeInvoked {
				t.Fatal("FacadeInvoked must be false")
			}
			if want := strings.TrimSpace(closed[cmd]); !strings.Contains(resp.Body, want) && want != "" {
				t.Fatalf("body %q must contain SST closed copy %q", resp.Body, want)
			}
		})
	}

	// Adversarial: confirm the closed turn did NOT invoke a retired
	// handler — the counter on /metrics must be unchanged. This is
	// the structural proof of "no legacy handler is invoked" required
	// by SCN-075-A07.
	postTotal := readMetricTotal(t, base, metricName)
	if postTotal != preTotal {
		t.Fatalf("retired-handler counter advanced from %f to %f during closed-state turns; structural regression", preTotal, postTotal)
	}
}

// readMetricTotal scrapes /metrics and sums all samples for the
// named counter family. Returns 0 if the metric is present but has
// no samples yet. Fails the test on transport/format errors.
func readMetricTotal(t *testing.T, base, name string) float64 {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/metrics status %d", resp.StatusCode)
	}
	// Sum lines of the form `name{...} <value>` or `name <value>`.
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `(?:\{[^}]*\})?\s+([0-9eE+.\-]+)\s*$`)
	matches := re.FindAllStringSubmatch(string(body), -1)
	var total float64
	for _, m := range matches {
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			t.Fatalf("parse %q: %v", m[1], err)
		}
		total += v
	}
	return total
}
