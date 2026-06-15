// Spec 089 SCOPE-04 — /model set/show/reset renderer tests. They drive the
// pure modelCommandReply against a real modelswitch.Allowlist + a fake
// claim-bound store, so the show/set/reset/off-allowlist behaviour is verified
// without Telegram I/O. SCN-089-A03.
package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelpref"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// fakeModelPrefStore is an in-memory modelpref.Store for the /model tests.
type fakeModelPrefStore struct {
	m map[string]modelpref.Preference
}

func (f *fakeModelPrefStore) Get(_ context.Context, userID string) (modelpref.Preference, bool, error) {
	p, ok := f.m[userID]
	return p, ok, nil
}
func (f *fakeModelPrefStore) Set(_ context.Context, userID, synthesisModel string) error {
	if f.m == nil {
		f.m = map[string]modelpref.Preference{}
	}
	f.m[userID] = modelpref.Preference{SynthesisModel: synthesisModel}
	return nil
}
func (f *fakeModelPrefStore) Clear(_ context.Context, userID string) error {
	delete(f.m, userID)
	return nil
}

func spec089ModelAllowlist(t *testing.T) *modelswitch.Allowlist {
	t.Helper()
	a, err := modelswitch.NewAllowlist(
		[]string{"deepseek-r1:32b", "deepseek-r1:7b", "gemma4:26b"},
		map[string]int{"gemma4:26b": 18432, "deepseek-r1:7b": 4864, "deepseek-r1:32b": 22528, "llama3.1:8b": 6144},
		0, // dev envelope: skip co-residence
		"gemma4:26b",
		"deepseek-r1:32b",
		[]string{"gemma4:26b", "llama3.1:8b"},
	)
	if err != nil {
		t.Fatalf("NewAllowlist: %v", err)
	}
	return a
}

// TestModelCommand_ShowListsEffectiveAllowedAndDefault_Spec089 — /model (no arg)
// lists the caller's effective model with its source tag, the full switchable
// set, and the system default; /model <id> sets (validated); an off-allowlist
// set is a no-op rejection (preference unchanged).
func TestModelCommand_ShowListsEffectiveAllowedAndDefault_Spec089(t *testing.T) {
	allow := spec089ModelAllowlist(t)
	store := &fakeModelPrefStore{}
	ctx := context.Background()

	t.Run("inherited_show_marks_system_default", func(t *testing.T) {
		show := modelCommandReply(ctx, allow, store, "user-A", "")
		for _, want := range []string{"deepseek-r1:32b (system default)", "System default: deepseek-r1:32b", "Switchable:", "deepseek-r1:7b", "gemma4:26b"} {
			if !strings.Contains(show, want) {
				t.Fatalf("inherited show MUST contain %q, got:\n%s", want, show)
			}
		}
	})

	t.Run("set_then_show_marks_your_default", func(t *testing.T) {
		set := modelCommandReply(ctx, allow, store, "user-A", "deepseek-r1:7b")
		if !strings.Contains(set, "deepseek-r1:7b") || !strings.Contains(set, "your default") {
			t.Fatalf("set MUST confirm the sticky selection, got %q", set)
		}
		show := modelCommandReply(ctx, allow, store, "user-A", "")
		if !strings.Contains(show, "deepseek-r1:7b (your default)") {
			t.Fatalf("post-set show MUST mark 'your default', got:\n%s", show)
		}
	})

	t.Run("off_allowlist_set_is_a_no_op_rejection", func(t *testing.T) {
		// user-A's sticky is deepseek-r1:7b from the previous subtest.
		reply := modelCommandReply(ctx, allow, store, "user-A", "gpt-4o")
		if !strings.Contains(reply, "not a switchable model") {
			t.Fatalf("off-allowlist set MUST render the verbatim rejection, got %q", reply)
		}
		if pref, ok, _ := store.Get(ctx, "user-A"); !ok || pref.SynthesisModel != "deepseek-r1:7b" {
			t.Fatalf("an off-allowlist set MUST be a no-op (preference unchanged); got ok=%v pref=%+v", ok, pref)
		}
	})
}

// TestModelCommand_ResetClearsStickyAndConfirms_Spec089 — /model default clears
// the sticky preference, confirms the revert, and a subsequent bare /model
// resolves the SST default.
func TestModelCommand_ResetClearsStickyAndConfirms_Spec089(t *testing.T) {
	allow := spec089ModelAllowlist(t)
	store := &fakeModelPrefStore{}
	ctx := context.Background()
	if err := store.Set(ctx, "user-A", "deepseek-r1:7b"); err != nil {
		t.Fatalf("seed Set: %v", err)
	}

	reply := modelCommandReply(ctx, allow, store, "user-A", "default")
	if !strings.Contains(reply, "reset") || !strings.Contains(reply, "deepseek-r1:32b") {
		t.Fatalf("reset MUST confirm the revert to the system default, got %q", reply)
	}
	if _, ok, _ := store.Get(ctx, "user-A"); ok {
		t.Fatalf("reset MUST clear the sticky preference")
	}
	show := modelCommandReply(ctx, allow, store, "user-A", "")
	if !strings.Contains(show, "deepseek-r1:32b (system default)") {
		t.Fatalf("after reset, show MUST resolve the SST default, got:\n%s", show)
	}

	// "reset" alias behaves identically to "default".
	if err := store.Set(ctx, "user-A", "deepseek-r1:7b"); err != nil {
		t.Fatalf("re-seed Set: %v", err)
	}
	if r := modelCommandReply(ctx, allow, store, "user-A", "reset"); !strings.Contains(r, "reset") {
		t.Fatalf("/model reset alias MUST also reset, got %q", r)
	}
	if _, ok, _ := store.Get(ctx, "user-A"); ok {
		t.Fatalf("/model reset alias MUST clear the sticky preference")
	}
}
