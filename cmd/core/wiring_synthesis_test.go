package main

import (
	"errors"
	"os"
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

func TestSynthesisStartupReconciliationWiringPrecedesAdmission(t *testing.T) {
	wiringSource, err := os.ReadFile("wiring.go")
	if err != nil {
		t.Fatalf("read wiring.go: %v", err)
	}
	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	violations := synthesisStartupWiringViolations(string(wiringSource), string(mainSource))
	if len(violations) != 0 {
		t.Fatalf("synthesis startup reconciliation wiring violations:\n- %s", strings.Join(violations, "\n- "))
	}
}

func synthesisStartupWiringViolations(wiringSource string, mainSource string) []string {
	wiring := strings.Join(strings.Fields(wiringSource), " ")
	main := strings.Join(strings.Fields(mainSource), " ")
	violations := make([]string, 0)

	for description, fragment := range map[string]string{
		"runtime retains coordinator":           "coordinator *intelligence.SynthesisCoordinator",
		"runtime validates coordinator":         "if r.coordinator == nil {",
		"constructor stores coordinator":        "coordinator: coordinator,",
		"startup helper exists":                 "func reconcileSynthesisStartup(",
		"one observation time is normalized":    "observedAt := now.UTC()",
		"closed required cadences are iterated": "for _, cadence := range synthesisRT.freshnessPolicy.RequiredCadences() {",
		"coordinator reconciles each cadence":   "synthesisRT.coordinator.ReconcileStartup(ctx, cfg.Synthesis.ActorUserID, cadence, observedAt)",
	} {
		if !strings.Contains(wiring, fragment) {
			violations = append(violations, description)
		}
	}

	helperStart := strings.Index(wiring, "func reconcileSynthesisStartup(")
	if helperStart >= 0 {
		helper := wiring[helperStart:]
		if nextFunction := strings.Index(helper[len("func reconcileSynthesisStartup("):], " func "); nextFunction >= 0 {
			helper = helper[:len("func reconcileSynthesisStartup(")+nextFunction]
		}
		if strings.Count(helper, ".ReconcileStartup(") != 1 {
			violations = append(violations, "startup helper calls ReconcileStartup exactly once per required-cadence iteration")
		}
		if strings.Contains(helper, "CadenceDaily") || strings.Contains(helper, "CadenceWeekly") {
			violations = append(violations, "startup helper cannot select a literal cadence subset")
		}
		if strings.Contains(helper, " continue ") || strings.Contains(helper, " break ") {
			violations = append(violations, "startup helper cannot skip a required cadence")
		}
		if strings.Contains(helper, " go ") {
			violations = append(violations, "startup reconciliation must remain synchronous")
		}
	}

	constructIndex := strings.Index(main, "synthesisRT, err := newSynthesisRuntime(cfg, svc)")
	reconcileIndex := strings.Index(main, "reconcileSynthesisStartup(ctx, cfg, synthesisRT, time.Now())")
	if constructIndex < 0 {
		violations = append(violations, "run constructs the synthesis runtime")
	}
	if reconcileIndex < 0 {
		violations = append(violations, "run invokes startup reconciliation")
	}
	if strings.Count(main, "reconcileSynthesisStartup(ctx, cfg, synthesisRT, time.Now())") != 1 {
		violations = append(violations, "run invokes startup reconciliation exactly once")
	}
	if constructIndex >= 0 && reconcileIndex >= 0 && reconcileIndex <= constructIndex {
		violations = append(violations, "startup reconciliation follows synthesis runtime construction")
	}
	for _, boundary := range []string{
		"registerConnectors(ctx, cfg, svc)",
		"buildAPIDeps(ctx, cfg, svc, synthesisRT, corpusGrantEnforce)",
		"api.NewRouter(deps)",
		"scheduler.New(",
		"sched.Start(ctx, cfg.DigestCron)",
	} {
		boundaryIndex := strings.Index(main, boundary)
		if boundaryIndex < 0 {
			violations = append(violations, "startup boundary remains present: "+boundary)
			continue
		}
		if reconcileIndex >= boundaryIndex {
			violations = append(violations, "startup reconciliation precedes "+boundary)
		}
	}

	return violations
}
