package pipeline

import (
	"testing"
	"unicode/utf8"
)

// --- truncateBytes basic cases ---

func TestTruncateBytes_ShortData(t *testing.T) {
	data := []byte("hello")
	result := truncateBytes(data, 100)
	if result != "hello" {
		t.Errorf("short data should be returned unchanged, got %q", result)
	}
}

func TestTruncateBytes_ExactLength(t *testing.T) {
	data := []byte("12345")
	result := truncateBytes(data, 5)
	if result != "12345" {
		t.Errorf("data at exact maxLen should be returned unchanged, got %q", result)
	}
}

func TestTruncateBytes_EmptyData(t *testing.T) {
	result := truncateBytes([]byte{}, 10)
	if result != "" {
		t.Errorf("empty data should return empty string, got %q", result)
	}
}

func TestTruncateBytes_ASCIITruncation(t *testing.T) {
	data := []byte("abcdefghij")
	result := truncateBytes(data, 5)
	if result != "abcde...(truncated)" {
		t.Errorf("expected truncated ASCII, got %q", result)
	}
}

func TestTruncateBytes_ValidUTF8Output(t *testing.T) {
	data := []byte("日本語テスト")
	result := truncateBytes(data, 7)
	// "日" = 3 bytes, "本" = 3 bytes, "語" starts at byte 6 (3 bytes)
	// Truncating at 7 splits "語" → should step back to byte 6
	if !utf8.ValidString(result) {
		t.Errorf("truncated result should be valid UTF-8, got %q", result)
	}
}

// --- truncateUTF8 basic cases ---

func TestTruncateUTF8_ShortString(t *testing.T) {
	result := truncateUTF8("hello", 100)
	if result != "hello" {
		t.Errorf("short string should be returned unchanged, got %q", result)
	}
}

func TestTruncateUTF8_ExactLength(t *testing.T) {
	result := truncateUTF8("12345", 5)
	if result != "12345" {
		t.Errorf("string at exact maxBytes should be returned unchanged, got %q", result)
	}
}

func TestTruncateUTF8_EmptyString(t *testing.T) {
	result := truncateUTF8("", 10)
	if result != "" {
		t.Errorf("empty string should return empty, got %q", result)
	}
}

func TestTruncateUTF8_ASCIITruncation(t *testing.T) {
	result := truncateUTF8("abcdefghij", 5)
	if result != "abcde" {
		t.Errorf("expected 'abcde', got %q", result)
	}
}

func TestTruncateUTF8_FourByteEmoji(t *testing.T) {
	// "ab😀cd" → "ab" = 2 bytes, "😀" = 4 bytes (bytes 2-5), "cd" = 2 bytes
	result := truncateUTF8("ab😀cd", 4)
	// maxBytes=4 would cut the emoji → should step back to byte 2
	if len(result) > 4 {
		t.Errorf("expected at most 4 bytes, got %d", len(result))
	}
	if !utf8.ValidString(result) {
		t.Errorf("result should be valid UTF-8, got %q", result)
	}
	if result != "ab" {
		t.Errorf("expected 'ab' (emoji excluded), got %q", result)
	}
}
