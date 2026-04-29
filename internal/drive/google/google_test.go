// Spec 038 Scope 1 DoD-4 — the Google Drive provider implements the neutral
// drive.Provider contract with a config-injected Capabilities() surface.
// These tests prove:
//
//  1. New(caps) returns a Provider whose Capabilities() is the exact value
//     supplied (config-injection path).
//  2. NewFromConfig translates SST-resolved scalar values into the neutral
//     Capabilities struct, including the runtime-tighter MaxFileSizeBytes
//     value sourced from drive.limits.max_file_size_bytes.
//  3. DefaultCapabilities advertises Google Drive's published platform
//     ceiling (5 TiB) so the init()-registered provider has a sensible
//     pre-configure default.
//  4. The init()-registered Google provider is reachable via the
//     package-default registry under the stable "google" ID with the
//     contract-required capability flags set.
//  5. Configure swaps capabilities at runtime so main wiring can overwrite
//     the init() default without re-registering.
//
// These tests run in unit category — no live Drive API calls.
package google

import (
	"testing"

	"github.com/smackerel/smackerel/internal/drive"
)

func TestGoogleProviderConfigInjectedCapabilities(t *testing.T) {
	want := drive.Capabilities{
		SupportsVersions:      true,
		SupportsSharing:       true,
		SupportsChangeHistory: true,
		MaxFileSizeBytes:      104857600, // matches dev SST DRIVE_LIMITS_MAX_FILE_SIZE_BYTES (100 MiB)
		SupportedMimeFilter:   []string{"application/pdf", "image/jpeg"},
	}

	p := New(want)
	got := p.Capabilities()

	if got.SupportsVersions != want.SupportsVersions {
		t.Errorf("SupportsVersions = %v, want %v", got.SupportsVersions, want.SupportsVersions)
	}
	if got.SupportsSharing != want.SupportsSharing {
		t.Errorf("SupportsSharing = %v, want %v", got.SupportsSharing, want.SupportsSharing)
	}
	if got.SupportsChangeHistory != want.SupportsChangeHistory {
		t.Errorf("SupportsChangeHistory = %v, want %v", got.SupportsChangeHistory, want.SupportsChangeHistory)
	}
	if got.MaxFileSizeBytes != want.MaxFileSizeBytes {
		t.Errorf("MaxFileSizeBytes = %d, want %d (config-injected value)", got.MaxFileSizeBytes, want.MaxFileSizeBytes)
	}
	if len(got.SupportedMimeFilter) != 2 {
		t.Fatalf("SupportedMimeFilter length = %d, want 2", len(got.SupportedMimeFilter))
	}
	if got.SupportedMimeFilter[0] != "application/pdf" || got.SupportedMimeFilter[1] != "image/jpeg" {
		t.Errorf("SupportedMimeFilter = %v, want [application/pdf image/jpeg]", got.SupportedMimeFilter)
	}
}

func TestGoogleProviderNewFromConfigUsesSSTLimits(t *testing.T) {
	// Simulate runtime wiring: pass the SST-resolved
	// drive.limits.max_file_size_bytes value (100 MiB in dev) and a
	// nil MIME filter (provider-level allow-any).
	const sstMaxBytes int64 = 100 * 1024 * 1024
	p := NewFromConfig(sstMaxBytes, nil)

	caps := p.Capabilities()
	if caps.MaxFileSizeBytes != sstMaxBytes {
		t.Errorf("NewFromConfig MaxFileSizeBytes = %d, want %d (SST value)", caps.MaxFileSizeBytes, sstMaxBytes)
	}
	if !caps.SupportsVersions || !caps.SupportsSharing || !caps.SupportsChangeHistory {
		t.Errorf("NewFromConfig dropped capability flags: %+v", caps)
	}
	if caps.SupportedMimeFilter != nil {
		t.Errorf("NewFromConfig SupportedMimeFilter = %v, want nil for allow-any", caps.SupportedMimeFilter)
	}
}

func TestGoogleProviderNewFromConfigFallsBackToDefaultCeilingOnZero(t *testing.T) {
	// When the caller passes a non-positive value (regression guard), the
	// provider MUST fall back to googleAPIHardCeilingBytes so the registry
	// always advertises a non-zero ceiling.
	p := NewFromConfig(0, nil)
	if got, want := p.Capabilities().MaxFileSizeBytes, googleAPIHardCeilingBytes; got != want {
		t.Errorf("zero-input fallback MaxFileSizeBytes = %d, want %d (hard ceiling)", got, want)
	}
}

func TestGoogleProviderDefaultCapabilitiesUsePublishedCeiling(t *testing.T) {
	caps := DefaultCapabilities()
	if !caps.SupportsVersions {
		t.Error("DefaultCapabilities.SupportsVersions = false, want true")
	}
	if !caps.SupportsSharing {
		t.Error("DefaultCapabilities.SupportsSharing = false, want true")
	}
	if !caps.SupportsChangeHistory {
		t.Error("DefaultCapabilities.SupportsChangeHistory = false, want true")
	}
	if caps.MaxFileSizeBytes != googleAPIHardCeilingBytes {
		t.Errorf("DefaultCapabilities.MaxFileSizeBytes = %d, want %d (Google 5 TiB ceiling)", caps.MaxFileSizeBytes, googleAPIHardCeilingBytes)
	}
	if caps.SupportedMimeFilter != nil {
		t.Errorf("DefaultCapabilities.SupportedMimeFilter = %v, want nil (allow-any at provider boundary)", caps.SupportedMimeFilter)
	}
}

func TestGoogleProviderRegistersWithDefaultRegistry(t *testing.T) {
	// init() must have registered the Google provider into
	// drive.DefaultRegistry under the stable ID "google".
	p, ok := drive.DefaultRegistry.Get("google")
	if !ok {
		t.Fatal(`drive.DefaultRegistry.Get("google") = ok=false; init() registration regressed`)
	}
	if p.ID() != "google" {
		t.Errorf("registered provider ID = %q, want %q", p.ID(), "google")
	}
	if p.DisplayName() != "Google Drive" {
		t.Errorf("registered provider DisplayName = %q, want %q", p.DisplayName(), "Google Drive")
	}
	caps := p.Capabilities()
	if !caps.SupportsVersions || !caps.SupportsSharing || !caps.SupportsChangeHistory {
		t.Errorf("registered provider Capabilities = %+v, want all three flags true", caps)
	}
	if caps.MaxFileSizeBytes <= 0 {
		t.Errorf("registered provider MaxFileSizeBytes = %d, want > 0", caps.MaxFileSizeBytes)
	}
}

func TestGoogleProviderConfigureOverwritesCapabilities(t *testing.T) {
	p := New(DefaultCapabilities())

	// Overwrite to simulate main wiring applying SST limits to the
	// init()-registered provider.
	const newMax int64 = 50 * 1024 * 1024
	p.Configure(drive.Capabilities{
		SupportsVersions:      false,
		SupportsSharing:       true,
		SupportsChangeHistory: false,
		MaxFileSizeBytes:      newMax,
		SupportedMimeFilter:   []string{"text/plain"},
	})

	caps := p.Capabilities()
	if caps.SupportsVersions {
		t.Error("after Configure SupportsVersions = true, want false")
	}
	if caps.SupportsChangeHistory {
		t.Error("after Configure SupportsChangeHistory = true, want false")
	}
	if caps.MaxFileSizeBytes != newMax {
		t.Errorf("after Configure MaxFileSizeBytes = %d, want %d", caps.MaxFileSizeBytes, newMax)
	}
	if len(caps.SupportedMimeFilter) != 1 || caps.SupportedMimeFilter[0] != "text/plain" {
		t.Errorf("after Configure SupportedMimeFilter = %v, want [text/plain]", caps.SupportedMimeFilter)
	}
}
