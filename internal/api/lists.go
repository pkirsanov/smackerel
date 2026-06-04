package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/list"
)

// maxListRequestBodyBytes caps every JSON body the list API accepts. 64 KiB is
// far above any legitimate request (title + a small handful of fields) and
// prevents an unbounded body from exhausting server memory before json.Decode
// can reject it.
const maxListRequestBodyBytes = 64 << 10

// maxAddItemContentLen and maxCreateListTitleLen cap user-supplied free-text
// fields server-side. Stable error codes let callers distinguish overflow from
// generic 400s.
const (
	maxAddItemContentLen  = 4096
	maxCreateListTitleLen = 256
)

// decodeListBody applies the body-size cap, decodes the JSON, and maps
// http.MaxBytesError to 413 with a stable error code. Returns false if the
// response was already written.
func decodeListBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxListRequestBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, `{"error":"request body exceeds 65536 bytes","code":"body_too_large"}`, http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return false
	}
	return true
}

// CreateListRequest is the JSON body for POST /api/lists.
type CreateListRequest struct {
	ListType    string   `json:"list_type"`
	Title       string   `json:"title"`
	ArtifactIDs []string `json:"artifact_ids,omitempty"`
	TagFilter   string   `json:"tag_filter,omitempty"`
	SearchQuery string   `json:"search_query,omitempty"`
	Domain      string   `json:"domain,omitempty"`
}

// AddItemRequest is the JSON body for POST /api/lists/{id}/items.
type AddItemRequest struct {
	Content  string `json:"content"`
	Category string `json:"category,omitempty"`
}

// CheckItemRequest is the JSON body for POST /api/lists/{id}/items/{itemId}/check.
type CheckItemRequest struct {
	Status       string `json:"status,omitempty"`       // "done", "skipped", "substituted"
	Substitution string `json:"substitution,omitempty"` // substitution text when status is "substituted"
}

// ListHandlers holds list API handler methods and dependencies.
type ListHandlers struct {
	Generator *list.Generator
	Store     list.ListStore
}

// CreateListHandler handles POST /api/lists.
func (h *ListHandlers) CreateListHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateListRequest
	if !decodeListBody(w, r, &req) {
		return
	}

	if req.Title == "" {
		http.Error(w, `{"error":"title is required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Title) > maxCreateListTitleLen {
		http.Error(w, `{"error":"title exceeds 256 characters","code":"title_too_long"}`, http.StatusBadRequest)
		return
	}

	if len(req.ArtifactIDs) == 0 && req.TagFilter == "" && req.SearchQuery == "" {
		http.Error(w, `{"error":"at least one of artifact_ids, tag_filter, or search_query is required"}`, http.StatusBadRequest)
		return
	}

	genReq := list.GenerateRequest{
		ListType:    list.ListType(req.ListType),
		Title:       req.Title,
		ArtifactIDs: req.ArtifactIDs,
		TagFilter:   req.TagFilter,
		SearchQuery: req.SearchQuery,
		Domain:      req.Domain,
	}

	result, err := h.Generator.Generate(r.Context(), genReq)
	if err != nil {
		slog.Error("failed to generate list", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// GetListHandler handles GET /api/lists/{id}.
func (h *ListHandlers) GetListHandler(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	if listID == "" {
		http.Error(w, `{"error":"list id required"}`, http.StatusBadRequest)
		return
	}

	result, err := h.Store.GetList(r.Context(), listID)
	if err != nil {
		slog.Error("failed to get list", "list_id", listID, "error", err)
		http.Error(w, `{"error":"list not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ListListsHandler handles GET /api/lists.
func (h *ListHandlers) ListListsHandler(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	typeFilter := r.URL.Query().Get("type")

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	lists, err := h.Store.ListLists(r.Context(), statusFilter, typeFilter, limit, offset)
	if err != nil {
		slog.Error("failed to list lists", "error", err)
		http.Error(w, `{"error":"failed to list lists"}`, http.StatusInternalServerError)
		return
	}

	if lists == nil {
		lists = []list.List{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"lists": lists,
		"count": len(lists),
	})
}

// AddItemHandler handles POST /api/lists/{id}/items.
func (h *ListHandlers) AddItemHandler(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	if listID == "" {
		http.Error(w, `{"error":"list id required"}`, http.StatusBadRequest)
		return
	}

	var req AddItemRequest
	if !decodeListBody(w, r, &req) {
		return
	}

	if req.Content == "" {
		http.Error(w, `{"error":"content is required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Content) > maxAddItemContentLen {
		http.Error(w, `{"error":"content exceeds 4096 characters","code":"content_too_long"}`, http.StatusBadRequest)
		return
	}

	item, err := h.Store.AddManualItem(r.Context(), listID, req.Content, req.Category)
	if err != nil {
		slog.Error("failed to add item", "list_id", listID, "error", err)
		http.Error(w, `{"error":"failed to add item"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(item)
}

// CheckItemHandler handles POST /api/lists/{id}/items/{itemId}/check.
func (h *ListHandlers) CheckItemHandler(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "itemId")
	if listID == "" || itemID == "" {
		http.Error(w, `{"error":"list id and item id required"}`, http.StatusBadRequest)
		return
	}

	var req CheckItemRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxListRequestBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, `{"error":"request body exceeds 65536 bytes","code":"body_too_large"}`, http.StatusRequestEntityTooLarge)
			return
		}
		// Default to "done" when the body is empty or malformed (back-compat:
		// the legacy check endpoint accepts an empty POST).
		req.Status = "done"
	}

	status := list.ItemDone
	switch req.Status {
	case "skipped":
		status = list.ItemSkipped
	case "substituted":
		status = list.ItemSubstituted
	default:
		status = list.ItemDone
	}

	err := h.Store.UpdateItemStatus(r.Context(), listID, itemID, status, req.Substitution)
	if err != nil {
		slog.Error("failed to check item", "list_id", listID, "item_id", itemID, "error", err)
		http.Error(w, `{"error":"failed to update item"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": string(status)})
}

// CompleteListHandler handles POST /api/lists/{id}/complete.
func (h *ListHandlers) CompleteListHandler(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	if listID == "" {
		http.Error(w, `{"error":"list id required"}`, http.StatusBadRequest)
		return
	}

	err := h.Store.CompleteList(r.Context(), listID)
	if err != nil {
		slog.Error("failed to complete list", "list_id", listID, "error", err)
		http.Error(w, `{"error":"failed to complete list"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "completed"})
}

// UpdateListRequest is the JSON body for PATCH /api/lists/{id}.
type UpdateListRequest struct {
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"` // "active", "archived"
}

// UpdateListHandler handles PATCH /api/lists/{id}.
func (h *ListHandlers) UpdateListHandler(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	if listID == "" {
		http.Error(w, `{"error":"list id required"}`, http.StatusBadRequest)
		return
	}

	var req UpdateListRequest
	if !decodeListBody(w, r, &req) {
		return
	}

	if req.Status == "archived" {
		if err := h.Store.ArchiveList(r.Context(), listID); err != nil {
			slog.Error("failed to archive list", "list_id", listID, "error", err)
			http.Error(w, `{"error":"failed to archive list"}`, http.StatusInternalServerError)
			return
		}
	}

	result, err := h.Store.GetList(r.Context(), listID)
	if err != nil {
		slog.Error("failed to get list after update", "list_id", listID, "error", err)
		http.Error(w, `{"error":"list not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ArchiveListHandler handles DELETE /api/lists/{id} (soft delete → archived).
func (h *ListHandlers) ArchiveListHandler(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	if listID == "" {
		http.Error(w, `{"error":"list id required"}`, http.StatusBadRequest)
		return
	}

	err := h.Store.ArchiveList(r.Context(), listID)
	if err != nil {
		slog.Error("failed to archive list", "list_id", listID, "error", err)
		http.Error(w, `{"error":"failed to archive list"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "archived"})
}

// RemoveItemHandler handles DELETE /api/lists/{id}/items/{itemId}.
func (h *ListHandlers) RemoveItemHandler(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "itemId")
	if listID == "" || itemID == "" {
		http.Error(w, `{"error":"list id and item id required"}`, http.StatusBadRequest)
		return
	}

	err := h.Store.RemoveItem(r.Context(), listID, itemID)
	if err != nil {
		slog.Error("failed to remove item", "list_id", listID, "item_id", itemID, "error", err)
		http.Error(w, `{"error":"failed to remove item"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
