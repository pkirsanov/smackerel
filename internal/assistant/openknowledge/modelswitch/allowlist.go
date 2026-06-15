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
	"log/slog"
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
	// ReasonNotToolCapable — spec 089 (Fork C): a per-request GATHER model
	// override (--gather-model= / gather_model) that is not a member of the
	// operator-curated tool_capable_gather_models SST set. The gather/tool
	// turns require a tool-calling-capable model; a non-tool-capable gather
	// is refused BEFORE any gather turn runs (FR-8), never silently applied.
	ReasonNotToolCapable = "model_not_tool_capable"
)

// Turn identifies WHICH turn a rejection refers to, carried on the HTTP
// envelope (rejected_turn) so a caller can tell a synthesis refusal from a
// gather refusal. Spec 089.
const (
	TurnSynthesis = "synthesis"
	TurnGather    = "gather"
)

// Source classifies HOW a resolved model was selected (spec 089 precedence:
// per-request > sticky > SST default). Carried into the attribution so a
// surface can render "this question" (per-request) vs "your default" (sticky)
// vs no footer (default). Closed set.
const (
	SourceDefault    = "default"
	SourceSticky     = "sticky"
	SourcePerRequest = "per_request"
)

// Override is the validated per-invocation result of Resolve. The zero
// value (SynthesisModel == "") is the baseline — no override — so the
// no-override path is byte-for-byte identical to spec 087 (FR-1 / NFR-4).
// Fork B: an override re-points the forced-final SYNTHESIS turn only;
// the gather/tool turns always keep the SST baseline tool model.
type Override struct {
	SynthesisModel string
	// GatherModel re-points the gather/tool turns (spec 089 Fork C). Empty
	// keeps the SST baseline gather model. Independent of SynthesisModel: a
	// caller may switch the gather turn, the synthesis turn, both, or neither.
	GatherModel string
}

// IsZero reports whether the override is the baseline (no model switch on
// EITHER turn) — so WithModelOverride returns the receiver unchanged and the
// no-selection path is byte-for-byte spec 087/088 (NFR-4).
func (o Override) IsZero() bool { return o.SynthesisModel == "" && o.GatherModel == "" }

// Effective is the resolved per-invocation selection: the synthesis and gather
// model ids that WILL run (always populated — a default resolves to the
// baseline ids) plus each one's Source classification. Built by
// ResolveEffective; consumed for attribution (the surfaces render the source
// tags) and to construct the per-invocation Override clone. Spec 089.
type Effective struct {
	SynthesisModel  string
	SynthesisSource string // SourceDefault | SourceSticky | SourcePerRequest
	GatherModel     string
	GatherSource    string // SourceDefault | SourcePerRequest (sticky gather deferred)
}

// Override builds the per-invocation clone input from the resolved Effective.
// A turn whose source is SourceDefault contributes NO override (so a pure-
// default invocation yields a zero Override and WithModelOverride returns the
// receiver unchanged, byte-for-byte baseline — NFR-4).
func (e Effective) Override() Override {
	var o Override
	if e.SynthesisSource != SourceDefault {
		o.SynthesisModel = e.SynthesisModel
	}
	if e.GatherSource != SourceDefault {
		o.GatherModel = e.GatherModel
	}
	return o
}

// Rejection is the typed, fail-loud refusal value. ONE struct, rendered
// verbatim by BOTH surfaces (Telegram reply text + HTTP 400 envelope) so
// the rejection is identical on every surface (SCN-088-A06). Message is
// the exact UX golden sentence keyed on ReasonCode.
type Rejection struct {
	RejectedModel string
	AllowedModels []string
	DefaultModel  string
	ReasonCode    string
	// RejectedTurn names which turn was refused ("synthesis" | "gather"),
	// surfaced as rejected_turn on the HTTP envelope. Empty on the spec-088
	// Resolve path (synthesis-only) so that envelope is unchanged (omitempty).
	RejectedTurn string
	Message      string
}

// Allowlist is the immutable (after build) closed-set validator. It owns
// the operator-curated switchable set, the memory profiles, the env
// ollama envelope, the co-resident gather model, and the baseline
// synthesis ("default") model used in the rejection wording.
type Allowlist struct {
	switchable        []string       // operator-curated switchable set (order preserved for messages)
	profiles          map[string]int // model_memory_profiles (MiB)
	envelopeMiB       int            // OllamaMemoryLimitMiB (0 ⇒ envelope check skipped, e.g. dev)
	gatherModel       string         // baseline llm_model_id, co-resident during the synthesis turn
	defaultModel      string         // baseline synthesis_model_id — the no-override synthesis model
	toolCapableGather []string       // spec 089: tool_capable_gather_models — the set a --gather-model= may switch to
}

// NewAllowlist builds a validated, immutable Allowlist. Fail-loud
// (G028 / no silent default): the switchable set MUST be non-empty, each
// entry MUST have a model_memory_profiles entry, each entry MUST
// co-resident-fit the env envelope when envelopeMiB != 0, and
// defaultModel MUST be non-empty. The same co-residence arithmetic the
// config-generation envelope guard uses (internal/config/config.go
// validateModelEnvelopes) — gather resident + candidate ≤ envelope, a
// single load when candidate == gather.
//
// Spec 089 (Fork C): toolCapableGather is the operator-curated
// tool_capable_gather_models set a per-request --gather-model= may switch
// to. REQUIRED non-empty when open-knowledge is enabled, no empty entries,
// and the baseline gatherModel MUST be a member (so the no-override gather
// path always passes). The config layer enforces the same rules + the
// per-entry profile at config-generation; this is the defensive runtime
// mirror.
func NewAllowlist(switchable []string, profiles map[string]int, envelopeMiB int, gatherModel, defaultModel string, toolCapableGather []string) (*Allowlist, error) {
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
	// Spec 089 — tool-capable gather set membership contract.
	if len(toolCapableGather) == 0 {
		errs = append(errs, "tool-capable gather model set is empty (REQUIRED non-empty when open-knowledge is enabled)")
	} else {
		baselineMember := false
		for _, m := range toolCapableGather {
			mt := strings.TrimSpace(m)
			if mt == "" {
				errs = append(errs, "tool-capable gather set contains an empty entry")
				continue
			}
			if mt == strings.TrimSpace(gatherModel) {
				baselineMember = true
			}
		}
		if strings.TrimSpace(gatherModel) != "" && !baselineMember {
			errs = append(errs, fmt.Sprintf("baseline gather model %q must be a member of the tool-capable gather set (the no-override gather path must always pass)", gatherModel))
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("modelswitch: invalid switchable allowlist: %s", strings.Join(errs, "; "))
	}
	// Defensive copies so the immutable contract holds even if the
	// caller later mutates the slices/maps it passed in.
	sw := append([]string(nil), switchable...)
	tcg := append([]string(nil), toolCapableGather...)
	pf := make(map[string]int, len(profiles))
	for k, v := range profiles {
		pf[k] = v
	}
	return &Allowlist{
		switchable:        sw,
		profiles:          pf,
		envelopeMiB:       envelopeMiB,
		gatherModel:       gatherModel,
		defaultModel:      defaultModel,
		toolCapableGather: tcg,
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

// ResolveEffective applies the spec 089 precedence (per-request > sticky > SST
// default) to BOTH turns, validates each WINNING model, classifies its Source,
// and returns either the resolved Effective or a typed, fail-loud *Rejection.
// Pure boundary guard run BEFORE any per-invocation agent config is built; it
// never touches the inference backend.
//
//   - perReqSynth / perReqGather are UNTRUSTED request values (e.g. parsed from
//     /ask --model= / --gather-model= or the HTTP model / gather_model fields).
//   - stickySynth is the caller's claim-bound stored synthesis preference
//     (already validated at set time; re-validated here defensively).
//
// Rules (deterministic):
//   - Synthesis: a per-request value is validated against the switchable set; a
//     failure is an explicit fail-loud Rejection (RejectedTurn=synthesis) — it
//     does NOT fall through to sticky/default (the user asked for a specific
//     model). A sticky value is re-validated; if the operator has RETIRED it
//     from the switchable set (orphaned), it resolves to the SST default + a
//     structured log, never breaking every /ask for that user.
//   - Gather: a per-request value is validated against the tool-capable set
//     (ResolveGather); a failure is a fail-loud Rejection (RejectedTurn=gather).
//     Sticky gather is deferred (F-STICKY-GATHER), so absent a per-request
//     gather the gather turn resolves to the baseline gather model.
func (a *Allowlist) ResolveEffective(perReqSynth, perReqGather, stickySynth string) (Effective, *Rejection) {
	var eff Effective

	// --- Synthesis turn: per-request > sticky > SST default ---
	switch ps := strings.TrimSpace(perReqSynth); {
	case ps != "":
		if !a.isSwitchable(ps) {
			rej := a.reject(ps)
			rej.RejectedTurn = TurnSynthesis
			return Effective{}, rej
		}
		eff.SynthesisModel = ps
		eff.SynthesisSource = SourcePerRequest
	default:
		if sticky := strings.TrimSpace(stickySynth); sticky != "" {
			if a.isSwitchable(sticky) {
				eff.SynthesisModel = sticky
				eff.SynthesisSource = SourceSticky
			} else {
				// Orphaned sticky — the operator retired this model from the
				// switchable set. NOT this turn's fault: resolve to the SST
				// default (never refuse every /ask for the user) + log so the
				// operator can see the stale preference.
				slog.Warn("modelswitch: orphaned sticky synthesis preference resolved to SST default",
					"sticky_model", sticky, "default_model", a.defaultModel)
				eff.SynthesisModel = a.defaultModel
				eff.SynthesisSource = SourceDefault
			}
		} else {
			eff.SynthesisModel = a.defaultModel
			eff.SynthesisSource = SourceDefault
		}
	}

	// --- Gather turn: per-request > SST default (sticky gather deferred) ---
	g, rej := a.ResolveGather(perReqGather)
	if rej != nil {
		return Effective{}, rej
	}
	if g != "" {
		eff.GatherModel = g
		eff.GatherSource = SourcePerRequest
	} else {
		eff.GatherModel = a.gatherModel
		eff.GatherSource = SourceDefault
	}

	return eff, nil
}

// ResolveGather validates an UNTRUSTED per-request gather model against the
// operator-curated tool_capable_gather_models set. Contract:
//   - ""            ⇒ "", nil (baseline; caller uses the SST gather model)
//   - tool-capable  ⇒ raw, nil
//   - anything else ⇒ "", *Rejection{ReasonNotToolCapable, RejectedTurn: gather}
//
// The gather/tool turns require a tool-calling-capable model; a non-tool-capable
// selection is refused BEFORE any gather turn runs (FR-8) — never silently
// applied or downgraded.
func (a *Allowlist) ResolveGather(raw string) (string, *Rejection) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if a.isToolCapable(raw) {
		return raw, nil
	}
	return "", &Rejection{
		RejectedModel: raw,
		AllowedModels: a.ToolCapableGatherModels(),
		DefaultModel:  a.gatherModel,
		ReasonCode:    ReasonNotToolCapable,
		RejectedTurn:  TurnGather,
		Message:       a.gatherMessage(raw),
	}
}

// ToolCapableGatherModels returns a defensive copy of the operator-curated
// tool-capable gather set (order preserved). Surfaces use it for discovery
// and the gather rejection envelope.
func (a *Allowlist) ToolCapableGatherModels() []string {
	return append([]string(nil), a.toolCapableGather...)
}

func (a *Allowlist) isToolCapable(raw string) bool {
	for _, m := range a.toolCapableGather {
		if m == raw {
			return true
		}
	}
	return false
}

// gatherMessage builds the fail-loud gather-rejection sentence in the same
// voice as message(): the deliberate capitalised NOT, the tool-capable set
// with the baseline marked, and a copy-paste --gather-model= retry. Spec 089.
func (a *Allowlist) gatherMessage(raw string) string {
	return fmt.Sprintf(
		"%q is not a tool-calling-capable gather model, so it can't run the search and tool turns. I did NOT use it, and I did NOT fall back to the default — nothing was sent to the model.\n"+
			"Tool-capable gather models: %s.\n"+
			"Retry e.g. /ask --gather-model=%s <your question>",
		raw, a.toolCapablePhrase(), a.toolCapableRetryModel())
}

// toolCapablePhrase renders the tool-capable set as "m1 (default), m2" with the
// baseline gather model marked in its list position.
func (a *Allowlist) toolCapablePhrase() string {
	parts := make([]string, 0, len(a.toolCapableGather))
	for _, m := range a.toolCapableGather {
		if m == a.gatherModel {
			parts = append(parts, m+" (default)")
		} else {
			parts = append(parts, m)
		}
	}
	return strings.Join(parts, ", ")
}

// toolCapableRetryModel picks a deterministic copy-paste example for the gather
// retry hint: the first tool-capable model that differs from the baseline (so
// the example is a meaningful switch), else the first, else the baseline.
func (a *Allowlist) toolCapableRetryModel() string {
	for _, m := range a.toolCapableGather {
		if m != a.gatherModel {
			return m
		}
	}
	if len(a.toolCapableGather) > 0 {
		return a.toolCapableGather[0]
	}
	return a.gatherModel
}
