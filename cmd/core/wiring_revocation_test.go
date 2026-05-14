package main

import (
	"os"
	"strings"
	"testing"
)

// HL-RESCAN-008: resolveBroadcasterInstanceID fail-loud gating.
//
// These tests are the adversarial proof for HL-RESCAN-008 (P2 finding from
// the 2026-05-14 home-lab readiness re-scan against c586f912). The pre-fix
// form at cmd/core/wiring.go:243 was:
//
//	instanceID := os.Getenv("HOSTNAME")
//	if instanceID == "" {
//	    instanceID = "smackerel-core"
//	}
//
// which violates Gate G028 (no-defaults SST policy: Go must be `os.Getenv`
// + empty check → fatal/refuse, never a hidden fallback string). It also
// collided every replica's broadcaster identity to the same literal name,
// defeating per-replica deduplication on the NATS revocation subject.
//
// The Empty test would FAIL if anyone restored a silent fallback or
// deleted the empty check; the NonEmpty test guards against an over-zealous
// rewrite that mistakenly errors on every read. Adversarial: the empty-input
// case exercises the exact pre-fix branch that HL-RESCAN-008 calls out.

// TestResolveBroadcasterInstanceID_NonEmpty proves the helper returns the
// HOSTNAME value verbatim when the env var is set to a non-empty string.
// This is the positive guard rail for the fail-loud gate.
func TestResolveBroadcasterInstanceID_NonEmpty(t *testing.T) {
	t.Setenv("HOSTNAME", "smackerel-core-replica-7")

	got, err := resolveBroadcasterInstanceID()
	if err != nil {
		t.Fatalf("expected nil error for non-empty HOSTNAME, got %v", err)
	}
	if got != "smackerel-core-replica-7" {
		t.Fatalf("expected instance id %q, got %q", "smackerel-core-replica-7", got)
	}
}

// TestResolveBroadcasterInstanceID_Empty_FailsLoud proves the helper returns
// a non-nil error referencing HL-RESCAN-008, Gate G028, and spec 044 when
// HOSTNAME is unset. This is the ADVERSARIAL regression sub-test for
// HL-RESCAN-008 — restoring the pre-fix silent fallback to the literal
// "smackerel-core" string would cause this test to FAIL (the helper would
// return a non-nil string and a nil error instead of the expected error).
func TestResolveBroadcasterInstanceID_Empty_FailsLoud(t *testing.T) {
	// t.Setenv "" is treated as "set to empty string", which os.Getenv
	// reports as "" — exactly the input we want to exercise.
	t.Setenv("HOSTNAME", "")

	got, err := resolveBroadcasterInstanceID()
	if err == nil {
		t.Fatalf("expected non-nil error for empty HOSTNAME (HL-RESCAN-008 silent fallback regression), got id=%q err=nil", got)
	}
	if got != "" {
		t.Fatalf("expected empty instance id when HOSTNAME is empty, got %q", got)
	}
	msg := err.Error()
	for _, want := range []string{"HOSTNAME", "HL-RESCAN-008", "Gate G028", "spec 044", "deduplication"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing required token %q; got: %s", want, msg)
		}
	}
}

// TestResolveBroadcasterInstanceID_UnsetEnv exercises the case where
// HOSTNAME is genuinely unset (not just set to empty string). os.Getenv
// returns "" for both unset and empty-set, so the behavior is identical
// — but the test guards against a future refactor that switches from
// os.Getenv to os.LookupEnv with mismatched empty/unset semantics.
func TestResolveBroadcasterInstanceID_UnsetEnv(t *testing.T) {
	// Save and unset HOSTNAME.
	prev, hadPrev := os.LookupEnv("HOSTNAME")
	if err := os.Unsetenv("HOSTNAME"); err != nil {
		t.Fatalf("failed to unset HOSTNAME: %v", err)
	}
	t.Cleanup(func() {
		if hadPrev {
			os.Setenv("HOSTNAME", prev)
		}
	})

	got, err := resolveBroadcasterInstanceID()
	if err == nil {
		t.Fatalf("expected non-nil error for unset HOSTNAME, got id=%q err=nil", got)
	}
	if got != "" {
		t.Fatalf("expected empty instance id when HOSTNAME is unset, got %q", got)
	}
}
