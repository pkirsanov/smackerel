package acceptance

import (
	"errors"
	"testing"
)

// validSurface returns a fresh, safe production-readonly surface. Each call
// allocates new slices so an adversarial mutation never aliases another case.
func validSurface() ProductionSurface {
	return ProductionSurface{
		Routes: []ProductionRoute{
			{Method: "GET", Template: "/", SideEffect: SideEffectRead},
			{Method: "GET", Template: "/login", SideEffect: SideEffectRead},
			{Method: "POST", Template: "/v1/web/login", SideEffect: SideEffectSessionEstablish},
			{Method: "GET", Template: "/api/digest", SideEffect: SideEffectRead},
			{Method: "POST", Template: "/search", SideEffect: SideEffectReadCompute},
			{Method: "GET", Template: "/api/health", SideEffect: SideEffectTelemetryRead},
		},
		Selectors:      []string{"search-input", "search-submit", "results-list", "login-form", "digest-region"},
		EvidenceFields: []string{"status", "route-id", "duration-ms", "state-enum", "body-digest", "outcome"},
		RunnerSources: []string{
			"const ctx = await browser.newContext(); const page = await ctx.newPage();",
			"await page.goto(mountedBaseURL); await page.getByLabel('Search').fill(probeRef);",
		},
	}
}

// TestProductionManifestRejectsWritesInterceptionInjectionAndTargetLiterals is
// TP-102-01-05. It proves a valid read-only fixture passes and that each
// independent adversarial mutation — a state-changing selector, an unclassified
// route, a non-session/non-read-compute POST, a mutating method, a
// production-forbidden class, canonical-classification drift, request
// interception, credential/cookie injection, a direct datastore read, a
// service-container exec, a concrete target literal (runner source, route
// template, IPv4, and evidence field), and a forbidden evidence field — is
// rejected with a closed E102-JOURNEY-CONTRACT-* code. A permissive guard (one
// that returned nil) would fail every adversarial subtest, so the canaries are
// real.
func TestProductionManifestRejectsWritesInterceptionInjectionAndTargetLiterals(t *testing.T) {
	// The two codes the guard emits are registry-known closed contract codes.
	t.Run("guard codes are closed registry codes", func(t *testing.T) {
		reg, err := DefaultFailureRegistry()
		if err != nil {
			t.Fatalf("DefaultFailureRegistry() error = %v; want nil", err)
		}
		for _, code := range []FailureCode{CodeContractUnsafeMutation, CodeContractEvidenceUnsafe} {
			meta, ok := reg.LookupFailure(code)
			if !ok {
				t.Errorf("guard code %q is not registered in the failure registry", code)
				continue
			}
			if meta.Category != CategoryContract || meta.Owner != OwnerAcceptanceContract {
				t.Errorf("guard code %q meta = %+v; want contract/acceptance-contract", code, meta)
			}
		}
	})

	// A valid read-only surface passes the static guard.
	t.Run("valid read-only surface passes", func(t *testing.T) {
		if err := ScanProductionSurface(validSurface(), nil); err != nil {
			t.Fatalf("ScanProductionSurface(valid) error = %v; want nil", err)
		}
	})

	adversarial := []struct {
		name     string
		mutate   func(s *ProductionSurface)
		wantCode FailureCode
	}{
		{
			name:     "state-changing selector",
			mutate:   func(s *ProductionSurface) { s.Selectors = append(s.Selectors, "delete-note-button") },
			wantCode: CodeContractUnsafeMutation,
		},
		{
			name: "unclassified route (empty side-effect)",
			mutate: func(s *ProductionSurface) {
				s.Routes = append(s.Routes, ProductionRoute{Method: "GET", Template: "/api/foo", SideEffect: ""})
			},
			wantCode: CodeContractUnsafeMutation,
		},
		{
			name: "POST that is neither session-establish nor read-compute",
			mutate: func(s *ProductionSurface) {
				s.Routes = append(s.Routes, ProductionRoute{Method: "POST", Template: "/notifications/mark", SideEffect: SideEffectRead})
			},
			wantCode: CodeContractUnsafeMutation,
		},
		{
			name: "mutating HTTP method",
			mutate: func(s *ProductionSurface) {
				s.Routes = append(s.Routes, ProductionRoute{Method: "DELETE", Template: "/api/cards/entry", SideEffect: SideEffectRead})
			},
			wantCode: CodeContractUnsafeMutation,
		},
		{
			name: "production-forbidden fixture-write class",
			mutate: func(s *ProductionSurface) {
				s.Routes = append(s.Routes, ProductionRoute{Method: "POST", Template: "/fixtures/seed", SideEffect: SideEffectFixtureWrite})
			},
			wantCode: CodeContractUnsafeMutation,
		},
		{
			name: "canonical side-effect drift",
			mutate: func(s *ProductionSurface) {
				// /search is canonically read-compute; declaring session-establish drifts.
				s.Routes = append(s.Routes, ProductionRoute{Method: "POST", Template: "/search", SideEffect: SideEffectSessionEstablish})
			},
			wantCode: CodeContractUnsafeMutation,
		},
		{
			name: "request interception",
			mutate: func(s *ProductionSurface) {
				s.RunnerSources = append(s.RunnerSources, "await page.route('**/api/**', r => r.fulfill({ status: 200 }));")
			},
			wantCode: CodeContractUnsafeMutation,
		},
		{
			name: "credential/cookie injection",
			mutate: func(s *ProductionSurface) {
				s.RunnerSources = append(s.RunnerSources, "await context.addCookies([{ name: 's', value: v }]);")
			},
			wantCode: CodeContractUnsafeMutation,
		},
		{
			name: "direct datastore read",
			mutate: func(s *ProductionSurface) {
				s.RunnerSources = append(s.RunnerSources, `rows := db.Query("SELECT * FROM users")`)
			},
			wantCode: CodeContractUnsafeMutation,
		},
		{
			name: "service-container exec",
			mutate: func(s *ProductionSurface) {
				s.RunnerSources = append(s.RunnerSources, `exec.Command("docker", "exec", "smackerel-core", "sh")`)
			},
			wantCode: CodeContractUnsafeMutation,
		},
		{
			name: "target literal in runner source",
			mutate: func(s *ProductionSurface) {
				s.RunnerSources = append(s.RunnerSources, "const base = 'https://smackerel.example.ts.net';")
			},
			wantCode: CodeContractEvidenceUnsafe,
		},
		{
			name:     "target IPv4 literal in runner source",
			mutate:   func(s *ProductionSurface) { s.RunnerSources = append(s.RunnerSources, "const host = '203.0.113.5';") },
			wantCode: CodeContractEvidenceUnsafe,
		},
		{
			name: "target literal in route template",
			mutate: func(s *ProductionSurface) {
				s.Routes = append(s.Routes, ProductionRoute{Method: "GET", Template: "https://host.example.ts.net/api/digest", SideEffect: SideEffectRead})
			},
			wantCode: CodeContractEvidenceUnsafe,
		},
		{
			name:     "forbidden evidence field",
			mutate:   func(s *ProductionSurface) { s.EvidenceFields = append(s.EvidenceFields, "response-body-raw") },
			wantCode: CodeContractEvidenceUnsafe,
		},
	}

	reg, err := DefaultFailureRegistry()
	if err != nil {
		t.Fatalf("DefaultFailureRegistry() error = %v; want nil", err)
	}
	for _, tc := range adversarial {
		t.Run("adversarial: "+tc.name, func(t *testing.T) {
			s := validSurface()
			tc.mutate(&s)
			err := ScanProductionSurface(s, nil)
			if err == nil {
				t.Fatalf("ScanProductionSurface(mutated) = nil; want a %q violation", tc.wantCode)
			}
			var gv *GuardViolation
			if !errors.As(err, &gv) {
				t.Fatalf("error %v is not a *GuardViolation", err)
			}
			if gv.Code != tc.wantCode {
				t.Fatalf("violation code = %q; want %q", gv.Code, tc.wantCode)
			}
			// The emitted code is a closed registry code (fail-closed vocabulary).
			if _, ok := reg.LookupFailure(gv.Code); !ok {
				t.Fatalf("violation code %q is not a registered failure code", gv.Code)
			}
		})
	}
}
