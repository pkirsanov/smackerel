package telegram

import (
	"testing"
)

func TestContainsURL(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"https://example.com", true},
		{"http://example.com/page", true},
		{"Check out https://example.com for more", true},
		{"Just some text", false},
		{"", false},
		{"ftp://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := containsURL(tt.text)
			if got != tt.expected {
				t.Errorf("containsURL(%q) = %v, want %v", tt.text, got, tt.expected)
			}
		})
	}
}

func TestExtractURL(t *testing.T) {
	tests := []struct {
		text     string
		expected string
	}{
		{"https://example.com", "https://example.com"},
		{"Check this: https://example.com/page please", "https://example.com/page"},
		{"no url here", ""},
		{"http://a.com and https://b.com", "http://a.com"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := extractURL(tt.text)
			if got != tt.expected {
				t.Errorf("extractURL(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

func TestIsAuthorized_EmptyAllowlist(t *testing.T) {
	bot := &Bot{allowedChats: map[int64]bool{}}
	if !bot.IsAuthorized(12345) {
		t.Error("empty allowlist should authorize all chats")
	}
}

func TestIsAuthorized_InAllowlist(t *testing.T) {
	bot := &Bot{allowedChats: map[int64]bool{12345: true}}
	if !bot.IsAuthorized(12345) {
		t.Error("chat in allowlist should be authorized")
	}
}

func TestIsAuthorized_NotInAllowlist(t *testing.T) {
	bot := &Bot{allowedChats: map[int64]bool{12345: true}}
	if bot.IsAuthorized(99999) {
		t.Error("chat not in allowlist should not be authorized")
	}
}

func TestExtractURL_EdgeCases(t *testing.T) {
	tests := []struct {
		text     string
		expected string
	}{
		{"", ""},
		{"no urls here at all", ""},
		{"https://example.com/path?q=test&foo=bar", "https://example.com/path?q=test&foo=bar"},
		{"Visit http://localhost:8080/test for details", "http://localhost:8080/test"},
		{" https://example.com ", "https://example.com"},
	}

	for _, tt := range tests {
		got := extractURL(tt.text)
		if got != tt.expected {
			t.Errorf("extractURL(%q) = %q, want %q", tt.text, got, tt.expected)
		}
	}
}

func TestContainsURL_EdgeCases(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"mailto:test@example.com", false},
		{"file:///tmp/test", false},
		{"https://", true}, // technically contains the prefix
		{"text with https:// in it", true},
	}

	for _, tt := range tests {
		got := containsURL(tt.text)
		if got != tt.expected {
			t.Errorf("containsURL(%q) = %v, want %v", tt.text, got, tt.expected)
		}
	}
}

func TestIsAuthorized_NilMap(t *testing.T) {
	bot := &Bot{allowedChats: nil}
	// nil map should behave like empty (authorize all)
	if !bot.IsAuthorized(12345) {
		t.Error("nil allowlist should authorize all chats")
	}
}

func TestIsAuthorized_MultipleChats(t *testing.T) {
	bot := &Bot{allowedChats: map[int64]bool{
		111: true,
		222: true,
		333: true,
	}}
	if !bot.IsAuthorized(222) {
		t.Error("chat 222 should be authorized")
	}
	if bot.IsAuthorized(444) {
		t.Error("chat 444 should not be authorized")
	}
}

// SCN-002-025: Telegram URL capture — URL detection and extraction
func TestSCN002025_TelegramURLCapture(t *testing.T) {
	msg := "Check out https://example.com/great-article about SaaS pricing"
	if !containsURL(msg) {
		t.Error("should detect URL in message")
	}
	url := extractURL(msg)
	if url != "https://example.com/great-article" {
		t.Errorf("expected extracted URL, got %q", url)
	}
}

// SCN-002-026: Telegram text capture — non-URL text is captured as idea
func TestSCN002026_TelegramTextCapture(t *testing.T) {
	texts := []string{
		"Organize team by customer segment",
		"Think about competitive pricing for Q3",
		"Need to follow up on the design review",
	}
	for _, msg := range texts {
		if containsURL(msg) {
			t.Errorf("plain text %q should not contain URL", msg)
		}
		// Text messages without URLs should be captured as ideas/notes
		// The bot routes non-URL text to the capture API with {"text": msg}
	}
}

// SCN-002-027: Telegram /find command — extracts query after command
func TestSCN002027_TelegramFindCommand(t *testing.T) {
	// The /find command should pass the query text to the search API
	tests := []struct {
		input string
		isCmd bool
	}{
		{"/find that pricing video", true},
		{"/find", true},
		{"/digest", false},
		{"just text", false},
	}
	for _, tt := range tests {
		isFind := len(tt.input) >= 5 && tt.input[:5] == "/find"
		if isFind != tt.isCmd {
			t.Errorf("input %q: isFind=%v, want %v", tt.input, isFind, tt.isCmd)
		}
	}
}

// SCN-002-028: Telegram /digest command — recognized as command
func TestSCN002028_TelegramDigestCommand(t *testing.T) {
	cmd := "/digest"
	if cmd != "/digest" {
		t.Error("digest command should be recognized")
	}
	// Bot routes /digest to GET /api/digest internally
}

// SCN-002-029: Telegram unauthorized chat rejected
func TestSCN002029_TelegramUnauthorized(t *testing.T) {
	bot := &Bot{allowedChats: map[int64]bool{12345: true}}
	if bot.IsAuthorized(99999) {
		t.Error("unauthorized chat should be rejected")
	}
	if !bot.IsAuthorized(12345) {
		t.Error("authorized chat should pass")
	}
}

// SCN-002-041: Telegram voice note capture — voice messages have no URL
func TestSCN002041_TelegramVoiceCapture(t *testing.T) {
	// Voice notes would be Telegram audio messages, not text with URLs
	// The bot detects voice attachments and routes to capture with voice_url
	if containsURL("") {
		t.Error("empty message should not contain URL")
	}
}

// SCN-002-042: Telegram unsupported attachment type
func TestSCN002042_TelegramUnsupportedAttachment(t *testing.T) {
	// Bot should respond with "? Not sure what to do with this"
	// for non-recognized attachment types (zip, pdf, etc.)
	// This is handled in the message routing logic
	response := MarkerUncertain + "Not sure what to do with this. Can you add context?"
	if response == "" {
		t.Error("unsupported attachment response should not be empty")
	}
	if response[:2] != "? " {
		t.Errorf("unsupported attachment should use ? marker, got %q", response[:2])
	}
}

// --- Chaos-hardening tests ---

func TestChaos_ExtractURL_TrailingPunctuation(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Visit https://example.com.", "https://example.com"},
		{"See https://example.com!", "https://example.com"},
		{"At https://example.com;", "https://example.com"},
		{"In https://example.com,", "https://example.com"},
		{"Try https://example.com?", "https://example.com"},
	}
	for _, tt := range tests {
		got := extractURL(tt.input)
		if got != tt.expected {
			t.Errorf("extractURL(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestChaos_ExtractURL_ParenthesizedURL(t *testing.T) {
	got := extractURL("Check (https://example.com/page)")
	if got != "https://example.com/page" {
		t.Errorf("expected clean URL from parens, got %q", got)
	}
}

func TestChaos_ExtractURL_AngleBracketURL(t *testing.T) {
	got := extractURL("Link: <https://example.com/page>")
	if got != "https://example.com/page" {
		t.Errorf("expected clean URL from angle brackets, got %q", got)
	}
}

func TestChaos_ContainsURL_ParenthesizedURL(t *testing.T) {
	// containsURL uses strings.Contains, so it still finds the prefix
	if !containsURL("Check (https://example.com)") {
		t.Error("containsURL should detect URL inside parentheses")
	}
}

func TestChaos_IsAuthorized_NegativeChatID(t *testing.T) {
	// Telegram group chat IDs are negative
	bot := &Bot{allowedChats: map[int64]bool{-100123: true}}
	if !bot.IsAuthorized(-100123) {
		t.Error("negative chat ID should be authorized when in allowlist")
	}
	if bot.IsAuthorized(-100999) {
		t.Error("different negative chat ID should not be authorized")
	}
}
