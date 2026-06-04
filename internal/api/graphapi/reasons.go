package graphapi

import (
	"errors"
	"fmt"
)

// ReasonKind is the closed-set of reason taxonomy categories that
// graph endpoints render into the CrossLink.Reason field. Adding a
// new kind is a design change (design.md §2 "Reason Taxonomy") and
// requires updating RenderReason in lockstep.
type ReasonKind string

const (
	// ReasonCoOccursWithTopic renders the design.md template
	// "shares topic <topic label>" — used when the cross-link target
	// is a topic node.
	ReasonCoOccursWithTopic ReasonKind = "co_occurs_with_topic"

	// ReasonMentionedInArtifact renders "mentioned in <artifact title>"
	// — used when the cross-link target is an artifact.
	ReasonMentionedInArtifact ReasonKind = "mentioned_in_artifact"

	// ReasonAttendedEvent renders "co-occurs with <person displayName>"
	// — used when the cross-link target is a person node. The legacy
	// constant name predates the design.md taxonomy alignment;
	// retained to avoid breaking Scope 02 call sites.
	ReasonAttendedEvent ReasonKind = "attended_event"

	// ReasonNearPlace renders "same place <place label>" — used when
	// the cross-link target is a place node.
	ReasonNearPlace ReasonKind = "near_place"

	// ReasonShareTimeWindow renders "captured on <YYYY-MM-DD>" — used
	// when the edge metadata exposes a same-day capture signal.
	ReasonShareTimeWindow ReasonKind = "share_time_window"
)

// ErrReasonRenderEmpty signals that a reason renderer was invoked
// without the metadata required to produce a non-empty CrossLink
// reason. Per design.md §2 the reason MUST be server-derived and
// non-empty; resolveEdges (SCOPE-080-04) propagates this error rather
// than emit a silent empty string.
var ErrReasonRenderEmpty = errors.New("graphapi: reason renderer called with empty label")

// ErrReasonRenderUnknownKind signals that a reason renderer was
// invoked with a ReasonKind outside the closed-set taxonomy. Adding
// a new kind requires a design change.
var ErrReasonRenderUnknownKind = errors.New("graphapi: reason kind is not in the closed-set taxonomy")

// RenderReason produces the server-derived, human-readable reason
// string for a CrossLink. Templates are taken verbatim from
// design.md §2 "Reason Taxonomy". When `args` is empty (label
// missing) the renderer returns a stable template-only fallback so
// in-flight responses do not abort; callers that need fail-loud
// behaviour (e.g. SCOPE-080-04 resolveEdges) MUST use ResolveReason
// instead.
func RenderReason(kind ReasonKind, args ...string) string {
	label := ""
	if len(args) >= 1 {
		label = args[0]
	}
	switch kind {
	case ReasonCoOccursWithTopic:
		if label == "" {
			return "shares topic"
		}
		return "shares topic " + label
	case ReasonMentionedInArtifact:
		if label == "" {
			return "mentioned in"
		}
		return "mentioned in " + label
	case ReasonAttendedEvent:
		if label == "" {
			return "co-occurs with"
		}
		return "co-occurs with " + label
	case ReasonNearPlace:
		if label == "" {
			return "same place"
		}
		return "same place " + label
	case ReasonShareTimeWindow:
		if label == "" {
			return "captured on"
		}
		return "captured on " + label
	default:
		return string(kind)
	}
}

// ResolveReason is the fail-loud cross-link reason resolver consumed
// by SCOPE-080-04's resolveEdges. It enforces the design.md §2
// invariant that every cross-link MUST carry a non-empty reason
// derived from server-side edge metadata: empty label or unknown
// taxonomy kind both produce a typed error rather than a silent
// fallback string.
func ResolveReason(kind ReasonKind, label string) (string, error) {
	if label == "" {
		return "", fmt.Errorf("%w: kind=%s", ErrReasonRenderEmpty, kind)
	}
	switch kind {
	case ReasonCoOccursWithTopic,
		ReasonMentionedInArtifact,
		ReasonAttendedEvent,
		ReasonNearPlace,
		ReasonShareTimeWindow:
		return RenderReason(kind, label), nil
	default:
		return "", fmt.Errorf("%w: kind=%q", ErrReasonRenderUnknownKind, kind)
	}
}

// ReasonKindForTargetKind maps an edge's destination node kind to
// the design.md §2 reason taxonomy entry that renders it. Used by
// resolveEdges (SCOPE-080-04) and by the per-resource detail
// handlers (topics / people / places) so every cross-link rendered
// anywhere in the spec 080 surface flows through the same taxonomy.
//
// The mapping is intentionally total over the closed-set of target
// kinds (`artifact|topic|person|place`); unknown kinds are an edges
// table corruption and are surfaced as ErrReasonRenderUnknownKind so
// the resolver can fail loud rather than emit a blank reason.
func ReasonKindForTargetKind(targetKind string) (ReasonKind, error) {
	switch targetKind {
	case "topic":
		return ReasonCoOccursWithTopic, nil
	case "artifact":
		return ReasonMentionedInArtifact, nil
	case "person":
		return ReasonAttendedEvent, nil
	case "place":
		return ReasonNearPlace, nil
	default:
		return "", fmt.Errorf("%w: targetKind=%q", ErrReasonRenderUnknownKind, targetKind)
	}
}
