package weather

import (
	"testing"
	"time"
)

func TestCache_HitMissBasics(t *testing.T) {
	c := NewCache(time.Minute, 4)
	if _, ok := c.Get("p", "Seattle", WindowNow); ok {
		t.Fatal("empty cache returned hit")
	}
	c.Put("p", "Seattle", WindowNow, Forecast{ForecastLine: "x", ProviderName: "p", RetrievedAt: time.Unix(100, 0).UTC()})
	got, ok := c.Get("p", "Seattle", WindowNow)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !got.RetrievedAt.Equal(time.Unix(100, 0).UTC()) {
		t.Errorf("retrieved_at mutated by cache: got %s", got.RetrievedAt)
	}
}

func TestCache_KeyByProviderLocationWindow(t *testing.T) {
	c := NewCache(time.Minute, 4)
	c.Put("p1", "Seattle", WindowNow, Forecast{ForecastLine: "a"})
	c.Put("p2", "Seattle", WindowNow, Forecast{ForecastLine: "b"})
	c.Put("p1", "Seattle", WindowToday, Forecast{ForecastLine: "c"})
	c.Put("p1", "Portland", WindowNow, Forecast{ForecastLine: "d"})

	cases := []struct {
		provider, location string
		window             ForecastWindow
		wantLine           string
	}{
		{"p1", "Seattle", WindowNow, "a"},
		{"p2", "Seattle", WindowNow, "b"},
		{"p1", "Seattle", WindowToday, "c"},
		{"p1", "Portland", WindowNow, "d"},
	}
	for _, tc := range cases {
		got, ok := c.Get(tc.provider, tc.location, tc.window)
		if !ok {
			t.Errorf("Get(%s,%s,%s) miss", tc.provider, tc.location, tc.window)
			continue
		}
		if got.ForecastLine != tc.wantLine {
			t.Errorf("Get(%s,%s,%s) got %q want %q", tc.provider, tc.location, tc.window, got.ForecastLine, tc.wantLine)
		}
	}
}

func TestCache_TTLExpiresEntry(t *testing.T) {
	c := NewCache(10*time.Millisecond, 4)
	now := time.Unix(1000, 0)
	c = c.withClock(func() time.Time { return now })
	c.Put("p", "Seattle", WindowNow, Forecast{ForecastLine: "x"})
	if _, ok := c.Get("p", "Seattle", WindowNow); !ok {
		t.Fatal("immediate get should hit")
	}
	now = now.Add(20 * time.Millisecond)
	if _, ok := c.Get("p", "Seattle", WindowNow); ok {
		t.Fatal("after TTL elapsed Get should miss")
	}
	if c.Len() != 0 {
		t.Errorf("expired entry not evicted, Len=%d", c.Len())
	}
}

func TestCache_CapacityEvictsOldest(t *testing.T) {
	c := NewCache(time.Hour, 2)
	c.Put("p", "A", WindowNow, Forecast{ForecastLine: "1"})
	c.Put("p", "B", WindowNow, Forecast{ForecastLine: "2"})
	c.Put("p", "C", WindowNow, Forecast{ForecastLine: "3"})
	if c.Len() != 2 {
		t.Fatalf("cap exceeded: Len=%d, want 2", c.Len())
	}
	if _, ok := c.Get("p", "A", WindowNow); ok {
		t.Error("expected oldest entry A to be evicted")
	}
	if _, ok := c.Get("p", "C", WindowNow); !ok {
		t.Error("newest entry C missing")
	}
}

func TestCache_LocationCanonicalization(t *testing.T) {
	c := NewCache(time.Hour, 4)
	c.Put("p", "Seattle", WindowNow, Forecast{ForecastLine: "x"})
	// Case + whitespace variants should map to the same entry so the
	// LLM cannot accidentally bypass the cache by reflowing the
	// location string.
	if _, ok := c.Get("p", "  seattle ", WindowNow); !ok {
		t.Error("expected case/whitespace insensitive cache key match")
	}
	if _, ok := c.Get("p", "SEATTLE", WindowNow); !ok {
		t.Error("expected case-insensitive cache key match")
	}
}

func TestCache_PutReplacesAndKeepsCap(t *testing.T) {
	c := NewCache(time.Hour, 2)
	c.Put("p", "A", WindowNow, Forecast{ForecastLine: "1"})
	c.Put("p", "A", WindowNow, Forecast{ForecastLine: "2"})
	if c.Len() != 1 {
		t.Errorf("replace produced %d entries, want 1", c.Len())
	}
	got, _ := c.Get("p", "A", WindowNow)
	if got.ForecastLine != "2" {
		t.Errorf("expected replaced value, got %q", got.ForecastLine)
	}
}
