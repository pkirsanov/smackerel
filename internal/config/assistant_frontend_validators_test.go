// Spec 073 SCOPE-1b TP-073-07 — web/mobile assistant frontend SST
// fail-loud test.
//
// SCN-073-A11 — when any of the eight web/mobile assistant SST keys
// is unset or empty, the core process MUST fail startup with a
// NO-DEFAULTS error naming the missing key. Mirrors the loader test
// pattern used for spec 074 (capture_fallback) and spec 075
// (legacy_retirement).
package config

import (
	"reflect"
	"strings"
	"testing"
)

// setAllWebAssistantValid pre-populates a valid baseline for the three
// web.assistant.* keys. Individual sub-tests then mutate one key to
// the unset/empty/invalid form under test.
func setAllWebAssistantValid(t *testing.T) {
	t.Helper()
	t.Setenv("WEB_ASSISTANT_ENABLED", "true")
	t.Setenv("WEB_ASSISTANT_BACKEND_BASE_URL", WebBackendSameOriginMarker)
	t.Setenv("WEB_ASSISTANT_SCHEMA_VERSION", FrontendSchemaVersionV1)
}

// setAllMobileAssistantValid pre-populates a valid baseline for the
// five mobile.assistant.* keys.
func setAllMobileAssistantValid(t *testing.T) {
	t.Helper()
	t.Setenv("MOBILE_ASSISTANT_ENABLED", "true")
	t.Setenv("MOBILE_ASSISTANT_BACKEND_BASE_URL", "https://assistant.invalid.example/")
	t.Setenv("MOBILE_ASSISTANT_SCHEMA_VERSION", FrontendSchemaVersionV1)
	t.Setenv("MOBILE_ASSISTANT_PLATFORMS", "ios,android")
	t.Setenv("MOBILE_ASSISTANT_AUTH_MODE", "bearer_token_per_user")
}

func TestLoadWebAssistant_HappyPath(t *testing.T) {
	setAllWebAssistantValid(t)
	cfg, err := LoadWebAssistant()
	if err != nil {
		t.Fatalf("LoadWebAssistant: %v", err)
	}
	if !cfg.Enabled || cfg.BackendBaseURL != WebBackendSameOriginMarker || cfg.SchemaVersion != FrontendSchemaVersionV1 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestLoadMobileAssistant_HappyPath(t *testing.T) {
	setAllWebAssistantValid(t)
	setAllMobileAssistantValid(t)
	cfg, err := LoadMobileAssistant()
	if err != nil {
		t.Fatalf("LoadMobileAssistant: %v", err)
	}
	if !reflect.DeepEqual(cfg.Platforms, []string{"ios", "android"}) {
		t.Fatalf("Platforms = %v, want [ios android]", cfg.Platforms)
	}
	if cfg.AuthMode != "bearer_token_per_user" || cfg.SchemaVersion != FrontendSchemaVersionV1 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

// TestWebAssistant_MissingKeysFailLoud_BS009 — one sub-test per web key
// proving fail-loud behavior when the key is unset OR empty.
func TestWebAssistant_MissingKeysFailLoud_BS009(t *testing.T) {
	keys := []string{
		"WEB_ASSISTANT_ENABLED",
		"WEB_ASSISTANT_BACKEND_BASE_URL",
		"WEB_ASSISTANT_SCHEMA_VERSION",
	}
	for _, key := range keys {
		key := key
		t.Run("unset/"+key, func(t *testing.T) {
			setAllWebAssistantValid(t)
			unsetEnvForTest(t, key)
			_, err := LoadWebAssistant()
			requireFailLoud(t, err, "[F073-SST-MISSING]", key)
		})
		t.Run("empty/"+key, func(t *testing.T) {
			setAllWebAssistantValid(t)
			t.Setenv(key, "")
			_, err := LoadWebAssistant()
			requireFailLoud(t, err, "[F073-SST-MISSING]", key)
		})
	}
}

// TestMobileAssistant_MissingKeysFailLoud_BS009 — one sub-test per
// mobile key proving fail-loud behavior when the key is unset OR empty.
func TestMobileAssistant_MissingKeysFailLoud_BS009(t *testing.T) {
	keys := []string{
		"MOBILE_ASSISTANT_ENABLED",
		"MOBILE_ASSISTANT_BACKEND_BASE_URL",
		"MOBILE_ASSISTANT_SCHEMA_VERSION",
		"MOBILE_ASSISTANT_PLATFORMS",
		"MOBILE_ASSISTANT_AUTH_MODE",
	}
	for _, key := range keys {
		key := key
		t.Run("unset/"+key, func(t *testing.T) {
			setAllMobileAssistantValid(t)
			unsetEnvForTest(t, key)
			_, err := LoadMobileAssistant()
			requireFailLoud(t, err, "[F073-SST-MISSING]", key)
		})
		t.Run("empty/"+key, func(t *testing.T) {
			setAllMobileAssistantValid(t)
			t.Setenv(key, "")
			_, err := LoadMobileAssistant()
			requireFailLoud(t, err, "[F073-SST-MISSING]", key)
		})
	}
}

func TestWebAssistant_InvalidValuesRejected(t *testing.T) {
	t.Run("schema_version drift", func(t *testing.T) {
		setAllWebAssistantValid(t)
		t.Setenv("WEB_ASSISTANT_SCHEMA_VERSION", "v2")
		_, err := LoadWebAssistant()
		requireFailLoud(t, err, "[F073-SST-INVALID]", "schema_version")
	})
	t.Run("invalid backend url", func(t *testing.T) {
		setAllWebAssistantValid(t)
		t.Setenv("WEB_ASSISTANT_BACKEND_BASE_URL", "not a url")
		_, err := LoadWebAssistant()
		requireFailLoud(t, err, "[F073-SST-INVALID]", "backend_base_url")
	})
	t.Run("explicit https url accepted", func(t *testing.T) {
		setAllWebAssistantValid(t)
		t.Setenv("WEB_ASSISTANT_BACKEND_BASE_URL", "https://assistant.example.com/")
		if _, err := LoadWebAssistant(); err != nil {
			t.Fatalf("https URL rejected: %v", err)
		}
	})
}

func TestMobileAssistant_InvalidValuesRejected(t *testing.T) {
	t.Run("schema_version drift", func(t *testing.T) {
		setAllMobileAssistantValid(t)
		t.Setenv("MOBILE_ASSISTANT_SCHEMA_VERSION", "v2")
		_, err := LoadMobileAssistant()
		requireFailLoud(t, err, "[F073-SST-INVALID]", "schema_version")
	})
	t.Run("http rejected (https required)", func(t *testing.T) {
		setAllMobileAssistantValid(t)
		t.Setenv("MOBILE_ASSISTANT_BACKEND_BASE_URL", "http://assistant.example.com/")
		_, err := LoadMobileAssistant()
		requireFailLoud(t, err, "[F073-SST-INVALID]", "backend_base_url")
	})
	t.Run("platforms missing android", func(t *testing.T) {
		setAllMobileAssistantValid(t)
		t.Setenv("MOBILE_ASSISTANT_PLATFORMS", "ios")
		_, err := LoadMobileAssistant()
		requireFailLoud(t, err, "[F073-SST-INVALID]", "platforms")
	})
	t.Run("platforms missing ios", func(t *testing.T) {
		setAllMobileAssistantValid(t)
		t.Setenv("MOBILE_ASSISTANT_PLATFORMS", "android")
		_, err := LoadMobileAssistant()
		requireFailLoud(t, err, "[F073-SST-INVALID]", "platforms")
	})
}

func requireFailLoud(t *testing.T, err error, prefix, key string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected fail-loud error containing %q and %q, got nil", prefix, key)
	}
	msg := err.Error()
	if !strings.Contains(msg, prefix) {
		t.Errorf("error prefix mismatch: got %q, want substring %q", msg, prefix)
	}
	if !strings.Contains(msg, key) {
		t.Errorf("error missing key %q: got %q", key, msg)
	}
}
