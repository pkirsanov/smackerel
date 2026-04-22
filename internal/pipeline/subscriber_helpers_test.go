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

// truncateUTF8 tests removed — local truncateUTF8 was eliminated in favour of
// stringutil.TruncateUTF8 (SMP-022-001). Coverage lives in stringutil_test.go.
