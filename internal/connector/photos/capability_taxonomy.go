// Spec 040 Scope 5 — Capability taxonomy single source of truth.
//
// The Go capability registry, the API limitation envelopes, the NATS
// payload tags, and the PWA Provider Limitation banner strings ALL
// derive from this file. The capability-taxonomy canary asserts that
// any drift between these surfaces fails CI.
package photos

import (
	"sort"
	"strings"
)

// LimitationCode is a stable, machine-readable identifier returned in
// `409 PROVIDER_LIMITATION` envelopes, in NATS payload `limitation_code`
// fields, in audit rows, and as the data-attribute the PWA reads to
// look up the user-visible banner string.
//
// The codes intentionally use snake_case + the unsupported capability
// name so an operator can tell at a glance what the provider does not
// do (e.g. `faces_write_not_supported_by_provider`).
type LimitationCode string

const (
	// LimitationFacesWriteNotSupported — the provider exposes face
	// clusters read-only; rename/merge calls would silently no-op.
	LimitationFacesWriteNotSupported LimitationCode = "faces_write_not_supported_by_provider"

	// LimitationSensitivityNotInferred — the provider does not return
	// any sensitivity hints; downstream classification still runs but
	// callers MUST NOT rely on a provider-supplied label.
	LimitationSensitivityNotInferred LimitationCode = "sensitivity_not_inferred_by_provider"

	// LimitationWriteAlbumNotSupported — the provider has no album
	// concept (or a write-disabled album surface).
	LimitationWriteAlbumNotSupported LimitationCode = "write_album_not_supported_by_provider"

	// LimitationWriteTagNotSupported — the provider has no tag
	// concept or rejects tag mutations on the connected scope.
	LimitationWriteTagNotSupported LimitationCode = "write_tag_not_supported_by_provider"

	// LimitationWriteFavoriteNotSupported — the provider has no
	// favorite/star surface.
	LimitationWriteFavoriteNotSupported LimitationCode = "write_favorite_not_supported_by_provider"

	// LimitationDeleteNotSupported — the provider does not allow
	// delete via API (read-only mirror).
	LimitationDeleteNotSupported LimitationCode = "delete_not_supported_by_provider"

	// LimitationArchiveNotSupported — the provider has no archive
	// state.
	LimitationArchiveNotSupported LimitationCode = "archive_not_supported_by_provider"

	// LimitationUploadNotSupported — the provider rejects new
	// uploads via the smackerel API surface.
	LimitationUploadNotSupported LimitationCode = "upload_not_supported_by_provider"
)

// LimitationDescriptor binds one capability + status pair to its
// stable code, the Capability it governs, the human banner string
// shown on the PWA Provider Limitation Notice (Screen 15), and the
// short reason rendered by the Telegram channel.
type LimitationDescriptor struct {
	Code        LimitationCode
	Capability  Capability
	Status      CapabilityStatus
	BannerTitle string
	BannerBody  string
	TelegramMsg string
}

// limitationRegistry is the single source of truth. Adding a new
// limitation MUST add a row here (the canary test guards drift).
var limitationRegistry = map[LimitationCode]LimitationDescriptor{
	LimitationFacesWriteNotSupported: {
		Code:        LimitationFacesWriteNotSupported,
		Capability:  CapFacesWrite,
		Status:      CapabilityUnsupported,
		BannerTitle: "Provider Limitation",
		BannerBody:  "Renaming face clusters is not supported by this provider — search and read-only browsing keep working.",
		TelegramMsg: "I can't rename faces on this provider, but search still works.",
	},
	LimitationSensitivityNotInferred: {
		Code:        LimitationSensitivityNotInferred,
		Capability:  CapSensitivity,
		Status:      CapabilityLimited,
		BannerTitle: "Provider Limitation",
		BannerBody:  "This provider does not return sensitivity hints — Smackerel infers sensitivity locally.",
		TelegramMsg: "Sensitivity for this photo was inferred locally because the provider doesn't return hints.",
	},
	LimitationWriteAlbumNotSupported: {
		Code:        LimitationWriteAlbumNotSupported,
		Capability:  CapWriteAlbum,
		Status:      CapabilityUnsupported,
		BannerTitle: "Provider Limitation",
		BannerBody:  "Adding photos to albums is not supported by this provider.",
		TelegramMsg: "I can't add photos to albums on this provider.",
	},
	LimitationWriteTagNotSupported: {
		Code:        LimitationWriteTagNotSupported,
		Capability:  CapWriteTag,
		Status:      CapabilityUnsupported,
		BannerTitle: "Provider Limitation",
		BannerBody:  "Tagging photos is not supported by this provider.",
		TelegramMsg: "I can't tag photos on this provider.",
	},
	LimitationWriteFavoriteNotSupported: {
		Code:        LimitationWriteFavoriteNotSupported,
		Capability:  CapWriteFavorite,
		Status:      CapabilityUnsupported,
		BannerTitle: "Provider Limitation",
		BannerBody:  "Marking favorites is not supported by this provider.",
		TelegramMsg: "I can't favorite photos on this provider.",
	},
	LimitationDeleteNotSupported: {
		Code:        LimitationDeleteNotSupported,
		Capability:  CapDelete,
		Status:      CapabilityUnsupported,
		BannerTitle: "Provider Limitation",
		BannerBody:  "Deleting photos is not supported by this provider — review and archive elsewhere.",
		TelegramMsg: "I can't delete photos on this provider.",
	},
	LimitationArchiveNotSupported: {
		Code:        LimitationArchiveNotSupported,
		Capability:  CapArchive,
		Status:      CapabilityUnsupported,
		BannerTitle: "Provider Limitation",
		BannerBody:  "Archiving photos is not supported by this provider.",
		TelegramMsg: "I can't archive photos on this provider.",
	},
	LimitationUploadNotSupported: {
		Code:        LimitationUploadNotSupported,
		Capability:  CapUpload,
		Status:      CapabilityUnsupported,
		BannerTitle: "Provider Limitation",
		BannerBody:  "Uploading new photos is not supported by this provider.",
		TelegramMsg: "I can't upload to this provider.",
	},
}

// AllLimitationDescriptors returns the registry sorted by code so the
// canary test can compare slices deterministically.
func AllLimitationDescriptors() []LimitationDescriptor {
	out := make([]LimitationDescriptor, 0, len(limitationRegistry))
	for _, descriptor := range limitationRegistry {
		out = append(out, descriptor)
	}
	sort.Slice(out, func(i int, j int) bool {
		return string(out[i].Code) < string(out[j].Code)
	})
	return out
}

// LimitationDescriptorFor returns the descriptor for `code`, or
// `(zero, false)` when the code is not registered.
func LimitationDescriptorFor(code LimitationCode) (LimitationDescriptor, bool) {
	descriptor, ok := limitationRegistry[code]
	return descriptor, ok
}

// LimitationCodeForCapability returns the registered limitation code
// for the (capability, status) pair, or `("", false)` if the pair is
// supported or not in the registry.
func LimitationCodeForCapability(capability Capability, status CapabilityStatus) (LimitationCode, bool) {
	for _, descriptor := range limitationRegistry {
		if descriptor.Capability == capability && descriptor.Status == status {
			return descriptor.Code, true
		}
	}
	return "", false
}

// CheckCapability inspects `report` and returns the registered
// limitation descriptor when `capability` is unsupported or limited.
// The first return is `nil` when the capability is supported.
func CheckCapability(report CapabilityReport, capability Capability) *LimitationDescriptor {
	entry, ok := report.Capabilities[capability]
	if !ok {
		return nil
	}
	if entry.Status == CapabilitySupported {
		return nil
	}
	if strings.TrimSpace(string(entry.LimitationCode)) != "" {
		if descriptor, found := LimitationDescriptorFor(LimitationCode(entry.LimitationCode)); found {
			return &descriptor
		}
	}
	if code, found := LimitationCodeForCapability(capability, entry.Status); found {
		descriptor := limitationRegistry[code]
		return &descriptor
	}
	return nil
}

// LimitationBannerStrings returns the (title, body) the PWA banner
// renders for `code`. Returns `("", "")` when the code is unknown.
func LimitationBannerStrings(code LimitationCode) (string, string) {
	descriptor, ok := limitationRegistry[code]
	if !ok {
		return "", ""
	}
	return descriptor.BannerTitle, descriptor.BannerBody
}
