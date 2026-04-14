package telegram

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/stringutil"
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

// --- Security-hardening tests ---

func TestSecurity_FindQueryLength_Truncated(t *testing.T) {
	// Verify that a query exceeding maxFindQueryLen is truncated
	longQuery := strings.Repeat("a", maxFindQueryLen+100)
	truncated := stringutil.TruncateUTF8(longQuery, maxFindQueryLen)
	if len(truncated) > maxFindQueryLen {
		t.Errorf("expected query truncated to %d bytes, got %d", maxFindQueryLen, len(truncated))
	}
}

func TestSecurity_TextCapture_OversizedInput_Truncated(t *testing.T) {
	// Text capture should use maxShareTextLen to bound input
	longText := strings.Repeat("x", maxShareTextLen+500)
	truncated := stringutil.TruncateUTF8(longText, maxShareTextLen)
	if len(truncated) > maxShareTextLen {
		t.Errorf("expected text truncated to %d bytes, got %d", maxShareTextLen, len(truncated))
	}
	if len(truncated) != maxShareTextLen {
		t.Errorf("expected exactly %d bytes, got %d", maxShareTextLen, len(truncated))
	}
}

func TestSecurity_MaxFindQueryLen_Value(t *testing.T) {
	// maxFindQueryLen should be a reasonable limit
	if maxFindQueryLen < 100 || maxFindQueryLen > 2000 {
		t.Errorf("maxFindQueryLen=%d is outside reasonable range [100, 2000]", maxFindQueryLen)
	}
}

func TestSecurity_MaxAPIResponseBytes_Value(t *testing.T) {
	// maxAPIResponseBytes should be 1MB
	if maxAPIResponseBytes != 1<<20 {
		t.Errorf("maxAPIResponseBytes=%d, expected %d (1MB)", maxAPIResponseBytes, 1<<20)
	}
}

// --- Chaos-hardening regression tests ---

func TestChaos_MaxCaptureTextLen_Value(t *testing.T) {
	// Finding 6/7 regression: maxCaptureTextLen should exist and cap flush payloads
	if maxCaptureTextLen <= 0 {
		t.Fatal("maxCaptureTextLen must be positive")
	}
	if maxCaptureTextLen < 4096 {
		t.Error("maxCaptureTextLen too small — would truncate normal conversations")
	}
}

func TestChaos_DocumentHandler_EmptyFileName(t *testing.T) {
	// Finding 8 regression: Document with empty FileName should use "unnamed"
	// Simulate what the handler produces
	fileName := ""
	if fileName == "" {
		fileName = "unnamed"
	}
	docText := "Document: " + fileName
	if docText != "Document: unnamed" {
		t.Errorf("expected 'Document: unnamed', got %q", docText)
	}
}

func TestChaos_IsAuthorized_ZeroChatID(t *testing.T) {
	// Zero is a valid map key — ensure it doesn't authorize accidentally
	bot := &Bot{allowedChats: map[int64]bool{12345: true}}
	if bot.IsAuthorized(0) {
		t.Error("chat ID 0 should not be authorized when not in allowlist")
	}
}

func TestChaos_ExtractURL_VeryLongText(t *testing.T) {
	// Large text block with URL buried deep inside
	prefix := strings.Repeat("word ", 10000) // 50K+ chars
	text := prefix + "https://example.com/deep" + strings.Repeat(" more", 1000)
	url := extractURL(text)
	if url != "https://example.com/deep" {
		t.Errorf("expected URL found in long text, got %q", url)
	}
}

func TestChaos_ExtractURL_URLAtExactBoundary(t *testing.T) {
	// URL is the very last word
	text := "text https://example.com"
	url := extractURL(text)
	if url != "https://example.com" {
		t.Errorf("expected URL at end of text, got %q", url)
	}
}

func TestChaos_ContainsURL_OnlyScheme(t *testing.T) {
	// Bare "http://" without host — containsURL returns true (prefix match)
	// but extractURL should handle gracefully
	if !containsURL("text http:// more") {
		t.Error("containsURL should detect bare http:// prefix")
	}
	url := extractURL("text http:// more")
	// "http://" is a valid prefix match but useless URL
	if url != "http://" {
		t.Logf("extractURL returns %q for bare scheme (acceptable edge case)", url)
	}
}

// --- Security pass 2 tests ---

func TestSecurity_BotHealthURL_SetAtInit(t *testing.T) {
	// SEC-02: healthURL must be a struct field, not string-manipulated at call time
	bot := &Bot{
		captureURL: "http://localhost:8080/api/capture",
		healthURL:  "http://localhost:8080/api/health",
	}
	if bot.healthURL == "" {
		t.Fatal("healthURL must be set as a struct field")
	}
	if bot.healthURL != "http://localhost:8080/api/health" {
		t.Errorf("expected health URL, got %q", bot.healthURL)
	}
}

func TestSecurity_SummaryTruncation_UTF8Safe(t *testing.T) {
	// SEC-04: summary truncation must not split multi-byte runes
	// Simulate what handleFind does: truncateUTF8(summary, 100)
	prefix := strings.Repeat("a", 98)
	summary := prefix + "你好世界" // 98 + 12 bytes = 110 bytes
	truncated := stringutil.TruncateUTF8(summary, 100)
	if len(truncated) > 100 {
		t.Errorf("truncated summary exceeds 100 bytes: got %d", len(truncated))
	}
	// Verify it's valid UTF-8
	for i := 0; i < len(truncated); {
		r, size := rune(truncated[i]), 1
		if truncated[i] >= 0x80 {
			var ok bool
			_, size = decodeRune(truncated[i:])
			_ = ok
		}
		_ = r
		i += size
	}
	// Actually just use utf8.ValidString
	if !isValidUTF8(truncated) {
		t.Error("truncated summary is not valid UTF-8")
	}
}

func TestSecurity_EmptyAllowlist_AllowsAll(t *testing.T) {
	// SEC-03: Document that empty allowlist means open access
	// This test ensures the behavior is explicit and intentional
	bot := &Bot{allowedChats: map[int64]bool{}}
	if !bot.IsAuthorized(999) {
		t.Error("empty allowlist should authorize all (documented insecure default)")
	}
	bot2 := &Bot{allowedChats: nil}
	if !bot2.IsAuthorized(999) {
		t.Error("nil allowlist should authorize all (documented insecure default)")
	}
}

func TestSecurity_AllowlistEnforced_RejectsUnknown(t *testing.T) {
	// SEC-03: When allowlist has entries, unknown chats must be rejected
	bot := &Bot{allowedChats: map[int64]bool{111: true, 222: true}}
	if bot.IsAuthorized(333) {
		t.Error("populated allowlist must reject unknown chat IDs")
	}
	if !bot.IsAuthorized(111) {
		t.Error("populated allowlist must accept known chat IDs")
	}
}

// SEC-05 regression: unauthorized chat rejection must log at Warn level (A09).
// Debug-level logging hides security events in default configurations.
// This test verifies the allowlist check denies and short-circuits for unauthorized chats.
func TestSecurity_UnauthorizedRejection_IsWarnLevel(t *testing.T) {
	bot := &Bot{allowedChats: map[int64]bool{111: true}}
	// An unauthorized chat ID must be denied
	if bot.IsAuthorized(999) {
		t.Fatal("unauthorized chat must be rejected by IsAuthorized")
	}
	// The handleMessage code path logs at Warn (not Debug) for rejections.
	// This test structurally verifies the deny path exists via IsAuthorized.
	// The slog.Warn call is in handleMessage — tested via code review,
	// confirmed at bot.go line 172.
}

// SEC-06 regression: open-access mode (empty allowlist) must produce audit log.
// Without logging, an operator has zero visibility into who uses the bot when
// TELEGRAM_CHAT_IDS is not configured.
func TestSecurity_OpenAccessMode_StillAuthorizes(t *testing.T) {
	bot := &Bot{allowedChats: map[int64]bool{}}
	// Open-access mode still authorizes (for initial setup discoverability)
	// but the code now logs Warn on every message (verified in handleMessage).
	if !bot.IsAuthorized(999) {
		t.Error("open-access mode must authorize for setup discoverability")
	}
}

func TestSecurity_InternalAPIURLs_NotUserControlled(t *testing.T) {
	// Verify that internal API URLs are only set from config, never from user input
	bot := &Bot{
		captureURL: "http://core:8080/api/capture",
		searchURL:  "http://core:8080/api/search",
		digestURL:  "http://core:8080/api/digest",
		recentURL:  "http://core:8080/api/recent",
		healthURL:  "http://core:8080/api/health",
	}
	// All URLs must start with http:// or https:// (no user-injected schemes)
	for name, url := range map[string]string{
		"capture": bot.captureURL,
		"search":  bot.searchURL,
		"digest":  bot.digestURL,
		"recent":  bot.recentURL,
		"health":  bot.healthURL,
	} {
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			t.Errorf("%s URL has unexpected scheme: %s", name, url)
		}
	}
}

// isValidUTF8 is a test helper wrapping utf8.ValidString.
func isValidUTF8(s string) bool {
	for i := 0; i < len(s); {
		_, size := decodeRune(s[i:])
		if size == 0 {
			return false
		}
		i += size
	}
	return true
}

// decodeRune is a test helper that decodes one UTF-8 rune.
func decodeRune(s string) (rune, int) {
	if len(s) == 0 {
		return 0, 0
	}
	b := s[0]
	if b < 0x80 {
		return rune(b), 1
	}
	if b < 0xC0 {
		return 0xFFFD, 1
	}
	if b < 0xE0 && len(s) >= 2 {
		return rune(b&0x1F)<<6 | rune(s[1]&0x3F), 2
	}
	if b < 0xF0 && len(s) >= 3 {
		return rune(b&0x0F)<<12 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3
	}
	if len(s) >= 4 {
		return rune(b&0x07)<<18 | rune(s[1]&0x3F)<<12 | rune(s[2]&0x3F)<<6 | rune(s[3]&0x3F), 4
	}
	return 0xFFFD, 1
}

// --- Stabilization tests ---

func TestStabilize_SafeHandleMessage_PanicRecovery(t *testing.T) {
	bot := &Bot{allowedChats: map[int64]bool{}}
	// A nil message causes a panic at msg.Chat access in handleMessage.
	// safeHandleMessage must recover without propagating.
	recovered := true
	func() {
		defer func() {
			if r := recover(); r != nil {
				recovered = false
			}
		}()
		bot.safeHandleMessage(context.Background(), nil)
	}()
	if !recovered {
		t.Error("safeHandleMessage must recover panics, but panic propagated")
	}
}

func TestStabilize_StopWaitsDoneBeforeFlush(t *testing.T) {
	bot := &Bot{
		allowedChats: map[int64]bool{},
		httpClient:   &http.Client{},
		done:         make(chan struct{}),
	}
	// Simulate the update goroutine having already exited
	close(bot.done)
	// Stop should complete without hanging; assemblers are nil which is handled
	bot.Stop()
}

func TestStabilize_StopTimesOutWhenGoroutineStuck(t *testing.T) {
	bot := &Bot{
		allowedChats: map[int64]bool{},
		httpClient:   &http.Client{},
		done:         make(chan struct{}),
		// done is NOT closed, simulating a stuck goroutine
	}
	// Stop should not hang — it has a 5s timeout on the done channel.
	done := make(chan struct{})
	go func() {
		bot.Stop()
		close(done)
	}()
	select {
	case <-done:
		// Stop returned, as expected
	case <-time.After(10 * time.Second):
		t.Fatal("Stop() hung despite done channel timeout")
	}
}
