// Package domain provides the domain extraction schema registry.
// It loads domain-extraction prompt contracts from YAML files and provides
// content-type-based lookup for the pipeline to dispatch domain extraction.
package domain

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DomainContract represents a domain extraction prompt contract loaded from YAML.
type DomainContract struct {
	Version          string   `yaml:"version"`
	Type             string   `yaml:"type"`
	Description      string   `yaml:"description"`
	ContentTypes     []string `yaml:"content_types"`
	URLQualifiers    []string `yaml:"url_qualifiers"`
	MinContentLen    int      `yaml:"min_content_length"`
	SystemPrompt     string   `yaml:"system_prompt"`
	ExtractionSchema any      `yaml:"extraction_schema"`
}

// Registry maps content types and URL patterns to domain extraction contracts.
type Registry struct {
	byContentType map[string]*DomainContract
	byURLPattern  []urlEntry
	contracts     []*DomainContract
}

type urlEntry struct {
	pattern  string
	contract *DomainContract
}

// LoadRegistry loads all domain-extraction prompt contracts from a directory.
// Files with type != "domain-extraction" are silently skipped.
// Duplicate content_type mappings are rejected with an error.
func LoadRegistry(contractsDir string) (*Registry, error) {
	reg := &Registry{
		byContentType: make(map[string]*DomainContract),
	}

	entries, err := os.ReadDir(contractsDir)
	if err != nil {
		return nil, fmt.Errorf("read contracts dir %s: %w", contractsDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}

		path := filepath.Join(contractsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read contract %s: %w", path, err)
		}

		var contract DomainContract
		if err := yaml.Unmarshal(data, &contract); err != nil {
			slog.Warn("skipping unparseable contract", "file", entry.Name(), "error", err)
			continue
		}

		if contract.Type != "domain-extraction" {
			continue // not a domain contract
		}

		if contract.Version == "" {
			return nil, fmt.Errorf("domain contract %s: version is required", entry.Name())
		}

		if len(contract.ContentTypes) == 0 {
			return nil, fmt.Errorf("domain contract %s: content_types is required", entry.Name())
		}

		// Register content type mappings — reject duplicates
		for _, ct := range contract.ContentTypes {
			if existing, ok := reg.byContentType[ct]; ok {
				return nil, fmt.Errorf(
					"duplicate content_type %q: claimed by both %q and %q",
					ct, existing.Version, contract.Version,
				)
			}
			c := contract // copy for pointer stability
			reg.byContentType[ct] = &c
		}

		// Register URL qualifier patterns
		c := contract
		for _, pattern := range contract.URLQualifiers {
			reg.byURLPattern = append(reg.byURLPattern, urlEntry{
				pattern:  strings.ToLower(pattern),
				contract: &c,
			})
		}

		reg.contracts = append(reg.contracts, &c)
		slog.Info("loaded domain contract",
			"version", contract.Version,
			"content_types", contract.ContentTypes,
			"url_qualifiers", len(contract.URLQualifiers),
		)
	}

	return reg, nil
}

// Match finds the domain contract for a given content type and source URL.
// Content type match takes priority. URL qualifier is a fallback for generic types.
func (r *Registry) Match(contentType, sourceURL string) *DomainContract {
	if r == nil {
		return nil
	}

	// Direct content type match
	if c, ok := r.byContentType[contentType]; ok {
		return c
	}

	// URL qualifier fallback — check if URL contains any registered pattern
	if sourceURL != "" {
		lower := strings.ToLower(sourceURL)
		for _, entry := range r.byURLPattern {
			if strings.Contains(lower, entry.pattern) {
				return entry.contract
			}
		}
	}

	return nil
}

// Count returns the number of loaded domain contracts.
func (r *Registry) Count() int {
	if r == nil {
		return 0
	}
	return len(r.contracts)
}

// Contracts returns all loaded domain contracts.
func (r *Registry) Contracts() []*DomainContract {
	if r == nil {
		return nil
	}
	return r.contracts
}
