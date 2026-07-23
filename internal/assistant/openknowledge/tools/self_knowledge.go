package tools

// self_knowledge.go — spec 104 SCOPE-04.
//
// The self_knowledge tool answers questions about smackerel ITSELF — its
// skills/scenarios, slash commands, recipes, and features — by semantically
// searching the product's own capability corpus (the "smackerel_self"
// namespace, ingested from the live SSTs by internal/assistant/selfknowledge).
// It is a normal openknowledge.Tool (same contract as internal_retrieval) and
// returns cited Source{Kind:SourceArtifact} entries that pass the cite-back
// verifier + provenance gate, so a product meta-answer is GROUNDED, never a
// hallucination. When nothing matches, the agent's honest refusal
// (BUG-061-009) is the fallback.
//
// The namespace is injected at construction (cmd/core passes
// selfknowledge.SelfKnowledgeNamespace) so this leaf tool package does not
// import the selfknowledge package.

import (
	"context"
	"encoding/json"
	"strings"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

// SelfKnowledgeToolName is the registry key + allowlist entry for the
// self_knowledge tool. cmd/core force-adds it to the effective allowlist so
// self-knowledge is always-on (FR-1: not operator-disableable to off).
const SelfKnowledgeToolName = "self_knowledge"

const selfKnowledgeSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["query", "k"],
  "properties": {
    "query": {"type": "string", "minLength": 1},
    "k": {"type": "integer", "minimum": 1, "maximum": 25}
  }
}`

// Typed sentinel errors returned via ToolResult.Error.
var (
	ErrSelfKnowledgeMalformed = &ok.ToolError{Code: "malformed_params", Message: "params do not match schema"}
	ErrSelfKnowledgeQuery     = &ok.ToolError{Code: "invalid_query", Message: "query must be non-empty after trim"}
	ErrSelfKnowledgeK         = &ok.ToolError{Code: "invalid_k", Message: "k must be > 0 and <= MaxInternalRetrievalK"}
	ErrSelfKnowledgeBackend   = &ok.ToolError{Code: "backend_failure", Message: "self-knowledge searcher returned an error"}
	ErrSelfKnowledgeArtifact  = &ok.ToolError{Code: "invalid_artifact", Message: "searcher returned an artifact with empty id"}
)

type selfKnowledgeParams struct {
	Query *string `json:"query"`
	K     *int    `json:"k"`
}

// SelfKnowledge is the registry-facing tool handle. It delegates to a
// SemanticSearcher bound to the smackerel_self namespace.
type SelfKnowledge struct {
	searcher  SemanticSearcher
	namespace string
}

// NewSelfKnowledge constructs the tool around a namespace-scoped searcher.
// Both are required (panic on nil/empty — G028 no silent no-op).
func NewSelfKnowledge(searcher SemanticSearcher, namespace string) *SelfKnowledge {
	if searcher == nil {
		panic("openknowledge: self_knowledge requires a non-nil SemanticSearcher")
	}
	if strings.TrimSpace(namespace) == "" {
		panic("openknowledge: self_knowledge requires a non-empty namespace")
	}
	return &SelfKnowledge{searcher: searcher, namespace: namespace}
}

// Name reports the registry key.
func (SelfKnowledge) Name() string { return SelfKnowledgeToolName }

// Description summarises the tool for the planner prompt.
func (SelfKnowledge) Description() string {
	return "Answer questions about smackerel itself — what it can do, its skills and scenarios, slash commands, recipes, and features. Use this for any question about the product, its abilities, or how to use it. Returns Source entries citing smackerel's own capability registry (Kind=SourceArtifact)."
}

// ParamsSchema returns the JSONSchema for Execute params.
func (SelfKnowledge) ParamsSchema() json.RawMessage {
	return json.RawMessage(selfKnowledgeSchema)
}

// Execute validates params, searches the smackerel_self namespace, and maps
// results to the canonical ToolResult envelope (same shape as internal_retrieval,
// so the cite-back verifier + renderers accept it unchanged).
func (t *SelfKnowledge) Execute(ctx context.Context, params json.RawMessage) (*ok.ToolResult, error) {
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p selfKnowledgeParams
	if err := dec.Decode(&p); err != nil {
		return &ok.ToolResult{Error: ErrSelfKnowledgeMalformed}, nil
	}
	if p.Query == nil || p.K == nil {
		return &ok.ToolResult{Error: ErrSelfKnowledgeMalformed}, nil
	}
	query := strings.TrimSpace(*p.Query)
	if query == "" {
		return &ok.ToolResult{Error: ErrSelfKnowledgeQuery}, nil
	}
	k := *p.K
	if k <= 0 || k > MaxInternalRetrievalK {
		return &ok.ToolResult{Error: ErrSelfKnowledgeK}, nil
	}

	artifacts, err := t.searcher.Search(ctx, t.namespace, query, k)
	if err != nil {
		return &ok.ToolResult{Error: &ok.ToolError{
			Code:    ErrSelfKnowledgeBackend.Code,
			Message: ErrSelfKnowledgeBackend.Message + ": " + err.Error(),
		}}, nil
	}

	snippets := make([]ok.Snippet, 0, len(artifacts))
	sources := make([]ok.Source, 0, len(artifacts))
	for _, a := range artifacts {
		if strings.TrimSpace(a.ID) == "" {
			return &ok.ToolResult{Error: ErrSelfKnowledgeArtifact}, nil
		}
		text := canonicalSnippetText(a.Title, a.Summary)
		snippets = append(snippets, ok.Snippet{
			Text:        text,
			ContentHash: snippetHash(text),
			SourceRef:   a.ID,
		})
		sources = append(sources, ok.Source{
			Kind: ok.SourceArtifact,
			Artifact: &ok.ArtifactRef{
				ID:    a.ID,
				Kind:  "capability",
				Title: a.Title,
			},
		})
	}
	return &ok.ToolResult{Snippets: snippets, Sources: sources}, nil
}
