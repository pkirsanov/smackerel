package transportconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// repoRoot walks up from this source file to find the repo root
// (identified by go.mod). The registry tests load
// config/smackerel.yaml directly off disk so the SST relationship
// is asserted on real source rather than a fixture.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("repo root not found (no go.mod above transportconfig/)")
	return ""
}

// loadYAMLLeafKeys returns every dotted leaf key in
// config/smackerel.yaml that lives under any namespace in
// TransportNamespaces.
func loadYAMLLeafKeys(t *testing.T) map[string]struct{} {
	t.Helper()
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "config", "smackerel.yaml"))
	if err != nil {
		t.Fatalf("read config/smackerel.yaml: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal config/smackerel.yaml: %v", err)
	}
	leaves := map[string]struct{}{}
	for _, ns := range TransportNamespaces {
		node := walkPath(doc, strings.Split(ns, "."))
		if node == nil {
			t.Fatalf("YAML namespace %q not found in config/smackerel.yaml", ns)
		}
		collectLeaves(node, ns, leaves)
	}
	return leaves
}

// walkPath descends a parsed YAML tree using dotted-path segments.
func walkPath(node any, segments []string) any {
	for _, seg := range segments {
		m, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		node, ok = m[seg]
		if !ok {
			return nil
		}
	}
	return node
}

// collectLeaves records the dotted path of every scalar/list leaf.
// Nested maps are descended; sequences are treated as leaves
// (matching how scripts/commands/config.sh emits CSV/JSON for lists).
func collectLeaves(node any, prefix string, out map[string]struct{}) {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			collectLeaves(child, prefix+"."+k, out)
		}
	default:
		out[prefix] = struct{}{}
	}
}

// SCN-062-A01: every per-transport key present in config/smackerel.yaml
// under a registered namespace has a registry entry.
func TestRegistry_CoversYAMLNamespaces(t *testing.T) {
	yamlKeys := loadYAMLLeafKeys(t)
	registered := map[string]struct{}{}
	for _, e := range Registry {
		registered[e.YAMLKey] = struct{}{}
	}
	var missing []string
	for k := range yamlKeys {
		if _, ok := registered[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("YAML keys in config/smackerel.yaml under TransportNamespaces have no transportconfig.Registry entry:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// SCN-062-A02: every registry entry maps to a key present in
// config/smackerel.yaml under one of the declared namespaces.
func TestRegistry_NoOrphanedEntries(t *testing.T) {
	yamlKeys := loadYAMLLeafKeys(t)
	var orphans []string
	for _, e := range Registry {
		if _, ok := yamlKeys[e.YAMLKey]; !ok {
			orphans = append(orphans, e.YAMLKey)
		}
		// Sanity: every entry's YAMLKey must lie under a declared namespace.
		matched := false
		for _, ns := range TransportNamespaces {
			if e.YAMLKey == ns || strings.HasPrefix(e.YAMLKey, ns+".") {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("registry entry %q is outside any declared TransportNamespaces", e.YAMLKey)
		}
	}
	if len(orphans) > 0 {
		sort.Strings(orphans)
		t.Fatalf("transportconfig.Registry entries reference YAML keys absent from config/smackerel.yaml:\n  %s",
			strings.Join(orphans, "\n  "))
	}
}
