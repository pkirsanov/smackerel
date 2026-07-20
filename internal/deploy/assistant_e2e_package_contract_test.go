package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertAssistantE2EPackageContract(dispatch, wrapper string) error {
	dispatchSignals := []string{
		"test e2e [--go-package assistant]",
		"GO_E2E_PACKAGE_SELECTOR=\"\"",
		"unsupported --go-package value",
		"go_e2e_args+=(--package \"$GO_E2E_PACKAGE_SELECTOR\")",
		"-z \"$GO_E2E_RUN_SELECTOR\" && -z \"$GO_E2E_PACKAGE_SELECTOR\"",
	}
	for _, signal := range dispatchSignals {
		if !strings.Contains(dispatch, signal) {
			return fmt.Errorf("smackerel.sh missing assistant package contract signal %q", signal)
		}
	}

	wrapperSignals := []string{
		"go_package_selector=\"\"",
		"unsupported --package value",
		"go_test_packages=(./tests/e2e/assistant)",
		"go_test_args+=(\"${go_test_packages[@]}\")",
	}
	for _, signal := range wrapperSignals {
		if !strings.Contains(wrapper, signal) {
			return fmt.Errorf("go-e2e.sh missing assistant package contract signal %q", signal)
		}
	}
	return nil
}

func TestAssistantE2EPackageContract_LiveRunnerTargetsOnlyAssistant(t *testing.T) {
	root := envsubstWrapperRepoRoot(t)
	dispatch, err := os.ReadFile(filepath.Join(root, "smackerel.sh"))
	if err != nil {
		t.Fatalf("read smackerel.sh: %v", err)
	}
	wrapper, err := os.ReadFile(filepath.Join(root, "scripts", "runtime", "go-e2e.sh"))
	if err != nil {
		t.Fatalf("read go-e2e.sh: %v", err)
	}
	if err := assertAssistantE2EPackageContract(string(dispatch), string(wrapper)); err != nil {
		t.Fatal(err)
	}
}

func TestAssistantE2EPackageContract_AdversarialRejectsAllPackageFallback(t *testing.T) {
	const dispatch = `test e2e [--go-package assistant]
GO_E2E_PACKAGE_SELECTOR=""
unsupported --go-package value
go_e2e_args+=(--package "$GO_E2E_PACKAGE_SELECTOR")
if [[ -z "$GO_E2E_RUN_SELECTOR" && -z "$GO_E2E_PACKAGE_SELECTOR" ]]; then true; fi`
	const wrapper = `go_package_selector=""
unsupported --package value
go_test_packages=(./tests/e2e/...)
go_test_args+=("${go_test_packages[@]}")`
	if err := assertAssistantE2EPackageContract(dispatch, wrapper); err == nil {
		t.Fatal("contract accepted a wrapper that maps assistant selection to the all-package path")
	}
}

func TestAssistantE2EPackageContract_AdversarialRejectsShellSuiteExecution(t *testing.T) {
	const dispatch = `test e2e [--go-package assistant]
GO_E2E_PACKAGE_SELECTOR=""
unsupported --go-package value
go_e2e_args+=(--package "$GO_E2E_PACKAGE_SELECTOR")
if [[ -z "$GO_E2E_RUN_SELECTOR" ]]; then true; fi`
	const wrapper = `go_package_selector=""
unsupported --package value
go_test_packages=(./tests/e2e/assistant)
go_test_args+=("${go_test_packages[@]}")`
	if err := assertAssistantE2EPackageContract(dispatch, wrapper); err == nil {
		t.Fatal("contract accepted a dispatcher that still runs shell suites for assistant-only selection")
	}
}

func assertAssistantE2EPrerequisitesContract(wrapper, nodeHelper, metricsTest string) error {
	nodeSource := `source "$(dirname "${BASH_SOURCE[0]}")/_ensure_node.sh"`
	sourceIndex := strings.Index(wrapper, nodeSource)
	callIndex := strings.Index(wrapper, `ensure_node "go-e2e"`)
	workspaceIndex := strings.Index(wrapper, "cd /workspace")
	if sourceIndex < 0 || callIndex < 0 {
		return fmt.Errorf("go-e2e.sh must source _ensure_node.sh and call ensure_node")
	}
	if sourceIndex > callIndex || callIndex > workspaceIndex {
		return fmt.Errorf("go-e2e.sh must source and call ensure_node before entering the workspace")
	}
	if !strings.Contains(nodeHelper, "ensure_node()") ||
		!strings.Contains(nodeHelper, `[[ "$#" -ne 1 || -z "$1" ]]`) ||
		!strings.Contains(nodeHelper, "apt-get install -y --no-install-recommends nodejs") ||
		strings.Count(nodeHelper, "command -v node") < 2 {
		return fmt.Errorf("_ensure_node.sh must require its tag, install nodejs, and verify node before and after install")
	}
	if !strings.Contains(metricsTest, `os.Getenv("CORE_EXTERNAL_URL")`) ||
		!strings.Contains(metricsTest, `return baseURL + "/metrics"`) {
		return fmt.Errorf("refusal join E2E must derive /metrics from canonical CORE_EXTERNAL_URL")
	}
	if strings.Contains(metricsTest, "SMACKEREL_CORE_METRICS_URL") || strings.Contains(metricsTest, "t.Skip") {
		return fmt.Errorf("refusal join E2E contains a noncanonical metrics variable or silent skip path")
	}
	return nil
}

func TestAssistantE2EPrerequisitesContract_LiveSources(t *testing.T) {
	root := envsubstWrapperRepoRoot(t)
	wrapper, err := os.ReadFile(filepath.Join(root, "scripts", "runtime", "go-e2e.sh"))
	if err != nil {
		t.Fatalf("read go-e2e.sh: %v", err)
	}
	nodeHelper, err := os.ReadFile(filepath.Join(root, "scripts", "runtime", "_ensure_node.sh"))
	if err != nil {
		t.Fatalf("read _ensure_node.sh: %v", err)
	}
	metricsTest, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "assistant", "intent_refusal_join_e2e_test.go"))
	if err != nil {
		t.Fatalf("read intent_refusal_join_e2e_test.go: %v", err)
	}
	if err := assertAssistantE2EPrerequisitesContract(string(wrapper), string(nodeHelper), string(metricsTest)); err != nil {
		t.Fatal(err)
	}
}

func TestAssistantE2EPrerequisitesContract_AdversarialRejectsMissingNodeCall(t *testing.T) {
	root := envsubstWrapperRepoRoot(t)
	wrapper := readDeployContractFile(t, filepath.Join(root, "scripts", "runtime", "go-e2e.sh"))
	nodeHelper := readDeployContractFile(t, filepath.Join(root, "scripts", "runtime", "_ensure_node.sh"))
	metricsTest := readDeployContractFile(t, filepath.Join(root, "tests", "e2e", "assistant", "intent_refusal_join_e2e_test.go"))
	broken := strings.Replace(wrapper, `ensure_node "go-e2e"`, "", 1)
	if err := assertAssistantE2EPrerequisitesContract(broken, nodeHelper, metricsTest); err == nil {
		t.Fatal("contract accepted an E2E wrapper that no longer invokes the Node bootstrap")
	}
}

func TestAssistantE2EPrerequisitesContract_AdversarialRejectsUnverifiedNodeInstall(t *testing.T) {
	root := envsubstWrapperRepoRoot(t)
	wrapper := readDeployContractFile(t, filepath.Join(root, "scripts", "runtime", "go-e2e.sh"))
	nodeHelper := readDeployContractFile(t, filepath.Join(root, "scripts", "runtime", "_ensure_node.sh"))
	metricsTest := readDeployContractFile(t, filepath.Join(root, "tests", "e2e", "assistant", "intent_refusal_join_e2e_test.go"))
	lastCheck := strings.LastIndex(nodeHelper, "command -v node")
	if lastCheck < 0 {
		t.Fatal("test fixture has no node verification to remove")
	}
	broken := nodeHelper[:lastCheck] + "node-check-removed" + nodeHelper[lastCheck+len("command -v node"):]
	if err := assertAssistantE2EPrerequisitesContract(wrapper, broken, metricsTest); err == nil {
		t.Fatal("contract accepted a Node install with no post-install executable verification")
	}
}

func TestAssistantE2EPrerequisitesContract_AdversarialRejectsMetricsSkip(t *testing.T) {
	root := envsubstWrapperRepoRoot(t)
	wrapper := readDeployContractFile(t, filepath.Join(root, "scripts", "runtime", "go-e2e.sh"))
	nodeHelper := readDeployContractFile(t, filepath.Join(root, "scripts", "runtime", "_ensure_node.sh"))
	metricsTest := readDeployContractFile(t, filepath.Join(root, "tests", "e2e", "assistant", "intent_refusal_join_e2e_test.go"))
	broken := strings.Replace(metricsTest, "t.Fatal(\"e2e: CORE_EXTERNAL_URL", "t.Skip(\"e2e: CORE_EXTERNAL_URL", 1)
	if err := assertAssistantE2EPrerequisitesContract(wrapper, nodeHelper, broken); err == nil {
		t.Fatal("contract accepted a silent skip when canonical metrics wiring is absent")
	}
}

func readDeployContractFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}
