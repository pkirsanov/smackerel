package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type synthesisSSTDocument struct {
	Intelligence struct {
		Synthesis struct {
			ActorUserID            string   `yaml:"actor_user_id"`
			DailyCron              string   `yaml:"daily_cron"`
			WeeklyCron             string   `yaml:"weekly_cron"`
			RetryBudget            int      `yaml:"retry_budget"`
			RetryBackoffSeconds    int      `yaml:"retry_backoff_seconds"`
			RetryMaxBackoffSeconds int      `yaml:"retry_max_backoff_seconds"`
			LeaseSeconds           int      `yaml:"lease_seconds"`
			PolicyVersion          string   `yaml:"policy_version"`
			RequiredSourceClasses  []string `yaml:"required_source_classes"`
			OptionalSourceClasses  []string `yaml:"optional_source_classes"`
			RetentionSeconds       int      `yaml:"retention_seconds"`
		} `yaml:"synthesis"`
	} `yaml:"intelligence"`
}

type synthesisGeneratorField struct {
	envKey   string
	sstPath  string
	readFunc string
}

var synthesisGeneratorFields = []synthesisGeneratorField{
	{envKey: "SYNTHESIS_ACTOR_USER_ID", sstPath: "intelligence.synthesis.actor_user_id", readFunc: "required_value"},
	{envKey: "SYNTHESIS_DAILY_CRON", sstPath: "intelligence.synthesis.daily_cron", readFunc: "required_value"},
	{envKey: "SYNTHESIS_WEEKLY_CRON", sstPath: "intelligence.synthesis.weekly_cron", readFunc: "required_value"},
	{envKey: "SYNTHESIS_RETRY_BUDGET", sstPath: "intelligence.synthesis.retry_budget", readFunc: "required_value"},
	{envKey: "SYNTHESIS_RETRY_BACKOFF_SECONDS", sstPath: "intelligence.synthesis.retry_backoff_seconds", readFunc: "required_value"},
	{envKey: "SYNTHESIS_RETRY_MAX_BACKOFF_SECONDS", sstPath: "intelligence.synthesis.retry_max_backoff_seconds", readFunc: "required_value"},
	{envKey: "SYNTHESIS_LEASE_SECONDS", sstPath: "intelligence.synthesis.lease_seconds", readFunc: "required_value"},
	{envKey: "SYNTHESIS_POLICY_VERSION", sstPath: "intelligence.synthesis.policy_version", readFunc: "required_value"},
	{envKey: "SYNTHESIS_REQUIRED_SOURCE_CLASSES", sstPath: "intelligence.synthesis.required_source_classes", readFunc: "required_json_value"},
	{envKey: "SYNTHESIS_OPTIONAL_SOURCE_CLASSES", sstPath: "intelligence.synthesis.optional_source_classes", readFunc: "required_json_value"},
	{envKey: "SYNTHESIS_RETENTION_SECONDS", sstPath: "intelligence.synthesis.retention_seconds", readFunc: "required_value"},
}

func assertSynthesisGeneratorContract(sstYAML, generator string) error {
	var doc synthesisSSTDocument
	if err := yaml.Unmarshal([]byte(sstYAML), &doc); err != nil {
		return fmt.Errorf("parse config/smackerel.yaml: %w", err)
	}

	got := doc.Intelligence.Synthesis
	if got.ActorUserID != "global-corpus" ||
		got.DailyCron != "0 2 * * *" ||
		got.WeeklyCron != "0 16 * * 0" ||
		got.RetryBudget != 2 ||
		got.RetryBackoffSeconds != 2 ||
		got.RetryMaxBackoffSeconds != 30 ||
		got.LeaseSeconds != 600 ||
		got.PolicyVersion != "synthesis/v1" ||
		got.RetentionSeconds != 7776000 ||
		!reflect.DeepEqual(got.RequiredSourceClasses, []string{"canonical-graph"}) ||
		!reflect.DeepEqual(got.OptionalSourceClasses, []string{}) {
		return fmt.Errorf("intelligence.synthesis SST values do not match the explicit Scope 7 contract: %#v", got)
	}

	for _, field := range synthesisGeneratorFields {
		read := field.envKey + `="$( `
		read = strings.Replace(read, "$( ", "$(", 1) + field.readFunc + " " + field.sstPath + `)"`
		if !strings.Contains(generator, read) {
			return fmt.Errorf("generator does not read %s through %s", field.sstPath, field.readFunc)
		}
		emit := field.envKey + "=${" + field.envKey + ":?"
		if !strings.Contains(generator, emit) {
			return fmt.Errorf("generator does not emit %s with a fail-loud empty-value guard", field.envKey)
		}
		for _, fallback := range []string{
			"${" + field.envKey + ":-",
			"${" + field.envKey + "-",
			"${" + field.envKey + ":=",
		} {
			if strings.Contains(generator, fallback) {
				return fmt.Errorf("generator contains forbidden fallback %q", fallback)
			}
		}
	}

	for _, validation := range []string{
		`validate_synthesis_uint "intelligence.synthesis.retry_budget" "$SYNTHESIS_RETRY_BUDGET" 0 10`,
		`validate_synthesis_uint "intelligence.synthesis.retry_backoff_seconds" "$SYNTHESIS_RETRY_BACKOFF_SECONDS" 1 2147483647`,
		`validate_synthesis_uint "intelligence.synthesis.retry_max_backoff_seconds" "$SYNTHESIS_RETRY_MAX_BACKOFF_SECONDS" 1 2147483647`,
		`validate_synthesis_uint "intelligence.synthesis.lease_seconds" "$SYNTHESIS_LEASE_SECONDS" 1 2147483647`,
		`validate_synthesis_uint "intelligence.synthesis.retention_seconds" "$SYNTHESIS_RETENTION_SECONDS" 1 2147483647`,
		`validate_synthesis_source_classes "$SYNTHESIS_REQUIRED_SOURCE_CLASSES" "$SYNTHESIS_OPTIONAL_SOURCE_CLASSES"`,
	} {
		if !strings.Contains(generator, validation) {
			return fmt.Errorf("generator is missing required synthesis validation %q", validation)
		}
	}
	return nil
}

func TestSynthesisRunPolicyGeneratorContract(t *testing.T) {
	sstYAML := readRepoFile(t, "config/smackerel.yaml")
	generator := readRepoFile(t, "scripts/commands/config.sh")
	if err := assertSynthesisGeneratorContract(sstYAML, generator); err != nil {
		t.Fatal(err)
	}

	t.Run("rejects_fallback_emission", func(t *testing.T) {
		broken := strings.Replace(
			generator,
			"SYNTHESIS_ACTOR_USER_ID=${SYNTHESIS_ACTOR_USER_ID:?",
			"SYNTHESIS_ACTOR_USER_ID=${SYNTHESIS_ACTOR_USER_ID:-global-corpus}",
			1,
		)
		if broken == generator {
			t.Fatal("adversarial fixture is stale: actor emission was not found")
		}
		if err := assertSynthesisGeneratorContract(sstYAML, broken); err == nil {
			t.Fatal("contract accepted a synthesis actor fallback")
		}
	})

	t.Run("rejects_dropped_required_read", func(t *testing.T) {
		broken := strings.Replace(
			generator,
			`SYNTHESIS_RETRY_BUDGET="$(required_value intelligence.synthesis.retry_budget)"`,
			"",
			1,
		)
		if broken == generator {
			t.Fatal("adversarial fixture is stale: retry-budget read was not found")
		}
		if err := assertSynthesisGeneratorContract(sstYAML, broken); err == nil {
			t.Fatal("contract accepted a missing retry-budget read")
		}
	})

	t.Run("rejects_dropped_weekly_cadence_read", func(t *testing.T) {
		broken := strings.Replace(
			generator,
			`SYNTHESIS_WEEKLY_CRON="$(required_value intelligence.synthesis.weekly_cron)"`,
			"",
			1,
		)
		if broken == generator {
			t.Fatal("adversarial fixture is stale: weekly cadence read was not found")
		}
		if err := assertSynthesisGeneratorContract(sstYAML, broken); err == nil {
			t.Fatal("contract accepted a missing weekly cadence read")
		}
	})
}
