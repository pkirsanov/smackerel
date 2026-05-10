//go:build integration

// spec 043 / BUG-001 — Adversarial Docker Hub registry-existence guard for
// the pinned ollama image.
//
// SCN-BUG-001-001: the value of `OLLAMA_IMAGE` in `config/generated/test.env`
//   MUST point to a tag whose manifest is currently published on Docker Hub.
//   If a future commit re-pins to a yanked or invalid tag, this test fails
//   loudly inside the integration run instead of inside `docker compose up`.
//
// SCN-BUG-001-002: adversarial — the same code path applied to the synthetic
//   input `ollama/ollama:0.6` MUST return HTTP 404. This proves the live test
//   is not tautological and would have caught BUG-001 at the time it was
//   introduced.
//
// References:
//   - specs/043-ollama-test-infrastructure/bugs/BUG-001-ollama-image-pin-stale/spec.md
//   - specs/043-ollama-test-infrastructure/bugs/BUG-001-ollama-image-pin-stale/design.md
//   - specs/043-ollama-test-infrastructure/bugs/BUG-001-ollama-image-pin-stale/scopes.md (Scope 01, T01-04 / T01-05)
//
// Hard constraints (per .github/copilot-instructions.md → Adversarial
// Regression Tests for Bug Fixes + spec 043 Scope 02 no-skip-guard):
//   - NO `t.Skip()` / `t.SkipNow()` / `t.Skipf(...)` calls anywhere in this
//     file. Missing env, broken network, or non-200 response are all
//     fail-loud conditions.
//   - The adversarial sub-test MUST exercise the same helper as the live
//     test, with a different input that is known to fail.
//   - Zero hardcoded ollama runtime values: the live test reads the live pin
//     from `config/generated/test.env` (the SST→env-file output). Only the
//     adversarial test uses a literal — and that literal is the *known-yanked*
//     synthetic input whose 404 is the assertion target (allowlisted in the
//     SST grep guard via the `tests/` directory carve-out).

package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	// dockerHubTagsAPIBase is the public read-only Docker Hub tags endpoint.
	// Returns 200 with JSON metadata when the tag exists and is published;
	// returns 404 when the tag has been yanked or never existed.
	// Reference: https://docs.docker.com/docker-hub/api/latest/#tag/repositories
	dockerHubTagsAPIBase = "https://hub.docker.com/v2/repositories/"

	// dockerHubHTTPTimeout is the per-request HTTP timeout. Generous enough
	// to absorb transient network jitter without masking a real outage.
	dockerHubHTTPTimeout = 15 * time.Second

	// knownYankedTag is the historical Smackerel pin that triggered BUG-001.
	// Pinned here as a literal because its 404 is the assertion target — it
	// MUST NOT track the live pin.
	knownYankedTag = "ollama/ollama:0.6"
)

// dockerHubTagExists HEADs the Docker Hub Tags API for the given image
// reference (in the form "<repo>:<tag>") and returns the HTTP status code.
// Fails the test loudly if the input is malformed or the network round-trip
// fails — never returns a fabricated status code.
func dockerHubTagExists(t *testing.T, imageRef string) int {
	t.Helper()

	if imageRef == "" {
		t.Fatalf("dockerHubTagExists: imageRef is empty (caller bug)")
	}
	colonIdx := strings.LastIndex(imageRef, ":")
	if colonIdx <= 0 || colonIdx == len(imageRef)-1 {
		t.Fatalf("dockerHubTagExists: imageRef %q is not in <repo>:<tag> form", imageRef)
	}
	repo := imageRef[:colonIdx]
	tag := imageRef[colonIdx+1:]
	if repo == "" || tag == "" {
		t.Fatalf("dockerHubTagExists: imageRef %q split into empty repo or tag (repo=%q tag=%q)", imageRef, repo, tag)
	}

	url := fmt.Sprintf("%s%s/tags/%s", dockerHubTagsAPIBase, repo, tag)
	ctx, cancel := context.WithTimeout(context.Background(), dockerHubHTTPTimeout)
	defer cancel()

	// HEAD avoids transferring the metadata body on the success path; the
	// status code alone is sufficient to prove published-or-yanked.
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		t.Fatalf("dockerHubTagExists: build request for %s: %v", url, err)
	}
	req.Header.Set("User-Agent", "smackerel-integration-test/spec043-bug001")

	client := &http.Client{Timeout: dockerHubHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("dockerHubTagExists: HEAD %s failed (network unreachable from integration runner — this is a fail-loud condition, not a skip): %v", url, err)
	}
	defer resp.Body.Close()

	return resp.StatusCode
}

// repoRootForOllamaImageGuard climbs from CWD looking for
// config/smackerel.yaml. Independent of `go test` working dir.
func repoRootForOllamaImageGuard(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "config", "smackerel.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root from %s", wd)
	return ""
}

// readEnvVarFromFile reads `path` line-by-line and returns the value of the
// first occurrence of `<key>=<value>` (with leading/trailing whitespace
// stripped from the value). Returns empty string when the key is absent.
func readEnvVarFromFile(t *testing.T, path, key string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	prefix := key + "="
	for _, raw := range strings.Split(string(contents), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

// TestOllamaImagePinIsPublished_LiveTag asserts that the live pin for the
// test environment (read from the SST-derived `config/generated/test.env`)
// resolves to a Docker Hub tag whose manifest is currently published.
//
// Fails loudly (no `t.Skip()`) when:
//   - `config/generated/test.env` does not exist (operator must run
//     `./smackerel.sh --env test config generate` before integration tests).
//   - `OLLAMA_IMAGE` is unset in the env file.
//   - `OLLAMA_IMAGE` is not in `<repo>:<tag>` form.
//   - Docker Hub returns HTTP != 200 for the pinned tag.
//   - The HTTP round-trip fails (e.g. DNS / TCP / TLS failure).
func TestOllamaImagePinIsPublished_LiveTag(t *testing.T) {
	root := repoRootForOllamaImageGuard(t)
	envPath := filepath.Join(root, "config", "generated", "test.env")
	if _, err := os.Stat(envPath); err != nil {
		t.Fatalf("test.env not found at %s — run `./smackerel.sh --env test config generate` first (this is a fail-loud condition, not a skip): %v", envPath, err)
	}

	imageRef := readEnvVarFromFile(t, envPath, "OLLAMA_IMAGE")
	if imageRef == "" {
		t.Fatalf("OLLAMA_IMAGE is unset in %s — config generation may have regressed (this is a fail-loud condition, not a skip)", envPath)
	}
	if !strings.HasPrefix(imageRef, "ollama/ollama:") {
		t.Fatalf("OLLAMA_IMAGE=%q does not target the upstream ollama/ollama repo — this guard only covers that pin (extend the test before re-pinning to a different repo)", imageRef)
	}

	status := dockerHubTagExists(t, imageRef)
	if status != http.StatusOK {
		t.Fatalf("live pin %q is NOT published on Docker Hub (HEAD %s%s/tags/%s returned HTTP %d, want 200) — yanked tag will fail `docker compose up` at the next integration run; re-pin to a currently-published tag",
			imageRef,
			dockerHubTagsAPIBase,
			imageRef[:strings.LastIndex(imageRef, ":")],
			imageRef[strings.LastIndex(imageRef, ":")+1:],
			status)
	}
	t.Logf("live OK: pinned tag %s published on Docker Hub (HTTP %d against %s%s/tags/%s)",
		imageRef,
		status,
		dockerHubTagsAPIBase,
		imageRef[:strings.LastIndex(imageRef, ":")],
		imageRef[strings.LastIndex(imageRef, ":")+1:])
}

// TestOllamaImagePinIsPublished_AdversarialYankedTag exercises the same
// helper as the live test against the historical `ollama/ollama:0.6` pin —
// the exact value that triggered BUG-001. Asserts HTTP 404 to prove the
// live test is not tautological: a real yanked tag returns a real 404, and
// the live test would have caught BUG-001 at the time it was introduced.
//
// If Docker Hub were ever to re-publish the `0.6` tag (extremely unlikely
// — yanked manifests are typically permanent), this test would fail and
// the operator would update the synthetic input to a different known-yanked
// tag, OR delete this test if no such tag exists.
func TestOllamaImagePinIsPublished_AdversarialYankedTag(t *testing.T) {
	status := dockerHubTagExists(t, knownYankedTag)
	if status == http.StatusOK {
		t.Fatalf("adversarial guard tautological: known-yanked tag %q now returns HTTP 200 — Docker Hub re-published the tag (extremely unusual). Update the synthetic input to a different known-yanked tag (or delete this test if none exists). This means the live registry-existence guard is no longer proven non-tautological.",
			knownYankedTag)
	}
	if status != http.StatusNotFound {
		t.Fatalf("adversarial guard inconclusive: HEAD against known-yanked tag %q returned HTTP %d, want 404 — Docker Hub may be returning an unexpected error (rate-limit, outage, etc.); cannot prove the live test is non-tautological under this status",
			knownYankedTag, status)
	}
	t.Logf("adversarial OK: yanked tag %s returned HTTP %d against Docker Hub tags API — confirms the live test would have caught BUG-001 at the time it was introduced",
		knownYankedTag, status)
}
