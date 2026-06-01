// Spec 071 SCOPE-02 — Sampler unit tests (SCN-071-A02).

package intenttrace

import "testing"

func TestRatioSampler_Validation(t *testing.T) {
	for _, bad := range []float64{-0.1, 1.1, 2.0, -1.0} {
		if _, err := NewRatioSampler(bad); err == nil {
			t.Fatalf("expected validation error for ratio %v", bad)
		}
	}
}

func TestRatioSampler_FullRatioAlwaysSamples(t *testing.T) {
	s, err := NewRatioSampler(1.0)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c", "trace-1", ""} {
		if !s.ShouldSample(id) {
			t.Fatalf("ratio=1.0 must sample %q", id)
		}
	}
}

func TestRatioSampler_ZeroRatioNeverSamples(t *testing.T) {
	s, err := NewRatioSampler(0.0)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "trace-1", "trace-2"} {
		if s.ShouldSample(id) {
			t.Fatalf("ratio=0.0 must not sample %q", id)
		}
	}
}

func TestRatioSampler_DeterministicForSameID(t *testing.T) {
	s, err := NewRatioSampler(0.5)
	if err != nil {
		t.Fatal(err)
	}
	first := s.ShouldSample("trace-deterministic")
	for i := 0; i < 5; i++ {
		if s.ShouldSample("trace-deterministic") != first {
			t.Fatalf("sampler must be deterministic for same trace id")
		}
	}
}

func TestRatioSampler_ApproximatesRatio(t *testing.T) {
	s, err := NewRatioSampler(0.25)
	if err != nil {
		t.Fatal(err)
	}
	sampled := 0
	total := 2000
	for i := 0; i < total; i++ {
		id := "trace-" + itoa(i)
		if s.ShouldSample(id) {
			sampled++
		}
	}
	// Allow ±5% absolute drift — deterministic hash on synthetic ids.
	ratio := float64(sampled) / float64(total)
	if ratio < 0.20 || ratio > 0.30 {
		t.Fatalf("expected ~0.25 sampled, got %v (sampled=%d total=%d)", ratio, sampled, total)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
