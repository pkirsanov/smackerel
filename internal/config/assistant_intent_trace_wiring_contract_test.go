package config

import (
	"fmt"
	"strings"
	"testing"
)

var intentTraceSSTKeys = []string{
	"ASSISTANT_INTENT_TRACE_SAMPLING_RATIO",
	"ASSISTANT_INTENT_TRACE_RETENTION_DAYS",
	"ASSISTANT_INTENT_TRACE_EXPORT_TARGETS",
	"ASSISTANT_INTENT_TRACE_REPLAY_ENABLED",
	"ASSISTANT_INTENT_TRACE_RETENTION_SWEEP_INTERVAL",
}

func assertIntentTraceSSTWiring(generator, assistantLoader string) error {
	for _, key := range intentTraceSSTKeys {
		if !strings.Contains(generator, key+"=\"$(required_value assistant.intent_trace.") {
			return fmt.Errorf("config generator does not read required intent-trace key %s", key)
		}
		if !strings.Contains(generator, key+"=${"+key+"}") {
			return fmt.Errorf("config generator does not emit required intent-trace key %s", key)
		}
	}
	if !strings.Contains(assistantLoader, "loadIntentTraceConfig(cfg, &errs)") {
		return fmt.Errorf("aggregate assistant loader does not invoke loadIntentTraceConfig")
	}
	return nil
}

func TestIntentTraceSSTWiringContract_LiveGeneratorAndLoader(t *testing.T) {
	generator := readRepoFile(t, "scripts/commands/config.sh")
	assistantLoader := readRepoFile(t, "internal/config/assistant.go")
	if err := assertIntentTraceSSTWiring(generator, assistantLoader); err != nil {
		t.Fatal(err)
	}
}

func TestIntentTraceSSTWiringContract_AdversarialRejectsMissingReplayEmission(t *testing.T) {
	generator := readRepoFile(t, "scripts/commands/config.sh")
	assistantLoader := readRepoFile(t, "internal/config/assistant.go")
	broken := strings.Replace(generator,
		"ASSISTANT_INTENT_TRACE_REPLAY_ENABLED=${ASSISTANT_INTENT_TRACE_REPLAY_ENABLED}",
		"", 1)
	if err := assertIntentTraceSSTWiring(broken, assistantLoader); err == nil {
		t.Fatal("contract accepted a generator that drops replay_enabled from generated env")
	}
}

func TestIntentTraceSSTWiringContract_AdversarialRejectsDetachedLoader(t *testing.T) {
	generator := readRepoFile(t, "scripts/commands/config.sh")
	assistantLoader := readRepoFile(t, "internal/config/assistant.go")
	broken := strings.Replace(assistantLoader, "loadIntentTraceConfig(cfg, &errs)", "", 1)
	if err := assertIntentTraceSSTWiring(generator, broken); err == nil {
		t.Fatal("contract accepted an intent-trace loader that is never called by aggregate config loading")
	}
}
