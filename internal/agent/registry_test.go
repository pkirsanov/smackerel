package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// validTool returns a fully-populated Tool ready for RegisterTool. Tests
// override individual fields to exercise validation paths.
func validTool(name string) Tool {
	return Tool{
		Name:            name,
		Description:     "test tool",
		InputSchema:     json.RawMessage(`{"type":"object"}`),
		OutputSchema:    json.RawMessage(`{"type":"object"}`),
		SideEffectClass: SideEffectRead,
		OwningPackage:   "agent_test",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	}
}

// expectPanic invokes f and reports whether the panic message contains substr.
func expectPanic(t *testing.T, substr string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q, got none", substr)
		}
		msg, _ := r.(string)
		if msg == "" {
			msg = toString(r)
		}
		if !strings.Contains(msg, substr) {
			t.Fatalf("panic message %q does not contain %q", msg, substr)
		}
	}()
	f()
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if e, ok := v.(error); ok {
		return e.Error()
	}
	return ""
}

func TestRegisterTool_HappyPath(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()

	tool := validTool("read_artifact")
	RegisterTool(tool)

	if !Has("read_artifact") {
		t.Fatal("expected tool to be registered")
	}
	got, ok := ByName("read_artifact")
	if !ok {
		t.Fatal("ByName returned !ok for registered tool")
	}
	if got.SideEffectClass != SideEffectRead {
		t.Errorf("side-effect class round-trip: got %q", got.SideEffectClass)
	}
	all := All()
	if len(all) != 1 || all[0].Name != "read_artifact" {
		t.Errorf("All() returned %+v", all)
	}
}

func TestRegisterTool_RejectsEmptyName(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	tool := validTool("")
	expectPanic(t, "empty tool name", func() { RegisterTool(tool) })
}

func TestRegisterTool_RejectsMissingDescription(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	tool := validTool("noop")
	tool.Description = ""
	expectPanic(t, "missing description", func() { RegisterTool(tool) })
}

func TestRegisterTool_RejectsMissingHandler(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	tool := validTool("noop")
	tool.Handler = nil
	expectPanic(t, "missing handler", func() { RegisterTool(tool) })
}

func TestRegisterTool_RejectsMissingOwningPackage(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	tool := validTool("noop")
	tool.OwningPackage = ""
	expectPanic(t, "missing owning_package", func() { RegisterTool(tool) })
}

func TestRegisterTool_RejectsMissingInputSchema(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	tool := validTool("noop")
	tool.InputSchema = nil
	expectPanic(t, "missing input schema", func() { RegisterTool(tool) })
}

func TestRegisterTool_RejectsMissingOutputSchema(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	tool := validTool("noop")
	tool.OutputSchema = nil
	expectPanic(t, "missing output schema", func() { RegisterTool(tool) })
}

func TestRegisterTool_RejectsInvalidSideEffectClass(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	tool := validTool("noop")
	tool.SideEffectClass = "purge"
	expectPanic(t, "invalid side_effect_class", func() { RegisterTool(tool) })
}

func TestRegisterTool_RejectsNegativeTimeout(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	tool := validTool("noop")
	tool.PerCallTimeoutMs = -1
	expectPanic(t, "negative per_call_timeout_ms", func() { RegisterTool(tool) })
}

func TestSchemasFor_ReturnsCompiledSchemasForRegisteredTool(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	RegisterTool(validTool("noop"))
	in, out, ok := SchemasFor("noop")
	if !ok || in == nil || out == nil {
		t.Fatalf("SchemasFor: ok=%v in=%v out=%v", ok, in, out)
	}
	if err := in.ValidateBytes(json.RawMessage(`{}`)); err != nil {
		t.Errorf("input schema rejected empty object: %v", err)
	}
}

func TestSchemasFor_UnknownReturnsFalse(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	if _, _, ok := SchemasFor("missing"); ok {
		t.Fatal("expected ok=false for unknown tool")
	}
}
