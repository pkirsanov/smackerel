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
		{"https://", true},      // technically contains the prefix
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
