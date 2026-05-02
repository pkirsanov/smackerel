package photos

import (
	"strings"
	"testing"
	"time"
)

// TestProviderCapabilityLimitationsReturnStableCodes proves the
// capability-taxonomy registry returns stable, lookup-able codes for
// every limited or unsupported capability the API and PWA depend on.
//
// SCN-040-013: Provider limitation is visible and non-mutating. The
// 409 PROVIDER_LIMITATION envelope MUST carry one of these codes.
func TestProviderCapabilityLimitationsReturnStableCodes(t *testing.T) {
	cases := []struct {
		name       string
		capability Capability
		status     CapabilityStatus
		want       LimitationCode
	}{
		{"faces_write_unsupported", CapFacesWrite, CapabilityUnsupported, LimitationFacesWriteNotSupported},
		{"sensitivity_limited", CapSensitivity, CapabilityLimited, LimitationSensitivityNotInferred},
		{"write_album_unsupported", CapWriteAlbum, CapabilityUnsupported, LimitationWriteAlbumNotSupported},
		{"write_tag_unsupported", CapWriteTag, CapabilityUnsupported, LimitationWriteTagNotSupported},
		{"write_favorite_unsupported", CapWriteFavorite, CapabilityUnsupported, LimitationWriteFavoriteNotSupported},
		{"delete_unsupported", CapDelete, CapabilityUnsupported, LimitationDeleteNotSupported},
		{"archive_unsupported", CapArchive, CapabilityUnsupported, LimitationArchiveNotSupported},
		{"upload_unsupported", CapUpload, CapabilityUnsupported, LimitationUploadNotSupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := LimitationCodeForCapability(tc.capability, tc.status)
			if !ok {
				t.Fatalf("LimitationCodeForCapability(%q, %q) = (_, false), want true", tc.capability, tc.status)
			}
			if got != tc.want {
				t.Fatalf("limitation code = %q, want %q", got, tc.want)
			}
			descriptor, ok := LimitationDescriptorFor(tc.want)
			if !ok {
				t.Fatalf("LimitationDescriptorFor(%q) returned !ok", tc.want)
			}
			if descriptor.Capability != tc.capability {
				t.Fatalf("descriptor.Capability = %q, want %q", descriptor.Capability, tc.capability)
			}
			if descriptor.Status != tc.status {
				t.Fatalf("descriptor.Status = %q, want %q", descriptor.Status, tc.status)
			}
			if !strings.HasSuffix(string(descriptor.Code), "_by_provider") {
				t.Fatalf("limitation code %q must end with _by_provider for stable taxonomy", descriptor.Code)
			}
			if descriptor.BannerTitle == "" || descriptor.BannerBody == "" {
				t.Fatalf("descriptor for %q is missing banner strings", tc.want)
			}
			if descriptor.TelegramMsg == "" {
				t.Fatalf("descriptor for %q is missing Telegram message", tc.want)
			}
		})
	}
}

// TestProviderCapabilityRegistryIsUnique guards against two descriptors
// claiming the same code or the same (capability, status) pair, which
// would let the API and PWA disagree on what a code means.
func TestProviderCapabilityRegistryIsUnique(t *testing.T) {
	all := AllLimitationDescriptors()
	if len(all) == 0 {
		t.Fatalf("AllLimitationDescriptors returned 0 entries")
	}
	seenCodes := map[LimitationCode]struct{}{}
	seenPairs := map[string]struct{}{}
	for _, descriptor := range all {
		if _, dup := seenCodes[descriptor.Code]; dup {
			t.Fatalf("limitation code %q appears twice in registry", descriptor.Code)
		}
		seenCodes[descriptor.Code] = struct{}{}
		key := string(descriptor.Capability) + "|" + string(descriptor.Status)
		if _, dup := seenPairs[key]; dup {
			t.Fatalf("(capability=%s, status=%s) appears twice in registry", descriptor.Capability, descriptor.Status)
		}
		seenPairs[key] = struct{}{}
	}
}

// TestCheckCapabilityRespectsCapabilityReport proves the lookup helper
// returns nil for supported capabilities and the registered descriptor
// for limited/unsupported ones, even when the entry omits an explicit
// limitation_code (the canary path the API depends on).
func TestCheckCapabilityRespectsCapabilityReport(t *testing.T) {
	report := CapabilityReport{
		Provider:        "test-provider",
		ProviderVersion: "v1.0",
		DetectedAt:      time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Capabilities: map[Capability]CapabilityEntry{
			CapRead:        {Status: CapabilitySupported},
			CapFacesWrite:  {Status: CapabilityUnsupported},
			CapSensitivity: {Status: CapabilityLimited, LimitationCode: string(LimitationSensitivityNotInferred)},
			CapWriteAlbum:  {Status: CapabilitySupported},
		},
	}
	if descriptor := CheckCapability(report, CapRead); descriptor != nil {
		t.Fatalf("CheckCapability(read) = %+v, want nil for supported", *descriptor)
	}
	if descriptor := CheckCapability(report, CapWriteAlbum); descriptor != nil {
		t.Fatalf("CheckCapability(write_album) = %+v, want nil for supported", *descriptor)
	}
	descriptor := CheckCapability(report, CapFacesWrite)
	if descriptor == nil {
		t.Fatalf("CheckCapability(faces_write) = nil, want LimitationFacesWriteNotSupported")
	}
	if descriptor.Code != LimitationFacesWriteNotSupported {
		t.Fatalf("descriptor.Code = %q, want %q", descriptor.Code, LimitationFacesWriteNotSupported)
	}
	descriptorSensitivity := CheckCapability(report, CapSensitivity)
	if descriptorSensitivity == nil {
		t.Fatalf("CheckCapability(sensitivity) = nil, want LimitationSensitivityNotInferred")
	}
	if descriptorSensitivity.Code != LimitationSensitivityNotInferred {
		t.Fatalf("sensitivity descriptor.Code = %q, want %q", descriptorSensitivity.Code, LimitationSensitivityNotInferred)
	}
}
