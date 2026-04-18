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
