// Spec 061 SCOPE-04 — slash-command shortcut lookup tests.

package assistant

import "testing"

func TestLookupShortcut(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		text      string
		wantID    string
		wantReset bool
		wantOK    bool
	}{
		// --- v1 slash commands, bare form ---
		{name: "/ask bare", text: "/ask", wantID: "open_knowledge", wantOK: true},
		{name: "/weather bare", text: "/weather", wantID: "weather_query", wantOK: true},
		{name: "/remind bare", text: "/remind", wantID: "notification_schedule", wantOK: true},
		{name: "/reset bare", text: "/reset", wantID: "", wantReset: true, wantOK: true},

		// --- with natural-language tail ---
		{name: "/ask with tail", text: "/ask what did paul say last week", wantID: "open_knowledge", wantOK: true},
		{name: "/weather with tail", text: "/weather in barcelona tomorrow", wantID: "weather_query", wantOK: true},
		{name: "/remind with tail", text: "/remind tomorrow 9am submit report", wantID: "notification_schedule", wantOK: true},
		{name: "/reset with trailing whitespace", text: "/reset\n", wantID: "", wantReset: true, wantOK: true},

		// --- whitespace handling (leading/trailing trimmed) ---
		{name: "/ask leading newline", text: "\n/ask hello", wantID: "open_knowledge", wantOK: true},
		{name: "/weather leading spaces", text: "   /weather barcelona", wantID: "weather_query", wantOK: true},
		{name: "/remind tab-separated tail", text: "/remind\tin 5 minutes", wantID: "notification_schedule", wantOK: true},

		// --- case sensitivity (design §3.4) ---
		{name: "/Ask uppercase A — no match", text: "/Ask hello", wantID: "", wantOK: false},
		{name: "/ASK all caps — no match", text: "/ASK hello", wantID: "", wantOK: false},
		{name: "/Weather mixed — no match", text: "/Weather barcelona", wantID: "", wantOK: false},

		// --- prefix-only / partial shortcut ---
		{name: "asking without slash — no match", text: "asking the assistant", wantID: "", wantOK: false},
		{name: "/asking longer command — no match", text: "/asking question", wantID: "", wantOK: false},
		{name: "/askx no whitespace — no match", text: "/askx", wantID: "", wantOK: false},
		{name: "/help non-v1 shortcut — no match", text: "/help", wantID: "", wantOK: false},

		// --- empty input ---
		{name: "empty string", text: "", wantID: "", wantOK: false},
		{name: "whitespace only", text: "   \n\t ", wantID: "", wantOK: false},

		// --- plain natural language never trips ---
		{name: "plain text — no match", text: "what time is it in barcelona", wantID: "", wantOK: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotID, gotReset, gotOK := LookupShortcut(tc.text)
			if gotOK != tc.wantOK || gotID != tc.wantID || gotReset != tc.wantReset {
				t.Errorf("LookupShortcut(%q) = (%q, %v, %v); want (%q, %v, %v)",
					tc.text, gotID, gotReset, gotOK, tc.wantID, tc.wantReset, tc.wantOK)
			}
		})
	}
}

// TestSlashShortcutsClosedVocabulary asserts the v1-frozen map shape:
// exactly six entries (4 original + /recipe, /cook added 2026-06-02
// to fix Telegram "Unknown command" regression), each pointing to its
// design-§3.4 target. /recipe and /cook both alias to recipe_search
// because the scenario itself is the same — the two shortcuts give
// users a natural surface for "find me a recipe" vs "cook this now".
func TestSlashShortcutsClosedVocabulary(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"/ask":     "open_knowledge",
		"/weather": "weather_query",
		"/remind":  "notification_schedule",
		"/recipe":  "recipe_search",
		"/cook":    "recipe_search",
		"/reset":   ResetActionID,
	}
	if len(SlashShortcuts) != len(want) {
		t.Fatalf("SlashShortcuts cardinality = %d; want %d (v1 set is frozen)",
			len(SlashShortcuts), len(want))
	}
	for k, v := range want {
		if got, ok := SlashShortcuts[k]; !ok || got != v {
			t.Errorf("SlashShortcuts[%q] = (%q, %v); want (%q, true)", k, got, ok, v)
		}
	}
}

// TestStripShortcutPrefix covers the Spec 061 Round-55 Defect-3 helper
// used by the capability-layer dispatch to extract the natural-language
// body from a slash-command input. Adversarial cases (unrecognized
// prefix, bare shortcut without body, mixed whitespace, case-sensitive
// non-matches) guard against regressions that would re-introduce the
// pre-fix behavior where the executor saw nil StructuredContext.
func TestStripShortcutPrefix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want string
	}{
		// --- v1 slash commands with natural-language bodies ---
		{name: "/ask with body", text: "/ask what is the weather", want: "what is the weather"},
		{name: "/weather with body", text: "/weather barcelona tomorrow", want: "barcelona tomorrow"},
		{name: "/remind with body", text: "/remind 9am submit report", want: "9am submit report"},
		{name: "/reset with body", text: "/reset now please", want: "now please"},

		// --- bare shortcuts return empty body (LLM gets empty query) ---
		{name: "/ask bare", text: "/ask", want: ""},
		{name: "/weather bare", text: "/weather", want: ""},
		{name: "/remind bare", text: "/remind", want: ""},
		{name: "/reset bare", text: "/reset", want: ""},

		// --- whitespace normalization ---
		{name: "leading whitespace", text: "   /ask hello", want: "hello"},
		{name: "trailing whitespace", text: "/ask hello   ", want: "hello"},
		{name: "mixed inner whitespace", text: "/ask   what    is    X", want: "what    is    X"},
		{name: "tab between prefix and body", text: "/remind\tin 5 minutes", want: "in 5 minutes"},
		{name: "newline between prefix and body", text: "/weather\nbarcelona", want: "barcelona"},

		// --- adversarial: unrecognized prefix must be returned verbatim ---
		{name: "/foobar unrecognized", text: "/foobar baz", want: "/foobar baz"},
		{name: "/help unrecognized", text: "/help anything", want: "/help anything"},
		{name: "/asking longer command", text: "/asking question", want: "/asking question"},
		{name: "/askx no whitespace", text: "/askx", want: "/askx"},

		// --- adversarial: case-sensitive non-match ---
		{name: "/Ask uppercase A", text: "/Ask hello", want: "/Ask hello"},
		{name: "/ASK all caps", text: "/ASK hello", want: "/ASK hello"},

		// --- plain text without slash returns trimmed verbatim ---
		{name: "plain text", text: "what time is it", want: "what time is it"},
		{name: "plain text with surrounding ws", text: "  hello world  ", want: "hello world"},

		// --- empty inputs ---
		{name: "empty string", text: "", want: ""},
		{name: "whitespace only", text: "   \n\t ", want: ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := StripShortcutPrefix(tc.text); got != tc.want {
				t.Errorf("StripShortcutPrefix(%q) = %q; want %q", tc.text, got, tc.want)
			}
		})
	}
}
