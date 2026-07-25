package web

// Spec 106 SCOPE-106-01 — Source-Locked Visual Assets foundation.
//
// One ExperienceAssetManifest is the single authority for the byte identity,
// provenance, licence, CSP class, and service-worker policy of every shared
// same-origin CSS/JS/icon asset that the coherent product experience serves to
// BOTH the server-rendered templates (internal/web) and the PWA (web/pwa).
//
// The manifest computes REAL SHA-256 digests over the actually-embedded bytes
// (web/pwa //go:embed). It NEVER fabricates a digest: an asset whose same-origin
// bytes are not present in the repo/environment (the pinned CDN htmx owned by
// BUG-002-006, and the not-yet-vendored fonts) is recorded honestly under
// ExternalDependencies WITHOUT a digest, never as a source-locked byte.
//
// This scope supplies the foundation only. It does NOT change active navigation
// or rewire any renderer head (that is SCOPE-106-04/05); it establishes the
// locked assets, tokens, appearance codec, and cache identity those later
// scopes consume.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	pwa "github.com/smackerel/smackerel/web/pwa"
)

// ExperienceAssetCSPClass names the CSP directive an asset is served under.
type ExperienceAssetCSPClass string

const (
	CSPClassStyle  ExperienceAssetCSPClass = "style-src"
	CSPClassScript ExperienceAssetCSPClass = "script-src"
	CSPClassImage  ExperienceAssetCSPClass = "img-src"
	CSPClassFont   ExperienceAssetCSPClass = "font-src"
)

// ExperienceServiceWorkerPolicy declares how the service worker treats an asset.
//
// Only manifest-approved, immutable, same-origin static GET assets are precached.
// Authenticated HTML, /api/*, /v1/*, non-GET requests, and business responses are
// network-only and never appear as precached ExperienceAssets.
type ExperienceServiceWorkerPolicy string

const (
	SWPrecacheImmutable ExperienceServiceWorkerPolicy = "precache-immutable"
)

// ExperienceAssetSource is the provenance class of a locked asset.
type ExperienceAssetSource string

const (
	SourceFirstParty ExperienceAssetSource = "first-party"
)

// firstPartyLicense is the licence carried by every first-party same-origin asset.
const firstPartyLicense = "Smackerel repository LICENSE"

// ExperienceAsset is one byte-integrity-locked same-origin asset.
type ExperienceAsset struct {
	// ServedPath is the same-origin URL path (e.g. /pwa/experience-tokens.css).
	ServedPath string
	// EmbedPath is the path within web/pwa //go:embed.
	EmbedPath string
	Source    ExperienceAssetSource
	License   string
	// SHA256 is the lowercase hex digest of the exact served bytes.
	SHA256    string
	Size      int64
	MediaType string
	CSPClass  ExperienceAssetCSPClass
	SWPolicy  ExperienceServiceWorkerPolicy
}

// ExternalExperienceDependency records an asset that is intentionally NOT yet a
// same-origin source-locked byte in this repo/environment. It carries no digest;
// its status is the honest, non-fabricated posture.
type ExternalExperienceDependency struct {
	Name            string
	Owner           string
	Status          string // pending-same-origin-migration | not-yet-vendored-network-required
	CurrentSource   string
	IntendedLicense string
	Note            string
}

const (
	ExtStatusPendingSameOrigin = "pending-same-origin-migration"
	ExtStatusNotVendored       = "not-yet-vendored-network-required"
)

// ExperienceAssetManifest is the single source-of-truth for the shared visual
// foundation. Version 1.
type ExperienceAssetManifest struct {
	Version              int
	Assets               []ExperienceAsset
	ExternalDependencies []ExternalExperienceDependency
	// CacheIdentity is a content-derived aggregate digest over every locked asset
	// (sorted served path + digest). It advances atomically whenever any locked
	// byte changes, which is what drives the service-worker cache version.
	CacheIdentity string
}

// experienceAssetSpec declares the locked assets by embed path + metadata. The
// bytes/size/digest are filled from the real embed at build time.
type experienceAssetSpec struct {
	embedPath string
	mediaType string
	cspClass  ExperienceAssetCSPClass
}

// lockedAssetSpecs is the closed set of same-origin foundation assets. Adding an
// asset is a reviewed change here; a missing declared asset fails the build.
var lockedAssetSpecs = []experienceAssetSpec{
	{"experience-appearance.js", "text/javascript; charset=utf-8", CSPClassScript},
	{"experience-tokens.css", "text/css; charset=utf-8", CSPClassStyle},
	{"style.css", "text/css; charset=utf-8", CSPClassStyle},
	{"app.js", "text/javascript; charset=utf-8", CSPClassScript},
	{"lib/appnav.js", "text/javascript; charset=utf-8", CSPClassScript},
	{"lib/queue.js", "text/javascript; charset=utf-8", CSPClassScript},
	{"icon.svg", "image/svg+xml", CSPClassImage},
}

// externalDependencies is the honest record of assets not yet same-origin locked.
var externalDependencies = []ExternalExperienceDependency{
	{
		Name:            "htmx.org@1.9.12",
		Owner:           "BUG-002-006",
		Status:          ExtStatusPendingSameOrigin,
		CurrentSource:   "https://unpkg.com/htmx.org@1.9.12 (pinned CDN <script> in internal/web/templates.go)",
		IntendedLicense: "BSD-0 / MIT (htmx)",
		Note:            "BUG-002-006 owns the same-origin vendoring + digest; this manifest references that digest once it exists and embeds no second copy.",
	},
	{
		Name:            "IBM Plex Sans",
		Owner:           "spec 106 SCOPE-106-01",
		Status:          ExtStatusNotVendored,
		CurrentSource:   "declared family in experience-tokens.css with platform fallback",
		IntendedLicense: "SIL OFL 1.1",
		Note:            "same-origin .woff2 byte vendoring requires a network fetch unavailable in this build environment; no digest fabricated.",
	},
	{
		Name:            "Source Serif 4",
		Owner:           "spec 106 SCOPE-106-01",
		Status:          ExtStatusNotVendored,
		CurrentSource:   "declared family in experience-tokens.css with platform fallback",
		IntendedLicense: "SIL OFL 1.1",
		Note:            "same-origin .woff2 byte vendoring requires a network fetch unavailable in this build environment; no digest fabricated.",
	},
	{
		Name:            "IBM Plex Mono",
		Owner:           "spec 106 SCOPE-106-01",
		Status:          ExtStatusNotVendored,
		CurrentSource:   "declared family in experience-tokens.css with platform fallback",
		IntendedLicense: "SIL OFL 1.1",
		Note:            "same-origin .woff2 byte vendoring requires a network fetch unavailable in this build environment; no digest fabricated.",
	},
}

// BuildExperienceAssetManifest reads the real embedded bytes and produces the
// version-1 manifest. It fails loud (returns an error) if any declared asset is
// missing, empty, or lacks required metadata — asset integrity is never assumed.
func BuildExperienceAssetManifest() (*ExperienceAssetManifest, error) {
	assets := make([]ExperienceAsset, 0, len(lockedAssetSpecs))
	for _, spec := range lockedAssetSpecs {
		data, err := fs.ReadFile(pwa.StaticFiles, spec.embedPath)
		if err != nil {
			return nil, fmt.Errorf("experience asset manifest: declared asset %q not present in web/pwa embed: %w", spec.embedPath, err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("experience asset manifest: declared asset %q is empty", spec.embedPath)
		}
		if spec.mediaType == "" || spec.cspClass == "" {
			return nil, fmt.Errorf("experience asset manifest: declared asset %q is missing media type or CSP class", spec.embedPath)
		}
		sum := sha256.Sum256(data)
		assets = append(assets, ExperienceAsset{
			ServedPath: "/pwa/" + spec.embedPath,
			EmbedPath:  spec.embedPath,
			Source:     SourceFirstParty,
			License:    firstPartyLicense,
			SHA256:     hex.EncodeToString(sum[:]),
			Size:       int64(len(data)),
			MediaType:  spec.mediaType,
			CSPClass:   spec.cspClass,
			SWPolicy:   SWPrecacheImmutable,
		})
	}

	m := &ExperienceAssetManifest{
		Version:              1,
		Assets:               assets,
		ExternalDependencies: externalDependencies,
	}
	m.CacheIdentity = computeCacheIdentity(assets)
	return m, nil
}

// computeCacheIdentity is a deterministic aggregate digest over the sorted
// (served path + byte digest) of every locked asset.
func computeCacheIdentity(assets []ExperienceAsset) string {
	lines := make([]string, 0, len(assets))
	for _, a := range assets {
		lines = append(lines, a.ServedPath+"@"+a.SHA256)
	}
	sort.Strings(lines)
	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h[:])
}

// networkOnlyPrefixes are the request classes the service worker must NEVER
// precache (design.md "HTMX And CSP"): authenticated APIs and versioned APIs.
// Non-GET and authenticated HTML are excluded by request method/credentials at
// the fetch handler, not by static-path prefix.
var networkOnlyPrefixes = []string{"/api/", "/v1/"}

// IsNetworkOnlyPath reports whether a same-origin GET path must bypass the
// precache and always hit the network.
func IsNetworkOnlyPath(path string) bool {
	for _, p := range networkOnlyPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
