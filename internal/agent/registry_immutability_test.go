package agent

import (
	"encoding/json"
	"testing"
)

// Adversarial regression for BS-005 (schema integrity). Once a tool is
// registered, mutating the original schema bytes MUST NOT change validation
// behavior. The registry compiles the schema and defensively copies the
// bytes; the compiled schema is what validates.
func TestRegisterTool_SchemaIsImmutableAfterRegistration(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()

	// Strict input schema: requires "id" (string).
	original := []byte(`{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`)
	tool := validTool("immut")
	tool.InputSchema = json.RawMessage(append([]byte(nil), original...))

	RegisterTool(tool)

	// Sanity: the strict schema rejects {} (missing required "id").
	in, _, ok := SchemasFor("immut")
	if !ok {
		t.Fatal("tool not registered")
	}
	if err := in.ValidateBytes(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected strict schema to reject {} before mutation")
	}

	// Now mutate the caller's original byte slice to a permissive schema.
	// This MUST NOT change the registered schema's validation behavior.
	permissive := []byte(`{"type":"object"}                                            `)
	for i := range tool.InputSchema {
		if i < len(permissive) {
			tool.InputSchema[i] = permissive[i]
		}
	}

	// Re-validate — the registered schema must still reject {}.
	in2, _, ok := SchemasFor("immut")
	if !ok {
		t.Fatal("tool disappeared from registry after mutation")
	}
	if err := in2.ValidateBytes(json.RawMessage(`{}`)); err == nil {
		t.Fatal("BS-005 regression: registered schema accepted {} after caller mutated source bytes; schema must be compiled, not re-read")
	}

	// And the registered Tool's InputSchema bytes are the defensive copy,
	// not the caller's slice — mutating one must not affect the other.
	got, _ := ByName("immut")
	if string(got.InputSchema[:len(original)]) != string(original) {
		t.Errorf("registered InputSchema bytes were aliased to caller buffer; want defensive copy")
	}
}
