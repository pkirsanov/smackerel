package photos

import (
	"testing"
)

// TestEditorSignatureMapping_AllSupportedEditors covers SCN-040-007: the
// stable-signal portion of the lifecycle pipeline must recognise every
// editor named in the planning artifact (Lightroom, Darktable, GIMP,
// RawTherapee, DaVinci Resolve) without inventing matches for unknown
// software strings.
func TestEditorSignatureMapping_AllSupportedEditors(t *testing.T) {
	cases := []struct {
		name       string
		exif       map[string]any
		wantSig    EditorSignature
		wantVer    string
		wantUnknwn bool
	}{
		{
			name:    "lightroom_classic_software_field",
			exif:    map[string]any{"Software": "Adobe Lightroom Classic 13.2 (Macintosh)"},
			wantSig: EditorLightroomClass,
			wantVer: "Adobe Lightroom Classic 13.2 (Macintosh)",
		},
		{
			name:    "lightroom_cloud_software_field",
			exif:    map[string]any{"software": "Adobe Lightroom 7.4 (Android)"},
			wantSig: EditorLightroomCC,
			wantVer: "Adobe Lightroom 7.4 (Android)",
		},
		{
			name:    "darktable_processing_software",
			exif:    map[string]any{"ProcessingSoftware": "darktable 4.6.1"},
			wantSig: EditorDarktable,
			wantVer: "darktable 4.6.1",
		},
		{
			name:    "gimp_creator_tool",
			exif:    map[string]any{"CreatorTool": "GIMP 2.10.36"},
			wantSig: EditorGIMP,
			wantVer: "GIMP 2.10.36",
		},
		{
			name:    "rawtherapee_software_field",
			exif:    map[string]any{"Software": "RawTherapee 5.10"},
			wantSig: EditorRawTherapee,
			wantVer: "RawTherapee 5.10",
		},
		{
			name:    "davinci_resolve_creator_tool",
			exif:    map[string]any{"CreatorTool": "DaVinci Resolve 19.0"},
			wantSig: EditorDaVinciResolve,
			wantVer: "DaVinci Resolve 19.0",
		},
		{
			name:       "phone_camera_no_editor",
			exif:       map[string]any{"Software": "iOS 17.5.1"},
			wantSig:    EditorUnknown,
			wantUnknwn: true,
			wantVer:    "iOS 17.5.1",
		},
		{
			name:       "missing_software_field_returns_unknown",
			exif:       map[string]any{"camera": "Synthetic Camera"},
			wantSig:    EditorUnknown,
			wantUnknwn: true,
		},
		{
			name:       "empty_exif_is_unknown",
			exif:       nil,
			wantSig:    EditorUnknown,
			wantUnknwn: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EditorSignatureFromEXIF(tc.exif)
			if got != tc.wantSig {
				t.Fatalf("EditorSignatureFromEXIF: got %q, want %q", got, tc.wantSig)
			}
			if gotVer := EditorVersionFromEXIF(tc.exif); gotVer != tc.wantVer {
				t.Fatalf("EditorVersionFromEXIF: got %q, want %q", gotVer, tc.wantVer)
			}
			if tc.wantUnknwn && got != EditorUnknown {
				t.Fatalf("expected EditorUnknown for %s, got %q", tc.name, got)
			}
		})
	}

	// Adversarial: a plausible-but-unsupported editor must NOT collide
	// with any supported signature. This catches future code that adds a
	// permissive substring match (e.g., "photo" / "edit") and silently
	// promotes random files into the lifecycle pipeline.
	for _, fake := range []string{"PhotoMagic Pro 2.0", "EditX Studio 9", "Photoshop 26.0"} {
		got := EditorSignatureFromEXIF(map[string]any{"Software": fake})
		if got != EditorUnknown {
			t.Fatalf("unsupported editor %q matched %q, expected EditorUnknown", fake, got)
		}
	}

	// Sanity: SupportedEditorSignatures returns the same set we asserted
	// above; if the list grows, the test plan owner must extend coverage.
	want := map[EditorSignature]struct{}{
		EditorLightroomCC:    {},
		EditorLightroomClass: {},
		EditorDarktable:      {},
		EditorGIMP:           {},
		EditorRawTherapee:    {},
		EditorDaVinciResolve: {},
	}
	for _, sig := range SupportedEditorSignatures() {
		if _, ok := want[sig]; !ok {
			t.Fatalf("SupportedEditorSignatures returned unexpected %q", sig)
		}
		delete(want, sig)
	}
	if len(want) > 0 {
		t.Fatalf("SupportedEditorSignatures missing entries: %v", want)
	}
}
