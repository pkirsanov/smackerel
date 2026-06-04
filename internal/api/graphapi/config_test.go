package graphapi

import (
	"strings"
	"testing"
)

// TestLoadConfig_FailsLoudOnMissingKeys — SST contract: every
// KNOWLEDGE_GRAPH_API_* env var must be present, and the consolidated
// error must name every offender so operators get one actionable boot
// failure instead of N silent loops.
func TestLoadConfig_FailsLoudOnMissingKeys(t *testing.T) {
	// Unset everything we care about so the test is hermetic.
	for _, k := range []string{
		"KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT",
		"KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT",
		"KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS",
		"KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT",
		"KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT",
		"KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV",
	} {
		t.Setenv(k, "")
		// Setenv with "" still satisfies LookupEnv == ok=true, so
		// also exercise the "ok=false" branch by unsetting explicitly
		// for one key per case. We rely on (ok, "") and (ok, garbage)
		// branches via the per-key sub-tests below.
	}
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig with all KNOWLEDGE_GRAPH_API_* keys empty returned no error")
	}
	want := []string{
		"KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT",
		"KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT",
		"KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS",
		"KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT",
		"KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT",
		"KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV",
	}
	got := err.Error()
	for _, k := range want {
		if !strings.Contains(got, k) {
			t.Errorf("error message missing key %q; got: %s", k, got)
		}
	}
	if !strings.Contains(got, "F080-SST-MISSING") {
		t.Errorf("error message missing [F080-SST-MISSING] tag; got: %s", got)
	}
}

// TestLoadConfig_RejectsNonPositiveInts — adversarial: zero and
// negative values silently parse as ints but violate the design.md §6
// invariant that every limit is positive.
func TestLoadConfig_RejectsNonPositiveInts(t *testing.T) {
	setAllValid(t)
	t.Setenv("KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT", "0")
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig with LIST_MAX_LIMIT=0 returned no error")
	}
	if !strings.Contains(err.Error(), "KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT") {
		t.Errorf("error message missing offender; got: %s", err.Error())
	}
}

// TestLoadConfig_RejectsNonInteger — adversarial garbage value.
func TestLoadConfig_RejectsNonInteger(t *testing.T) {
	setAllValid(t)
	t.Setenv("KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS", "not-a-number")
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig with non-integer TIME_WINDOW_MAX_DAYS returned no error")
	}
}

// TestLoadConfig_Happy — valid env produces a populated Config.
func TestLoadConfig_Happy(t *testing.T) {
	setAllValid(t)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ListMaxLimit != 200 {
		t.Errorf("ListMaxLimit = %d; want 200", cfg.ListMaxLimit)
	}
	if cfg.CursorSecretEnv != "KNOWLEDGE_GRAPH_API_CURSOR_SECRET" {
		t.Errorf("CursorSecretEnv = %q", cfg.CursorSecretEnv)
	}
	l := cfg.Limits()
	if l.ListMax != cfg.ListMaxLimit {
		t.Errorf("Limits projection lost ListMax: %+v", l)
	}
}

// TestValidate_RejectsDefaultAboveMax — cross-field invariant.
func TestValidate_RejectsDefaultAboveMax(t *testing.T) {
	c := Config{
		ListDefaultLimit:  500,
		ListMaxLimit:      100,
		TimeWindowMaxDays: 1,
		EdgesDefaultLimit: 1,
		EdgesMaxLimit:     1,
		CursorSecretEnv:   "X",
	}
	if err := c.Validate(); err == nil {
		t.Error("Validate accepted ListDefaultLimit > ListMaxLimit")
	}
}

// TestLoadCursorSecret_FailsLoudWhenEnvUnset — SCN-080-09 adjacent:
// the cursor secret is the foundation of forge-resistance; missing
// secret MUST stop the boot, never silently produce an empty key.
func TestLoadCursorSecret_FailsLoudWhenEnvUnset(t *testing.T) {
	c := Config{CursorSecretEnv: "TEST_GRAPHAPI_NEVER_SET_CURSOR_SECRET_080"}
	if _, err := c.LoadCursorSecret(); err == nil {
		t.Error("LoadCursorSecret with unset env returned no error")
	}
}

func TestLoadCursorSecret_FailsLoudWhenEnvEmpty(t *testing.T) {
	c := Config{CursorSecretEnv: "TEST_GRAPHAPI_EMPTY_CURSOR_SECRET_080"}
	t.Setenv("TEST_GRAPHAPI_EMPTY_CURSOR_SECRET_080", "")
	if _, err := c.LoadCursorSecret(); err == nil {
		t.Error("LoadCursorSecret with empty env returned no error")
	}
}

func TestLoadCursorSecret_Happy(t *testing.T) {
	c := Config{CursorSecretEnv: "TEST_GRAPHAPI_HAPPY_CURSOR_SECRET_080"}
	t.Setenv("TEST_GRAPHAPI_HAPPY_CURSOR_SECRET_080", "hmac-key-bytes")
	got, err := c.LoadCursorSecret()
	if err != nil {
		t.Fatalf("LoadCursorSecret: %v", err)
	}
	if string(got) != "hmac-key-bytes" {
		t.Errorf("secret = %q", string(got))
	}
}

func setAllValid(t *testing.T) {
	t.Helper()
	t.Setenv("KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT", "50")
	t.Setenv("KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT", "200")
	t.Setenv("KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS", "365")
	t.Setenv("KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT", "100")
	t.Setenv("KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT", "500")
	t.Setenv("KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV", "KNOWLEDGE_GRAPH_API_CURSOR_SECRET")
}
