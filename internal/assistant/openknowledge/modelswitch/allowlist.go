// Package modelswitch — Spec 088 SCOPE-01 foundation: the shared,
// surface-agnostic validator + allowlist that gates a runtime
// per-invocation open-knowledge model override.
//
// One Allowlist instance is built at wiring from SST and installed as a
// package singleton in internal/assistant/openknowledge/agenttool; BOTH
// the Telegram facade fast-path and the web/HTTP fast-path resolve
// through the SAME instance (SCN-088-A06 parity). The override is a
// per-request PARAMETER, never a runtime SST write (C6).
//
// Resolve converts an UNTRUSTED, user-supplied model string into either
// a validated Override (applied to the spec-087 forced-final SYNTHESIS
// turn only, Fork B) or a typed, fail-loud Rejection — NEVER a silent
// default, NEVER a backend passthrough (FR-3 / SCN-088-A02 / A07). An
// empty string is the baseline (zero Override), so the no-override path
// is byte-for-byte today's behaviour (FR-1 / NFR-4).
//
// The package is pure (stdlib only): no project imports, no side
// effects, no I/O. That keeps it a leaf (no import cycle with agent /
// agenttool / facade / api) and makes the closed-set validation
// table-testable without any live agent or Ollama daemon.
package modelswitch

import (
	"fmt"
	"strings"
)

// Reason codes distinguishing WHY an off-allowlist model was refused.
// model_not_allowlisted — unknown / un-profiled / not offered by the
// operator. model_over_memory_envelope — profiled, but co-resident with
// the gather model it busts the target environment's ollama envelope
// (the spec-087 F-OPTUP "raise the envelope first" opt-up).
const (
	ReasonNotAllowlisted  = "model_not_allowlisted"
	ReasonOverMemEnvelope = "model_over_memory_envelope"
)

// Override is the validated per-invocation result of Resolve. The zero
// value (SynthesisModel == "") is the baseline — no override — so the
// no-override path is byte-for-byte identical to spec 087 (FR-1 / NFR-4).
// Fork B: an override re-points the forced-final SYNTHESIS turn only;
// the gather/tool turns always keep the SST baseline tool model.
type Override struct {
	SynthesisModel string
}

// IsZero reports whether the override is the baseline (no model switch).
func (o Override) IsZero() bool { return o.SynthesisModel == "" }

// Rejection is the typed, fail-loud refusal value. ONE struct, rendered
// verbatim by BOTH surfaces (Telegram reply text + HTTP 400 envelope) so
// the rejection is identical on every surface (SCN-088-A06). Message is
// the exact UX golden sentence keyed on ReasonCode.
type Rejection struct {
	RejectedModel string
	AllowedModels []string
	DefaultModel  string
	ReasonCode    string
	Message       string
}

// Allowlist is the immutable (after build) closed-set validator. It owns
// the operator-curated switchable set, the memory profiles, the env
// ollama envelope, the co-resident gather model, and the baseline
// synthesis ("default") model used in the rejection wording.
type Allowlist struct {
	switchable   []string       // operator-curated switchable set (order preserved for messages)
	profiles     map[string]int // model_memory_profiles (MiB)
	envelopeMiB  int            // OllamaMemoryLimitMiB (0 ⇒ envelope check skipped, e.g. dev)
	gatherModel  string         // baseline llm_model_id, co-resident during the synthesis turn
	defaultModel string         // baseline synthesis_model_id — the no-override synthesis model
}

// NewAllowlist builds a validated, immutable Allowlist. Fail-loud
// (G028 / no silent default): the switchable set MUST be non-empty, each
// entry MUST have a model_memory_profiles entry, each entry MUST
// co-resident-fit the env envelope when envelopeMiB != 0, and
// defaultModel MUST be non-empty. The same co-residence arithmetic the
// config-generation envelope guard uses (internal/config/config.go
// validateModelEnvelopes) — gather resident + candidate ≤ envelope, a
// single load when candidate == gather.
func NewAllowlist(switchable []string, profiles map[string]int, envelopeMiB int, gatherModel, defaultModel string) (*Allowlist, error) {
	var errs []string
	if len(switchable) == 0 {
		errs = append(errs, "switchable model set is empty (REQUIRED non-empty when open-knowledge is enabled)")
	}
	if strings.TrimSpace(defaultModel) == "" {
		errs = append(errs, "defaultModel (baseline synthesis_model_id) is empty")
	}
	base := profiles[gatherModel]
	for _, m := range switchable {
		mt := strings.TrimSpace(m)
		if mt == "" {
			errs = append(errs, "switchable set contains an empty entry")
			continue
		}
		profileMiB, ok := profiles[mt]
		if !ok {
			errs = append(errs, fmt.Sprintf("switchable model %q has no model_memory_profiles entry", mt))
			continue
		}
		if envelopeMiB != 0 {
			coresident := base
			if mt != gatherModel {
				coresident += profileMiB
			}
			if coresident > envelopeMiB {
				errs = append(errs, fmt.Sprintf("switchable model %q + gather model needs %d MiB but the ollama envelope is %d MiB", mt, coresident, envelopeMiB))
			}
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("modelswitch: invalid switchable allowlist: %s", strings.Join(errs, "; "))
	}
	// Defensive copies so the immutable contract holds even if the
	// caller later mutates the slices/maps it passed in.
	sw := append([]string(nil), switchable...)
	pf := make(map[string]int, len(profiles))
	for k, v := range profiles {
		pf[k] = v
	}
	return &Allowlist{
		switchable:   sw,
		profiles:     pf,
		envelopeMiB:  envelopeMiB,
		gatherModel:  gatherModel,
		defaultModel: defaultModel,
	}, nil
}

// Resolve converts an untrusted raw model string into a validated
// Override or a typed Rejection. Contract:
//   - ""            ⇒ zero Override, nil Rejection (baseline; FR-1, NFR-4)
//   - in-list model ⇒ Override{SynthesisModel: raw}, nil Rejection
//   - anything else ⇒ zero Override, a *Rejection (FR-3: never a silent
//     default, never a backend passthrough)
//
// Resolve never sends anything to the inference backend; it is a pure
// boundary guard run BEFORE any per-invocation agent config is built.
func (a *Allowlist) Resolve(raw string) (Override, *Rejection) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Override{}, nil
	}
	if a.isSwitchable(raw) {
		return Override{SynthesisModel: raw}, nil
	}
	return Override{}, a.reject(raw)
}

// AllowedModels returns a defensive copy of the operator-curated
// switchable set (order preserved). Surfaces use it for discovery.
func (a *Allowlist) AllowedModels() []string {
	return append([]string(nil), a.switchable...)
}

// DefaultModel returns the baseline synthesis model (the no-override
// synthesis model), surfaced as default_model in the rejection envelope.
func (a *Allowlist) DefaultModel() string { return a.defaultModel }

func (a *Allowlist) isSwitchable(raw string) bool {
	for _, m := range a.switchable {
		if m == raw {
			return true
		}
	}
	return false
}

// reject classifies an off-allowlist raw model into the correct
// reason-code. A profiled model whose co-resident sum busts the envelope
// is model_over_memory_envelope (the helpful "raise the envelope" path);
// everything else (unknown / un-profiled / profiled-but-fits-yet-not-
// offered) is model_not_allowlisted.
func (a *Allowlist) reject(raw string) *Rejection {
	code := ReasonNotAllowlisted
	if profileMiB, ok := a.profiles[raw]; ok {
		coresident := a.profiles[a.gatherModel]
		if raw != a.gatherModel {
			coresident += profileMiB
		}
		if a.envelopeMiB != 0 && coresident > a.envelopeMiB {
			code = ReasonOverMemEnvelope
		}
	}
	return &Rejection{
		RejectedModel: raw,
		AllowedModels: a.AllowedModels(),
		DefaultModel:  a.defaultModel,
		ReasonCode:    code,
		Message:       a.message(raw, code),
	}
}

// message builds the exact UX golden sentence (spec.md §UI Wireframes,
// binding). Voice contract: sentence-case, capital "I", em-dash, the
// deliberate capitalised NOT (the fail-loud "your model was not used"
// emphasis design may not soften), the allowed set with the default
// marked, and a copy-paste retry. ONE string, used verbatim by both
// surfaces.
func (a *Allowlist) message(raw, code string) string {
	switch code {
	case ReasonOverMemEnvelope:
		return fmt.Sprintf(
			"%q needs more memory than this environment's model budget allows, so it isn't switchable here. I did NOT use it and did NOT fall back to the default — nothing was sent to the model.\n"+
				"Switchable models that fit: %s.\n"+
				"To use a larger model, raise the environment's Ollama memory envelope first (operator opt-up).",
			raw, a.allowedPhrase())
	default: // ReasonNotAllowlisted
		return fmt.Sprintf(
			"%q is not a switchable model. I did NOT use it, and I did NOT fall back to the default — nothing was sent to the model.\n"+
				"Switchable models: %s.\n"+
				"Retry e.g. /ask --model=%s <your question>",
			raw, a.allowedPhrase(), a.retryModel())
	}
}

// allowedPhrase renders the switchable set as "m1 (default), m2" with the
// default model marked in its list position.
func (a *Allowlist) allowedPhrase() string {
	parts := make([]string, 0, len(a.switchable))
	for _, m := range a.switchable {
		if m == a.defaultModel {
			parts = append(parts, m+" (default)")
		} else {
			parts = append(parts, m)
		}
	}
	return strings.Join(parts, ", ")
}

// retryModel picks a deterministic copy-paste example for the retry
// hint: the first switchable model that differs from the default (so the
// example is a meaningful switch), else the first switchable model.
func (a *Allowlist) retryModel() string {
	for _, m := range a.switchable {
		if m != a.defaultModel {
			return m
		}
	}
	if len(a.switchable) > 0 {
		return a.switchable[0]
	}
	return a.defaultModel
}
