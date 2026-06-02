package transportconfig

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// docRow mirrors the columns rendered in
// docs/Transport_Configuration.md so the test can assert set equality
// with Registry without coupling to row ordering or section headings.
type docRow struct {
	yamlKey       string
	envVar        string
	required      bool
	owningPackage string
}

// parseTransportConfigDoc reads docs/Transport_Configuration.md and
// extracts every table row whose first column is a YAML key under one
// of the declared TransportNamespaces. The doc format is:
//
//	| YAML Key | Env Var | Required | Owning Package |
//
// Cells are wrapped in backticks; required is "yes"|"no".
func parseTransportConfigDoc(t *testing.T) []docRow {
	t.Helper()
	root := repoRoot(t)
	docPath := filepath.Join(root, "docs", "Transport_Configuration.md")
	f, err := os.Open(docPath)
	if err != nil {
		t.Fatalf("open %s: %v", docPath, err)
	}
	defer f.Close()

	var rows []docRow
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "|") {
			continue
		}
		// Split on '|' and trim. A markdown table row with 4 columns
		// has 6 fragments (leading + trailing empties).
		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}
		cells := make([]string, 0, 4)
		for _, p := range parts[1 : len(parts)-1] {
			cells = append(cells, strings.TrimSpace(p))
		}
		if len(cells) != 4 {
			continue
		}
		// Skip header and divider rows.
		if cells[0] == "YAML Key" || strings.HasPrefix(cells[0], "---") || strings.HasPrefix(cells[0], ":-") {
			continue
		}
		yamlKey := stripBackticks(cells[0])
		// Only rows whose first cell is a real per-transport key.
		if !belongsToTransport(yamlKey) {
			continue
		}
		row := docRow{
			yamlKey:       yamlKey,
			envVar:        stripBackticks(cells[1]),
			owningPackage: stripBackticks(cells[3]),
		}
		switch strings.ToLower(cells[2]) {
		case "yes":
			row.required = true
		case "no":
			row.required = false
		default:
			t.Fatalf("docs/Transport_Configuration.md line %d: Required column for %q must be \"yes\" or \"no\", got %q",
				lineNo, yamlKey, cells[2])
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", docPath, err)
	}
	return rows
}

func stripBackticks(s string) string {
	return strings.Trim(s, "`")
}

func belongsToTransport(key string) bool {
	for _, ns := range TransportNamespaces {
		if key == ns || strings.HasPrefix(key, ns+".") {
			return true
		}
	}
	return false
}

// SCN-062-A06: docs/Transport_Configuration.md MUST render exactly one
// row per Registry entry, with matching env-var, required-flag, and
// owning-package columns. Any drift fails the build.
func TestRegistry_DocSync(t *testing.T) {
	docRows := parseTransportConfigDoc(t)

	docByKey := map[string]docRow{}
	for _, r := range docRows {
		if existing, dup := docByKey[r.yamlKey]; dup {
			t.Fatalf("docs/Transport_Configuration.md has duplicate row for %q (first=%+v, second=%+v)",
				r.yamlKey, existing, r)
		}
		docByKey[r.yamlKey] = r
	}

	regByKey := map[string]Entry{}
	for _, e := range Registry {
		regByKey[e.YAMLKey] = e
	}

	var missingFromDoc, extraInDoc, mismatched []string

	for key, e := range regByKey {
		dr, ok := docByKey[key]
		if !ok {
			missingFromDoc = append(missingFromDoc, key)
			continue
		}
		var issues []string
		if dr.envVar != e.EnvVar {
			issues = append(issues, fmt.Sprintf("envVar doc=%q reg=%q", dr.envVar, e.EnvVar))
		}
		if dr.required != e.Required {
			issues = append(issues, fmt.Sprintf("required doc=%v reg=%v", dr.required, e.Required))
		}
		if dr.owningPackage != e.OwningPackage {
			issues = append(issues, fmt.Sprintf("owningPackage doc=%q reg=%q", dr.owningPackage, e.OwningPackage))
		}
		if len(issues) > 0 {
			mismatched = append(mismatched, fmt.Sprintf("%s: %s", key, strings.Join(issues, "; ")))
		}
	}
	for key := range docByKey {
		if _, ok := regByKey[key]; !ok {
			extraInDoc = append(extraInDoc, key)
		}
	}

	sort.Strings(missingFromDoc)
	sort.Strings(extraInDoc)
	sort.Strings(mismatched)

	if len(missingFromDoc) > 0 || len(extraInDoc) > 0 || len(mismatched) > 0 {
		var b strings.Builder
		b.WriteString("docs/Transport_Configuration.md is out of sync with transportconfig.Registry.\n")
		b.WriteString("Update the doc in the same commit that touches the registry.\n")
		if len(missingFromDoc) > 0 {
			b.WriteString("\nRegistry entries missing from doc:\n  ")
			b.WriteString(strings.Join(missingFromDoc, "\n  "))
			b.WriteString("\n")
		}
		if len(extraInDoc) > 0 {
			b.WriteString("\nDoc rows with no matching registry entry:\n  ")
			b.WriteString(strings.Join(extraInDoc, "\n  "))
			b.WriteString("\n")
		}
		if len(mismatched) > 0 {
			b.WriteString("\nColumn mismatches:\n  ")
			b.WriteString(strings.Join(mismatched, "\n  "))
			b.WriteString("\n")
		}
		t.Fatal(b.String())
	}
}
