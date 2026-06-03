//go:build e2e

// Spec 076 SCOPE-6d — TP-076-06-07 / SCN-075-A07.
//
// Live-stack e2e proof: when the spec 075 SST resolves a retired
// command to the WindowClosed branch, ClosedResponseFor returns the
// canonical unknown-command copy loaded from the live SST
// (LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY) verbatim —
// proving the closed-window branch uses the canonical body, never a
// bespoke string. The Policy + ConfigCatalog wiring here mirrors
// the facade exactly; the only override is the SST WindowState the
// resolver is constructed with so the closed branch can be
// exercised without flipping the live SST mid-run.

package legacyretirement_e2e

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

// TestRetirement_ClosedWindowReturnsCanonicalResponse covers
// SCN-075-A07 against the live e2e SST.
func TestRetirement_ClosedWindowReturnsCanonicalResponse(t *testing.T) {
	stack := loadStack(t)
	waitHealthy(t, stack.BaseURL)

	rawClosed := os.Getenv("LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY")
	if rawClosed == "" {
		t.Fatal("LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY not set; live stack is up but SST closed-copy missing — wiring bug")
	}
	var closedCopy map[string]string
	if err := json.Unmarshal([]byte(rawClosed), &closedCopy); err != nil {
		t.Fatalf("decode closed-copy SST: %v", err)
	}
	if len(closedCopy) == 0 {
		t.Fatal("closed-copy SST map is empty — facade cannot serve canonical body")
	}
	rawNotice := os.Getenv("LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND")
	if rawNotice == "" {
		t.Fatal("LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND not set; wiring bug")
	}
	var noticeCopy map[string]string
	if err := json.Unmarshal([]byte(rawNotice), &noticeCopy); err != nil {
		t.Fatalf("decode notice-copy SST: %v", err)
	}
	hmacKey := os.Getenv("LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY")
	if hmacKey == "" {
		t.Fatal("LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY not set; wiring bug")
	}

	cat, err := legacyretirement.NewConfigCatalog(legacyretirement.CatalogConfig{
		NoticeCopyPerCommand:          noticeCopy,
		PostWindowUnknownResponseCopy: closedCopy,
	})
	if err != nil {
		t.Fatalf("NewConfigCatalog: %v", err)
	}
	resolver, err := legacyretirement.NewWindowStateResolver(
		legacyretirement.SSTStateConfig{WindowID: stack.WindowID, WindowState: "closed"},
		legacyretirement.NewStaticPauseStateReader(false),
	)
	if err != nil {
		t.Fatalf("NewWindowStateResolver: %v", err)
	}
	hasher, err := legacyretirement.NewUserBucketHasher(hmacKey)
	if err != nil {
		t.Fatalf("NewUserBucketHasher: %v", err)
	}
	ledger := legacyretirement.NewInMemoryNoticeLedger()
	pol, err := legacyretirement.NewPolicy(legacyretirement.PolicyConfig{
		Catalog:       cat,
		Ledger:        ledger,
		StateResolver: resolver,
		BucketHasher:  hasher,
		WindowID:      stack.WindowID,
		Clock:         time.Now,
	})
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}

	for command, canonical := range closedCopy {
		canonical := strings.TrimSpace(canonical)
		decision, err := pol.Handle(context.Background(), legacyretirement.AssistantTurn{
			UserID:     "tp-076-06-07-user",
			Transport:  "web",
			RawText:    command + " sample input",
			ReceivedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("Policy.Handle %q: %v", command, err)
		}
		if !decision.Matched {
			t.Fatalf("Matched=false for retired command %q", command)
		}
		if decision.EffectiveState != legacyretirement.WindowClosed {
			t.Fatalf("EffectiveState=%q, want closed", decision.EffectiveState)
		}
		if decision.ServeNL {
			t.Errorf("ServeNL=true for closed window %q — facade MUST suppress NL on closed branch", command)
		}
		resp, err := legacyretirement.ClosedResponseFor(decision)
		if err != nil {
			t.Fatalf("ClosedResponseFor %q: %v", command, err)
		}
		if resp.Status != "unavailable" {
			t.Errorf("Status=%q for %q, want unavailable", resp.Status, command)
		}
		if resp.ErrorCause != "retired_command_closed" {
			t.Errorf("ErrorCause=%q for %q, want retired_command_closed", resp.ErrorCause, command)
		}
		if resp.FacadeInvoked {
			t.Errorf("FacadeInvoked=true for closed branch %q — MUST stay false (no facade dispatch)", command)
		}
		if resp.Body != canonical {
			t.Errorf("closed body for %q diverges from SST canonical copy:\n got=%q\nwant=%q", command, resp.Body, canonical)
		}
	}
}
