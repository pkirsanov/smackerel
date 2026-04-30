package drive_test

import (
	"testing"

	"github.com/smackerel/smackerel/internal/drive"
)

// TestNativeGoogleDocRevisionAppendsVersionChainWithoutNewArtifact proves
// SCN-038-011 (BS-007, BS-013, design.md §3.4): a native Google Doc
// revision update MUST refresh the existing artifact identity and append
// the new provider revision id to version_chain — it MUST NOT mint a new
// artifact per revision.
//
// Adversarial guards:
//   - calling AppendRevision twice with the same revisionID MUST NOT
//     duplicate the chain entry (would break Versions tab UX);
//   - empty revisionID MUST NOT extend the chain (would inject blanks
//     into Versions tab);
//   - ProviderArtifactID MUST be revision-independent — if the helper
//     ever folded revisionID into the artifact id, two revisions of the
//     same file would yield two distinct artifact ids and the test would
//     fail with len(distinctArtifacts) != 1.
func TestNativeGoogleDocRevisionAppendsVersionChainWithoutNewArtifact(t *testing.T) {
	const (
		providerID     = "google"
		connectionID   = "conn-038-scope4"
		providerFileID = "native-doc-air-fryer-manual"
	)

	type fakeFile struct {
		artifactID   string
		versionChain []string
	}

	files := map[string]fakeFile{}
	upsertRevision := func(revisionID string) {
		artifactID := drive.ProviderArtifactID(providerID, connectionID, providerFileID)
		existing := files[providerFileID]
		chain := drive.AppendRevision(existing.versionChain, revisionID)
		files[providerFileID] = fakeFile{artifactID: artifactID, versionChain: chain}
	}

	upsertRevision("rev-1")
	upsertRevision("rev-2")
	upsertRevision("rev-2") // duplicate revision must not double-append.
	upsertRevision("")      // empty revision must be a no-op.

	record := files[providerFileID]
	wantArtifactID := "drive:" + providerID + ":" + connectionID + ":" + providerFileID
	if record.artifactID != wantArtifactID {
		t.Fatalf("artifact identity drifted across revisions: got %q, want %q", record.artifactID, wantArtifactID)
	}
	if len(record.versionChain) != 2 || record.versionChain[0] != "rev-1" || record.versionChain[1] != "rev-2" {
		t.Fatalf("version chain = %v, want [rev-1 rev-2] (de-duplicated, empty filtered)", record.versionChain)
	}

	distinctArtifacts := map[string]struct{}{}
	for _, file := range files {
		distinctArtifacts[file.artifactID] = struct{}{}
	}
	if len(distinctArtifacts) != 1 {
		t.Fatalf("native doc revision created %d distinct artifact identities, want exactly 1", len(distinctArtifacts))
	}

	// Adversarial: a different provider_file_id MUST yield a different
	// artifact identity. If ProviderArtifactID ever ignored its inputs
	// and returned a constant id, this guard would fail.
	otherArtifactID := drive.ProviderArtifactID(providerID, connectionID, "different-doc")
	if otherArtifactID == wantArtifactID {
		t.Fatalf("artifact identity collapse: provider_file_id is not part of the identity (got %q for both files)", otherArtifactID)
	}
}

// TestProviderArtifactIDIsRevisionIndependent guards the explicit contract
// from design.md §3.4: the artifact identity MUST NOT vary with revision.
// Without this guard a future refactor could quietly add a revisionID
// parameter and re-introduce the per-revision artifact bug.
func TestProviderArtifactIDIsRevisionIndependent(t *testing.T) {
	first := drive.ProviderArtifactID("google", "conn-x", "doc-1")
	second := drive.ProviderArtifactID("google", "conn-x", "doc-1")
	if first != second || first == "" {
		t.Fatalf("ProviderArtifactID instability: first=%q second=%q", first, second)
	}
	if first != "drive:google:conn-x:doc-1" {
		t.Fatalf("unexpected identity format: got %q", first)
	}
}

// TestAppendRevisionAdversarial covers the per-call contract used by
// scan/monitor: existing chain preserved, no duplicates, empty noop.
func TestAppendRevisionAdversarial(t *testing.T) {
	cases := []struct {
		name   string
		chain  []string
		new    string
		expect []string
	}{
		{name: "empty chain new revision", chain: nil, new: "rev-1", expect: []string{"rev-1"}},
		{name: "preserves existing", chain: []string{"rev-1"}, new: "rev-2", expect: []string{"rev-1", "rev-2"}},
		{name: "rejects duplicate", chain: []string{"rev-1", "rev-2"}, new: "rev-2", expect: []string{"rev-1", "rev-2"}},
		{name: "empty revision noop", chain: []string{"rev-1"}, new: "", expect: []string{"rev-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := drive.AppendRevision(tc.chain, tc.new)
			if len(got) != len(tc.expect) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tc.expect), got)
			}
			for i := range got {
				if got[i] != tc.expect[i] {
					t.Fatalf("entry %d = %q, want %q (got %v)", i, got[i], tc.expect[i], got)
				}
			}
		})
	}
}
