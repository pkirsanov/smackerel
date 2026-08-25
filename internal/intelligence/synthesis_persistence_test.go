package intelligence

import (
	"testing"
	"time"
)

// BUG-004-004 SCOPE-01 unit coverage for the logical key.
//
// The key is what makes a retry the same run, so these are the properties the
// idempotence claim rests on. They need no database: the derivation is pure.

func baseKey() SynthesisRunKey {
	return SynthesisRunKey{
		Cadence:       CadenceDaily,
		Principal:     "operator",
		WindowStart:   time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
		WindowEnd:     time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
		PolicyVersion: "v1",
		SourceIDs:     []string{"art-b", "art-a", "art-c"},
	}
}

func TestLogicalKey_IsDeterministic(t *testing.T) {
	if got, want := baseKey().LogicalKey(), baseKey().LogicalKey(); got != want {
		t.Fatalf("same inputs produced different keys:\n got %s\nwant %s", got, want)
	}
}

func TestLogicalKey_IgnoresSourceOrder(t *testing.T) {
	// The eligible source SET identifies the run. If order mattered, a query
	// plan change would silently split one logical run into two.
	a := baseKey()
	b := baseKey()
	b.SourceIDs = []string{"art-c", "art-a", "art-b"}
	if a.LogicalKey() != b.LogicalKey() {
		t.Fatalf("source order changed the key; the set must decide, not the ordering")
	}
}

func TestLogicalKey_IgnoresDuplicateSources(t *testing.T) {
	a := baseKey()
	b := baseKey()
	b.SourceIDs = []string{"art-a", "art-a", "art-b", "art-c", "art-c"}
	if a.LogicalKey() != b.LogicalKey() {
		t.Fatalf("duplicate source ids changed the key")
	}
}

func TestLogicalKey_NormalizesWindowToUTC(t *testing.T) {
	// A scheduler in a non-UTC location computes the same INSTANT. If the key
	// were location-sensitive it would produce a second run for the same window.
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable in this environment: %v", err)
	}
	a := baseKey()
	b := baseKey()
	b.WindowStart = a.WindowStart.In(loc)
	b.WindowEnd = a.WindowEnd.In(loc)
	if a.LogicalKey() != b.LogicalKey() {
		t.Fatalf("same instant in a different location produced a different key")
	}
}

func TestLogicalKey_DistinguishesEveryField(t *testing.T) {
	base := baseKey().LogicalKey()
	for name, mutate := range map[string]func(*SynthesisRunKey){
		"cadence":       func(k *SynthesisRunKey) { k.Cadence = CadenceWeekly },
		"principal":     func(k *SynthesisRunKey) { k.Principal = "someone-else" },
		"windowStart":   func(k *SynthesisRunKey) { k.WindowStart = k.WindowStart.Add(-time.Hour) },
		"windowEnd":     func(k *SynthesisRunKey) { k.WindowEnd = k.WindowEnd.Add(time.Hour) },
		"policyVersion": func(k *SynthesisRunKey) { k.PolicyVersion = "v2" },
		"sourceSet":     func(k *SynthesisRunKey) { k.SourceIDs = append(k.SourceIDs, "art-d") },
	} {
		t.Run(name, func(t *testing.T) {
			k := baseKey()
			mutate(&k)
			if k.LogicalKey() == base {
				t.Fatalf("changing %s did not change the logical key; runs that differ would collide", name)
			}
		})
	}
}

// ADVERSARIAL. This is the case the length-prefixing in LogicalKey exists for.
// Concatenating fields without a length prefix makes ("ab","c") and ("a","bc")
// hash identically, so two genuinely different runs would share one key and the
// second would be swallowed as an idempotent no-change. If the prefix is
// removed, this test fails.
func TestLogicalKey_FieldBoundariesCannotBeShifted(t *testing.T) {
	a := baseKey()
	a.Cadence = CadenceDaily
	a.Principal = "ab"

	b := baseKey()
	b.Cadence = CadenceDaily
	b.Principal = "ab"

	// Shift a character across the principal/policy boundary.
	a.PolicyVersion = "xy"
	b.Principal = "abx"
	b.PolicyVersion = "y"

	if a.LogicalKey() == b.LogicalKey() {
		t.Fatalf("field boundary collision: (%q,%q) and (%q,%q) produced the same key",
			a.Principal, a.PolicyVersion, b.Principal, b.PolicyVersion)
	}
}

func TestSourceSetDigest_IsOrderIndependent(t *testing.T) {
	a := baseKey()
	b := baseKey()
	b.SourceIDs = []string{"art-c", "art-b", "art-a"}
	if a.SourceSetDigest() != b.SourceSetDigest() {
		t.Fatalf("source set digest depended on ordering")
	}
}

func TestNewSynthesisPersistence_RejectsNilPool(t *testing.T) {
	// Fail at wiring time, not at the first scheduled run hours later.
	if _, err := NewSynthesisPersistence(nil); err == nil {
		t.Fatalf("expected a nil pool to be rejected")
	}
}

func TestDedupeStrings_PreservesFirstOccurrenceOrder(t *testing.T) {
	got := dedupeStrings([]string{"b", "a", "b", "c", "a"})
	want := []string{"b", "a", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
