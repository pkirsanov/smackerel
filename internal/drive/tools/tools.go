// Package tools registers the four Spec 038 Scope 7 drive agent tools
// with the spec 037 scenario-agent registry. The tools route every call
// through the existing drive runtime services (retrieve.Service,
// save.Service, rules.Repository, policy.Engine) so the LLM cannot
// bypass the same authorization, sensitivity policy, idempotency, and
// trace contracts that the HTTP API and Telegram bot already enforce.
//
// The registered tools are:
//
//	drive_search    (read)     — search drive_file artifacts
//	drive_get_file  (external) — retrieve bytes / provider link
//	drive_save_file (external) — save bytes via the Save Service
//	drive_list_rules(read)     — list configured Save Rules
//
// The package is a sibling of internal/drive (rather than tools.go
// inside the drive package) because the drive subpackages save, rules,
// retrieve, and policy already import internal/drive — registering
// tools that touch those services from inside drive itself would
// produce an import cycle. The agent registry contract ("tools register
// from the package that owns the data") is preserved by keeping the
// registration in this drive-owned subpackage and never importing it
// from anywhere except the cmd/core wiring.
//
// Wiring: production code in cmd/core constructs a *ToolServices and
// calls SetToolServices once at startup. Until SetToolServices is
// called the handlers return a structured `{"ok":false,
// "error":"drive_tools_not_configured"}` payload — failing loudly
// inside the trace instead of crashing the binary.
package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/drive/policy"
	"github.com/smackerel/smackerel/internal/drive/retrieve"
	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
)

// ToolNames enumerates the four agent tools registered by this package.
// Surfaces (allowlist generators, lint-style checks) MUST consult this
// list rather than hard-coding strings to avoid drift.
var ToolNames = []string{
	"drive_search",
	"drive_get_file",
	"drive_save_file",
	"drive_list_rules",
}

// SaveRequestInput is the structured argument the agent supplies when
// calling drive_save_file. Mirrors the fields rules.Rule + save.Bytes
// require so the agent surface stays decoupled from internal types.
type SaveRequestInput struct {
	ArtifactID     string            `json:"artifact_id"`
	Title          string            `json:"title"`
	MimeType       string            `json:"mime_type"`
	Classification string            `json:"classification"`
	Sensitivity    string            `json:"sensitivity"`
	Confidence     float64           `json:"confidence"`
	Tokens         map[string]string `json:"tokens"`
	ContentBase64  string            `json:"content_base64"`
}

// ToolServices holds the runtime dependencies required by the four
// drive tools. Production wiring constructs one in cmd/core and calls
// SetToolServices once before the agent bridge starts dispatching.
// Tests construct their own and override via SetToolServices /
// ResetForTest.
type ToolServices struct {
	// Retriever powers drive_search (candidate listing) and
	// drive_get_file (single-candidate delivery).
	Retriever *retrieve.Service
	// SaveService powers drive_save_file.
	SaveService *save.Service
	// RulesRepo powers drive_list_rules and the rule lookup performed
	// by drive_save_file.
	RulesRepo *rules.Repository
	// RulesEngine evaluates the Save Rule that drive_save_file picks.
	RulesEngine *rules.Engine
	// Policy is consulted by drive_save_file before issuing provider
	// writes. Required so save tool calls cannot bypass the policy
	// engine that the Save Rules HTTP path enforces.
	Policy *policy.Engine
}

var (
	servicesMu sync.RWMutex
	services   *ToolServices
)

// SetToolServices wires the production drive runtime into the four
// agent-tool handlers. Pass nil to clear (test-only). Calling
// SetToolServices is idempotent; the most recent non-nil call wins.
func SetToolServices(s *ToolServices) {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	services = s
}

// ResetForTest clears the wired services. Test-only — production code
// MUST NOT call this.
func ResetForTest() {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	services = nil
}

// loadServices returns the wired services or a structured error envelope
// if SetToolServices has not been called. Handlers MUST call this rather
// than accessing the package variable directly so the "not configured"
// path is uniform.
func loadServices() (*ToolServices, error) {
	servicesMu.RLock()
	defer servicesMu.RUnlock()
	if services == nil {
		return nil, errors.New("drive_tools_not_configured")
	}
	return services, nil
}

func init() {
	registerDriveTools()
}

// registerDriveTools registers the four tools. Public so tests can
// re-register after a registry reset; production wiring relies on the
// package init() call above.
func registerDriveTools() {
	agent.RegisterTool(agent.Tool{
		Name:             "drive_search",
		Description:      "Search drive files by free-text query; returns provider-neutral candidates with title, folder, provider, sensitivity",
		InputSchema:      driveSearchInputSchema,
		OutputSchema:     driveSearchOutputSchema,
		SideEffectClass:  agent.SideEffectRead,
		OwningPackage:    "internal/drive/tools",
		PerCallTimeoutMs: 5000,
		Handler:          handleDriveSearch,
	})
	agent.RegisterTool(agent.Tool{
		Name:             "drive_get_file",
		Description:      "Retrieve a single drive file by artifact id; respects sensitivity policy and channel size limits",
		InputSchema:      driveGetFileInputSchema,
		OutputSchema:     driveGetFileOutputSchema,
		SideEffectClass:  agent.SideEffectExternal,
		OwningPackage:    "internal/drive/tools",
		PerCallTimeoutMs: 15000,
		Handler:          handleDriveGetFile,
	})
	agent.RegisterTool(agent.Tool{
		Name:             "drive_save_file",
		Description:      "Save bytes to drive via the configured Save Rules; returns destination folder, provider URL, or skip reason",
		InputSchema:      driveSaveFileInputSchema,
		OutputSchema:     driveSaveFileOutputSchema,
		SideEffectClass:  agent.SideEffectExternal,
		OwningPackage:    "internal/drive/tools",
		PerCallTimeoutMs: 30000,
		Handler:          handleDriveSaveFile,
	})
	agent.RegisterTool(agent.Tool{
		Name:             "drive_list_rules",
		Description:      "List configured Save Rules including provider, classification filter, and target folder template",
		InputSchema:      driveListRulesInputSchema,
		OutputSchema:     driveListRulesOutputSchema,
		SideEffectClass:  agent.SideEffectRead,
		OwningPackage:    "internal/drive/tools",
		PerCallTimeoutMs: 5000,
		Handler:          handleDriveListRules,
	})
}

// -------------------- schemas (JSON Schema Draft 2020-12) --------------------

var driveSearchInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["query"],
  "properties": {
    "query":  {"type": "string", "minLength": 1},
    "limit":  {"type": "integer", "minimum": 1, "maximum": 25},
    "channel": {"type": "string", "enum": ["telegram"]},
    "allowed_classifications": {
      "type": "array",
      "items": {"type": "string"}
    }
  }
}`)

var driveSearchOutputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["ok"],
  "properties": {
    "ok":    {"type": "boolean"},
    "error": {"type": "string"},
    "candidates": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["artifact_id", "title", "folder", "provider", "sensitivity", "size_bytes", "provider_url"],
        "properties": {
          "artifact_id":  {"type": "string"},
          "title":        {"type": "string"},
          "folder":       {"type": "string"},
          "provider":     {"type": "string"},
          "sensitivity": {"type": "string", "enum": ["none", "financial", "medical", "identity"]},
          "size_bytes":  {"type": "integer", "minimum": 0},
          "provider_url": {"type": "string"}
        }
      }
    }
  }
}`)

var driveGetFileInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["query"],
  "properties": {
    "query":               {"type": "string", "minLength": 1},
    "selected_artifact_id": {"type": "string"},
    "channel":              {"type": "string", "enum": ["telegram"]}
  }
}`)

var driveGetFileOutputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["ok", "mode"],
  "properties": {
    "ok":            {"type": "boolean"},
    "error":         {"type": "string"},
    "mode":          {"type": "string", "enum": ["bytes", "secure_link", "provider_link", "refused", "disambiguate"]},
    "url":           {"type": "string"},
    "title":         {"type": "string"},
    "sensitivity":   {"type": "string"},
    "policy_reason": {"type": "string"},
    "hint":          {"type": "string"},
    "mime_type":     {"type": "string"},
    "bytes_base64":  {"type": "string"},
    "candidates": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["artifact_id", "title", "folder", "provider", "sensitivity", "size_bytes", "provider_url"],
        "properties": {
          "artifact_id":  {"type": "string"},
          "title":        {"type": "string"},
          "folder":       {"type": "string"},
          "provider":     {"type": "string"},
          "sensitivity": {"type": "string"},
          "size_bytes":  {"type": "integer"},
          "provider_url": {"type": "string"}
        }
      }
    }
  }
}`)

var driveSaveFileInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["artifact_id", "title", "classification", "sensitivity", "confidence"],
  "properties": {
    "artifact_id":   {"type": "string", "minLength": 1},
    "title":          {"type": "string", "minLength": 1},
    "mime_type":     {"type": "string"},
    "classification": {"type": "string", "minLength": 1},
    "sensitivity":   {"type": "string", "enum": ["none", "financial", "medical", "identity"]},
    "confidence":    {"type": "number", "minimum": 0, "maximum": 1},
    "tokens":        {"type": "object", "additionalProperties": {"type": "string"}},
    "content_base64": {"type": "string"}
  }
}`)

var driveSaveFileOutputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["ok"],
  "properties": {
    "ok":            {"type": "boolean"},
    "error":         {"type": "string"},
    "saved":         {"type": "boolean"},
    "skipped":       {"type": "boolean"},
    "rule_id":       {"type": "string"},
    "folder":        {"type": "string"},
    "provider_url": {"type": "string"},
    "policy_reason": {"type": "string"},
    "reason":        {"type": "string"}
  }
}`)

var driveListRulesInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {}
}`)

var driveListRulesOutputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["ok", "rules"],
  "properties": {
    "ok":    {"type": "boolean"},
    "error": {"type": "string"},
    "rules": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "name", "enabled", "provider_id", "target_folder_template"],
        "properties": {
          "id":                      {"type": "string"},
          "name":                    {"type": "string"},
          "enabled":                {"type": "boolean"},
          "source_kinds":           {"type": "array", "items": {"type": "string"}},
          "classification":         {"type": "string"},
          "sensitivity_in":         {"type": "array", "items": {"type": "string"}},
          "confidence_min":         {"type": "number"},
          "provider_id":            {"type": "string"},
          "target_folder_template": {"type": "string"}
        }
      }
    }
  }
}`)

// -------------------- handlers --------------------

func notConfiguredOutput(name string) (json.RawMessage, error) {
	payload := map[string]any{"ok": false, "error": "drive_tools_not_configured"}
	if name == "drive_get_file" {
		payload["mode"] = string(retrieve.ModeRefused)
	}
	if name == "drive_list_rules" {
		payload["rules"] = []any{}
	}
	return json.Marshal(payload)
}

func handleDriveSearch(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	svc, err := loadServices()
	if err != nil {
		return notConfiguredOutput("drive_search")
	}
	if svc.Retriever == nil {
		return notConfiguredOutput("drive_search")
	}
	var input struct {
		Query                  string   `json:"query"`
		Limit                  int      `json:"limit"`
		Channel                string   `json:"channel"`
		AllowedClassifications []string `json:"allowed_classifications"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return marshalErr("drive_search", err)
	}
	channel := retrieve.ChannelTelegram
	if input.Channel != "" {
		channel = retrieve.Channel(input.Channel)
	}
	delivery, err := svc.Retriever.Retrieve(ctx, retrieve.RetrieveRequest{
		Channel:        channel,
		Query:          input.Query,
		Limit:          input.Limit,
		AllowedClassif: input.AllowedClassifications,
	})
	if err != nil {
		return marshalErr("drive_search", err)
	}
	return json.Marshal(map[string]any{
		"ok":         true,
		"candidates": candidatesPayload(delivery.Candidates),
	})
}

func handleDriveGetFile(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	svc, err := loadServices()
	if err != nil {
		return notConfiguredOutput("drive_get_file")
	}
	if svc.Retriever == nil {
		return notConfiguredOutput("drive_get_file")
	}
	var input struct {
		Query              string `json:"query"`
		SelectedArtifactID string `json:"selected_artifact_id"`
		Channel            string `json:"channel"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return marshalErr("drive_get_file", err)
	}
	channel := retrieve.ChannelTelegram
	if input.Channel != "" {
		channel = retrieve.Channel(input.Channel)
	}
	delivery, err := svc.Retriever.Retrieve(ctx, retrieve.RetrieveRequest{
		Channel:            channel,
		Query:              input.Query,
		SelectedArtifactID: input.SelectedArtifactID,
	})
	if err != nil {
		return marshalErr("drive_get_file", err)
	}
	out := map[string]any{
		"ok":            true,
		"mode":          string(delivery.Mode),
		"url":           delivery.URL,
		"title":         delivery.Title,
		"sensitivity":   delivery.Sensitivity,
		"policy_reason": delivery.PolicyReason,
		"hint":          delivery.Hint,
		"candidates":    candidatesPayload(delivery.Candidates),
	}
	if delivery.Mode == retrieve.ModeBytes {
		out["mime_type"] = delivery.MimeType
		out["bytes_base64"] = base64.StdEncoding.EncodeToString(delivery.Bytes)
	}
	return json.Marshal(out)
}

func handleDriveSaveFile(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	svc, err := loadServices()
	if err != nil {
		return notConfiguredOutput("drive_save_file")
	}
	if svc.SaveService == nil || svc.RulesRepo == nil || svc.RulesEngine == nil || svc.Policy == nil {
		return notConfiguredOutput("drive_save_file")
	}
	var input SaveRequestInput
	if err := json.Unmarshal(args, &input); err != nil {
		return marshalErr("drive_save_file", err)
	}
	body, err := base64.StdEncoding.DecodeString(input.ContentBase64)
	if err != nil && input.ContentBase64 != "" {
		return marshalErr("drive_save_file", fmt.Errorf("decode content_base64: %w", err))
	}

	// Pre-flight policy check: block sensitive shareable saves before
	// the Save Service runs. The Save Service applies its own
	// confirmation/idempotency logic; the agent surface adds an
	// explicit policy refusal so the LLM cannot smuggle sensitive
	// content through this path even if the rule guardrails are
	// misconfigured.
	//
	// We pass WouldCreateLink: true because the agent has no way to
	// guarantee a save will not create a provider-side link (the rule
	// guardrails decide), so the conservative, fail-loud assumption is
	// that it could. With WouldCreateLink=true the policy engine
	// refuses every sensitive save (financial, medical, identity)
	// regardless of which rule the rules engine would have selected.
	verdict, err := svc.Policy.Evaluate(policy.Action{
		Surface:         policy.SurfaceSaveLinkShare,
		Sensitivity:     policy.Sensitivity(strings.ToLower(input.Sensitivity)),
		WouldCreateLink: true,
	})
	if err != nil {
		return marshalErr("drive_save_file", err)
	}
	if verdict.Decision == policy.DecisionRefuse {
		return json.Marshal(map[string]any{
			"ok":            true,
			"saved":         false,
			"skipped":       true,
			"policy_reason": verdict.Reason,
			"reason":        "policy_refuse",
		})
	}

	allRules, err := svc.RulesRepo.List(ctx)
	if err != nil {
		return marshalErr("drive_save_file", err)
	}
	decision := svc.RulesEngine.Evaluate(ctx, rules.Artifact{
		ID:             input.ArtifactID,
		SourceKind:     string(rules.SourceMobile), // agent-driven saves default to mobile source
		Classification: input.Classification,
		Sensitivity:    input.Sensitivity,
		Confidence:     input.Confidence,
		Tokens:         input.Tokens,
	}, allRules)
	if decision.Selected == nil {
		return json.Marshal(map[string]any{
			"ok":      true,
			"saved":   false,
			"skipped": true,
			"reason":  "no_rule_matched",
		})
	}
	var rule rules.Rule
	for _, r := range allRules {
		if r.ID == decision.Selected.RuleID {
			rule = r
			break
		}
	}
	if rule.ID == "" {
		return marshalErr("drive_save_file", errors.New("matched rule missing from repository"))
	}
	res, err := svc.SaveService.Save(ctx, save.Request{
		Rule:             rule,
		SourceArtifactID: input.ArtifactID,
		ConfirmRequired:  decision.Selected.ConfirmRequired,
		RenderedPath:     decision.Selected.RenderedPath,
		Bytes: save.Bytes{
			Title:    input.Title,
			MimeType: input.MimeType,
			Body:     body,
		},
	})
	if err != nil {
		return marshalErr("drive_save_file", err)
	}
	return json.Marshal(map[string]any{
		"ok":           true,
		"saved":        res.Status == save.StatusWritten,
		"skipped":      res.Status == save.StatusSkipped,
		"rule_id":      rule.ID,
		"folder":       res.TargetPath,
		"provider_url": res.ProviderURL,
		"reason":       string(res.Status),
	})
}

func handleDriveListRules(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	svc, err := loadServices()
	if err != nil {
		return notConfiguredOutput("drive_list_rules")
	}
	if svc.RulesRepo == nil {
		return notConfiguredOutput("drive_list_rules")
	}
	all, err := svc.RulesRepo.List(ctx)
	if err != nil {
		return marshalErr("drive_list_rules", err)
	}
	rows := make([]map[string]any, 0, len(all))
	for _, r := range all {
		rows = append(rows, map[string]any{
			"id":                     r.ID,
			"name":                   r.Name,
			"enabled":                r.Enabled,
			"source_kinds":           append([]string{}, r.SourceKinds...),
			"classification":         r.Classification,
			"sensitivity_in":         append([]string{}, r.SensitivityIn...),
			"confidence_min":         r.ConfidenceMin,
			"provider_id":            r.ProviderID,
			"target_folder_template": r.TargetFolderTemplate,
		})
	}
	return json.Marshal(map[string]any{
		"ok":    true,
		"rules": rows,
	})
}

func candidatesPayload(in []retrieve.RetrieveCandidate) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, c := range in {
		out = append(out, map[string]any{
			"artifact_id":  c.ArtifactID,
			"title":        c.Title,
			"folder":       c.Folder,
			"provider":     c.Provider,
			"sensitivity":  c.Sensitivity,
			"size_bytes":   c.SizeBytes,
			"provider_url": c.ProviderURL,
		})
	}
	return out
}

func marshalErr(tool string, err error) (json.RawMessage, error) {
	payload := map[string]any{"ok": false, "error": err.Error()}
	if tool == "drive_get_file" {
		payload["mode"] = string(retrieve.ModeRefused)
	}
	if tool == "drive_list_rules" {
		payload["rules"] = []any{}
	}
	return json.Marshal(payload)
}
