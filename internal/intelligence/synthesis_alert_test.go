package intelligence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// BUG-004-004 SCOPE-04 — T004-06-ALERT.
//
// Two failure modes, both silent:
//
//   1. The state mapping collapses two states into one, so an alert that
//      should fire never does.
//   2. The alert rule names a metric nothing publishes. Prometheus does not
//      complain about a rule whose series never appears; it simply never
//      fires, which looks exactly like a healthy system.

func TestMetricStateFor_StatesAreExclusiveAndOrdered(t *testing.T) {
	for name, tc := range map[string]struct {
		outcome SynthesisPersistenceOutcome
		want    int
	}{
		"never run": {
			SynthesisPersistenceOutcome{Phase: PhaseNoRun},
			SynthesisMetricNeverRun,
		},
		"probe error": {
			SynthesisPersistenceOutcome{Phase: PhaseProbeError},
			SynthesisMetricProbeError,
		},
		"write failed": {
			SynthesisPersistenceOutcome{Phase: PhaseWriteFailed},
			SynthesisMetricFailed,
		},
		"committed and fresh": {
			SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindFull},
			SynthesisMetricUp,
		},
		"committed quiet is still up": {
			SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindQuiet},
			SynthesisMetricUp,
		},
		"committed partial is never full health": {
			SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindPartial},
			SynthesisMetricPartial,
		},
		"stale outranks partial": {
			SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindPartial, Stale: true},
			SynthesisMetricStale,
		},
		// The one that matters most: a commit whose read-back did not succeed is
		// not a success. Reporting it as up is precisely the claim this packet
		// exists to remove.
		"committed but unverified is not up": {
			SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackMismatch, Output: OutputKindFull},
			SynthesisMetricFailed,
		},
		"failure is not cleared by a prior output": {
			SynthesisPersistenceOutcome{Phase: PhaseWriteFailed, HasPriorVerifiedOutput: true},
			SynthesisMetricFailed,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := MetricStateFor(tc.outcome); got != tc.want {
				t.Fatalf("state %d, want %d", got, tc.want)
			}
		})
	}
}

// Every state must be reachable and distinct. A mapping that returned the same
// value for two states would satisfy each individual case above while making
// one alert permanently unreachable.
func TestMetricStateFor_EveryStateIsDistinct(t *testing.T) {
	seen := map[int]string{}
	for name, outcome := range map[string]SynthesisPersistenceOutcome{
		"never-run":   {Phase: PhaseNoRun},
		"up":          {Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindFull},
		"stale":       {Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindFull, Stale: true},
		"partial":     {Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindPartial},
		"failed":      {Phase: PhaseWriteFailed},
		"probe-error": {Phase: PhaseProbeError},
	} {
		state := MetricStateFor(outcome)
		if prior, clash := seen[state]; clash {
			t.Fatalf("%q and %q both map to state %d; one of their alerts can never fire", name, prior, state)
		}
		seen[state] = name
	}
	if len(seen) != 6 {
		t.Fatalf("reached %d distinct states, want 6", len(seen))
	}
}

// The alert rules must reference metrics this code actually publishes.
// Prometheus never complains about a rule whose series does not exist -- it
// just never fires, which is indistinguishable from a healthy system.
func TestSynthesisAlertRules_ReferenceLivePublishedMetrics(t *testing.T) {
	rulesPath := filepath.Join("..", "..", "config", "prometheus", "alerts.yml")
	body, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("read alert rules: %v", err)
	}
	rules := string(body)

	for _, metric := range []string{
		"smackerel_synthesis_state",
		"smackerel_synthesis_last_verified_output_unixtime",
	} {
		if !strings.Contains(rules, metric) {
			t.Fatalf("alert rules never mention %q; the state it describes cannot be alerted on", metric)
		}
	}

	// Each numeric state an alert fires on must be one the mapping can actually
	// produce. A rule on `== 7` would be permanently dead.
	reachable := map[string]bool{}
	for _, outcome := range []SynthesisPersistenceOutcome{
		{Phase: PhaseNoRun},
		{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindFull},
		{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindFull, Stale: true},
		{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindPartial},
		{Phase: PhaseWriteFailed},
		{Phase: PhaseProbeError},
	} {
		reachable[string(rune('0'+MetricStateFor(outcome)))] = true
	}

	for _, line := range strings.Split(rules, "\n") {
		trimmed := strings.TrimSpace(line)
		const prefix = "expr: smackerel_synthesis_state == "
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		want := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		if !reachable[want] {
			t.Fatalf("an alert fires on smackerel_synthesis_state == %s, which MetricStateFor can never produce; that rule is dead", want)
		}
	}

	// Control: the alerts an operator depends on must be present by name. The
	// checks above would all pass on a file with no synthesis rules at all.
	for _, alert := range []string{
		"SmackerelSynthesisFailing",
		"SmackerelSynthesisStale",
		"SmackerelSynthesisNeverRun",
		"SmackerelSynthesisStateUnreadable",
	} {
		if !strings.Contains(rules, alert) {
			t.Fatalf("alert %q is absent; its state would be durable and unreported", alert)
		}
	}
}

type synthesisAlertDocument struct {
	Groups []struct {
		Rules []struct {
			Alert string `yaml:"alert"`
			Expr  string `yaml:"expr"`
			For   string `yaml:"for"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

type synthesisAlertContract struct {
	expression string
	window     string
}

func TestSynthesisAlertRules_ConsumeCanonicalStateWithConfiguredForWindows(t *testing.T) {
	rulesPath := filepath.Join("..", "..", "config", "prometheus", "alerts.yml")
	body, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("read alert rules: %v", err)
	}

	if err := validateSynthesisAlertContracts(body); err != nil {
		t.Fatal(err)
	}

	failingExpression := "(smackerel_synthesis_state == 3) or (smackerel_synthesis_state == 4)"
	mutated := strings.Replace(string(body), failingExpression, "smackerel_synthesis_state == 4", 1)
	if mutated == string(body) {
		t.Fatal("adversarial setup could not remove partial state 3 from SmackerelSynthesisFailing")
	}
	if err := validateSynthesisAlertContracts([]byte(mutated)); err == nil {
		t.Fatal("alert contract accepted SmackerelSynthesisFailing after partial state 3 was removed")
	}
}

func validateSynthesisAlertContracts(body []byte) error {
	want := map[string]synthesisAlertContract{
		"SmackerelSynthesisFailing": {
			expression: "(smackerel_synthesis_state == 3) or (smackerel_synthesis_state == 4)",
			window:     "30m",
		},
		"SmackerelSynthesisStale": {
			expression: "smackerel_synthesis_state == 2",
			window:     "1h",
		},
		"SmackerelSynthesisNeverRun": {
			expression: "smackerel_synthesis_state == 0",
			window:     "24h",
		},
		"SmackerelSynthesisStateUnreadable": {
			expression: "smackerel_synthesis_state == 5",
			window:     "15m",
		},
	}

	var document synthesisAlertDocument
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parse Prometheus alert YAML: %w", err)
	}

	seen := make(map[string]bool, len(want))
	for _, group := range document.Groups {
		for _, rule := range group.Rules {
			expected, required := want[rule.Alert]
			if !required {
				continue
			}
			if seen[rule.Alert] {
				return fmt.Errorf("synthesis alert %q is declared more than once", rule.Alert)
			}
			seen[rule.Alert] = true
			if rule.Expr != expected.expression {
				return fmt.Errorf("synthesis alert %q expression = %q, want %q", rule.Alert, rule.Expr, expected.expression)
			}
			if rule.For != expected.window {
				return fmt.Errorf("synthesis alert %q for window = %q, want %q", rule.Alert, rule.For, expected.window)
			}
		}
	}

	for alert := range want {
		if !seen[alert] {
			return fmt.Errorf("required synthesis alert %q is absent", alert)
		}
	}
	return nil
}

// A failing run must not be reported healthy merely because time has passed, so
// staleness and failure are evaluated against the same outcome rather than
// separately.
func TestMetricStateFor_StalenessDoesNotDowngradeAFailure(t *testing.T) {
	failedAndOld := SynthesisPersistenceOutcome{
		Phase:                  PhaseWriteFailed,
		Stale:                  true,
		HasPriorVerifiedOutput: true,
	}
	if got := MetricStateFor(failedAndOld); got != SynthesisMetricFailed {
		t.Fatalf("state %d for a failed-and-stale outcome, want failed (%d); staleness must not soften a failure",
			got, SynthesisMetricFailed)
	}
}
