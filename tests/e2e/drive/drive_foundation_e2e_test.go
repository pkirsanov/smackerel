//go:build e2e

// Spec 038 Scope 1 — drive foundation e2e rows.
//
// TestDriveFoundationE2E_MissingRequiredConfigFailsLoudly maps to
// SCN-038-001 e2e row in scopes.md. It copies config/smackerel.yaml to
// a temp file, strips the required drive.classification.confidence_threshold
// key, and runs the repo config generator (the implementation invoked by
// `./smackerel.sh config generate`) against the modified file. The
// generator MUST exit non-zero with the missing key name in stderr.
//
// TestDriveFoundationE2E_SecondProviderUsesNeutralContract maps to
// SCN-038-003 e2e row. It hits the live test stack's
// GET /v1/connectors/drive endpoint and asserts the response surfaces
// the registered drive providers through the provider-neutral contract
// (id + display_name + capabilities), without provider-specific
// branching in the wire shape. The multi-provider neutrality proof is
// carried by the unit row TestProviderRegistryExposesCapabilitiesWithoutProviderBranching;
// this e2e row proves the contract is intact through the real HTTP
// boundary on a running test stack.
package drive

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// repoRoot is the workspace root inside the e2e test container. The
// integration runner mounts the repo at /workspace; we resolve relative
// to the current working directory (which the runner sets to /workspace)
// rather than hard-coding the path so the test still works locally.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// The e2e tests run from /workspace/tests/e2e/drive; walk up to find
	// the directory containing config/smackerel.yaml.
	dir := wd
	for i := 0; i < 8; i = i + 1 {
		if _, err := os.Stat(filepath.Join(dir, "config", "smackerel.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root from %s (no config/smackerel.yaml ancestor)", wd)
	return ""
}

// TestDriveFoundationE2E_MissingRequiredConfigFailsLoudly proves SCN-038-001
// over the real config-generation pipeline: deleting a required drive key
// causes the generator to fail loud with the missing key name.
func TestDriveFoundationE2E_MissingRequiredConfigFailsLoudly(t *testing.T) {
	root := repoRoot(t)
	srcYAML := filepath.Join(root, "config", "smackerel.yaml")
	srcBytes, err := os.ReadFile(srcYAML)
	if err != nil {
		t.Fatalf("read source yaml: %v", err)
	}

	const target = "    confidence_threshold:"
	if !strings.Contains(string(srcBytes), target) {
		t.Fatalf("source smackerel.yaml does not contain required %q (drive block missing or moved)", target)
	}

	// Strip every line whose trimmed prefix is `confidence_threshold:`
	// so the modified yaml deterministically violates the SST contract.
	lines := strings.Split(string(srcBytes), "\n")
	out := make([]string, 0, len(lines))
	stripped := 0
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "confidence_threshold:") &&
			strings.Contains(ln, "min confidence to apply classification") {
			stripped = stripped + 1
			continue
		}
		out = append(out, ln)
	}
	if stripped == 0 {
		t.Fatalf("expected to strip at least one confidence_threshold line, stripped=%d", stripped)
	}

	tmpDir := t.TempDir()
	dstYAML := filepath.Join(tmpDir, "smackerel.yaml")
	if err := os.WriteFile(dstYAML, []byte(strings.Join(out, "\n")), 0o600); err != nil {
		t.Fatalf("write tmp yaml: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// scripts/commands/config.sh is the implementation invoked by
	// `./smackerel.sh config generate`. Calling it with --config <path>
	// against our modified file isolates the generator run from the
	// rest of the repo state.
	cmd := exec.CommandContext(ctx, "bash",
		filepath.Join(root, "scripts", "commands", "config.sh"),
		"--config", dstYAML,
		"--env", "dev",
	)
	cmd.Env = append(os.Environ(), "TARGET_ENV_GUARD=e2e-038-001")
	combined, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("run config.sh: %v (output=%s)", err, string(combined))
		}
	}

	t.Logf("config.sh exit=%d stripped=%d output=%s", exitCode, stripped, string(combined))

	if exitCode == 0 {
		t.Fatalf("config.sh exit=0 with missing drive.classification.confidence_threshold; expected non-zero. output=%s",
			string(combined))
	}
	if !strings.Contains(string(combined), "drive.classification.confidence_threshold") {
		t.Fatalf("config.sh stderr does not mention drive.classification.confidence_threshold; output=%s",
			string(combined))
	}
}

// TestDriveFoundationE2E_SecondProviderUsesNeutralContract proves SCN-038-003
// across the live HTTP boundary: the connectors-list endpoint emits every
// registered provider through the same neutral surface (id + display_name +
// capabilities) so adding a second provider at compile time requires no
// PWA-side or downstream-consumer changes.
func TestDriveFoundationE2E_SecondProviderUsesNeutralContract(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	url := strings.TrimRight(cfg.CoreURL, "/") + "/v1/connectors/drive"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(body))
	}

	type capView struct {
		SupportsVersions      bool     `json:"supports_versions"`
		SupportsSharing       bool     `json:"supports_sharing"`
		SupportsChangeHistory bool     `json:"supports_change_history"`
		MaxFileSizeBytes      int64    `json:"max_file_size_bytes"`
		SupportedMimeFilter   []string `json:"supported_mime_filter"`
	}
	type providerView struct {
		ID           string                 `json:"id"`
		DisplayName  string                 `json:"display_name"`
		Capabilities capView                `json:"capabilities"`
		Extra        map[string]interface{} `json:"-"`
	}
	var decoded struct {
		Providers []providerView `json:"providers"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode body: %v body=%s", err, string(body))
	}
	if decoded.Providers == nil {
		t.Fatalf("providers is null; want non-null array; body=%s", string(body))
	}

	// Adversarial neutrality assertion: every provider entry MUST expose
	// EXACTLY the neutral keys {id, display_name, capabilities}. Any
	// extra top-level key is provider-specific branching leaking into
	// the wire shape and would silently couple downstream consumers to
	// concrete provider types.
	var raw struct {
		Providers []map[string]interface{} `json:"providers"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode raw body: %v", err)
	}
	allowedKeys := map[string]bool{"id": true, "display_name": true, "capabilities": true}
	for i, p := range raw.Providers {
		for k := range p {
			if !allowedKeys[k] {
				t.Errorf("provider[%d] contains non-neutral key %q (only id/display_name/capabilities are allowed); full=%v",
					i, k, p)
			}
		}
	}

	var googleSeen bool
	for _, p := range decoded.Providers {
		if p.ID == "" {
			t.Errorf("provider missing id; full=%+v", p)
		}
		if p.DisplayName == "" {
			t.Errorf("provider %q missing display_name", p.ID)
		}
		if p.Capabilities.MaxFileSizeBytes <= 0 {
			t.Errorf("provider %q has non-positive max_file_size_bytes %d (must be SST-injected)",
				p.ID, p.Capabilities.MaxFileSizeBytes)
		}
		if p.ID == "google" {
			googleSeen = true
			if p.Capabilities.MaxFileSizeBytes >= 5*1024*1024*1024*1024 {
				t.Errorf("google max_file_size_bytes=%d looks like the 5 TiB hard ceiling — wiring forgot to call Configure",
					p.Capabilities.MaxFileSizeBytes)
			}
		}
	}
	if !googleSeen {
		t.Fatalf("google provider absent from response; body=%s", string(body))
	}
}
