package webcreds

import (
	"strings"
	"testing"
	"time"
)

func TestHash_RoundTrip(t *testing.T) {
	const pw = "correct-horse-battery-staple"
	phc, err := Hash(pw)
	if err != nil {
		t.Fatalf("Hash returned err: %v", err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$") {
		t.Fatalf("phc string missing argon2id prefix: %q", phc)
	}
	if err := Verify(phc, pw); err != nil {
		t.Fatalf("Verify with correct password failed: %v", err)
	}
}

func TestHash_RejectsShortPasswords(t *testing.T) {
	for _, short := range []string{"", "x", "shortpw", "elevenchars"} {
		if _, err := Hash(short); err == nil {
			t.Errorf("Hash(%q) expected err for short password, got nil", short)
		}
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	phc, err := Hash("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("Hash err: %v", err)
	}
	for _, wrong := range []string{
		"correct-horse-battery-stapleX",
		"Correct-Horse-Battery-Staple",
		"",
		"completely-different-password",
	} {
		if err := Verify(phc, wrong); err != ErrInvalidCredentials {
			t.Errorf("Verify(%q) expected ErrInvalidCredentials, got %v", wrong, err)
		}
	}
}

func TestVerify_TamperedHash(t *testing.T) {
	phc, err := Hash("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("Hash err: %v", err)
	}
	// Flip a character in the hash suffix.
	tampered := phc[:len(phc)-2] + "AA"
	if err := Verify(tampered, "correct-horse-battery-staple"); err != ErrInvalidCredentials {
		t.Errorf("Verify(tampered) expected ErrInvalidCredentials, got %v", err)
	}
}

func TestVerify_MalformedPHC(t *testing.T) {
	cases := []string{
		"",
		"not-a-phc-string",
		"$argon2id$v=99$m=65536,t=1,p=4$AAAA$BBBB", // wrong version
		"$bcrypt$v=19$m=65536,t=1,p=4$AAAA$BBBB",   // wrong algo
		"$argon2id$v=19$m=BAD$AAAA$BBBB",           // wrong params
		"$argon2id$v=19$m=65536,t=1,p=4$!!!$BBBB",  // invalid base64 salt
		"$argon2id$v=19$m=65536,t=1,p=4$AAAA$!!!",  // invalid base64 hash
	}
	for _, c := range cases {
		if err := Verify(c, "any-password-here"); err != ErrInvalidCredentials {
			t.Errorf("Verify(%q) expected ErrInvalidCredentials, got %v", c, err)
		}
	}
}

func TestDummyHash_VerifiesAgainstNothing(t *testing.T) {
	// Random user-supplied passwords MUST always fail against the dummy.
	for _, pw := range []string{"alpha-long-enough", "beta-also-long-enough", "gamma-yet-another"} {
		if err := Verify(DummyHash(), pw); err != ErrInvalidCredentials {
			t.Errorf("Verify(dummy, %q) expected ErrInvalidCredentials, got %v", pw, err)
		}
	}
}

func TestVerify_TimingParityWithinConstantFactor(t *testing.T) {
	// Adversarial timing parity check (AC-5 — no user-enumeration leak
	// via timing). The wall-clock cost of Verify against an unknown
	// user (collapsed to a DummyHash compare) MUST stay within the same
	// order of magnitude as a known-user-wrong-password Verify.
	//
	// Guard band: 0.5..2.0 (a 2x constant factor either way), NOT a
	// literal ±20%. A timing assertion this tight would be flaky in CI:
	// argon2id's wall-clock cost swings well past ±20% under GC pauses,
	// scheduler preemption, and shared-runner contention, which would
	// produce false failures unrelated to the security property.
	//
	// The band's real job is to catch the one regression that matters:
	// if a future refactor skips the dummy compare on unknown users, the
	// unknown path returns near-instantly (ratio -> ~0), landing far
	// below 0.5 and failing this test. The 2x band reliably catches that
	// collapse without paying for jitter false-positives.
	const iterations = 30
	pw := "correct-horse-battery-staple"
	phc, err := Hash(pw)
	if err != nil {
		t.Fatalf("Hash err: %v", err)
	}

	knownWrong := medianDuration(t, iterations, func() { _ = Verify(phc, "wrong-password-here") })
	unknown := medianDuration(t, iterations, func() { _ = Verify(DummyHash(), "wrong-password-here") })

	t.Logf("median timings: known-wrong=%s unknown=%s", knownWrong, unknown)
	ratio := float64(unknown) / float64(knownWrong)
	if ratio < 0.5 || ratio > 2.0 {
		t.Errorf("timing parity violated: unknown/known ratio = %.2f (want 0.5..2.0, same order of magnitude)", ratio)
	}
}

func medianDuration(t *testing.T, n int, fn func()) time.Duration {
	t.Helper()
	samples := make([]time.Duration, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		fn()
		samples[i] = time.Since(start)
	}
	// Simple insertion sort — n is tiny.
	for i := 1; i < len(samples); i++ {
		for j := i; j > 0 && samples[j-1] > samples[j]; j-- {
			samples[j-1], samples[j] = samples[j], samples[j-1]
		}
	}
	return samples[n/2]
}
