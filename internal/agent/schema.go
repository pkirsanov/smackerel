// Schema compilation and validation helpers shared by the tool registry
// (Scope 2) and the scenario loader (Scope 3). Schemas use JSON Schema
// Draft 2020-12 via santhosh-tekuri/jsonschema/v6.
//
// Compilation happens once at registration / load time; the resulting
// CompiledSchema is immutable and safe to reuse from concurrent goroutines.
// The raw bytes are NOT re-read at validation time, so mutating the source
// buffer after registration cannot change validation behavior (BS-005).

package agent

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// CompiledSchema wraps a compiled JSON Schema. The zero value is invalid;
// always obtain one through CompileSchema.
type CompiledSchema struct {
	sch *jsonschema.Schema
}

// CompileSchema parses raw JSON Schema bytes and compiles them.
// Returns a structured error if the bytes are not JSON or the schema is
// not a valid JSON Schema Draft 2020-12 document.
func CompileSchema(raw json.RawMessage) (*CompiledSchema, error) {
	if len(bytesTrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("schema is empty")
	}
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("schema is not valid JSON: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", parsed); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return &CompiledSchema{sch: sch}, nil
}

// Validate validates an already-decoded JSON value (any) against the
// compiled schema. Returns nil when valid.
func (c *CompiledSchema) Validate(value any) error {
	if c == nil || c.sch == nil {
		return fmt.Errorf("schema not compiled")
	}
	return c.sch.Validate(value)
}

// ValidateBytes decodes raw JSON bytes and validates them against the
// compiled schema. Convenience wrapper used by tests and the executor.
func (c *CompiledSchema) ValidateBytes(raw json.RawMessage) error {
	if c == nil || c.sch == nil {
		return fmt.Errorf("schema not compiled")
	}
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("instance is not valid JSON: %w", err)
	}
	return c.sch.Validate(v)
}

// bytesTrimSpace is a tiny helper to detect "empty / whitespace only" JSON.
func bytesTrimSpace(b []byte) []byte {
	s, e := 0, len(b)
	for s < e && (b[s] == ' ' || b[s] == '\t' || b[s] == '\n' || b[s] == '\r') {
		s++
	}
	for e > s && (b[e-1] == ' ' || b[e-1] == '\t' || b[e-1] == '\n' || b[e-1] == '\r') {
		e--
	}
	return b[s:e]
}
