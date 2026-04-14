package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot returns the absolute path to the repository root.
func repoRoot(t *testing.T) string {
	t.Helper()
	// internal/config/ → repo root is two levels up
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	return filepath.Join(wd, "..", "..")
}

func readRepoFile(t *testing.T, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), relPath))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	return string(data)
}

// --- SCN-020-001: All Docker host-forwarded ports bind to 127.0.0.1 ---

func TestDockerCompose_AllPortsBindLocalhost(t *testing.T) {
	content := readRepoFile(t, "docker-compose.yml")

	// Match all port mapping lines (YAML list items under ports:).
	// Valid format: - "127.0.0.1:${...}:${...}"
	portLine := regexp.MustCompile(`(?m)^\s+-\s+"([^"]+)"`)
	matches := portLine.FindAllStringSubmatch(content, -1)

	if len(matches) == 0 {
		t.Fatal("no port mappings found in docker-compose.yml")
	}

	for _, m := range matches {
		mapping := m[1]
		if !strings.HasPrefix(mapping, "127.0.0.1:") {
			t.Errorf("port mapping %q does not bind to 127.0.0.1", mapping)
		}
	}
}

// --- Docker Compose: all services have no-new-privileges ---

func TestDockerCompose_NoNewPrivileges(t *testing.T) {
	content := readRepoFile(t, "docker-compose.yml")

	// Each service definition that has a build: or image: should also have
	// security_opt: - no-new-privileges:true
	services := []string{"postgres", "nats", "smackerel-core", "smackerel-ml", "ollama"}

	for _, svc := range services {
		t.Run(svc, func(t *testing.T) {
			// Find the service block and check for no-new-privileges
			idx := strings.Index(content, "  "+svc+":")
			if idx == -1 {
				t.Fatalf("service %s not found in docker-compose.yml", svc)
			}

			// Get the service block text (up to next top-level service or EOF)
			rest := content[idx:]
			svcHeader := "  " + svc + ":"
			afterHeader := rest[len(svcHeader):]
			sections := regexp.MustCompile(`\n  \S`)
			svcBlock := rest
			if loc := sections.FindStringIndex(afterHeader); loc != nil {
				svcBlock = rest[:len(svcHeader)+loc[0]]
			}

			if !strings.Contains(svcBlock, "no-new-privileges:true") {
				t.Errorf("service %s missing security_opt: no-new-privileges:true", svc)
			}
		})
	}
}

// --- SCN-020-003: NATS uses config file, not --auth CLI arg ---

func TestDockerCompose_NATSUsesConfigFile(t *testing.T) {
	content := readRepoFile(t, "docker-compose.yml")

	if strings.Contains(content, "--auth") {
		t.Error("docker-compose.yml still contains --auth flag; NATS should use config file")
	}

	if !strings.Contains(content, "--config") {
		t.Error("docker-compose.yml missing --config flag for NATS")
	}

	if !strings.Contains(content, "nats.conf:/etc/nats/nats.conf:ro") {
		t.Error("docker-compose.yml missing nats.conf volume mount")
	}
}

// --- Dockerfiles: non-root USER directive ---

func TestDockerfile_CoreRunsAsNonRoot(t *testing.T) {
	content := readRepoFile(t, "Dockerfile")

	if !strings.Contains(content, "USER smackerel") {
		t.Error("Dockerfile missing USER smackerel — container runs as root")
	}

	// Ensure USER comes after COPY (runtime stage, not build stage)
	userIdx := strings.LastIndex(content, "USER smackerel")
	copyIdx := strings.LastIndex(content, "COPY --from=builder")
	if copyIdx == -1 || userIdx < copyIdx {
		t.Error("USER directive should come after COPY --from=builder in runtime stage")
	}
}

func TestDockerfile_MLRunsAsNonRoot(t *testing.T) {
	content := readRepoFile(t, "ml/Dockerfile")

	if !strings.Contains(content, "USER smackerel") {
		t.Error("ML Dockerfile missing USER smackerel — container runs as root")
	}
}

// --- .dockerignore: sensitive files excluded ---

func TestDockerignore_ExcludesSecrets(t *testing.T) {
	content := readRepoFile(t, ".dockerignore")

	required := []string{
		"config/generated/",
		".git",
		"specs/",
		"tests/",
	}

	for _, entry := range required {
		if !strings.Contains(content, entry) {
			t.Errorf(".dockerignore missing exclusion for %q", entry)
		}
	}
}

func TestDockerignore_ML_ExcludesTests(t *testing.T) {
	content := readRepoFile(t, "ml/.dockerignore")

	if !strings.Contains(content, "tests/") {
		t.Error("ml/.dockerignore missing exclusion for tests/")
	}
	if !strings.Contains(content, "__pycache__/") {
		t.Error("ml/.dockerignore missing exclusion for __pycache__/")
	}
}

// --- SEC-SWEEP-002: Application containers drop all Linux capabilities ---

func TestDockerCompose_CapDropAll(t *testing.T) {
	content := readRepoFile(t, "docker-compose.yml")

	// Application services that must have cap_drop: ALL
	services := []string{"nats", "smackerel-core", "smackerel-ml"}

	for _, svc := range services {
		t.Run(svc, func(t *testing.T) {
			// Find the service block
			idx := strings.Index(content, "  "+svc+":")
			if idx == -1 {
				t.Fatalf("service %s not found in docker-compose.yml", svc)
			}

			// Get the service block text (up to next top-level service or EOF)
			rest := content[idx:]
			svcHeader := "  " + svc + ":"
			afterHeader := rest[len(svcHeader):]
			sections := regexp.MustCompile(`\n  \S`)
			svcBlock := rest
			if loc := sections.FindStringIndex(afterHeader); loc != nil {
				svcBlock = rest[:len(svcHeader)+loc[0]]
			}

			if !strings.Contains(svcBlock, "cap_drop:") || !strings.Contains(svcBlock, "- ALL") {
				t.Errorf("service %s missing cap_drop: [ALL]", svc)
			}
		})
	}
}
