# Design: BUG-061-013 — Root cause and fix directions

## Root cause

**An anchored regex encoding an assumption about invocation *syntax* rather than *semantics*, with
no zero-match guard.**

Two independent mistakes compose. Either alone would be survivable; together they produce a check
that cannot fail.

### Cause 1 — the locator is lexical and anchored, so it encodes a syntax assumption

```go
// internal/deploy/envsubst_wrapper_contract_test.go:82
var envsubstGoTestRE = regexp.MustCompile(`(?m)^\s*go\s+test\b`)
```

`(?m)^\s*` requires the `go test` token to be the **first non-whitespace text on its line**. That is
true only for the narrowest possible invocation shape. It is false for every shell construct that
puts something in front of the command — which is most of them: `if`, `if !`, `&&`, `||`, `then`,
`time`, `exec`, `env VAR=x`, a `$( … )` substitution, or an assignment prefix.

The regex's own comment shows the author was thinking about *trailing* flexibility and not *leading*:

```go
// envsubstGoTestRE matches an actual `go test` invocation. This must
// appear AFTER the ensure_envsubst call. Whitespace-leading is OK; a
// trailing `\` to indicate continuation is OK.
```

"Whitespace-leading is OK" is the whole allowance. Nothing else may precede the token.

This is not a typo; it is a category error. The contract's *subject* is semantic — "the command that
runs the tests" — and the implementation binds it to one lexical spelling of that command. The
binding held only as long as nobody wrote the command a different legal way.

### Cause 2 — absence is read as compliance

```go
// internal/deploy/envsubst_wrapper_contract_test.go:107-111
goTestIdx := envsubstGoTestRE.FindStringIndex(src)
if goTestIdx != nil && goTestIdx[0] < callIdx[0] {
    return fmt.Errorf(…)
}

return nil
```

The `goTestIdx != nil &&` short-circuit is the entire silent-pass. When the locator misses, the
comparison is skipped and the function returns `nil`.

**Why the `!= nil` guard looks defensible, and why it is not.** The reasoning behind it is: *"if
there is no `go test`, there is nothing to order against, so the ordering trivially holds."* That is
sound **only if the locator is complete** — only if a zero match really means "this wrapper runs no
`go test`". Because the locator is lexical and anchored, a zero match actually means *"this wrapper
runs `go test` in a form I do not recognise"*, which supports the opposite conclusion.

And the code already contains the proof that the charitable reading is wrong. `envsubstTrackedWrappers`
(lines 55–60) is a hand-maintained list whose documented entry condition is exactly *"a new `go-*`
wrapper that runs `go test`"*. Membership in that list is an assertion that the wrapper runs
`go test`. So a tracked wrapper with zero matches is **self-contradictory**: the list says the
invocation exists, the locator says it does not. Exactly one of them is wrong, and it is never the
list — the list is a human declaration of intent, the locator is a fallible pattern. The correct
resolution of that contradiction is to fail, and instead the function returns success.

### Why nothing caught it

Three factors had to line up, and did:

1. **The three adversarial fixtures all use the bare form.** Lines 184, 204, and 224 each write
   `go test ./...` at line start, so the regex matches in every fixture the suite owns. The
   zero-match branch has no test at all — adversarial or otherwise.
2. **The failure is silent by construction.** A locator that misses produces no output, no skip
   marker, and no warning. Go's test runner cannot report "this subtest asserted nothing", because
   from its perspective the subtest ran and did not call `t.Fatalf`.
3. **The change and the guard live in different files with no build-level coupling.** `c7667d99`
   edited a shell script; the guard is Go source reading that script as data. Nothing links them
   except the regex, and the regex silently stopped linking them.

## Fix directions

### (a) Make zero-match a hard failure — **PRIMARY**

Replace the `!= nil` short-circuit with an explicit absence branch, alongside the two that already
exist for the source line and the call:

```go
goTestIdx := envsubstGoTestRE.FindStringIndex(src)
if goTestIdx == nil {
    return fmt.Errorf("%s: could not LOCATE a `go test` invocation …; "+
        "the wrapper is not necessarily wrong — the matcher may need widening …", wrapperName)
}
if goTestIdx[0] < callIdx[0] { … }
```

| | |
|---|---|
| **Size** | Smallest of the three — one branch. |
| **Leverage** | Highest. It fixes the *class*, not the instance: every future silent pass, in all four tracked wrappers and every wrapper added later, becomes loud. |
| **Cost** | It goes RED at HEAD. That is not a defect of the fix — it is the fix *working*, surfacing the one true finding that exists today. |
| **Blind spot** | It does not, by itself, restore the ordering check for `go-integration.sh`. It converts a silent pass into a loud failure; something else must then make the matcher see line 76. |

### (b) Broaden the regex to tolerate real shell forms

Allow a leading conditional, list operator, or pipeline segment before the token — `if`, `if !`,
`&&`, `||`, `|`, `then`.

| | |
|---|---|
| **Size** | Small — one pattern. |
| **Leverage** | Restores the check for `go-integration.sh` today. |
| **Cost** | Fragile **on its own**. It re-encodes an assumption about syntax; the next legal shape the author did not anticipate (`env GOFLAGS=… go test`, `timeout 900 go test`, a `$(…)` capture) breaks it again — and *without (a), it breaks silently, exactly as this bug did*. Widening a blind matcher buys one round. |
| **Blind spot** | Every form not enumerated. |

### (c) Parse semantically rather than lexically

Resolve the wrapper to its actual command sequence — a shell AST, or a harness that sources the
wrapper with a `go` shim on `PATH` and records invocation order — and assert ordering over
*commands* rather than over *text offsets*.

| | |
|---|---|
| **Size** | Largest. |
| **Leverage** | Strongest. Immune to syntax entirely; this whole bug class disappears. |
| **Cost** | Adds a shell-parsing dependency (or an execution harness) to a contract test that is currently pure stdlib and runs in 0.115s. An execution harness also has to not actually run the tests, which is its own correctness problem. |
| **Blind spot** | Complexity. A parser that is subtly wrong is a new silent-failure surface — the same class of defect, relocated. |

### Composition, and the chosen primary

**(a) and (b) compose, and should land together. (a) is the fix; (b) is the remediation of the one
finding (a) surfaces.**

The primary is **(a)**, and the justification is that it is the only option that addresses what
actually went wrong. (b) and (c) both improve the locator's *reach*; neither changes what the guard
does when the locator is **out of reach** — and that, not the reach, is the defect. A guard with a
perfect matcher and a silent zero-match branch is still a guard that will one day pass on nothing.
(a) makes the detector honest about its own blindness, which is the property that makes every
subsequent improvement safe to attempt.

Sequencing within a single change:

1. Add the (a) branch. The live subtest for `go-integration.sh` goes RED — the finding this packet
   documents, now visible.
2. Widen the matcher per (b) so the line-76 form is located. RED → GREEN, with the ordering
   genuinely asserted this time.
3. Add the EB-5 adversarial fixture: a wrapper whose invocation the matcher cannot locate must be
   REJECTED. This is what stops (a) from silently regressing later.

(c) is recorded as the strongest long-term direction and deliberately **not** chosen now. Its cost is
real, and with (a) in place the cheap lexical approach is no longer dangerous — its failures announce
themselves. If widening proves to recur, (c) becomes the right answer, and (a) is what will tell us.

## Change boundary

**Allowed:** `internal/deploy/envsubst_wrapper_contract_test.go` only.

**Explicitly excluded — and this is load-bearing, not cautious:**

- `scripts/runtime/go-integration.sh` — the wrapper is **correct**. Reverting line 76 to a bare
  `go test` would make the test pass while re-breaking the BUG-061-011 eval gate that the conditional
  form exists to serve, and would leave the zero-match hole open for the next wrapper. Editing the
  subject to satisfy a broken detector is the inverse of a fix.
- `scripts/runtime/{go-unit,go-e2e,go-stress}.sh` and `_ensure_envsubst.sh` — unaffected.
- The `specs/061-conversational-assistant/bugs/BUG-061-011-*/` packet — a regression phase is in
  flight against it.

## Risk

Low. The change is confined to one test file; its worst realistic outcome is a RED lane that names a
real gap. The notable risk is the *opposite* of the usual one: a fix that lands (b) without (a) would
turn the lane green and look complete while leaving the actual defect — the silent zero-match — fully
intact.
