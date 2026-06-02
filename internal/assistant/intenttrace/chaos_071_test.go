// Spec 071 — bubbles.chaos pass.
//
// Stochastic fuzz against the redaction surface and the persisted-row
// schema-validation path used by StoreReplay. Goals:
//   - DefaultRedactor.Redact never panics, never returns raw text when
//     PersistRawText=false, and redacted_count matches the actual
//     overlap between policy SensitiveSlotClasses and slot keys.
//   - StoreReplay.Run returns a typed error (ErrTraceNotFound,
//     ErrTraceSampledOut, ErrTraceSchemaInvalid) or a valid
//     ReplayComparison for every random IntentTraceRow; never panics.

package intenttrace

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"
)

type chaosRowStore struct{ row IntentTraceRow }

func (s *chaosRowStore) Put(_ context.Context, _ IntentTraceRow) error { return nil }
func (s *chaosRowStore) Get(_ context.Context, id string) (IntentTraceRow, error) {
	if id == "" || id != s.row.TraceID {
		return IntentTraceRow{}, errors.New("not found")
	}
	return s.row, nil
}
func (s *chaosRowStore) SweepExpired(_ context.Context, _ time.Time) (SweepResult, error) {
	return SweepResult{}, nil
}

// TestChaos071_Redactor_NeverLeaksRawAndCountIsHonest fires 500
// random (policy, rawText, slots) triples through DefaultRedactor.
// Invariants:
//   - no panic
//   - RawText is one of {"absent","present"}
//   - When PersistRawText=false → RawText == "absent" regardless of input
//   - RedactedCount equals the size of policy.SensitiveSlotClasses ∩ keys(slots)
//   - every slot key appears in Summary.SlotClasses exactly once with
//     value "redacted" or "safe"
func TestChaos071_Redactor_NeverLeaksRawAndCountIsHonest(t *testing.T) {
	seed := time.Now().UnixNano()
	t.Logf("chaos-071 redactor seed=%d", seed)
	rng := rand.New(rand.NewSource(seed))
	r := NewDefaultRedactor()

	const N = 500
	for i := 0; i < N; i++ {
		persistRaw := rng.Intn(2) == 0
		sensitive := randomKeys(rng, rng.Intn(8))
		policy := NewSourcePolicy(persistRaw, sensitive)
		slots := randomSlotMap(rng, rng.Intn(10))
		rawText := randomChaosText(rng)

		var res RedactionResult
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("chaos-071 redactor panic at i=%d seed=%d: %v", i, seed, p)
				}
			}()
			res = r.Redact(policy, rawText, slots)
		}()

		if res.RawText != "absent" && res.RawText != "present" {
			t.Fatalf("chaos-071 raw disposition out-of-vocab %q at i=%d seed=%d", res.RawText, i, seed)
		}
		if !persistRaw && res.RawText != "absent" {
			t.Fatalf("chaos-071 leak: PersistRawText=false but RawText=%q at i=%d seed=%d", res.RawText, i, seed)
		}
		if len(res.Summary.SlotClasses) != len(slots) {
			t.Fatalf("chaos-071 SlotClasses size=%d != slots size=%d at i=%d seed=%d",
				len(res.Summary.SlotClasses), len(slots), i, seed)
		}
		expectedRedacted := 0
		for k := range slots {
			class, ok := res.Summary.SlotClasses[k]
			if !ok {
				t.Fatalf("chaos-071 slot key %q missing from SlotClasses at i=%d seed=%d", k, i, seed)
			}
			_, sensitiveKey := policy.SensitiveSlotClasses[k]
			if sensitiveKey {
				expectedRedacted++
				if class != "redacted" {
					t.Fatalf("chaos-071 sensitive key %q got class %q at i=%d seed=%d", k, class, i, seed)
				}
			} else if class != "safe" {
				t.Fatalf("chaos-071 non-sensitive key %q got class %q at i=%d seed=%d", k, class, i, seed)
			}
		}
		if res.Summary.RedactedCount != expectedRedacted {
			t.Fatalf("chaos-071 RedactedCount=%d, expected=%d at i=%d seed=%d",
				res.Summary.RedactedCount, expectedRedacted, i, seed)
		}
	}
}

// TestChaos071_StoreReplay_NeverPanicsOnRandomRows constructs 300
// random IntentTraceRows and runs them through StoreReplay. Replay
// must either return a typed error or a valid ReplayComparison with
// SideEffectsInvoked == false. No panics.
func TestChaos071_StoreReplay_NeverPanicsOnRandomRows(t *testing.T) {
	seed := time.Now().UnixNano()
	t.Logf("chaos-071 replay seed=%d", seed)
	rng := rand.New(rand.NewSource(seed))

	const N = 300
	for i := 0; i < N; i++ {
		row := randomRow(rng)
		store := &chaosRowStore{row: row}
		replay := NewStoreReplay(store)

		// Sometimes ask for a totally different trace id to exercise
		// the not-found path.
		askID := row.TraceID
		if rng.Intn(4) == 0 {
			askID = randomChaosString(rng, rng.Intn(40))
		}

		var (
			cmp ReplayComparison
			err error
		)
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("chaos-071 replay panic at i=%d seed=%d: %v", i, seed, p)
				}
			}()
			cmp, err = replay.Run(context.Background(), askID)
		}()

		if err != nil {
			// Must be one of the typed errors.
			if !(errors.Is(err, ErrTraceNotFound) ||
				errors.Is(err, ErrTraceSampledOut) ||
				errors.Is(err, ErrTraceSchemaInvalid)) {
				t.Fatalf("chaos-071 replay untyped error at i=%d seed=%d: %v", i, seed, err)
			}
			continue
		}
		if cmp.SideEffectsInvoked {
			t.Fatalf("chaos-071 replay SideEffectsInvoked=true at i=%d seed=%d", i, seed)
		}
		if !cmp.ReadOnly {
			t.Fatalf("chaos-071 replay ReadOnly=false at i=%d seed=%d", i, seed)
		}
	}
}

// ---- helpers ----

func randomKeys(rng *rand.Rand, n int) []string {
	out := make([]string, 0, n)
	pool := []string{"phone", "email", "address", "name", "ssn", "dob", "city", "note"}
	for i := 0; i < n; i++ {
		out = append(out, pool[rng.Intn(len(pool))])
	}
	return out
}

func randomSlotMap(rng *rand.Rand, n int) map[string]any {
	out := make(map[string]any, n)
	pool := []string{"phone", "email", "city", "topic", "amount", "color", "note", "extra"}
	for i := 0; i < n; i++ {
		out[pool[rng.Intn(len(pool))]] = rng.Int()
	}
	return out
}

func randomChaosText(rng *rand.Rand) string {
	if rng.Intn(3) == 0 {
		return ""
	}
	return randomChaosString(rng, rng.Intn(256))
}

func randomChaosString(rng *rand.Rand, n int) string {
	if n <= 0 {
		return ""
	}
	const alphabet = "abcdef0123456789-_/\"\\{} \n\t\xc3\xa9\xe4\xb8\xad"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(b)
}

func randomTransport(rng *rand.Rand) Transport {
	if rng.Intn(5) == 0 {
		return Transport(randomChaosString(rng, 6))
	}
	return AllTransports[rng.Intn(len(AllTransports))]
}

func randomStatus(rng *rand.Rand) FinalResponseStatus {
	if rng.Intn(5) == 0 {
		return FinalResponseStatus(randomChaosString(rng, 6))
	}
	return AllFinalResponseStatuses[rng.Intn(len(AllFinalResponseStatuses))]
}

func randomRow(rng *rand.Rand) IntentTraceRow {
	schema := SchemaVersionV1
	if rng.Intn(8) == 0 {
		schema = randomChaosString(rng, 4)
	}
	traceID := randomChaosString(rng, 16)
	if rng.Intn(10) == 0 {
		traceID = ""
	}
	turnID := randomChaosString(rng, 16)
	if rng.Intn(10) == 0 {
		turnID = ""
	}
	sampled := rng.Intn(2) == 0
	row := IntentTraceRow{
		TraceID:             traceID,
		SchemaVersion:       schema,
		TurnID:              turnID,
		UserIDHash:          randomChaosString(rng, 32),
		Transport:           randomTransport(rng),
		TransportMessageID:  randomChaosString(rng, 16),
		Sampled:             sampled,
		ActionClass:         randomChaosString(rng, 8),
		SideEffectClass:     randomChaosString(rng, 8),
		RouteDecision:       randomChaosString(rng, 8),
		FinalResponseStatus: randomStatus(rng),
		EmittedAt:           time.Now(),
		ExpiresAt:           time.Now().Add(time.Hour),
		RedactedPayload: RedactedPayload{
			SchemaVersion:       schema,
			TraceID:             traceID,
			TurnID:              turnID,
			Transport:           randomTransport(rng),
			Sampled:             sampled,
			ActionClass:         randomChaosString(rng, 8),
			SideEffectClass:     randomChaosString(rng, 8),
			FinalResponseStatus: randomStatus(rng),
			ToolCalls:           []ToolCallSummary{},
			SlotsRedactionSummary: SlotsRedactionSummary{
				RawText:     "absent",
				SlotClasses: map[string]string{},
			},
		},
		SlotsRedactionSummary: SlotsRedactionSummary{
			RawText:     "absent",
			SlotClasses: map[string]string{},
		},
	}
	return row
}
