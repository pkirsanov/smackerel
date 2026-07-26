// evidence.go implements the value-safe evidence sanitizer. It converts a raw
// observation into a content-free EvidenceEntry that carries only a closed
// evidence class, the safe field-name identifiers, and a cryptographic digest —
// never a credential, personal content, or target detail. An unsafe field is
// not redacted-and-accepted; the whole observation is rejected with
// E102-JOURNEY-CONTRACT-EVIDENCE-UNSAFE and the offending raw value is never
// echoed. The forbidden-token and target-literal scanners are reused from
// read_only_guard.go so evidence safety and the read-only static guard share one
// source of truth.

package acceptance

import "sort"

// EvidenceClass is a closed evidence entry class.
type EvidenceClass string

const (
	EvidenceHTTP          EvidenceClass = "http-observation"
	EvidenceDOM           EvidenceClass = "dom-observation"
	EvidenceAccessibility EvidenceClass = "accessibility-observation"
	EvidenceTelemetry     EvidenceClass = "telemetry-observation"
	EvidenceFreshness     EvidenceClass = "freshness-observation"
	EvidenceSafety        EvidenceClass = "safety-observation"
)

var closedEvidenceClasses = map[EvidenceClass]bool{
	EvidenceHTTP:          true,
	EvidenceDOM:           true,
	EvidenceAccessibility: true,
	EvidenceTelemetry:     true,
	EvidenceFreshness:     true,
	EvidenceSafety:        true,
}

// RawObservation is an untrusted observation captured by the runner. Fields maps
// a value-safe field-name identifier to its raw observed value; the sanitizer
// scans both and preserves neither raw value.
type RawObservation struct {
	Class  EvidenceClass
	Fields map[string]string
	Digest string
}

// EvidenceEntry is the sanitized, content-free evidence record that may be
// persisted. It carries only the class, the sorted safe field names, and the
// digest — never a raw value.
type EvidenceEntry struct {
	Class          EvidenceClass
	SafeFieldNames []string
	Digest         string
}

// EvidenceSanitizer converts raw observations into content-free evidence
// entries, rejecting any unsafe field.
type EvidenceSanitizer struct{}

// Sanitize validates the observation and returns a content-free EvidenceEntry,
// or a *ContractError. It never echoes a raw field value.
func (EvidenceSanitizer) Sanitize(obs RawObservation) (EvidenceEntry, error) {
	if !closedEvidenceClasses[obs.Class] {
		return EvidenceEntry{}, contractErr(CodeUnknownEnum, "observation declares unknown evidence class %q", obs.Class)
	}
	if obs.Digest == "" {
		return EvidenceEntry{}, contractErr(CodeMalformed, "observation carries no content digest")
	}
	names := make([]string, 0, len(obs.Fields))
	for name, value := range obs.Fields {
		// The field NAME must not name a credential, personal-content, or target
		// field (deny-by-schema).
		if verr := scanEvidenceFields([]string{name}); verr != nil {
			return EvidenceEntry{}, contractErr(verr.Code, "evidence field name is unsafe: %s", verr.Reason)
		}
		// The field VALUE must not carry a concrete target literal (host, URL,
		// tailnet identity, operator path, IPv4, or private key). The raw value is
		// never included in the error.
		if code, why, bad := targetLiteral(value); bad {
			return EvidenceEntry{}, contractErr(code, "evidence field %q value carries a concrete target literal (%s)", name, why)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return EvidenceEntry{Class: obs.Class, SafeFieldNames: names, Digest: obs.Digest}, nil
}
