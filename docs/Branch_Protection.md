# Branch Protection Rules

Recommended GitHub branch protection settings for the `main` branch.

## Required Settings

### Status Checks

- **Require status checks to pass before merging:** Enabled
- **Required checks:**
  - `lint-and-test` (from `.github/workflows/ci.yml`)
  - `build` (from `.github/workflows/ci.yml`)
- **Require branches to be up to date before merging:** Enabled

### Pull Request Reviews

- **Require a pull request before merging:** Enabled
- **Required approving reviews:** 1 (optional for solo developer — can be set to 0)
- **Dismiss stale pull request approvals when new commits are pushed:** Enabled

### Branch Restrictions

- **Do not allow bypassing the above settings:** Enabled (prevents admin override)
- **Restrict who can push to matching branches:** Enabled (only through PRs)

## Optional Settings

These are recommended but may be adjusted based on workflow needs:

| Setting | Recommended | Notes |
|---------|-------------|-------|
| Require signed commits | Off | Adds friction for solo dev; enable for teams |
| Require linear history | On | Keeps git log clean with squash/rebase |
| Allow force pushes | Off | Prevents history rewriting on main |
| Allow deletions | Off | Prevents accidental branch deletion |

## Setup Steps

1. Navigate to **Settings → Branches** in the GitHub repository
2. Click **Add branch protection rule**
3. Set **Branch name pattern** to `main`
4. Enable settings as listed above
5. Click **Create** / **Save changes**

## CI Integration

The CI workflow at `.github/workflows/ci.yml` runs:
- `./smackerel.sh lint` — Go vet + Python ruff
- `./smackerel.sh test unit` — Go + Python unit tests
- `./smackerel.sh build` — Docker image compilation

A failing CI job blocks the PR from merging when branch protection is configured.
