//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/db"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// provisionStoresForCaptureDurability brings a RAW postgres + NATS pair (as stood
// up by the stores-only `integration-light` lane, which runs NO core) up to the
// schema + JetStream-stream state core establishes at startup in the heavy lane —
// by calling the SAME production functions core calls, so there is NO schema or
// stream-creation duplication / drift:
//
//   - db.Migrate (internal/db/migrate.go) — embedded //go:embed SQL migrations,
//     advisory-locked, idempotent via the schema_migrations guard. cmd/core invokes
//     it in buildCoreServices right after db.Connect ("Run schema migrations").
//   - (*smacknats.Client).EnsureStreams (internal/nats/client.go) — CreateOrUpdateStream
//     per smacknats.AllStreams(), idempotent. cmd/core invokes it right after
//     smacknats.Connect ("Ensure JetStream streams").
//
// Both production functions are idempotent, so calling this at the top of a
// durability test is the missing-fixture fix in the light lane (no core ran) AND a
// harmless no-op in the heavy lane (core already provisioned both). It is scoped to
// the durability test's setup deliberately — there is no package-level TestMain — so
// the existing heavy-lane integration tests are unaffected.
func provisionStoresForCaptureDurability(t *testing.T, pool *pgxpool.Pool, client *smacknats.Client) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// (a) Apply the production DB schema migrations (creates the artifacts table
	// et al. that the durability assertions read).
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("provision stores: db.Migrate (production migration entrypoint) failed: %v", err)
	}

	// (b) Ensure the production JetStream streams (creates ARTIFACTS et al. that
	// the durable NATS publish lands in).
	if err := client.EnsureStreams(ctx, captureDurabilityStreamCaps()); err != nil {
		t.Fatalf("provision stores: EnsureStreams (production stream entrypoint) failed: %v", err)
	}
}

// captureDurabilityStreamCaps builds the per-stream MaxBytes map EnsureStreams
// requires (spec 046 FR-046-003 forbids unbounded streams). Stream NAMES are
// enumerated from the production smacknats.AllStreams() (no name drift), and each
// cap is sourced from the SAME SST envelope production consumes —
// NATS_STREAM_MAX_BYTES_JSON, which the lane injects via --env-file test.env. A
// positive fallback covers any stream absent from the envelope so EnsureStreams'
// "every stream MUST have a positive bound" contract always holds; the durability
// test asserts on stream existence + publish landing, never on the byte value, so
// the fallback is sound.
func captureDurabilityStreamCaps() map[string]int64 {
	parsed := map[string]int64{}
	if raw := os.Getenv("NATS_STREAM_MAX_BYTES_JSON"); raw != "" {
		var entries []struct {
			Stream string `json:"stream"`
			Bytes  int64  `json:"bytes"`
		}
		if err := json.Unmarshal([]byte(raw), &entries); err == nil {
			for _, e := range entries {
				if e.Stream != "" && e.Bytes > 0 {
					parsed[e.Stream] = e.Bytes
				}
			}
		}
	}

	const fallbackCap int64 = 1 << 30 // 1 GiB positive bound; value is never asserted on
	caps := make(map[string]int64, len(smacknats.AllStreams()))
	for _, sc := range smacknats.AllStreams() {
		if v, ok := parsed[sc.Name]; ok {
			caps[sc.Name] = v
		} else {
			caps[sc.Name] = fallbackCap
		}
	}
	return caps
}
