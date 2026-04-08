package pipeline

import (
	"sync"
	"testing"

	"github.com/smackerel/smackerel/internal/extract"
)

func TestDedupChecker_NilPool(t *testing.T) {
	checker := &DedupChecker{Pool: nil}
	if checker.Pool != nil {
		t.Error("expected nil pool")
	}
}

func TestDedupResult_NotDuplicate(t *testing.T) {
	result := &DedupResult{IsDuplicate: false}
	if result.IsDuplicate {
		t.Error("expected not duplicate")
	}
	if result.ExistingID != "" {
		t.Error("expected empty existing ID for non-duplicate")
	}
}

func TestDedupResult_IsDuplicate(t *testing.T) {
	result := &DedupResult{
		IsDuplicate: true,
		ExistingID:  "01HXYZ",
		Title:       "Test Article",
	}
	if !result.IsDuplicate {
		t.Error("expected duplicate")
	}
	if result.ExistingID != "01HXYZ" {
		t.Errorf("expected existing ID '01HXYZ', got %q", result.ExistingID)
	}
	if result.Title != "Test Article" {
		t.Errorf("expected title 'Test Article', got %q", result.Title)
	}
}

func TestDedupResult_DuplicateWithEmptyTitle(t *testing.T) {
	result := &DedupResult{
		IsDuplicate: true,
		ExistingID:  "01ABC",
		Title:       "",
	}
	if !result.IsDuplicate {
		t.Error("expected duplicate even with empty title")
	}
	if result.ExistingID == "" {
		t.Error("existing ID should not be empty for duplicate")
	}
}

func TestDedupError_Fields(t *testing.T) {
	err := &DuplicateError{
		ExistingID: "01DEF",
		Title:      "Existing Article",
	}
	if err.ExistingID != "01DEF" {
		t.Errorf("expected '01DEF', got %q", err.ExistingID)
	}
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("error message should not be empty")
	}
}

func TestHashContent_ConcurrentSafety(t *testing.T) {
	const goroutines = 10
	input := "identical content for concurrent hashing"

	results := make([]string, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = extract.HashContent(input)
		}(i)
	}
	wg.Wait()

	for i := 1; i < goroutines; i++ {
		if results[i] != results[0] {
			t.Errorf("goroutine %d produced hash %q, expected %q", i, results[i], results[0])
		}
	}
}
