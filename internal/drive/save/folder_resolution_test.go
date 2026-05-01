package save

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/smackerel/smackerel/internal/drive/rules"
)

// memFolderStore is an in-process FolderStore that emulates the
// drive_folder_resolutions UNIQUE constraint so the unit test can prove
// FolderResolver.Resolve produces exactly one winning mapping under
// concurrent callers.
type memFolderStore struct {
	mu      sync.Mutex
	mapping map[string]string // (connection|path) -> provider_folder_id
	inserts int32             // total INSERT attempts (winning + losing)
}

func newMemFolderStore() *memFolderStore {
	return &memFolderStore{mapping: make(map[string]string)}
}

func (s *memFolderStore) Lookup(_ context.Context, connectionID, folderPath string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.mapping[connectionID+"|"+folderPath]
	if !ok {
		return "", nil
	}
	return value, nil
}

func (s *memFolderStore) TryInsert(_ context.Context, connectionID, _ string, folderPath, providerFolderID, _ string) (string, error) {
	atomic.AddInt32(&s.inserts, 1)
	s.mu.Lock()
	defer s.mu.Unlock()
	key := connectionID + "|" + folderPath
	if existing, ok := s.mapping[key]; ok {
		return existing, nil
	}
	s.mapping[key] = providerFolderID
	return providerFolderID, nil
}

func (s *memFolderStore) inserted() int32 {
	return atomic.LoadInt32(&s.inserts)
}

// countingEnsurer mints unique provider folder ids for every concurrent
// caller. The unit test asserts that even when N concurrent goroutines
// receive N distinct provider ids, exactly one survives in the resolution
// store and the FolderResolver returns it for every caller.
type countingEnsurer struct {
	mu       sync.Mutex
	counter  int32
	calls    int32
	failNext bool
}

func (c *countingEnsurer) EnsureFolder(_ context.Context, _ string, folderPath string) (string, error) {
	atomic.AddInt32(&c.calls, 1)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failNext {
		c.failNext = false
		return "", errors.New("fixture: ensure folder failed")
	}
	c.counter++
	return "fixture-folder-" + folderPath + "-" + itoa(int(c.counter)), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n = n / 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// TestConcurrentFolderResolutionCreatesOneMapping is the SCN-038-015 unit
// anchor. It asserts that when many goroutines concurrently resolve the
// same missing folder path, the FolderResolver returns one stable
// provider_folder_id to every caller and the underlying store records
// exactly one mapping. Adversarial sub-cases prove the resolver:
//   - DOES NOT return distinct provider ids per caller (would be the
//     "every save creates its own folder" regression),
//   - DOES NOT silently swallow EnsureFolder failures and return an
//     empty id (would be the "happy-path-only" regression).
func TestConcurrentFolderResolutionCreatesOneMapping(t *testing.T) {
	t.Run("32 concurrent callers collapse to one mapping", func(t *testing.T) {
		store := newMemFolderStore()
		ensurer := &countingEnsurer{}
		resolver := NewFolderResolver(store, ensurer)
		const callers = 32
		results := make([]string, callers)
		errs := make([]error, callers)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < callers; i = i + 1 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-start
				id, err := resolver.Resolve(context.Background(), "conn-1", "google", "Receipts/2026", "req-"+itoa(idx), rules.OnMissingCreate)
				results[idx] = id
				errs[idx] = err
			}(i)
		}
		close(start)
		wg.Wait()

		first := results[0]
		if first == "" {
			t.Fatalf("Resolve returned empty id (errs[0]=%v)", errs[0])
		}
		for i, got := range results {
			if errs[i] != nil {
				t.Fatalf("caller[%d] err = %v", i, errs[i])
			}
			if got != first {
				t.Fatalf("caller[%d] got %q, want stable %q (mapping not collapsed)", i, got, first)
			}
		}
		mapped, _ := store.Lookup(context.Background(), "conn-1", "Receipts/2026")
		if mapped != first {
			t.Fatalf("store mapping = %q, want %q", mapped, first)
		}
		if store.inserted() < 1 {
			t.Fatalf("store inserts = %d, want at least 1", store.inserted())
		}
		if ensurer.calls < 1 {
			t.Fatalf("EnsureFolder calls = %d, want at least 1", ensurer.calls)
		}
	})

	t.Run("ensure failure surfaces error (adversarial)", func(t *testing.T) {
		store := newMemFolderStore()
		ensurer := &countingEnsurer{failNext: true}
		resolver := NewFolderResolver(store, ensurer)
		id, err := resolver.Resolve(context.Background(), "conn-1", "google", "Receipts/2026", "req-x", rules.OnMissingCreate)
		if err == nil || id != "" {
			t.Fatalf("Resolve(failure) = %q, %v ; want empty id + non-nil error", id, err)
		}
	})

	t.Run("missing ensurer + on_missing_fail surfaces explicit error", func(t *testing.T) {
		store := newMemFolderStore()
		resolver := NewFolderResolver(store, nil)
		id, err := resolver.Resolve(context.Background(), "conn-1", "google", "Receipts/2026", "req-x", rules.OnMissingFail)
		if err == nil || id != "" {
			t.Fatalf("Resolve(no ensurer, fail policy) = %q, %v ; want empty id + on_missing_folder=fail error", id, err)
		}
	})
}
