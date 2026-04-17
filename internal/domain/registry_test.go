package domain

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempContract(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRegistry_LoadsDomainContracts(t *testing.T) {
	dir := t.TempDir()

	writeTempContract(t, dir, "recipe-extraction-v1.yaml", `
version: "recipe-extraction-v1"
type: "domain-extraction"
description: "Extract recipe data"
content_types:
  - "recipe"
url_qualifiers:
  - "allrecipes"
  - "epicurious"
min_content_length: 200
system_prompt: "Extract recipe data"
`)

	writeTempContract(t, dir, "product-extraction-v1.yaml", `
version: "product-extraction-v1"
type: "domain-extraction"
description: "Extract product data"
content_types:
  - "product"
url_qualifiers:
  - "amazon"
min_content_length: 100
system_prompt: "Extract product data"
`)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	if reg.Count() != 2 {
		t.Fatalf("expected 2 contracts, got %d", reg.Count())
	}
}

func TestLoadRegistry_SkipsNonDomainContracts(t *testing.T) {
	dir := t.TempDir()

	writeTempContract(t, dir, "ingest-synthesis-v1.yaml", `
version: "ingest-synthesis-v1"
type: "ingest-synthesis"
description: "Not a domain contract"
`)

	writeTempContract(t, dir, "recipe-extraction-v1.yaml", `
version: "recipe-extraction-v1"
type: "domain-extraction"
content_types:
  - "recipe"
system_prompt: "Extract recipe data"
`)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	if reg.Count() != 1 {
		t.Fatalf("expected 1 domain contract (skipping non-domain), got %d", reg.Count())
	}
}

func TestLoadRegistry_RejectsDuplicateContentType(t *testing.T) {
	dir := t.TempDir()

	writeTempContract(t, dir, "recipe-v1.yaml", `
version: "recipe-v1"
type: "domain-extraction"
content_types:
  - "recipe"
system_prompt: "test"
`)

	writeTempContract(t, dir, "recipe-v2.yaml", `
version: "recipe-v2"
type: "domain-extraction"
content_types:
  - "recipe"
system_prompt: "test"
`)

	_, err := LoadRegistry(dir)
	if err == nil {
		t.Fatal("expected error for duplicate content_type")
	}
	if !contains(err.Error(), "duplicate content_type") {
		t.Fatalf("expected duplicate error, got: %v", err)
	}
}

func TestLoadRegistry_RejectsEmptyVersion(t *testing.T) {
	dir := t.TempDir()

	writeTempContract(t, dir, "bad.yaml", `
type: "domain-extraction"
content_types:
  - "recipe"
system_prompt: "test"
`)

	_, err := LoadRegistry(dir)
	if err == nil {
		t.Fatal("expected error for empty version")
	}
}

func TestLoadRegistry_RejectsEmptyContentTypes(t *testing.T) {
	dir := t.TempDir()

	writeTempContract(t, dir, "bad.yaml", `
version: "bad-v1"
type: "domain-extraction"
system_prompt: "test"
`)

	_, err := LoadRegistry(dir)
	if err == nil {
		t.Fatal("expected error for empty content_types")
	}
}

func TestMatch_ByContentType(t *testing.T) {
	dir := t.TempDir()

	writeTempContract(t, dir, "recipe.yaml", `
version: "recipe-extraction-v1"
type: "domain-extraction"
content_types:
  - "recipe"
system_prompt: "test"
`)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}

	c := reg.Match("recipe", "")
	if c == nil {
		t.Fatal("expected match for content_type=recipe")
	}
	if c.Version != "recipe-extraction-v1" {
		t.Fatalf("expected recipe-extraction-v1, got %s", c.Version)
	}
}

func TestMatch_ByURLQualifier(t *testing.T) {
	dir := t.TempDir()

	writeTempContract(t, dir, "recipe.yaml", `
version: "recipe-extraction-v1"
type: "domain-extraction"
content_types:
  - "recipe"
url_qualifiers:
  - "allrecipes"
  - "epicurious"
system_prompt: "test"
`)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}

	// URL qualifier match for non-matching content type
	c := reg.Match("article", "https://www.allrecipes.com/recipe/123")
	if c == nil {
		t.Fatal("expected URL qualifier match for allrecipes URL")
	}
	if c.Version != "recipe-extraction-v1" {
		t.Fatalf("expected recipe-extraction-v1, got %s", c.Version)
	}
}

func TestMatch_ContentTypePriorityOverURL(t *testing.T) {
	dir := t.TempDir()

	writeTempContract(t, dir, "recipe.yaml", `
version: "recipe-extraction-v1"
type: "domain-extraction"
content_types:
  - "recipe"
url_qualifiers:
  - "allrecipes"
system_prompt: "test"
`)

	writeTempContract(t, dir, "product.yaml", `
version: "product-extraction-v1"
type: "domain-extraction"
content_types:
  - "product"
url_qualifiers:
  - "amazon"
system_prompt: "test"
`)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}

	// content_type "recipe" should match even if URL contains "amazon"
	c := reg.Match("recipe", "https://amazon.com/recipe-book")
	if c == nil {
		t.Fatal("expected match")
	}
	if c.Version != "recipe-extraction-v1" {
		t.Fatalf("content_type should take priority, got %s", c.Version)
	}
}

func TestMatch_NoMatch(t *testing.T) {
	dir := t.TempDir()

	writeTempContract(t, dir, "recipe.yaml", `
version: "recipe-extraction-v1"
type: "domain-extraction"
content_types:
  - "recipe"
system_prompt: "test"
`)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}

	c := reg.Match("article", "https://example.com/news")
	if c != nil {
		t.Fatal("expected no match for unregistered content type and URL")
	}
}

func TestMatch_NilRegistry(t *testing.T) {
	var reg *Registry
	c := reg.Match("recipe", "")
	if c != nil {
		t.Fatal("expected nil from nil registry")
	}
}

func TestCount_NilRegistry(t *testing.T) {
	var reg *Registry
	if reg.Count() != 0 {
		t.Fatal("expected 0 from nil registry")
	}
}

func TestLoadRegistry_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("empty dir should not error: %v", err)
	}
	if reg.Count() != 0 {
		t.Fatalf("expected 0 contracts from empty dir, got %d", reg.Count())
	}
}

func TestLoadRegistry_SkipsNonYAMLFiles(t *testing.T) {
	dir := t.TempDir()

	writeTempContract(t, dir, "readme.txt", "not yaml")
	writeTempContract(t, dir, "recipe.yaml", `
version: "recipe-extraction-v1"
type: "domain-extraction"
content_types:
  - "recipe"
system_prompt: "test"
`)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Count() != 1 {
		t.Fatalf("expected 1, got %d", reg.Count())
	}
}

func TestMatch_URLQualifier_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()

	writeTempContract(t, dir, "recipe.yaml", `
version: "recipe-extraction-v1"
type: "domain-extraction"
content_types:
  - "recipe"
url_qualifiers:
  - "AllRecipes"
system_prompt: "test"
`)

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}

	c := reg.Match("article", "https://www.ALLRECIPES.COM/recipe/123")
	if c == nil {
		t.Fatal("expected case-insensitive URL match")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
