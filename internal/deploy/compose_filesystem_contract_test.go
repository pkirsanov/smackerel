// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// This test file extends the deploy compose contract with spec 045
// read-only-root filesystem invariants (Scope 2). It is in the same
// `package deploy` as compose_contract_test.go and re-uses repoRoot()
// from that file.
//
// The contract enforced by spec 045 FR-045-003:
//
//   1. smackerel-core, smackerel-ml, ollama MUST declare `read_only: true`.
//      These services are stateless or have all writes scoped to a known
//      named volume (ollama-data) — running with a read-only root rejects
//      every persistent mutation outside the explicit allowlist.
//
//   2. postgres + nats MUST NOT declare `read_only: true`. Postgres writes
//      WAL/control files outside /var/lib/postgresql/data; NATS server
//      JetStream needs broader root access. Forcing read-only on these
//      would break startup. The exemption is enforced as a hard contract
//      so a future "harden everything" sweep cannot accidentally lock
//      out the stateful services.
//
//   3. Every read-only service's `tmpfs:` entries MUST be in the
//      documented allowlist for that service:
//
//        smackerel-core: /tmp (single tmpfs, log buffering)
//        smackerel-ml:   /tmp (HF cache + ST cache + uvicorn temp)
//        ollama:         /tmp + /.ollama_tmp (model store stays on
//                        the ollama-data named volume, NOT tmpfs)
//
// Three adversarial sub-tests prove the contract function would FAIL if
// any future edit regresses the contract:
//
//   - TestFilesystemContract_AdversarialMissingReadOnly: smackerel-core
//     drops read_only:true → must FAIL (the most likely accidental
//     regression — someone editing the file might forget to re-add it).
//
//   - TestFilesystemContract_AdversarialPostgresReadOnly: postgres adds
//     read_only:true → must FAIL (a "harden everything" sweep would
//     break Postgres startup).
//
//   - TestFilesystemContract_AdversarialUnauthorizedTmpfs: smackerel-core
//     declares `/var/run` tmpfs that is not in its allowlist → must FAIL
//     (proves the allowlist matching, not just presence-of-tmpfs).
//
// Cross-reference:
//   - specs/045-deploy-resource-filesystem-hardening/spec.md  FR-045-003
//   - specs/045-deploy-resource-filesystem-hardening/design.md §"Read-only Root Set" + "Tmpfs Mounts"
//   - specs/045-deploy-resource-filesystem-hardening/scenario-manifest.json SCN-045-A03

package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeFilesystemDoc captures `read_only` and `tmpfs` for every service.
// The tmpfs field is parsed as a slice of strings because compose entries
// use the short form "<path>:<options>" — long-form objects are not used
// in this contract.
type composeFilesystemDoc struct {
	Services map[string]struct {
		ReadOnly bool     `yaml:"read_only"`
		Tmpfs    []string `yaml:"tmpfs"`
	} `yaml:"services"`
}

// readOnlyAllowlist defines, for every service that MUST declare
// read_only: true, the set of mount points that are allowed under
// `tmpfs:`. Adding a new tmpfs mount point requires extending this
// allowlist (which forces the design discussion to capture WHY a new
// writable area is needed).
//
// Mount-point strings are compared by the path portion only (the part
// before `:`) because the size/mode options are tunables, not contract.
var readOnlyAllowlist = map[string][]string{
	"smackerel-core": {"/tmp"},
	"smackerel-ml":   {"/tmp"},
	"ollama":         {"/tmp", "/.ollama_tmp"},
	// Spec 049 — Prometheus inherits the spec 045 read-only-root
	// contract. The TSDB lives on a named volume mounted at /prometheus
	// (not a tmpfs); the only writable tmpfs is /tmp for query
	// buffers and the wal-segment scratch area.
	"prometheus": {"/tmp"},
}

// readOnlyExempt is the set of services that MUST NOT declare
// read_only: true. Postgres and NATS need broader root access for
// pre-data-dir writes; forcing read-only would break startup.
var readOnlyExempt = []string{"postgres", "nats"}

// tmpfsMountPath returns the path portion of a compose tmpfs short-form
// entry. "/tmp:size=64m,mode=1777,nosuid,noexec,nodev" → "/tmp".
// "/tmp" alone (no options) → "/tmp".
func tmpfsMountPath(entry string) string {
	if idx := strings.IndexByte(entry, ':'); idx > 0 {
		return entry[:idx]
	}
	return entry
}

// assertFilesystemContract returns nil iff every read-only-required
// service declares read_only: true with only allowlisted tmpfs entries,
// AND every exempt service does NOT declare read_only: true. Returns a
// non-nil error naming the specific service and violation on any breach.
func assertFilesystemContract(yamlBytes []byte) error {
	var doc composeFilesystemDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}

	// Sort service names for deterministic error output across runs.
	roServices := make([]string, 0, len(readOnlyAllowlist))
	for name := range readOnlyAllowlist {
		roServices = append(roServices, name)
	}
	sort.Strings(roServices)

	// Check 1 — every read-only-required service has read_only: true.
	// Services absent from this fixture are SKIPPED; the live-file
	// test (TestFilesystemContract_LiveFile) calls
	// `assertFilesystemContractRequiresAll` to enforce presence.
	// This lets adversarial fixtures scope down to a single service
	// to prove the violation mode without redundant boilerplate.
	for _, svcName := range roServices {
		svc, ok := doc.Services[svcName]
		if !ok {
			continue
		}
		if !svc.ReadOnly {
			return fmt.Errorf("contract violation: services.%s.read_only is missing or false — spec 045 FR-045-003 requires this stateless service to run with a read-only root filesystem (set `read_only: true`)", svcName)
		}
	}

	// Check 2 — every exempt service does NOT have read_only: true.
	for _, svcName := range readOnlyExempt {
		svc, ok := doc.Services[svcName]
		if !ok {
			// Exempt services may legitimately be absent from a fixture
			// (e.g. the dev compose may scope down). Skip rather than fail.
			continue
		}
		if svc.ReadOnly {
			return fmt.Errorf("contract violation: services.%s.read_only=true — spec 045 FR-045-003 EXEMPTS this stateful service from read-only because it writes outside its data-dir at startup; forcing read-only would break the service. The exemption is encoded in the contract test allowlist; if the operator believes this service can run read-only now, update both the design doc and this test", svcName)
		}
	}

	// Check 3 — every read-only service's tmpfs entries are in the allowlist.
	for _, svcName := range roServices {
		svc, ok := doc.Services[svcName]
		if !ok {
			continue
		}
		allowed := readOnlyAllowlist[svcName]
		allowedSet := make(map[string]struct{}, len(allowed))
		for _, p := range allowed {
			allowedSet[p] = struct{}{}
		}
		for i, entry := range svc.Tmpfs {
			path := tmpfsMountPath(entry)
			if _, ok := allowedSet[path]; !ok {
				return fmt.Errorf("contract violation: services.%s.tmpfs[%d]=%q (path=%q) is NOT in the spec 045 FR-045-003 allowlist for this service (allowed: %v) — every writable area in a read-only-root container is a security boundary; adding a new one requires updating both the design doc and the readOnlyAllowlist in this test", svcName, i, entry, path, allowed)
			}
		}
	}

	return nil
}

// assertFilesystemContractRequiresAll asserts every read-only-required
// service in the contract set is PRESENT in the document, then
// delegates to assertFilesystemContract for the full violation walk.
// The live-file tests use this stricter form so a regression that
// silently drops a whole service block is still caught.
func assertFilesystemContractRequiresAll(yamlBytes []byte) error {
	var doc composeFilesystemDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}
	for svcName := range readOnlyAllowlist {
		if _, ok := doc.Services[svcName]; !ok {
			return fmt.Errorf("contract violation: services.%s not found in compose document — every service in the spec 045 FR-045-003 read-only allowlist MUST exist in the live deploy compose file", svcName)
		}
	}
	return assertFilesystemContract(yamlBytes)
}

// TestFilesystemContract_LiveFile is the primary spec 045 FR-045-003
// assertion. It loads the live deploy/compose.deploy.yml and proves the
// read-only + tmpfs allowlist contract holds. SCN-045-A03 Gherkin:
// `When deploy/compose.deploy.yml is inspected ... Then smackerel-core,
// smackerel-ml, and ollama declare read_only: true; postgres and nats
// do NOT; required writable directories are backed by explicit tmpfs.`
func TestFilesystemContract_LiveFile(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "deploy", "compose.deploy.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live compose file %q: %v", composePath, err)
	}
	if err := assertFilesystemContractRequiresAll(yamlBytes); err != nil {
		t.Fatalf("live deploy/compose.deploy.yml violates spec 045 FR-045-003 read-only filesystem contract: %v", err)
	}
	t.Logf("contract OK: deploy/compose.deploy.yml satisfies spec 045 FR-045-003 (read-only allowlist {smackerel-core, smackerel-ml, ollama, prometheus} all declare read_only:true; exempt set {postgres, nats} do NOT; every tmpfs entry is in the documented allowlist)")
}

// TestFilesystemContract_LiveFile_DevCompose mirrors the live-file
// assertion against the dev docker-compose.yml so the dev stack runs
// hardened too. Per the scope 2 DoD: "docker-compose.yml: smackerel-core,
// smackerel-ml, ollama mirror the same read_only:true + tmpfs blocks so
// dev stack runs hardened (every read-only service declares its writable
// paths via tmpfs in an allowlist)."
//
// The dev compose intentionally has additional services or variants
// (e.g. build: blocks) that the deploy compose does not. The contract
// is the same shape — only the file path differs.
func TestFilesystemContract_LiveFile_DevCompose(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "docker-compose.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live dev compose file %q: %v", composePath, err)
	}
	if err := assertFilesystemContractRequiresAll(yamlBytes); err != nil {
		t.Fatalf("live docker-compose.yml violates spec 045 FR-045-003 read-only filesystem contract (dev stack mirror): %v", err)
	}
	t.Logf("contract OK: docker-compose.yml mirrors deploy compose hardening (dev stack runs read-only too)")
}

// TestFilesystemContract_AdversarialMissingReadOnly proves the contract
// catches a regression where smackerel-core drops read_only:true.
// SCN-045-A03 — most likely accidental regression: someone editing the
// service block forgets to re-add it.
func TestFilesystemContract_AdversarialMissingReadOnly(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    tmpfs:
      - /tmp:size=64m
  smackerel-ml:
    read_only: true
    tmpfs:
      - /tmp:size=768m
  ollama:
    read_only: true
    tmpfs:
      - /tmp:size=64m
      - /.ollama_tmp:size=64m
  postgres: {}
  nats: {}
`
	err := assertFilesystemContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: smackerel-core without read_only:true was accepted (the contract is tautological — it would NOT catch a regression that drops the read-only-root hardening)")
	}
	if !strings.Contains(err.Error(), "smackerel-core") {
		t.Fatalf("adversarial contract test failed: error did not mention 'smackerel-core': %v", err)
	}
	if !strings.Contains(err.Error(), "read_only") {
		t.Fatalf("adversarial contract test failed: error did not mention 'read_only': %v", err)
	}
	if !strings.Contains(err.Error(), "FR-045-003") {
		t.Fatalf("adversarial contract test failed: error did not reference spec 045 FR-045-003: %v", err)
	}
	t.Logf("adversarial OK: smackerel-core without read_only is rejected with: %v", err)
}

// TestFilesystemContract_AdversarialPostgresReadOnly proves the contract
// catches a regression where postgres acquires read_only:true. A
// "harden everything" sweep could accidentally apply the hardening to
// stateful services and break startup.
func TestFilesystemContract_AdversarialPostgresReadOnly(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    read_only: true
    tmpfs:
      - /tmp:size=64m
  smackerel-ml:
    read_only: true
    tmpfs:
      - /tmp:size=768m
  ollama:
    read_only: true
    tmpfs:
      - /tmp:size=64m
      - /.ollama_tmp:size=64m
  postgres:
    read_only: true
  nats: {}
`
	err := assertFilesystemContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: postgres with read_only:true was accepted (the contract is tautological — it would NOT catch a 'harden everything' sweep that breaks Postgres startup)")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("adversarial contract test failed: error did not mention 'postgres': %v", err)
	}
	if !strings.Contains(err.Error(), "EXEMPTS") {
		t.Fatalf("adversarial contract test failed: error did not mention the exemption: %v", err)
	}
	t.Logf("adversarial OK: postgres with read_only:true is rejected with: %v", err)
}

// TestFilesystemContract_AdversarialUnauthorizedTmpfs proves the contract
// catches a regression where a read-only service declares a tmpfs mount
// that is NOT in its allowlist. Every writable area in a read-only-root
// container is a security boundary; adding one without updating the
// allowlist defeats the purpose.
func TestFilesystemContract_AdversarialUnauthorizedTmpfs(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    read_only: true
    tmpfs:
      - /tmp:size=64m
      - /var/run:size=16m
  smackerel-ml:
    read_only: true
    tmpfs:
      - /tmp:size=768m
  ollama:
    read_only: true
    tmpfs:
      - /tmp:size=64m
      - /.ollama_tmp:size=64m
  postgres: {}
  nats: {}
`
	err := assertFilesystemContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: smackerel-core with unauthorized /var/run tmpfs was accepted (the contract is tautological — it would NOT catch a regression that adds an unauthorized writable area in a read-only-root container)")
	}
	if !strings.Contains(err.Error(), "smackerel-core") {
		t.Fatalf("adversarial contract test failed: error did not mention 'smackerel-core': %v", err)
	}
	if !strings.Contains(err.Error(), "/var/run") {
		t.Fatalf("adversarial contract test failed: error did not name the unauthorized path '/var/run': %v", err)
	}
	if !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("adversarial contract test failed: error did not mention 'allowlist': %v", err)
	}
	t.Logf("adversarial OK: smackerel-core with unauthorized /var/run tmpfs is rejected with: %v", err)
}

// TestFilesystemContract_AdversarialNATSReadOnly mirrors the postgres
// adversarial case for nats. Same exemption rationale; same regression
// risk.
func TestFilesystemContract_AdversarialNATSReadOnly(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    read_only: true
    tmpfs:
      - /tmp:size=64m
  smackerel-ml:
    read_only: true
    tmpfs:
      - /tmp:size=768m
  ollama:
    read_only: true
    tmpfs:
      - /tmp:size=64m
      - /.ollama_tmp:size=64m
  postgres: {}
  nats:
    read_only: true
`
	err := assertFilesystemContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: nats with read_only:true was accepted")
	}
	if !strings.Contains(err.Error(), "nats") {
		t.Fatalf("adversarial contract test failed: error did not mention 'nats': %v", err)
	}
	t.Logf("adversarial OK: nats with read_only:true is rejected with: %v", err)
}

// ─────────────────────────────────────────────────────────────────────────
// Spec 082 SCOPE-082-03 — embedding-model cache persistence.
//
// The ML image runs as non-root USER smackerel and bakes the embedding model
// into /home/smackerel/.cache at build time (ml/Dockerfile). The pre-082
// deploy compose OVERRODE HF_HOME=/tmp/hf-cache onto ephemeral tmpfs, which
// both ignored the baked model AND lost it on restart — forcing a re-download
// from HuggingFace on the reboot-recovery path. SCOPE-082-03 mounts a
// PERSISTENT named volume at the baked cache path /home/smackerel/.cache so
// Docker initializes the volume from the image content on first mount (model
// present, correct ownership) and the model survives restarts with no
// HuggingFace dependency. read_only root is preserved (writes land on the
// volume mount).
//
// Covers SCN-082-C01.
// ─────────────────────────────────────────────────────────────────────────

const mlModelCacheMountPath = "/home/smackerel/.cache"

// composeMLCacheDoc captures the ML service's read_only flag, environment,
// and volume mounts, plus the top-level named-volume declarations.
type composeMLCacheDoc struct {
	Services map[string]struct {
		ReadOnly    bool              `yaml:"read_only"`
		Environment map[string]string `yaml:"environment"`
		Volumes     []string          `yaml:"volumes"`
	} `yaml:"services"`
	Volumes map[string]struct {
		Name   string            `yaml:"name"`
		Labels map[string]string `yaml:"labels"`
	} `yaml:"volumes"`
}

// assertMLModelCacheContract returns nil iff the smackerel-ml service mounts
// a persistent named volume at the model-cache path, points HF_HOME and
// SENTENCE_TRANSFORMERS_HOME into it (not /tmp), and keeps read_only:true.
func assertMLModelCacheContract(yamlBytes []byte) error {
	var doc composeMLCacheDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}
	ml, ok := doc.Services["smackerel-ml"]
	if !ok {
		return fmt.Errorf("contract violation: services.smackerel-ml not found — SCOPE-082-03 governs the ML model-cache persistence")
	}
	if !ml.ReadOnly {
		return fmt.Errorf("contract violation: services.smackerel-ml.read_only must remain true (SCOPE-082-03 keeps read-only root; the cache is writable only via the named-volume mount)")
	}
	// 1. The persistent ml-model-cache volume is mounted at the cache path.
	mounted := false
	for _, v := range ml.Volumes {
		if strings.HasPrefix(v, "ml-model-cache:"+mlModelCacheMountPath) {
			mounted = true
			break
		}
	}
	if !mounted {
		return fmt.Errorf("contract violation: services.smackerel-ml does not mount `ml-model-cache:%s` — SCOPE-082-03 requires the persistent embedding-model cache mounted at the image's baked cache path; got volumes %v", mlModelCacheMountPath, ml.Volumes)
	}
	// 2. HF_HOME + SENTENCE_TRANSFORMERS_HOME point INTO the persistent mount,
	//    not at the ephemeral /tmp tmpfs (the pre-082 re-download cause).
	for _, key := range []string{"HF_HOME", "SENTENCE_TRANSFORMERS_HOME"} {
		val := strings.TrimSpace(ml.Environment[key])
		if val == "" {
			return fmt.Errorf("contract violation: services.smackerel-ml.environment[%s] is missing — SCOPE-082-03 requires it to point into %s", key, mlModelCacheMountPath)
		}
		if strings.HasPrefix(val, "/tmp") {
			return fmt.Errorf("contract violation: services.smackerel-ml.environment[%s]=%q points at ephemeral /tmp — SCOPE-082-03 forbids this (it re-downloads the model on every restart); point it into the persistent %s mount", key, val, mlModelCacheMountPath)
		}
		if !strings.HasPrefix(val, mlModelCacheMountPath) {
			return fmt.Errorf("contract violation: services.smackerel-ml.environment[%s]=%q is not under the persistent mount %s — SCOPE-082-03 requires the caches to live on the durable volume", key, val, mlModelCacheMountPath)
		}
	}
	// 3. The top-level ml-model-cache volume is declared with an SST name and
	//    the persistent lifecycle label.
	vol, ok := doc.Volumes["ml-model-cache"]
	if !ok {
		return fmt.Errorf("contract violation: top-level volumes.ml-model-cache is not declared — SCOPE-082-03 requires it with name: ${ML_MODEL_CACHE_VOLUME_NAME}")
	}
	if !strings.Contains(vol.Name, "${ML_MODEL_CACHE_VOLUME_NAME}") {
		return fmt.Errorf("contract violation: volumes.ml-model-cache.name=%q is not the SST form ${ML_MODEL_CACHE_VOLUME_NAME} — SCOPE-082-03 keeps per-env volume isolation", vol.Name)
	}
	if vol.Labels["com.smackerel.lifecycle"] != "persistent" {
		return fmt.Errorf("contract violation: volumes.ml-model-cache must be labelled com.smackerel.lifecycle: persistent — SCOPE-082-03 protects the model cache from clean")
	}
	return nil
}

// TestMLModelCacheContract_LiveFile asserts the live deploy compose persists
// the embedding-model cache on a named volume at the baked cache path.
func TestMLModelCacheContract_LiveFile(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "deploy", "compose.deploy.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live compose file %q: %v", composePath, err)
	}
	if err := assertMLModelCacheContract(yamlBytes); err != nil {
		t.Fatalf("live deploy/compose.deploy.yml violates SCOPE-082-03 model-cache persistence contract: %v", err)
	}
	t.Logf("contract OK: smackerel-ml mounts persistent ml-model-cache at %s; HF_HOME/SENTENCE_TRANSFORMERS_HOME point into it; read-only root preserved (SCOPE-082-03)", mlModelCacheMountPath)
}

// TestMLModelCacheContract_AdversarialTmpHFHome proves the contract catches a
// regression that points HF_HOME back at the ephemeral /tmp tmpfs (the
// pre-082 re-download form).
func TestMLModelCacheContract_AdversarialTmpHFHome(t *testing.T) {
	const fixture = `services:
  smackerel-ml:
    read_only: true
    environment:
      HF_HOME: /tmp/hf-cache
      SENTENCE_TRANSFORMERS_HOME: /tmp/st-cache
    volumes:
      - ml-model-cache:/home/smackerel/.cache
volumes:
  ml-model-cache:
    name: ${ML_MODEL_CACHE_VOLUME_NAME}
    labels:
      com.smackerel.lifecycle: persistent
`
	err := assertMLModelCacheContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: HF_HOME=/tmp/hf-cache was ACCEPTED (a SCOPE-082-03 regression that re-downloads the model on every restart would NOT be caught)")
	}
	if !strings.Contains(err.Error(), "HF_HOME") {
		t.Fatalf("adversarial contract test failed: error did not mention 'HF_HOME': %v", err)
	}
	if !strings.Contains(err.Error(), "/tmp") {
		t.Fatalf("adversarial contract test failed: error did not flag the ephemeral /tmp path: %v", err)
	}
	t.Logf("adversarial OK: HF_HOME=/tmp/hf-cache is rejected with: %v", err)
}

// TestMLModelCacheContract_AdversarialMissingMount proves the contract fails
// if the persistent volume mount is dropped entirely.
func TestMLModelCacheContract_AdversarialMissingMount(t *testing.T) {
	const fixture = `services:
  smackerel-ml:
    read_only: true
    environment:
      HF_HOME: /home/smackerel/.cache/huggingface
      SENTENCE_TRANSFORMERS_HOME: /home/smackerel/.cache/sentence-transformers
    volumes:
      - ./prompt_contracts:/app/prompt_contracts:ro
volumes:
  ml-model-cache:
    name: ${ML_MODEL_CACHE_VOLUME_NAME}
    labels:
      com.smackerel.lifecycle: persistent
`
	err := assertMLModelCacheContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: missing ml-model-cache mount was ACCEPTED (the model would live on the read-only root with nowhere durable to persist)")
	}
	if !strings.Contains(err.Error(), "ml-model-cache") {
		t.Fatalf("adversarial contract test failed: error did not mention 'ml-model-cache': %v", err)
	}
	t.Logf("adversarial OK: missing persistent mount is rejected with: %v", err)
}
