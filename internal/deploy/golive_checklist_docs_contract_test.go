// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Spec 082 SCOPE-082-08 — operator Go-Live Readiness Checklist docs contract.
//
// A holistic MVP → <deploy-host> readiness review found that the go-live
// dependencies (spec 051 secrets, spec 052 L2 knb secret injection, spec 017
// local-operator vs CI trust, Compose profile enablement, backup/restore
// sequencing, supervised canary) were scattered across spec internals with no
// single operator-facing checklist. SCOPE-082-08 adds one consolidated
// "Go-Live Readiness Checklist" section to docs/Deployment.md.
//
// This contract pins the checklist's required anchors so a future docs edit
// cannot silently drop one of the load-bearing go-live steps. The adversarial
// sub-test proves a missing anchor is rejected.
//
// Cross-reference:
//   - specs/082-mvp-target-readiness-hardening/spec.md FR-082-008
//   - specs/082-mvp-target-readiness-hardening/scenario-manifest.json SCN-082-H01
//   - docs/Deployment.md ("Go-Live Readiness Checklist")
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goLiveChecklistAnchors are the substrings the Go-Live Readiness Checklist
// MUST contain. Each maps a load-bearing go-live dependency to a stable
// anchor string.
var goLiveChecklistAnchors = []struct {
	what   string
	anchor string
}{
	{"section heading", "Go-Live Readiness Checklist"},
	{"spec 051 secret POSTGRES_PASSWORD", "POSTGRES_PASSWORD"},
	{"spec 051 secret AUTH_SIGNING_ACTIVE_PRIVATE_KEY", "AUTH_SIGNING_ACTIVE_PRIVATE_KEY"},
	{"spec 051 secret AUTH_SIGNING_ACTIVE_KEY_ID", "AUTH_SIGNING_ACTIVE_KEY_ID"},
	{"spec 051 secret AUTH_AT_REST_HASHING_KEY", "AUTH_AT_REST_HASHING_KEY"},
	{"spec 051 secret AUTH_BOOTSTRAP_TOKEN", "AUTH_BOOTSTRAP_TOKEN"},
	{"spec 051 reference", "spec 051"},
	{"spec 052 L2 injection reference", "spec 052"},
	{"spec 017 local-operator trust reference", "spec 017"},
	{"local-operator trust model", "local-operator"},
	{"ollama profile enablement", "--profile ollama"},
	{"monitoring profile enablement", "--profile monitoring"},
	{"searxng profile enablement", "--profile searxng"},
	{"backup sequencing", "restore-drill"},
	{"promote gate G112 (backup-freshness)", "G112"},
	{"promote gate G113 (restore-drill currency)", "G113"},
	{"supervised canary first apply", "supervised canary"},
}

// assertGoLiveChecklist returns nil iff every required anchor is present in
// the supplied docs content. Returns a non-nil error naming the first
// missing anchor.
func assertGoLiveChecklist(content string) error {
	for _, a := range goLiveChecklistAnchors {
		if !strings.Contains(content, a.anchor) {
			return fmt.Errorf("contract violation: docs/Deployment.md Go-Live Readiness Checklist is missing the %s anchor %q — SCOPE-082-08 requires the consolidated checklist to enumerate every go-live dependency (5 secrets, L2 injection, local/CI trust, the three profiles, backup/restore sequencing, supervised canary)", a.what, a.anchor)
		}
	}
	return nil
}

// TestGoLiveChecklist_LiveFile asserts the live docs/Deployment.md contains
// the full Go-Live Readiness Checklist.
func TestGoLiveChecklist_LiveFile(t *testing.T) {
	docPath := filepath.Join(repoRoot(t), "docs", "Deployment.md")
	b, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("failed to read %q: %v", docPath, err)
	}
	if err := assertGoLiveChecklist(string(b)); err != nil {
		t.Fatalf("live docs/Deployment.md violates SCOPE-082-08 go-live checklist contract: %v", err)
	}
	t.Logf("contract OK: docs/Deployment.md Go-Live Readiness Checklist contains all %d required anchors (SCOPE-082-08)", len(goLiveChecklistAnchors))
}

// TestGoLiveChecklist_AdversarialMissingAnchor proves the contract catches a
// regression that drops a required anchor (here: the supervised-canary step).
func TestGoLiveChecklist_AdversarialMissingAnchor(t *testing.T) {
	// A doc body that has everything EXCEPT "supervised canary".
	var b strings.Builder
	b.WriteString("## Go-Live Readiness Checklist\n")
	for _, a := range goLiveChecklistAnchors {
		if a.anchor == "supervised canary" {
			continue // deliberately omitted
		}
		b.WriteString(a.anchor + "\n")
	}
	err := assertGoLiveChecklist(b.String())
	if err == nil {
		t.Fatal("adversarial contract test failed: a checklist missing the 'supervised canary' anchor was ACCEPTED (the contract is tautological — it would NOT catch a dropped go-live step)")
	}
	if !strings.Contains(err.Error(), "supervised canary") {
		t.Fatalf("adversarial contract test failed: error did not name the missing 'supervised canary' anchor: %v", err)
	}
	t.Logf("adversarial OK: missing 'supervised canary' anchor is rejected with: %v", err)
}
