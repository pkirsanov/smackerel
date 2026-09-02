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

func TestLogicalKey_TracksCanonicalSourceSetIdentity(t *testing.T) {
	base := baseKey()
	baseLogicalKey := base.LogicalKey()

	reordered := baseKey()
	reordered.SourceIDs = []string{"art-c", "art-a", "art-b"}
	if reordered.LogicalKey() != baseLogicalKey {
		t.Fatal("reordering the same source set changed the logical key")
	}

	// ADVERSARIAL: removing SourceSetDigest from LogicalKey makes every changed
	// case collide with the base run. Changed inputs must create a replacement
	// run; the separate actor/cadence/window advisory key serializes that change.
	for _, changed := range []struct {
		name      string
		sourceIDs []string
	}{
		{name: "grew by ingest", sourceIDs: []string{"art-a", "art-b", "art-c", "art-d"}},
		{name: "shrank by purge", sourceIDs: []string{"art-a"}},
		{name: "replaced", sourceIDs: []string{"art-x", "art-y"}},
		{name: "emptied", sourceIDs: nil},
	} {
		t.Run(changed.name, func(t *testing.T) {
			candidate := baseKey()
			candidate.SourceIDs = changed.sourceIDs
			if candidate.LogicalKey() == baseLogicalKey {
				t.Fatalf("changed source set %s did not change the logical key", changed.name)
			}
		})
	}
}

func TestSourceSet_StaysVisibleAsProvenance(t *testing.T) {
	// Dropping the source set from IDENTITY must not drop it from the record.
	// If this digest stopped discriminating, we would lose the ability to say
	// what a given run actually read.
	a := baseKey()
	b := baseKey()
	b.SourceIDs = []string{"art-a", "art-b"}
	if a.SourceSetDigest() == b.SourceSetDigest() {
		t.Fatalf("source set digest stopped distinguishing corpora; provenance would be unrecoverable")
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
// Without a prefix the fields are concatenated raw, so a character shifted
// across a boundary produces an identical hash and two genuinely different runs
// share one key -- the second silently swallowed as an idempotent no-change.
//
// The shift must be between ADJACENT fields to collide. LogicalKey hashes in
// the order cadence, principal, windowStart, windowEnd, policyVersion, sources,
// so cadence and principal are the adjacent pair used here:
//
//	a: "daily" + "x"  -> "dailyx"
//	b: "dail"  + "yx" -> "dailyx"
//
// Verified adversarial: replacing the length-prefixed write in LogicalKey with
// a raw fmt.Fprintf(h, "%s", part) makes this test FAIL. An earlier version of
// this test shifted between principal and policyVersion, which are NOT
// adjacent -- the two window timestamps sit between them -- so it passed under
// that same mutation and proved nothing.
func TestLogicalKey_FieldBoundariesCannotBeShifted(t *testing.T) {
	a := baseKey()
	a.Cadence = "daily"
	a.Principal = "x"

	b := baseKey()
	b.Cadence = "dail"
	b.Principal = "yx"

	if a.LogicalKey() == b.LogicalKey() {
		t.Fatalf("field boundary collision: (%q,%q) and (%q,%q) produced the same key",
			a.Cadence, a.Principal, b.Cadence, b.Principal)
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
