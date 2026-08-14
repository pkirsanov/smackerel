# User Validation: BUG-061-012

Items describing behaviour that already holds are checked. Items describing behaviour this bug will
deliver are unchecked until a human has run the steps and observed them — no agent may check those.

## Checklist

### [Defect] BUG-061-012 The model currently supplies the identity

- [x] **What:** `retrieval_search` declares `user_id` as a required tool argument and the server
  validates only that it is non-empty, so the identity the corpus is read under is chosen by the
  language model.
  - **Steps:**
    1. `sed -n '104,113p' internal/agent/tools/retrieval/tool.go`
    2. `sed -n '178,184p' internal/agent/tools/retrieval/tool.go`
  - **Expected:** `user_id` appears in `required`, and the only check is `in.UserID == ""`.
  - **Verify:** Observed exactly that at HEAD `0f4b4826`.
  - **Evidence:** report.md → Before Fix — Reproduction, STEP 1 and STEP 2
  - **Notes:** This bounds the fix. The retrieval path itself is correct; it is the identity input
    that is wrong. The implementing agent must not rewrite the search engine or its ranking.

### [Bug Fix] BUG-061-012 Identity is server-derived and the corpus requires a grant

- [ ] **What:** No agent tool accepts a caller identity as an argument; retrieval resolves the
  principal from the request context and requires `corpus:read`; a call with no principal fails
  closed rather than searching.
  - **Steps:**
    1. `grep -rn 'user_id' internal/agent/tools/` — expect no schema property match
    2. `./smackerel.sh test unit --go --go-run 'TestToolSchemas_DeclareNoCallerIdentity|TestRetrieval_' --verbose`
    3. `./smackerel.sh test integration`
  - **Expected:** Step 1 finds no caller-identity property in any tool schema. Step 2 passes,
    including the no-principal and grant-required cases. Step 3 exits 0.
  - **Verify:** Exit code 0 on steps 2 and 3, and step 1 returning no schema match.
  - **Notes:** Also confirm the guard has teeth — the schema-contract test must FAIL if `user_id` is
    put back. A test that passes both before and after the fix does not protect it.

- [ ] **What:** A mapped Telegram chat can still retrieve, and an unmapped one cannot.
  - **Steps:**
    1. `./smackerel.sh test unit --go --go-run 'TestTelegramBridge_' --verbose`
  - **Expected:** The mapped-chat case injects a principal and retrieval succeeds; the unmapped-chat
    case injects none and the corpus tool fails closed.
  - **Notes:** This is the row most likely to reveal a regression. Failing closed is correct
    security behaviour and also the most plausible way to break a working surface, so it is checked
    explicitly rather than assumed.
