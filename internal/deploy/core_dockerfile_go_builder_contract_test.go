// Spec 047 BUG-047-004 — production Go builder patch floor contract.
//
// Three trivy CRITICAL/HIGH findings have now raised this floor (BUG-047-002,
// BUG-047-004, and the 1.25.12 -> 1.25.13 stdlib bump). Each bump previously
// meant rewriting the same version literal in four places, so the contract
// message and both adversarial fixtures are DERIVED from the constants below.
// minimumAcceptedGoBuilderPatch is the one literal that stays hardcoded: it is
// what makes lowering the floor fail.
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
	// Floor 13 fixes the Go stdlib CVEs trivy flags at 1.25.12: CVE-2026-33818,
	// CVE-2026-46600, CVE-2026-56853, CVE-2026-56858, CVE-2026-56859,
	// CVE-2026-56860, CVE-2026-56862. Prior floor 12 covered CVE-2026-39822.
	requiredGoBuilderPatch = 13

	// minimumAcceptedGoBuilderPatch may only ever be RAISED. Deriving the
	// adversarial fixtures from requiredGoBuilderPatch keeps routine bumps
	// cheap, but it also means those fixtures would keep passing if the floor
	// were lowered. This literal is the guarantee they can no longer give.
	minimumAcceptedGoBuilderPatch = 13
)

// goBuilderSecurityFloor renders the active floor as major.minor.patch.
func goBuilderSecurityFloor() string {
	return fmt.Sprintf("%d.%d.%d", requiredGoBuilderMajor, requiredGoBuilderMinor, requiredGoBuilderPatch)
}

// goBuilderFixture renders a minimal builder-stage Dockerfile pinned to the
// given patch on the required major.minor line.
func goBuilderFixture(patch int) []byte {
	return []byte(fmt.Sprintf("FROM golang:%d.%d.%d-alpine AS builder\n", requiredGoBuilderMajor, requiredGoBuilderMinor, patch))
}

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
				"contract violation: builder Go version %s is below the required %s security floor",
				version, goBuilderSecurityFloor(),
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
	// One patch below the floor must fail, so a regression to the CVE-bearing
	// pin cannot pass silently.
	patch := requiredGoBuilderPatch - 1
	err := assertCoreDockerfileGoBuilderContract(goBuilderFixture(patch))
	if err == nil {
		t.Fatalf("expected below-floor Go %d.%d.%d builder to be rejected", requiredGoBuilderMajor, requiredGoBuilderMinor, patch)
	}
	if !strings.Contains(err.Error(), "security floor") {
		t.Fatalf("expected security-floor error, got: %v", err)
	}
}

func TestCoreDockerfileGoBuilderContract_AdversarialAcceptsPatchedPatch(t *testing.T) {
	if err := assertCoreDockerfileGoBuilderContract(goBuilderFixture(requiredGoBuilderPatch)); err != nil {
		t.Fatalf("expected at-floor Go %s builder to pass, got: %v", goBuilderSecurityFloor(), err)
	}
}

func TestCoreDockerfileGoBuilderContract_SecurityFloorRatchet(t *testing.T) {
	// The adversarial fixtures above track requiredGoBuilderPatch, so they
	// cannot detect the floor being lowered. This can.
	if requiredGoBuilderPatch < minimumAcceptedGoBuilderPatch {
		t.Fatalf(
			"security floor lowered: requiredGoBuilderPatch=%d is below minimumAcceptedGoBuilderPatch=%d; that constant may only ever be raised",
			requiredGoBuilderPatch, minimumAcceptedGoBuilderPatch,
		)
	}
}
