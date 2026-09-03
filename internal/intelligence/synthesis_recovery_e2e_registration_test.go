package intelligence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	synthesisRecoveryE2ERunnerName = "synthesis_recovery_health_e2e_test"
	synthesisRecoveryE2EFilename   = synthesisRecoveryE2ERunnerName + ".sh"
)

type synthesisRecoveryRegistrationSources struct {
	runner     string
	dispatcher string
	testScript string
}

func TestSynthesisRecoveryHealthE2ERegistration(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	sources := synthesisRecoveryRegistrationSources{
		runner: readRegistrationSource(t, filepath.Join(repositoryRoot, "tests", "e2e", "run_all.sh")),
		dispatcher: readRegistrationSource(t, filepath.Join(repositoryRoot, "smackerel.sh")),
		testScript: readRegistrationSource(t, filepath.Join(repositoryRoot, "tests", "e2e", synthesisRecoveryE2EFilename)),
	}

	if err := validateSynthesisRecoveryRegistration(sources); err != nil {
		t.Fatal(err)
	}

	mutations := map[string]func(synthesisRecoveryRegistrationSources) synthesisRecoveryRegistrationSources{
		"runner lifecycle": func(mutated synthesisRecoveryRegistrationSources) synthesisRecoveryRegistrationSources {
			mutated.runner = removeRegistrationMember(t, mutated.runner, "LIFECYCLE_TESTS=", "\n", synthesisRecoveryE2ERunnerName)
			return mutated
		},
		"runner required": func(mutated synthesisRecoveryRegistrationSources) synthesisRecoveryRegistrationSources {
			mutated.runner = removeRegistrationMember(t, mutated.runner, "REQUIRED_TESTS=", "\n", synthesisRecoveryE2ERunnerName)
			return mutated
		},
		"targeted timeout": func(mutated synthesisRecoveryRegistrationSources) synthesisRecoveryRegistrationSources {
			mutated.dispatcher = removeRegistrationMember(t, mutated.dispatcher, "e2e_shell_timeout_for() {", "e2e_shell_test_manages_stack() {", synthesisRecoveryE2EFilename)
			return mutated
		},
		"targeted lifecycle ownership": func(mutated synthesisRecoveryRegistrationSources) synthesisRecoveryRegistrationSources {
			mutated.dispatcher = removeRegistrationMember(t, mutated.dispatcher, "e2e_shell_test_manages_stack() {", "e2e_print_test_stack_state() {", synthesisRecoveryE2EFilename)
			return mutated
		},
		"required shell inventory": func(mutated synthesisRecoveryRegistrationSources) synthesisRecoveryRegistrationSources {
			mutated.dispatcher = removeRegistrationMember(t, mutated.dispatcher, "e2e_required_shell_tests=(", "e2e_shell_test_is_required() {", synthesisRecoveryE2EFilename)
			return mutated
		},
		"full lane lifecycle": func(mutated synthesisRecoveryRegistrationSources) synthesisRecoveryRegistrationSources {
			mutated.dispatcher = removeRegistrationMember(t, mutated.dispatcher, "e2e_lifecycle_scripts=(", "for e2e_script in", synthesisRecoveryE2EFilename)
			return mutated
		},
	}

	for name, mutate := range mutations {
		t.Run("rejects missing "+name, func(t *testing.T) {
			if err := validateSynthesisRecoveryRegistration(mutate(sources)); err == nil {
				t.Fatalf("registration contract accepted missing %s", name)
			}
		})
	}
}

func readRegistrationSource(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func validateSynthesisRecoveryRegistration(sources synthesisRecoveryRegistrationSources) error {
	for _, required := range []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		`source "$SCRIPT_DIR/lib/helpers.sh"`,
		`trap 'cleanup "$?"' EXIT`,
	} {
		if !strings.Contains(sources.testScript, required) {
			return fmt.Errorf("%s lacks required lifecycle source %q", synthesisRecoveryE2EFilename, required)
		}
	}

	checks := []struct {
		name   string
		source string
		start  string
		end    string
		member string
	}{
		{name: "runner lifecycle", source: sources.runner, start: "LIFECYCLE_TESTS=", end: "\n", member: synthesisRecoveryE2ERunnerName},
		{name: "runner required", source: sources.runner, start: "REQUIRED_TESTS=", end: "\n", member: synthesisRecoveryE2ERunnerName},
		{name: "targeted timeout", source: sources.dispatcher, start: "e2e_shell_timeout_for() {", end: "e2e_shell_test_manages_stack() {", member: synthesisRecoveryE2EFilename},
		{name: "targeted lifecycle ownership", source: sources.dispatcher, start: "e2e_shell_test_manages_stack() {", end: "e2e_print_test_stack_state() {", member: synthesisRecoveryE2EFilename},
		{name: "required shell inventory", source: sources.dispatcher, start: "e2e_required_shell_tests=(", end: "e2e_shell_test_is_required() {", member: synthesisRecoveryE2EFilename},
		{name: "full lane lifecycle", source: sources.dispatcher, start: "e2e_lifecycle_scripts=(", end: "for e2e_script in", member: synthesisRecoveryE2EFilename},
	}
	for _, check := range checks {
		block, err := registrationBlock(check.source, check.start, check.end)
		if err != nil {
			return fmt.Errorf("%s: %w", check.name, err)
		}
		if !strings.Contains(block, check.member) {
			return fmt.Errorf("%s does not register %s", check.name, check.member)
		}
	}

	exactMatchers := []struct {
		name  string
		start string
		end   string
		want  string
	}{
		{name: "lifecycle classifier", start: "is_lifecycle_test() {", end: "is_required_test() {", want: `[[ "$name" == "$lt" ]]`},
		{name: "required classifier", start: "is_required_test() {", end: "run_test() {", want: `[[ "$name" == "$rt" ]]`},
	}
	for _, matcher := range exactMatchers {
		block, err := registrationBlock(sources.runner, matcher.start, matcher.end)
		if err != nil {
			return fmt.Errorf("%s: %w", matcher.name, err)
		}
		if !strings.Contains(block, matcher.want) || strings.Contains(block, `$name.sh`) {
			return fmt.Errorf("%s must compare the extensionless runner basename exactly", matcher.name)
		}
	}
	return nil
}

func registrationBlock(source, start, end string) (string, error) {
	startIndex := strings.Index(source, start)
	if startIndex < 0 {
		return "", fmt.Errorf("start marker %q is absent", start)
	}
	endIndex := strings.Index(source[startIndex+len(start):], end)
	if endIndex < 0 {
		return "", fmt.Errorf("end marker %q is absent after %q", end, start)
	}
	return source[startIndex : startIndex+len(start)+endIndex], nil
}

func removeRegistrationMember(t *testing.T, source, start, end, member string) string {
	t.Helper()
	block, err := registrationBlock(source, start, end)
	if err != nil {
		t.Fatal(err)
	}
	mutatedBlock := strings.Replace(block, member, "", 1)
	if mutatedBlock == block {
		t.Fatalf("test setup could not remove %s from block beginning %s", member, start)
	}
	return strings.Replace(source, block, mutatedBlock, 1)
}