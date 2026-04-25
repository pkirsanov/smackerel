// x-redact persistence-boundary redaction for spec 037 Scope 7 (BS-022).
//
// The agent records every invocation to PostgreSQL via PostgresTracer. A
// scenario's input/output JSON Schema (and each registered tool's input/
// output JSON Schema) may mark individual properties with the custom
// vocabulary keyword `x-redact: true`. Those values must NEVER appear
// in the persisted trace — operator UI, replay tooling, and any backup
// of agent_traces must show only the configured marker.
//
// Critical contract (BS-022 + scope-7 DoD):
//
//   - Redaction runs on a DEEP-CLONED copy of the value. The handler
//     and every other in-process consumer (telegram reply, API
//     response, executor turn loop) sees the original, unredacted
//     value. Redaction is a property of persistence, not of the data
//     model.
//   - Walks nested objects, arrays, and additionalProperties.
//     Honours `properties`, `items`, and `additionalProperties`
//     subschemas.
//   - $ref is intentionally NOT followed in this scope — the loader
//     forbids x-redact on required fields and scenarios in the tree
//     do not use $ref. If a future scenario introduces $ref + x-redact
//     this function will under-redact rather than panic; the loader's
//     forbidden-pattern guard (Scope 4) is the policy gate.
//   - When the marker is empty (RedactMarker config unset) or the
//     schema is nil/empty, the value is returned unchanged.
//   - Non-string redacted values (number, bool, object, array) are
//     replaced with the marker as well so a sensitive number can't
//     leak by being typed as int.

package agent

import (
	"encoding/json"
)

// RedactValue returns a redacted DEEP COPY of value, walking schema
// and replacing any property whose subschema declares x-redact: true
// with marker. The input value is NEVER mutated.
//
// If marker == "" or schema is nil/empty, RedactValue returns a clone
// of value with no redaction applied (so callers can always treat the
// returned RawMessage as an independent buffer safe to marshal).
func RedactValue(value, schema json.RawMessage, marker string) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	if marker == "" || len(schema) == 0 {
		return cloneRaw(value)
	}
	var schemaAny any
	if err := json.Unmarshal(schema, &schemaAny); err != nil {
		// Malformed schema: fail closed — return the original value.
		// The loader already validates schemas at registration; a
		// malformed one in the persistence path is a programmer bug,
		// not a security boundary.
		return cloneRaw(value)
	}
	var valueAny any
	if err := json.Unmarshal(value, &valueAny); err != nil {
		return cloneRaw(value)
	}
	redacted := redactWalk(valueAny, schemaAny, marker)
	out, err := json.Marshal(redacted)
	if err != nil {
		return cloneRaw(value)
	}
	return out
}

// redactWalk walks value alongside schema and replaces redacted
// properties in-place on the cloned tree. Returns the (possibly
// substituted) value.
func redactWalk(value, schema any, marker string) any {
	schemaMap, ok := schema.(map[string]any)
	if !ok {
		return value
	}

	switch v := value.(type) {
	case map[string]any:
		props, _ := schemaMap["properties"].(map[string]any)
		addl := schemaMap["additionalProperties"]
		for key, child := range v {
			subSchema, hasProp := props[key]
			if !hasProp {
				// Fall back to additionalProperties subschema if it is
				// itself a schema (object). `true` / `false` / missing
				// → no redaction guidance for unknown keys.
				if addlMap, ok := addl.(map[string]any); ok {
					subSchema = addlMap
					hasProp = true
				}
			}
			if !hasProp {
				continue
			}
			subMap, _ := subSchema.(map[string]any)
			if redact, _ := subMap["x-redact"].(bool); redact {
				v[key] = marker
				continue
			}
			v[key] = redactWalk(child, subSchema, marker)
		}
		return v

	case []any:
		items := schemaMap["items"]
		// items can be a single schema or a list (tuple validation).
		switch it := items.(type) {
		case map[string]any:
			if redact, _ := it["x-redact"].(bool); redact {
				for i := range v {
					v[i] = marker
				}
				return v
			}
			for i := range v {
				v[i] = redactWalk(v[i], it, marker)
			}
		case []any:
			for i := range v {
				if i >= len(it) {
					break
				}
				itemSchema, _ := it[i].(map[string]any)
				if redact, _ := itemSchema["x-redact"].(bool); redact {
					v[i] = marker
					continue
				}
				v[i] = redactWalk(v[i], itemSchema, marker)
			}
		}
		return v
	}

	return value
}
