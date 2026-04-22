package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/list"
)

// mockListStore implements list.ListStore for API handler tests.
type mockListStore struct {
	lists map[string]*list.ListWithItems
}

func newMockListStore() *mockListStore {
	return &mockListStore{lists: make(map[string]*list.ListWithItems)}
}

func (m *mockListStore) CreateList(_ context.Context, l *list.List, items []list.ListItem) error {
	l.TotalItems = len(items)
	m.lists[l.ID] = &list.ListWithItems{List: *l, Items: items}
	return nil
}

func (m *mockListStore) GetList(_ context.Context, id string) (*list.ListWithItems, error) {
	if lwi, ok := m.lists[id]; ok {
		return lwi, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockListStore) ListLists(_ context.Context, statusFilter, typeFilter string, limit, offset int) ([]list.List, error) {
	var result []list.List
	for _, lwi := range m.lists {
		if statusFilter != "" && string(lwi.List.Status) != statusFilter {
			continue
		}
		if typeFilter != "" && string(lwi.List.ListType) != typeFilter {
			continue
		}
		result = append(result, lwi.List)
	}
	return result, nil
}

func (m *mockListStore) UpdateItemStatus(_ context.Context, listID, itemID string, status list.ItemStatus, sub string) error {
	lwi, ok := m.lists[listID]
	if !ok {
		return fmt.Errorf("list not found")
	}
	for i := range lwi.Items {
		if lwi.Items[i].ID == itemID {
			lwi.Items[i].Status = status
			lwi.Items[i].Substitution = sub
			return nil
		}
	}
	return fmt.Errorf("item not found")
}

func (m *mockListStore) AddManualItem(_ context.Context, listID, content, category string) (*list.ListItem, error) {
	item := &list.ListItem{
		ID:       "new-item-1",
		ListID:   listID,
		Content:  content,
		Category: category,
		Status:   list.ItemPending,
		IsManual: true,
	}
	if lwi, ok := m.lists[listID]; ok {
		lwi.Items = append(lwi.Items, *item)
	}
	return item, nil
}

func (m *mockListStore) CompleteList(_ context.Context, id string) error {
	if lwi, ok := m.lists[id]; ok {
		lwi.List.Status = list.StatusCompleted
		return nil
	}
	return fmt.Errorf("not found")
}

func (m *mockListStore) ArchiveList(_ context.Context, id string) error {
	if lwi, ok := m.lists[id]; ok {
		lwi.List.Status = list.StatusArchived
		return nil
	}
	return fmt.Errorf("not found")
}

func (m *mockListStore) RemoveItem(_ context.Context, listID, itemID string) error {
	lwi, ok := m.lists[listID]
	if !ok {
		return fmt.Errorf("list not found")
	}
	for i := range lwi.Items {
		if lwi.Items[i].ID == itemID {
			lwi.Items = append(lwi.Items[:i], lwi.Items[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("item not found")
}

func seedTestList(store *mockListStore) {
	store.lists["test-list-1"] = &list.ListWithItems{
		List: list.List{
			ID:       "test-list-1",
			ListType: list.TypeShopping,
			Title:    "Test Shopping List",
			Status:   list.StatusActive,
		},
		Items: []list.ListItem{
			{ID: "item-1", ListID: "test-list-1", Content: "Garlic", Status: list.ItemPending},
			{ID: "item-2", ListID: "test-list-1", Content: "Flour", Status: list.ItemPending},
		},
	}
}

func TestGetListHandler(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Get("/api/lists/{id}", h.GetListHandler)

	req := httptest.NewRequest("GET", "/api/lists/test-list-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result list.ListWithItems
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.List.Title != "Test Shopping List" {
		t.Errorf("expected title 'Test Shopping List', got %q", result.List.Title)
	}
	if len(result.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(result.Items))
	}
}

func TestGetListHandler_NotFound(t *testing.T) {
	store := newMockListStore()
	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Get("/api/lists/{id}", h.GetListHandler)

	req := httptest.NewRequest("GET", "/api/lists/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestListListsHandler(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Get("/api/lists", h.ListListsHandler)

	req := httptest.NewRequest("GET", "/api/lists?status=active", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	lists := result["lists"].([]any)
	if len(lists) != 1 {
		t.Errorf("expected 1 active list, got %d", len(lists))
	}
}

func TestListListsHandler_Empty(t *testing.T) {
	store := newMockListStore()
	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Get("/api/lists", h.ListListsHandler)

	req := httptest.NewRequest("GET", "/api/lists", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	lists := result["lists"].([]any)
	if len(lists) != 0 {
		t.Errorf("expected 0 lists, got %d", len(lists))
	}
}

func TestAddItemHandler(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists/{id}/items", h.AddItemHandler)

	body := `{"content":"paper towels","category":"household"}`
	req := httptest.NewRequest("POST", "/api/lists/test-list-1/items", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var item list.ListItem
	if err := json.NewDecoder(w.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	if item.Content != "paper towels" {
		t.Errorf("expected content 'paper towels', got %q", item.Content)
	}
	if !item.IsManual {
		t.Error("expected is_manual=true")
	}
}

func TestAddItemHandler_MissingContent(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists/{id}/items", h.AddItemHandler)

	body := `{"category":"household"}`
	req := httptest.NewRequest("POST", "/api/lists/test-list-1/items", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCheckItemHandler(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists/{id}/items/{itemId}/check", h.CheckItemHandler)

	body := `{"status":"done"}`
	req := httptest.NewRequest("POST", "/api/lists/test-list-1/items/item-1/check", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify item was updated in store
	lwi := store.lists["test-list-1"]
	if lwi.Items[0].Status != list.ItemDone {
		t.Errorf("expected item status done, got %s", lwi.Items[0].Status)
	}
}

func TestCheckItemHandler_DefaultDone(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists/{id}/items/{itemId}/check", h.CheckItemHandler)

	// Empty body should default to "done"
	req := httptest.NewRequest("POST", "/api/lists/test-list-1/items/item-2/check", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCompleteListHandler(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists/{id}/complete", h.CompleteListHandler)

	req := httptest.NewRequest("POST", "/api/lists/test-list-1/complete", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify list was completed in store
	lwi := store.lists["test-list-1"]
	if lwi.List.Status != list.StatusCompleted {
		t.Errorf("expected status completed, got %s", lwi.List.Status)
	}
}

func TestCompleteListHandler_NotFound(t *testing.T) {
	store := newMockListStore()
	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists/{id}/complete", h.CompleteListHandler)

	req := httptest.NewRequest("POST", "/api/lists/nonexistent/complete", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateListHandler_MissingTitle(t *testing.T) {
	store := newMockListStore()
	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists", h.CreateListHandler)

	body := `{"list_type":"shopping","artifact_ids":["a1"]}`
	req := httptest.NewRequest("POST", "/api/lists", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateListHandler_NoSources(t *testing.T) {
	store := newMockListStore()
	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists", h.CreateListHandler)

	body := `{"list_type":"shopping","title":"My List"}`
	req := httptest.NewRequest("POST", "/api/lists", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateListHandler_InvalidJSON(t *testing.T) {
	store := newMockListStore()
	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists", h.CreateListHandler)

	req := httptest.NewRequest("POST", "/api/lists", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCheckItemHandler_SkipItem(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists/{id}/items/{itemId}/check", h.CheckItemHandler)

	body := `{"status":"skipped"}`
	req := httptest.NewRequest("POST", "/api/lists/test-list-1/items/item-1/check", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify item was skipped in store
	lwi := store.lists["test-list-1"]
	if lwi.Items[0].Status != list.ItemSkipped {
		t.Errorf("expected item status skipped, got %s", lwi.Items[0].Status)
	}

	// Verify response body
	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "skipped" {
		t.Errorf("expected response status 'skipped', got %q", result["status"])
	}
}

func TestCheckItemHandler_SubstituteItem(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists/{id}/items/{itemId}/check", h.CheckItemHandler)

	body := `{"status":"substituted","substitution":"almond milk"}`
	req := httptest.NewRequest("POST", "/api/lists/test-list-1/items/item-1/check", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify item was substituted in store with the substitution text
	lwi := store.lists["test-list-1"]
	if lwi.Items[0].Status != list.ItemSubstituted {
		t.Errorf("expected item status substituted, got %s", lwi.Items[0].Status)
	}
	if lwi.Items[0].Substitution != "almond milk" {
		t.Errorf("expected substitution 'almond milk', got %q", lwi.Items[0].Substitution)
	}

	// Verify response body
	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "substituted" {
		t.Errorf("expected response status 'substituted', got %q", result["status"])
	}
}

func TestCheckItemHandler_ItemNotFound(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists/{id}/items/{itemId}/check", h.CheckItemHandler)

	body := `{"status":"done"}`
	req := httptest.NewRequest("POST", "/api/lists/test-list-1/items/nonexistent/check", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for missing item, got %d", w.Code)
	}
}

func TestListListsHandler_FilterByType(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)
	// Add a reading list
	store.lists["test-list-2"] = &list.ListWithItems{
		List: list.List{
			ID:       "test-list-2",
			ListType: list.TypeReading,
			Title:    "My Reading List",
			Status:   list.StatusActive,
		},
		Items: []list.ListItem{},
	}

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Get("/api/lists", h.ListListsHandler)

	req := httptest.NewRequest("GET", "/api/lists?type=reading", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	lists := result["lists"].([]any)
	if len(lists) != 1 {
		t.Errorf("expected 1 reading list, got %d", len(lists))
	}
}

func TestArchiveListHandler(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Delete("/api/lists/{id}", h.ArchiveListHandler)

	req := httptest.NewRequest("DELETE", "/api/lists/test-list-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify list was archived in store
	lwi := store.lists["test-list-1"]
	if lwi.List.Status != list.StatusArchived {
		t.Errorf("expected status archived, got %s", lwi.List.Status)
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "archived" {
		t.Errorf("expected response status 'archived', got %q", result["status"])
	}
}

func TestArchiveListHandler_NotFound(t *testing.T) {
	store := newMockListStore()
	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Delete("/api/lists/{id}", h.ArchiveListHandler)

	req := httptest.NewRequest("DELETE", "/api/lists/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestUpdateListHandler_ArchiveViaUpdate(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Patch("/api/lists/{id}", h.UpdateListHandler)

	body := `{"status":"archived"}`
	req := httptest.NewRequest("PATCH", "/api/lists/test-list-1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify list was archived
	lwi := store.lists["test-list-1"]
	if lwi.List.Status != list.StatusArchived {
		t.Errorf("expected status archived, got %s", lwi.List.Status)
	}
}

func TestUpdateListHandler_InvalidJSON(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Patch("/api/lists/{id}", h.UpdateListHandler)

	req := httptest.NewRequest("PATCH", "/api/lists/test-list-1", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRemoveItemHandler(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Delete("/api/lists/{id}/items/{itemId}", h.RemoveItemHandler)

	req := httptest.NewRequest("DELETE", "/api/lists/test-list-1/items/item-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify item was removed from store
	lwi := store.lists["test-list-1"]
	for _, item := range lwi.Items {
		if item.ID == "item-1" {
			t.Error("item-1 should have been removed")
		}
	}
}

func TestRemoveItemHandler_NotFound(t *testing.T) {
	store := newMockListStore()
	seedTestList(store)

	h := &ListHandlers{Store: store}

	r := chi.NewRouter()
	r.Delete("/api/lists/{id}/items/{itemId}", h.RemoveItemHandler)

	req := httptest.NewRequest("DELETE", "/api/lists/test-list-1/items/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// TestCreateListHandler_Success tests the happy path for POST /api/lists.
// Gherkin Scope 6: "Create shopping list via API"
// Given 2 recipe artifacts with domain_data exist
// When POST /api/lists is called with {"list_type":"shopping","artifact_ids":["a1","a2"]}
// Then status 201 is returned with the generated list and items
func TestCreateListHandler_Success(t *testing.T) {
	store := newMockListStore()

	resolver := &mockAPIArtifactResolver{
		byIDs: map[string][]list.AggregationSource{
			"a1": {{
				ArtifactID: "a1",
				DomainData: []byte(`{"domain":"recipe","ingredients":[{"name":"garlic","quantity":"2","unit":"cloves"}]}`),
			}},
			"a2": {{
				ArtifactID: "a2",
				DomainData: []byte(`{"domain":"recipe","ingredients":[{"name":"garlic","quantity":"3","unit":"cloves"},{"name":"flour","quantity":"1","unit":"cup"}]}`),
			}},
		},
	}

	gen := list.NewGenerator(resolver, store, map[string]list.Aggregator{
		"recipe": &list.RecipeAggregator{},
	})

	h := &ListHandlers{Generator: gen, Store: store}

	r := chi.NewRouter()
	r.Post("/api/lists", h.CreateListHandler)

	body := `{"list_type":"shopping","title":"Weekend Groceries","artifact_ids":["a1","a2"]}`
	req := httptest.NewRequest("POST", "/api/lists", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result list.ListWithItems
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.List.Title != "Weekend Groceries" {
		t.Errorf("expected title 'Weekend Groceries', got %q", result.List.Title)
	}
	if result.List.ListType != list.TypeShopping {
		t.Errorf("expected type shopping, got %s", result.List.ListType)
	}
	if result.List.Status != list.StatusDraft {
		t.Errorf("expected status draft, got %s", result.List.Status)
	}
	// Garlic should be merged: 2+3 = 5 cloves. Flour is separate. Total 2 items.
	if len(result.Items) != 2 {
		t.Errorf("expected 2 items (merged garlic + flour), got %d", len(result.Items))
	}
}

// mockAPIArtifactResolver implements list.ArtifactResolver for API handler tests.
type mockAPIArtifactResolver struct {
	byIDs map[string][]list.AggregationSource
}

func (m *mockAPIArtifactResolver) ResolveByIDs(_ context.Context, ids []string) ([]list.AggregationSource, error) {
	var result []list.AggregationSource
	for _, id := range ids {
		if sources, ok := m.byIDs[id]; ok {
			result = append(result, sources...)
		}
	}
	return result, nil
}

func (m *mockAPIArtifactResolver) ResolveByTag(_ context.Context, _ string) ([]list.AggregationSource, error) {
	return nil, nil
}

func (m *mockAPIArtifactResolver) ResolveByQuery(_ context.Context, _ string) ([]list.AggregationSource, error) {
	return nil, nil
}
