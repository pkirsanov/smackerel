# User Validation — Spec 093 (Admin-Generated Registration Invites, DB-Backed, Single-Use)

**Report:** [report.md](report.md) · **Scopes:** [scopes.md](scopes.md) · **Spec:** [spec.md](spec.md)

> Items are **checked `[x]` by default** (the expected validated behavior). The operator **unchecks `[ ]`** to report a behavior that is broken — an unchecked item is a BLOCKING user-reported regression for `/bubbles.validate` to investigate. Evidence for each item lands under the matching [report.md](report.md) anchor at implement/test time.

## Checklist

- [x] As a logged-in operator I can open **Account Invites** from `/cards/admin` and generate a single-use registration invite; the one-time token is shown **exactly once** and never again. (UC-1 / AC-3 / AC-4)
- [x] The generated invite list shows only metadata (label, who created it, when, status, and who used it) — it never shows the token or its hash. (UC-8 / AC-9)
- [x] A new person can register **once** at `/register` with a generated invite; the account is created and the invite is marked used. (UC-2 / AC-5)
- [x] The same generated invite cannot create a **second** account — reusing it is rejected. (UC-3 / AC-5)
- [x] An expired or revoked invite can no longer be used to register. (UC-4 / UC-9 / AC-5)
- [x] I can **revoke** an outstanding invite from the UI, and it can no longer register afterward. (UC-9 / AC-3)
- [x] The original static invite token (`WEB_REGISTRATION_INVITE_TOKEN`) still works as the bootstrap path — registering with it succeeds exactly as before and is **not** used up. (UC-5 / AC-6 / AC-10)
- [x] A wrong / unknown / used / revoked / expired invite — and a wrong static token — all show the **same** generic "registration is not available or the invite is invalid" message, revealing nothing about why. (UC-6 / AC-7)
- [x] A not-logged-in visitor cannot reach the generate / list / revoke pages. (UC-7 / AC-8)
- [x] The existing `/register`, `/login`, and `/cards` pages still work exactly as before. (UC-10 / AC-10)
