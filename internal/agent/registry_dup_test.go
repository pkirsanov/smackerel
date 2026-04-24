package agent

import (
	"strings"
	"testing"
)

// Adversarial regression for §3.1: a duplicate tool name MUST cause the
// process to refuse to start, and the panic message MUST name BOTH
// registration call sites so the operator can fix the duplicate without
// guessing which package owns the conflict.
func TestRegisterTool_DuplicateNamePanicsWithBothCallSites(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()

	RegisterTool(validTool("dup_tool"))

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected duplicate registration to panic")
		}
		msg := toString(r)
		if !strings.Contains(msg, `duplicate tool name "dup_tool"`) {
			t.Errorf("panic missing duplicate name marker: %q", msg)
		}
		// The panic message must reference THIS test file twice — once for
		// the second registration ("registered at <file>") and once for the
		// first ("first registered at <file>"). Two separate :line markers
		// are enough to prove both sites are recorded.
		if strings.Count(msg, "registry_dup_test.go") < 2 {
			t.Errorf("panic message must reference both call sites; got %q", msg)
		}
	}()

	RegisterTool(validTool("dup_tool"))
}
