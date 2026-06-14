package modelswitch

import (
	"strings"
	"testing"
)

// spec088TestAllowlist builds the canonical home-lab-shaped allowlist used
// across the spec-088 SCOPE-01 validator tests. The default model is
// gemma4:26b so the golden rejection wording reads "gemma4:26b (default)"
// (matching spec.md §UI Wireframes verbatim); the retry hint then resolves
// to the first non-default switchable model (deepseek-r1:7b).
func spec088TestAllowlist(t *testing.T) *Allowlist {
	t.Helper()
	a, err := NewAllowlist(
		[]string{"gemma4:26b", "deepseek-r1:7b"},
		map[string]int{
			"gemma4:26b":      18432,
			"deepseek-r1:7b":  4864,
			"deepseek-r1:32b": 22528, // profiled but busts the envelope co-resident with gather
			"gemma3:4b":       4096,
		},
		28672,        // home-lab ollama_memory_limit
		"gemma4:26b", // gather model (co-resident during synthesis)
		"gemma4:26b", // default = baseline synthesis model (wireframe marks this "(default)")
	)
	if err != nil {
		t.Fatalf("spec088TestAllowlist: unexpected build error: %v", err)
	}
	return a
}

// SCN-088-A02 (ADVERSARIAL) — an off-allowlist raw string is rejected with
// reason-code model_not_allowlisted; it is NEVER converted into an
// Override, and Resolve NEVER silently falls back to the baseline. Fails
// if Resolve ever returns a zero Override with a nil Rejection for an
// off-list model (the silent-fallback regression this gate forbids).
func TestAllowlist_Resolve_OffListRejected_ModelNotAllowlisted_Spec088(t *testing.T) {
	a := spec088TestAllowlist(t)
	ov, rej := a.Resolve("gpt-4o")

	if rej == nil {
		t.Fatalf("off-allowlist model MUST be rejected, got nil rejection (silent fallback to baseline is forbidden, SCN-088-A02)")
	}
	if !ov.IsZero() {
		t.Fatalf("rejected model MUST NOT become an Override, got SynthesisModel=%q", ov.SynthesisModel)
	}
	if rej.ReasonCode != ReasonNotAllowlisted {
		t.Fatalf("reason code = %q, want %q", rej.ReasonCode, ReasonNotAllowlisted)
	}
	if rej.RejectedModel != "gpt-4o" {
		t.Fatalf("rejected_model = %q, want %q", rej.RejectedModel, "gpt-4o")
	}
	// The rejected model id must NEVER appear in the allowed set the
	// surfaces render (it was not used and is not switchable).
	for _, m := range rej.AllowedModels {
		if m == "gpt-4o" {
			t.Fatalf("rejected model leaked into AllowedModels: %v", rej.AllowedModels)
		}
	}
}

// SCN-088-A02 — the rejection Message is the exact UX golden sentence
// (capital "I", em-dash, capitalised NOT, allowed set with the default
// marked, copy-paste retry). ONE string both surfaces render verbatim.
func TestAllowlist_RejectionMessage_GoldenWording_Spec088(t *testing.T) {
	a := spec088TestAllowlist(t)
	_, rej := a.Resolve("gpt-4o")
	if rej == nil {
		t.Fatalf("expected rejection for gpt-4o")
	}
	want := "\"gpt-4o\" is not a switchable model. I did NOT use it, and I did NOT fall back to the default — nothing was sent to the model.\n" +
		"Switchable models: gemma4:26b (default), deepseek-r1:7b.\n" +
		"Retry e.g. /ask --model=deepseek-r1:7b <your question>"
	if rej.Message != want {
		t.Fatalf("golden message mismatch.\n got: %q\nwant: %q", rej.Message, want)
	}
	// Fail-loud anchors the design marks as binding (spec.md §UI Wireframes):
	if !strings.Contains(rej.Message, "NOT") {
		t.Fatalf("message must carry the capitalised NOT emphasis: %q", rej.Message)
	}
	if !strings.Contains(rej.Message, "—") {
		t.Fatalf("message must use the em-dash voice anchor: %q", rej.Message)
	}
}

// SCN-088-A07 (ADVERSARIAL) — an un-profiled arbitrary string is rejected
// as model_not_allowlisted before any per-invocation config exists; it is
// never an Override. Fails if an unknown string is ever accepted.
func TestAllowlist_Resolve_UnprofiledRejected_ModelNotAllowlisted_Spec088(t *testing.T) {
	a := spec088TestAllowlist(t)
	ov, rej := a.Resolve("totally-made-up")
	if rej == nil {
		t.Fatalf("un-profiled model MUST be rejected, got nil rejection")
	}
	if !ov.IsZero() {
		t.Fatalf("un-profiled model MUST NOT become an Override, got %q", ov.SynthesisModel)
	}
	if rej.ReasonCode != ReasonNotAllowlisted {
		t.Fatalf("reason code = %q, want %q (un-profiled is not over-envelope)", rej.ReasonCode, ReasonNotAllowlisted)
	}
}

// SCN-088-A07 (ADVERSARIAL) — a profiled-but-too-big model
// (deepseek-r1:32b co-resident with the gemma4:26b gather model =
// 40960 MiB against a 28672 MiB envelope) is rejected with the distinct
// model_over_memory_envelope reason and the raise-the-envelope opt-up
// wording. Fails if it is mislabeled model_not_allowlisted or accepted.
func TestAllowlist_Resolve_ProfiledOverEnvelopeRejected_ModelOverMemEnvelope_Spec088(t *testing.T) {
	a := spec088TestAllowlist(t)
	ov, rej := a.Resolve("deepseek-r1:32b")
	if rej == nil {
		t.Fatalf("over-envelope model MUST be rejected, got nil rejection")
	}
	if !ov.IsZero() {
		t.Fatalf("over-envelope model MUST NOT become an Override, got %q", ov.SynthesisModel)
	}
	if rej.ReasonCode != ReasonOverMemEnvelope {
		t.Fatalf("reason code = %q, want %q (profiled-but-busts-envelope is distinct from not-allowlisted)", rej.ReasonCode, ReasonOverMemEnvelope)
	}
	wantMsg := "\"deepseek-r1:32b\" needs more memory than this environment's model budget allows, so it isn't switchable here. I did NOT use it and did NOT fall back to the default — nothing was sent to the model.\n" +
		"Switchable models that fit: gemma4:26b (default), deepseek-r1:7b.\n" +
		"To use a larger model, raise the environment's Ollama memory envelope first (operator opt-up)."
	if rej.Message != wantMsg {
		t.Fatalf("over-envelope golden message mismatch.\n got: %q\nwant: %q", rej.Message, wantMsg)
	}
}

// supplementary (A01/A03) — the baseline (empty) and applied (in-list)
// resolve contracts the spine relies on.
func TestAllowlist_Resolve_BaselineEmptyReturnsZeroOverride_Spec088(t *testing.T) {
	a := spec088TestAllowlist(t)
	for _, raw := range []string{"", "   ", "\t"} {
		ov, rej := a.Resolve(raw)
		if rej != nil {
			t.Fatalf("empty/blank override %q MUST be baseline (nil rejection), got %+v", raw, rej)
		}
		if !ov.IsZero() {
			t.Fatalf("empty/blank override %q MUST be a zero Override (baseline), got %q", raw, ov.SynthesisModel)
		}
	}
}

func TestAllowlist_Resolve_InListAppliedToSynthesis_Spec088(t *testing.T) {
	a := spec088TestAllowlist(t)
	ov, rej := a.Resolve("deepseek-r1:7b")
	if rej != nil {
		t.Fatalf("in-list model MUST NOT be rejected, got %+v", rej)
	}
	if ov.IsZero() || ov.SynthesisModel != "deepseek-r1:7b" {
		t.Fatalf("in-list model MUST become Override{SynthesisModel:\"deepseek-r1:7b\"}, got %q", ov.SynthesisModel)
	}
	// Leading/trailing whitespace is trimmed before matching.
	ov2, rej2 := a.Resolve("  deepseek-r1:7b  ")
	if rej2 != nil || ov2.SynthesisModel != "deepseek-r1:7b" {
		t.Fatalf("whitespace-padded in-list model MUST resolve to the trimmed Override, got ov=%q rej=%+v", ov2.SynthesisModel, rej2)
	}
}

// supplementary (A06 parity seam) — the SAME off-allowlist string yields a
// byte-identical Rejection on every call. This is the one shared contract
// both surfaces render verbatim (design Two-Surface Parity table).
func TestAllowlist_Resolve_SingleRejectionContract_Spec088(t *testing.T) {
	a := spec088TestAllowlist(t)
	_, r1 := a.Resolve("gpt-4o")
	_, r2 := a.Resolve("gpt-4o")
	if r1 == nil || r2 == nil {
		t.Fatalf("expected a rejection on both calls")
	}
	if r1.Message != r2.Message || r1.ReasonCode != r2.ReasonCode || r1.RejectedModel != r2.RejectedModel || r1.DefaultModel != r2.DefaultModel {
		t.Fatalf("rejection is not deterministic across calls:\n r1=%+v\n r2=%+v", *r1, *r2)
	}
	if strings.Join(r1.AllowedModels, ",") != strings.Join(r2.AllowedModels, ",") {
		t.Fatalf("AllowedModels diverged: %v vs %v", r1.AllowedModels, r2.AllowedModels)
	}
}

// build fail-loud — NewAllowlist rejects an empty set, an un-profiled
// entry, an over-envelope entry, and an empty default model. Each case
// MUST fail (G028 — no silent default).
func TestAllowlist_NewAllowlist_FailLoudBuild_Spec088(t *testing.T) {
	profiles := map[string]int{
		"gemma4:26b":      18432,
		"deepseek-r1:7b":  4864,
		"deepseek-r1:32b": 22528,
	}
	cases := []struct {
		name         string
		switchable   []string
		envelopeMiB  int
		gatherModel  string
		defaultModel string
		wantSubstr   string
	}{
		{"empty switchable set", nil, 28672, "gemma4:26b", "gemma4:26b", "empty"},
		{"unprofiled entry", []string{"no-such-model"}, 28672, "gemma4:26b", "gemma4:26b", "no model_memory_profiles entry"},
		{"over-envelope entry", []string{"deepseek-r1:32b"}, 28672, "gemma4:26b", "gemma4:26b", "ollama envelope"},
		{"empty default model", []string{"gemma4:26b"}, 28672, "gemma4:26b", "", "defaultModel"},
		{"blank entry", []string{"gemma4:26b", "   "}, 28672, "gemma4:26b", "gemma4:26b", "empty entry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := NewAllowlist(tc.switchable, profiles, tc.envelopeMiB, tc.gatherModel, tc.defaultModel)
			if err == nil {
				t.Fatalf("expected fail-loud build error for %q, got allowlist %+v", tc.name, a)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error should mention %q, got: %v", tc.wantSubstr, err)
			}
		})
	}
}

// On dev (envelopeMiB == 0) the co-residence check is skipped: a profiled
// switchable set builds even with no ollama envelope known, matching the
// runtime/config envelope-skip semantics.
func TestAllowlist_NewAllowlist_DevEnvelopeSkipped_Spec088(t *testing.T) {
	a, err := NewAllowlist(
		[]string{"gemma3:4b"},
		map[string]int{"gemma3:4b": 4096},
		0, // dev: no ollama envelope
		"gemma3:4b",
		"gemma3:4b",
	)
	if err != nil {
		t.Fatalf("dev build (envelope 0) MUST succeed for a profiled set, got: %v", err)
	}
	ov, rej := a.Resolve("gemma3:4b")
	if rej != nil || ov.SynthesisModel != "gemma3:4b" {
		t.Fatalf("dev in-list resolve failed: ov=%q rej=%+v", ov.SynthesisModel, rej)
	}
}
