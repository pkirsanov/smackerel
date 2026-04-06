# <img src="../../icons/bubbles-glasses.svg" width="28"> Annotated Example: REST API Endpoint

> **This is a reference example, not a template.** It shows what completed Bubbles
> artifacts look like for a typical backend feature: adding a REST API endpoint
> with authentication, validation, and admin access.
>
> The feature demonstrated is a **User Profile API** with three operations:
> - `GET /api/v1/users/me` — authenticated user reads own profile
> - `PUT /api/v1/users/me` — authenticated user updates own profile
> - `GET /api/v1/admin/users/:id` — admin reads any user's profile
>
> All three artifacts (spec.md, design.md, scopes.md) are shown below in order.
> In a real project these would be separate files under `specs/NNN-feature-name/`.

---

# ARTIFACT 1: spec.md

<!--
  📝 ANNOTATION: spec.md is the WHAT document. It describes the feature from the
  user's perspective — actors, use cases, acceptance criteria, and non-functional
  requirements. It NEVER describes HOW to implement. Tech-agnostic language is
  preferred (say "persistent store" not "PostgreSQL").
-->

# User Profile API

## Summary

Users need the ability to view and update their own profile information through
the API. Administrators need the ability to view any user's profile for support
and moderation purposes. The API must enforce authentication, field-level
validation, and authorization boundaries.

## Actors

<!--
  📝 ANNOTATION: List every distinct actor. Each actor becomes a column you check
  against in authorization scenarios. 3+ actors is typical for any feature with
  admin access.
-->

| Actor | Description |
|-------|-------------|
| **Authenticated User** | A logged-in user with a valid JWT. Can read/update their own profile. |
| **Platform Administrator** | A user with the `admin` role. Can read any user's profile. |
| **Unauthenticated Client** | A client without a valid JWT. Must be rejected at all endpoints. |

## Use Cases

<!--
  📝 ANNOTATION: Use cases define the actor→system interactions. Each use case
  should map to at least one Gherkin scenario in Acceptance Criteria. 5+ is
  typical for a feature of this scope.
-->

| ID | Actor | Use Case | Priority |
|----|-------|----------|----------|
| UC-01 | Authenticated User | View own profile via `GET /api/v1/users/me` | P0 |
| UC-02 | Authenticated User | Update own profile fields (name, bio, preferences) via `PUT /api/v1/users/me` | P0 |
| UC-03 | Platform Administrator | View any user's profile via `GET /api/v1/admin/users/:id` | P0 |
| UC-04 | Unauthenticated Client | Receive 401 when accessing any profile endpoint | P0 |
| UC-05 | Authenticated User | Receive 403 when accessing admin endpoint without admin role | P1 |
| UC-06 | Authenticated User | Receive validation errors when submitting invalid profile data | P1 |
| UC-07 | Platform Administrator | Receive 404 when requesting a nonexistent user | P1 |

## Acceptance Criteria

<!--
  📝 ANNOTATION: Every Gherkin scenario here becomes a MANDATORY E2E test.
  The count of scenarios should match or exceed the count of E2E rows in the
  Test Plan. Group them logically: happy path first, then auth, validation,
  error handling, concurrency/edge cases.
-->

### Happy Path Scenarios

```gherkin
Scenario: UC-01 — Authenticated user retrieves own profile
  Given an authenticated user "alice" with a valid JWT
  When alice sends GET /api/v1/users/me
  Then the response status is 200
  And the response body contains alice's name, email, bio, and preferences
  And the response includes an "updatedAt" timestamp

Scenario: UC-02 — Authenticated user updates own profile
  Given an authenticated user "alice" with a valid JWT
  And alice's current bio is "Old bio"
  When alice sends PUT /api/v1/users/me with body {"name": "Alice Smith", "bio": "New bio"}
  Then the response status is 200
  And the response body contains name "Alice Smith" and bio "New bio"
  And the "updatedAt" timestamp is later than before the request

Scenario: UC-03 — Admin retrieves another user's profile
  Given an authenticated admin user "admin01"
  And a user "bob" exists with id "uuid-bob-123"
  When admin01 sends GET /api/v1/admin/users/uuid-bob-123
  Then the response status is 200
  And the response body contains bob's name, email, bio, and preferences
```

### Authentication Failure Scenarios

```gherkin
Scenario: UC-04a — Missing Authorization header returns 401
  Given a client sends GET /api/v1/users/me without an Authorization header
  Then the response status is 401
  And the response body contains error code "MISSING_AUTH"

Scenario: UC-04b — Expired JWT returns 401
  Given a client has a JWT that expired 5 minutes ago
  When the client sends GET /api/v1/users/me with the expired JWT
  Then the response status is 401
  And the response body contains error code "TOKEN_EXPIRED"

Scenario: UC-04c — Malformed JWT returns 401
  Given a client has Authorization header "Bearer not-a-real-jwt"
  When the client sends GET /api/v1/users/me
  Then the response status is 401
  And the response body contains error code "TOKEN_INVALID"
```

### Authorization Failure Scenarios

```gherkin
Scenario: UC-05 — Non-admin accessing admin endpoint returns 403
  Given an authenticated user "alice" without the "admin" role
  When alice sends GET /api/v1/admin/users/uuid-bob-123
  Then the response status is 403
  And the response body contains error code "FORBIDDEN"
```

### Input Validation Scenarios

```gherkin
Scenario: UC-06a — Empty name rejected
  Given an authenticated user "alice"
  When alice sends PUT /api/v1/users/me with body {"name": ""}
  Then the response status is 400
  And the response body contains error code "VALIDATION_ERROR"
  And the error details include field "name" with reason "must not be empty"

Scenario: UC-06b — Name exceeding 200 characters rejected
  Given an authenticated user "alice"
  When alice sends PUT /api/v1/users/me with a name that is 201 characters long
  Then the response status is 400
  And the error details include field "name" with reason "must be at most 200 characters"

Scenario: UC-06c — Bio exceeding 2000 characters rejected
  Given an authenticated user "alice"
  When alice sends PUT /api/v1/users/me with a bio that is 2001 characters long
  Then the response status is 400
  And the error details include field "bio" with reason "must be at most 2000 characters"

Scenario: UC-06d — Unknown preference keys rejected
  Given an authenticated user "alice"
  When alice sends PUT /api/v1/users/me with preferences {"invalid_pref": true}
  Then the response status is 400
  And the error details include field "preferences" with reason "unknown key: invalid_pref"

Scenario: UC-06e — Email field update attempt rejected
  Given an authenticated user "alice"
  When alice sends PUT /api/v1/users/me with body {"email": "newemail@example.com"}
  Then the response status is 400
  And the error details include field "email" with reason "email cannot be changed via profile update"
```

### Not Found Scenarios

```gherkin
Scenario: UC-07 — Admin requesting nonexistent user returns 404
  Given an authenticated admin user "admin01"
  When admin01 sends GET /api/v1/admin/users/uuid-does-not-exist
  Then the response status is 404
  And the response body contains error code "USER_NOT_FOUND"
```

### Concurrency & Idempotency Scenarios

<!--
  📝 ANNOTATION: Always include concurrency and idempotency scenarios for write
  endpoints. These often reveal bugs in production. Choose either optimistic
  locking (with version/ETag) or last-write-wins — but be explicit.
-->

```gherkin
Scenario: Concurrent updates — last write wins
  Given an authenticated user "alice"
  And alice opens two browser tabs
  When tab 1 sends PUT /api/v1/users/me with {"name": "Name A"} and succeeds
  And tab 2 sends PUT /api/v1/users/me with {"name": "Name B"} and succeeds
  Then the stored name is "Name B" (last write wins)
  And both responses had status 200

Scenario: Idempotent PUT — repeated identical update produces same result
  Given an authenticated user "alice"
  When alice sends PUT /api/v1/users/me with {"name": "Alice"} twice
  Then both responses have status 200
  And the final profile has name "Alice"
  And the "updatedAt" timestamp of the second response is >= the first
```

## Non-Functional Requirements

<!--
  📝 ANNOTATION: NFRs with specific numbers become inputs for stress tests.
  If you define a latency SLA (like p95 < 200ms), Gate G026 requires a stress
  test in the DoD. Always include atomicity, logging, and security NFRs.
-->

| ID | Requirement | Measurement |
|----|-------------|-------------|
| NFR-01 | GET/PUT profile latency p95 < 200ms under 100 concurrent users | Stress test with k6 or equivalent |
| NFR-02 | Profile updates are atomic — partial field writes never visible | Integration test with concurrent writes |
| NFR-03 | All profile operations logged with user_id and action for audit | Log output verification in tests |
| NFR-04 | No sensitive data (password hash, internal IDs beyond user_id) leaked in responses | E2E response body assertions |
| NFR-05 | Rate limiting: max 60 profile updates per user per minute | Stress test |

---

# ARTIFACT 2: design.md

<!--
  📝 ANNOTATION: design.md is the HOW document. It describes the technical
  approach: API surface, data schema, handler logic, error formats. This is
  where you commit to specific technologies (Rust, PostgreSQL, etc).
-->

# User Profile API — Design

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md)

## API Surface

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/users/me` | JWT (any role) | Returns the authenticated user's profile |
| `PUT` | `/api/v1/users/me` | JWT (any role) | Updates the authenticated user's profile |
| `GET` | `/api/v1/admin/users/:id` | JWT (admin only) | Returns any user's profile by ID |

## Database Schema

<!--
  📝 ANNOTATION: Always include constraints, indexes, and defaults. The schema
  in design.md is the source of truth — the migration file must match exactly.
-->

```sql
CREATE TABLE user_profiles (
    id             UUID PRIMARY KEY,              -- matches auth system user ID
    name           VARCHAR(200) NOT NULL,
    email          VARCHAR(255) NOT NULL UNIQUE,   -- immutable via profile API
    bio            TEXT NOT NULL DEFAULT '',        -- max 2000 chars enforced in app
    preferences    JSONB NOT NULL DEFAULT '{}',    -- validated against allowed keys
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for admin lookup by ID (primary key already covers this)
-- Index for email uniqueness (already covered by UNIQUE constraint)

-- Allowed preference keys enforced at application layer:
-- "theme", "language", "timezone", "notifications_enabled", "email_digest"
```

## Allowed Preference Keys

<!--
  📝 ANNOTATION: Define enum-like values explicitly so tests can validate
  rejection of unknown keys. This list appears in spec (UC-06d) and is
  enforced by the handler validation logic.
-->

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `theme` | `string` ("light" \| "dark") | `"light"` | UI theme preference |
| `language` | `string` (ISO 639-1) | `"en"` | Display language |
| `timezone` | `string` (IANA tz) | `"UTC"` | User timezone |
| `notifications_enabled` | `boolean` | `true` | Email notification toggle |
| `email_digest` | `string` ("daily" \| "weekly" \| "never") | `"weekly"` | Digest frequency |

## Handler Pseudocode

### GET /api/v1/users/me

```
fn get_own_profile(req: AuthenticatedRequest) -> Response:
    user_id = req.auth.user_id
    profile = repo.get_profile_by_id(user_id)
    if profile is None:
        // Should not happen — profile created at registration
        return 500, InternalError("profile missing for authenticated user")
    return 200, serialize(profile, exclude=["password_hash"])
```

### PUT /api/v1/users/me

```
fn update_own_profile(req: AuthenticatedRequest, body: UpdateProfileRequest) -> Response:
    // 1. Validate: reject if "email" field is present
    if body.has("email"):
        return 400, ValidationError("email", "email cannot be changed via profile update")

    // 2. Validate individual fields
    errors = validate_profile_fields(body)
    if errors:
        return 400, ValidationError(errors)

    // 3. Apply update
    user_id = req.auth.user_id
    updated = repo.update_profile(user_id, body.name, body.bio, body.preferences)
    return 200, serialize(updated)
```

### GET /api/v1/admin/users/:id

```
fn admin_get_user_profile(req: AuthenticatedRequest, path_id: UUID) -> Response:
    // Auth middleware already verified admin role
    profile = repo.get_profile_by_id(path_id)
    if profile is None:
        return 404, NotFoundError("USER_NOT_FOUND")
    return 200, serialize(profile, exclude=["password_hash"])
```

## Validation Rules

<!--
  📝 ANNOTATION: This table is the definitive reference for what the handler
  validates. Each row should map to a Gherkin scenario in spec.md and a test
  case in scopes.md.
-->

| Field | Rule | Error Code | Scenario |
|-------|------|------------|----------|
| `name` | Required, 1–200 characters | `VALIDATION_ERROR` | UC-06a, UC-06b |
| `bio` | Optional, max 2000 characters | `VALIDATION_ERROR` | UC-06c |
| `preferences` | Keys must be in allowed set | `VALIDATION_ERROR` | UC-06d |
| `email` | Must NOT be present in update body | `VALIDATION_ERROR` | UC-06e |

## Error Response Format

<!--
  📝 ANNOTATION: Define the error envelope once. Every error scenario in spec.md
  must produce a response in this format. Consistency across endpoints prevents
  frontend bugs.
-->

All error responses follow this envelope:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "One or more fields failed validation",
    "details": [
      {
        "field": "name",
        "reason": "must not be empty"
      }
    ]
  }
}
```

### Error Response Examples

| Scenario | Status | Code | Message |
|----------|--------|------|---------|
| Missing auth header | 401 | `MISSING_AUTH` | "Authorization header required" |
| Expired JWT | 401 | `TOKEN_EXPIRED` | "Token has expired" |
| Malformed JWT | 401 | `TOKEN_INVALID` | "Token is not valid" |
| Non-admin on admin endpoint | 403 | `FORBIDDEN` | "Admin role required" |
| Empty name | 400 | `VALIDATION_ERROR` | "One or more fields failed validation" |
| Admin requests nonexistent user | 404 | `USER_NOT_FOUND` | "User not found" |

## Database Query Approach

- All queries use **prepared statements** (parameterized queries) — no string interpolation
- Profile read: single `SELECT` by primary key — O(1) index lookup
- Profile update: single `UPDATE ... SET ... WHERE id = $1 RETURNING *` — atomic, returns updated row
- No N+1 queries — each handler issues exactly one SQL statement
- Connection pooling via application connection pool (e.g., `sqlx::PgPool`)

## Security Considerations

- JWT validation happens in middleware before any handler code runs
- Admin role check uses middleware, not inline handler logic
- `password_hash` and any internal fields excluded from all serialization
- SQL injection prevented by parameterized queries exclusively
- Rate limiting enforced at gateway level (60 PUT/min per user)
- All profile mutations logged with `user_id`, `action`, and `timestamp`

---

# ARTIFACT 3: scopes.md

<!--
  📝 ANNOTATION: scopes.md breaks the feature into implementable chunks.
  Each scope has its own Gherkin scenarios (derived from spec.md), test plan,
  and Definition of Done. Scopes form a DAG via "Depends On" declarations.

  The two tiers in DoD are:
  - Core Items: scope-specific, each gets individual evidence blocks
  - Build Quality Gate: standard checks grouped into one combined block
-->

# Scopes

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 01: Database & Repository Layer

**Status:** Not Started
**Priority:** P0
**Depends On:** None

<!--
  📝 ANNOTATION: This scope covers the data layer only — schema + repository
  functions. No HTTP handlers, no auth. This separation lets you test the
  repository in isolation before wiring up the API.
-->

### Gherkin Scenarios

```gherkin
Scenario: Read existing user profile by ID
  Given a user with id "uuid-alice" exists in the database
  When the repository function get_profile_by_id("uuid-alice") is called
  Then it returns the complete profile record including name, email, bio, preferences, timestamps

Scenario: Read nonexistent user returns None
  Given no user with id "uuid-nonexistent" exists in the database
  When the repository function get_profile_by_id("uuid-nonexistent") is called
  Then it returns None

Scenario: Update user profile fields
  Given a user "alice" exists with name "Old Name" and bio "Old Bio"
  When update_profile("uuid-alice", name="New Name", bio="New Bio", preferences=null) is called
  Then the returned profile has name "New Name" and bio "New Bio"
  And the updated_at timestamp is later than before the call
  And the email remains unchanged

Scenario: Update preserves unmodified fields
  Given a user "alice" exists with name "Alice" and bio "My bio" and preferences {"theme": "dark"}
  When update_profile("uuid-alice", name="Alice Updated", bio=null, preferences=null) is called
  Then the returned profile has name "Alice Updated"
  And the bio remains "My bio"
  And the preferences remain {"theme": "dark"}

Scenario: Database migration creates schema correctly
  Given a fresh database with no user_profiles table
  When the migration is applied
  Then the user_profiles table exists with columns: id, name, email, bio, preferences, created_at, updated_at
  And the email column has a UNIQUE constraint
  And the bio column has a NOT NULL constraint with default ''
  And the preferences column has a NOT NULL constraint with default '{}'
```

### Implementation Plan

1. Create migration file: `migrations/YYYYMMDD_create_user_profiles.up.sql`
2. Create down migration: `migrations/YYYYMMDD_create_user_profiles.down.sql`
3. Implement `UserProfile` model struct with serde serialization
4. Implement `UserProfileRepository` trait with `get_by_id` and `update` methods
5. Implement PostgreSQL repository with parameterized queries
6. Write unit tests for model serialization
7. Write functional tests against real test database

### Test Plan

<!--
  📝 ANNOTATION: Every row here must have a matching DoD checkbox item.
  Row count must equal DoD test-related item count (this is enforced by Gate 5).
  The "Description" column should reference specific Gherkin scenarios.
-->

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Unit | `unit` | `tests/unit/models/user_profile_test.rs` | UserProfile model serialization, field defaults, preference key validation | `./project.sh dev test --rust` | No |
| Unit | `unit` | `tests/unit/repo/user_profile_repo_test.rs` | Repository method input validation (UUID format, field lengths) | `./project.sh dev test --rust` | No |
| Functional | `functional` | `tests/functional/repo/user_profile_test.rs` | Scenarios: read existing, read nonexistent, update fields, preserve unmodified (real test DB) | `./project.sh dev test --rust` | Yes |
| Functional | `functional` | `tests/functional/migrations/user_profiles_test.rs` | Scenario: migration creates correct schema with constraints | `./project.sh dev test --rust` | Yes |

### Definition of Done — Tiered Validation

<!--
  📝 ANNOTATION: The DoD uses a two-tier structure:
  1. Core Items — scope-specific, each with its own evidence block
  2. Build Quality Gate — standard checks grouped together

  EVERY checkbox must start unchecked [ ]. They get checked [x] ONLY when
  an agent pastes real terminal output into the evidence block.

  The evidence block format is:
    ```
    [PASTE VERBATIM terminal output here]
    ```
  This gets replaced with actual command output during implementation.
-->

#### Core Items (scope-specific — each needs individual inline evidence)

- [ ] Migration creates `user_profiles` table with all columns, constraints, and defaults per design.md
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — migration apply + schema inspection]
    ```
- [ ] Down migration cleanly drops table
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — down migration + verify table gone]
    ```
- [ ] `get_by_id` returns complete profile for existing user (Gherkin: "Read existing user profile")
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — functional test output showing pass]
    ```
- [ ] `get_by_id` returns None for nonexistent user (Gherkin: "Read nonexistent user returns None")
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — functional test output showing pass]
    ```
- [ ] `update_profile` modifies specified fields and updates timestamp (Gherkin: "Update user profile fields")
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — functional test output showing pass]
    ```
- [ ] `update_profile` preserves null/unspecified fields (Gherkin: "Update preserves unmodified fields")
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — functional test output showing pass]
    ```
- [ ] UserProfile model serialization excludes sensitive fields (password_hash)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — unit test output showing pass]
    ```
- [ ] Preference key validation rejects unknown keys
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — unit test output showing pass]
    ```
- [ ] Unit tests pass (`unit`) — model + repo input validation
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — test runner showing pass/fail counts]
    ```
- [ ] Functional tests pass (`functional`) — all repository scenarios against real test DB
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — test runner showing pass/fail counts]
    ```

#### Build Quality Gate (standard — single combined evidence block)

- [ ] Integration completeness verified (Gate G029) — repository module imported by handler layer, migration file included in migration runner
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM grep output showing imports/usage]
    ```
- [ ] No defaults/fallbacks in production code (Gate G028) — zero `unwrap_or()`, `|| default`, `?? fallback` patterns
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM grep output showing no matches]
    ```
- [ ] Build quality gate passes: zero warnings + no TODOs/FIXMEs/stubs + lint/format clean + artifact lint exits 0 + documentation updated
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM combined build, grep, lint, artifact-lint output]
    ```

---

## Scope 02: API Handlers + Auth Middleware + E2E

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 01

<!--
  📝 ANNOTATION: This scope wires up the HTTP layer on top of the repository
  from Scope 01. It includes auth middleware, validation, all error handling,
  and the full E2E + stress test suite. This is where every spec.md scenario
  gets its E2E test.
-->

### Gherkin Scenarios

<!--
  📝 ANNOTATION: Every acceptance criterion from spec.md appears here as a
  concrete Gherkin scenario. Each scenario gets its own E2E test AND a
  matching DoD item. The count matters: 14 scenarios → 14+ E2E tests →
  14+ DoD items referencing them.
-->

```gherkin
# Happy path
Scenario: GET /api/v1/users/me returns own profile (UC-01)
  Given user "alice" is authenticated with a valid JWT
  When alice sends GET /api/v1/users/me
  Then the response status is 200
  And the body contains alice's name, email, bio, preferences, updatedAt

Scenario: PUT /api/v1/users/me updates profile (UC-02)
  Given user "alice" is authenticated
  When alice sends PUT /api/v1/users/me with {"name": "Alice Smith", "bio": "Updated"}
  Then the response status is 200
  And the body reflects the updated fields
  And updatedAt is later than before

Scenario: Admin GET /api/v1/admin/users/:id returns other user (UC-03)
  Given admin "admin01" is authenticated
  And user "bob" exists
  When admin01 sends GET /api/v1/admin/users/{bob_id}
  Then the response status is 200
  And the body contains bob's profile

# Auth failures
Scenario: Missing Authorization header → 401 MISSING_AUTH (UC-04a)
  When a client sends GET /api/v1/users/me without Authorization header
  Then the response status is 401
  And error code is "MISSING_AUTH"

Scenario: Expired JWT → 401 TOKEN_EXPIRED (UC-04b)
  Given a client has a JWT that expired 5 minutes ago
  When the client sends GET /api/v1/users/me
  Then the response status is 401
  And error code is "TOKEN_EXPIRED"

Scenario: Malformed JWT → 401 TOKEN_INVALID (UC-04c)
  Given a client has Authorization "Bearer not-a-jwt"
  When the client sends GET /api/v1/users/me
  Then the response status is 401
  And error code is "TOKEN_INVALID"

# Authorization
Scenario: Non-admin on admin endpoint → 403 FORBIDDEN (UC-05)
  Given user "alice" is authenticated with role "user" (not admin)
  When alice sends GET /api/v1/admin/users/{some_id}
  Then the response status is 403
  And error code is "FORBIDDEN"

# Validation
Scenario: Empty name → 400 VALIDATION_ERROR (UC-06a)
  Given user "alice" is authenticated
  When alice sends PUT /api/v1/users/me with {"name": ""}
  Then the response status is 400
  And error details include field "name" reason "must not be empty"

Scenario: Name exceeds 200 chars → 400 (UC-06b)
  Given user "alice" is authenticated
  When alice sends PUT with a 201-character name
  Then the response status is 400
  And error details include field "name" reason "must be at most 200 characters"

Scenario: Bio exceeds 2000 chars → 400 (UC-06c)
  Given user "alice" is authenticated
  When alice sends PUT with a 2001-character bio
  Then the response status is 400
  And error details include field "bio" reason "must be at most 2000 characters"

Scenario: Unknown preference key → 400 (UC-06d)
  Given user "alice" is authenticated
  When alice sends PUT with preferences {"invalid_pref": true}
  Then the response status is 400
  And error details include field "preferences" reason "unknown key: invalid_pref"

Scenario: Email change attempt → 400 (UC-06e)
  Given user "alice" is authenticated
  When alice sends PUT with {"email": "new@example.com"}
  Then the response status is 400
  And error details include field "email" reason "email cannot be changed via profile update"

# Not found
Scenario: Admin requests nonexistent user → 404 (UC-07)
  Given admin "admin01" is authenticated
  When admin01 sends GET /api/v1/admin/users/uuid-nonexistent
  Then the response status is 404
  And error code is "USER_NOT_FOUND"

# Concurrency & idempotency
Scenario: Concurrent updates — last write wins
  Given user "alice" sends PUT with {"name": "A"} and then PUT with {"name": "B"}
  Then the final stored name is "B"

Scenario: Idempotent PUT — same payload twice yields same result
  Given user "alice" sends PUT with {"name": "Alice"} twice
  Then both responses have status 200
  And the final name is "Alice"
```

### Implementation Plan

1. Implement JWT auth middleware (extract user_id, role from token; return 401 on failure)
2. Implement admin role guard middleware (return 403 if role != admin)
3. Implement `UpdateProfileRequest` deserialization with validation
4. Implement `GET /api/v1/users/me` handler using repository from Scope 01
5. Implement `PUT /api/v1/users/me` handler with validation + repository
6. Implement `GET /api/v1/admin/users/:id` handler with admin guard + repository
7. Register routes in application router
8. Write E2E tests for every Gherkin scenario
9. Write stress tests for NFR-01 (p95 < 200ms under 100 concurrent users)

### Test Plan

<!--
  📝 ANNOTATION: Each E2E row names the SPECIFIC Gherkin scenario it validates.
  Generic descriptions like "API workflow" are forbidden. Stress tests are
  REQUIRED because spec.md defines NFR-01 with a latency SLA.
-->

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Unit | `unit` | `tests/unit/handlers/profile_handler_test.rs` | Handler validation logic: empty name, long name, long bio, unknown prefs, email rejection | `./project.sh dev test --rust` | No |
| Unit | `unit` | `tests/unit/middleware/auth_test.rs` | JWT extraction, expiry detection, malformation detection | `./project.sh dev test --rust` | No |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: GET own profile returns 200 with correct fields (UC-01) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: PUT own profile updates fields and timestamp (UC-02) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Admin GET other user's profile (UC-03) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Missing auth header → 401 MISSING_AUTH (UC-04a) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Expired JWT → 401 TOKEN_EXPIRED (UC-04b) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Malformed JWT → 401 TOKEN_INVALID (UC-04c) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Non-admin on admin endpoint → 403 FORBIDDEN (UC-05) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Empty name → 400 VALIDATION_ERROR (UC-06a) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Name > 200 chars → 400 (UC-06b) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Bio > 2000 chars → 400 (UC-06c) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Unknown preference key → 400 (UC-06d) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Email update attempt → 400 (UC-06e) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Admin requests nonexistent user → 404 (UC-07) | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Concurrent updates — last write wins | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Scenario: Idempotent PUT — repeated identical update | `./project.sh dev test` | Yes |
| E2E API | `e2e-api` | `tests/e2e/api/user_profile_test.rs` | Round-trip: PUT profile → GET profile → verify fields match | `./project.sh dev test` | Yes |
| Stress | `stress` | `tests/stress/user_profile_load.rs` | NFR-01: 100 concurrent users, GET+PUT mix, p95 < 200ms, 60s sustained | `./project.sh dev test --stress` | Yes |
| Stress | `stress` | `tests/stress/user_profile_rate_limit.rs` | NFR-05: 61+ PUTs in 60s from same user → rate limited | `./project.sh dev test --stress` | Yes |

### Definition of Done — Tiered Validation

#### Core Items (scope-specific — each needs individual inline evidence)

<!--
  📝 ANNOTATION: Notice how each DoD item references a specific Gherkin scenario
  ID (UC-01, UC-04a, etc.) and specifies the test category. This traceability
  is how agents verify that every scenario has real evidence.
-->

- [ ] Auth middleware extracts user_id and role from valid JWT — handler receives authenticated context
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — unit test for auth middleware]
    ```
- [ ] Auth middleware returns 401 MISSING_AUTH for missing header (UC-04a)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-04a]
    ```
- [ ] Auth middleware returns 401 TOKEN_EXPIRED for expired JWT (UC-04b)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-04b]
    ```
- [ ] Auth middleware returns 401 TOKEN_INVALID for malformed JWT (UC-04c)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-04c]
    ```
- [ ] Admin guard returns 403 FORBIDDEN for non-admin role (UC-05)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-05]
    ```
- [ ] GET /api/v1/users/me returns 200 with correct profile fields (UC-01)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-01]
    ```
- [ ] PUT /api/v1/users/me updates fields and returns 200 with updated profile (UC-02)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-02]
    ```
- [ ] GET /api/v1/admin/users/:id returns 200 with correct profile for admin (UC-03)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-03]
    ```
- [ ] Empty name rejected with 400 and field-level error (UC-06a)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-06a]
    ```
- [ ] Name > 200 chars rejected with 400 (UC-06b)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-06b]
    ```
- [ ] Bio > 2000 chars rejected with 400 (UC-06c)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-06c]
    ```
- [ ] Unknown preference keys rejected with 400 (UC-06d)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-06d]
    ```
- [ ] Email update attempt rejected with 400 (UC-06e)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-06e]
    ```
- [ ] Admin requesting nonexistent user gets 404 USER_NOT_FOUND (UC-07)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for UC-07]
    ```
- [ ] Concurrent updates use last-write-wins correctly
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for concurrency scenario]
    ```
- [ ] Idempotent PUT produces consistent results
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api test for idempotency scenario]
    ```
- [ ] Round-trip verification: PUT fields → GET → verify all fields match
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api round-trip test]
    ```
- [ ] password_hash and internal fields excluded from all API responses (NFR-04)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — e2e-api response body assertion]
    ```
- [ ] All operations logged with user_id and action (NFR-03)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — log output from test run showing structured audit entries]
    ```
- [ ] Stress test: p95 < 200ms under 100 concurrent users for GET+PUT (NFR-01)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — stress test results with latency percentiles]
    ```
- [ ] Stress test: rate limiting enforced at 60 PUTs/min per user (NFR-05)
  - Raw output evidence (inline, verbatim terminal output only):
    ```
    [PASTE VERBATIM terminal output — rate limit stress test results]
    ```

#### Build Quality Gate (standard — single combined evidence block)

- [ ] E2E test suite passes — all 18 scenario tests green
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM terminal output — full e2e test run with pass/fail counts]
    ```
- [ ] Stress test suite passes — both load and rate-limit scenarios
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM terminal output — stress test summary]
    ```
- [ ] Integration completeness verified (Gate G029) — routes registered in router, handlers call repository from Scope 01, middleware wired into route groups
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM grep output showing route registration + handler→repo imports]
    ```
- [ ] Vertical slice complete (Gate G035) — endpoints registered in router, called by at least one frontend or documented as API-only
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM cross-reference output]
    ```
- [ ] No defaults/fallbacks in production code (Gate G028)
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM grep output showing no matches for unwrap_or, default, fallback]
    ```
- [ ] Build quality gate passes: zero warnings + no TODOs/FIXMEs/stubs + lint/format clean + artifact lint exits 0 + documentation updated
  - Raw output evidence (inline):
    ```
    [PASTE VERBATIM combined build, grep, lint, artifact-lint output]
    ```
