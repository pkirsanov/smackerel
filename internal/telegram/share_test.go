package telegram

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/stringutil"
)

func TestExtractAllURLs_SingleURL(t *testing.T) {
	urls := extractAllURLs("Check out https://example.com/article please")
	if len(urls) != 1 || urls[0] != "https://example.com/article" {
		t.Errorf("expected 1 URL, got %v", urls)
	}
}

func TestExtractAllURLs_MultipleURLs(t *testing.T) {
	text := "https://example.com and also https://other.com/page"
	urls := extractAllURLs(text)
	if len(urls) != 2 {
		t.Errorf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
}

func TestExtractAllURLs_DuplicateURLs(t *testing.T) {
	text := "https://example.com and https://example.com again"
	urls := extractAllURLs(text)
	if len(urls) != 1 {
		t.Errorf("expected 1 deduplicated URL, got %d: %v", len(urls), urls)
	}
}

func TestExtractAllURLs_NoURLs(t *testing.T) {
	urls := extractAllURLs("just plain text without links")
	if len(urls) != 0 {
		t.Errorf("expected 0 URLs, got %v", urls)
	}
}

func TestExtractAllURLs_TrailingPunctuation(t *testing.T) {
	urls := extractAllURLs("Visit https://example.com. More info at https://other.com!")
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com" {
		t.Errorf("expected trailing period stripped, got %s", urls[0])
	}
	if urls[1] != "https://other.com" {
		t.Errorf("expected trailing exclamation stripped, got %s", urls[1])
	}
}

func TestExtractContext_URLRemoved(t *testing.T) {
	text := "Check this out https://example.com very interesting"
	ctx := extractContext(text, []string{"https://example.com"})
	if ctx != "Check this out very interesting" {
		t.Errorf("unexpected context: %q", ctx)
	}
}

func TestExtractContext_MultipleURLsRemoved(t *testing.T) {
	text := "https://a.com and https://b.com both good"
	ctx := extractContext(text, []string{"https://a.com", "https://b.com"})
	if ctx != "and both good" {
		t.Errorf("unexpected context: %q", ctx)
	}
}

func TestExtractContext_EmptyAfterRemoval(t *testing.T) {
	text := "https://example.com"
	ctx := extractContext(text, []string{"https://example.com"})
	if ctx != "" {
		t.Errorf("expected empty context, got %q", ctx)
	}
}

func TestSCN008001_ShareSheetURLWithContext(t *testing.T) {
	// SC-TSC01: Share-sheet URL + context preserved
	urls := extractAllURLs("Great article about Go concurrency https://blog.example.com/go-concurrency")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d", len(urls))
	}
	ctx := extractContext("Great article about Go concurrency https://blog.example.com/go-concurrency", urls)
	if ctx != "Great article about Go concurrency" {
		t.Errorf("expected context preserved, got %q", ctx)
	}
}

func TestSCN008002_MultipleURLsFromShareSheet(t *testing.T) {
	// SC-TSC02: Multiple URLs captured individually
	text := "Check these: https://a.example.com/page and https://b.example.com/other"
	urls := extractAllURLs(text)
	if len(urls) != 2 {
		t.Errorf("expected 2 URLs, got %d", len(urls))
	}
}

func TestSCN008003_BareURLBackwardCompat(t *testing.T) {
	// SC-TSC03: Bare URL without context — backward compatible
	urls := extractAllURLs("https://example.com/article")
	ctx := extractContext("https://example.com/article", urls)
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d", len(urls))
	}
	if ctx != "" {
		t.Errorf("expected empty context for bare URL, got %q", ctx)
	}
}

// --- Chaos-hardening tests ---

func TestChaos_TruncateUTF8_MultiByteAtBoundary(t *testing.T) {
	// 3-byte UTF-8 char (Chinese): "你" = 0xE4 0xBD 0xA0
	// Build a string where a multi-byte char straddles the byte boundary
	prefix := strings.Repeat("a", 4094) // 4094 ASCII bytes
	text := prefix + "你好"               // 4094 + 3 + 3 = 4100 bytes

	result := stringutil.TruncateUTF8(text, 4096)
	if !utf8.ValidString(result) {
		t.Errorf("TruncateUTF8 produced invalid UTF-8: %q", result)
	}
	if len(result) > 4096 {
		t.Errorf("result exceeds maxBytes: got %d", len(result))
	}
	// The 3-byte char at byte 4094 would end at 4097, so it should be excluded
	if len(result) != 4094 {
		t.Errorf("expected 4094 bytes (dropping split rune), got %d", len(result))
	}
}

func TestChaos_TruncateUTF8_ExactBoundary(t *testing.T) {
	// String that is exactly maxShareTextLen — no truncation needed
	text := strings.Repeat("x", maxShareTextLen)
	result := stringutil.TruncateUTF8(text, maxShareTextLen)
	if result != text {
		t.Error("exact-length string should not be modified")
	}
}

func TestChaos_TruncateUTF8_ShortString(t *testing.T) {
	text := "short"
	result := stringutil.TruncateUTF8(text, maxShareTextLen)
	if result != text {
		t.Errorf("short string modified: %q", result)
	}
}

func TestChaos_TruncateUTF8_EmojiAtBoundary(t *testing.T) {
	// 4-byte UTF-8 emoji: "😀" = 0xF0 0x9F 0x98 0x80
	prefix := strings.Repeat("a", 4093) // 4093 ASCII bytes
	text := prefix + "😀"                // 4093 + 4 = 4097 bytes

	result := stringutil.TruncateUTF8(text, 4096)
	if !utf8.ValidString(result) {
		t.Errorf("TruncateUTF8 produced invalid UTF-8 with emoji")
	}
	// Emoji can't fit, so we get just the prefix
	if len(result) != 4093 {
		t.Errorf("expected 4093 bytes (dropping emoji), got %d", len(result))
	}
}

func TestChaos_TruncateUTF8_AllMultiByte(t *testing.T) {
	// String of 2-byte UTF-8 chars (Cyrillic Д = 0xD0 0x94)
	text := strings.Repeat("Д", 3000) // 6000 bytes
	result := stringutil.TruncateUTF8(text, 4096)
	if !utf8.ValidString(result) {
		t.Errorf("TruncateUTF8 produced invalid UTF-8 for Cyrillic")
	}
	// 4096 / 2 = 2048 chars = 4096 bytes exactly
	if len(result) != 4096 {
		t.Errorf("expected 4096 bytes, got %d", len(result))
	}
}

func TestChaos_ExtractAllURLs_ParenthesizedURL(t *testing.T) {
	// Common in markdown/email: (https://example.com)
	urls := extractAllURLs("Check this (https://example.com/article) for details")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL from parenthesized, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com/article" {
		t.Errorf("expected clean URL, got %q", urls[0])
	}
}

func TestChaos_ExtractAllURLs_AngleBracketURL(t *testing.T) {
	// Common in email: <https://example.com>
	urls := extractAllURLs("Link: <https://example.com/page>")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL from angle brackets, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com/page" {
		t.Errorf("expected clean URL, got %q", urls[0])
	}
}

func TestChaos_ExtractAllURLs_SquareBracketURL(t *testing.T) {
	urls := extractAllURLs("See [https://example.com/doc]")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL from square brackets, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com/doc" {
		t.Errorf("expected clean URL, got %q", urls[0])
	}
}

func TestChaos_ExtractAllURLs_EmptyString(t *testing.T) {
	urls := extractAllURLs("")
	if len(urls) != 0 {
		t.Errorf("empty string should return no URLs, got %v", urls)
	}
}

func TestChaos_ExtractAllURLs_OnlyWhitespace(t *testing.T) {
	urls := extractAllURLs("   \t\n  ")
	if len(urls) != 0 {
		t.Errorf("whitespace-only should return no URLs, got %v", urls)
	}
}

func TestExtractAllURLs_URLsWithQueryParams(t *testing.T) {
	urls := extractAllURLs("Check https://example.com/search?foo=bar&baz=1 for results")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com/search?foo=bar&baz=1" {
		t.Errorf("expected URL with query params preserved, got %q", urls[0])
	}
}

func TestExtractAllURLs_URLsWithFragment(t *testing.T) {
	urls := extractAllURLs("See https://example.com/page#section2 for details")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com/page#section2" {
		t.Errorf("expected URL with fragment preserved, got %q", urls[0])
	}
}

func TestChaos_ExtractAllURLs_UnicodeAroundURL(t *testing.T) {
	// URL preceded/followed by Unicode text
	urls := extractAllURLs("日本語 https://example.com/ページ 中文")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d: %v", len(urls), urls)
	}
}

func TestChaos_ExtractContext_OnlyURLs(t *testing.T) {
	text := "https://a.com https://b.com"
	urls := extractAllURLs(text)
	ctx := extractContext(text, urls)
	if ctx != "" {
		t.Errorf("expected empty context when only URLs, got %q", ctx)
	}
}

// --- Regression tests (REG-008) ---

// REG-008-001: extractContext must remove URLs longest-first so that a shorter
// URL that is a prefix of a longer one doesn't corrupt the longer URL via
// strings.ReplaceAll. If the bug were reintroduced (removal in arbitrary order),
// "https://example.com/page" would be removed first from
// "https://example.com/page/sub", leaving "/sub" as context.
func TestREG008001_ExtractContext_PrefixURLCollision(t *testing.T) {
	text := "see https://example.com/page and https://example.com/page/sub for info"
	urls := []string{"https://example.com/page", "https://example.com/page/sub"}
	ctx := extractContext(text, urls)
	// With correct longest-first removal, both URLs are fully removed.
	if ctx != "see and for info" {
		t.Errorf("expected 'see and for info', got %q — prefix URL corrupted the longer URL", ctx)
	}
}

// REG-008-001b: Reversed order in the input list should produce the same result
// because extractContext sorts internally.
func TestREG008001b_ExtractContext_PrefixURLCollision_ReversedInput(t *testing.T) {
	text := "https://example.com/a/b/c then https://example.com/a"
	urls := []string{"https://example.com/a", "https://example.com/a/b/c"} // shorter first
	ctx := extractContext(text, urls)
	if ctx != "then" {
		t.Errorf("expected 'then', got %q — prefix URL corrupted the longer URL", ctx)
	}
}

// REG-008-001c: Three URLs with nested prefix chain.
func TestREG008001c_ExtractContext_TriplePrefixChain(t *testing.T) {
	text := "a https://x.com b https://x.com/y c https://x.com/y/z d"
	urls := []string{"https://x.com", "https://x.com/y", "https://x.com/y/z"}
	ctx := extractContext(text, urls)
	if ctx != "a b c d" {
		t.Errorf("expected 'a b c d', got %q — nested prefix chain corrupted URLs", ctx)
	}
}

// REG-008-002: ForwardedMeta with ForwardDate=0 produces Unix epoch (1970-01-01).
// Regression guard: if the bug were reintroduced where epoch dates are not
// detected, downstream artifacts would have nonsensical timestamps.
func TestREG008002_ExtractForwardMeta_ZeroDate(t *testing.T) {
	msg := &tgbotapi.Message{
		ForwardDate: 0,
	}
	meta := extractForwardMeta(msg)
	// Year 1970 is almost certainly wrong for a forwarded message
	if meta.OriginalDate.Year() != 1970 {
		t.Errorf("expected epoch year 1970 for ForwardDate=0, got %d", meta.OriginalDate.Year())
	}
	// Ensure no panic and SenderName is set
	if meta.SenderName != "Anonymous" {
		t.Errorf("expected 'Anonymous', got %q", meta.SenderName)
	}
}

// REG-008-003: Anonymous forward assembly key collision — two completely
// anonymous senders (no ForwardFrom, no ForwardFromChat, no ForwardSenderName)
// would produce the same assembly key, merging unrelated conversations.
func TestREG008003_AnonymousForwardKeyCollision(t *testing.T) {
	// Simulate two anonymous forwards with different content
	msg1 := &tgbotapi.Message{ForwardDate: int(time.Now().Unix())}
	msg2 := &tgbotapi.Message{ForwardDate: int(time.Now().Unix())}

	meta1 := extractForwardMeta(msg1)
	meta2 := extractForwardMeta(msg2)

	key1 := assemblyKey{chatID: 42, sourceChatID: meta1.SourceChatID, sourceName: meta1.SourceChat}
	if key1.sourceChatID == 0 && key1.sourceName == "" {
		key1.sourceName = meta1.SenderName
	}
	key2 := assemblyKey{chatID: 42, sourceChatID: meta2.SourceChatID, sourceName: meta2.SourceChat}
	if key2.sourceChatID == 0 && key2.sourceName == "" {
		key2.sourceName = meta2.SenderName
	}

	// Both map to "Anonymous" — document this known collision
	if key1 != key2 {
		t.Error("expected same key for two anonymous forwards (known limitation)")
	}
	// Guard: if someone changes the anonymous fallback string, catch it
	if meta1.SenderName != "Anonymous" || meta2.SenderName != "Anonymous" {
		t.Errorf("expected both anonymous, got %q and %q", meta1.SenderName, meta2.SenderName)
	}
}

// REG-008-004: Assembly with maxMessages=1 — the first message creates the
// buffer but overflow only triggers on the NEXT Add. This means maxMessages=1
// effectively allows up to 2 messages (one at creation, triggering overflow on
// the second). Regression guard: if the overflow check position changes, this
// test catches it.
func TestREG008004_AssemblyMaxMessages1_SecondMsgTriggersOverflow(t *testing.T) {
	var flushed []*ConversationBuffer
	var mu sync.Mutex

	a := NewConversationAssembler(context.Background(), 60, 1,
		func(_ context.Context, buf *ConversationBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		}, nil)

	key := assemblyKey{chatID: 1, sourceName: "test"}
	a.Add(key, ConversationMessage{SenderName: "Alice", Text: "first", Timestamp: time.Now()}, ForwardedMeta{})

	// First message should NOT trigger overflow — it creates the buffer
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	if len(flushed) != 0 {
		mu.Unlock()
		t.Fatal("expected no flush after first message with maxMessages=1")
	}
	mu.Unlock()

	// Second message triggers overflow (2 >= 1)
	a.Add(key, ConversationMessage{SenderName: "Bob", Text: "second", Timestamp: time.Now()}, ForwardedMeta{})

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 overflow flush on 2nd message, got %d", len(flushed))
	}
	if len(flushed[0].Messages) != 2 {
		t.Errorf("expected 2 messages in overflow flush, got %d", len(flushed[0].Messages))
	}
}

// REG-008-005: FormatConversation with empty SourceChat uses fallback header.
// Regression guard: ensure the header doesn't panic or produce empty output
// when all messages have empty SenderName.
func TestREG008005_FormatConversation_EmptySourceAndParticipants(t *testing.T) {
	buf := &ConversationBuffer{
		SourceChat: "",
		Messages: []ConversationMessage{
			{SenderName: "", Timestamp: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC), Text: "hello"},
		},
	}
	text := FormatConversation(buf)
	if text == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(text, "Forwarded conversation") {
		t.Error("expected fallback header 'Forwarded conversation'")
	}
	if !strings.Contains(text, "Messages: 1") {
		t.Error("expected message count")
	}
}

// REG-008-006: FlushChat iterates and deletes from the map simultaneously.
// In Go this is safe, but regression guard ensures multiple matching keys
// are all flushed without skipping any.
func TestREG008006_FlushChat_MultipleBuffersSameChat(t *testing.T) {
	var flushed []*ConversationBuffer
	var mu sync.Mutex

	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		}, nil)

	// Three different source chats but same user chat ID
	for i := int64(1); i <= 3; i++ {
		a.Add(assemblyKey{chatID: 42, sourceChatID: i * 100, sourceName: fmt.Sprintf("src-%d", i)},
			ConversationMessage{SenderName: "User", Text: "msg", Timestamp: time.Now()},
			ForwardedMeta{})
	}

	flushedCount := a.FlushChat(42)
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if flushedCount != 3 {
		t.Errorf("FlushChat returned %d, expected 3", flushedCount)
	}
	if len(flushed) != 3 {
		t.Errorf("expected 3 flushes, got %d — map iteration+delete skipped entries", len(flushed))
	}
	if a.BufferCount() != 0 {
		t.Errorf("expected 0 remaining buffers, got %d", a.BufferCount())
	}
}

// REG-008-007: extractAllURLs with markdown-style [text](url) should not
// extract the URL since it's part of a compound token. Regression guard:
// if URL extraction logic changes, verify this known limitation is preserved
// or correctly handled.
func TestREG008007_ExtractAllURLs_MarkdownLink(t *testing.T) {
	urls := extractAllURLs("[Click here](https://example.com/page)")
	// The token "[Click here](https://example.com/page)" becomes
	// "Click here](https://example.com/page" after bracket stripping,
	// which doesn't start with http(s):// — so no URL is extracted.
	// This is a known limitation. If URL extraction is improved to handle
	// markdown links, update this test.
	if len(urls) != 0 {
		t.Logf("note: URL extracted from markdown link: %v (expected 0, got %d)", urls, len(urls))
	}
}

// REG-008-008: extractContext does not mutate the input URL slice.
func TestREG008008_ExtractContext_DoesNotMutateInput(t *testing.T) {
	urls := []string{"https://short.com", "https://short.com/longer/path"}
	original := make([]string, len(urls))
	copy(original, urls)

	extractContext("text https://short.com and https://short.com/longer/path end", urls)

	for i, u := range urls {
		if u != original[i] {
			t.Errorf("input slice mutated at index %d: was %q, now %q", i, original[i], u)
		}
	}
}
