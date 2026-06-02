package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/auth"
)

// TestChaos069 is a seeded chaos HTTP-probe suite for the spec 069
// assistant HTTP transport surface. It fires N randomized POSTs at
// an in-process httptest server wrapping the real HTTPAdapter and
// asserts the universal invariants:
//
//  1. No panics escape the handler (httptest would surface as a
//     connection error; the test fails on the first such error).
//  2. No 5xx responses leak internals — every body MUST be a
//     v1 envelope (schema_version="v1", transport="web"),
//     ErrorCause is a stable string token, never a raw error.
//  3. Wire envelope is always v1, regardless of input garbage.
//
// Seed is fixed for reproducibility; flip CHAOS069_SEED env if
// needed to reproduce a different run.
func TestChaos069(t *testing.T) {
	const seed = int64(0x6900D5EED069)
	const probes = 150

	rng := rand.New(rand.NewSource(seed))
	t.Logf("chaos-069 seed=%d probes=%d", seed, probes)

	cfg := defaultConfig()
	cfg.SharedUserID = "chaos-user"

	facade := &chaosFacade{rng: rng}
	adapter, err := NewHTTPAdapter(Options{
		Facade:  facade,
		Capture: func(context.Context, string, string, string) {},
		Clock:   func() time.Time { return time.Unix(1735689600, 0).UTC() },
		Config:  cfg,
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter: %v", err)
	}

	// Inject a shared-token session so the adapter resolves the
	// SharedUserID branch instead of 401-ing every probe. Real
	// auth middleware is out of scope for chaos at the adapter
	// surface — we are stressing the wire+facade contract.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithSession(r.Context(), auth.Session{
			Source: auth.SessionSourceSharedToken,
		})
		adapter.ServeHTTP(w, r.WithContext(ctx))
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	var (
		ok2xx, bad4xx, server5xx, envelopeFail int
		statusBuckets                          = map[int]int{}
	)

	for i := 0; i < probes; i++ {
		body, label := generateProbe(rng, i)
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/assistant/turn", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("probe %d (%s): build request: %v", i, label, err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("probe %d (%s) seed=%d body=%q: transport error (possible panic): %v",
				i, label, seed, truncate(body, 200), err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		statusBuckets[resp.StatusCode]++
		switch {
		case resp.StatusCode >= 500:
			server5xx++
		case resp.StatusCode >= 400:
			bad4xx++
		default:
			ok2xx++
		}

		var out TurnResponse
		if err := json.Unmarshal(raw, &out); err != nil {
			envelopeFail++
			t.Errorf("probe %d (%s) status=%d: response is not a TurnResponse envelope: %v\nbody=%q",
				i, label, resp.StatusCode, err, truncate(raw, 200))
			continue
		}
		if out.SchemaVersion != SchemaVersionV1 {
			envelopeFail++
			t.Errorf("probe %d (%s) status=%d: schema_version=%q want %q",
				i, label, resp.StatusCode, out.SchemaVersion, SchemaVersionV1)
		}
		if out.Transport != TransportName {
			envelopeFail++
			t.Errorf("probe %d (%s) status=%d: transport=%q want %q",
				i, label, resp.StatusCode, out.Transport, TransportName)
		}
		// 5xx MUST still carry a stable error code token and MUST NOT
		// leak Go errors or stack-trace fragments via ErrorCause/Body.
		if resp.StatusCode >= 500 {
			if out.ErrorCause == "" {
				t.Errorf("probe %d (%s) status=%d: 5xx without ErrorCause; body=%q",
					i, label, resp.StatusCode, truncate(raw, 200))
			}
			lower := strings.ToLower(out.ErrorCause + " " + out.Body)
			for _, leak := range []string{"goroutine ", "panic:", "runtime error", "/home/", "\n\tat "} {
				if strings.Contains(lower, leak) {
					t.Errorf("probe %d (%s): 5xx body leaks internals (%q): %q",
						i, label, leak, truncate(raw, 200))
				}
			}
		}
	}

	t.Logf("chaos-069 result: 2xx=%d 4xx=%d 5xx=%d envelopeFail=%d statusBuckets=%v facadeCalls=%d facadeErrs=%d",
		ok2xx, bad4xx, server5xx, envelopeFail, statusBuckets, facade.calls, facade.errs)

	if envelopeFail > 0 {
		t.Fatalf("chaos-069: %d responses violated the v1 envelope invariant", envelopeFail)
	}
	// 5xx is allowed (the stub facade randomly returns errors), but
	// every 5xx must be a well-formed envelope — already asserted
	// above. The aggregate is logged for visibility.
}

// chaosFacade returns a randomized AssistantResponse and occasionally
// returns an error so we exercise the adapter's 5xx path. Counts are
// recorded for the chaos summary line.
type chaosFacade struct {
	rng   *rand.Rand
	calls int
	errs  int
}

func (f *chaosFacade) Handle(_ context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	f.calls++
	// 10% of the time, return an error so the adapter exercises the
	// assistant_turn_failed → 500 path with the envelope contract.
	if f.rng.Intn(10) == 0 {
		f.errs++
		return contracts.AssistantResponse{}, fmt.Errorf("chaos-induced facade error for %s", msg.TransportMessageID)
	}
	statuses := []contracts.StatusToken{
		contracts.StatusThinking,
		contracts.StatusSavedAsIdea,
		contracts.StatusUnavailable,
	}
	return contracts.AssistantResponse{
		Status:    statuses[f.rng.Intn(len(statuses))],
		Body:      randomString(f.rng, 1, 80),
		EmittedAt: time.Unix(1735689600, 0).UTC(),
	}, nil
}

// generateProbe builds a request body that ranges from spec-valid
// to deeply malformed. Returns the body and a label for diagnostics.
func generateProbe(rng *rand.Rand, i int) ([]byte, string) {
	bucket := rng.Intn(100)
	switch {
	case bucket < 50:
		return validTextTurn(rng, i), "valid-text"
	case bucket < 65:
		return validConfirmTurn(rng, i), "valid-confirm"
	case bucket < 75:
		return validDisambigTurn(rng, i), "valid-disambig"
	case bucket < 82:
		return validResetTurn(rng, i), "valid-reset"
	case bucket < 88:
		return malformedJSON(rng), "malformed-json"
	case bucket < 94:
		return wrongSchemaVersion(rng, i), "bad-schema"
	case bucket < 97:
		return unknownHint(rng, i), "unknown-hint"
	default:
		return giantBody(rng), "giant-body"
	}
}

func validTextTurn(rng *rand.Rand, i int) []byte {
	hints := []string{"web", "mobile", "bridge", ""}
	req := TurnRequest{
		SchemaVersion:      SchemaVersionV1,
		TransportMessageID: fmt.Sprintf("chaos-%d-%d", i, rng.Int63()),
		Kind:               string(contracts.KindText),
		TransportHint:      hints[rng.Intn(len(hints))],
		Text:               randomString(rng, 1, 200),
	}
	b, _ := json.Marshal(req)
	return b
}

func validConfirmTurn(rng *rand.Rand, i int) []byte {
	choices := []contracts.ConfirmChoice{contracts.ConfirmPositive, contracts.ConfirmNegative}
	req := TurnRequest{
		SchemaVersion:      SchemaVersionV1,
		TransportMessageID: fmt.Sprintf("chaos-%d-%d", i, rng.Int63()),
		Kind:               string(contracts.KindConfirm),
		ConfirmRef:         fmt.Sprintf("cr-%d", rng.Int63()),
		ConfirmChoice:      string(choices[rng.Intn(len(choices))]),
	}
	b, _ := json.Marshal(req)
	return b
}

func validDisambigTurn(rng *rand.Rand, i int) []byte {
	req := TurnRequest{
		SchemaVersion:        SchemaVersionV1,
		TransportMessageID:   fmt.Sprintf("chaos-%d-%d", i, rng.Int63()),
		Kind:                 string(contracts.KindDisambiguation),
		DisambiguationRef:    fmt.Sprintf("dr-%d", rng.Int63()),
		DisambiguationChoice: rng.Intn(5) + 1,
	}
	b, _ := json.Marshal(req)
	return b
}

func validResetTurn(rng *rand.Rand, i int) []byte {
	req := TurnRequest{
		SchemaVersion:      SchemaVersionV1,
		TransportMessageID: fmt.Sprintf("chaos-%d-%d", i, rng.Int63()),
		Kind:               string(contracts.KindReset),
	}
	b, _ := json.Marshal(req)
	return b
}

func malformedJSON(rng *rand.Rand) []byte {
	variants := [][]byte{
		[]byte(""),
		[]byte("{"),
		[]byte("not json at all"),
		[]byte(`{"schema_version": "v1", "kind":`),
		[]byte(`{"schema_version": null, "kind": 42}`),
		[]byte(`[]`),
		[]byte(`null`),
	}
	return variants[rng.Intn(len(variants))]
}

func wrongSchemaVersion(rng *rand.Rand, i int) []byte {
	versions := []string{"v0", "v2", "V1", "", "1", "v1.0"}
	req := TurnRequest{
		SchemaVersion:      versions[rng.Intn(len(versions))],
		TransportMessageID: fmt.Sprintf("chaos-%d-%d", i, rng.Int63()),
		Kind:               string(contracts.KindText),
		Text:               "hi",
	}
	b, _ := json.Marshal(req)
	return b
}

func unknownHint(rng *rand.Rand, i int) []byte {
	hints := []string{"carrier-pigeon", "telegram", "smoke-signal", "🚀", strings.Repeat("x", 256)}
	req := TurnRequest{
		SchemaVersion:      SchemaVersionV1,
		TransportMessageID: fmt.Sprintf("chaos-%d-%d", i, rng.Int63()),
		Kind:               string(contracts.KindText),
		Text:               "hi",
		TransportHint:      hints[rng.Intn(len(hints))],
	}
	b, _ := json.Marshal(req)
	return b
}

func giantBody(rng *rand.Rand) []byte {
	req := TurnRequest{
		SchemaVersion:      SchemaVersionV1,
		TransportMessageID: fmt.Sprintf("chaos-giant-%d", rng.Int63()),
		Kind:               string(contracts.KindText),
		Text:               randomString(rng, 1<<15, 1<<15), // 32 KiB
	}
	b, _ := json.Marshal(req)
	return b
}

const printable = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 .,!?-_/\"'\n\t"

func randomString(rng *rand.Rand, min, max int) string {
	n := min
	if max > min {
		n = min + rng.Intn(max-min+1)
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = printable[rng.Intn(len(printable))]
	}
	return string(b)
}

func truncate(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}
