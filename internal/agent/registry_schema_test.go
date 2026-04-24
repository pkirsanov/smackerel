package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRegisterTool_MalformedInputSchemaPanics(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	tool := validTool("bad_in")
	tool.InputSchema = json.RawMessage(`{not json`)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for malformed input schema")
		}
		msg := toString(r)
		if !strings.Contains(msg, `tool "bad_in"`) ||
			!strings.Contains(msg, "input_schema failed to compile") {
			t.Errorf("panic must name tool + input_schema compile failure; got %q", msg)
		}
	}()
	RegisterTool(tool)
}

func TestRegisterTool_MalformedOutputSchemaPanics(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	tool := validTool("bad_out")
	tool.OutputSchema = json.RawMessage(`{"type": 42}`) // type must be string|array

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for malformed output schema")
		}
		msg := toString(r)
		if !strings.Contains(msg, `tool "bad_out"`) ||
			!strings.Contains(msg, "output_schema failed to compile") {
			t.Errorf("panic must name tool + output_schema compile failure; got %q", msg)
		}
	}()
	RegisterTool(tool)
}

func TestRegisterTool_EmptySchemaPanics(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	tool := validTool("empty_in")
	tool.InputSchema = json.RawMessage(`   `)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty input schema")
		}
		msg := toString(r)
		if !strings.Contains(msg, "schema is empty") {
			t.Errorf("panic must name empty schema; got %q", msg)
		}
	}()
	RegisterTool(tool)
}
