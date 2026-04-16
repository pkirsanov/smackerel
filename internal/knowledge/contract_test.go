package knowledge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadContract_ValidIngestSynthesis(t *testing.T) {
	// Use the real prompt contract file from the repo.
	contractPath := filepath.Join("..", "..", "config", "prompt_contracts", "ingest-synthesis-v1.yaml")
	if _, err := os.Stat(contractPath); os.IsNotExist(err) {
		t.Fatalf("contract file not found at %s", contractPath)
	}

	contract, err := LoadContract(contractPath)
	if err != nil {
		t.Fatalf("LoadContract returned error: %v", err)
	}

	if contract.Version != "ingest-synthesis-v1" {
		t.Errorf("expected version 'ingest-synthesis-v1', got %q", contract.Version)
	}
	if contract.Type != "ingest-synthesis" {
		t.Errorf("expected type 'ingest-synthesis', got %q", contract.Type)
	}
	if contract.Description == "" {
		t.Error("expected non-empty description")
	}
	if contract.SystemPrompt == "" {
		t.Error("expected non-empty system_prompt")
	}

	// ExtractionSchema should be valid JSON.
	if len(contract.ExtractionSchema) == 0 {
		t.Fatal("expected non-empty extraction_schema")
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(contract.ExtractionSchema, &schema); err != nil {
		t.Fatalf("extraction_schema is not valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("extraction_schema type should be 'object', got %v", schema["type"])
	}

	// ValidationRules
	if contract.ValidationRules.MaxConcepts != 10 {
		t.Errorf("expected MaxConcepts=10, got %d", contract.ValidationRules.MaxConcepts)
	}
	if contract.ValidationRules.MaxEntities != 20 {
		t.Errorf("expected MaxEntities=20, got %d", contract.ValidationRules.MaxEntities)
	}
	if contract.ValidationRules.MaxRelationships != 30 {
		t.Errorf("expected MaxRelationships=30, got %d", contract.ValidationRules.MaxRelationships)
	}
}

func TestLoadContract_ValidCrossSource(t *testing.T) {
	contractPath := filepath.Join("..", "..", "config", "prompt_contracts", "cross-source-connection-v1.yaml")
	if _, err := os.Stat(contractPath); os.IsNotExist(err) {
		t.Fatalf("contract file not found at %s", contractPath)
	}

	contract, err := LoadContract(contractPath)
	if err != nil {
		t.Fatalf("LoadContract returned error: %v", err)
	}

	if contract.Version != "cross-source-connection-v1" {
		t.Errorf("expected version 'cross-source-connection-v1', got %q", contract.Version)
	}
	if contract.Type != "cross-source-connection" {
		t.Errorf("expected type 'cross-source-connection', got %q", contract.Type)
	}
	if contract.SystemPrompt == "" {
		t.Error("expected non-empty system_prompt")
	}

	if len(contract.ExtractionSchema) == 0 {
		t.Fatal("expected non-empty extraction_schema")
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(contract.ExtractionSchema, &schema); err != nil {
		t.Fatalf("extraction_schema is not valid JSON: %v", err)
	}
}

func TestLoadContract_InvalidYAML(t *testing.T) {
	data := []byte("{{invalid yaml}}")
	_, err := ParseContract(data)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadContract_MissingRequiredFields(t *testing.T) {
	data := []byte(`
version: ""
type: ""
description: ""
system_prompt: ""
extraction_schema:
  type: object
validation_rules:
  max_concepts: 0
  max_entities: 0
  max_relationships: 0
`)
	_, err := ParseContract(data)
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
}

func TestLoadContract_MissingExtractionSchema(t *testing.T) {
	data := []byte(`
version: "test-v1"
type: "test"
description: "A test contract"
system_prompt: "You are a test"
validation_rules:
  max_concepts: 10
  max_entities: 20
  max_relationships: 30
`)
	_, err := ParseContract(data)
	if err == nil {
		t.Fatal("expected error for missing extraction_schema")
	}
}

func TestLoadContract_InvalidExtractionSchema(t *testing.T) {
	data := []byte(`
version: "test-v1"
type: "test"
description: "A test contract"
system_prompt: "You are a test"
extraction_schema: "not-a-json-object"
validation_rules:
  max_concepts: 10
  max_entities: 20
  max_relationships: 30
`)
	_, err := ParseContract(data)
	if err == nil {
		t.Fatal("expected error for invalid extraction_schema")
	}
}

func TestLoadContractsFromDir(t *testing.T) {
	contractDir := filepath.Join("..", "..", "config", "prompt_contracts")
	if _, err := os.Stat(contractDir); os.IsNotExist(err) {
		t.Fatalf("contract directory not found at %s", contractDir)
	}

	contracts, err := LoadContractsFromDir(contractDir)
	if err != nil {
		t.Fatalf("LoadContractsFromDir returned error: %v", err)
	}

	if len(contracts) < 2 {
		t.Errorf("expected at least 2 contracts, got %d", len(contracts))
	}

	if _, ok := contracts["ingest-synthesis-v1"]; !ok {
		t.Error("missing ingest-synthesis-v1 contract")
	}
	if _, ok := contracts["cross-source-connection-v1"]; !ok {
		t.Error("missing cross-source-connection-v1 contract")
	}
}

func TestLoadContract_NonexistentFile(t *testing.T) {
	_, err := LoadContract("/nonexistent/path/contract.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
