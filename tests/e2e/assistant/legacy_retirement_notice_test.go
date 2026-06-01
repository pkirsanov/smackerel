//go:build e2e

// Spec 075 SCOPE-075-06.3 (TP-075-09, re-targeted from the prior
// Playwright plan to a Go e2e per the photos_capability_test
// pattern). Drives the LIVE chi-mounted POST /api/assistant/turn
// route via the running core service with a retired-command turn
// during the open window and asserts:
//
//  1. The wire response carries the schema 069 v1 OPTIONAL `notice`
//     payload populated by the facade Policy dispatch (SCOPE-075-06.1
//     + 06.2 + 06.2b end-to-end).
//  2. The PWA renderer at web/pwa/lib/render_descriptor_v1.js
//     projects that notice into a render-descriptor-v1 text node
//     AFTER the primary body, proving the addendum is a non-blocking
//     one-line follow-up rather than a replacement of the assistant
//     response (SCOPE-075-06.3 renderer contract).
//
// Live-stack inputs come exclusively from the SST-managed environment
// the e2e harness exports. Per the repo NO-DEFAULTS policy, missing or
// empty values are fail-loud — the only legitimate skip is "no live
// stack here" (CORE_EXTERNAL_URL unset) or "this window state cannot
// exercise this branch" (LEGACY_RETIREMENT_WINDOW_STATE != "open").
// A live stack with the legacy-retirement SST missing or the bearer
// token missing is a wiring bug and fails the test.

package assistant_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

type legacyRetirementNoticeLiveStack struct {
	BaseURL       string
	AuthToken     string
	WindowState   string
	WindowID      string
	RepoRoot      string
	RetiredCmd    string
}

// loadLegacyRetirementNoticeLiveStack mirrors loadHTTPTurnLiveStack
// (http_turn_test.go) plus the spec 075 SST knobs that drive the
// retired-command branch. The retired command is hard-coded to
// "/weather" because spec 075 SST guarantees an entry for it in
// legacy_retirement.notice_copy_per_command and the assistant facade
// Policy dispatch wires that into the NoticePayload.
func loadLegacyRetirementNoticeLiveStack(t *testing.T) legacyRetirementNoticeLiveStack {
	t.Helper()
	baseURL := strings.TrimRight(os.Getenv("CORE_EXTERNAL_URL"), "/")
	if baseURL == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}
	tok := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if tok == "" {
		t.Fatalf("SMACKEREL_AUTH_TOKEN not set; live stack is up but auth wiring is missing — run via ./smackerel.sh test e2e")
	}
	state := os.Getenv("LEGACY_RETIREMENT_WINDOW_STATE")
	if state == "" {
		t.Fatalf("LEGACY_RETIREMENT_WINDOW_STATE not set; live stack is up but spec 075 SST is missing — wiring bug")
	}
	windowID := os.Getenv("LEGACY_RETIREMENT_WINDOW_ID")
	if windowID == "" {
		t.Fatalf("LEGACY_RETIREMENT_WINDOW_ID not set; live stack is up but spec 075 SST is missing — wiring bug")
	}
	repoRoot := findRepoRootForNoticeRenderer(t)
	return legacyRetirementNoticeLiveStack{
		BaseURL:     baseURL,
		AuthToken:   tok,
		WindowState: state,
		WindowID:    windowID,
		RepoRoot:    repoRoot,
		RetiredCmd:  "/weather",
	}
}

// findRepoRootForNoticeRenderer walks upward from the current working
// directory looking for a `go.mod` so the test can resolve the JS
// renderer CLI path without depending on the test working directory.
// Fails loud if no go.mod is reachable.
func findRepoRootForNoticeRenderer(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root (go.mod) from cwd")
	return ""
}

func waitLegacyRetirementNoticeHealthy(t *testing.T, stack legacyRetirementNoticeLiveStack, maxWait time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, stack.BaseURL+"/api/health", nil)
		resp, err := client.Do(req)
		cancel()
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("e2e: core not healthy after %s at %s", maxWait, stack.BaseURL)
}

func postNoticeAssistantTurn(t *testing.T, stack legacyRetirementNoticeLiveStack, text, turnID string) (*http.Response, []byte) {
	t.Helper()
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               text,
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal turn request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, stack.BaseURL+"/api/assistant/turn", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+stack.AuthToken)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("POST /api/assistant/turn: %v", err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, buf.Bytes()
}

// runPWAReeRenderer feeds the raw assistant_turn_v1 JSON body through
// the JS renderer CLI at web/pwa/lib/render_descriptor_v1_cli.js and
// returns the decoded render-descriptor-v1 map. The CLI exit-codes
// non-zero on any decode or schema_version error; that is propagated
// as a fatal so a renderer regression cannot silently degrade.
func runPWARenderer(t *testing.T, stack legacyRetirementNoticeLiveStack, responseJSON []byte) map[string]any {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Fatalf("node not on PATH; spec 075 SCOPE-075-06.3 e2e requires node to run the PWA renderer: %v", err)
	}
	cliPath := filepath.Join(stack.RepoRoot, "web", "pwa", "lib", "render_descriptor_v1_cli.js")
	if _, err := os.Stat(cliPath); err != nil {
		t.Fatalf("PWA renderer CLI missing at %s: %v", cliPath, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", cliPath)
	cmd.Stdin = bytes.NewReader(responseJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("PWA renderer CLI failed: %v\nstderr=%s\nstdin=%s", err, stderr.String(), string(responseJSON))
	}
	var descriptor map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &descriptor); err != nil {
		t.Fatalf("decode renderer output: %v\nstdout=%s", err, stdout.String())
	}
	return descriptor
}

// TestLegacyRetirementNoticeE2E_OpenWindowRendersAddendumWithoutBlockingBody
// is the SCOPE-075-06.3 / TP-075-09 live regression. It proves the
// happy path: a retired-command turn during the open window returns
// the schema-v1 wire body with `notice` populated, and the PWA
// renderer surfaces the notice as a text-node ADDENDUM after the
// primary body — never as a replacement.
func TestLegacyRetirementNoticeE2E_OpenWindowRendersAddendumWithoutBlockingBody(t *testing.T) {
	stack := loadLegacyRetirementNoticeLiveStack(t)
	if stack.WindowState != "open" {
		t.Skipf("LEGACY_RETIREMENT_WINDOW_STATE=%q — SCN-075-A14 / TP-075-09 only exercises the open branch", stack.WindowState)
	}
	waitLegacyRetirementNoticeHealthy(t, stack, 30*time.Second)

	turnID := "e2e-scope-075-06.3-notice-" + time.Now().UTC().Format("20060102T150405.000")
	resp, raw := postNoticeAssistantTurn(t, stack, stack.RetiredCmd, turnID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, string(raw))
	}
	var out httpadapter.TurnResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response: %v\nbody=%s", err, string(raw))
	}
	if out.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version = %q, want %q (notice is additive — must NOT bump v1)", out.SchemaVersion, httpadapter.SchemaVersionV1)
	}
	if out.Notice == nil {
		t.Fatalf("response.notice is nil for retired command %q during open window — Policy dispatch did not attach NoticePayload; body=%s", stack.RetiredCmd, string(raw))
	}
	if out.Notice.Command != stack.RetiredCmd {
		t.Errorf("notice.command = %q, want %q", out.Notice.Command, stack.RetiredCmd)
	}
	if strings.TrimSpace(out.Notice.ReplacementExample) == "" {
		t.Errorf("notice.replacement_example is empty — SST notice_copy_per_command must be present and non-empty")
	}
	if strings.TrimSpace(out.Notice.CopyKey) == "" {
		t.Errorf("notice.copy_key is empty — ledger dedup key is required")
	}
	if out.Notice.WindowID != stack.WindowID {
		t.Errorf("notice.window_id = %q, want %q (SST LEGACY_RETIREMENT_WINDOW_ID)", out.Notice.WindowID, stack.WindowID)
	}

	// Adversarial: the primary body MUST still be non-empty. The notice
	// is an addendum — never a replacement — so a renderer regression
	// that swallowed the body would fail here.
	if strings.TrimSpace(out.Body) == "" {
		t.Fatalf("response.body is empty — notice must NOT replace the primary assistant response; body=%s", string(raw))
	}

	// Drive the PWA renderer against the live wire body and assert
	// the descriptor projects (a) a body text node FIRST and (b) a
	// notice text node AFTER carrying the replacement_example copy.
	descriptor := runPWARenderer(t, stack, raw)
	if v, _ := descriptor["schema_version"].(string); v != "render-descriptor.v1" {
		t.Fatalf("descriptor.schema_version = %q, want render-descriptor.v1", v)
	}
	nodesRaw, ok := descriptor["nodes"].([]any)
	if !ok {
		t.Fatalf("descriptor.nodes missing or wrong type: %T", descriptor["nodes"])
	}
	textNodes := make([]string, 0, len(nodesRaw))
	for _, n := range nodesRaw {
		m, ok := n.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := m["kind"].(string); kind == "text" {
			if txt, _ := m["text"].(string); txt != "" {
				textNodes = append(textNodes, txt)
			}
		}
	}
	if len(textNodes) < 2 {
		t.Fatalf("expected at least 2 text nodes (primary body + notice addendum); got %d (nodes=%v)", len(textNodes), nodesRaw)
	}
	// First text node MUST be the primary body — proves the notice
	// did not push the body off, did not replace it, and rendered AS
	// AN ADDENDUM AFTER the primary response.
	if textNodes[0] != out.Body {
		t.Errorf("first text node = %q, want primary body %q (notice must be appended after body, never replace it)", textNodes[0], out.Body)
	}
	// Notice text node MUST appear after the primary body and carry
	// the SST replacement_example copy verbatim.
	foundNotice := false
	for i := 1; i < len(textNodes); i++ {
		if textNodes[i] == out.Notice.ReplacementExample {
			foundNotice = true
			break
		}
	}
	if !foundNotice {
		t.Errorf("notice replacement_example %q not found in descriptor text nodes after body; nodes=%v", out.Notice.ReplacementExample, textNodes)
	}
}

// TestLegacyRetirementNoticeE2E_NonRetiredTurnOmitsNotice is the
// adversarial counterpart: a benign text turn that does NOT match
// any retired command MUST yield a wire body with `notice` absent
// (omitempty) and a descriptor with NO injected addendum. Catches a
// regression where the facade Policy fires on every turn instead of
// only on retired-command matches.
func TestLegacyRetirementNoticeE2E_NonRetiredTurnOmitsNotice(t *testing.T) {
	stack := loadLegacyRetirementNoticeLiveStack(t)
	waitLegacyRetirementNoticeHealthy(t, stack, 30*time.Second)

	turnID := "e2e-scope-075-06.3-no-notice-" + time.Now().UTC().Format("20060102T150405.000")
	resp, raw := postNoticeAssistantTurn(t, stack, "hello", turnID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, string(raw))
	}

	// Strict decode: the v1 wire contract MUST omit `notice` entirely
	// when no retired-command match — omitempty is the back-compat
	// guarantee for every v1 client that has not regenerated its
	// bindings.
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("decode raw body as map: %v\nbody=%s", err, string(raw))
	}
	if _, present := asMap["notice"]; present {
		t.Errorf("wire body contains `notice` key for a non-retired turn — omitempty regression; body=%s", string(raw))
	}

	descriptor := runPWARenderer(t, stack, raw)
	nodesRaw, ok := descriptor["nodes"].([]any)
	if !ok {
		t.Fatalf("descriptor.nodes missing or wrong type: %T", descriptor["nodes"])
	}
	// No notice payload on the wire → no notice text node injected.
	// We can't tell text nodes apart from primary body without the
	// body string, so assert the descriptor projection is byte-stable
	// against a renderer run with the notice key explicitly stripped.
	var stripped map[string]any
	if err := json.Unmarshal(raw, &stripped); err != nil {
		t.Fatalf("re-decode body: %v", err)
	}
	delete(stripped, "notice")
	strippedBytes, err := json.Marshal(stripped)
	if err != nil {
		t.Fatalf("re-marshal stripped body: %v", err)
	}
	strippedDescriptor := runPWARenderer(t, stack, strippedBytes)
	strippedNodes, _ := strippedDescriptor["nodes"].([]any)
	if len(strippedNodes) != len(nodesRaw) {
		t.Errorf("descriptor node count diverges with vs without notice key on a non-retired turn (%d vs %d) — renderer leaked a notice node where none was sent", len(strippedNodes), len(nodesRaw))
	}
}
