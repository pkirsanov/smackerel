// BUG-069-006 — confirm redemption must be single-flight under concurrency.
//
// SCN-BUG069006-001 (TP-BUG069006-01) and SCN-BUG069006-003 (TP-BUG069006-02).
//
// These tests hold the read-check-write window open deliberately instead of
// hoping the scheduler interleaves. design.md requires this: a concurrency test
// that has never been observed red proves nothing, because an unreliable
// interleaving produces a green run on broken code. The barrier makes the RED
// deterministic, so a later GREEN is attributable to the fix rather than to
// scheduling luck.

package confirm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
)

// loadBarrierStore detains every caller inside Load until `parties` callers
// have finished reading. That is precisely the interval between machine.go's
// Load and its clear, so both racers enter redemption believing they own the
// reference.
type loadBarrierStore struct {
	assistantctx.Store // embedded: forwards every method this decorator does not override

	parties int
	mu      sync.Mutex
	arrived int
	release chan struct{}
}

func newLoadBarrierStore(inner assistantctx.Store, parties int) *loadBarrierStore {
	return &loadBarrierStore{Store: inner, parties: parties, release: make(chan struct{})}
}

func (s *loadBarrierStore) Load(ctx context.Context, userID, transport string) (assistantctx.Conversation, bool, error) {
	conv, ok, err := s.Store.Load(ctx, userID, transport)
	s.mu.Lock()
	s.arrived++
	if s.arrived == s.parties {
		close(s.release)
	}
	s.mu.Unlock()
	<-s.release
	return conv, ok, err
}

// racingFixture seeds a live pending confirm through an unbarriered Machine,
// then returns a second Machine whose Load is barriered. Seeding separately
// keeps Propose's own Load from consuming a barrier slot.
func racingFixture(t *testing.T, parties int, expiresAt time.Time) (*Machine, *memWriter, ProposalInput) {
	t.Helper()
	store := newMemStore()
	writer := &memWriter{}
	in := freshProposal()
	in.ExpiresAt = expiresAt
	if err := NewMachine(store, writer).Propose(context.Background(), in, time.Date(2026, 1, 1, 17, 40, 0, 0, time.UTC)); err != nil {
		t.Fatalf("seed Propose: unexpected error: %v", err)
	}
	return NewMachine(newLoadBarrierStore(store, parties), writer), writer, in
}

func countOutcomes(rows []ProposalArtifact, ref string) map[Outcome]int {
	counts := map[Outcome]int{}
	for _, r := range rows {
		if r.ConfirmRef == ref {
			counts[r.Outcome]++
		}
	}
	return counts
}

// TestMachineConfirm_ConcurrentRedemptionExecutesOnce covers SCN-BUG069006-001.
func TestMachineConfirm_ConcurrentRedemptionExecutesOnce(t *testing.T) {
	m, writer, in := racingFixture(t, 2, time.Date(2026, 1, 1, 17, 50, 0, 0, time.UTC))
	now := time.Date(2026, 1, 1, 17, 45, 0, 0, time.UTC)

	const racers = 2
	errs := make([]error, racers)
	results := make([]ConfirmResult, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = m.Confirm(context.Background(), ConfirmInput{
				UserID:     in.UserID,
				Transport:  in.Transport,
				ConfirmRef: in.ConfirmRef,
			}, now)
		}(i)
	}
	wg.Wait()

	var won, lost int
	for i, err := range errs {
		switch {
		case err == nil:
			won++
			if len(results[i].Payload) == 0 {
				t.Errorf("racer %d won but returned an empty payload", i)
			}
		case errors.Is(err, ErrPendingNotFound):
			lost++
		default:
			t.Errorf("racer %d: unexpected error: %v", i, err)
		}
	}
	if won != 1 {
		t.Errorf("winners: got %d want 1 — the gated action would execute %d times", won, won)
	}
	if lost != racers-1 {
		t.Errorf("losers receiving ErrPendingNotFound: got %d want %d", lost, racers-1)
	}

	counts := countOutcomes(writer.snapshot(), in.ConfirmRef)
	if counts[OutcomeConfirmed] != 1 {
		t.Errorf("confirmed audit rows for %q: got %d want 1", in.ConfirmRef, counts[OutcomeConfirmed])
	}
}

// TestMachineConfirm_RacingSweepProducesOneTerminalOutcome covers SCN-BUG069006-003.
func TestMachineConfirm_RacingSweepProducesOneTerminalOutcome(t *testing.T) {
	expired := time.Date(2026, 1, 1, 17, 50, 0, 0, time.UTC)
	m, writer, in := racingFixture(t, 2, expired)
	now := expired.Add(time.Minute)

	var wg sync.WaitGroup
	var confirmErr, sweepErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, confirmErr = m.Confirm(context.Background(), ConfirmInput{
			UserID:     in.UserID,
			Transport:  in.Transport,
			ConfirmRef: in.ConfirmRef,
		}, now)
	}()
	go func() {
		defer wg.Done()
		_, sweepErr = m.SweepTimeouts(context.Background(), []ExpiredPending{{
			UserID:     in.UserID,
			Transport:  in.Transport,
			ConfirmRef: in.ConfirmRef,
		}}, now)
	}()
	wg.Wait()

	if confirmErr != nil && !errors.Is(confirmErr, ErrPendingNotFound) {
		t.Errorf("confirm: unexpected error: %v", confirmErr)
	}
	if sweepErr != nil {
		t.Errorf("sweep: unexpected error: %v", sweepErr)
	}

	counts := countOutcomes(writer.snapshot(), in.ConfirmRef)
	terminal := counts[OutcomeConfirmed] + counts[OutcomeDiscardedTimeout]
	if terminal != 1 {
		t.Errorf("terminal audit rows for %q: got %d (confirmed=%d discarded_timeout=%d) want exactly 1",
			in.ConfirmRef, terminal, counts[OutcomeConfirmed], counts[OutcomeDiscardedTimeout])
	}
}
