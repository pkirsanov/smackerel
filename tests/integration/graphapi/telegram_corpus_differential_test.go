//go:build integration

// Spec 108 Scope 04 — TP-04-09 / design.md §10.10 **T4**, the decisive layer.
//
// WHAT WAS MISSING. Before this file no test anywhere carried a Telegram
// corpus differential. `internal/telegram/scope_literal_guard_test.go` (T1)
// proves the package SOURCE holds no scope literal;
// `internal/telegram/per_user_token_scope_claim_test.go` (T4-08) proves the
// minted PASETO's `scope` claim equals `recorded ∩ ceiling`. Neither drives the
// minted bearer at a real gate, so neither could show that a principal without
// `corpus:read` is actually turned away from the corpus surface it reaches
// THROUGH the bridge. The Consumer Impact Sweep recorded that absence as a
// negative result; this file closes it.
//
// ── WHY BOTH ARMS, ON TWO AXES ──────────────────────────────────────────────
//
// A negative-only test is worthless here. "Corpus refused" is exactly what a
// totally broken bridge produces, so on its own it cannot distinguish
// "correctly denied" from "nothing works". This file pairs the arms on TWO
// independent axes so no single failure mode can satisfy both:
//
//	PER-CAPABILITY (T4 as design.md words it) — ONE principal, ONE mint,
//	recorded set exactly {annotation:edit}: the corpus command is refused 403
//	AND the annotation write SUCCEEDS 201 from the same bearer. A derivation
//	that drops everything, or a bridge that is simply broken, fails the
//	annotation arm.
//
//	PER-PRINCIPAL — two chats differing ONLY in their recorded grant set: the
//	{annotation:edit} principal is refused on all sixteen corpus groups and the
//	{corpus:read, annotation:edit} principal is admitted on all sixteen. A
//	regression to a hardcoded {annotation:edit, corpus:read} list fails the
//	first; a regression to a hardcoded {annotation:edit} list fails the second.
//
// ── THE REINTRODUCTION GUARD IS EXECUTABLE, NOT PROSE ───────────────────────
//
// design.md §10.10 asks that T4 be shown to FAIL against a tree patched back to
// the literal list. Patching production source to produce a one-off red
// transcript would bend the source to fit a test and would prove nothing on any
// later run. The equivalent guarantee is expressed here as a PERMANENT
// assertion instead: the T4 verdict is factored into the pure predicate
// `telegramDifferentialHolds`, and
// `TestIntegration_TelegramBridge_FixedScopeListCollapsesTheDifferential`
// asserts that predicate returns FALSE for a minter whose claim does not track
// the principal — for each of the three fixed lists a reintroduced literal
// could plausibly carry. A minter-side literal and a principal-independent
// grant reader are behaviourally identical at the only place detection can
// happen (the minted claim stops tracking the principal), so that test is a
// faithful adversarial simulation, it runs through the REAL production
// `MintForChat`, and it runs on every invocation rather than once.
//
// ── NOTHING IS FAKED ON THE PATH UNDER TEST ─────────────────────────────────
//
// Real `telegram.PerUserTokenMinter` over a real production-environment `Bot`
// with a real chat→user mapping; real `MintForChat` → real
// `auth.DeriveTelegramBridgeGrants` → real `auth.IssueToken`; real
// `api.NewRouter` with `CorpusGrantEnforce: true` over real PostgreSQL, real
// per-user PASETO middleware, a real `intelligence.NewEngine` so the eight
// Tier B groups genuinely register, and a real `annotation.NewStore` so the
// positive arm is a genuine database write rather than a stub's 503. The only
// substituted component is the `PrincipalGrantReader` — the authority SOURCE,
// which is the seam the test must vary to express a grant matrix at all.
package graphapi_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/metrics"
	"github.com/smackerel/smackerel/internal/telegram"
)

const (
	// Two mapped chats whose principals differ ONLY in their recorded grant
	// set. Everything else — signing key, issuer, TTL, key id, router, route,
	// request bytes — is identical, so any divergence in outcome is
	// attributable to the grant and to nothing else.
	tgDiffChatAnnotationOnly int64 = 900_001
	tgDiffChatCorpusHolder   int64 = 900_002

	tgDiffUserAnnotationOnly = "tp0409-telegram-annotation-only"
	tgDiffUserCorpusHolder   = "tp0409-telegram-corpus-holder"

	// tgDiffUnmappedChat has no mapping entry. Under `environment=production`
	// it must refuse to mint at all; asserted so the mapping half of
	// `MintForChat` is exercised rather than assumed.
	tgDiffUnmappedChat int64 = 900_999

	// Verdict labels for the per-group outcome map.
	bridgeRefused  = "refused"
	bridgeAdmitted = "admitted"
)

// bridgeKeyID is the PASETO footer `kid` the bridge mints under. It is an
// identifier, not a credential; the signing keypair is generated per run by
// `newCorpusEnforceStack`.
const bridgeKeyID = corpusKeyID //gitleaks:allow — PASETO key *identifier* (kid), not a credential

// ── grant readers ───────────────────────────────────────────────────────────

// recordedGrantReader answers each principal with its OWN recorded set — the
// production semantic of `*auth.BearerStore.GrantsForPrincipal`, which is the
// sole authority source per spec.md §18 decision 3.
type recordedGrantReader struct{ byPrincipal map[string][]string }

func (r recordedGrantReader) GrantsForPrincipal(_ context.Context, userID string) (auth.RecordedGrants, error) {
	scopes, ok := r.byPrincipal[userID]
	if !ok {
		// Unrecorded rather than empty: the minter must fail loudly, which is
		// a visible test failure rather than a silent scopeless mint.
		return auth.RecordedGrants{TokenID: "tp0409-" + userID}, nil
	}
	return auth.RecordedGrants{
		TokenID:  "tp0409-" + userID,
		Scopes:   append([]string(nil), scopes...),
		Recorded: true,
	}, nil
}

// fixedScopeGrantReader IGNORES the principal entirely and answers the same set
// for every caller. That is precisely the observable behaviour of a
// reintroduced minter-side hardcoded scope list: the minted claim stops
// tracking the principal. It is the adversarial fixture for the reintroduction
// guard below.
type fixedScopeGrantReader struct{ scopes []string }

func (r fixedScopeGrantReader) GrantsForPrincipal(_ context.Context, userID string) (auth.RecordedGrants, error) {
	return auth.RecordedGrants{
		TokenID:  "tp0409-fixed-" + userID,
		Scopes:   append([]string(nil), r.scopes...),
		Recorded: true,
	}, nil
}

// ── the bridge under test ───────────────────────────────────────────────────

// newBridgeMinter builds the REAL production minter over a production-
// environment bot carrying the real two-chat mapping, with `grants` as the
// authority source.
func newBridgeMinter(t *testing.T, privateHex string, grants telegram.PrincipalGrantReader) *telegram.PerUserTokenMinter {
	t.Helper()
	m, err := telegram.NewPerUserTokenMinter(telegram.PerUserTokenMinterOptions{
		Bot: telegram.NewBotForTest("production", map[int64]string{
			tgDiffChatAnnotationOnly: tgDiffUserAnnotationOnly,
			tgDiffChatCorpusHolder:   tgDiffUserCorpusHolder,
		}),
		PrincipalGrants: grants,
		SigningKey:      privateHex,
		KeyID:           bridgeKeyID,
		Issuer:          corpusIssuer,
		TTL:             time.Hour,
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	return m
}

// mintViaBridge drives the REAL `MintForChat` — chat→user resolution, grant
// read, ceiling narrowing, PASETO issuance — and returns the wire bearer a
// Telegram command would actually carry.
func mintViaBridge(t *testing.T, m *telegram.PerUserTokenMinter, chatID int64) string {
	t.Helper()
	minted, err := m.MintForChat(context.Background(), chatID)
	if err != nil {
		t.Fatalf("MintForChat(chat=%d): %v — the bridge failed to mint, so no arm below proves anything", chatID, err)
	}
	if minted.WireToken == "" {
		t.Fatalf("MintForChat(chat=%d) returned an EMPTY wire token with no error", chatID)
	}
	return minted.WireToken
}

// bridgeMintedScopes verifies the wire token the same way the middleware does
// and returns its `scope` claim, so the claim can be compared to the recorded
// set independently of what the gate then did with it.
func bridgeMintedScopes(t *testing.T, publicHex, wire string) []string {
	t.Helper()
	parsed, err := auth.VerifyAndParse(wire, auth.VerifyOptions{
		ActivePublicKey:    publicHex,
		ActiveKeyID:        bridgeKeyID,
		Issuer:             corpusIssuer,
		ClockSkewTolerance: 30 * time.Second,
		Now:                time.Now,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse(bridge mint): %v", err)
	}
	sorted := append([]string(nil), parsed.Scopes...)
	slices.Sort(sorted)
	return sorted
}

// newTelegramBridgeRouter builds the ENFORCE router the bridge's bearer is
// driven against.
//
// It is a sibling of `newCorpusEnforceRouter` rather than a call to it because
// it adds ONE thing that file deliberately excludes: the annotation surface.
// `corpus_enforce_test.go` omits `AnnotationHandlers` on the grounds that
// wiring a placeholder handler would substitute a fake for the thing under
// test — and that reasoning is right. This file needs the annotation surface
// for T4's positive arm, so it wires a REAL `annotation.NewStore` over the
// live pool, satisfying the same standard. `corpus_enforce_test.go` is left
// byte-identical so its four green tests are not perturbed.
func newTelegramBridgeRouter(
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
			Enabled:                              true,
			TokenFormat:                          "paseto-v4-public",
			ProductionSharedTokenFallbackEnabled: true,
		},
		AuthVerifyOptions: auth.VerifyOptions{
			ActivePublicKey:    activePublicHex,
			ActiveKeyID:        bridgeKeyID,
			Issuer:             corpusIssuer,
			ClockSkewTolerance: 30 * time.Second,
			Now:                time.Now,
		},
		RevocationCache: revocation.NewCache(),

		// ENFORCE — the stage the differential is asserted at.
		CorpusGrantEnforce: true,

		// Real engine so the eight Tier B groups genuinely register; nil here
		// would make half the corpus sweep vacuous.
		IntelligenceEngine: intelligence.NewEngine(pool, nil),
		ContextHandler:     &api.ContextHandler{},

		// Real store over the live pool — T4's positive arm is a genuine
		// database write, not a stub's 503.
		AnnotationHandlers: &api.AnnotationHandlers{
			Store:       annotation.NewStore(pool, nil),
			Environment: "production",
		},

		GraphCapability: graphCap,
		TopicsHandlers:  graphapi.NewTopicsHandlers(pool, limits, codec),
		PeopleHandlers:  graphapi.NewPeopleHandlers(pool, limits, codec),
		PlacesHandlers:  graphapi.NewPlacesHandlers(pool, limits, codec),
		TimeHandlers:    graphapi.NewTimeHandlers(pool, limits),
		EdgesHandlers:   graphapi.NewEdgesHandlers(pool, limits, codec),
	})
}

// ── outcome capture ─────────────────────────────────────────────────────────

// isScopeGateDenial discriminates a refusal by the scope its body NAMES, not by
// its status. A bare 403 is not enough: adjacent gates answer 403 to the same
// principal, and conflating them would let a deleted corpus gate "pass" on the
// strength of the gate next door.
func isScopeGateDenial(resp *http.Response, body []byte, scope string) bool {
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	var d corpusScopeDenial
	if err := json.Unmarshal(body, &d); err != nil {
		return false
	}
	return d.Error == "scope_required" && slices.Contains(d.Required, scope)
}

// bridgeCorpusOutcome drives one minted bearer at one route per corpus group
// and records what the gate did, WITHOUT asserting which answer is correct.
// Classification rather than assertion is what lets the adversarial test below
// observe a collapsed differential instead of dying on the first arm.
//
// Two conditions still fail hard, because either would make every verdict
// vacuous: an unmounted route (chi 404 — nothing was guarding anything) and a
// 401 (the bearer stopped authenticating, so the comparison is meaningless).
func bridgeCorpusOutcome(t *testing.T, base, bearer, label string) map[metrics.CorpusRouteGroup]string {
	t.Helper()
	table := corpusEnforceAllRoutes()
	groups := corpusEnforceClosedSet(t, table)

	out := make(map[metrics.CorpusRouteGroup]string, len(groups))
	for _, g := range groups {
		rt := table[g][0]
		resp, body := corpusDo(t, base, bearer, rt)
		switch {
		case corpusIsChiNotFound(resp, body):
			t.Fatalf("%s/%s %s %s: route is NOT MOUNTED (chi 404). An unmounted route makes every verdict in this file vacuous.",
				label, g, rt.method, rt.path)
		case resp.StatusCode == http.StatusUnauthorized:
			t.Fatalf("%s/%s %s %s: status=401 — the bridge bearer stopped authenticating, so this run proves nothing about the grant. body=%s",
				label, g, rt.method, rt.path, string(body))
		case isCorpusGateDenial(resp, body):
			out[g] = bridgeRefused
		default:
			out[g] = bridgeAdmitted
		}
	}
	return out
}

// telegramDifferentialHolds is the EXACT predicate TP-04-09 asserts: the
// principal WITHOUT `corpus:read` is refused on every corpus group, and the
// principal WITH it is admitted on every corpus group.
//
// It is factored out as a pure function for one reason. It makes the
// reintroduction guard executable: the adversarial test asserts this same
// predicate returns FALSE for a minter whose claim does not track the
// principal. A comment claiming "a literal would fail this" is not evidence;
// running the predicate against a literal and watching it return false is.
func telegramDifferentialHolds(annotationOnly, corpusHolder map[metrics.CorpusRouteGroup]string) bool {
	if len(annotationOnly) != 16 || len(corpusHolder) != 16 {
		return false
	}
	for _, v := range annotationOnly {
		if v != bridgeRefused {
			return false
		}
	}
	for _, v := range corpusHolder {
		if v != bridgeAdmitted {
			return false
		}
	}
	return true
}

// describeOutcome renders an outcome map deterministically for failure output.
func describeOutcome(o map[metrics.CorpusRouteGroup]string) string {
	keys := make([]string, 0, len(o))
	for g := range o {
		keys = append(keys, string(g))
	}
	sort.Strings(keys)
	s := ""
	for _, k := range keys {
		s += k + "=" + o[metrics.CorpusRouteGroup(k)] + " "
	}
	return s
}

// seedAnnotatableArtifact inserts one disposable artifact row so the annotation
// write has a real FK target, and removes it on cleanup. Ephemeral by
// construction: the row is created by this test and deleted by this test.
func seedAnnotatableArtifact(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := fmt.Sprintf("tp0409-artifact-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, 'idea', $2, $3, 'capture')
	`, id, "spec108 tp0409 telegram bridge annotation target", "h-"+id); err != nil {
		t.Fatalf("seed artifacts row: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM annotations WHERE artifact_id = $1`, id)
		_, _ = pool.Exec(cctx, `DELETE FROM artifacts WHERE id = $1`, id)
	})
	return id
}

// bridgeAnnotationWrite performs the annotation write a Telegram bridge command
// would perform, carrying the minted bridge bearer.
func bridgeAnnotationWrite(t *testing.T, base, bearer, artifactID string) (*http.Response, []byte) {
	t.Helper()
	rt := corpusEnforceRoute{
		method: http.MethodPost,
		path:   "/api/artifacts/" + artifactID + "/annotations",
		body:   `{"text":"spec108 tp0409 bridge annotation write"}`,
	}
	return corpusDoWithSource(t, base, bearer, rt, string(annotation.ChannelTelegram))
}

// corpusDoWithSource is `corpusDo` plus the closed-set `X-Smackerel-Source`
// header the annotation router requires (spec 027 scope 9 PLAN-9-04).
func corpusDoWithSource(t *testing.T, base, bearer string, rt corpusEnforceRoute, source string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(rt.method, base+rt.path, bytes.NewBufferString(rt.body))
	if err != nil {
		t.Fatalf("NewRequest(%s %s): %v", rt.method, rt.path, err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.AnnotationSourceHeader, source)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", rt.method, rt.path, err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body for %s %s: %v", rt.method, rt.path, err)
	}
	return resp, body
}

// ── TP-04-09 / design.md §10.10 T4 ──────────────────────────────────────────

// TestIntegration_TelegramBridge_CorpusDifferentialUnderEnforce is the decisive
// layer.
//
// Four arms, two axes, one router, ENFORCE:
//
//	A1 (T4 negative, per-capability) — the {annotation:edit} principal's mint is
//	   refused on every one of the sixteen corpus groups, with a clean envelope
//	   that echoes none of the canaries planted in the request.
//	A2 (T4 positive, per-capability, SAME MINT) — that same bearer performs a
//	   real annotation write against real PostgreSQL and receives 201.
//	A3 (per-principal positive) — the {corpus:read, annotation:edit} principal's
//	   mint is admitted on every one of the sixteen groups.
//	A4 (claim differential) — the two minted `scope` claims are parsed and
//	   asserted DIFFERENT, each equal to its own `recorded ∩ ceiling`.
//
// A1 alone would pass against a totally broken bridge. A2 and A3 are what
// separate "correctly denied" from "nothing works". A4 localises a failure to
// the mint rather than the gate when the wire arms disagree.
func TestIntegration_TelegramBridge_CorpusDifferentialUnderEnforce(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "TGCORPUSDIFF")

	minter := newBridgeMinter(t, privateHex, recordedGrantReader{byPrincipal: map[string][]string{
		tgDiffUserAnnotationOnly: {auth.GrantAnnotationEdit},
		tgDiffUserCorpusHolder:   {auth.GrantGlobalCorpusRead, auth.GrantAnnotationEdit},
	}})

	annotationOnlyBearer := mintViaBridge(t, minter, tgDiffChatAnnotationOnly)
	corpusHolderBearer := mintViaBridge(t, minter, tgDiffChatCorpusHolder)

	srv := httptest.NewServer(newTelegramBridgeRouter(t, stack.pool, stack.codec, stack.limits, stack.graphCap, stack.publicHex))
	t.Cleanup(srv.Close)
	base := srv.URL

	// ---- A4: the minted claims themselves ----
	gotAnnotationOnly := bridgeMintedScopes(t, stack.publicHex, annotationOnlyBearer)
	gotCorpusHolder := bridgeMintedScopes(t, stack.publicHex, corpusHolderBearer)

	wantAnnotationOnly := []string{auth.GrantAnnotationEdit}
	wantCorpusHolder := []string{auth.GrantAnnotationEdit, auth.GrantGlobalCorpusRead}
	slices.Sort(wantAnnotationOnly)
	slices.Sort(wantCorpusHolder)

	if !slices.Equal(gotAnnotationOnly, wantAnnotationOnly) {
		t.Fatalf("annotation-only principal minted scope=%v, want %v (recorded ∩ ceiling). "+
			"The bridge is not deriving the claim from this principal's recorded grants.", gotAnnotationOnly, wantAnnotationOnly)
	}
	if !slices.Equal(gotCorpusHolder, wantCorpusHolder) {
		t.Fatalf("corpus-holder principal minted scope=%v, want %v (recorded ∩ ceiling). "+
			"The bridge is not deriving the claim from this principal's recorded grants.", gotCorpusHolder, wantCorpusHolder)
	}
	if slices.Equal(gotAnnotationOnly, gotCorpusHolder) {
		t.Fatalf("both principals minted the SAME scope claim %v despite different recorded grants — "+
			"the minted claim does not track the principal, which is the observable signature of a minter-side scope list.", gotAnnotationOnly)
	}

	// ---- A1 + A3: the wire, all sixteen groups, both principals ----
	outAnnotationOnly := bridgeCorpusOutcome(t, base, annotationOnlyBearer, "annotation-only")
	outCorpusHolder := bridgeCorpusOutcome(t, base, corpusHolderBearer, "corpus-holder")

	if !telegramDifferentialHolds(outAnnotationOnly, outCorpusHolder) {
		t.Fatalf("TP-04-09 differential FAILED.\n  annotation-only (must be refused everywhere): %s\n  corpus-holder   (must be admitted everywhere): %s",
			describeOutcome(outAnnotationOnly), describeOutcome(outCorpusHolder))
	}

	// The negative arm must also be a CLEAN refusal — no canary echoed back,
	// no disclosure of corpus size or existence.
	table := corpusEnforceAllRoutes()
	probe := table[metrics.CorpusRouteGroupSearch][0]
	resp, body := corpusDo(t, base, annotationOnlyBearer, probe)
	assertCorpusDenialIsClean(t, "TP-04-09/annotation-only/"+probe.path, resp, body)

	// ---- A2: SAME MINT, annotation capability genuinely works ----
	artifactID := seedAnnotatableArtifact(t, stack.pool)
	aResp, aBody := bridgeAnnotationWrite(t, base, annotationOnlyBearer, artifactID)
	if isScopeGateDenial(aResp, aBody, auth.GrantAnnotationEdit) {
		t.Fatalf("TP-04-09 positive arm: the annotation gate REFUSED the {annotation:edit} principal. "+
			"Derivation dropped the grant it was supposed to carry, so the negative arm above is indistinguishable from a broken bridge. body=%s",
			string(aBody))
	}
	if aResp.StatusCode != http.StatusCreated {
		t.Fatalf("TP-04-09 positive arm: annotation write returned status=%d, want 201. "+
			"design.md §10.10 T4 requires the annotation write to SUCCEED from the same mint that was refused corpus access. body=%s",
			aResp.StatusCode, string(aBody))
	}

	// ---- mapping half: an UNMAPPED production chat mints nothing ----
	if _, err := minter.MintForChat(context.Background(), tgDiffUnmappedChat); err == nil {
		t.Fatalf("MintForChat(unmapped chat %d) succeeded in production — an unmapped chat must refuse to mint, never downgrade to a synthetic actor",
			tgDiffUnmappedChat)
	}

	t.Logf("TP-04-09: per-capability arms (corpus 403 + annotation 201 from one mint) and per-principal arms (16 refused / 16 admitted) both hold; minted claims %v vs %v",
		gotAnnotationOnly, gotCorpusHolder)
}

// TestIntegration_TelegramBridge_FixedScopeListCollapsesTheDifferential is the
// REINTRODUCTION GUARD, expressed as a permanent executable assertion rather
// than a one-off red transcript against a patched tree.
//
// It runs the REAL production `MintForChat` against a grant reader that ignores
// the principal — the behavioural twin of a reintroduced minter-side hardcoded
// scope list, since the only thing detection can observe is that the minted
// claim stopped tracking the principal. For each of the three fixed lists such
// a literal could plausibly carry, it asserts:
//
//   - both principals mint the IDENTICAL claim (confirming the fixture really is
//     principal-independent, so the case is not vacuous), and
//   - `telegramDifferentialHolds` — the exact predicate TP-04-09 asserts —
//     returns FALSE.
//
// If a future change made the T4 predicate satisfiable by a fixed list, this
// test fails, and it says so before a literal can ship. That is the guarantee
// design.md §10.10's "adversarial demonstration" clause exists to obtain, and
// unlike a patched-tree transcript it is re-proved on every run.
func TestIntegration_TelegramBridge_FixedScopeListCollapsesTheDifferential(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "TGFIXEDLIST")

	srv := httptest.NewServer(newTelegramBridgeRouter(t, stack.pool, stack.codec, stack.limits, stack.graphCap, stack.publicHex))
	t.Cleanup(srv.Close)
	base := srv.URL

	// First establish the CONTROL: the real, principal-tracking reader does
	// satisfy the predicate on this very router. Without it, "the fixed lists
	// fail" would be unremarkable — everything might fail here.
	realMinter := newBridgeMinter(t, privateHex, recordedGrantReader{byPrincipal: map[string][]string{
		tgDiffUserAnnotationOnly: {auth.GrantAnnotationEdit},
		tgDiffUserCorpusHolder:   {auth.GrantGlobalCorpusRead, auth.GrantAnnotationEdit},
	}})
	controlA := bridgeCorpusOutcome(t, base, mintViaBridge(t, realMinter, tgDiffChatAnnotationOnly), "control/annotation-only")
	controlC := bridgeCorpusOutcome(t, base, mintViaBridge(t, realMinter, tgDiffChatCorpusHolder), "control/corpus-holder")
	if !telegramDifferentialHolds(controlA, controlC) {
		t.Fatalf("CONTROL failed: the real principal-tracking reader does not satisfy the T4 predicate on this router, "+
			"so the adversarial cases below would prove nothing.\n  annotation-only: %s\n  corpus-holder:   %s",
			describeOutcome(controlA), describeOutcome(controlC))
	}

	for _, fixed := range []struct {
		name   string
		scopes []string
		why    string
	}{
		{
			name:   "literal_annotation_only",
			scopes: []string{auth.GrantAnnotationEdit},
			why:    "the historical hardcoded list §18 decision 3 rejected — every mapped chat gets annotation and nothing else, so the corpus HOLDER is wrongly refused",
		},
		{
			name:   "literal_union_annotation_and_corpus",
			scopes: []string{auth.GrantAnnotationEdit, auth.GrantGlobalCorpusRead},
			why:    "the union literal design.md §10.10 T4 names explicitly — every mapped chat gets corpus access, so the UNGRANTED principal is wrongly admitted",
		},
		{
			name:   "literal_corpus_only",
			scopes: []string{auth.GrantGlobalCorpusRead},
			why:    "the mirror image — the ungranted principal is wrongly admitted and the annotation capability is silently revoked",
		},
	} {
		t.Run(fixed.name, func(t *testing.T) {
			minter := newBridgeMinter(t, privateHex, fixedScopeGrantReader{scopes: fixed.scopes})

			aBearer := mintViaBridge(t, minter, tgDiffChatAnnotationOnly)
			cBearer := mintViaBridge(t, minter, tgDiffChatCorpusHolder)

			// The fixture must genuinely be principal-independent, or this
			// case is not the adversary it claims to be.
			aScopes := bridgeMintedScopes(t, stack.publicHex, aBearer)
			cScopes := bridgeMintedScopes(t, stack.publicHex, cBearer)
			if !slices.Equal(aScopes, cScopes) {
				t.Fatalf("adversarial fixture is not principal-independent: %v vs %v. "+
					"This case does not simulate a fixed scope list and proves nothing.", aScopes, cScopes)
			}

			outA := bridgeCorpusOutcome(t, base, aBearer, fixed.name+"/annotation-only")
			outC := bridgeCorpusOutcome(t, base, cBearer, fixed.name+"/corpus-holder")

			// The collapse itself: with a fixed list the two principals cannot
			// be told apart on the wire.
			if !maps.Equal(outA, outC) {
				t.Fatalf("a principal-independent claim produced DIFFERENT wire outcomes, which is impossible unless the probe is non-deterministic.\n  A: %s\n  C: %s",
					describeOutcome(outA), describeOutcome(outC))
			}

			if telegramDifferentialHolds(outA, outC) {
				t.Fatalf("REINTRODUCTION GUARD BREACHED: a fixed scope list %v SATISFIED the TP-04-09 differential predicate. "+
					"The T4 assertion has no teeth — a minter-side hardcoded list would ship green. (%s)\n  outcome: %s",
					fixed.scopes, fixed.why, describeOutcome(outA))
			}

			t.Logf("%s: fixed claim %v collapses the differential as required (%s)", fixed.name, aScopes, fixed.why)
		})
	}
}
