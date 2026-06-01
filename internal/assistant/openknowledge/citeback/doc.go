// Package citeback implements the mechanical cite-back verifier for
// the open-ended knowledge agent (spec 064, SCOPE-08).
//
// # Invariant
//
// Every Citation in the agent's final answer MUST map to a Source
// recorded in the turn's ToolTrace. A Citation that has no matching
// recorded Source is by definition fabricated. This is the hard
// product invariant from design.md (P8 Trust Through Transparency)
// and the precondition the spec 061 provenance gate consumes via the
// SCOPE-10 amendment.
//
// # Per-Kind verification rules
//
// SourceArtifact:
//
//	Citation.ArtifactID MUST equal a recorded
//	Source.Artifact.ID. No hash is required because the artifact
//	itself is the system-of-record entry.
//
// SourceWeb:
//
//	Citation.URL (after normalisation, see normalizeURL) MUST equal a
//	recorded Source.Web.URL (also normalised), AND
//	Citation.ContentHash MUST equal the recorded Source.Web.ContentHash
//	byte-for-byte. The recorded ContentHash is produced by
//	web.CanonicalContentHash and is the SHA-256 hex of
//	URL + "\n" + Title + "\n" + Snippet.
//
// SourceToolComputation:
//
//	Citation.Tool MUST equal a recorded
//	Source.Computation.Tool, AND the canonical input+output hash of
//	the citation (see ComputationCanonicalHash) MUST equal the
//	canonical hash of the recorded Source.Computation. This catches
//	the "calculator says 5 but was asked for 3+1" class of
//	fabrication.
//
// # Boundary
//
// The Verifier is a pure function: stdlib only, no I/O, no LLM. It
// returns a typed VerifyResult. It does NOT:
//
//   - construct the canonical refusal body (SCOPE-10 / spec 061),
//   - increment the fabricated_source_total metric (SCOPE-09 /
//     SCOPE-14 agent loop owns that),
//   - decide whether an empty Verified slice is acceptable (caller
//     policy).
//
// The verifier produces verdicts; callers act on them.
//
// # Hash form parity with SCOPE-06 / SCOPE-07
//
// The SourceArtifact verification deliberately uses ArtifactID as the
// match key, matching the SCOPE-06 contract where the
// Snippet.ContentHash is a derived projection of Title+Summary, but
// the authoritative graph-store identifier is the artifact ID. The
// SourceWeb hash form mirrors web.CanonicalContentHash exactly
// (SCOPE-07). The SourceToolComputation hash form is defined for the
// first time here as
// sha256_hex(Tool + "\n" + canonicalJSON(Input) + "\n" + canonicalJSON(Output))
// where canonicalJSON re-encodes JSON objects with sorted keys.
package citeback
