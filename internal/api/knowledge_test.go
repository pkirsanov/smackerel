package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/smackerel/smackerel/internal/knowledge"
)

// mockKnowledgeStore implements KnowledgeSearcher for testing.
type mockKnowledgeStore struct {
	searchResult  *knowledge.ConceptMatch
	searchErr     error
	concepts      []*knowledge.ConceptPage
	conceptsTotal int
	conceptsErr   error
	concept       *knowledge.ConceptPage
	conceptErr    error
	entities      []*knowledge.EntityProfile
	entitiesTotal int
	entitiesErr   error
	entity        *knowledge.EntityProfile
	entityErr     error
	lintReport    *knowledge.LintReport
	lintErr       error
	stats         *knowledge.KnowledgeStats
	statsErr      error
	healthStats   *knowledge.KnowledgeHealthStats
	healthErr     error
	healthDelay   time.Duration // optional delay to simulate slow DB (C-023-C001)
}

func (m *mockKnowledgeStore) SearchConcepts(_ context.Context, _ string, _ float64) (*knowledge.ConceptMatch, error) {
	return m.searchResult, m.searchErr
}

func (m *mockKnowledgeStore) GetConceptByID(_ context.Context, _ string) (*knowledge.ConceptPage, error) {
	return m.concept, m.conceptErr
}

func (m *mockKnowledgeStore) GetEntityByID(_ context.Context, _ string) (*knowledge.EntityProfile, error) {
	return m.entity, m.entityErr
}

func (m *mockKnowledgeStore) ListConceptsFiltered(_ context.Context, _, _ string, _, _ int) ([]*knowledge.ConceptPage, int, error) {
	return m.concepts, m.conceptsTotal, m.conceptsErr
}

func (m *mockKnowledgeStore) ListEntitiesFiltered(_ context.Context, _, _ string, _, _ int) ([]*knowledge.EntityProfile, int, error) {
	return m.entities, m.entitiesTotal, m.entitiesErr
}

func (m *mockKnowledgeStore) GetLatestLintReport(_ context.Context) (*knowledge.LintReport, error) {
	return m.lintReport, m.lintErr
}

func (m *mockKnowledgeStore) GetStats(_ context.Context) (*knowledge.KnowledgeStats, error) {
	return m.stats, m.statsErr
}

func (m *mockKnowledgeStore) GetKnowledgeHealthStats(_ context.Context) (*knowledge.KnowledgeHealthStats, error) {
	if m.healthDelay > 0 {
		time.Sleep(m.healthDelay)
	}
	return m.healthStats, m.healthErr
}

func (m *mockKnowledgeStore) CountEntitiesForConcept(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockKnowledgeStore) HasContradictions(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// --- T3-05: GET /concepts → list with correct sort/limit/offset ---

func TestKnowledgeConceptsHandler_List(t *testing.T) {
	now := time.Now()
	store := &mockKnowledgeStore{
		concepts: []*knowledge.ConceptPage{
			{
				ID: "c1", Title: "Negotiation", Summary: "Art of negotiation",
				SourceArtifactIDs:   []string{"a1", "a2", "a3"},
				SourceTypeDiversity: []string{"email", "video"},
				UpdatedAt:           now,
			},
			{
				ID: "c2", Title: "Leadership", Summary: "Leading teams",
				SourceArtifactIDs:   []string{"a4"},
				SourceTypeDiversity: []string{"article"},
				UpdatedAt:           now,
			},
		},
		conceptsTotal: 2,
	}

	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      now,
		KnowledgeStore: store,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/concepts?sort=citations&limit=5&offset=0", nil)
	rec := httptest.NewRecorder()

	deps.KnowledgeConceptsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ConceptListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
	if len(resp.Concepts) != 2 {
		t.Fatalf("expected 2 concepts, got %d", len(resp.Concepts))
	}
	if resp.Concepts[0].CitationCount != 3 {
		t.Errorf("expected citation_count=3 for Negotiation, got %d", resp.Concepts[0].CitationCount)
	}
	if resp.Limit != 5 {
		t.Errorf("expected limit=5, got %d", resp.Limit)
	}
}

// --- T3-06: GET /concepts/{id} → full concept detail ---

func TestKnowledgeConceptDetailHandler_Found(t *testing.T) {
	now := time.Now()
	store := &mockKnowledgeStore{
		concept: &knowledge.ConceptPage{
			ID:                "c1",
			Title:             "Negotiation",
			Summary:           "The art of negotiation",
			Claims:            json.RawMessage(`[{"text":"Claim 1"}]`),
			SourceArtifactIDs: []string{"a1", "a2"},
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}

	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      now,
		KnowledgeStore: store,
	}

	// Use chi context for URL param extraction
	r := chi.NewRouter()
	r.Get("/api/knowledge/concepts/{id}", deps.KnowledgeConceptDetailHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/concepts/c1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var concept knowledge.ConceptPage
	if err := json.Unmarshal(rec.Body.Bytes(), &concept); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if concept.Title != "Negotiation" {
		t.Errorf("expected title=Negotiation, got %q", concept.Title)
	}
}

// --- T3-10: GET /concepts/{invalid-id} → 404 ---

func TestKnowledgeConceptDetailHandler_NotFound(t *testing.T) {
	store := &mockKnowledgeStore{
		concept:    nil,
		conceptErr: pgxNoRowsError(),
	}

	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      time.Now(),
		KnowledgeStore: store,
	}

	r := chi.NewRouter()
	r.Get("/api/knowledge/concepts/{id}", deps.KnowledgeConceptDetailHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/concepts/nonexistent", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", resp.Error.Code)
	}
}

// --- T3-07: GET /entities → list ---

func TestKnowledgeEntitiesHandler_List(t *testing.T) {
	now := time.Now()
	store := &mockKnowledgeStore{
		entities: []*knowledge.EntityProfile{
			{
				ID:               "e1",
				Name:             "John Doe",
				EntityType:       "person",
				Summary:          "A contact",
				SourceTypes:      []string{"email"},
				InteractionCount: 5,
				UpdatedAt:        now,
			},
		},
		entitiesTotal: 1,
	}

	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      now,
		KnowledgeStore: store,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/entities?q=john&limit=10", nil)
	rec := httptest.NewRecorder()

	deps.KnowledgeEntitiesHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp EntityListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Total)
	}
	if len(resp.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(resp.Entities))
	}
	if resp.Entities[0].Name != "John Doe" {
		t.Errorf("expected name=John Doe, got %q", resp.Entities[0].Name)
	}
}

// --- T3-08: GET /entities/{id} → full entity detail ---

func TestKnowledgeEntityDetailHandler_Found(t *testing.T) {
	now := time.Now()
	store := &mockKnowledgeStore{
		entity: &knowledge.EntityProfile{
			ID:               "e1",
			Name:             "John Doe",
			EntityType:       "person",
			Summary:          "A frequent contact",
			Mentions:         json.RawMessage(`[{"artifact_id":"a1"}]`),
			InteractionCount: 5,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}

	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      now,
		KnowledgeStore: store,
	}

	r := chi.NewRouter()
	r.Get("/api/knowledge/entities/{id}", deps.KnowledgeEntityDetailHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/entities/e1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- T3-09: GET /knowledge/stats → correct counts ---

func TestKnowledgeStatsHandler(t *testing.T) {
	store := &mockKnowledgeStore{
		stats: &knowledge.KnowledgeStats{
			ConceptCount:       32,
			EntityCount:        87,
			EdgeCount:          150,
			SynthesisCompleted: 100,
			SynthesisPending:   5,
			SynthesisFailed:    2,
		},
	}

	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      time.Now(),
		KnowledgeStore: store,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/stats", nil)
	rec := httptest.NewRecorder()

	deps.KnowledgeStatsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var stats knowledge.KnowledgeStats
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stats.ConceptCount != 32 {
		t.Errorf("expected concept_count=32, got %d", stats.ConceptCount)
	}
	if stats.EntityCount != 87 {
		t.Errorf("expected entity_count=87, got %d", stats.EntityCount)
	}
}

// --- Knowledge unavailable when store is nil ---

func TestKnowledgeConceptsHandler_Unavailable(t *testing.T) {
	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      time.Now(),
		KnowledgeStore: nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/concepts", nil)
	rec := httptest.NewRecorder()

	deps.KnowledgeConceptsHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

// --- Lint report handler ---

func TestKnowledgeLintHandler_NoReport(t *testing.T) {
	store := &mockKnowledgeStore{
		lintReport: nil,
		lintErr:    pgxNoRowsError(),
	}

	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      time.Now(),
		KnowledgeStore: store,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/lint", nil)
	rec := httptest.NewRecorder()

	deps.KnowledgeLintHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error.Code != "NO_LINT_REPORT" {
		t.Errorf("expected NO_LINT_REPORT, got %q", resp.Error.Code)
	}
}

// --- Query param validation ---

func TestKnowledgeConceptsHandler_InvalidLimit(t *testing.T) {
	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      time.Now(),
		KnowledgeStore: &mockKnowledgeStore{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/concepts?limit=999", nil)
	rec := httptest.NewRecorder()

	deps.KnowledgeConceptsHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestKnowledgeConceptsHandler_InvalidOffset(t *testing.T) {
	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      time.Now(),
		KnowledgeStore: &mockKnowledgeStore{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge/concepts?offset=-1", nil)
	rec := httptest.NewRecorder()

	deps.KnowledgeConceptsHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Auth required for knowledge endpoints ---

func TestKnowledgeEndpoints_RequireAuth(t *testing.T) {
	store := &mockKnowledgeStore{
		concepts:      []*knowledge.ConceptPage{},
		conceptsTotal: 0,
		entities:      []*knowledge.EntityProfile{},
		entitiesTotal: 0,
		stats:         &knowledge.KnowledgeStats{},
	}

	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      time.Now(),
		AuthToken:      "secret",
		KnowledgeStore: store,
	}

	router := NewRouter(deps)

	endpoints := []string{
		"/api/knowledge/concepts",
		"/api/knowledge/concepts/test-id",
		"/api/knowledge/entities",
		"/api/knowledge/entities/test-id",
		"/api/knowledge/lint",
		"/api/knowledge/stats",
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodGet, ep, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("endpoint %s: expected 401 without auth, got %d", ep, rec.Code)
		}
	}
}

// pgxNoRowsError returns an error that simulates pgx.ErrNoRows through the store layer.
func pgxNoRowsError() error {
	return fmt.Errorf("scan concept: %w", pgx.ErrNoRows)
}
