package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smackerel/smackerel/internal/config"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

type fakeRecommendationStore struct {
	input recstore.CreateRequestInput
}

func (s *fakeRecommendationStore) CreateNoProviderRequest(_ context.Context, input recstore.CreateRequestInput) (recstore.RequestRecord, error) {
	s.input = input
	return recstore.RequestRecord{ID: "req-test", TraceID: "trace-test", Status: input.Status}, nil
}

type emptyRecommendationRegistry struct{}

func (emptyRecommendationRegistry) Len() int { return 0 }

func (emptyRecommendationRegistry) List() []recprovider.Provider { return nil }

func TestRecommendationCreateRequestNoProvidersPersistsAndReturnsOutcome(t *testing.T) {
	store := &fakeRecommendationStore{}
	h := NewRecommendationHandlers(store, emptyRecommendationRegistry{}, config.RecommendationsConfig{
		Ranking: config.RecommendationRankingConfig{StandardStyle: "balanced", StandardResultCount: 3},
	})

	body := bytes.NewBufferString(`{"query":"quiet ramen nearby","source":"api","precision_policy":"neighborhood"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/recommendations/requests", body)

	h.CreateRequest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp createRecommendationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "no_providers" || resp.RequestID != "req-test" || resp.TraceID != "trace-test" {
		t.Fatalf("response = %+v, want no_providers req-test trace-test", resp)
	}
	if len(resp.Recommendations) != 0 {
		t.Fatalf("recommendations length = %d, want 0", len(resp.Recommendations))
	}
	if store.input.Status != "no_providers" {
		t.Fatalf("persisted status = %q, want no_providers", store.input.Status)
	}
	if store.input.LocationPrecisionRequested != "neighborhood" || store.input.LocationPrecisionSent != "neighborhood" {
		t.Fatalf("precision persisted as requested=%q sent=%q", store.input.LocationPrecisionRequested, store.input.LocationPrecisionSent)
	}
}

func TestRecommendationCreateRequestRejectsMissingPrecision(t *testing.T) {
	h := NewRecommendationHandlers(&fakeRecommendationStore{}, emptyRecommendationRegistry{}, config.RecommendationsConfig{
		Ranking: config.RecommendationRankingConfig{StandardStyle: "balanced", StandardResultCount: 3},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/recommendations/requests", bytes.NewBufferString(`{"query":"quiet ramen","source":"api"}`))
	h.CreateRequest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
