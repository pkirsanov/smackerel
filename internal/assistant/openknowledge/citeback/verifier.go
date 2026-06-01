package citeback

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

// Rejection sentinels. Errors are compared with errors.Is.
var (
	ReasonNotInTrace        = errors.New("citeback: citation has no matching entry in the recorded tool trace")
	ReasonHashMismatch      = errors.New("citeback: citation matches a recorded source by locator but the content hash differs")
	ReasonMalformedCitation = errors.New("citeback: citation is missing a required field for its Kind")
	ReasonKindMismatch      = errors.New("citeback: citation Kind does not match the recorded Source Kind for the same locator")
)

// ToolInvocation is one entry in the per-turn ToolTrace. RecordedSources
// is the Sources slice the Tool returned in its ToolResult envelope.
type ToolInvocation struct {
	ToolName        string
	RecordedSources []ok.Source
}

// ToolTrace is the ordered list of tool invocations made during a
// single agent turn.
type ToolTrace []ToolInvocation

// Citation is the per-Kind locator a final answer attaches to a claim.
// Fields outside the Kind's required set are ignored; see
// package-level rules.
type Citation struct {
	Kind ok.SourceKind

	// SourceArtifact: required.
	ArtifactID string

	// SourceWeb: required.
	URL         string
	ContentHash string

	// SourceToolComputation: required.
	Tool   string
	Input  json.RawMessage
	Output json.RawMessage
}

// RejectedCitation is a Citation that failed verification, paired
// with the typed sentinel reason.
type RejectedCitation struct {
	Citation Citation
	Reason   error
}

// VerifyResult is the verdict the caller acts on. OK is true iff
// Rejected is empty. Verified deduplicates by per-Kind locator so the
// same recorded Source counted multiple times by an over-claiming
// answer appears once.
type VerifyResult struct {
	OK       bool
	Verified []ok.Source
	Rejected []RejectedCitation
}

// Verify is the pure verification entry point. No I/O, no goroutines.
func Verify(answerCitations []Citation, trace ToolTrace) VerifyResult {
	res := VerifyResult{OK: true}

	seen := make(map[string]struct{}, len(answerCitations))

	for _, c := range answerCitations {
		match, reason := lookup(c, trace)
		if reason != nil {
			res.OK = false
			res.Rejected = append(res.Rejected, RejectedCitation{Citation: c, Reason: reason})
			continue
		}
		key := dedupKey(c)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		res.Verified = append(res.Verified, match)
	}

	return res
}

// lookup walks the trace once and returns either the matching Source
// or a typed rejection reason.
func lookup(c Citation, trace ToolTrace) (ok.Source, error) {
	if err := validateCitation(c); err != nil {
		return ok.Source{}, err
	}

	var (
		kindMatchSeen bool
		hashMismatch  bool
	)

	for _, inv := range trace {
		for _, src := range inv.RecordedSources {
			same, locatorMatch, kindMatch := compareCitation(c, src)
			if !locatorMatch {
				continue
			}
			if !kindMatch {
				kindMatchSeen = true
				continue
			}
			if same {
				return src, nil
			}
			hashMismatch = true
		}
	}

	switch {
	case hashMismatch:
		return ok.Source{}, ReasonHashMismatch
	case kindMatchSeen:
		return ok.Source{}, ReasonKindMismatch
	default:
		return ok.Source{}, ReasonNotInTrace
	}
}

// validateCitation enforces the per-Kind required-field contract.
func validateCitation(c Citation) error {
	switch c.Kind {
	case ok.SourceArtifact:
		if strings.TrimSpace(c.ArtifactID) == "" {
			return ReasonMalformedCitation
		}
	case ok.SourceWeb:
		if strings.TrimSpace(c.URL) == "" || strings.TrimSpace(c.ContentHash) == "" {
			return ReasonMalformedCitation
		}
	case ok.SourceToolComputation:
		if strings.TrimSpace(c.Tool) == "" || len(c.Input) == 0 || len(c.Output) == 0 {
			return ReasonMalformedCitation
		}
		// Re-encoding catches non-JSON payloads early.
		if _, err := canonicalJSON(c.Input); err != nil {
			return ReasonMalformedCitation
		}
		if _, err := canonicalJSON(c.Output); err != nil {
			return ReasonMalformedCitation
		}
	default:
		return ReasonMalformedCitation
	}
	return nil
}

// compareCitation reports three booleans:
//
//	same         — full identity (locator + content) match
//	locatorMatch — Kind-specific locator (artifact ID / URL / tool name) matches
//	kindMatch    — the recorded Source.Kind matches the citation's Kind
//
// kindMatch is reported only when locatorMatch is true; callers use
// the (locatorMatch && !kindMatch) signal to raise ReasonKindMismatch.
func compareCitation(c Citation, src ok.Source) (same, locatorMatch, kindMatch bool) {
	switch c.Kind {
	case ok.SourceArtifact:
		// Locator: ArtifactID equality (post-trim). We look at both
		// the recorded Kind and, when the recorded entry is in fact a
		// web source carrying the same string somewhere, we still want
		// "no locator match" — only true Artifact entries are
		// candidates for SourceArtifact locator matching.
		if src.Kind == ok.SourceArtifact && src.Artifact != nil {
			if normaliseID(src.Artifact.ID) == normaliseID(c.ArtifactID) {
				return true, true, true
			}
		}
		// Cross-kind locator collision: a non-artifact recorded source
		// whose own ID-shaped field matches. Treat ID strings as
		// artifact-domain only; do not raise kind mismatch from string
		// punning across kinds.
		return false, false, false

	case ok.SourceWeb:
		if src.Web == nil {
			return false, false, false
		}
		recordedURL := normalizeURL(src.Web.URL)
		citedURL := normalizeURL(c.URL)
		if recordedURL == "" || citedURL == "" || recordedURL != citedURL {
			return false, false, false
		}
		if src.Kind != ok.SourceWeb {
			return false, true, false
		}
		if src.Web.ContentHash == c.ContentHash {
			return true, true, true
		}
		return false, true, true

	case ok.SourceToolComputation:
		if src.Computation == nil {
			return false, false, false
		}
		if src.Computation.Tool != c.Tool {
			return false, false, false
		}
		if src.Kind != ok.SourceToolComputation {
			return false, true, false
		}
		recordedHash, rErr := ComputationCanonicalHash(src.Computation.Tool, src.Computation.Input, src.Computation.Output)
		citedHash, cErr := ComputationCanonicalHash(c.Tool, c.Input, c.Output)
		if rErr != nil || cErr != nil {
			return false, true, true
		}
		if recordedHash == citedHash {
			return true, true, true
		}
		return false, true, true
	}
	return false, false, false
}

// dedupKey collapses repeat citations of the same logical source.
func dedupKey(c Citation) string {
	switch c.Kind {
	case ok.SourceArtifact:
		return "a:" + normaliseID(c.ArtifactID)
	case ok.SourceWeb:
		return "w:" + normalizeURL(c.URL) + "#" + c.ContentHash
	case ok.SourceToolComputation:
		h, err := ComputationCanonicalHash(c.Tool, c.Input, c.Output)
		if err != nil {
			return "c:" + c.Tool + ":<malformed>"
		}
		return "c:" + c.Tool + ":" + h
	}
	return "?"
}

func normaliseID(s string) string { return strings.TrimSpace(s) }

// normalizeURL is the documented URL equality rule. Two URLs are
// considered the same iff after the following transforms they are
// byte-equal:
//
//   - the scheme is lower-cased,
//   - the host is lower-cased and percent-decoded,
//   - the path is percent-decoded (case preserved — POSIX paths are
//     case-sensitive),
//   - a single trailing slash on the path is dropped unless the path
//     is exactly "/" (root preserved as "/"),
//   - the query and fragment are preserved verbatim.
//
// Unparseable URLs fall back to a trimmed string comparison so they
// can never silently pass equality with parseable inputs.
func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Host != "" {
		if dec, derr := url.PathUnescape(u.Host); derr == nil {
			u.Host = strings.ToLower(dec)
		} else {
			u.Host = strings.ToLower(u.Host)
		}
	}
	if u.Path != "" {
		if dec, derr := url.PathUnescape(u.Path); derr == nil {
			u.Path = dec
		}
		if len(u.Path) > 1 && strings.HasSuffix(u.Path, "/") {
			u.Path = strings.TrimRight(u.Path, "/")
		}
	}
	return u.String()
}

// ComputationCanonicalHash returns the canonical content hash for a
// SourceToolComputation entry. Form:
//
//	sha256_hex( Tool + "\n" + canonicalJSON(Input) + "\n" + canonicalJSON(Output) )
//
// canonicalJSON re-encodes JSON with object keys sorted recursively so
// {"a":1,"b":2} and {"b":2,"a":1} produce the same digest.
func ComputationCanonicalHash(tool string, input, output json.RawMessage) (string, error) {
	inCanon, err := canonicalJSON(input)
	if err != nil {
		return "", err
	}
	outCanon, err := canonicalJSON(output)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(tool))
	h.Write([]byte{'\n'})
	h.Write(inCanon)
	h.Write([]byte{'\n'})
	h.Write(outCanon)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// canonicalJSON re-emits the given JSON value with sorted object keys.
// Arrays preserve order. Whitespace is stripped.
func canonicalJSON(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("citeback: empty JSON payload")
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeCanonical(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, el := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, el); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}
