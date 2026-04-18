package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMealPlanHandler_CreatePlan_MissingTitle(t *testing.T) {
	handler := &MealPlanHandler{}
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	body := `{"title":"","start_date":"2026-04-20","end_date":"2026-04-26"}`
	req := httptest.NewRequest(http.MethodPost, "/api/meal-plans", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["code"] != "MEAL_PLAN_VALIDATION" {
		t.Errorf("code = %q, want MEAL_PLAN_VALIDATION", resp["code"])
	}
}

func TestMealPlanHandler_CreatePlan_InvalidDateFormat(t *testing.T) {
	handler := &MealPlanHandler{}
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	body := `{"title":"Test Plan","start_date":"not-a-date","end_date":"2026-04-26"}`
	req := httptest.NewRequest(http.MethodPost, "/api/meal-plans", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestMealPlanHandler_AddSlot_MissingFields(t *testing.T) {
	handler := &MealPlanHandler{}
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Missing meal_type
	body := `{"slot_date":"2026-04-20","meal_type":"","recipe_artifact_id":"abc","servings":4}`
	req := httptest.NewRequest(http.MethodPost, "/api/meal-plans/plan-1/slots", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestMealPlanHandler_QueryByDate_MissingDate(t *testing.T) {
	handler := &MealPlanHandler{}
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/meal-plans/query", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestMealPlanHandler_CalendarSync_NotConfigured(t *testing.T) {
	handler := &MealPlanHandler{} // nil Calendar and CalendarSync=false by default
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/api/meal-plans/plan-1/calendar-sync", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnprocessableEntity)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["code"] != "MEAL_PLAN_CALDAV_NOT_CONFIGURED" {
		t.Errorf("code = %q, want MEAL_PLAN_CALDAV_NOT_CONFIGURED", resp["code"])
	}
}

func TestMealPlanHandler_CopyPlan_MissingStartDate(t *testing.T) {
	handler := &MealPlanHandler{}
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	body := `{"new_title":"Copy"}`
	req := httptest.NewRequest(http.MethodPost, "/api/meal-plans/plan-1/copy", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestWriteMealPlanError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeMealPlanError(rr, http.StatusNotFound, "MEAL_PLAN_NOT_FOUND", "plan not found")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["code"] != "MEAL_PLAN_NOT_FOUND" {
		t.Errorf("code = %q, want MEAL_PLAN_NOT_FOUND", resp["code"])
	}
	if resp["error"] != "plan not found" {
		t.Errorf("error = %q, want %q", resp["error"], "plan not found")
	}
}
