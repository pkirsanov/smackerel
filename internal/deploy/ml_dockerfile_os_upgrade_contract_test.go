// Spec 047 BUG-047-002 — ML Dockerfile OS-package upgrade static contract.
//
// Static-file contract for `ml/Dockerfile`. The contract:
//
//  1. `ml/Dockerfile` is a multi-stage Dockerfile. The LAST `FROM` stage is
//     the runtime stage that ships to the registry.
//  2. That runtime stage MUST contain a single `RUN` step that combines
//     `apt-get update` AND `apt-get -y upgrade` (or `apt-get upgrade -y`)
//     in the same RUN, so the apt index and the upgrade run against the
//     same package list view. (DD-1, AC-1, AC-2.)
//  3. That RUN MUST appear BEFORE any `COPY app/...` directive in the
//     runtime stage. The upgrade must be part of the immutable system
//     layer, not stacked on top of the mutable application layer.
//     (Design.md §Decision DD-2 condition 4.)
//
// These invariants live in `ml/Dockerfile`. This test parses the file
// directly (no Docker daemon required) and asserts the three conditions
// hold. Adversarial sub-tests prove the contract would FAIL if any
// invariant regressed (proves the test is not tautological).
//
// References:
//   - specs/047-ci-image-vulnerability-gate/bugs/BUG-047-002-trivy-ml-fixable-cve-regression/spec.md
//   - specs/047-ci-image-vulnerability-gate/bugs/BUG-047-002-trivy-ml-fixable-cve-regression/design.md
//   - specs/047-ci-image-vulnerability-gate/bugs/BUG-047-002-trivy-ml-fixable-cve-regression/scopes.md
//   - specs/047-ci-image-vulnerability-gate/report.md (R14 entry)
package deploy

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// loadMLDockerfile reads the live `ml/Dockerfile` from the repository root.
func loadMLDockerfile(t *testing.T) []byte {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "ml", "Dockerfile")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

// dockerfileLine represents one logical Dockerfile line. Continuation
// backslashes are folded so a multi-line `RUN ... && ... && ...` step
// shows up as a single logical line.
type dockerfileLine struct {
	directive string // upper-cased Dockerfile instruction (FROM, RUN, COPY, LABEL, ARG, ENV, WORKDIR, ...)
	body      string // everything after the directive, with continuations folded into one line
}

// parseDockerfile folds continuation lines and yields one dockerfileLine
// per logical instruction. Comments and blank lines are skipped.
func parseDockerfile(raw []byte) []dockerfileLine {
	var out []dockerfileLine
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var folded strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			// Comments and blanks end any in-progress fold.
			if folded.Len() > 0 {
				out = appendDirective(out, folded.String())
				folded.Reset()
			}
			continue
		}
		if strings.HasSuffix(trimmed, "\\") {
			folded.WriteString(strings.TrimSuffix(trimmed, "\\"))
			folded.WriteString(" ")
			continue
		}
		folded.WriteString(trimmed)
		out = appendDirective(out, folded.String())
		folded.Reset()
	}
	if folded.Len() > 0 {
		out = appendDirective(out, folded.String())
	}
	return out
}

// appendDirective splits "<DIRECTIVE> <body>" and appends to the slice.
func appendDirective(out []dockerfileLine, logical string) []dockerfileLine {
	logical = strings.TrimSpace(logical)
	if logical == "" {
		return out
	}
	fields := strings.SplitN(logical, " ", 2)
	directive := strings.ToUpper(fields[0])
	body := ""
	if len(fields) == 2 {
		body = strings.TrimSpace(fields[1])
	}
	return append(out, dockerfileLine{directive: directive, body: body})
}

// runtimeStage returns the slice of logical lines that belong to the LAST
// `FROM` stage. The runtime stage is by definition the one whose image
// gets tagged and pushed.
func runtimeStage(lines []dockerfileLine) []dockerfileLine {
	lastFromIdx := -1
	for i, l := range lines {
		if l.directive == "FROM" {
			lastFromIdx = i
		}
	}
	if lastFromIdx == -1 {
		return nil
	}
	return lines[lastFromIdx:]
}

// runContainsAptUpgradeBundle reports true if the RUN body invokes
// `apt-get update` AND (`apt-get -y upgrade` OR `apt-get upgrade -y`)
// in the same step. Both flag orders are accepted because both are
// idiomatic Debian/Ubuntu usage.
func runContainsAptUpgradeBundle(body string) bool {
	// Normalize multiple spaces (the parser already folded continuations).
	normalized := strings.Join(strings.Fields(body), " ")
	if !strings.Contains(normalized, "apt-get update") {
		return false
	}
	if !strings.Contains(normalized, "apt-get -y upgrade") &&
		!strings.Contains(normalized, "apt-get upgrade -y") {
		return false
	}
	return true
}

// assertMLDockerfileOSUpgradeContract checks that `ml/Dockerfile`'s runtime
// stage contains the apt-upgrade RUN step BEFORE any application COPY.
// Returns nil on success or an error describing the first violation.
func assertMLDockerfileOSUpgradeContract(raw []byte) error {
	lines := parseDockerfile(raw)
	if len(lines) == 0 {
		return fmt.Errorf("contract violation: ml/Dockerfile parsed to zero logical lines (file empty or unreadable)")
	}

	stage := runtimeStage(lines)
	if len(stage) == 0 || stage[0].directive != "FROM" {
		return fmt.Errorf("contract violation: ml/Dockerfile has no `FROM` line — cannot identify runtime stage")
	}

	// Locate the first apt-upgrade RUN and the first `COPY app/...` in the runtime stage.
	upgradeRunIdx := -1
	firstAppCopyIdx := -1
	for i, l := range stage {
		if l.directive == "RUN" && runContainsAptUpgradeBundle(l.body) && upgradeRunIdx == -1 {
			upgradeRunIdx = i
		}
		if l.directive == "COPY" && firstAppCopyIdx == -1 {
			// `COPY app/` (relative source) OR `COPY --from=builder ... app/`
			// (multi-stage source pointing into an application directory) both
			// count as the application layer for ordering purposes. The
			// general rule is "any COPY whose first source token references
			// `app/` or `app` as a path component".
			if copyTouchesAppDir(l.body) {
				firstAppCopyIdx = i
			}
		}
	}

	if upgradeRunIdx == -1 {
		return fmt.Errorf("contract violation: ml/Dockerfile runtime stage missing required `RUN apt-get update && apt-get -y upgrade` step (DD-1 / AC-1 / AC-2 in specs/047-ci-image-vulnerability-gate/bugs/BUG-047-002-trivy-ml-fixable-cve-regression/design.md)")
	}

	if firstAppCopyIdx != -1 && upgradeRunIdx > firstAppCopyIdx {
		return fmt.Errorf("contract violation: ml/Dockerfile apt-upgrade RUN at runtime-stage index %d appears AFTER application COPY at index %d — the upgrade must run in the immutable system layer before the application layer is added (design.md §Decision DD-2 condition 4)",
			upgradeRunIdx, firstAppCopyIdx)
	}

	return nil
}

// copyTouchesAppDir reports true if the COPY body references the application
// directory `app` or `app/...` as a source path. Both `COPY app/ /app/`
// and `COPY --from=builder /opt/foo app/...` flag.
func copyTouchesAppDir(body string) bool {
	for _, tok := range strings.Fields(body) {
		if tok == "app" || strings.HasPrefix(tok, "app/") {
			return true
		}
		if strings.HasSuffix(tok, "/app") || strings.Contains(tok, "/app/") {
			return true
		}
	}
	return false
}

// TestMLDockerfileOSUpgradeContract_LiveFile verifies the live
// `ml/Dockerfile` satisfies the DD-1 upgrade-step contract.
func TestMLDockerfileOSUpgradeContract_LiveFile(t *testing.T) {
	raw := loadMLDockerfile(t)
	if err := assertMLDockerfileOSUpgradeContract(raw); err != nil {
		t.Fatalf("live ml/Dockerfile violates spec 047 BUG-047-002 contract: %v", err)
	}
	t.Logf("contract OK: ml/Dockerfile runtime stage contains apt-upgrade RUN before application COPY")
}

// TestMLDockerfileOSUpgradeContract_AdversarialMissingUpgrade proves the
// contract function catches a regression where the apt-upgrade RUN step
// is removed from the runtime stage entirely. This is the exact mutation
// that would re-introduce BUG-047-002.
func TestMLDockerfileOSUpgradeContract_AdversarialMissingUpgrade(t *testing.T) {
	synthetic := []byte(`# Synthetic Dockerfile mirroring ml/Dockerfile structure but with the
# DD-1 apt-upgrade RUN removed. The contract MUST reject this.
FROM python:3.12-slim AS builder
WORKDIR /build
RUN pip install --upgrade pip
COPY requirements.txt /build/
RUN pip install --no-cache-dir --target /build/python-packages -r requirements.txt

FROM python:3.12-slim
ARG VERSION=unknown
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.source="https://github.com/smackerel/smackerel"
# DD-1 RUN step intentionally missing here — adversarial mutation.
WORKDIR /app
COPY --from=builder /build/python-packages /usr/local/lib/python3.12/site-packages
COPY app/ /app/
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8001"]
`)
	err := assertMLDockerfileOSUpgradeContract(synthetic)
	if err == nil {
		t.Fatal("expected adversarial Dockerfile (missing apt-upgrade RUN) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "missing required `RUN apt-get update && apt-get -y upgrade` step") {
		t.Fatalf("expected missing-upgrade violation, got: %v", err)
	}
	t.Logf("adversarial OK: Dockerfile without apt-upgrade RUN is rejected with: %v", err)
}

// TestMLDockerfileOSUpgradeContract_AdversarialDowngradedToUpdateOnly proves
// the contract catches a regression where someone keeps `apt-get update`
// but silently drops `apt-get -y upgrade` (the actual security action).
// Update-only refreshes the apt index but does not pull any updated
// packages — exactly the false-comfort failure mode the contract must
// catch.
func TestMLDockerfileOSUpgradeContract_AdversarialDowngradedToUpdateOnly(t *testing.T) {
	synthetic := []byte(`FROM python:3.12-slim AS builder
WORKDIR /build
COPY requirements.txt /build/

FROM python:3.12-slim
LABEL org.opencontainers.image.source="https://github.com/smackerel/smackerel"
# Downgraded: apt-get update kept, apt-get upgrade silently dropped.
RUN apt-get update && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY app/ /app/
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8001"]
`)
	err := assertMLDockerfileOSUpgradeContract(synthetic)
	if err == nil {
		t.Fatal("expected adversarial Dockerfile (apt-get update only, no upgrade) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "missing required `RUN apt-get update && apt-get -y upgrade` step") {
		t.Fatalf("expected missing-upgrade violation, got: %v", err)
	}
	t.Logf("adversarial OK: Dockerfile with apt-get update but no upgrade is rejected with: %v", err)
}

// TestMLDockerfileOSUpgradeContract_AdversarialUpgradeAfterAppCopy proves
// the contract catches the ordering regression where someone places the
// apt-upgrade RUN AFTER the application COPY. That ordering breaks the
// immutable-system-layer principle (DD-2 condition 4): the upgrade would
// sit in a higher layer than the application, defeating cache reuse and
// making the upgrade rebuild whenever the application changes.
func TestMLDockerfileOSUpgradeContract_AdversarialUpgradeAfterAppCopy(t *testing.T) {
	synthetic := []byte(`FROM python:3.12-slim AS builder
WORKDIR /build
COPY requirements.txt /build/

FROM python:3.12-slim
LABEL org.opencontainers.image.source="https://github.com/smackerel/smackerel"
WORKDIR /app
COPY app/ /app/
# Ordering violation: upgrade runs AFTER the application COPY.
RUN apt-get update && apt-get -y upgrade && rm -rf /var/lib/apt/lists/*
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8001"]
`)
	err := assertMLDockerfileOSUpgradeContract(synthetic)
	if err == nil {
		t.Fatal("expected adversarial Dockerfile (apt-upgrade after COPY app/) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "appears AFTER application COPY") {
		t.Fatalf("expected ordering violation, got: %v", err)
	}
	t.Logf("adversarial OK: Dockerfile with apt-upgrade after COPY app/ is rejected with: %v", err)
}
