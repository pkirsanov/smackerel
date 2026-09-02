package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const synthesisCadenceOverlayName = "docker-compose.synthesis-cadence-e2e.override.yml"

type synthesisCadenceOverlayDocument struct {
	Services map[string]struct {
		Environment map[string]string `yaml:"environment"`
	} `yaml:"services"`
}

func assertSynthesisCadenceOverlayContract(yamlBytes []byte) error {
	var doc synthesisCadenceOverlayDocument
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("parse synthesis cadence overlay: %w", err)
	}
	if len(doc.Services) != 1 {
		return fmt.Errorf("overlay must override exactly one service, got %d", len(doc.Services))
	}
	core, ok := doc.Services["smackerel-core"]
	if !ok {
		return fmt.Errorf("overlay must override only services.smackerel-core")
	}
	if len(core.Environment) != 2 {
		return fmt.Errorf("core overlay must override exactly two environment keys, got %d", len(core.Environment))
	}
	want := map[string]string{
		"SYNTHESIS_DAILY_CRON":  "@every 8s",
		"SYNTHESIS_WEEKLY_CRON": "@every 11s",
	}
	for key, value := range want {
		if core.Environment[key] != value {
			return fmt.Errorf("core overlay %s = %q, want %q", key, core.Environment[key], value)
		}
	}
	for key := range core.Environment {
		if _, allowed := want[key]; !allowed {
			return fmt.Errorf("core overlay contains out-of-scope environment key %s", key)
		}
	}
	return nil
}

func TestSynthesisCadenceE2EOverlayIsCoreOnlyAndBounded(t *testing.T) {
	overlayPath := filepath.Join(repoRoot(t), synthesisCadenceOverlayName)
	yamlBytes, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatalf("read synthesis cadence E2E overlay %q: %v", overlayPath, err)
	}
	if !strings.Contains(string(yamlBytes), "TEST-ONLY") || !strings.Contains(string(yamlBytes), "production-inert") {
		t.Fatal("synthesis cadence overlay must document that it is TEST-ONLY and production-inert")
	}
	if err := assertSynthesisCadenceOverlayContract(yamlBytes); err != nil {
		t.Fatal(err)
	}
}

func TestSynthesisCadenceE2EOverlayIsNotActivatedByDefaultOrProduction(t *testing.T) {
	for _, relativePath := range []string{
		"docker-compose.yml",
		"docker-compose.prod.yml",
		filepath.Join("deploy", "compose.deploy.yml"),
		filepath.Join("scripts", "lib", "runtime.sh"),
		"smackerel.sh",
	} {
		path := filepath.Join(repoRoot(t), relativePath)
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read production/default surface %q: %v", path, err)
		}
		if strings.Contains(string(contents), synthesisCadenceOverlayName) {
			t.Fatalf("production/default surface %s activates or names the test-only cadence overlay", relativePath)
		}
	}
}

func TestSynthesisCadenceE2EOverlayRejectsAdditionalMutation(t *testing.T) {
	const extraEnvironment = `services:
  smackerel-core:
    environment:
      SYNTHESIS_DAILY_CRON: "@every 8s"
      SYNTHESIS_WEEKLY_CRON: "@every 11s"
      LOG_LEVEL: debug
`
	if err := assertSynthesisCadenceOverlayContract([]byte(extraEnvironment)); err == nil {
		t.Fatal("overlay contract accepted an unrelated core environment override")
	}

	const extraService = `services:
  smackerel-core:
    environment:
      SYNTHESIS_DAILY_CRON: "@every 8s"
      SYNTHESIS_WEEKLY_CRON: "@every 11s"
  postgres:
    environment: {}
`
	if err := assertSynthesisCadenceOverlayContract([]byte(extraService)); err == nil {
		t.Fatal("overlay contract accepted an unrelated service override")
	}
}
