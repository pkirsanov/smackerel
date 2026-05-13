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
	for _, svcName := range roServices {
		svc, ok := doc.Services[svcName]
		if !ok {
			return fmt.Errorf("contract violation: services.%s not found in compose document", svcName)
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
		svc := doc.Services[svcName]
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
	if err := assertFilesystemContract(yamlBytes); err != nil {
		t.Fatalf("live deploy/compose.deploy.yml violates spec 045 FR-045-003 read-only filesystem contract: %v", err)
	}
	t.Logf("contract OK: deploy/compose.deploy.yml satisfies spec 045 FR-045-003 (read-only allowlist {smackerel-core, smackerel-ml, ollama} all declare read_only:true; exempt set {postgres, nats} do NOT; every tmpfs entry is in the documented allowlist)")
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
	if err := assertFilesystemContract(yamlBytes); err != nil {
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
