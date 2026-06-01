# User Validation — Spec 070

> Items default to checked `[x]` once validated. Owner unchecks `[ ]`
> to report regressions.

- [ ] I can run `smackerel-core users add <name>` inside the deployed
  container and create a user with a password I choose.
- [ ] I can open https://<deploy-host>.<tailnet>.ts.net/login, enter my
  username + password, and land on the admin home page.
- [ ] After login, I can navigate to multiple admin pages without
  being asked to log in again (single sign-on across the UI).
- [ ] Wrong password is rejected with a generic error (no
  username-existence leak).
- [ ] Token-form login still works for the Telegram OAuth callback
  (no regression).
