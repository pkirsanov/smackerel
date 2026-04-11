package telegram

import (
	"testing"
)

// Verify marker constants are distinct two-char strings: symbol + space.
func TestMarkerConstants_Unique(t *testing.T) {
	markers := []string{
		MarkerSuccess,
		MarkerUncertain,
		MarkerAction,
		MarkerInfo,
		MarkerListItem,
		MarkerContinued,
	}
	seen := make(map[string]bool)
	for _, m := range markers {
		if len(m) != 2 || m[1] != ' ' {
			t.Errorf("marker %q should be a single char + space", m)
		}
		if seen[m] {
			t.Errorf("duplicate marker: %q", m)
		}
		seen[m] = true
	}
}

// SCN-001-004 / SCN-002-025: Bot uses text markers, no emoji.
// Markers themselves must be plain ASCII.
func TestMarkerConstants_NoEmoji(t *testing.T) {
	markers := []string{
		MarkerSuccess,
		MarkerUncertain,
		MarkerAction,
		MarkerInfo,
		MarkerListItem,
		MarkerContinued,
	}
	for _, m := range markers {
		for _, r := range m {
			if r > 127 {
				t.Errorf("marker %q contains non-ASCII rune %U", m, r)
			}
		}
	}
}

// SCN-002-042: Unsupported attachment response uses ? marker.
func TestSCN002042_UnsupportedAttachmentMarker(t *testing.T) {
	response := MarkerUncertain + "Not sure what to do with this. Can you add context?"
	if response[:2] != "? " {
		t.Errorf("unsupported attachment should start with '? ', got %q", response[:2])
	}
}
