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
		{name: "/ask bare", text: "/ask", wantID: "retrieval_qa", wantOK: true},
		{name: "/weather bare", text: "/weather", wantID: "weather_query", wantOK: true},
		{name: "/remind bare", text: "/remind", wantID: "notification_schedule", wantOK: true},
		{name: "/reset bare", text: "/reset", wantID: "", wantReset: true, wantOK: true},

		// --- with natural-language tail ---
		{name: "/ask with tail", text: "/ask what did paul say last week", wantID: "retrieval_qa", wantOK: true},
		{name: "/weather with tail", text: "/weather in barcelona tomorrow", wantID: "weather_query", wantOK: true},
		{name: "/remind with tail", text: "/remind tomorrow 9am submit report", wantID: "notification_schedule", wantOK: true},
		{name: "/reset with trailing whitespace", text: "/reset\n", wantID: "", wantReset: true, wantOK: true},

		// --- whitespace handling (leading/trailing trimmed) ---
		{name: "/ask leading newline", text: "\n/ask hello", wantID: "retrieval_qa", wantOK: true},
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
// exactly four entries, each pointing to its design-§3.4 target.
func TestSlashShortcutsClosedVocabulary(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"/ask":     "retrieval_qa",
		"/weather": "weather_query",
		"/remind":  "notification_schedule",
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
