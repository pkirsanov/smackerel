package knowledge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PromptContract defines the schema for a versioned prompt contract loaded from YAML.
type PromptContract struct {
	Version          string          `yaml:"version"          json:"version"`
	Type             string          `yaml:"type"             json:"type"`
	Description      string          `yaml:"description"      json:"description"`
	SystemPrompt     string          `yaml:"system_prompt"    json:"system_prompt"`
	ExtractionSchema json.RawMessage `yaml:"-"                json:"extraction_schema"`
	RawSchema        interface{}     `yaml:"extraction_schema" json:"-"`
	ValidationRules  ValidationRules `yaml:"validation_rules" json:"validation_rules"`
	TokenBudget      int             `yaml:"token_budget"     json:"token_budget"`
	Temperature      float64         `yaml:"temperature"      json:"temperature"`
	ModelPreference  string          `yaml:"model_preference" json:"model_preference"`
}

// ValidationRules defines the limits enforced on LLM extraction output.
type ValidationRules struct {
	MaxConcepts       int `yaml:"max_concepts"       json:"max_concepts"`
	MaxEntities       int `yaml:"max_entities"       json:"max_entities"`
	MaxRelationships  int `yaml:"max_relationships"  json:"max_relationships"`
	MaxContradictions int `yaml:"max_contradictions"  json:"max_contradictions"`
}

// LoadContract reads a prompt contract from a YAML file and validates required fields.
func LoadContract(path string) (*PromptContract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read contract file %s: %w", path, err)
	}
	return ParseContract(data)
}

// ParseContract parses a prompt contract from YAML bytes and validates required fields.
func ParseContract(data []byte) (*PromptContract, error) {
	var c PromptContract
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse contract YAML: %w", err)
	}

	if err := validateContractFields(&c); err != nil {
		return nil, err
	}

	// Convert the raw schema (parsed as map by YAML) to JSON for JSON Schema use.
	if c.RawSchema != nil {
		schemaJSON, err := json.Marshal(c.RawSchema)
		if err != nil {
			return nil, fmt.Errorf("marshal extraction_schema to JSON: %w", err)
		}
		c.ExtractionSchema = schemaJSON
	}

	// Validate that ExtractionSchema is valid JSON object.
	if len(c.ExtractionSchema) == 0 {
		return nil, fmt.Errorf("extraction_schema is required")
	}
	var schemaObj map[string]interface{}
	if err := json.Unmarshal(c.ExtractionSchema, &schemaObj); err != nil {
		return nil, fmt.Errorf("extraction_schema is not valid JSON: %w", err)
	}

	return &c, nil
}

// LoadContractsFromDir loads all prompt contracts from a directory.
// Returns a map keyed by contract version string.
func LoadContractsFromDir(dir string) (map[string]*PromptContract, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read contracts directory %s: %w", dir, err)
	}

	contracts := make(map[string]*PromptContract)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		contract, err := LoadContract(filepath.Join(dir, entry.Name()))
		if err != nil {
			// Skip contracts with different types (e.g., domain-extraction)
			// that share the same contracts directory but have different schemas.
			continue
		}
		contracts[contract.Version] = contract
	}
	return contracts, nil
}

func validateContractFields(c *PromptContract) error {
	var missing []string

	if c.Version == "" {
		missing = append(missing, "version")
	}
	if c.Type == "" {
		missing = append(missing, "type")
	}
	if c.Description == "" {
		missing = append(missing, "description")
	}
	if c.SystemPrompt == "" {
		missing = append(missing, "system_prompt")
	}
	if c.ValidationRules.MaxConcepts == 0 && c.ValidationRules.MaxEntities == 0 &&
		c.ValidationRules.MaxRelationships == 0 {
		missing = append(missing, "validation_rules (all limits are zero)")
	}

	if len(missing) > 0 {
		return fmt.Errorf("prompt contract missing required fields: %v", missing)
	}
	return nil
}
