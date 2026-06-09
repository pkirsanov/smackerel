# User Validation Checklist: [BUG-056-002]

## Checklist

- [x] Bug BUG-056-002 documented and triaged with verified diagnostic evidence (HEAD `9638b065`)
- [ ] Maintainer product decision recorded — Path A (build PKCE) vs Path B (de-scope + correct claims) (validated after design.md Q1 is resolved)
- [ ] User-owned endpoints (bookmarks/likes/users-me) authenticate with a user-context token, not App-Only (validated after the deferred fix — Path A)
- [ ] Adversarial: App-Only bearer on a user-owned endpoint is rejected, not silently accepted (validated after the deferred fix)
- [ ] Public endpoints (`tweets`/`mentions`) still authenticate with App-Only — no regression (validated after the deferred fix)
- [ ] spec 056 report.md no longer claims PKCE delivery unless PKCE actually shipped (validated after the deferred closure)
- [ ] `x-rate-limit-remaining` gauge updated after each API call — R-016 (validated after the deferred fix)

Unchecked items indicate work pending the deferred deliberate delivery pass. The single checked item reflects only that this tracked bug packet was created with verified evidence — it does NOT claim the fix is done.
