# User Validation — Spec 091 (Web Self-Registration, Invite-Token Gated)

**Report:** [report.md](report.md) · **Scopes:** [scopes.md](scopes.md) · **Spec:** [spec.md](spec.md)

> Items are **checked `[x]` by default** (the expected validated behavior). The operator **unchecks `[ ]`** to report a behavior that is broken — an unchecked item is a BLOCKING user-reported regression for `/bubbles.validate` to investigate. Evidence for each item lands under the matching [report.md](report.md) anchor at implement/test time.

## Checklist

- [x] Opening `/register` in the browser shows a form with username, password, confirm-password, and a masked invite-token field, plus a link to sign in. (UC-1 / AC-1)
- [x] Submitting `/register` with the correct invite token, a new username, and two matching passwords creates the account and lands on the `/login` page with an "Account created — sign in." notice. (UC-1 / AC-2 / AC-8)
- [x] The same invite token keeps working for more than one account — it is not used up after the first registration. (UC-1 / Goal 2)
- [x] A newly registered account can sign in at `/login` with its username + password and reach `/cards`. (UC-6 / AC-8)
- [x] Submitting `/register` with a wrong, missing, or unconfigured invite token is rejected and creates no account, with a message that does not reveal whether the username was free or the gate is configured. (UC-2 / UC-3 / AC-4 / AC-5 / AC-10)
- [x] Registering a username that already exists is rejected and does not change the existing account's password. (UC-4 / AC-3)
- [x] Mismatched passwords, a too-short password, or a missing field are each rejected with a clear message and create no account. (UC-5 / AC-6)
- [x] The existing `/login` page still works exactly as before for both the token form and the username/password form. (UC-7 / AC-9)
- [x] Submitting the registration form many times in quick succession from one machine is rate-limited. (UC-8 / AC-7)
