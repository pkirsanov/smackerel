package photos

import (
	"strings"
	"time"
)

// EditorSignature names the photo editor that produced a derived export.
// Mapping is stable-signal only (FR-004A): software/processing-version
// strings come straight from EXIF and are not LLM-owned. Any LLM-based
// lifecycle decision must use the LifecycleAnalyzer pathway instead.
type EditorSignature string

const (
	EditorUnknown        EditorSignature = "unknown"
	EditorLightroomCC    EditorSignature = "lightroom_cc"
	EditorLightroomClass EditorSignature = "lightroom_classic"
	EditorDarktable      EditorSignature = "darktable"
	EditorGIMP           EditorSignature = "gimp"
	EditorRawTherapee    EditorSignature = "rawtherapee"
	EditorDaVinciResolve EditorSignature = "davinci_resolve"
)

// SupportedEditorSignatures lists every editor that the lifecycle pipeline
// can recognize as a stable-signal hint. The order is deterministic so
// downstream code can iterate without re-sorting.
func SupportedEditorSignatures() []EditorSignature {
	return []EditorSignature{
		EditorLightroomCC,
		EditorLightroomClass,
		EditorDarktable,
		EditorGIMP,
		EditorRawTherapee,
		EditorDaVinciResolve,
	}
}

// EditorSignatureFromEXIF extracts an editor signature from EXIF Software
// fields and related tags. The mapping is intentionally narrow: only the
// editors enumerated by SupportedEditorSignatures match. Anything else
// returns EditorUnknown so callers can route the photo to the LLM
// lifecycle analyzer instead of inventing a heuristic.
func EditorSignatureFromEXIF(exif map[string]any) EditorSignature {
	if len(exif) == 0 {
		return EditorUnknown
	}
	for _, key := range []string{"Software", "software", "ProcessingSoftware", "processing_software", "CreatorTool", "creator_tool"} {
		raw, ok := exif[key]
		if !ok {
			continue
		}
		text, ok := raw.(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		if signature := classifyEditorString(text); signature != EditorUnknown {
			return signature
		}
	}
	return EditorUnknown
}

func classifyEditorString(software string) EditorSignature {
	lower := strings.ToLower(strings.TrimSpace(software))
	switch {
	case strings.Contains(lower, "lightroom classic"), strings.Contains(lower, "lightroom 6"), strings.Contains(lower, "lightroom 5"):
		return EditorLightroomClass
	case strings.Contains(lower, "lightroom"), strings.Contains(lower, "adobe lightroom"):
		return EditorLightroomCC
	case strings.Contains(lower, "darktable"):
		return EditorDarktable
	case strings.Contains(lower, "gimp"):
		return EditorGIMP
	case strings.Contains(lower, "rawtherapee"):
		return EditorRawTherapee
	case strings.Contains(lower, "davinci resolve"), strings.Contains(lower, "davinci"):
		return EditorDaVinciResolve
	}
	return EditorUnknown
}

// EditorVersionFromEXIF returns the literal Software / CreatorTool string
// so audit rows preserve the exact provider-supplied version. The Go core
// never invents version numbers — empty input returns an empty string.
func EditorVersionFromEXIF(exif map[string]any) string {
	if len(exif) == 0 {
		return ""
	}
	for _, key := range []string{"Software", "software", "ProcessingSoftware", "processing_software", "CreatorTool", "creator_tool"} {
		if value, ok := exif[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// LifecycleSeed groups the stable signals that justify pairing a RAW with
// a derived export. The LifecycleAnalyzer combines these with the LLM
// rationale to decide whether a link is automatic or review-required.
type LifecycleSeed struct {
	RawPhotoID         string
	DerivedPhotoID     string
	Editor             EditorSignature
	EditorVersion      string
	Method             string
	CapturedAtSkewSecs int
	SeenAt             time.Time
}
