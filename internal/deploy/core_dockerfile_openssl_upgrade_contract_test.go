// Spec 047 BUG-047-003 — smackerel-core Dockerfile OpenSSL apk-upgrade static contract.
//
// Static-file contract for the top-level `Dockerfile` (smackerel-core). The contract:
//
//  1. The top-level `Dockerfile` is multi-stage. The LAST `FROM` stage is the
//     runtime stage (`FROM alpine:3.22 AS core`) whose image ships to the registry.
//  2. That runtime stage MUST contain a `RUN` step that invokes `apk upgrade`
//     AND names BOTH `libssl3` and `libcrypto3`, so the OpenSSL packages are
//     pulled forward to the CVE-2026-45447-patched Alpine version (3.5.7-r0+).
//     (DD-1, AC-3, AC-5.)
//  3. That RUN MUST appear BEFORE the application COPY of the `smackerel-core`
//     binary, so the upgrade sits in the immutable system layer rather than on
//     top of the mutable application layer. (Design.md §Decision DD-2.)
//
// These invariants live in the top-level `Dockerfile`. This test parses the file
// directly (no Docker daemon required) and asserts the three conditions hold.
// Adversarial sub-tests prove the contract would FAIL if any invariant regressed
// (proves the test is not tautological).
//
// Shared Dockerfile parse helpers (parseDockerfile, runtimeStage, dockerfileLine)
// are defined in the sibling ml_dockerfile_os_upgrade_contract_test.go (same
// `deploy` package); this file adds only core-specific assertions, so there is
// no duplication.
//
// References:
//   - specs/047-ci-image-vulnerability-gate/bugs/BUG-047-003-trivy-core-openssl-cve-regression/spec.md
//   - specs/047-ci-image-vulnerability-gate/bugs/BUG-047-003-trivy-core-openssl-cve-regression/design.md
//   - specs/047-ci-image-vulnerability-gate/bugs/BUG-047-003-trivy-core-openssl-cve-regression/scopes.md
//   - specs/047-ci-image-vulnerability-gate/report.md (R15 entry)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// loadCoreDockerfile reads the live top-level `Dockerfile` from the repo root.
func loadCoreDockerfile(t *testing.T) []byte {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "Dockerfile")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

// runContainsApkOpenSSLUpgrade reports true if the RUN body invokes
// `apk upgrade` AND names BOTH `libssl3` and `libcrypto3` as upgrade
// targets in the same step. A bare `apk upgrade` that does not name the
// OpenSSL packages does NOT satisfy the contract (the targeted form is
// the spec 047 R15 / DD-1 fix shape).
func runContainsApkOpenSSLUpgrade(body string) bool {
	normalized := strings.Join(strings.Fields(body), " ")
	if !strings.Contains(normalized, "apk upgrade") {
		return false
	}
	if !strings.Contains(normalized, "libssl3") || !strings.Contains(normalized, "libcrypto3") {
		return false
	}
	return true
}

// copyTouchesCoreBinary reports true if the COPY body references the
// `smackerel-core` binary as a path token (source or destination). The
// real Dockerfile uses `COPY --from=builder /bin/smackerel-core
// /usr/local/bin/smackerel-core`, so any token containing the binary name
// flags the application layer for ordering purposes.
func copyTouchesCoreBinary(body string) bool {
	for _, tok := range strings.Fields(body) {
		if strings.Contains(tok, "smackerel-core") {
			return true
		}
	}
	return false
}

// assertCoreDockerfileOpenSSLUpgradeContract checks that the top-level
// `Dockerfile`'s runtime stage contains the apk-upgrade-openssl RUN step
// BEFORE the smackerel-core binary COPY. Returns nil on success or an
// error describing the first violation.
func assertCoreDockerfileOpenSSLUpgradeContract(raw []byte) error {
	lines := parseDockerfile(raw)
	if len(lines) == 0 {
		return fmt.Errorf("contract violation: Dockerfile parsed to zero logical lines (file empty or unreadable)")
	}

	stage := runtimeStage(lines)
	if len(stage) == 0 || stage[0].directive != "FROM" {
		return fmt.Errorf("contract violation: Dockerfile has no `FROM` line — cannot identify runtime stage")
	}

	// Locate the first apk-upgrade-openssl RUN and the first smackerel-core
	// binary COPY in the runtime stage.
	upgradeRunIdx := -1
	firstBinaryCopyIdx := -1
	for i, l := range stage {
		if l.directive == "RUN" && runContainsApkOpenSSLUpgrade(l.body) && upgradeRunIdx == -1 {
			upgradeRunIdx = i
		}
		if l.directive == "COPY" && firstBinaryCopyIdx == -1 && copyTouchesCoreBinary(l.body) {
			firstBinaryCopyIdx = i
		}
	}

	if upgradeRunIdx == -1 {
		return fmt.Errorf("contract violation: Dockerfile runtime stage missing required `RUN apk upgrade ... libssl3 libcrypto3` step (DD-1 / AC-3 / AC-5 in specs/047-ci-image-vulnerability-gate/bugs/BUG-047-003-trivy-core-openssl-cve-regression/design.md)")
	}

	if firstBinaryCopyIdx != -1 && upgradeRunIdx > firstBinaryCopyIdx {
		return fmt.Errorf("contract violation: Dockerfile apk-upgrade RUN at runtime-stage index %d appears AFTER the smackerel-core binary COPY at index %d — the upgrade must run in the immutable system layer before the application binary is added (design.md §Decision DD-2)",
			upgradeRunIdx, firstBinaryCopyIdx)
	}

	return nil
}

// TestCoreDockerfileOpenSSLUpgradeContract_LiveFile verifies the live
// top-level `Dockerfile` satisfies the DD-1 OpenSSL upgrade-step contract.
func TestCoreDockerfileOpenSSLUpgradeContract_LiveFile(t *testing.T) {
	raw := loadCoreDockerfile(t)
	if err := assertCoreDockerfileOpenSSLUpgradeContract(raw); err != nil {
		t.Fatalf("live Dockerfile violates spec 047 BUG-047-003 contract: %v", err)
	}
	t.Logf("contract OK: Dockerfile runtime stage contains apk-upgrade(libssl3,libcrypto3) RUN before the smackerel-core binary COPY")
}

// TestCoreDockerfileOpenSSLUpgradeContract_AdversarialMissingUpgrade proves the
// contract catches a regression where the apk-upgrade RUN step is removed from
// the runtime stage entirely. This is the exact mutation that would re-introduce
// BUG-047-003 (CVE-2026-45447 re-surfaces).
func TestCoreDockerfileOpenSSLUpgradeContract_AdversarialMissingUpgrade(t *testing.T) {
	synthetic := []byte(`# Synthetic Dockerfile mirroring the smackerel-core structure but with the
# DD-1 apk-upgrade step removed. The contract MUST reject this.
FROM golang:1.25-alpine AS builder
WORKDIR /src
RUN CGO_ENABLED=0 go build -o /bin/smackerel-core ./cmd/core

FROM alpine:3.22 AS core
LABEL org.opencontainers.image.source="https://github.com/smackerel/smackerel"
# DD-1 apk-upgrade step intentionally missing here — adversarial mutation.
RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S smackerel && adduser -S smackerel -G smackerel
COPY --from=builder /bin/smackerel-core /usr/local/bin/smackerel-core
USER smackerel
ENTRYPOINT ["smackerel-core"]
`)
	err := assertCoreDockerfileOpenSSLUpgradeContract(synthetic)
	if err == nil {
		t.Fatal("expected adversarial Dockerfile (missing apk-upgrade RUN) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "missing required `RUN apk upgrade ... libssl3 libcrypto3` step") {
		t.Fatalf("expected missing-upgrade violation, got: %v", err)
	}
	t.Logf("adversarial OK: Dockerfile without apk-upgrade RUN is rejected with: %v", err)
}

// TestCoreDockerfileOpenSSLUpgradeContract_AdversarialUpgradeMissingOpenSSL proves
// the contract catches a regression where someone keeps an `apk upgrade` but
// drops the OpenSSL package targets (e.g. upgrades only busybox). Such an upgrade
// would NOT pull libssl3/libcrypto3 forward — exactly the false-comfort failure
// mode the contract must catch.
func TestCoreDockerfileOpenSSLUpgradeContract_AdversarialUpgradeMissingOpenSSL(t *testing.T) {
	synthetic := []byte(`FROM golang:1.25-alpine AS builder
RUN CGO_ENABLED=0 go build -o /bin/smackerel-core ./cmd/core

FROM alpine:3.22 AS core
LABEL org.opencontainers.image.source="https://github.com/smackerel/smackerel"
# Weakened: apk upgrade present but does NOT name libssl3/libcrypto3.
RUN apk add --no-cache ca-certificates tzdata && apk upgrade --no-cache busybox
COPY --from=builder /bin/smackerel-core /usr/local/bin/smackerel-core
USER smackerel
ENTRYPOINT ["smackerel-core"]
`)
	err := assertCoreDockerfileOpenSSLUpgradeContract(synthetic)
	if err == nil {
		t.Fatal("expected adversarial Dockerfile (apk upgrade not covering OpenSSL) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "missing required `RUN apk upgrade ... libssl3 libcrypto3` step") {
		t.Fatalf("expected missing-upgrade violation, got: %v", err)
	}
	t.Logf("adversarial OK: Dockerfile with apk upgrade not covering libssl3/libcrypto3 is rejected with: %v", err)
}

// TestCoreDockerfileOpenSSLUpgradeContract_AdversarialUpgradeAfterAppCopy proves
// the contract catches the ordering regression where the apk-upgrade RUN is
// placed AFTER the smackerel-core binary COPY. That ordering breaks the
// immutable-system-layer principle (DD-2): the upgrade would sit in a higher
// layer than the application binary, defeating cache reuse and rebuilding the
// upgrade whenever the binary changes.
func TestCoreDockerfileOpenSSLUpgradeContract_AdversarialUpgradeAfterAppCopy(t *testing.T) {
	synthetic := []byte(`FROM golang:1.25-alpine AS builder
RUN CGO_ENABLED=0 go build -o /bin/smackerel-core ./cmd/core

FROM alpine:3.22 AS core
LABEL org.opencontainers.image.source="https://github.com/smackerel/smackerel"
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/smackerel-core /usr/local/bin/smackerel-core
# Ordering violation: upgrade runs AFTER the smackerel-core binary COPY.
RUN apk upgrade --no-cache libssl3 libcrypto3
USER smackerel
ENTRYPOINT ["smackerel-core"]
`)
	err := assertCoreDockerfileOpenSSLUpgradeContract(synthetic)
	if err == nil {
		t.Fatal("expected adversarial Dockerfile (apk-upgrade after smackerel-core COPY) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "appears AFTER the smackerel-core binary COPY") {
		t.Fatalf("expected ordering violation, got: %v", err)
	}
	t.Logf("adversarial OK: Dockerfile with apk-upgrade after the binary COPY is rejected with: %v", err)
}
