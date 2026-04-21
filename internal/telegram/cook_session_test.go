package telegram

import (
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/recipe"
)

func TestCookSessionStore_CreateAndGet(t *testing.T) {
	store := NewCookSessionStore(120)

	session := &CookSession{
		RecipeArtifactID: "art-123",
		RecipeTitle:      "Pasta Carbonara",
		Steps:            makeTestSteps(6),
		Ingredients:      makeTestIngredients(5),
		CurrentStep:      1,
		TotalSteps:       6,
		ScaleFactor:      1.0,
	}

	store.Create(12345, session)

	got := store.Get(12345)
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.RecipeTitle != "Pasta Carbonara" {
		t.Errorf("expected title 'Pasta Carbonara', got %q", got.RecipeTitle)
	}
	if got.CurrentStep != 1 {
		t.Errorf("expected current step 1, got %d", got.CurrentStep)
	}
	if got.TotalSteps != 6 {
		t.Errorf("expected total steps 6, got %d", got.TotalSteps)
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestCookSessionStore_GetNonExistent(t *testing.T) {
	store := NewCookSessionStore(120)

	got := store.Get(99999)
	if got != nil {
		t.Fatal("expected nil for non-existent session")
	}
}

func TestCookSessionStore_UpdateStep(t *testing.T) {
	store := NewCookSessionStore(120)

	session := &CookSession{
		RecipeArtifactID: "art-123",
		RecipeTitle:      "Test",
		CurrentStep:      2,
		TotalSteps:       6,
		ScaleFactor:      1.0,
	}
	store.Create(12345, session)

	// Update step
	got := store.Get(12345)
	got.CurrentStep = 3
	store.Touch(12345)

	got2 := store.Get(12345)
	if got2.CurrentStep != 3 {
		t.Errorf("expected step 3, got %d", got2.CurrentStep)
	}
}

func TestCookSessionStore_Delete(t *testing.T) {
	store := NewCookSessionStore(120)

	session := &CookSession{
		RecipeArtifactID: "art-123",
		RecipeTitle:      "Test",
		CurrentStep:      1,
		TotalSteps:       3,
		ScaleFactor:      1.0,
	}
	store.Create(12345, session)

	store.Delete(12345)

	got := store.Get(12345)
	if got != nil {
		t.Fatal("expected nil after deletion")
	}
}

func TestCookSessionStore_Sweep(t *testing.T) {
	store := NewCookSessionStore(1) // 1 minute timeout

	session := &CookSession{
		RecipeArtifactID: "art-123",
		RecipeTitle:      "Test",
		CurrentStep:      1,
		TotalSteps:       3,
		ScaleFactor:      1.0,
	}
	store.Create(12345, session)

	// Manually set LastInteraction to the past
	got := store.Get(12345)
	got.LastInteraction = time.Now().Add(-2 * time.Minute)

	store.sweep()

	if store.Get(12345) != nil {
		t.Error("expected expired session to be swept")
	}
}

func TestCookSessionStore_SweepPreservesActive(t *testing.T) {
	store := NewCookSessionStore(120) // 120 minutes timeout

	session := &CookSession{
		RecipeArtifactID: "art-123",
		RecipeTitle:      "Test",
		CurrentStep:      1,
		TotalSteps:       3,
		ScaleFactor:      1.0,
	}
	store.Create(12345, session)

	store.sweep()

	if store.Get(12345) == nil {
		t.Error("expected active session to be preserved after sweep")
	}
}

func TestCookSessionStore_ReplaceExisting(t *testing.T) {
	store := NewCookSessionStore(120)

	session1 := &CookSession{
		RecipeArtifactID: "art-123",
		RecipeTitle:      "Recipe A",
		CurrentStep:      3,
		TotalSteps:       6,
		ScaleFactor:      1.0,
	}
	store.Create(12345, session1)

	session2 := &CookSession{
		RecipeArtifactID: "art-456",
		RecipeTitle:      "Recipe B",
		CurrentStep:      1,
		TotalSteps:       4,
		ScaleFactor:      2.0,
	}
	store.Create(12345, session2)

	got := store.Get(12345)
	if got.RecipeTitle != "Recipe B" {
		t.Errorf("expected 'Recipe B', got %q", got.RecipeTitle)
	}
	if got.CurrentStep != 1 {
		t.Errorf("expected step 1, got %d", got.CurrentStep)
	}
}

func TestCookSessionStore_TouchNonExistent(t *testing.T) {
	store := NewCookSessionStore(120)

	// Touch on a chat with no session should not panic
	store.Touch(99999)

	got := store.Get(99999)
	if got != nil {
		t.Error("expected nil for non-existent session after Touch")
	}
}

func TestCookSessionStore_DeleteNonExistent(t *testing.T) {
	store := NewCookSessionStore(120)

	// Delete on a chat with no session should not panic
	store.Delete(99999)
}

func TestCookSessionStore_StopIdempotent(t *testing.T) {
	store := NewCookSessionStore(120)
	store.StartCleanup()

	// Stop twice should not panic
	store.Stop()
	store.Stop()
}

func TestCookSessionStore_StopConcurrent(t *testing.T) {
	store := NewCookSessionStore(120)
	store.StartCleanup()

	// Concurrent Stop calls must not panic (double-close on channel)
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			store.Stop()
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestCookSessionStore_StartCleanupIdempotent(t *testing.T) {
	store := NewCookSessionStore(120)

	// Multiple StartCleanup calls must not spawn duplicate goroutines or panic
	store.StartCleanup()
	store.StartCleanup()
	store.StartCleanup()

	store.Stop()
}

func TestCookSessionStore_SweepCleansStaleDisambiguations(t *testing.T) {
	store := NewCookSessionStore(1) // 1 minute timeout

	// Set disambiguation without a corresponding session
	store.SetDisambiguation(12345, &CookDisambiguation{
		Options: []CookDisambiguationOption{
			{ArtifactID: "art-1", Title: "Recipe A"},
		},
	})

	// Verify disambiguation exists
	if store.GetDisambiguation(12345) == nil {
		t.Fatal("expected disambiguation to exist")
	}

	// Sweep should clean stale disambiguation (no corresponding session)
	store.sweep()

	if store.GetDisambiguation(12345) != nil {
		t.Error("expected stale disambiguation to be swept")
	}
}

func TestCookSessionStore_SweepPreservesDisambiguationWithSession(t *testing.T) {
	store := NewCookSessionStore(120) // 120 minutes timeout

	// Create session and disambiguation for same chat
	session := &CookSession{
		RecipeArtifactID: "art-123",
		RecipeTitle:      "Test",
		CurrentStep:      1,
		TotalSteps:       3,
		ScaleFactor:      1.0,
	}
	store.Create(12345, session)
	store.SetDisambiguation(12345, &CookDisambiguation{
		Options: []CookDisambiguationOption{
			{ArtifactID: "art-2", Title: "Recipe B"},
		},
	})

	store.sweep()

	// Session is active, so disambiguation should be preserved
	if store.GetDisambiguation(12345) == nil {
		t.Error("expected disambiguation to be preserved with active session")
	}
}

func makeTestSteps(n int) []recipe.Step {
	steps := make([]recipe.Step, n)
	for i := 0; i < n; i++ {
		dur := 5
		steps[i] = recipe.Step{
			Number:          i + 1,
			Instruction:     "Do step " + string(rune('A'+i)),
			DurationMinutes: &dur,
			Technique:       "technique",
		}
	}
	return steps
}

func makeTestIngredients(n int) []recipe.Ingredient {
	ingredients := make([]recipe.Ingredient, n)
	for i := 0; i < n; i++ {
		ingredients[i] = recipe.Ingredient{
			Name:     "ingredient " + string(rune('A'+i)),
			Quantity: "1",
			Unit:     "cup",
		}
	}
	return ingredients
}
