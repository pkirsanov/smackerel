package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/intelligence"
)

func TestRunSynthesisRuntimePoliciesMapConfigExactly(t *testing.T) {
	cfg := config.SynthesisConfig{
		ActorUserID:           "configured-actor",
		RetryBudget:           4,
		RetryBackoff:          3 * time.Second,
		RetryMaxBackoff:       19 * time.Second,
		LeaseTTL:              47 * time.Second,
		PolicyVersion:         "synthesis/test-v9",
		RequiredSourceClasses: []string{"canonical-graph"},
		OptionalSourceClasses: []string{},
		Retention:             73 * time.Hour,
	}

	maxAttempts, err := cfg.MaxAttempts()
	if err != nil {
		t.Fatalf("MaxAttempts: %v", err)
	}
	runPolicy := synthesisRunPolicyFromConfig(cfg)
	retryPolicy := synthesisRetryPolicyFromConfig(cfg, maxAttempts)

	wantRunPolicy := intelligence.SynthesisRunPolicy{
		Actor:                 cfg.ActorUserID,
		PolicyVersion:         cfg.PolicyVersion,
		RequiredSourceClasses: []string{"canonical-graph"},
		OptionalSourceClasses: []string{},
		Retention:             cfg.Retention,
	}
	wantRetryPolicy := intelligence.SynthesisRetryPolicy{
		MaxAttempts:  cfg.RetryBudget + 1,
		InitialDelay: cfg.RetryBackoff,
		MaxDelay:     cfg.RetryMaxBackoff,
		LeaseTTL:     cfg.LeaseTTL,
	}
	if !reflect.DeepEqual(runPolicy, wantRunPolicy) {
		t.Fatalf("run policy mismatch\n got: %#v\nwant: %#v", runPolicy, wantRunPolicy)
	}
	if !reflect.DeepEqual(retryPolicy, wantRetryPolicy) {
		t.Fatalf("retry policy mismatch\n got: %#v\nwant: %#v", retryPolicy, wantRetryPolicy)
	}

	cfg.RequiredSourceClasses[0] = "mutated-after-mapping"
	if runPolicy.RequiredSourceClasses[0] != "canonical-graph" {
		t.Fatalf("run policy retained the config slice instead of copying it: %#v", runPolicy.RequiredSourceClasses)
	}
}

func TestRunSynthesisHolderIsProcessUniqueAndFailsLoud(t *testing.T) {
	t.Run("hostname lookup error", func(t *testing.T) {
		lookupErr := errors.New("lookup unavailable")
		holder, err := resolveSynthesisHolder(func() (string, error) {
			return "", lookupErr
		}, 123)
		if !errors.Is(err, lookupErr) {
			t.Fatalf("expected hostname error to propagate, got holder=%q err=%v", holder, err)
		}
	})

	t.Run("blank hostname", func(t *testing.T) {
		holder, err := resolveSynthesisHolder(func() (string, error) {
			return "  ", nil
		}, 123)
		if err == nil || !strings.Contains(err.Error(), "hostname is empty") {
			t.Fatalf("expected blank hostname refusal, got holder=%q err=%v", holder, err)
		}
	})

	t.Run("invalid process id", func(t *testing.T) {
		holder, err := resolveSynthesisHolder(func() (string, error) {
			return "core-host", nil
		}, 0)
		if err == nil || !strings.Contains(err.Error(), "positive PID") {
			t.Fatalf("expected invalid PID refusal, got holder=%q err=%v", holder, err)
		}
	})

	t.Run("hostname plus process id", func(t *testing.T) {
		holder, err := resolveSynthesisHolder(func() (string, error) {
			return "core-host", nil
		}, 7123)
		if err != nil {
			t.Fatalf("resolve holder: %v", err)
		}
		if holder != "core-host-7123" {
			t.Fatalf("expected process-unique holder %q, got %q", "core-host-7123", holder)
		}
	})
}

func TestRunSynthesisRuntimeRejectsMissingDependencies(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		svc     *coreServices
		wantErr string
	}{
		{name: "configuration", cfg: nil, svc: &coreServices{}, wantErr: "requires configuration"},
		{name: "core services", cfg: &config.Config{}, svc: nil, wantErr: "requires core services"},
		{name: "postgres service", cfg: &config.Config{}, svc: &coreServices{}, wantErr: "requires the postgres service"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, err := newSynthesisRuntime(test.cfg, test.svc)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected %q error, got runtime=%v err=%v", test.wantErr, runtime, err)
			}
		})
	}
}
