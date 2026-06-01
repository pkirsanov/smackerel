package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestAssistantTurnV1GoldenContract (TP-073-25, SCN-073-A02) pins
// the canonical wire-schema artifact, the Go contract types in
// types.go, and the canonical request/response fixtures together.
// Any change to the schema, the Go types, or the fixtures that
// breaks lockstep fails this test before downstream web or shared
// mobile codegen runs.
func TestAssistantTurnV1GoldenContract(t *testing.T) {
	doc := loadSchemaDoc(t)

	if got := doc["schema_version"]; got != SchemaVersionV1 {
		t.Fatalf("schema artifact schema_version drift: got %v want %s", got, SchemaVersionV1)
	}

	assertStringList(t, doc["transport_hint_allowlist"], AllowedTransportHints, "transport_hint_allowlist")
	assertStringList(t, doc["kind_allowlist"], AllowedKinds, "kind_allowlist")

	defs, ok := doc["definitions"].(map[string]any)
	if !ok {
		t.Fatalf("schema artifact has no definitions map")
	}

	t.Run("TurnRequest_pins_Go_type", func(t *testing.T) {
		assertGoTypeMatchesSchemaDef(t, defs, "TurnRequest", reflect.TypeOf(TurnRequest{}))
	})
	t.Run("TurnResponse_pins_Go_type", func(t *testing.T) {
		assertGoTypeMatchesSchemaDef(t, defs, "TurnResponse", reflect.TypeOf(TurnResponse{}))
	})
	t.Run("NoticePayload_pins_Go_type", func(t *testing.T) {
		assertGoTypeMatchesSchemaDef(t, defs, "NoticePayload", reflect.TypeOf(NoticePayload{}))
	})
	t.Run("Source_pins_Go_type", func(t *testing.T) {
		assertGoTypeMatchesSchemaDef(t, defs, "Source", reflect.TypeOf(Source{}))
	})
	t.Run("ConfirmCard_pins_Go_type", func(t *testing.T) {
		assertGoTypeMatchesSchemaDef(t, defs, "ConfirmCard", reflect.TypeOf(ConfirmCard{}))
	})
	t.Run("Disambiguation_pins_Go_type", func(t *testing.T) {
		assertGoTypeMatchesSchemaDef(t, defs, "Disambiguation", reflect.TypeOf(Disambiguation{}))
	})
	t.Run("DisambiguationChoice_pins_Go_type", func(t *testing.T) {
		assertGoTypeMatchesSchemaDef(t, defs, "DisambiguationChoice", reflect.TypeOf(DisambiguationChoice{}))
	})
	t.Run("Trace_pins_Go_type", func(t *testing.T) {
		assertGoTypeMatchesSchemaDef(t, defs, "Trace", reflect.TypeOf(Trace{}))
	})

	t.Run("request_v1_fixture_round_trip", func(t *testing.T) {
		raw := mustReadGolden(t, "request_v1.json")
		assertFixtureKeysMatchDef(t, raw, defs, "TurnRequest", "request_v1.json")

		var req TurnRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatalf("unmarshal request fixture: %v", err)
		}
		if req.SchemaVersion != SchemaVersionV1 {
			t.Fatalf("request fixture schema_version drift: got %q want %q", req.SchemaVersion, SchemaVersionV1)
		}
		out, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("re-marshal request: %v", err)
		}
		assertJSONEqual(t, raw, out, "request_v1.json")
	})

	t.Run("response_v1_fixture_round_trip", func(t *testing.T) {
		raw := mustReadGolden(t, "response_v1.json")
		assertFixtureKeysMatchDef(t, raw, defs, "TurnResponse", "response_v1.json")

		var resp TurnResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal response fixture: %v", err)
		}
		if resp.SchemaVersion != SchemaVersionV1 {
			t.Fatalf("response fixture schema_version drift: got %q want %q", resp.SchemaVersion, SchemaVersionV1)
		}
		if resp.Notice != nil {
			t.Fatalf("response_v1.json notice-absent fixture must decode with Notice==nil; got %+v", resp.Notice)
		}
		out, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("re-marshal response: %v", err)
		}
		assertJSONEqual(t, raw, out, "response_v1.json")
	})

	// TP-075-25: notice-present round trip. Same schema_version="v1";
	// optional `notice` sub-object is additive and v1-compatible.
	t.Run("response_v1_notice_fixture_round_trip", func(t *testing.T) {
		raw := mustReadGolden(t, "response_v1_notice.json")
		assertFixtureKeysMatchDef(t, raw, defs, "TurnResponse", "response_v1_notice.json")

		var resp TurnResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal notice fixture: %v", err)
		}
		if resp.SchemaVersion != SchemaVersionV1 {
			t.Fatalf("notice fixture schema_version drift: got %q want %q", resp.SchemaVersion, SchemaVersionV1)
		}
		if resp.Notice == nil {
			t.Fatalf("notice-present fixture must decode with non-nil Notice")
		}
		if resp.Notice.Command == "" || resp.Notice.ReplacementExample == "" ||
			resp.Notice.CopyKey == "" || resp.Notice.WindowID == "" {
			t.Fatalf("notice fixture must populate all NoticePayload fields; got %+v", *resp.Notice)
		}
		out, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("re-marshal notice fixture: %v", err)
		}
		assertJSONEqual(t, raw, out, "response_v1_notice.json")
	})
}

// TestAssistantTurnV1GoldenContract_AdversarialDrift proves the
// golden contract test would fail on real drift. It mutates an
// in-memory copy of each artifact and asserts the check fires.
func TestAssistantTurnV1GoldenContract_AdversarialDrift(t *testing.T) {
	defs := map[string]any{
		"TurnRequest": map[string]any{
			"required":   []any{"schema_version"},
			"properties": map[string]any{"schema_version": map[string]any{}},
		},
	}
	// Reflect-derived type has more fields than the trimmed schema.
	// assertGoTypeMatchesSchemaDef MUST detect this.
	stub := &recordingT{T: t}
	assertGoTypeMatchesSchemaDef(stub, defs, "TurnRequest", reflect.TypeOf(TurnRequest{}))
	if !stub.failed {
		t.Fatalf("adversarial drift (schema missing fields) was not detected")
	}

	// Fixture with an extra unknown key MUST be rejected by the
	// fixture-vs-schema key-set check.
	stub2 := &recordingT{T: t}
	bogus := []byte(`{"schema_version":"v1","unknown_field":true}`)
	assertFixtureKeysMatchDef(stub2, bogus, map[string]any{
		"TurnRequest": map[string]any{
			"required":   []any{"schema_version"},
			"properties": map[string]any{"schema_version": map[string]any{}},
		},
	}, "TurnRequest", "bogus.json")
	if !stub2.failed {
		t.Fatalf("adversarial drift (fixture extra key) was not detected")
	}
}

// --- helpers -----------------------------------------------------

func loadSchemaDoc(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("assistant_turn_v1.json")
	if err != nil {
		t.Fatalf("read schema artifact: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse schema artifact: %v", err)
	}
	return doc
}

func mustReadGolden(t testing.TB, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return raw
}

func jsonTagsOf(rt reflect.Type) []string {
	out := make([]string, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.SplitN(tag, ",", 2)[0]
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// optionalJSONTagsOf returns the json tag names whose tag includes
// the `omitempty` modifier. Used to enforce that schema properties
// that are not in the `required` list have a Go field with
// `,omitempty`.
func optionalJSONTagsOf(rt reflect.Type) map[string]bool {
	out := make(map[string]bool, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		parts := strings.Split(tag, ",")
		name := parts[0]
		for _, p := range parts[1:] {
			if p == "omitempty" {
				out[name] = true
			}
		}
	}
	return out
}

func defFieldNames(t fataler, defs map[string]any, name string) []string {
	t.Helper()
	def, ok := defs[name].(map[string]any)
	if !ok {
		t.Fatalf("schema definition %q missing", name)
	}
	props, ok := def["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema definition %q has no properties", name)
	}
	out := make([]string, 0, len(props))
	for k := range props {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func defRequiredNames(t fataler, defs map[string]any, name string) []string {
	t.Helper()
	def, ok := defs[name].(map[string]any)
	if !ok {
		t.Fatalf("schema definition %q missing", name)
	}
	req, ok := def["required"].([]any)
	if !ok {
		t.Fatalf("schema definition %q missing required[]", name)
	}
	out := make([]string, 0, len(req))
	for _, r := range req {
		s, ok := r.(string)
		if !ok {
			t.Fatalf("schema definition %q required[] contains non-string", name)
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func assertGoTypeMatchesSchemaDef(t fataler, defs map[string]any, defName string, rt reflect.Type) {
	t.Helper()
	want := defFieldNames(t, defs, defName)
	required := defRequiredNames(t, defs, defName)
	requiredSet := make(map[string]bool, len(required))
	for _, r := range required {
		requiredSet[r] = true
	}
	// Required MUST be a subset of properties.
	for _, r := range required {
		found := false
		for _, p := range want {
			if p == r {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("schema definition %q: required field %q has no property entry", defName, r)
		}
	}
	got := jsonTagsOf(rt)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Go type %s and schema definition %q drift\n go type tags: %v\n schema props: %v", rt.Name(), defName, got, want)
	}
	// Every schema property that is NOT in `required` MUST be tagged
	// with `,omitempty` on the Go struct so the wire field is
	// omitted when absent (v1-compatible additive optional).
	optional := optionalJSONTagsOf(rt)
	for _, p := range want {
		if requiredSet[p] {
			continue
		}
		if !optional[p] {
			t.Fatalf("schema definition %q optional property %q has no `,omitempty` on Go type %s", defName, p, rt.Name())
		}
	}
}

func assertFixtureKeysMatchDef(t fataler, raw []byte, defs map[string]any, defName, fixtureName string) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode fixture %s: %v", fixtureName, err)
	}
	gotSet := make(map[string]bool, len(m))
	for k := range m {
		gotSet[k] = true
	}
	props := defFieldNames(t, defs, defName)
	propSet := make(map[string]bool, len(props))
	for _, p := range props {
		propSet[p] = true
	}
	required := defRequiredNames(t, defs, defName)
	// Fixture keys MUST be a subset of properties (no unknown
	// wire fields under additionalProperties:false).
	for k := range gotSet {
		if !propSet[k] {
			t.Fatalf("fixture %s contains unknown key %q not in schema %q properties %v", fixtureName, k, defName, props)
		}
	}
	// Fixture MUST include every required field.
	for _, r := range required {
		if !gotSet[r] {
			t.Fatalf("fixture %s missing required schema %q field %q", fixtureName, defName, r)
		}
	}
}

func assertJSONEqual(t fataler, a, b []byte, label string) {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		t.Fatalf("decode %s (a): %v", label, err)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		t.Fatalf("decode %s (b): %v", label, err)
	}
	if !reflect.DeepEqual(av, bv) {
		t.Fatalf("%s round-trip drift\n got:  %v\n want: %v", label, bv, av)
	}
}

func assertStringList(t *testing.T, raw any, want []string, label string) {
	t.Helper()
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("schema %s is not an array: %T", label, raw)
	}
	got := make([]string, 0, len(arr))
	for _, v := range arr {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("schema %s contains non-string element", label)
		}
		got = append(got, s)
	}
	sortedGot := append([]string(nil), got...)
	sortedWant := append([]string(nil), want...)
	sort.Strings(sortedGot)
	sort.Strings(sortedWant)
	if !reflect.DeepEqual(sortedGot, sortedWant) {
		t.Fatalf("schema %s drift\n got:  %v\n want: %v", label, got, want)
	}
}

// fataler is the minimal slice of *testing.T behaviour the helpers
// use, so the adversarial-drift test can swap in a recorder.
type fataler interface {
	Helper()
	Fatalf(format string, args ...any)
}

type recordingT struct {
	*testing.T
	failed bool
}

func (r *recordingT) Fatalf(format string, args ...any) {
	r.Helper()
	r.failed = true
	// Do NOT call the embedded T.Fatalf — we want the outer test to
	// continue and assert that the helper fired.
}
