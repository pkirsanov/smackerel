# BUG-061-010 — Scopes

> **Status:** all scopes Blocked (operator live smoke) — this packet performed
> diagnosis only. Its fix was routed to spec 104, which delivered it.

This is a **diagnosis-and-route** packet. It never implemented, tested, or
validated a fix, so its scopes model the work it actually performed: observe the
grounding gap on the live deployment, diagnose the root cause, choose a fix
direction, and route that direction to an implementing spec.

The implementing spec is [104-universal-ask-self-knowledge](../../../104-universal-ask-self-knowledge/).
Every implementation, test, and validation obligation for the fix is owned there.
Restating those obligations here would claim work this packet did not perform.

## Why every DoD item below is unchecked

Two distinct reasons apply. Each item names which one, so a reader never has to
guess whether an empty box means "not done" or "not ours".

- **(P) prior-session evidence.** The work was genuinely performed on 2026-07-23
  by the session that filed this bug, and its evidence is recorded in
  [report.md](report.md), [bug.md](bug.md), and `state.json`. The session that
  authored these scope artifacts did not re-execute it. Under
  `evidence-rules.md` the claim source for a command this agent did not run is
  `not-run`, and `not-run` may never back an `[x]`. The box is empty because the
  provenance is honest, not because the work is absent.
- **(R) routed to spec 104.** The work is owned and certified by spec 104. This
  packet neither performed it nor certifies it.

## Test artifacts owned here

| Test type | File | Owner |
|---|---|---|
| none | none | this packet owns no test artifact |

This packet changed no source, so it designs no regression test. The adversarial
regression obligation for the delivered fix is discharged on spec 104, against
the code that actually changed. The connector-level `/ask` smoke that spec 104
records at commit `4a7c545d` is likewise a spec-104 artifact.

## Scenario contracts owned here

None. This packet declares no Gherkin scenario contract, because it delivers no
behavior. The behavioral scenarios for the delivered fix are declared on spec
104 and are certified there.

---

## SCOPE-01 — Observe the grounding gap on the live deployment

**Status:** Blocked (operator live smoke)
**Depends On:** BUG-061-009 (done — it made this case refuse honestly instead of masking the refusal as a capture acknowledgement)

### What this scope covered

A human asked the deployed bot `/ask how smackerel works as second brain or llm
wiki?` during the BUG-061-009 close-out and received the honest refusal instead
of an answer. This scope is the observation work that turned that single user
report into a characterised, reproducible condition: confirm the agent ran to
completion, confirm it grounded zero sources, and confirm the state of each
grounding channel on the running system rather than from reading code.

### Definition of Done

- [ ] (P) The refusal is reproduced against the deployed bot and the agent is confirmed to terminate normally while grounding zero sources
- [ ] (P) The open_knowledge tool wiring on the running core is read from the startup log rather than inferred from configuration
- [ ] (P) The search integration is confirmed enabled in the deployed environment, ruling out "search is off" as the explanation
- [ ] (P) A direct query to the deployed search provider is executed to establish what the public web actually returns for the product name

> **Uncertainty Declaration (applies to all four items above)**
>
> **What is known:** all four observations were made on 2026-07-23 and their
> results are recorded verbatim in `state.json` under `diagnosis.liveEvidence`
> and narrated in [bug.md](bug.md) and [report.md](report.md).
> **What is not known to this session:** nothing was re-executed here, so this
> session holds no first-hand output for any of them.
> **What would resolve it:** re-running the four observations against the
> current deployment and recording the raw output. That requires the deployed
> messaging channel and the production runtime endpoint, which are operator-only.
> **Why it is not a blocker for the diagnosis:** the fix chosen from these
> observations was delivered and independently verified on spec 104.

---

## SCOPE-02 — Diagnose the root cause and choose a fix direction

**Status:** Blocked (operator live smoke)
**Depends On:** SCOPE-01

### What this scope covered

Turning the observations into a causal explanation, then converting that
explanation into a decision. The load-bearing move is negative: search was
working, so a broken-tool hypothesis is ruled out and the investigation is
redirected from the mechanism to the corpus. The conclusion is that the refusal
was correct and the gap is that the second brain had never been told about
itself. The reasoning is written up in [design.md](design.md).

### Definition of Done

- [ ] (P) Each of the three grounding channels is individually accounted for, and none is left as an untested assumption
- [ ] (P) The refusal path is exonerated in writing, so no change is proposed to the cite-back verifier or the provenance gate
- [ ] (P) The condition is classified as a knowledge/data gap rather than a code defect, with the reasoning recorded
- [ ] (P) Three candidate directions are stated with their trade-offs, and one is chosen with a recorded rationale

> **Uncertainty Declaration (applies to all four items above)**
>
> **What is known:** the analysis and the decision are recorded in
> [bug.md](bug.md) and [design.md](design.md); the chosen direction is option A
> with a dedicated system-knowledge collection.
> **What is not known to this session:** the analysis rests on the SCOPE-01
> observations, which this session did not re-execute.
> **What would resolve it:** the same operator-owned re-observation named in
> SCOPE-01.

---

## SCOPE-03 — Route the chosen direction to an implementing spec

**Status:** Blocked (operator live smoke)
**Depends On:** SCOPE-02

### What this scope covered

Handing the decision to a packet that can implement it, and keeping this packet
honest about not having implemented anything. The product owner selected option
A with a dedicated system-knowledge collection, and spec 104 designed, built,
and deployed it.

### Definition of Done

- [ ] (R) The chosen direction is accepted by an implementing spec and built there
- [ ] (R) The delivered corpus is addressable independently of the user's personal knowledge graph
- [ ] (R) The cite-back verifier, the provenance gate, and the BUG-061-009 refusal are confirmed unmodified by the delivered fix
- [ ] (R) The delivered behavior is proven by tests owned by the implementing spec
- [ ] (P) This packet records that it performed no implementation and certifies no delivery, so its terminal status mirrors spec 104 rather than claiming `done`
- [ ] A live behavioural confirmation of `/ask` through the deployed messaging channel is recorded by the operator

> **Uncertainty Declaration (applies to the four (R) items above)**
>
> **What is known:** spec 104 is the owner of all four, and this packet's
> `state.json` records the delivery and the live deployment verification of
> 2026-07-23.
> **What is not known to this session:** this packet holds no certification
> authority over spec 104's artifacts and must not assert their state.
> **What would resolve it:** reading spec 104's own certification. Marking these
> `[x]` here would import a certification this packet never earned, which is the
> precise shape of cross-packet fabrication.

> **Uncertainty Declaration (final item — the live behavioural confirmation)**
>
> **What is known:** this is the single genuinely-open item, and it is shared
> with spec 104. The render half of it was automated at commit `4a7c545d` by a
> connector-only `/ask` smoke owned by spec 104, which narrows the open item to
> a confirmation against the live deployment.
> **What was attempted:** nothing in this session. It is not agent-attemptable.
> **Why:** an agent cannot send a message through the deployed Telegram channel,
> and the production assistant HTTP surface requires a per-user PASETO token
> that agents do not hold.
> **What would resolve it:** the operator sending a product meta-question to the
> deployed bot and recording the reply, then authoring the human acceptance
> record in [uservalidation.md](uservalidation.md).
> **Owner:** operator.
