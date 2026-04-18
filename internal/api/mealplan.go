package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/mealplan"
)

// MealPlanHandler handles meal plan API endpoints.
type MealPlanHandler struct {
	Service  *mealplan.Service
	Shopping *mealplan.ShoppingBridge
	Calendar *mealplan.CalendarBridge
}

// NewMealPlanHandler creates a new meal plan handler.
func NewMealPlanHandler(svc *mealplan.Service, shopping *mealplan.ShoppingBridge, calendar *mealplan.CalendarBridge) *MealPlanHandler {
	return &MealPlanHandler{Service: svc, Shopping: shopping, Calendar: calendar}
}

// RegisterRoutes registers meal plan API routes on the given Chi router.
func (h *MealPlanHandler) RegisterRoutes(r chi.Router) {
	r.Route("/api/meal-plans", func(r chi.Router) {
		r.Post("/", h.CreatePlan)
		r.Get("/", h.ListPlans)
		r.Get("/query", h.QueryByDate)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetPlan)
			r.Patch("/", h.UpdatePlan)
			r.Delete("/", h.DeletePlan)
			r.Post("/slots", h.AddSlot)
			r.Patch("/slots/{slotId}", h.UpdateSlot)
			r.Delete("/slots/{slotId}", h.DeleteSlot)
			r.Post("/shopping-list", h.GenerateShoppingList)
			r.Post("/copy", h.CopyPlan)
			r.Post("/calendar-sync", h.CalendarSync)
		})
	})
}

type createPlanRequest struct {
	Title     string `json:"title"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

func (h *MealPlanHandler) CreatePlan(w http.ResponseWriter, r *http.Request) {
	var req createPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "invalid JSON body")
		return
	}

	if req.Title == "" {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "title is required")
		return
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "start_date must be YYYY-MM-DD")
		return
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "end_date must be YYYY-MM-DD")
		return
	}

	result, err := h.Service.CreatePlan(r.Context(), req.Title, startDate, endDate)
	if err != nil {
		handleMealPlanServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (h *MealPlanHandler) ListPlans(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	statusFilter := q.Get("status")

	var fromDate, toDate *time.Time
	if v := q.Get("from"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "from must be YYYY-MM-DD")
			return
		}
		fromDate = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "to must be YYYY-MM-DD")
			return
		}
		toDate = &t
	}

	plans, err := h.Service.ListPlans(r.Context(), statusFilter, fromDate, toDate)
	if err != nil {
		slog.Error("list plans failed", "error", err)
		writeMealPlanError(w, http.StatusInternalServerError, "MEAL_PLAN_INTERNAL", "failed to list plans")
		return
	}

	if plans == nil {
		plans = []mealplan.Plan{}
	}
	writeJSON(w, http.StatusOK, plans)
}

func (h *MealPlanHandler) GetPlan(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	result, err := h.Service.GetPlan(r.Context(), planID)
	if err != nil {
		handleMealPlanServiceError(w, err)
		return
	}
	if result == nil {
		writeMealPlanError(w, http.StatusNotFound, "MEAL_PLAN_NOT_FOUND", "plan not found")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type updatePlanRequest struct {
	Title  *string `json:"title,omitempty"`
	Status *string `json:"status,omitempty"`
}

func (h *MealPlanHandler) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	var req updatePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "invalid JSON body")
		return
	}

	force := r.URL.Query().Get("force") == "true"

	if req.Title != nil && *req.Title != "" {
		if err := h.Service.UpdatePlanTitle(r.Context(), planID, *req.Title); err != nil {
			handleMealPlanServiceError(w, err)
			return
		}
	}

	if req.Status != nil {
		newStatus := mealplan.PlanStatus(*req.Status)
		if newStatus == mealplan.StatusActive {
			overlap, err := h.Service.ActivatePlan(r.Context(), planID, force)
			if err != nil {
				if svcErr, ok := err.(*mealplan.ServiceError); ok && svcErr.Code == "MEAL_PLAN_OVERLAP" {
					writeJSON(w, svcErr.Status, map[string]any{
						"error":   svcErr.Message,
						"code":    svcErr.Code,
						"details": overlap,
					})
					return
				}
				handleMealPlanServiceError(w, err)
				return
			}
		} else {
			if err := h.Service.TransitionPlan(r.Context(), planID, newStatus); err != nil {
				handleMealPlanServiceError(w, err)
				return
			}
		}
	}

	// Return updated plan
	result, err := h.Service.GetPlan(r.Context(), planID)
	if err != nil {
		handleMealPlanServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *MealPlanHandler) DeletePlan(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	if err := h.Service.DeletePlan(r.Context(), planID); err != nil {
		handleMealPlanServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type addSlotRequest struct {
	SlotDate         string `json:"slot_date"`
	MealType         string `json:"meal_type"`
	RecipeArtifactID string `json:"recipe_artifact_id"`
	Servings         int    `json:"servings"`
	BatchFlag        bool   `json:"batch_flag"`
	Notes            string `json:"notes"`
}

func (h *MealPlanHandler) AddSlot(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	var req addSlotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "invalid JSON body")
		return
	}

	slotDate, err := time.Parse("2006-01-02", req.SlotDate)
	if err != nil {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "slot_date must be YYYY-MM-DD")
		return
	}

	if req.MealType == "" {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "meal_type is required")
		return
	}
	if req.RecipeArtifactID == "" {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "recipe_artifact_id is required")
		return
	}

	slot, err := h.Service.AddSlot(r.Context(), planID, slotDate, req.MealType, req.RecipeArtifactID, req.Servings, req.BatchFlag, req.Notes)
	if err != nil {
		handleMealPlanServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, slot)
}

type updateSlotRequest struct {
	RecipeArtifactID string `json:"recipe_artifact_id"`
	Servings         int    `json:"servings"`
	BatchFlag        bool   `json:"batch_flag"`
	Notes            string `json:"notes"`
}

func (h *MealPlanHandler) UpdateSlot(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	slotID := chi.URLParam(r, "slotId")

	var req updateSlotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "invalid JSON body")
		return
	}

	slot, err := h.Service.UpdateSlot(r.Context(), planID, slotID, req.RecipeArtifactID, req.Servings, req.BatchFlag, req.Notes)
	if err != nil {
		handleMealPlanServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, slot)
}

func (h *MealPlanHandler) DeleteSlot(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	slotID := chi.URLParam(r, "slotId")

	if err := h.Service.DeleteSlot(r.Context(), planID, slotID); err != nil {
		handleMealPlanServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *MealPlanHandler) QueryByDate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dateStr := q.Get("date")
	if dateStr == "" {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "date query parameter is required")
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "date must be YYYY-MM-DD")
		return
	}

	mealType := q.Get("meal")
	slots, plan, err := h.Service.QueryByDate(r.Context(), date, mealType)
	if err != nil {
		slog.Error("query by date failed", "error", err)
		writeMealPlanError(w, http.StatusInternalServerError, "MEAL_PLAN_INTERNAL", "query failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"plan":  plan,
		"slots": slots,
	})
}

func (h *MealPlanHandler) GenerateShoppingList(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	force := r.URL.Query().Get("force") == "true"

	if h.Shopping == nil {
		writeMealPlanError(w, http.StatusInternalServerError, "MEAL_PLAN_INTERNAL", "shopping bridge not configured")
		return
	}

	plan, err := h.Service.GetPlan(r.Context(), planID)
	if err != nil {
		handleMealPlanServiceError(w, err)
		return
	}
	if plan == nil {
		writeMealPlanError(w, http.StatusNotFound, "MEAL_PLAN_NOT_FOUND", "plan not found")
		return
	}

	if len(plan.Slots) == 0 {
		writeMealPlanError(w, http.StatusUnprocessableEntity, "MEAL_PLAN_EMPTY", "plan has no recipe assignments")
		return
	}

	result, err := h.Shopping.GenerateFromPlan(r.Context(), *plan, force)
	if err != nil {
		handleMealPlanServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

type copyPlanRequest struct {
	NewStartDate     string         `json:"new_start_date"`
	NewTitle         string         `json:"new_title"`
	ServingOverrides map[string]int `json:"serving_overrides,omitempty"`
}

func (h *MealPlanHandler) CopyPlan(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	var req copyPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "invalid JSON body")
		return
	}

	if req.NewStartDate == "" {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "new_start_date is required")
		return
	}

	newStart, err := time.Parse("2006-01-02", req.NewStartDate)
	if err != nil {
		writeMealPlanError(w, http.StatusBadRequest, "MEAL_PLAN_VALIDATION", "new_start_date must be YYYY-MM-DD")
		return
	}

	result, err := h.Service.CopyPlan(r.Context(), planID, newStart, req.NewTitle, req.ServingOverrides)
	if err != nil {
		handleMealPlanServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (h *MealPlanHandler) CalendarSync(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")

	if h.Calendar == nil || !h.Service.CalendarSync {
		writeMealPlanError(w, http.StatusUnprocessableEntity, "MEAL_PLAN_CALDAV_NOT_CONFIGURED",
			"CalDAV calendar sync is not configured. Enable meal_planning.calendar_sync in smackerel.yaml and configure a CalDAV connector.")
		return
	}

	plan, err := h.Service.GetPlan(r.Context(), planID)
	if err != nil {
		handleMealPlanServiceError(w, err)
		return
	}
	if plan == nil {
		writeMealPlanError(w, http.StatusNotFound, "MEAL_PLAN_NOT_FOUND", "plan not found")
		return
	}

	result, err := h.Calendar.SyncPlan(r.Context(), *plan)
	if err != nil {
		slog.Error("calendar sync failed", "plan_id", planID, "error", err)
		writeMealPlanError(w, http.StatusInternalServerError, "MEAL_PLAN_INTERNAL", "calendar sync failed")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// --- helpers ---

func writeMealPlanError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
		"code":  code,
	})
}

func handleMealPlanServiceError(w http.ResponseWriter, err error) {
	if svcErr, ok := err.(*mealplan.ServiceError); ok {
		resp := map[string]any{
			"error": svcErr.Message,
			"code":  svcErr.Code,
		}
		if svcErr.Details != nil {
			resp["details"] = svcErr.Details
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(svcErr.Status)
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	slog.Error("meal plan internal error", "error", err)
	writeMealPlanError(w, http.StatusInternalServerError, "MEAL_PLAN_INTERNAL", "internal server error")
}
