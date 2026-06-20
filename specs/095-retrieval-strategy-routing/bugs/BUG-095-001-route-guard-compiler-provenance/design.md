# BUG-095-001 Design — Spec-068 route-bypass guard false-positive

## Current Truth

The spec-068 guard `policyguard.ReportRawRouteBypasses`
(`internal/assistant/intent/policyguard/guard.go`) walks a source tree and
reports every non-`_test.go` file that matches the Route-call regex but not the
compiler regex, unless allowlisted:

```go
reRouterRoute = regexp.MustCompile(`\b\w+\.Route\s*\(`)
reCompiler    = regexp.MustCompile(`intent\.Compiler|intentCompiler|IntentCompiler`)
...
if !reRouterRoute.MatchString(text) { return nil }   // no Route call → skip
if reCompiler.MatchString(text)     { return nil }   // has compiler ref → skip
findings = append(findings, Finding{...})            // else: report
```

`AllowedRouteCallers` currently contains only `facade.go`.

`internal/assistant/retrieval_strategy_routing.go` (spec 095, SCOPE-06):

```go
func (f *Facade) selectRetrievalStrategy(in intent.CompiledIntent, compiledOK bool, ...) *routing.StrategySelection {
	if f.retrievalRouter == nil || !compiledOK || !isRetrievalClass(in) {
		return nil
	}
	sel := f.retrievalRouter.Route(in)   // ← matches reRouterRoute
	...
}
```

The file references `intent.CompiledIntent` many times but never the literal
`intent.Compiler`. Critically, `intent.CompiledIntent` does NOT contain the
substring `intent.Compiler` (`...Compile`**d**`Intent` vs `...Compile`**r**), so
`reCompiler` does NOT match and the file is reported.

The sibling `facade.go` passes the SAME guard because it carries the spec-068
compiler as a field and call site:

```go
intentCompiler   intent.Compiler                       // facade.go:175
func (f *Facade) WithIntentCompiler(c intent.Compiler) // facade.go:267
ci, _, cerr := f.intentCompiler.Compile(ctx, rawTurn)  // facade.go:781
```

So the guard's intent is satisfied by `facade.go`'s real compiler provenance;
spec 095 added a legitimate downstream `Route` caller WITHOUT that truthful
provenance reference. This is a string-heuristic false positive, not a real
bypass: the spec-095 router routes the already-compiled intent (gated on
`compiledOK`), opens no store, and makes no second LLM call.

## Fix

Add a TRUTHFUL `intent.Compiler` provenance reference to the
`selectRetrievalStrategy` doc comment (the function that performs the `.Route()`
call), expressing the real invariant:

```go
// ... The `in intent.CompiledIntent` passed here is the OUTPUT of the spec 068
// intent.Compiler, produced UPSTREAM in the facade ingress (facade.go) before
// this seam is ever reached; this router only CONSUMES that already-compiled
// intent — it never sees raw text and never invokes the intent.Compiler itself
// (NFR-1 — no second LLM round-trip). ...
```

This satisfies `reCompiler` via the guard's OWN sanctioned satisfaction pattern
(the guard's adversarial baseline in `intent_bypass_guard_test.go` step 3 treats
an `intent.Compiler` comment reference as legitimate) AND documents the real
design invariant. No allowlist edit, no policy edit, no runtime change.

## Why a comment (not an allowlist entry, not a dead var)

- **Allowlist entry is FORBIDDEN**: adding `retrieval_strategy_routing.go` to
  `AllowedRouteCallers` would blind the guard to that file forever — a future
  real bypass introduced into the same file would go undetected. It is also a
  cross-spec policy edit (spec 068 owns the guard).
- **Dead `var _ intent.Compiler` / fake token is FORBIDDEN**: it would satisfy
  the regex without expressing the truth and would be a misleading,
  unmaintainable artifact. The guard's design intent is that the file
  references its compiler provenance HONESTLY (as `facade.go` does).
- **A truthful doc comment** both documents the real invariant (the router is
  downstream of the spec-068 compiler) and satisfies the guard the way
  `facade.go` does — by genuinely referencing the compiler in a true statement.

## Change Boundary

- **Allowed:** `internal/assistant/retrieval_strategy_routing.go`
  (`selectRetrievalStrategy` doc comment only).
- **Excluded:** `internal/assistant/intent/policyguard/guard.go` (guard +
  `AllowedRouteCallers` + `ScanSubdirs`),
  `tests/integration/policy/intent_bypass_guard_test.go`,
  `policy-exception-baseline.json`, all other policy files, all runtime/source
  code, and spec 095 top-level `state.json`.

## Test Strategy

| Test | Type | Asserts |
|------|------|---------|
| T-BUG-095-001-1 | regression (red→green) | guard test passes; zero findings under `internal/assistant` |
| T-BUG-095-001-2 | adversarial baseline (pre-existing) | the existing `intent_bypass_guard_test.go` step-3 baseline (fixture WITH vs WITHOUT an `intent.Compiler` reference) proves the guard still catches a real raw-text bypass and does not always-fire — this IS the regression guard; no redundant new guard test is added |
| T-BUG-095-001-3 | no-regression | `./smackerel.sh test unit --go` all packages green |
