package telegram

import (
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/recipe"
)

// CookSession holds the state of an active cook-mode walkthrough.
type CookSession struct {
	RecipeArtifactID string
	RecipeTitle      string
	Steps            []recipe.Step
	Ingredients      []recipe.Ingredient
	CurrentStep      int // 1-based index
	TotalSteps       int
	ScaleFactor      float64
	OriginalServings int
	ScaledServings   int
	CreatedAt        time.Time
	LastInteraction  time.Time

	// Pending replacement state (Scope 06)
	PendingReplacement string // artifact ID of pending new recipe
	PendingRecipeData  *recipe.RecipeData
	PendingServings    int
	PendingRecipeName  string
}

// CookSessionStore manages per-chat cook sessions with configurable TTL.
type CookSessionStore struct {
	sessions sync.Map      // key: int64 (chatID), value: *CookSession
	timeout  time.Duration // from config: telegram.cook_session_timeout_minutes
	done     chan struct{}  // signals cleanup goroutine to stop
}

// NewCookSessionStore creates a new session store with the given timeout.
func NewCookSessionStore(timeoutMinutes int) *CookSessionStore {
	s := &CookSessionStore{
		timeout: time.Duration(timeoutMinutes) * time.Minute,
		done:    make(chan struct{}),
	}
	return s
}

// StartCleanup begins the background sweep goroutine.
func (s *CookSessionStore) StartCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				s.sweep()
			}
		}
	}()
}

// Stop signals the cleanup goroutine to exit.
func (s *CookSessionStore) Stop() {
	select {
	case <-s.done:
		// Already closed
	default:
		close(s.done)
	}
}

// Create creates or replaces a cook session for the given chat.
func (s *CookSessionStore) Create(chatID int64, session *CookSession) {
	now := time.Now()
	session.CreatedAt = now
	session.LastInteraction = now
	s.sessions.Store(chatID, session)
}

// Get retrieves the cook session for the given chat, or nil if none exists.
func (s *CookSessionStore) Get(chatID int64) *CookSession {
	val, ok := s.sessions.Load(chatID)
	if !ok {
		return nil
	}
	return val.(*CookSession)
}

// Delete removes the cook session for the given chat.
func (s *CookSessionStore) Delete(chatID int64) {
	s.sessions.Delete(chatID)
}

// Touch updates the LastInteraction timestamp for the session.
func (s *CookSessionStore) Touch(chatID int64) {
	val, ok := s.sessions.Load(chatID)
	if !ok {
		return
	}
	session := val.(*CookSession)
	session.LastInteraction = time.Now()
}

// sweep removes expired sessions.
func (s *CookSessionStore) sweep() {
	now := time.Now()
	s.sessions.Range(func(key, value any) bool {
		session := value.(*CookSession)
		if now.Sub(session.LastInteraction) > s.timeout {
			s.sessions.Delete(key)
		}
		return true
	})
}
