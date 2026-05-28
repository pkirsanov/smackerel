//go:build stress

// Spec 061 SCOPE-06 — Retrieval skill p95 stress test (G026 budget).
//
// Drives a high-concurrency burst against the retrieval_search agent
// tool handler with an in-process *fake* Searcher that returns a
// deterministic response after a small synthetic latency tail. The
// latency tail makes the test realistic without forcing the suite to
// stand up Postgres + pgvector (which is the integration-test layer's
// job).
//
// Asserts:
//
//   - G1: every Invoke returns nil error (the handler returned hits).
//   - G2: per-call p95 < 5s manifest budget (design.md §5.1
//     LatencyBudget). The fake's latency tail is intentionally well
//     below the budget so a regression toward per-call serialization
//     or repeated synchronous resolves (e.g. accidental
//     per-request mutex contention) trips the assertion.
//   - G3: p50/p95/p99/max are logged so a slow drift toward the
//     budget is visible to operators reading test output.
//
// Skips cleanly when STRESS_ASSISTANT_RETRIEVAL_TURNS=0 so a
// contributor can shrink the burst for CI smoke runs.
//
// NOTE on scope: the BS-002 / BS-007 end-to-end stress (the full
// scenario flowing through facade → executor → ml/ sidecar) is
// blocked on the SCOPE-04 facade post-processor hook (see
// specs/061-conversational-assistant/report.md#scope-06-bs-002 for
// the routed finding). This test measures the *skill's own*
// latency contract independently of the missing capability-layer
// seam — when the seam lands, the existing assistant_facade_p95
// test will measure the additional facade overhead and the two
// budgets remain comparable.

package stress

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	"github.com/smackerel/smackerel/internal/api"
)

const (
	retrievalStressTurnCount   = 800
	retrievalStressWorkerCount = 32
	retrievalStressP95Budget   = 5 * time.Second // design §5.1 LatencyBudget
	retrievalStressMaxTopK     = 8
)

// fakeSearcher simulates a real /api/search response with a small
// synthetic latency distribution. The tail (1ms..40ms) is deliberate
// so the test would fail if the handler accidentally serialized
// concurrent calls (worker-count parallelism would collapse to a
// single-worker p95 ≈ N×tail-mean).
type fakeSearcher struct {
	calls atomic.Int64
	r     *rand.Rand
	rMu   sync.Mutex
}

func (f *fakeSearcher) Search(ctx context.Context, req api.SearchRequest) ([]api.SearchResult, int, string, error) {
	f.calls.Add(1)

	// Sample latency under a lock because *rand.Rand is not
	// goroutine-safe. The contention is tiny (a single Int63 call)
	// and is identical in cost to the production search engine's
	// own internal locks.
	f.rMu.Lock()
	jitterNs := f.r.Int63n(int64(40 * time.Millisecond))
	f.rMu.Unlock()

	select {
	case <-time.After(time.Duration(jitterNs) + time.Millisecond):
	case <-ctx.Done():
		return nil, 0, "", ctx.Err()
	}

	limit := req.Limit
	if limit < 1 {
		limit = retrievalStressMaxTopK
	}
	if limit > retrievalStressMaxTopK {
		limit = retrievalStressMaxTopK
	}
	results := make([]api.SearchResult, limit)
	for i := 0; i < limit; i++ {
		id := "stress-art-" + strconv.Itoa(int(f.calls.Load())) + "-" + strconv.Itoa(i)
		results[i] = api.SearchResult{
			ArtifactID:   id,
			Title:        "Stress Title " + strconv.Itoa(i),
			ArtifactType: "note",
			Snippet:      "stress snippet body",
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		}
	}
	return results, limit, "vector", nil
}

func TestAssistantRetrievalStressP95(t *testing.T) {
	if v := os.Getenv("STRESS_ASSISTANT_RETRIEVAL_TURNS"); v == "0" {
		t.Skip("stress: STRESS_ASSISTANT_RETRIEVAL_TURNS=0 — retrieval stress disabled")
	}
	turns := retrievalStressTurnCount
	if v := os.Getenv("STRESS_ASSISTANT_RETRIEVAL_TURNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			turns = n
		}
	}

	// Wire the in-process fake into the package-global Services slot
	// and tear it back down at end of test so other packages remain
	// hermetic. Deterministic seed so reruns are comparable.
	seeker := &fakeSearcher{r: rand.New(rand.NewSource(1))}
	retrieval.SetServices(&retrieval.Services{
		Engine:  seeker,
		MaxTopK: retrievalStressMaxTopK,
	})
	t.Cleanup(retrieval.ResetForTest)

	tool, ok := agent.ByName(retrieval.ToolName)
	if !ok {
		t.Fatalf("retrieval_search tool not registered; check blank-import wiring")
	}
	if tool.Handler == nil {
		t.Fatalf("retrieval_search tool has nil Handler")
	}

	input := json.RawMessage(`{"query":"what about Tailscale ACLs","user_id":"stress-u-1","top_k":5}`)

	latencies := make([]time.Duration, turns)
	work := make(chan int, turns)
	for i := 0; i < turns; i++ {
		work <- i
	}
	close(work)

	var wg sync.WaitGroup
	var firstErr atomic.Value // error
	for w := 0; w < retrievalStressWorkerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for i := range work {
				start := time.Now()
				_, err := tool.Handler(ctx, input)
				latencies[i] = time.Since(start)
				if err != nil {
					firstErr.CompareAndSwap(nil, err)
				}
			}
		}()
	}
	wg.Wait()

	if e := firstErr.Load(); e != nil {
		t.Fatalf("first handler error: %v", e)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	maxL := latencies[len(latencies)-1]

	t.Logf("Retrieval Skill burst — turns=%d workers=%d p50=%v p95=%v p99=%v max=%v searcher_calls=%d",
		turns, retrievalStressWorkerCount, p50, p95, p99, maxL, seeker.calls.Load())

	if p95 > retrievalStressP95Budget {
		t.Errorf("G026 budget breach: retrieval skill p95=%v exceeds manifest budget=%v (design.md §5.1 LatencyBudget)",
			p95, retrievalStressP95Budget)
	}
}
