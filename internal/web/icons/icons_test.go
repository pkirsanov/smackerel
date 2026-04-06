package icons

import (
	"strings"
	"testing"
)

func TestAllIcons_Count(t *testing.T) {
	all := AllIcons()
	if len(all) != 32 {
		t.Errorf("expected 32 icons, got %d", len(all))
	}
}

func TestSourceIcons_Count(t *testing.T) {
	if len(Source) != 8 {
		t.Errorf("expected 8 source icons, got %d", len(Source))
	}
}

func TestArtifactIcons_Count(t *testing.T) {
	if len(Artifact) != 8 {
		t.Errorf("expected 8 artifact icons, got %d", len(Artifact))
	}
}

func TestStatusIcons_Count(t *testing.T) {
	if len(Status) != 4 {
		t.Errorf("expected 4 status icons, got %d", len(Status))
	}
}

func TestActionIcons_Count(t *testing.T) {
	if len(Action) != 4 {
		t.Errorf("expected 4 action icons, got %d", len(Action))
	}
}

func TestNavigationIcons_Count(t *testing.T) {
	if len(Navigation) != 8 {
		t.Errorf("expected 8 navigation icons, got %d", len(Navigation))
	}
}

func TestAllIcons_ValidSVG(t *testing.T) {
	for name, svg := range AllIcons() {
		t.Run(name, func(t *testing.T) {
			if !strings.HasPrefix(svg, "<svg") {
				t.Errorf("icon %s does not start with <svg", name)
			}
			if !strings.HasSuffix(svg, "</svg>") {
				t.Errorf("icon %s does not end with </svg>", name)
			}
			if !strings.Contains(svg, `viewBox="0 0 24 24"`) {
				t.Errorf("icon %s missing 24x24 viewBox", name)
			}
			if !strings.Contains(svg, `stroke-width="1.5"`) {
				t.Errorf("icon %s missing 1.5px stroke-width", name)
			}
			if !strings.Contains(svg, `stroke="currentColor"`) {
				t.Errorf("icon %s missing currentColor stroke", name)
			}
			if !strings.Contains(svg, `fill="none"`) {
				t.Errorf("icon %s missing fill=none", name)
			}
			if !strings.Contains(svg, `stroke-linecap="round"`) {
				t.Errorf("icon %s missing round linecap", name)
			}
			if !strings.Contains(svg, `stroke-linejoin="round"`) {
				t.Errorf("icon %s missing round linejoin", name)
			}
		})
	}
}

func TestAllIcons_NoEmoji(t *testing.T) {
	for name, svg := range AllIcons() {
		for _, r := range svg {
			if r > 0x1F600 && r < 0x1F9FF {
				t.Errorf("icon %s contains emoji character U+%X", name, r)
			}
		}
	}
}

func TestAllIcons_NoColorFills(t *testing.T) {
	badPatterns := []string{
		"fill=\"#", "fill=\"rgb", "fill=\"hsl",
		"stroke=\"#", "stroke=\"rgb", "stroke=\"hsl",
	}
	for name, svg := range AllIcons() {
		for _, pattern := range badPatterns {
			if strings.Contains(svg, pattern) {
				t.Errorf("icon %s contains hardcoded color: %s", name, pattern)
			}
		}
	}
}
