package agent

import "encoding/json"

// ScenarioForTest is the input shape for NewScenarioForTest. The
// constructor exists so integration tests in other packages can build
// a fully-formed *Scenario (including the unexported compiled JSON
// schemas) without round-tripping through a YAML file. Production code
// MUST go through DefaultLoader().Load().
type ScenarioForTest struct {
	ID              string
	Version         string
	Description     string
	IntentExamples  []string
	SystemPrompt    string
	AllowedTools    []AllowedTool
	InputSchema     json.RawMessage
	OutputSchema    json.RawMessage
	InputCompiled   *CompiledSchema
	OutputCompiled  *CompiledSchema
	Limits          ScenarioLimits
	TokenBudget     int
	Temperature     float64
	ModelPreference string
	SideEffectClass SideEffectClass
	ContentHash     string
	SourcePath      string
}

// NewScenarioForTest builds a fully-formed *Scenario from in-memory
// values. Test-only entrypoint; production code uses DefaultLoader().
func NewScenarioForTest(in ScenarioForTest) *Scenario {
	return &Scenario{
		ID:              in.ID,
		Version:         in.Version,
		Description:     in.Description,
		IntentExamples:  append([]string(nil), in.IntentExamples...),
		SystemPrompt:    in.SystemPrompt,
		AllowedTools:    append([]AllowedTool(nil), in.AllowedTools...),
		InputSchema:     in.InputSchema,
		OutputSchema:    in.OutputSchema,
		Limits:          in.Limits,
		TokenBudget:     in.TokenBudget,
		Temperature:     in.Temperature,
		ModelPreference: in.ModelPreference,
		SideEffectClass: in.SideEffectClass,
		ContentHash:     in.ContentHash,
		SourcePath:      in.SourcePath,
		inputSchema:     in.InputCompiled,
		outputSchema:    in.OutputCompiled,
	}
}
