// Spec 047 BUG-047-004 — production Go builder patch floor contract.
package deploy

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

const (
	requiredGoBuilderMajor = 1
	requiredGoBuilderMinor = 25
	requiredGoBuilderPatch = 12
)

func assertCoreDockerfileGoBuilderContract(raw []byte) error {
	lines := parseDockerfile(raw)
	for _, line := range lines {
		if line.directive != "FROM" {
			continue
		}

		fields := strings.Fields(line.body)
		if len(fields) < 3 || !strings.EqualFold(fields[len(fields)-2], "AS") || !strings.EqualFold(fields[len(fields)-1], "builder") {
			continue
		}

		const imagePrefix = "golang:"
		const imageSuffix = "-alpine"
		image := fields[0]
		if !strings.HasPrefix(image, imagePrefix) || !strings.HasSuffix(image, imageSuffix) {
			return fmt.Errorf("contract violation: builder image %q must use golang:<version>-alpine", image)
		}

		version := strings.TrimSuffix(strings.TrimPrefix(image, imagePrefix), imageSuffix)
		parts := strings.Split(version, ".")
		if len(parts) != 3 {
			return fmt.Errorf("contract violation: builder Go version %q must contain major.minor.patch", version)
		}

		major, majorErr := strconv.Atoi(parts[0])
		minor, minorErr := strconv.Atoi(parts[1])
		patch, patchErr := strconv.Atoi(parts[2])
		if majorErr != nil || minorErr != nil || patchErr != nil {
			return fmt.Errorf("contract violation: builder Go version %q must be numeric major.minor.patch", version)
		}

		if major != requiredGoBuilderMajor || minor != requiredGoBuilderMinor || patch < requiredGoBuilderPatch {
			return fmt.Errorf(
				"contract violation: builder Go version %s is below the required 1.25.12 security floor for CVE-2026-39822",
				version,
			)
		}

		return nil
	}

	return fmt.Errorf("contract violation: Dockerfile has no stage named builder")
}

func TestCoreDockerfileGoBuilderContract_LiveFile(t *testing.T) {
	raw := loadCoreDockerfile(t)
	if err := assertCoreDockerfileGoBuilderContract(raw); err != nil {
		t.Fatalf("live Dockerfile violates BUG-047-004 builder contract: %v", err)
	}
}

func TestCoreDockerfileGoBuilderContract_AdversarialRejectsVulnerablePatch(t *testing.T) {
	// This invalid pre-fix pin must fail so reintroducing CVE-2026-39822 cannot pass silently.
	raw := []byte("FROM golang:1.25.11-alpine AS builder\n")
	err := assertCoreDockerfileGoBuilderContract(raw)
	if err == nil {
		t.Fatal("expected vulnerable Go 1.25.11 builder to be rejected")
	}
	if !strings.Contains(err.Error(), "below the required 1.25.12 security floor") {
		t.Fatalf("expected security-floor error, got: %v", err)
	}
}

func TestCoreDockerfileGoBuilderContract_AdversarialAcceptsPatchedPatch(t *testing.T) {
	raw := []byte("FROM golang:1.25.12-alpine AS builder\n")
	if err := assertCoreDockerfileGoBuilderContract(raw); err != nil {
		t.Fatalf("expected patched Go 1.25.12 builder to pass, got: %v", err)
	}
}
