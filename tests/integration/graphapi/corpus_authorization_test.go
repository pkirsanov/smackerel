//go:build integration

// Spec 080 / BUG-080-001 SCOPE-02 — T080-09-CORPUS (SCN-080-001-09).
//
// THE SINGLE OPERATOR-OWNED GLOBAL CORPUS, THREE-IDENTITY GRANT MATRIX.
//
// design.md "## Corpus Ownership And Authorization Model" establishes ONE
// operator-owned global knowledge corpus read by three authenticated
// identity classes. Authorization is by GRANT, never by a tenant or
// per-user ROW partition:
//
//	| identity      | private graph content | operational metadata | denial |
//	| operator      | all                   | may read             | n/a    |
//	| grant-holder  | authorized global projection of the SAME rows | no | n/a |
//	| ungranted     | none                  | none                 | leak-free 403 |
//
// This file proves that matrix END-TO-END through the REAL production
// router (internal/api.NewRouter — the router cmd/core builds at boot),
// over a REAL loopback HTTP server (httptest), against the disposable
// stack's REAL PostgreSQL (DATABASE_URL), with the REAL per-user PASETO
// bearer middleware and the REAL auth.RequireScope("knowledge-graph:read")
// gate. There is NO request interception, NO mock, NO stub.
//
// Why in-process rather than the running smackerel-core container? The
// LIVE container CANNOT express this matrix: auth_test.go's
// TestGraphAPI_403_MissingScope_LiveStackConstraint documents that the
// test stack runs AUTH_ENABLED=false, so the shared bearer collapses
// RequireScope to its SessionSourceSharedToken bypass branch and EVERY
// identity looks identical. Constructing api.NewRouter in-process with
// Environment="production" + AuthConfig.Enabled=true (mirroring
// cmd/core/wiring.go's per-user branch) is the ONLY way to vary the scope
// set PER REQUEST while keeping the router, the middleware chain, the
// PostgreSQL-backed handlers, and the scope gate real. That is exactly
// the "flavor of the test stack with AUTH_ENABLED=true" the constraint
// test routed to bubbles.implement.
//
// The three bearers are all real credentials verified by real code:
//   - operator     — the shared operator token, admitted by
//     bearerAuthMiddleware branch 2 as SessionSourceSharedToken
//     (Session.IsAdmin() == true), which RequireScope bypasses.
//   - grant-holder — a real PASETO v4.public token minted by
//     auth.IssueToken carrying scope ["knowledge-graph:read"].
//   - ungranted    — a real PASETO v4.public token minted by the SAME
//     key carrying a DIFFERENT real product scope ["annotation:edit"].
//     Authentication succeeds; the grant is absent.
//
// Adversarial value. This test FAILS if a regression:
//   - grants the ungranted identity any graph read (403 -> 2xx/404),
//   - leaks a seeded id/label/title, a count, or an existence hint into
//     the denial body, or makes "denied" distinguishable from "absent",
//   - introduces a per-user/tenant WHERE predicate on any read path, so
//     the operator and the grant-holder stop observing the SAME global
//     row set,
//   - silently promotes a grant-holder to the operator metadata tier, or
//   - drifts the live router away from the documented grant model in
//     internal/api/graphapi/activation.go.

package graphapi_integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
)

const (
	// corpusIssuer is the `iss` claim shared by the minting and the
	// verifying side, exactly as cmd/core wires it.
	corpusIssuer = "smackerel"
	// corpusKeyID is the footer kid routing verification to the active
	// signing key.
	corpusKeyID = "bug080it-corpus-active-k1"
	// corpusOperatorToken is the shared OPERATOR bearer. It is a test
	// value generated for this file only; it never appears in config.
	corpusOperatorToken = "bug080it-corpus-operator-shared-bearer" //gitleaks:allow
	// corpusOtherScope is a REAL product scope (router.go gates the
	// annotation surface with it) that is NOT the graph read grant. The
	// ungranted identity is authenticated and scoped — just not for the
	// graph.
	corpusOtherScope = "annotation:edit"
)

// corpusIdentity binds a real wire bearer to the documented grant class
// its GraphIdentity classifies to, and to the HTTP outcome the live
// router MUST produce for it. Holding both sides in one record is what
// lets the assertions bind the live router to the documented model
// instead of testing them independently.
type corpusIdentity struct {
	name       string
	bearer     string
	identity   graphapi.GraphIdentity
	wantGrant  graphapi.GraphGrant
	wantStatus int
}

// corpusListPaths are the parameter-free graph family list endpoints.
// Both authorized identities MUST read them; the row-set comparison runs
// over topics and people because those carry seeded, deterministic ids.
var corpusListPaths = []string{"/api/topics?limit=200", "/api/people?limit=200", "/api/places?limit=200"}

// mintCorpusToken issues a REAL PASETO v4.public token under the supplied
// signing key with the supplied scope claim. No fake token, no stub
// verifier: bearerAuthMiddleware verifies this exact wire token with
// auth.VerifyAndParse.
func mintCorpusToken(t *testing.T, privateHex, userID string, scopes []string) string {
	t.Helper()
	res, err := auth.IssueToken(auth.IssueOptions{
		UserID:     userID,
		TokenID:    userID + "-token",
		SigningKey: privateHex,
		KeyID:      corpusKeyID,
		TTL:        time.Hour,
		Issuer:     corpusIssuer,
		Now:        time.Now,
		Scopes:     scopes,
	})
	if err != nil {
		t.Fatalf("IssueToken(%s, scopes=%v): %v", userID, scopes, err)
	}
	return res.WireToken
}

// newCorpusAuthRouter builds the REAL production router with per-user
// PASETO validation ACTIVE (Environment="production" +
// AuthConfig.Enabled=true — bearerAuthMiddleware's perUserActive branch)
// plus the operator shared-token fallback, over the live
// PostgreSQL-backed graph handlers. This mirrors cmd/core/wiring.go's
// enabled branch; only the auth profile differs from
// newGraphRouterOverPool, which pins Environment="test" and therefore
// cannot vary scopes per request.
func newCorpusAuthRouter(
	t *testing.T,
	pool *pgxpool.Pool,
	codec *graphapi.CursorCodec,
	limits graphapi.Limits,
	graphCap *graphapi.GraphCapability,
	activePublicHex string,
) http.Handler {
	t.Helper()
	return api.NewRouter(&api.Dependencies{
		Environment: "production",
		AuthToken:   corpusOperatorToken,
		AuthConfig: config.AuthConfig{
			Enabled:     true,
			TokenFormat: "paseto-v4-public",
			// The operator identity authenticates with the shared
			// operator bearer through the supported production
			// fallback; that yields SessionSourceSharedToken, which
			// Session.IsAdmin() reports as the operator class.
			ProductionSharedTokenFallbackEnabled: true,
		},
		AuthVerifyOptions: auth.VerifyOptions{
			ActivePublicKey:    activePublicHex,
			ActiveKeyID:        corpusKeyID,
			Issuer:             corpusIssuer,
			ClockSkewTolerance: 30 * time.Second,
			Now:                time.Now,
		},
		RevocationCache: revocation.NewCache(),
		GraphCapability: graphCap,
		TopicsHandlers:  graphapi.NewTopicsHandlers(pool, limits, codec),
		PeopleHandlers:  graphapi.NewPeopleHandlers(pool, limits, codec),
		PlacesHandlers:  graphapi.NewPlacesHandlers(pool, limits, codec),
		TimeHandlers:    graphapi.NewTimeHandlers(pool, limits),
		EdgesHandlers:   graphapi.NewEdgesHandlers(pool, limits, codec),
	})
}

// corpusGET drives one bearer against one path on the real router.
func corpusGET(t *testing.T, base, bearer, path string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatalf("NewRequest(%s): %v", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body for GET %s: %v", path, err)
	}
	return resp, body
}

// corpusIDField extracts the "id" of every element of the response's
// "items" array without binding to a per-family struct, so one collector
// serves topics, people, and places.
func corpusIDField(t *testing.T, label string, body []byte) []string {
	t.Helper()
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("%s: decode items: %v body=%s", label, err, string(body))
	}
	out := make([]string, 0, len(page.Items))
	for _, it := range page.Items {
		if it.ID == "" {
			t.Fatalf("%s: authorized item carries an empty id — the projection is malformed. body=%s", label, string(body))
		}
		out = append(out, it.ID)
	}
	return out
}

// corpusSeededSubset restricts an observed id list to this run's unique
// fixture prefix, so the comparison is immune to unrelated rows that
// other integration packages may write concurrently.
func corpusSeededSubset(ids []string, prefix string) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		if strings.HasPrefix(id, prefix) {
			out[id] = true
		}
	}
	return out
}

// corpusSameSet reports set equality between two id collections.
func corpusSameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// corpusSameSequence reports order-sensitive equality of two observed id
// pages. Used only to establish that the corpus was STABLE across the
// comparison window before the strongest whole-page assertion runs.
func corpusSameSequence(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// assertNoCountLikeField walks a decoded denial body and fails on ANY
// key that would disclose corpus size or content shape — `total`,
// anything containing `count`, an `items` collection, or a pagination
// cursor. An ungranted caller must not be able to infer that the corpus
// exists, let alone how large it is.
func assertNoCountLikeField(t *testing.T, label string, v any, path string) {
	t.Helper()
	switch node := v.(type) {
	case map[string]any:
		for k, child := range node {
			lower := strings.ToLower(k)
			if lower == "total" || lower == "items" || lower == "nextcursor" || strings.Contains(lower, "count") {
				t.Fatalf("%s: leak — denial body exposes corpus-shape field %q at %s. A denied identity must not learn the corpus exists or how big it is.",
					label, k, path+"."+k)
			}
			assertNoCountLikeField(t, label, child, path+"."+k)
		}
	case []any:
		for i, child := range node {
			assertNoCountLikeField(t, label, child, path+"[]")
			_ = i
		}
	}
}

// assertDenialLeaksNothing is the leak-free denial contract: 403, no
// seeded id/label/title anywhere in the bytes, and no count/total/items
// disclosure.
func assertDenialLeaksNothing(t *testing.T, label string, resp *http.Response, body []byte, secrets []string) {
	t.Helper()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("%s: status=%d; want 403 unauthorized-scope. An authenticated identity WITHOUT %s must be denied, never served. body=%s",
			label, resp.StatusCode, graphapi.GraphReadScope, string(body))
	}
	lower := strings.ToLower(string(body))
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(secret)) {
			t.Fatalf("%s: leak — denial body discloses seeded graph content %q. body=%s", label, secret, string(body))
		}
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		// A non-JSON denial body is itself a contract break: the denial
		// must be the typed envelope, not an arbitrary payload.
		t.Fatalf("%s: denial body is not JSON: %v body=%s", label, err, string(body))
	}
	assertNoCountLikeField(t, label, decoded, "$")
}

// TestGlobalCorpusGrantMatrixOperatorGrantedUngrantedNoRowIsolation is
// T080-09-CORPUS: the SCN-080-001-09 three-identity authorization matrix
// over the single operator-owned global corpus, plus the explicit
// no-per-identity-row-isolation proof.
func TestGlobalCorpusGrantMatrixOperatorGrantedUngrantedNoRowIsolation(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live stack not available")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	// ---- Disposable GLOBAL fixtures, tagged with a unique prefix ----
	// These rows belong to the ONE global corpus. Nothing about them is
	// owned by, or scoped to, any of the three identities below — which
	// is precisely the property the row-isolation assertion checks.
	prefix := fixturePrefix(t)
	defer cleanupFixtures(t, conn, prefix)
	topicIDs := seedTopics(t, conn, prefix, 3)
	peopleIDs := seedPeople(t, conn, prefix, 2)
	artifactIDs := seedArtifacts(t, conn, prefix, 2)

	// Every seeded id doubles as its label/name (see helpers_test.go), and
	// seedArtifacts writes "<id>-title". This is the closed set of strings
	// that MUST NOT appear in an ungranted denial body.
	seededSecrets := make([]string, 0, len(topicIDs)+len(peopleIDs)+2*len(artifactIDs))
	seededSecrets = append(seededSecrets, topicIDs...)
	seededSecrets = append(seededSecrets, peopleIDs...)
	for _, id := range artifactIDs {
		seededSecrets = append(seededSecrets, id, id+"-title")
	}

	// ---- Real signing key + real minted bearers ----
	privateHex, publicHex := auth.GenerateSigningKeypair()
	grantHolderBearer := mintCorpusToken(t, privateHex, prefix+"-grant-holder", []string{graphapi.GraphReadScope})
	ungrantedBearer := mintCorpusToken(t, privateHex, prefix+"-ungranted", []string{corpusOtherScope})

	graphCap, codec, limits := enabledGraphWiring(t, "CORPUS")
	srv := httptest.NewServer(newCorpusAuthRouter(t, pool, codec, limits, graphCap, publicHex))
	t.Cleanup(srv.Close)

	// The GraphIdentity for each bearer is built from EXACTLY the inputs
	// design.md prescribes for the router adapter: Operator =
	// Session.IsAdmin() (the shared-token/bootstrap sources), Grants =
	// Session.Scopes, Authenticated = a session exists. No tenant id, no
	// owner id, no row selector — the model structurally cannot express
	// per-user row isolation.
	operator := corpusIdentity{
		name:       "operator",
		bearer:     corpusOperatorToken,
		identity:   graphapi.GraphIdentity{Authenticated: true, Operator: true},
		wantGrant:  graphapi.GrantOperator,
		wantStatus: http.StatusOK,
	}
	grantHolder := corpusIdentity{
		name:       "grant-holder",
		bearer:     grantHolderBearer,
		identity:   graphapi.GraphIdentity{Authenticated: true, Grants: []string{graphapi.GraphReadScope}},
		wantGrant:  graphapi.GrantHolder,
		wantStatus: http.StatusOK,
	}
	ungranted := corpusIdentity{
		name:       "ungranted",
		bearer:     ungrantedBearer,
		identity:   graphapi.GraphIdentity{Authenticated: true, Grants: []string{corpusOtherScope}},
		wantGrant:  graphapi.GrantNone,
		wantStatus: http.StatusForbidden,
	}
	matrix := []corpusIdentity{operator, grantHolder, ungranted}

	// ---------------------------------------------------------------
	// (1) The three-identity grant matrix, live router bound to model.
	// ---------------------------------------------------------------
	t.Run("grant_matrix_binds_live_router_to_documented_model", func(t *testing.T) {
		for _, id := range matrix {
			gotGrant := graphapi.ClassifyGraphGrant(id.identity)
			if gotGrant != id.wantGrant {
				t.Fatalf("%s: ClassifyGraphGrant=%q; want %q — the documented global-corpus grant model drifted",
					id.name, gotGrant, id.wantGrant)
			}
			denial := graphapi.AuthorizeGraphRead(gotGrant)
			switch id.wantStatus {
			case http.StatusOK:
				if denial != nil {
					t.Fatalf("%s: AuthorizeGraphRead(%q) denied with %+v; want authorized", id.name, gotGrant, denial)
				}
			default:
				if denial == nil {
					t.Fatalf("%s: AuthorizeGraphRead(%q) authorized an ungranted identity; want the leak-free denial", id.name, gotGrant)
				}
				if denial.Status != http.StatusForbidden {
					t.Fatalf("%s: denial status=%d; want 403 (the typed unauthorized-scope outcome)", id.name, denial.Status)
				}
				if denial.Code != "forbidden" {
					t.Fatalf("%s: denial code=%q; want \"forbidden\"", id.name, denial.Code)
				}
			}

			// The LIVE router MUST reach the same verdict on every
			// parameter-free family. Any divergence between the model and
			// the wire is the regression this row binds.
			for _, path := range corpusListPaths {
				resp, body := corpusGET(t, srv.URL, id.bearer, path)
				if resp.StatusCode != id.wantStatus {
					t.Fatalf("%s: GET %s status=%d; want %d (model grant=%q). body=%s",
						id.name, path, resp.StatusCode, id.wantStatus, gotGrant, string(body))
				}
			}
		}
	})

	// ---------------------------------------------------------------
	// (2) Operator reads all private content PLUS the operational tier;
	//     the grant-holder reads content but is NOT promoted to operator.
	// ---------------------------------------------------------------
	t.Run("operator_tier_is_a_superset_and_is_not_granted_to_a_grant_holder", func(t *testing.T) {
		// Operator sees every seeded row of private graph content.
		for _, path := range []string{"/api/topics?limit=200", "/api/people?limit=200"} {
			resp, body := corpusGET(t, srv.URL, operator.bearer, path)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("operator: GET %s status=%d; want 200. body=%s", path, resp.StatusCode, string(body))
			}
		}
		_, topicsBody := corpusGET(t, srv.URL, operator.bearer, "/api/topics?limit=200")
		seen := corpusSeededSubset(corpusIDField(t, "operator topics", topicsBody), prefix)
		for _, want := range topicIDs {
			if !seen[want] {
				t.Fatalf("operator: seeded topic %s missing — the operator must read ALL private graph content of the global corpus", want)
			}
		}
		_, peopleBody := corpusGET(t, srv.URL, operator.bearer, "/api/people?limit=200")
		seenPeople := corpusSeededSubset(corpusIDField(t, "operator people", peopleBody), prefix)
		for _, want := range peopleIDs {
			if !seenPeople[want] {
				t.Fatalf("operator: seeded person %s missing — the operator must read ALL private graph content of the global corpus", want)
			}
		}

		// The operational-metadata tier belongs to the operator class ONLY.
		// A grant-holder that classified as GrantOperator would silently
		// inherit it; assert the classes stay distinct.
		if graphapi.ClassifyGraphGrant(grantHolder.identity) == graphapi.GrantOperator {
			t.Fatalf("grant-holder classified as %q — the knowledge-graph:read grant must NOT confer the operator operational-metadata tier",
				graphapi.GrantOperator)
		}
		if graphapi.ClassifyGraphGrant(operator.identity) != graphapi.GrantOperator {
			t.Fatalf("operator did not classify as %q — the operator tier regressed", graphapi.GrantOperator)
		}
		// The operator tier is READ-ONLY: it is reached through the same
		// read-only capability, and its activation projection is
		// value-safe (state/presence-class/code only, never the secret).
		act := graphCap.Activation()
		if act.State != graphapi.ActivationEnabled {
			t.Fatalf("operator operational metadata: activation state=%q; want %q", act.State, graphapi.ActivationEnabled)
		}
		if act.Code != graphapi.CodeActivationOK {
			t.Fatalf("operator operational metadata: activation code=%q; want %q", act.Code, graphapi.CodeActivationOK)
		}
	})

	// ---------------------------------------------------------------
	// (3) NO per-identity row isolation. Operator and grant-holder MUST
	//     observe the SAME underlying global row set. A regression that
	//     adds a per-user/tenant WHERE predicate fails here.
	// ---------------------------------------------------------------
	t.Run("operator_and_grant_holder_observe_the_same_global_rows", func(t *testing.T) {
		for _, fam := range []struct {
			path string
			want []string
		}{
			{"/api/topics?limit=200", topicIDs},
			{"/api/people?limit=200", peopleIDs},
		} {
			_, opBody1 := corpusGET(t, srv.URL, operator.bearer, fam.path)
			opPage1 := corpusIDField(t, "operator "+fam.path, opBody1)

			_, ghBody := corpusGET(t, srv.URL, grantHolder.bearer, fam.path)
			ghPage := corpusIDField(t, "grant-holder "+fam.path, ghBody)

			_, opBody2 := corpusGET(t, srv.URL, operator.bearer, fam.path)
			opPage2 := corpusIDField(t, "operator(repeat) "+fam.path, opBody2)

			opSeeded := corpusSeededSubset(opPage1, prefix)
			ghSeeded := corpusSeededSubset(ghPage, prefix)

			// UNCONDITIONAL: both identities see EVERY seeded global row.
			// A per-user/tenant predicate would strip the seeded rows from
			// the grant-holder (they carry no owner matching that user),
			// failing right here.
			for _, want := range fam.want {
				if !opSeeded[want] {
					t.Fatalf("operator %s: seeded global row %s absent (%d rows returned) — the global corpus was partitioned",
						fam.path, want, len(opPage1))
				}
				if !ghSeeded[want] {
					t.Fatalf("grant-holder %s: seeded global row %s absent (%d rows returned) — a per-identity/tenant row predicate was introduced; the grant differentiates the PROJECTION, never the ROW SET",
						fam.path, want, len(ghPage))
				}
			}
			// UNCONDITIONAL: the two identities' views of the seeded global
			// rows are the SAME SET, not merely both non-empty.
			if !corpusSameSet(opSeeded, ghSeeded) {
				t.Fatalf("%s: operator and grant-holder observe DIFFERENT global row sets (operator=%d, grant-holder=%d over prefix %s) — the corpus is partitioned per identity",
					fam.path, len(opSeeded), len(ghSeeded), prefix)
			}

			// STRONGEST form: when the corpus was provably stable across
			// the comparison window (the two operator reads agree), the
			// grant-holder's WHOLE page must match byte-for-byte in id and
			// order. When another concurrent integration package wrote to
			// the shared database mid-window the whole-page comparison is
			// not meaningful; the unconditional seeded-set assertions above
			// have already proven the no-isolation property either way.
			if corpusSameSequence(opPage1, opPage2) {
				if !corpusSameSequence(opPage1, ghPage) {
					t.Fatalf("%s: over a STABLE corpus the grant-holder page differs from the operator page (operator=%d ids, grant-holder=%d ids) — the read path partitions rows by identity",
						fam.path, len(opPage1), len(ghPage))
				}
			} else {
				t.Logf("%s: corpus changed across the comparison window (concurrent writer); whole-page identity comparison skipped — the unconditional seeded-row set equality above still proves no per-identity partition", fam.path)
			}
		}
	})

	// ---------------------------------------------------------------
	// (4) Ungranted denial is leak-free AND indistinguishable from
	//     "empty" / "absent".
	// ---------------------------------------------------------------
	t.Run("ungranted_denial_is_leak_free_and_indistinguishable_from_absent", func(t *testing.T) {
		// A path naming a REAL seeded row and a path naming a row that
		// certainly does not exist MUST be answered identically. If the
		// denial ever varied with existence, an ungranted caller could
		// enumerate the corpus.
		existing := "/api/topics/" + topicIDs[0]
		absent := "/api/topics/" + prefix + "-topic-does-not-exist"

		paths := []string{
			"/api/topics?limit=200",
			existing,
			absent,
			"/api/people?limit=200",
			"/api/people/" + peopleIDs[0],
			"/api/places?limit=200",
			"/api/places/" + prefix + "-place-does-not-exist",
			"/api/time?from=2024-01-01&to=2024-01-02",
			"/api/graph/edges?sourceKind=artifact&sourceId=" + artifactIDs[0],
		}

		var existingBody, absentBody []byte
		for _, path := range paths {
			resp, body := corpusGET(t, srv.URL, ungranted.bearer, path)
			assertDenialLeaksNothing(t, "ungranted GET "+path, resp, body, seededSecrets)
			switch path {
			case existing:
				existingBody = body
			case absent:
				absentBody = body
			}
		}

		if string(existingBody) != string(absentBody) {
			t.Fatalf("leak — the ungranted denial for an EXISTING row (%q) differs from the denial for an ABSENT row (%q); an ungranted identity can enumerate the corpus by diffing denials",
				string(existingBody), string(absentBody))
		}

		// And an unauthenticated caller must never be upgraded past the
		// denial boundary either: no bearer is still not a graph read.
		resp, body := corpusGET(t, srv.URL, "", "/api/topics?limit=200")
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("a caller with an empty bearer read the global corpus (status 200) — authentication is not optional. body=%s", string(body))
		}
		lower := strings.ToLower(string(body))
		for _, secret := range seededSecrets {
			if strings.Contains(lower, strings.ToLower(secret)) {
				t.Fatalf("leak — the unauthenticated rejection body discloses seeded graph content %q. body=%s", secret, string(body))
			}
		}
	})
}
