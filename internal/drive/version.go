package drive

// ProviderArtifactID is the canonical artifact identifier for a provider
// drive file. It is intentionally derived ONLY from
// (provider_id, connection_id, provider_file_id) and MUST NOT incorporate
// the provider revision identifier so that successive native Google Doc
// revisions update the same artifact identity rather than fragment into
// per-revision artifacts.
//
// Spec 038 Scope 4 SCN-038-011 / BS-007 / BS-013 and design.md §3.4
// require this stability: the head version is reachable through
// artifacts.id while prior provider revisions are recorded in
// drive_files.version_chain.
func ProviderArtifactID(providerID, connectionID, providerFileID string) string {
	return "drive:" + providerID + ":" + connectionID + ":" + providerFileID
}

// AppendRevision returns the version chain with revisionID appended,
// preserving existing entries in order and de-duplicating. An empty
// revisionID is a no-op so callers can safely chain native-doc updates
// even when a provider omits the revision identifier.
func AppendRevision(chain []string, revisionID string) []string {
	if revisionID == "" {
		return chain
	}
	for _, existing := range chain {
		if existing == revisionID {
			return chain
		}
	}
	return append(chain, revisionID)
}
