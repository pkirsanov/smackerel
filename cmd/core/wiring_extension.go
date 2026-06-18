// Spec 095 F-095-EXT-INGEST — share the evergreen Scorer with the secondary
// (spec-058 Chrome-extension) ingestion front door.
//
// The PRIMARY connector front door (buildCoreServices, cmd/core/services.go)
// injects svc.evergreenScorer into its pipeline.RawArtifactPublisher so
// connector-ingested artifacts are scored at the ingestion front door (spec 095
// SCOPE-07 / PKT-095-B). The spec-058 extension ingest path
// (extensioningest.NewHandler) builds its OWN publisher in buildAPIDeps
// (cmd/core/wiring.go); until this increment it did NOT carry the scorer, so
// extension-captured artifacts persisted a NULL evergreen_score (safe per
// Principle 9 / NFR-3 — NULL is treated as not-excluded downstream — but
// unscored). This helper mirrors the connector-path injection so BOTH ingestion
// surfaces score identically.
package main

import (
	"github.com/smackerel/smackerel/internal/pipeline"
	"github.com/smackerel/smackerel/internal/retrieval/evergreen"
)

// shareEvergreenScorer injects the connector front door's evergreen Scorer into
// the spec-058 extension ingest publisher (F-095-EXT-INGEST) so both ingestion
// surfaces score identically.
//
// nil-safe + typed-nil-safe: the Scorer field is set ONLY when the concrete
// *evergreen.Scorer is non-nil, mirroring the services.go connector-path guard.
// When retrieval.evergreen.enabled=false svc.evergreenScorer is nil; leaving
// pub.Scorer a nil interface makes PublishRawArtifact persist a NULL
// evergreen_score and keeps ingestion byte-for-byte unchanged (NFR-3,
// Principle 9). Assigning a typed-nil *evergreen.Scorer into the interface field
// would instead make pub.Scorer != nil and panic in scoreEvergreen — the
// concrete-nil guard prevents that.
func shareEvergreenScorer(pub *pipeline.RawArtifactPublisher, scorer *evergreen.Scorer) {
	if pub == nil || scorer == nil {
		return
	}
	pub.Scorer = scorer
}
