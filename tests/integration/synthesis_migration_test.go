//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence"
)

// BUG-004-004 T004-01-MIGRATE / T004-01-ROLLBACK-COMPAT.
//
// Two properties that a passing feature test would not catch:
//
//  1. BOOTSTRAP CANARY. A brand-new database must end up with the full durable
//     shape after migration. The feature tests all run against a database that
//     already migrated, so they would keep passing if a migration silently
//     stopped shipping a table an operator's fresh install needs.
//
//  2. NON-DESTRUCTIVE. The synthesis migrations must only ADD. If one dropped or
//     rewrote a pre-existing table, rolling back would take an operator's data
//     with it, and no amount of green feature tests would reveal that.

func TestSynthesisMigration_BootstrapCanaryCreatesFullShape(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	// Every table the durable contract depends on. Naming them explicitly is the
	// point: a migration that quietly stopped creating one would fail here even
	// though the feature tests, running on an already-migrated database, would
	// not notice.
	for _, table := range []string{
		"synthesis_runs",
		"synthesis_run_attempts",
		"synthesis_outputs",
		"synthesis_output_insights",
		"synthesis_citations",
		"synthesis_output_source_classes",
	} {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table).Scan(&exists); err != nil {
			t.Fatalf("probe %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %s is absent after bootstrap; a fresh install would not work", table)
		}
	}

	// The columns that carry the meaning, not merely the table names. An output
	// without output_kind cannot distinguish quiet from full, which is the whole
	// point of SCN-07 and SCN-08.
	for _, col := range []string{"output_kind", "evaluated_artifact_count"} {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'synthesis_outputs' AND column_name = $1
			)`, col).Scan(&exists); err != nil {
			t.Fatalf("probe column %s: %v", col, err)
		}
		if !exists {
			t.Fatalf("synthesis_outputs.%s is absent; quiet and full outputs would be indistinguishable", col)
		}
	}

	// Idempotence is a database constraint, not application logic. If this index
	// vanished, the duplicate test would still pass whenever timing happened to
	// serialise the two runs -- and fail in production under real concurrency.
	var uniqueOnLogicalKey bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE tablename = 'synthesis_runs' AND indexdef ILIKE '%UNIQUE%logical_key%'
		)`).Scan(&uniqueOnLogicalKey); err != nil {
		t.Fatalf("probe unique index: %v", err)
	}
	if !uniqueOnLogicalKey {
		t.Fatal("synthesis_runs has no UNIQUE index on logical_key; idempotence would rest on a race, not a constraint")
	}
}

func TestSynthesisMigration_IsNonDestructive(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "db", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}

	// Pre-existing tables an operator already has data in. A synthesis migration
	// touching any of them destructively is the failure mode this guards.
	preExisting := []string{"artifacts", "topics", "edges", "synthesis_insights"}
	destructive := regexp.MustCompile(`(?i)\b(DROP\s+TABLE|TRUNCATE|DELETE\s+FROM|DROP\s+COLUMN)\b`)

	var checked int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "064_") && !strings.HasPrefix(name, "065_") {
			continue
		}
		checked++

		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		// Strip comments first -- prose describing what the migration deliberately
		// does NOT do would otherwise trip the scan and teach everyone to ignore it.
		var sql strings.Builder
		for _, line := range strings.Split(string(body), "\n") {
			if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "--") {
				continue
			}
			sql.WriteString(line)
			sql.WriteString("\n")
		}

		for _, stmt := range strings.Split(sql.String(), ";") {
			if !destructive.MatchString(stmt) {
				continue
			}
			for _, table := range preExisting {
				if regexp.MustCompile(`(?i)\b` + table + `\b`).MatchString(stmt) {
					t.Fatalf("%s contains a destructive statement against pre-existing table %q; rolling back would take operator data with it:\n%s",
						name, table, strings.TrimSpace(stmt))
				}
			}
		}
	}

	if checked != 2 {
		t.Fatalf("expected to check migrations 064 and 065, checked %d; a renamed migration would silently skip this guard", checked)
	}
}

// The pre-existing synthesis_insights table is LEGACY. The durable contract is
// the new tables, and a legacy row must never be mixed into a verified output --
// otherwise an operator reading "3 insights" could be seeing rows that never
// passed validation, citation checks, or read-back.
func TestSynthesisMigration_LegacyInsightsAreNotReadAsDurableOutput(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)
	seedSynthesisCluster(t, pool)
	ctx := context.Background()

	// A legacy row of exactly the kind the old producer would have left behind,
	// had it ever written one.
	// t.Fatalf, never t.Skipf: a skip here would turn "the legacy table changed
	// shape" into a silent pass, which is the failure mode this whole packet is
	// about.
	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_insights (id, insight_type, through_line, source_artifact_ids, confidence)
		VALUES ('legacy-insight-1', 'through_line', 'a legacy row that never passed validation', ARRAY['art-e2e-0'], 0.9)`); err != nil {
		t.Fatalf("seed legacy insight: %v", err)
	}

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	producer, err := intelligence.NewSynthesisProducer(&intelligence.Engine{Pool: pool}, persistence)
	if err != nil {
		t.Fatalf("construct producer: %v", err)
	}

	agg, err := producer.RunAndPersist(ctx, intelligence.CadenceDaily, "scheduler", time.Now().UTC())
	if err != nil {
		t.Fatalf("run and persist: %v", err)
	}

	for _, insight := range agg.Insights {
		if insight.ID == "legacy-insight-1" {
			t.Fatal("a legacy synthesis_insights row surfaced inside a verified output; legacy rows never passed validation or read-back and must stay out")
		}
		if insight.ThroughLine == "a legacy row that never passed validation" {
			t.Fatal("legacy content surfaced in a verified output")
		}
	}

	// The legacy row is still there -- classification is not deletion. An
	// operator's old data is untouched; it simply is not read as durable output.
	var legacyRows int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM synthesis_insights WHERE id = 'legacy-insight-1'`).Scan(&legacyRows); err != nil {
		t.Fatalf("count legacy rows: %v", err)
	}
	if legacyRows != 1 {
		t.Fatalf("legacy row count %d, want 1; classifying legacy data must not destroy it", legacyRows)
	}
}
