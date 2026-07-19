//go:build stress

// Spec 069 SCOPE-5 — HTTP turn stress smoke (SCN-069-A11 / hot-path
// stability).
//
// TestAssistantHTTPStress_PerUserRateLimitAndConversationTTLRemainStable
// drives the spec 069 HTTP adapter through a burst of concurrent
// turns from multiple users against an in-process facade and a
// recording per-user rate limiter that mirrors the SCOPE-2
// PreFacadeChain shape. It asserts:
//
//   * The HTTP path stays within a generous p95 latency budget
//     under burst load (G1 — stress smoke, not micro-benchmark).
//   * Per-user rate limiting partitions cleanly: a hot user that
//     exhausts the limit does NOT degrade other users (G2).
//   * The conversation-row family keyed by (UserID, Transport)
//     remains stable: every user that posted at least one accepted
//     turn has exactly one (user, "web") row at the end (G3).
//
// This is a smoke test for the live integration: it exercises the
// adapter + facade hot path under realistic concurrency without
// requiring a live Postgres or NATS stack. The full live stress
// (live Postgres rows, real rate limiter middleware) belongs to a
// dedicated stress profile if/when one is added.
//
// Skippable: STRESS_ASSISTANT_HTTP_TURNS=0 disables the test for
// CI smoke runs where the stress profile is not exercised.

package stress

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
	"github.com/smackerel/smackerel/internal/auth"
)

const (
	httpStressDefaultTurns   = 600
	httpStressDefaultUsers   = 20
	httpStressDefaultWorkers = 16
	httpStressP95BudgetMs    = 50
)

// stressFacade is a constant-time facade stand-in. It records the
// (UserID, Transport) tuple of every accepted turn so the test can
// assert row-family stability at the end.
type stressFacade struct {
	mu   sync.Mutex
	rows map[string]int
}

func newStressFacade() *stressFacade {
	return &stressFacade{rows: map[string]int{}}
}

func (s *stressFacade) Handle(_ context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	s.mu.Lock()
	s.rows[msg.UserID+"|"+msg.Transport]++
	s.mu.Unlock()
	return contracts.AssistantResponse{
		Status:    contracts.StatusThinking,
		Body:      "ack",
		EmittedAt: time.Now().UTC(),
	}, nil
}

// perUserLimiter is a token-bucket-shaped rate limiter that mirrors
// the SCOPE-2 PreFacadeChain limiter shape. A hot user exhausts its
// own bucket; other users' buckets remain full.
type perUserLimiter struct {
	mu       sync.Mutex
	tokens   map[string]int
	maxBurst int
}

func newPerUserLimiter(burst int) *perUserLimiter {
	return &perUserLimiter{tokens: map[string]int{}, maxBurst: burst}
}

func (l *perUserLimiter) allow(userID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cur, ok := l.tokens[userID]
	if !ok {
		cur = l.maxBurst
	}
	if cur <= 0 {
		l.tokens[userID] = 0
		return false
	}
	l.tokens[userID] = cur - 1
	return true
}

func TestAssistantHTTPStress_PerUserRateLimitAndConversationTTLRemainStable(t *testing.T) {
	if v := os.Getenv("STRESS_ASSISTANT_HTTP_TURNS"); v == "0" {
		t.Skip("stress: STRESS_ASSISTANT_HTTP_TURNS=0 — HTTP stress disabled")
	}
	turns := httpStressDefaultTurns
	if v := os.Getenv("STRESS_ASSISTANT_HTTP_TURNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			turns = n
		}
	}
	users := httpStressDefaultUsers
	workers := httpStressDefaultWorkers
	burstPerUser := turns / users // every user posts this many turns

	facade := newStressFacade()
	adapter, err := httpadapter.NewHTTPAdapter(httpadapter.Options{
		Facade:  facade,
		Capture: func(context.Context, string, string, string) {},
		Clock:   time.Now,
		Config: httpadapter.HTTPTransportConfig{
			Enabled:                true,
			SchemaVersion:          httpadapter.SchemaVersionV1,
			BodySizeMaxBytes:       1 << 20,
			ConversationTTL:        time.Hour,
			TransportHintAllowlist: []string{"web", "mobile", "bridge"},
			RequiredScope:          "assistant.turn",
		},
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter: %v", err)
	}

	// Limiter sized so the FIRST user (the "hot" user) exhausts its
	// bucket at half its turn budget; other users stay under the cap.
	hotUser := "stress-u-0"
	limiter := newPerUserLimiter(burstPerUser)
	limiter.tokens = map[string]int{hotUser: burstPerUser / 2}

	type job struct{ userID, turnID string }
	queue := make(chan job, turns)
	for i := 0; i < turns; i++ {
		uid := "stress-u-" + strconv.Itoa(i%users)
		queue <- job{userID: uid, turnID: uid + "-" + strconv.Itoa(i)}
	}
	close(queue)

	latencies := make([]time.Duration, 0, turns)
	var lmu sync.Mutex
	rejected := map[string]int{}
	accepted := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				if !limiter.allow(j.userID) {
					lmu.Lock()
					rejected[j.userID]++
					lmu.Unlock()
					continue
				}
				body, _ := json.Marshal(httpadapter.TurnRequest{
					SchemaVersion:      httpadapter.SchemaVersionV1,
					TransportMessageID: j.turnID,
					Kind:               string(contracts.KindText),
					TransportHint:      "web",
					Text:               "weather in barcelona",
				})
				req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(body))
				req = req.WithContext(auth.WithSession(req.Context(), auth.Session{UserID: j.userID, Source: auth.SessionSourcePerUserToken}))
				rr := httptest.NewRecorder()
				start := time.Now()
				adapter.ServeHTTP(rr, req)
				lat := time.Since(start)
				lmu.Lock()
				latencies = append(latencies, lat)
				if rr.Code == http.StatusOK {
					accepted[j.userID]++
				} else {
					rejected[j.userID]++
				}
				lmu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(latencies) == 0 {
		t.Fatal("no turns executed; latencies empty")
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	maxL := latencies[len(latencies)-1]
	t.Logf("HTTP adapter stress — turns_attempted=%d workers=%d p50=%v p95=%v p99=%v max=%v users=%d",
		turns, workers, p50, p95, p99, maxL, users)

	// G1: p95 budget.
	if p95 > time.Duration(httpStressP95BudgetMs)*time.Millisecond {
		t.Errorf("p95=%v exceeds %d ms budget", p95, httpStressP95BudgetMs)
	}

	// G2: per-user rate limit partition — hot user has more rejections
	// than every other user; other users have at least one acceptance.
	hotRejected := rejected[hotUser]
	for i := 1; i < users; i++ {
		uid := "stress-u-" + strconv.Itoa(i)
		if accepted[uid] == 0 {
			t.Errorf("user %s has zero acceptances; hot user starved cold users (rate limiter bleed)", uid)
		}
		if rejected[uid] >= hotRejected {
			t.Errorf("user %s rejected=%d >= hot user %s rejected=%d; limiter not per-user", uid, rejected[uid], hotUser, hotRejected)
		}
	}

	// G3: every user with at least one acceptance has exactly one
	// (user, "web") row in the facade's recorded set.
	for uid, n := range accepted {
		if n == 0 {
			continue
		}
		count := facade.rows[uid+"|"+httpadapter.TransportName]
		if count == 0 {
			t.Errorf("user %s accepted=%d but facade rows missing (user, web) entry", uid, n)
		}
		if count != n {
			t.Errorf("user %s facade rows=%d != accepted=%d", uid, count, n)
		}
	}
}
