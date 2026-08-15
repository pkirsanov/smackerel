package graphsynthetic

// synthetic_http_outcome_test.go — BUG-080-001 SCOPE-03 (SCN-080-001-03),
// REFUSAL arm, Layer 2.
//
// Layer 1 proves the reducer refuses. This layer proves the whole path:
// a real HTTP outcome is classified into the right closed family code,
// and that failed family really does refuse the aggregate.
//
// Nothing internal is mocked or stubbed. The Synthetic, its Config, its
// status classifier, and the aggregate reducer are the production types,
// and the observer is the production NopObserver. The only fixture is the
// SERVER on the far side of a real socket, which is the sole deterministic
// way to induce a 5xx, a typed invalid-cursor 400, or a contract-invalid
// body at the process boundary the synthetic actually reads through.
//
// The stub also asserts two structural properties of every request it
// receives: the synthetic only ever issues GET (it is READ-ONLY by
// construction), and it always presents a scoped session.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api/graphapi"
)

// graphStubSeedID is the row id the stub's list families expose. The
// synthetic uses it only to build the detail and edges request URLs.
const graphStubSeedID = "synthetic-stub-seed"

// graphStubOKBody returns the contract-valid 200 body for one canonical
// synthetic request path, or "" when the path is not part of the fixed
// family sequence.
func graphStubOKBody(path string) string {
	switch path {
	case "/api/topics/", "/api/people/", "/api/places/", "/api/graph/edges":
		return `{"items":[{"id":"` + graphStubSeedID + `"}],"nextCursor":""}`
	case "/api/time":
		return `{"days":[{"date":"2026-01-01","artifacts":[]}]}`
	}
	if strings.HasPrefix(path, "/api/topics/") ||
		strings.HasPrefix(path, "/api/people/") ||
		strings.HasPrefix(path, "/api/places/") {
		return `{"id":"` + graphStubSeedID + `"}`
	}
	return ""
}

// graphStubIsTopicsList matches the topics LIST route only.
func graphStubIsTopicsList(path string) bool { return path == "/api/topics/" }

// graphStubIsTopicDetail matches the topics DETAIL route only, so a case
// can populate a list and still withhold its seeded row.
func graphStubIsTopicDetail(path string) bool {
	return strings.HasPrefix(path, "/api/topics/") && path != "/api/topics/"
}

// newGraphStubServer starts a real HTTP server that answers the fixed
// family sequence with contract-valid bodies, except on the paths match
// selects, where it answers with the supplied status and body. A nil
// match serves the fully-valid corpus.
func newGraphStubServer(t *testing.T, match func(string) bool, status int, body string, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)

		if r.Method != http.MethodGet {
			t.Errorf("the synthetic issued a %s to %q; it is READ-ONLY by construction and must only ever GET", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") == "" {
			t.Errorf("the synthetic issued an unauthenticated request to %q; every read carries the real scoped session", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")

		if match != nil && match(r.URL.Path) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
			return
		}

		ok := graphStubOKBody(r.URL.Path)
		if ok == "" {
			t.Errorf("the stub server received the unmodelled path %q; the fixture and the synthetic's route set have drifted", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, ok)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// graphStubConfig is a valid synthetic configuration pointed at the stub.
// AllowEmptyFamilies and OptionalFamilies are both empty on purpose:
// nothing is excused, so neither a silent empty pass nor a degraded
// aggregate is reachable and only a genuine outcome can be observed.
func graphStubConfig(baseURL string) Config {
	now := time.Now().UTC()
	return Config{
		BaseURL:            baseURL,
		BearerToken:        "stub-scoped-session-token",
		WindowFrom:         now.Add(-24 * time.Hour),
		WindowTo:           now.Add(time.Hour),
		RequestTimeout:     5 * time.Second,
		EdgeSourceKind:     "topic",
		AllowEmptyFamilies: []graphapi.GraphRouteFamily{},
		OptionalFamilies:   []graphapi.GraphRouteFamily{},
		HTTPClient:         &http.Client{Timeout: 10 * time.Second},
	}
}

// graphStubActivation is the explicit ENABLED activation policy every
// observation below runs under.
func graphStubActivation() graphapi.Activation {
	return graphapi.Activation{
		State:          graphapi.ActivationEnabled,
		SecretPresence: graphapi.SecretPresent,
		Code:           graphapi.CodeActivationOK,
	}
}

// graphSyntheticHTTPOutcomeCases covers every SCN-080-001-03 outcome
// class as a REAL HTTP response. The schema class carries two inputs — an
// undecodable body and a well-formed but contract-invalid one — because
// the contract check and the parser are distinct guards.
var graphSyntheticHTTPOutcomeCases = []struct {
	name     string
	match    func(string) bool
	status   int
	body     string
	failing  graphapi.GraphRouteFamily
	wantCode string
}{
	{
		name:     "401_unauthenticated",
		match:    graphStubIsTopicsList,
		status:   http.StatusUnauthorized,
		body:     `{"error":{"code":"unauthenticated"}}`,
		failing:  graphapi.FamilyTopics,
		wantCode: CodeUnauthenticated,
	},
	{
		name:     "403_forbidden",
		match:    graphStubIsTopicsList,
		status:   http.StatusForbidden,
		body:     `{"error":{"code":"forbidden"}}`,
		failing:  graphapi.FamilyTopics,
		wantCode: CodeForbidden,
	},
	{
		name:     "404_route_absent",
		match:    graphStubIsTopicsList,
		status:   http.StatusNotFound,
		body:     `{"error":{"code":"not_found"}}`,
		failing:  graphapi.FamilyTopics,
		wantCode: CodeRouteAbsent,
	},
	{
		name:     "5xx_server_error",
		match:    graphStubIsTopicsList,
		status:   http.StatusBadGateway,
		body:     `{"error":{"code":"internal_error"}}`,
		failing:  graphapi.FamilyTopics,
		wantCode: CodeServerError,
	},
	{
		name:     "schema_invalid_undecodable_body",
		match:    graphStubIsTopicsList,
		status:   http.StatusOK,
		body:     `{"items":`,
		failing:  graphapi.FamilyTopics,
		wantCode: CodeSchemaInvalid,
	},
	{
		name:     "schema_invalid_contract_invalid_body",
		match:    graphStubIsTopicsList,
		status:   http.StatusOK,
		body:     `{"items":[{"id":"` + graphStubSeedID + `"}]}`,
		failing:  graphapi.FamilyTopics,
		wantCode: CodeSchemaInvalid,
	},
	{
		name:     "cursor_invalid",
		match:    graphStubIsTopicsList,
		status:   http.StatusBadRequest,
		body:     `{"error":{"code":"` + graphapi.CodeInvalidCursor + `"}}`,
		failing:  graphapi.FamilyTopics,
		wantCode: CodeCursorInvalid,
	},
	{
		name:     "row_missing",
		match:    graphStubIsTopicDetail,
		status:   http.StatusNotFound,
		body:     `{"error":{"code":"not_found"}}`,
		failing:  graphapi.FamilyTopicDetail,
		wantCode: CodeRowMissing,
	},
}

// TestGraphSyntheticHTTPOutcomeRefusesAggregate drives the production
// runner against a real HTTP server and proves each outcome class both
// fails its family with the right closed code AND refuses the aggregate.
func TestGraphSyntheticHTTPOutcomeRefusesAggregate(t *testing.T) {
	if len(graphSyntheticHTTPOutcomeCases) == 0 {
		t.Fatalf("anti-vacuity: the HTTP outcome case table is empty; this test would assert nothing")
	}
	required := graphapi.RequiredGraphFamilies()
	if len(required) == 0 {
		t.Fatalf("anti-vacuity: graphapi.RequiredGraphFamilies() is empty; the family assertions would be vacuous")
	}

	executed := 0
	for _, tc := range graphSyntheticHTTPOutcomeCases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int64
			srv := newGraphStubServer(t, tc.match, tc.status, tc.body, &hits)

			synth, err := New(graphStubConfig(srv.URL), NopObserver{})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			agg, err := synth.Run(ctx, graphStubActivation())
			if err != nil {
				t.Fatalf("Synthetic.Run over real HTTP: %v", err)
			}
			if hits.Load() == 0 {
				t.Fatalf("anti-vacuity: the synthetic issued zero requests to the stub server; nothing was exercised")
			}

			var row GraphFamilyResult
			found := false
			for _, candidate := range agg.Families {
				if candidate.Family == tc.failing {
					row, found = candidate, true
					break
				}
			}
			if !found {
				t.Fatalf("anti-vacuity: the aggregate carries no %q row, so the outcome assertion would be vacuous", tc.failing)
			}
			if row.State != StateFailed {
				t.Fatalf("family %q state = %q; want %q after HTTP %d", tc.failing, row.State, StateFailed, tc.status)
			}
			if row.Code != tc.wantCode {
				t.Fatalf("family %q code = %q; want %q after HTTP %d", tc.failing, row.Code, tc.wantCode, tc.status)
			}

			if agg.Available() {
				t.Fatalf("Available() = true after required family %q failed with %s; the aggregate MUST be refused", tc.failing, tc.wantCode)
			}
			if agg.State != AggregateUnavailable {
				t.Fatalf("aggregate state = %q; want %q after required family %q failed", agg.State, AggregateUnavailable, tc.failing)
			}
			if agg.Code != tc.wantCode {
				t.Fatalf("aggregate code = %q; want %q — the failing family's own cause MUST propagate, not be flattened", agg.Code, tc.wantCode)
			}
			if err := agg.Validate(); err != nil {
				t.Fatalf("the refused aggregate failed its own closed-vocabulary contract: %v", err)
			}
			executed++
		})
	}

	if executed != len(graphSyntheticHTTPOutcomeCases) {
		t.Fatalf("anti-vacuity: %d of %d HTTP outcome cases executed", executed, len(graphSyntheticHTTPOutcomeCases))
	}
}

// TestGraphSyntheticHTTPContractValidReadsProduceAvailableAggregate is the
// Layer 2 positive control. Without it, a runner that failed every read
// would satisfy every refusal assertion above.
func TestGraphSyntheticHTTPContractValidReadsProduceAvailableAggregate(t *testing.T) {
	required := graphapi.RequiredGraphFamilies()
	if len(required) == 0 {
		t.Fatalf("anti-vacuity: graphapi.RequiredGraphFamilies() is empty; the availability assertions would be vacuous")
	}

	var hits atomic.Int64
	srv := newGraphStubServer(t, nil, 0, "", &hits)

	synth, err := New(graphStubConfig(srv.URL), NopObserver{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agg, err := synth.Run(ctx, graphStubActivation())
	if err != nil {
		t.Fatalf("Synthetic.Run over real HTTP: %v", err)
	}

	// One authenticated GET per canonical family is the floor for a run
	// that genuinely executed the fixed sequence.
	if got := hits.Load(); got < int64(len(required)) {
		t.Fatalf("anti-vacuity: the synthetic issued %d requests for %d canonical families; the fixed sequence was not exercised", got, len(required))
	}

	if !agg.Available() {
		t.Fatalf("Available() = false against a fully contract-valid corpus; state=%q code=%q", agg.State, agg.Code)
	}
	if agg.State != AggregateAvailable {
		t.Fatalf("aggregate state = %q; want %q", agg.State, AggregateAvailable)
	}
	if agg.Code != CodeOK {
		t.Fatalf("aggregate code = %q; want %q", agg.Code, CodeOK)
	}
	if err := agg.Validate(); err != nil {
		t.Fatalf("available aggregate failed its own closed-vocabulary contract: %v", err)
	}
	if len(agg.Families) != len(required) {
		t.Fatalf("aggregate carries %d family rows; the canonical manifest requires %d", len(agg.Families), len(required))
	}
	for _, row := range agg.Families {
		if row.State != StatePopulated {
			t.Fatalf("family %q state = %q; want %q — every family reads a populated corpus here", row.Family, row.State, StatePopulated)
		}
	}
}
