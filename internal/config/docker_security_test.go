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

// --- IMP-020-002: NATS config file quotes token value to prevent metachar injection ---

func TestNATSConfTemplate_TokenIsQuoted(t *testing.T) {
	// Read the config generation script and verify the NATS token
	// is written inside double-quotes. Without quoting, a token containing
	// NATS config metacharacters (}, {, #, whitespace) breaks config parsing.
	content := readRepoFile(t, "scripts/commands/config.sh")

	// The token should appear inside double quotes in the NATS auth section
	if !strings.Contains(content, `token: \"`) {
		t.Error("NATS config template does not quote the auth token — metacharacters will break config parsing (IMP-020-002)")
	}
}

// --- GAP-020-R30-001: NATS config generator escapes " and \ inside token value ---

func TestNATSConfGenerator_EscapesSpecialCharsInToken(t *testing.T) {
	// The config generator must escape backslash and double-quote inside
	// the token BEFORE interpolating into the NATS quoted string. Without
	// this, a token like abc"def produces broken NATS config:
	//   token: "abc"def"  → syntax error or config injection (CWE-74)
	content := readRepoFile(t, "scripts/commands/config.sh")

	// Verify backslash-then-double-quote escaping is present
	// The script must contain substitution for \ → \\ BEFORE " → \"
	if !strings.Contains(content, `ESCAPED_NATS_TOKEN`) {
		t.Fatal("NATS config generator does not escape special characters in the auth token (GAP-020-R30-001)")
	}

	// Verify backslash escaping happens first (order matters: \ before ")
	backslashEsc := strings.Index(content, `//\\/\\\\`)
	quoteEsc := strings.Index(content, `//\"/\\\"`)
	if backslashEsc == -1 {
		t.Error("NATS config generator does not escape backslash in auth token")
	}
	if quoteEsc == -1 {
		t.Error("NATS config generator does not escape double-quote in auth token")
	}
	if backslashEsc != -1 && quoteEsc != -1 && backslashEsc > quoteEsc {
		t.Error("NATS config generator escapes \" before \\ — order must be \\ first to avoid double-escaping")
	}
}

func TestNATSConf_GeneratedFile_TokenProperlyQuoted(t *testing.T) {
	// Verify the generated nats.conf file (if it exists) has syntactically
	// valid token quoting — no unescaped " inside the token value.
	path := "config/generated/nats.conf"
	content := readRepoFile(t, path)

	// Find the token line
	tokenLine := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "token:") {
			tokenLine = trimmed
			break
		}
	}
	if tokenLine == "" {
		t.Skip("no token line in nats.conf (auth may be disabled)")
	}

	// The token value must be wrapped in exactly one pair of double quotes.
	// Extract everything after "token: "
	val := strings.TrimPrefix(tokenLine, "token:")
	val = strings.TrimSpace(val)
	if !strings.HasPrefix(val, `"`) || !strings.HasSuffix(val, `"`) {
		t.Errorf("NATS token value is not double-quoted: %s", val)
	}

	// Inside the quotes, count unescaped double-quotes — there should be zero.
	inner := val[1 : len(val)-1]
	for i := 0; i < len(inner); i++ {
		if inner[i] == '"' && (i == 0 || inner[i-1] != '\\') {
			t.Errorf("NATS token value contains unescaped double-quote at position %d: %s", i, inner)
		}
	}
}

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

// --- DEV-015-001: All connector env vars read by main.go are wired through docker-compose.yml ---

func TestDockerCompose_ConnectorEnvVarsWired(t *testing.T) {
	compose := readRepoFile(t, "docker-compose.yml")

	// Every env var that main.go reads via os.Getenv for auto-starting connectors
	// MUST appear in the smackerel-core environment section of docker-compose.yml.
	// If missing, the connector silently never starts in Docker deployments.
	requiredEnvVars := []struct {
		envVar    string
		connector string
	}{
		// Twitter connector (DEV-015-001)
		{"TWITTER_ENABLED", "twitter"},
		{"TWITTER_SYNC_MODE", "twitter"},
		{"TWITTER_ARCHIVE_DIR", "twitter"},
		{"TWITTER_BEARER_TOKEN", "twitter"},
		{"TWITTER_SYNC_SCHEDULE", "twitter"},
		// Discord connector
		{"DISCORD_ENABLED", "discord"},
		{"DISCORD_BOT_TOKEN", "discord"},
		{"DISCORD_SYNC_SCHEDULE", "discord"},
		{"DISCORD_ENABLE_GATEWAY", "discord"},
		{"DISCORD_BACKFILL_LIMIT", "discord"},
		{"DISCORD_INCLUDE_THREADS", "discord"},
		{"DISCORD_INCLUDE_PINS", "discord"},
		{"DISCORD_CAPTURE_COMMANDS", "discord"},
		{"DISCORD_MONITORED_CHANNELS", "discord"},
		// Weather connector
		{"WEATHER_ENABLED", "weather"},
		{"WEATHER_SYNC_SCHEDULE", "weather"},
		{"WEATHER_LOCATIONS", "weather"},
		// Gov Alerts connector
		{"GOV_ALERTS_ENABLED", "gov-alerts"},
		{"GOV_ALERTS_SYNC_SCHEDULE", "gov-alerts"},
		{"GOV_ALERTS_MIN_EARTHQUAKE_MAG", "gov-alerts"},
		{"GOV_ALERTS_SOURCE_WEATHER", "gov-alerts"},
		{"GOV_ALERTS_SOURCE_TSUNAMI", "gov-alerts"},
		{"GOV_ALERTS_SOURCE_VOLCANO", "gov-alerts"},
		{"GOV_ALERTS_SOURCE_WILDFIRE", "gov-alerts"},
		{"GOV_ALERTS_SOURCE_AIRNOW", "gov-alerts"},
		{"GOV_ALERTS_SOURCE_GDACS", "gov-alerts"},
		{"GOV_ALERTS_AIRNOW_API_KEY", "gov-alerts"},
		{"GOV_ALERTS_LOCATIONS", "gov-alerts"},
		{"GOV_ALERTS_TRAVEL_LOCATIONS", "gov-alerts"},
	}

	for _, tc := range requiredEnvVars {
		t.Run(tc.envVar, func(t *testing.T) {
			if !strings.Contains(compose, tc.envVar) {
				t.Errorf("docker-compose.yml missing env var %s for %s connector — connector will silently never start in Docker",
					tc.envVar, tc.connector)
			}
		})
	}
}

// --- DEV-015-002: File-import connectors have volume mounts in docker-compose.yml ---

func TestDockerCompose_ImportVolumesMounted(t *testing.T) {
	compose := readRepoFile(t, "docker-compose.yml")

	// Every file-import connector needs a corresponding volume mount so the
	// host archive directory is accessible inside the container.
	requiredMounts := []struct {
		containerPath string
		connector     string
	}{
		{"/data/bookmarks-import", "bookmarks"},
		{"/data/maps-import", "maps"},
		{"/data/browser-history/History", "browser-history"},
		{"/data/twitter-archive", "twitter"},
	}

	for _, tc := range requiredMounts {
		t.Run(tc.connector, func(t *testing.T) {
			if !strings.Contains(compose, tc.containerPath) {
				t.Errorf("docker-compose.yml missing volume mount for %s connector (container path %s)",
					tc.connector, tc.containerPath)
			}
		})
	}
}
