package acceptance

import (
	"errors"
	"testing"
)

// TestEveryFailureCodeHasOneCategoryAndOwner is TP-102-01-04. It proves the
// closed E102-JOURNEY-* failure registry maps every code to exactly one category
// and one owner, that an unknown code lookup fails closed with no guessed
// default, and — via independent adversarial canaries — that a duplicate code, a
// code with no owner, an owner-mismatched code, a category-mismatched code, an
// unknown-category code, an unfamilied code, and an empty registry each make
// Validate() return contract-invalid. A permissive Validate() (one that tolerated
// any of these) would fail the adversarial subtests below, so the canaries are
// real.
func TestEveryFailureCodeHasOneCategoryAndOwner(t *testing.T) {
	// The canonical registry declares no duplicate code (guards our own source).
	t.Run("canonical declarations contain no duplicate code", func(t *testing.T) {
		decls := canonicalFailureDecls()
		seen := make(map[FailureCode]bool, len(decls))
		for _, d := range decls {
			if seen[d.Code] {
				t.Fatalf("canonical decls contain duplicate code %q", d.Code)
			}
			seen[d.Code] = true
		}
		if len(decls) == 0 {
			t.Fatal("canonical decls are empty")
		}
	})

	// The canonical registry is valid and every code has exactly one closed
	// category and one closed owner, with the owner being the category's
	// canonical owner and the category matching the code's family prefix.
	t.Run("canonical registry is valid and every code has one category and owner", func(t *testing.T) {
		reg, err := DefaultFailureRegistry()
		if err != nil {
			t.Fatalf("DefaultFailureRegistry() error = %v; want nil", err)
		}
		decls := canonicalFailureDecls()
		if got, want := len(reg.Codes()), len(decls); got != want {
			t.Fatalf("registry has %d codes; want %d (a collapse implies a duplicate)", got, want)
		}
		for _, d := range decls {
			meta, ok := reg.LookupFailure(d.Code)
			if !ok {
				t.Errorf("LookupFailure(%q) not ok; want registered", d.Code)
				continue
			}
			if !IsClosedCategory(meta.Category) {
				t.Errorf("code %q category %q is not closed", d.Code, meta.Category)
			}
			if !IsClosedOwner(meta.Owner) {
				t.Errorf("code %q owner %q is not closed", d.Code, meta.Owner)
			}
			if want := categoryToOwner[meta.Category]; meta.Owner != want {
				t.Errorf("code %q owner = %q; want canonical owner %q for category %q", d.Code, meta.Owner, want, meta.Category)
			}
			derived, dok := categoryForCode(d.Code)
			if !dok || derived != meta.Category {
				t.Errorf("code %q family prefix implies category (%q, ok=%v); want %q", d.Code, derived, dok, meta.Category)
			}
		}
	})

	// The one cross-cutting family stem resolves unambiguously: a capability code
	// is capability-status (never contract), and a contract code is contract.
	t.Run("capability prefix wins over the contract stem", func(t *testing.T) {
		if got, _ := categoryForCode("E102-JOURNEY-CONTRACT-CAPABILITY-POLICY"); got != CategoryCapabilityStatus {
			t.Errorf("capability code derived category = %q; want %q", got, CategoryCapabilityStatus)
		}
		if got, _ := categoryForCode("E102-JOURNEY-CONTRACT-MISSING"); got != CategoryContract {
			t.Errorf("contract code derived category = %q; want %q", got, CategoryContract)
		}
	})

	// Unknown-code lookup fails closed and never returns a guessed default.
	t.Run("unknown code lookup returns not-ok with no default", func(t *testing.T) {
		reg, err := DefaultFailureRegistry()
		if err != nil {
			t.Fatalf("DefaultFailureRegistry() error = %v; want nil", err)
		}
		for _, code := range []FailureCode{"", "E102-JOURNEY-NOT-A-REAL-CODE", "E102-JOURNEY-SEARCH-INVENTED"} {
			if meta, ok := reg.LookupFailure(code); ok || meta != (FailureMeta{}) {
				t.Errorf("LookupFailure(%q) = (%+v, %v); want (zero, false)", code, meta, ok)
			}
		}
	})

	// Closed verdict / outcome vocabularies reject unknown values.
	t.Run("closed verdict and outcome vocabularies reject unknowns", func(t *testing.T) {
		for _, v := range []AggregateVerdict{
			VerdictAccepted, VerdictAcceptedDegraded, VerdictBlockedPrerequisite,
			VerdictRejected, VerdictContractInvalid, VerdictTimedOut,
		} {
			if !IsClosedVerdict(v) {
				t.Errorf("IsClosedVerdict(%q) = false; want true", v)
			}
		}
		if IsClosedVerdict("approved") || IsClosedVerdict("") {
			t.Error("IsClosedVerdict accepted an unknown verdict")
		}
		for _, o := range []JourneyOutcome{
			OutcomePassed, OutcomeAllowedEmpty, OutcomeAllowedQuiet, OutcomeAllowedOptional,
			OutcomeAllowedDegraded, OutcomeFailed, OutcomeBlocked, OutcomeTimedOut, OutcomeNotEvaluated,
		} {
			if !IsClosedJourneyOutcome(o) {
				t.Errorf("IsClosedJourneyOutcome(%q) = false; want true", o)
			}
		}
		if IsClosedJourneyOutcome("skipped") || IsClosedJourneyOutcome("") {
			t.Error("IsClosedJourneyOutcome accepted an unknown outcome")
		}
	})

	// Adversarial canaries: each independently mutated declaration set MUST make
	// Validate() return contract-invalid. A duplicate/ownerless/mismatched code
	// is never tolerated.
	adversarial := []struct {
		name  string
		decls []FailureDecl
	}{
		{
			name: "duplicate code",
			decls: []FailureDecl{
				{Code: "E102-JOURNEY-SEARCH-HTTP", Category: CategorySearch, Owner: OwnerSearch},
				{Code: "E102-JOURNEY-SEARCH-HTTP", Category: CategorySearch, Owner: OwnerSearch},
			},
		},
		{
			name: "code with no owner",
			decls: []FailureDecl{
				{Code: "E102-JOURNEY-SEARCH-HTTP", Category: CategorySearch, Owner: ""},
			},
		},
		{
			name: "owner-mismatched code",
			decls: []FailureDecl{
				{Code: "E102-JOURNEY-SEARCH-HTTP", Category: CategorySearch, Owner: OwnerDigest},
			},
		},
		{
			name: "category-mismatched code",
			decls: []FailureDecl{
				{Code: "E102-JOURNEY-SEARCH-HTTP", Category: CategoryDigest, Owner: OwnerDigest},
			},
		},
		{
			name: "unknown category",
			decls: []FailureDecl{
				{Code: "E102-JOURNEY-SEARCH-HTTP", Category: FailureCategory("made-up"), Owner: OwnerSearch},
			},
		},
		{
			name: "code outside every closed family prefix",
			decls: []FailureDecl{
				{Code: "E102-BOGUS-CODE", Category: CategoryContract, Owner: OwnerAcceptanceContract},
			},
		},
		{
			name:  "empty registry",
			decls: nil,
		},
	}
	for _, tc := range adversarial {
		t.Run("adversarial: "+tc.name, func(t *testing.T) {
			reg := newFailureRegistry(tc.decls)
			err := reg.Validate()
			if !errors.Is(err, ErrFailureRegistryInvalid) {
				t.Fatalf("Validate() error = %v; want errors.Is ErrFailureRegistryInvalid (contract-invalid)", err)
			}
			// A rejected registry never exposes a lookup (fail closed).
			if meta, ok := reg.LookupFailure("E102-JOURNEY-SEARCH-HTTP"); ok || meta != (FailureMeta{}) {
				t.Errorf("rejected registry LookupFailure = (%+v, %v); want (zero, false)", meta, ok)
			}
		})
	}
}
