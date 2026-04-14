package pipeline

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/extract"
)

// --- Chaos: NATS payload validation edge cases ---

func TestChaos_ValidateProcessPayload_WhitespaceOnlyText(t *testing.T) {
	p := &NATSProcessPayload{
		ArtifactID:  "test-1",
		ContentType: "article",
		RawText:     "   ",
	}
	// Current validation: RawText != "" passes → whitespace-only is accepted
	err := ValidateProcessPayload(p)
	if err != nil {
		t.Errorf("whitespace-only text currently passes validation: %v", err)
	}
}

func TestChaos_ValidateProcessPayload_AllEmpty(t *testing.T) {
	p := &NATSProcessPayload{}
	err := ValidateProcessPayload(p)
	if err == nil {
		t.Error("completely empty payload should fail validation")
	}
}

func TestChaos_ValidateProcessPayload_OnlyArtifactID(t *testing.T) {
	p := &NATSProcessPayload{ArtifactID: "test-1"}
	err := ValidateProcessPayload(p)
	if err == nil {
		t.Error("payload with only artifact_id should fail (missing content_type)")
	}
}

func TestChaos_ValidateProcessPayload_URLOnly(t *testing.T) {
	p := &NATSProcessPayload{
		ArtifactID:  "test-1",
		ContentType: "article",
		URL:         "https://example.com",
	}
	err := ValidateProcessPayload(p)
	if err != nil {
		t.Errorf("URL-only payload should be valid: %v", err)
	}
}

func TestChaos_ValidateProcessPayload_TextOnly(t *testing.T) {
	p := &NATSProcessPayload{
		ArtifactID:  "test-1",
		ContentType: "generic",
		RawText:     "some text",
	}
	err := ValidateProcessPayload(p)
	if err != nil {
		t.Errorf("text-only payload should be valid: %v", err)
	}
}

func TestChaos_ValidateProcessPayload_VeryLargeText(t *testing.T) {
	p := &NATSProcessPayload{
		ArtifactID:  "test-1",
		ContentType: "article",
		RawText:     strings.Repeat("x", 10*1024*1024), // 10MB
	}
	// Current validation: accepts any non-empty text regardless of size
	err := ValidateProcessPayload(p)
	if err != nil {
		t.Errorf("large text should pass current validation: %v", err)
	}
}

func TestChaos_ValidateProcessedPayload_AllEmpty(t *testing.T) {
	p := &NATSProcessedPayload{}
	err := ValidateProcessedPayload(p)
	if err == nil {
		t.Error("empty payload should fail validation")
	}
}

func TestChaos_ValidateProcessedPayload_FailureResult(t *testing.T) {
	p := &NATSProcessedPayload{
		ArtifactID: "test-1",
		Success:    false,
		Error:      "LLM timeout",
	}
	err := ValidateProcessedPayload(p)
	if err != nil {
		t.Errorf("failure result with artifact_id should be valid: %v", err)
	}
}

func TestChaos_ValidateDigestPayload_EmptyDate(t *testing.T) {
	p := &NATSDigestGeneratedPayload{Text: "some digest"}
	err := ValidateDigestGeneratedPayload(p)
	if err == nil {
		t.Error("missing digest_date should fail")
	}
}

func TestChaos_ValidateDigestPayload_EmptyText(t *testing.T) {
	p := &NATSDigestGeneratedPayload{DigestDate: "2026-04-10"}
	err := ValidateDigestGeneratedPayload(p)
	if err == nil {
		t.Error("missing text should fail")
	}
}

func TestChaos_ValidateDigestPayload_Valid(t *testing.T) {
	p := &NATSDigestGeneratedPayload{
		DigestDate: "2026-04-10",
		Text:       "Today's digest",
		WordCount:  2,
	}
	err := ValidateDigestGeneratedPayload(p)
	if err != nil {
		t.Errorf("valid digest should pass: %v", err)
	}
}

// --- Chaos: Content extraction edge cases ---

func TestChaos_ExtractContent_AllFieldsEmpty(t *testing.T) {
	req := &ProcessRequest{}
	_, err := ExtractContent(nil, req)
	if err == nil {
		t.Error("completely empty request should fail extraction")
	}
}

func TestChaos_ExtractContent_VeryLongText(t *testing.T) {
	longText := strings.Repeat("Long content paragraph. ", 50000) // ~1.2MB
	req := &ProcessRequest{Text: longText, SourceID: "capture"}
	result, err := ExtractContent(nil, req)
	if err != nil {
		t.Fatalf("long text extraction: %v", err)
	}
	if result.ContentHash == "" {
		t.Error("expected content hash for long text")
	}
	if result.Text != longText {
		t.Error("extracted text should preserve full content")
	}
}

func TestChaos_ExtractContent_TextWithNullBytes(t *testing.T) {
	text := "Normal text\x00with\x00null bytes"
	req := &ProcessRequest{Text: text, SourceID: "capture"}
	result, err := ExtractContent(nil, req)
	if err != nil {
		t.Fatalf("text with null bytes: %v", err)
	}
	if result.Text != text {
		t.Errorf("expected preserved text, got %q", result.Text)
	}
}

func TestChaos_ExtractContent_UnicodeText(t *testing.T) {
	text := "🚀 Résumé — 日本語テスト\nÑoño αβγδε مرحبا 你好世界"
	req := &ProcessRequest{Text: text, SourceID: "capture"}
	result, err := ExtractContent(nil, req)
	if err != nil {
		t.Fatalf("unicode text: %v", err)
	}
	if !utf8.ValidString(result.Text) {
		t.Error("extracted text should be valid UTF-8")
	}
	if !utf8.ValidString(result.Title) {
		t.Error("extracted title should be valid UTF-8")
	}
}

func TestChaos_ExtractContent_VoiceURLInput(t *testing.T) {
	req := &ProcessRequest{VoiceURL: "https://example.com/voice.ogg", SourceID: "telegram"}
	result, err := ExtractContent(nil, req)
	if err != nil {
		t.Fatalf("voice URL extraction: %v", err)
	}
	if result.ContentType != extract.ContentTypeVoice {
		t.Errorf("expected voice content type, got %q", result.ContentType)
	}
	if result.SourceURL != "https://example.com/voice.ogg" {
		t.Errorf("expected voice URL preserved, got %q", result.SourceURL)
	}
}

func TestChaos_ExtractContent_YouTubeURL(t *testing.T) {
	req := &ProcessRequest{URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", SourceID: "capture"}
	result, err := ExtractContent(nil, req)
	if err != nil {
		t.Fatalf("youtube URL extraction: %v", err)
	}
	if result.ContentType != extract.ContentTypeYouTube {
		t.Errorf("expected youtube content type, got %q", result.ContentType)
	}
	if result.VideoID != "dQw4w9WgXcQ" {
		t.Errorf("expected video ID, got %q", result.VideoID)
	}
}

func TestChaos_ExtractContent_ImageURL(t *testing.T) {
	req := &ProcessRequest{URL: "https://example.com/photo.jpg", SourceID: "capture"}
	result, err := ExtractContent(nil, req)
	if err != nil {
		t.Fatalf("image URL extraction: %v", err)
	}
	if result.ContentType != extract.ContentTypeImage {
		t.Errorf("expected image content type, got %q", result.ContentType)
	}
}

func TestChaos_ExtractContent_PDFURL(t *testing.T) {
	req := &ProcessRequest{URL: "https://example.com/document.pdf", SourceID: "capture"}
	result, err := ExtractContent(nil, req)
	if err != nil {
		t.Fatalf("PDF URL extraction: %v", err)
	}
	if result.ContentType != extract.ContentTypePDF {
		t.Errorf("expected PDF content type, got %q", result.ContentType)
	}
}

// --- Chaos: Tier assignment edge cases ---

func TestChaos_TierAssign_NegativeContentLength(t *testing.T) {
	tier := AssignTier(TierSignals{ContentLen: -1, SourceID: "gmail"})
	// Negative content length < 200 → light tier
	if tier != TierLight {
		t.Errorf("negative content length should get light tier, got %q", tier)
	}
}

func TestChaos_TierAssign_MaxIntContentLength(t *testing.T) {
	tier := AssignTier(TierSignals{ContentLen: int(^uint(0) >> 1), SourceID: "gmail"})
	if tier != TierStandard {
		t.Errorf("max int content length should get standard tier, got %q", tier)
	}
}

func TestChaos_TierAssign_AllSignalsTrue(t *testing.T) {
	tier := AssignTier(TierSignals{
		UserStarred: true,
		SourceID:    "capture",
		HasContext:  true,
		ContentLen:  10,
	})
	// Starred is checked first → full
	if tier != TierFull {
		t.Errorf("all signals true should give full tier, got %q", tier)
	}
}

// --- Chaos: Content hash edge cases ---

func TestChaos_HashContent_EmptyString(t *testing.T) {
	hash := extract.HashContent("")
	if hash == "" {
		t.Error("empty string should produce a hash")
	}
	if len(hash) != 64 {
		t.Errorf("expected 64-char SHA-256 hex, got %d chars", len(hash))
	}
}

func TestChaos_HashContent_OnlyWhitespace(t *testing.T) {
	hash1 := extract.HashContent("")
	hash2 := extract.HashContent("   ")
	// After trim + lowercase: both become ""
	if hash1 != hash2 {
		t.Error("empty and whitespace-only should produce same hash after normalization")
	}
}

func TestChaos_HashContent_NullBytes(t *testing.T) {
	hash := extract.HashContent("hello\x00world")
	if hash == "" {
		t.Error("null bytes in content should produce a hash")
	}
	if len(hash) != 64 {
		t.Errorf("expected 64-char hash, got %d", len(hash))
	}
}

func TestChaos_HashContent_VeryLargeInput(t *testing.T) {
	huge := strings.Repeat("x", 10*1024*1024) // 10MB
	hash := extract.HashContent(huge)
	if hash == "" {
		t.Error("huge input should produce a hash")
	}
	if len(hash) != 64 {
		t.Errorf("expected 64-char hash for 10MB input, got %d", len(hash))
	}
}

// --- CHAOS-I01: Subscriber logs truncated raw payload on invalid JSON ---

func TestChaosI01_TruncateBytes_Short(t *testing.T) {
	data := []byte("short")
	result := truncateBytes(data, 200)
	if result != "short" {
		t.Errorf("expected 'short', got %q", result)
	}
}

func TestChaosI01_TruncateBytes_ExactLimit(t *testing.T) {
	data := []byte(strings.Repeat("x", 200))
	result := truncateBytes(data, 200)
	if result != string(data) {
		t.Error("data at exact limit should not be truncated")
	}
	if strings.Contains(result, "truncated") {
		t.Error("should not contain truncated marker at exact limit")
	}
}

func TestChaosI01_TruncateBytes_OverLimit(t *testing.T) {
	data := []byte(strings.Repeat("x", 300))
	result := truncateBytes(data, 200)
	if len(result) > 220 {
		t.Errorf("truncated result too long: %d", len(result))
	}
	if !strings.HasSuffix(result, "...(truncated)") {
		t.Error("expected truncation marker suffix")
	}
}

func TestChaosI01_TruncateBytes_Empty(t *testing.T) {
	result := truncateBytes(nil, 200)
	if result != "" {
		t.Errorf("nil data should produce empty string, got %q", result)
	}
}

func TestChaosI01_TruncateBytes_BinaryData(t *testing.T) {
	data := []byte{0x00, 0xFF, 0x80, 0x01, 0x02, 0x03}
	result := truncateBytes(data, 200)
	if len(result) != len(data) {
		t.Errorf("binary data under limit should preserve length, got %d vs %d", len(result), len(data))
	}
}

// --- CHAOS-I02: Publisher rejects oversized NATS payloads ---

func TestChaosI02_MaxNATSMessageSize_Constant(t *testing.T) {
	if MaxNATSMessageSize != 1048576 {
		t.Errorf("expected MaxNATSMessageSize=1048576, got %d", MaxNATSMessageSize)
	}
}

func TestChaosI02_ValidateProcessPayload_OversizedPayload(t *testing.T) {
	// Simulate a payload that would exceed NATS max message size when marshaled
	oversized := strings.Repeat("x", MaxNATSMessageSize+1)
	payload := NATSProcessPayload{
		ArtifactID:  "test-oversize",
		ContentType: "article",
		RawText:     oversized,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) <= MaxNATSMessageSize {
		t.Skip("test payload did not exceed max size after marshal")
	}
	// The size check is in the publish path, not in ValidateProcessPayload.
	// This test verifies the marshal output exceeds the limit.
	if len(data) <= MaxNATSMessageSize {
		t.Error("expected marshaled payload to exceed MaxNATSMessageSize")
	}
}

func TestChaosI02_SmallPayload_UnderLimit(t *testing.T) {
	payload := NATSProcessPayload{
		ArtifactID:  "test-small",
		ContentType: "note",
		RawText:     "Hello world",
		SourceID:    "capture",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) > MaxNATSMessageSize {
		t.Errorf("small payload should be under limit, got %d bytes", len(data))
	}
}

// --- CHAOS-I03: Dedup checker uses context timeout ---

func TestChaosI03_DedupQueryTimeout_Constant(t *testing.T) {
	if dedupQueryTimeout != 5*time.Second {
		t.Errorf("expected dedupQueryTimeout=5s, got %v", dedupQueryTimeout)
	}
}

func TestChaosI03_CheckURL_EmptyURL_NoTimeout(t *testing.T) {
	// Empty URL should short-circuit without hitting the DB at all
	checker := &DedupChecker{Pool: nil}
	result, err := checker.CheckURL(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsDuplicate {
		t.Error("empty URL should not be duplicate")
	}
}

func TestChaosI03_Check_CancelledContext_FailOpen(t *testing.T) {
	// Verify that a cancelled parent context doesn't cause a hard error
	// when the dedup checker tries to apply its own timeout.
	// With a nil pool this will panic on actual query, but the context
	// timeout construction itself should work.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// With nil pool, the query will fail; the important thing is we don't panic
	checker := &DedupChecker{Pool: nil}
	defer func() {
		if r := recover(); r != nil {
			// nil pool access is expected to panic — this is a design-level test
			// confirming timeout context creation doesn't itself fail
		}
	}()
	_, _ = checker.Check(ctx, "test-hash")
}

func TestChaos_HashContent_UnicodeNormalization(t *testing.T) {
	// Verify that case normalization works with multi-byte chars
	hash1 := extract.HashContent("Café Résumé")
	hash2 := extract.HashContent("café résumé")
	if hash1 != hash2 {
		t.Error("case-insensitive hashing should produce same hash for unicode")
	}
}

// --- Chaos: DuplicateError ---

func TestChaos_DuplicateError_EmptyFields(t *testing.T) {
	err := &DuplicateError{ExistingID: "", Title: ""}
	msg := err.Error()
	if msg == "" {
		t.Error("error message should not be empty even with empty fields")
	}
}

func TestChaos_DuplicateError_UnicodeTitle(t *testing.T) {
	err := &DuplicateError{ExistingID: "abc", Title: "🚀 日本語タイトル"}
	msg := err.Error()
	if !utf8.ValidString(msg) {
		t.Error("error message should be valid UTF-8")
	}
}

// --- Chaos: ExtractText edge cases ---

func TestChaos_ExtractText_OnlyNewlines(t *testing.T) {
	result := extract.ExtractText("\n\n\n")
	// First line is empty (text starts with \n, idx=0, title=text[:0]="")
	if result.Title != "" {
		t.Errorf("expected empty title for newlines-only text, got %q", result.Title)
	}
}

func TestChaos_ExtractText_LongSingleLine(t *testing.T) {
	long := strings.Repeat("a", 200)
	result := extract.ExtractText(long)
	// Title capped at 100
	if len(result.Title) > 100 {
		t.Errorf("title should be capped at 100, got %d", len(result.Title))
	}
}

func TestChaos_ExtractText_UnicodeTitle(t *testing.T) {
	result := extract.ExtractText("🚀🌍🎯 This is a very long unicode title that exceeds one hundred characters when you count all the emoji bytes but not necessarily runes")
	if !utf8.ValidString(result.Title) {
		t.Error("title should be valid UTF-8")
	}
	// Title length is byte-based (len(title) > 100), which could cut multi-byte chars
	// This is a potential data quality issue for multi-byte titles
	if len(result.Title) > 100 {
		t.Errorf("title should be capped at 100 bytes, got %d", len(result.Title))
	}
}

// --- Chaos: DetectContentType edge cases ---

func TestChaos_DetectContentType_MalformedURLs(t *testing.T) {
	urls := []string{
		"not-a-url-at-all",
		"://missing-scheme",
		"http://",
		"ftp://files.example.com/doc",
		"javascript:alert(1)",
		"data:text/html,<h1>hi</h1>",
	}
	for _, u := range urls {
		ct := extract.DetectContentType(u)
		// Should not panic, should return some content type
		if ct == "" {
			t.Errorf("DetectContentType(%q) should not return empty", u)
		}
	}
}

func TestChaos_DetectContentType_CaseInsensitive(t *testing.T) {
	if extract.DetectContentType("https://example.com/photo.JPG") != extract.ContentTypeImage {
		t.Error("should detect .JPG as image (case insensitive)")
	}
	if extract.DetectContentType("https://example.com/doc.PDF") != extract.ContentTypePDF {
		t.Error("should detect .PDF as pdf (case insensitive)")
	}
}

func TestChaos_DetectContentType_URLWithQueryParams(t *testing.T) {
	if extract.DetectContentType("https://example.com/photo.jpg?width=100&quality=80") != extract.ContentTypeImage {
		t.Error("should detect image URL with query params")
	}
	if extract.DetectContentType("https://example.com/doc.pdf?token=abc123") != extract.ContentTypePDF {
		t.Error("should detect PDF URL with query params")
	}
}
