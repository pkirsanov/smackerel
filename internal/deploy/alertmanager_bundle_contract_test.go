// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy spec 102 SCOPE-102-02 — durable Prometheus -> Alertmanager ->
// ntfy routing bundle contract.
//
// These tests prove the alerting-delivery path is folded INTO the product
// bundle so it SURVIVES every apply with zero manual re-run (retiring the knb
// post-apply alertmanager-standup.sh). They generate a REAL config bundle via
// scripts/commands/config.sh (reusing the bundle_secret_contract_test.go
// harness), extract it, and assert:
//
//	SCN-102-C2-01  the rendered prometheus.yml carries an alerting: block
//	               targeting alertmanager:9093, the compose declares the
//	               alertmanager service under profiles:[monitoring], and the
//	               bundle contains alertmanager.yml + the rendered url_file.
//	SCN-102-C2-02  a re-generated/re-extracted bundle STILL carries the block +
//	               service (no host-side standup needed).
//	SCN-102-C2-04  the product alertmanager.yml uses a url_file (NOT a url) and
//	               carries NO operator ntfy host/topic literal; the real
//	               endpoint is adapter-injected.
//
// Adversarial sub-tests prove each assertion function has bite (a dropped
// alerting block / dropped service / inlined ntfy literal is rejected), so a
// regression cannot silently re-open the R-082-C gap.
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The three-way parity anchor: the alerting target port MUST equal the SST
// monitoring.alertmanager.container_port AND the compose alertmanager
// --web.listen-address port. 9093 is Alertmanager's fixed upstream API port.
const alertmanagerTarget = "alertmanager:9093"

// -----------------------------------------------------------------------------
// Assertion functions (pure — take parsed/raw content so adversarial sub-tests
// can feed corrupted inputs and prove the checks reject them).
// -----------------------------------------------------------------------------

// assertPrometheusHasAlerting returns nil iff the rendered prometheus.yml has an
// alerting.alertmanagers[].static_configs[].targets entry equal to wantTarget.
func assertPrometheusHasAlerting(promYAML []byte, wantTarget string) error {
	var doc struct {
		Alerting struct {
			Alertmanagers []struct {
				StaticConfigs []struct {
					Targets []string `yaml:"targets"`
				} `yaml:"static_configs"`
			} `yaml:"alertmanagers"`
		} `yaml:"alerting"`
	}
	if err := yaml.Unmarshal(promYAML, &doc); err != nil {
		return fmt.Errorf("prometheus.yml does not parse as YAML: %w", err)
	}
	if len(doc.Alerting.Alertmanagers) == 0 {
		return fmt.Errorf("contract violation: prometheus.yml has NO alerting.alertmanagers block — the 21 alert rules would fire into a void (R-082-C). The static alerting block MUST be present in config/prometheus/prometheus.yml.tmpl so every bundle carries it")
	}
	for _, am := range doc.Alerting.Alertmanagers {
		for _, sc := range am.StaticConfigs {
			for _, tgt := range sc.Targets {
				if tgt == wantTarget {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("contract violation: prometheus.yml has an alerting block but no static target %q — Prometheus would not reach the in-stack Alertmanager", wantTarget)
}

// composeAMDoc is the minimal compose shape needed to assert the alertmanager +
// bridge services and their profile gating.
type composeAMServiceDoc struct {
	Image    string   `yaml:"image"`
	Profiles []string `yaml:"profiles"`
	Command  []string `yaml:"command"`
	Networks []string `yaml:"networks"`
}

type composeAMDoc struct {
	Services map[string]composeAMServiceDoc `yaml:"services"`
	Networks map[string]struct{}            `yaml:"networks"`
}

func hasProfile(profiles []string, want string) bool {
	for _, p := range profiles {
		if p == want {
			return true
		}
	}
	return false
}

// assertComposeHasAlertmanagerServices returns nil iff docker-compose.yml
// declares BOTH the alertmanager and alertmanager-ntfy-bridge services under
// profiles:[monitoring], the alertmanager image is the SST-substituted
// ${ALERTMANAGER_IMAGE...} form, and its --web.listen-address is SST-driven.
func assertComposeHasAlertmanagerServices(composeYAML []byte) error {
	var doc composeAMDoc
	if err := yaml.Unmarshal(composeYAML, &doc); err != nil {
		return fmt.Errorf("docker-compose.yml does not parse as YAML: %w", err)
	}

	am, ok := doc.Services["alertmanager"]
	if !ok {
		return fmt.Errorf("contract violation: docker-compose.yml declares NO `alertmanager` service — alerting has no in-stack router (R-082-C). The service MUST be folded into deploy/compose.deploy.yml under profiles:[monitoring]")
	}
	if !hasProfile(am.Profiles, "monitoring") {
		return fmt.Errorf("contract violation: the alertmanager service is not gated by profiles:[monitoring] (profiles=%v) — it MUST be off by default like prometheus", am.Profiles)
	}
	if !strings.Contains(am.Image, "${ALERTMANAGER_IMAGE") {
		return fmt.Errorf("contract violation: the alertmanager service image=%q is not the SST-substituted ${ALERTMANAGER_IMAGE...} form (digest pin lives in config/smackerel.yaml + deploy/contract.yaml)", am.Image)
	}
	sawSSTListen := false
	for _, arg := range am.Command {
		if strings.Contains(arg, "--web.listen-address") && strings.Contains(arg, "${ALERTMANAGER_CONTAINER_PORT}") {
			sawSSTListen = true
		}
	}
	if !sawSSTListen {
		return fmt.Errorf("contract violation: the alertmanager service --web.listen-address is not SST-driven via ${ALERTMANAGER_CONTAINER_PORT} (command=%v) — the port MUST come from monitoring.alertmanager.container_port, not a literal", am.Command)
	}

	bridge, ok := doc.Services["alertmanager-ntfy-bridge"]
	if !ok {
		return fmt.Errorf("contract violation: docker-compose.yml declares NO `alertmanager-ntfy-bridge` service — without the templating bridge, ntfy would receive the raw Alertmanager JSON instead of titled/priority messages (SCN-102-C2-03)")
	}
	if !hasProfile(bridge.Profiles, "monitoring") {
		return fmt.Errorf("contract violation: the alertmanager-ntfy-bridge service is not gated by profiles:[monitoring] (profiles=%v)", bridge.Profiles)
	}
	if !strings.Contains(bridge.Image, "${SMACKEREL_CORE_IMAGE") {
		return fmt.Errorf("contract violation: the bridge image=%q is not the ${SMACKEREL_CORE_IMAGE...} form — it MUST ride the already-pinned core image (no new external image to pin/sign)", bridge.Image)
	}
	return nil
}

func assertMonitoringIngressIsolatedFromML(composeYAML []byte) error {
	var doc composeAMDoc
	if err := yaml.Unmarshal(composeYAML, &doc); err != nil {
		return fmt.Errorf("docker-compose.yml does not parse as YAML: %w", err)
	}
	if _, ok := doc.Networks["monitoring-tier"]; !ok {
		return fmt.Errorf("contract violation: root networks has no monitoring-tier — unauthenticated monitoring ingress cannot be isolated from the ML compute tier")
	}

	requireNetworks := func(service string, want ...string) error {
		got, ok := doc.Services[service]
		if !ok {
			return fmt.Errorf("contract violation: required service %q is absent", service)
		}
		if len(got.Networks) != len(want) {
			return fmt.Errorf("contract violation: service %q networks=%v, want exactly %v", service, got.Networks, want)
		}
		for _, wantNetwork := range want {
			found := false
			for _, gotNetwork := range got.Networks {
				if gotNetwork == wantNetwork {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("contract violation: service %q networks=%v, missing required %q", service, got.Networks, wantNetwork)
			}
		}
		return nil
	}

	for service, want := range map[string][]string{
		"smackerel-ml":             {"compute-tier"},
		"prometheus":               {"compute-tier", "monitoring-tier"},
		"alertmanager":             {"monitoring-tier"},
		"alertmanager-ntfy-bridge": {"monitoring-tier"},
	} {
		if err := requireNetworks(service, want...); err != nil {
			return err
		}
	}
	return nil
}

// forbiddenNtfyLiterals are operator-private / env-specific ntfy tokens that
// MUST NEVER appear in the product-owned alertmanager.yml (they belong to the
// retired knb overlay). "No env-specific content" (SCN-102-C2-04).
var forbiddenNtfyLiterals = []string{"self-hosted-ntfy", "self-hosted-alerts", "18090"}

// assertAlertmanagerConfigNoLiteral returns nil iff the product alertmanager.yml
// routes via url_file (NOT an inline url) and carries NO operator ntfy literal.
func assertAlertmanagerConfigNoLiteral(amYAML []byte) error {
	var doc struct {
		Receivers []struct {
			Name           string `yaml:"name"`
			WebhookConfigs []struct {
				URL     string `yaml:"url"`
				URLFile string `yaml:"url_file"`
			} `yaml:"webhook_configs"`
		} `yaml:"receivers"`
	}
	if err := yaml.Unmarshal(amYAML, &doc); err != nil {
		return fmt.Errorf("alertmanager.yml does not parse as YAML: %w", err)
	}
	if len(doc.Receivers) == 0 {
		return fmt.Errorf("contract violation: alertmanager.yml declares no receivers")
	}
	sawURLFile := false
	for _, rcv := range doc.Receivers {
		for _, wc := range rcv.WebhookConfigs {
			if strings.TrimSpace(wc.URL) != "" {
				return fmt.Errorf("contract violation: alertmanager.yml receiver %q uses an inline `url: %s` — the product repo MUST use `url_file` so it carries no endpoint literal (SCN-102-C2-04). The real endpoint is adapter-injected", rcv.Name, wc.URL)
			}
			if strings.TrimSpace(wc.URLFile) != "" {
				sawURLFile = true
			}
		}
	}
	if !sawURLFile {
		return fmt.Errorf("contract violation: alertmanager.yml has no webhook_configs.url_file — the routing target MUST come from a mounted url_file (SCN-102-C2-04)")
	}
	// Belt-and-braces: no operator ntfy token anywhere in the raw text.
	lc := strings.ToLower(string(amYAML))
	for _, tok := range forbiddenNtfyLiterals {
		if strings.Contains(lc, strings.ToLower(tok)) {
			return fmt.Errorf("contract violation: alertmanager.yml contains the operator-private ntfy literal %q — env-specific content MUST NOT appear in the product repo (SCN-102-C2-04)", tok)
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// SCN-102-C2-01 — the generated bundle carries the alerting block + services.
// -----------------------------------------------------------------------------

func TestBundle_CarriesAlertingBlockAndService_Spec102(t *testing.T) {
	tmpRoot := setupTestRepoRoot(t, nil, nil)
	outputDir := filepath.Join(t.TempDir(), "bundle-out")
	out, exit := runConfigGenerate(t, tmpRoot, "self-hosted", outputDir)
	if exit != 0 {
		t.Fatalf("loader exited %d (expected 0)\n--- output ---\n%s\n--- end ---", exit, out)
	}
	files := extractTarGz(t, bundlePath(outputDir))

	prom, ok := files["prometheus.yml"]
	if !ok {
		t.Fatal("bundle missing prometheus.yml")
	}
	if err := assertPrometheusHasAlerting(prom, alertmanagerTarget); err != nil {
		t.Fatalf("SCN-102-C2-01: %v", err)
	}

	compose, ok := files["docker-compose.yml"]
	if !ok {
		t.Fatal("bundle missing docker-compose.yml")
	}
	if err := assertComposeHasAlertmanagerServices(compose); err != nil {
		t.Fatalf("SCN-102-C2-01: %v", err)
	}

	if _, ok := files["alertmanager.yml"]; !ok {
		t.Fatal("SCN-102-C2-01: bundle missing alertmanager.yml (the routing config MUST be staged into the bundle)")
	}
	urlFile, ok := files["alertmanager_ntfy_url"]
	if !ok {
		t.Fatal("SCN-102-C2-01: bundle missing rendered alertmanager_ntfy_url")
	}
	// The rendered url_file target is the GENERIC in-stack bridge (a compose
	// service name), never a real operator host.
	if !strings.Contains(string(urlFile), "alertmanager-ntfy-bridge") {
		t.Fatalf("SCN-102-C2-01: alertmanager_ntfy_url=%q does not point at the in-stack bridge", strings.TrimSpace(string(urlFile)))
	}
	for _, tok := range forbiddenNtfyLiterals {
		if strings.Contains(strings.ToLower(string(urlFile)), strings.ToLower(tok)) {
			t.Fatalf("SCN-102-C2-01: rendered alertmanager_ntfy_url leaks operator ntfy literal %q", tok)
		}
	}

	// Three-way port parity: SST container_port == the alerting target port.
	assertSSTAlertmanagerPortParity(t)

	t.Logf("SCN-102-C2-01 OK — bundle carries: prometheus.yml alerting -> %s; alertmanager + alertmanager-ntfy-bridge services (profiles:[monitoring]); alertmanager.yml + generic bridge url_file", alertmanagerTarget)
}

func TestCompose_MonitoringIngressIsolatedFromML_Spec102(t *testing.T) {
	liveCompose, err := os.ReadFile(filepath.Join(repoRoot(t), "deploy", "compose.deploy.yml"))
	if err != nil {
		t.Fatalf("read live deploy compose: %v", err)
	}
	if err := assertMonitoringIngressIsolatedFromML(liveCompose); err != nil {
		t.Fatalf("SECURITY F2: live deploy compose: %v", err)
	}

	tmpRoot := setupTestRepoRoot(t, nil, nil)
	outputDir := filepath.Join(t.TempDir(), "bundle-out")
	if out, exit := runConfigGenerate(t, tmpRoot, "self-hosted", outputDir); exit != 0 {
		t.Fatalf("loader exited %d\n--- output ---\n%s\n--- end ---", exit, out)
	}
	files := extractTarGz(t, bundlePath(outputDir))
	if err := assertMonitoringIngressIsolatedFromML(files["docker-compose.yml"]); err != nil {
		t.Fatalf("SECURITY F2: bundled deploy compose: %v", err)
	}

	oldSharedTier := []byte(`
services:
  smackerel-ml:
    networks: [compute-tier]
  prometheus:
    networks: [compute-tier]
  alertmanager:
    networks: [compute-tier]
  alertmanager-ntfy-bridge:
    networks: [compute-tier]
networks:
  data-tier: {}
  compute-tier: {}
`)
	if err := assertMonitoringIngressIsolatedFromML(oldSharedTier); err == nil {
		t.Fatal("adversarial FAILED: the old topology sharing compute-tier between ML and the unauthenticated bridge was accepted")
	} else {
		t.Logf("adversarial OK — shared ML/monitoring ingress tier rejected: %v", err)
	}
}

// assertSSTAlertmanagerPortParity reads the LIVE config/smackerel.yaml and
// asserts monitoring.alertmanager.container_port == the alerting target port.
func assertSSTAlertmanagerPortParity(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "config", "smackerel.yaml"))
	if err != nil {
		t.Fatalf("read live smackerel.yaml: %v", err)
	}
	var doc struct {
		Monitoring struct {
			Alertmanager struct {
				ContainerPort int `yaml:"container_port"`
			} `yaml:"alertmanager"`
		} `yaml:"monitoring"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse smackerel.yaml: %v", err)
	}
	want := fmt.Sprintf("alertmanager:%d", doc.Monitoring.Alertmanager.ContainerPort)
	if want != alertmanagerTarget {
		t.Fatalf("port parity violation: config/smackerel.yaml monitoring.alertmanager.container_port=%d yields target %q, but prometheus.yml.tmpl targets %q — the SST port and the alerting target MUST match (drift lock)", doc.Monitoring.Alertmanager.ContainerPort, want, alertmanagerTarget)
	}
}

// -----------------------------------------------------------------------------
// SCN-102-C2-02 — alerting survives a re-extract (no host-side standup).
// -----------------------------------------------------------------------------

func TestBundle_AlertingSurvivesReExtract_Spec102(t *testing.T) {
	tmpRoot := setupTestRepoRoot(t, nil, nil)

	// Generate the bundle TWICE into separate output dirs — this simulates a
	// re-apply, which re-extracts a freshly generated bundle. Both MUST carry
	// the alerting block + service; nothing depends on a host-side injection.
	for i, name := range []string{"first-apply", "second-apply"} {
		outputDir := filepath.Join(t.TempDir(), name)
		out, exit := runConfigGenerate(t, tmpRoot, "self-hosted", outputDir)
		if exit != 0 {
			t.Fatalf("[%d] loader exited %d\n--- output ---\n%s\n--- end ---", i, exit, out)
		}
		files := extractTarGz(t, bundlePath(outputDir))
		if err := assertPrometheusHasAlerting(files["prometheus.yml"], alertmanagerTarget); err != nil {
			t.Fatalf("SCN-102-C2-02 [%s]: alerting block dropped on re-extract: %v", name, err)
		}
		if err := assertComposeHasAlertmanagerServices(files["docker-compose.yml"]); err != nil {
			t.Fatalf("SCN-102-C2-02 [%s]: alertmanager service dropped on re-extract: %v", name, err)
		}
	}
	t.Log("SCN-102-C2-02 OK — two consecutive bundle extractions both carry the alerting block + alertmanager service (no post-apply standup required)")
}

// -----------------------------------------------------------------------------
// SCN-102-C2-04 — the product alertmanager.yml uses url_file, no ntfy literal.
// -----------------------------------------------------------------------------

func TestAlertmanagerConfig_NoNtfyLiteral_UsesUrlFile_Spec102(t *testing.T) {
	// Assert against BOTH the live committed file and the bundled copy.
	livePath := filepath.Join(repoRoot(t), "config", "prometheus", "alertmanager.yml")
	liveRaw, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read live alertmanager.yml: %v", err)
	}
	if err := assertAlertmanagerConfigNoLiteral(liveRaw); err != nil {
		t.Fatalf("SCN-102-C2-04 (live file): %v", err)
	}

	tmpRoot := setupTestRepoRoot(t, nil, nil)
	outputDir := filepath.Join(t.TempDir(), "bundle-out")
	if out, exit := runConfigGenerate(t, tmpRoot, "self-hosted", outputDir); exit != 0 {
		t.Fatalf("loader exited %d\n--- output ---\n%s\n--- end ---", exit, out)
	}
	files := extractTarGz(t, bundlePath(outputDir))
	bundled, ok := files["alertmanager.yml"]
	if !ok {
		t.Fatal("bundle missing alertmanager.yml")
	}
	if err := assertAlertmanagerConfigNoLiteral(bundled); err != nil {
		t.Fatalf("SCN-102-C2-04 (bundled copy): %v", err)
	}
	t.Log("SCN-102-C2-04 OK — alertmanager.yml routes via url_file with no operator ntfy literal (live + bundled)")
}

// -----------------------------------------------------------------------------
// Adversarial — each assertion function must reject the regression it guards.
// -----------------------------------------------------------------------------

func TestBundle_AdversarialMissingAlertingBlock_Spec102(t *testing.T) {
	// A prometheus.yml with rule_files + scrape_configs but NO alerting block —
	// exactly the pre-spec-102 state where alerts fired into a void.
	noAlerting := []byte(`
global:
  scrape_interval: 15s
rule_files:
  - /etc/prometheus/alerts.yml
scrape_configs:
  - job_name: smackerel-core
    static_configs:
      - targets: ["smackerel-core:8080"]
`)
	if err := assertPrometheusHasAlerting(noAlerting, alertmanagerTarget); err == nil {
		t.Fatal("adversarial FAILED: a prometheus.yml with NO alerting block was accepted (the contract is tautological — it would NOT catch the R-082-C regression)")
	} else {
		t.Logf("adversarial OK — missing alerting block rejected: %v", err)
	}
}

func TestBundle_AdversarialMissingAlertmanagerService_Spec102(t *testing.T) {
	// A compose with prometheus but NO alertmanager / bridge services.
	noAM := []byte(`
services:
  prometheus:
    image: ${PROMETHEUS_IMAGE}
    profiles: [monitoring]
`)
	if err := assertComposeHasAlertmanagerServices(noAM); err == nil {
		t.Fatal("adversarial FAILED: a compose with NO alertmanager service was accepted")
	} else {
		t.Logf("adversarial OK — missing alertmanager service rejected: %v", err)
	}

	// A compose with alertmanager but NO bridge (raw-JSON regression).
	noBridge := []byte(`
services:
  alertmanager:
    image: ${ALERTMANAGER_IMAGE:?x}
    profiles: [monitoring]
    command:
      - "--web.listen-address=:${ALERTMANAGER_CONTAINER_PORT}"
`)
	if err := assertComposeHasAlertmanagerServices(noBridge); err == nil {
		t.Fatal("adversarial FAILED: a compose with NO bridge service was accepted (ntfy would get raw JSON)")
	} else {
		t.Logf("adversarial OK — missing bridge service rejected: %v", err)
	}
}

func TestAlertmanagerConfig_AdversarialNtfyLiteral_Spec102(t *testing.T) {
	// The retired knb overlay form: an inline url pointing at the operator ntfy
	// endpoint. It MUST be rejected in the product repo.
	inlineLiteral := []byte(`
route:
  receiver: ntfy-self-hosted-alerts
receivers:
  - name: ntfy-self-hosted-alerts
    webhook_configs:
      - url: http://self-hosted-ntfy:8080/self-hosted-alerts
        send_resolved: true
`)
	if err := assertAlertmanagerConfigNoLiteral(inlineLiteral); err == nil {
		t.Fatal("adversarial FAILED: an inline ntfy url literal was accepted (No env-specific content would be violated silently)")
	} else {
		t.Logf("adversarial OK — inline ntfy literal rejected: %v", err)
	}
}
