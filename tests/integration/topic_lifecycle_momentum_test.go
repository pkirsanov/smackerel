//go:build integration

package integration

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/topics"
)

const momentumTolerance = 0.0001

type topicMomentumResult struct {
	momentum float64
	state    string
}

func TestTopicLifecycleMomentumFromPersistedStars(t *testing.T) {
	pool := testPool(t)
	fixturePrefix := testID(t)
	registerTopicMomentumCleanup(t, pool, fixturePrefix)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	assertTopicsStarCountColumnAbsent(t, ctx, pool)
	seedTopicMomentumFixtures(t, ctx, pool, fixturePrefix)
	assertDuplicateBelongsToRejected(t, ctx, pool, fixturePrefix)

	lifecycle := topics.NewLifecycle(pool)
	if err := lifecycle.UpdateAllMomentum(ctx); err != nil {
		t.Fatalf("UpdateAllMomentum against canonical migrations: %v", err)
	}

	zeroStar := readTopicMomentum(t, ctx, pool, fixturePrefix+"-topic-zero")
	assertTopicMomentum(t, "zero-star", zeroStar, 0.5, string(topics.StateDormant))
	t.Logf("zero-star persisted momentum=%.4f state=%s", zeroStar.momentum, zeroStar.state)

	multipleStars := readTopicMomentum(t, ctx, pool, fixturePrefix+"-topic-multiple")
	assertTopicMomentum(t, "multiple-stars", multipleStars, 11.5, string(topics.StateActive))
	t.Logf("multiple-stars persisted momentum=%.4f state=%s", multipleStars.momentum, multipleStars.state)

	t.Log("PASS: canonical topics schema has no star_count column")
	t.Log("PASS: zero linked starred artifacts contribute 0.0 star momentum")
	t.Log("PASS: one linked unstarred artifact contributes only 0.5 connection momentum")
	t.Log("PASS: two distinct linked starred artifacts contribute exactly 10.0 star momentum")
	t.Log("PASS: three linked relationships contribute exactly 1.5 connection momentum")
	t.Log("PASS: an unrelated starred artifact contributes nothing to the tested topic")
}

func assertTopicsStarCountColumnAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	var columnExists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'topics'
			  AND column_name = 'star_count'
		)
	`).Scan(&columnExists)
	if err != nil {
		t.Fatalf("inspect canonical topics columns: %v", err)
	}
	if columnExists {
		t.Fatal("canonical topics schema unexpectedly contains star_count")
	}
}

func seedTopicMomentumFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool, prefix string) {
	t.Helper()

	_, err := pool.Exec(ctx, `
		INSERT INTO topics (
			id, name, state, momentum_score, capture_count_total,
			capture_count_30d, capture_count_90d, search_hit_count_30d, last_active
		) VALUES
			($1, $2, 'emerging', -1.0, 1, 0, 0, 0, NOW()),
			($3, $4, 'emerging', -1.0, 3, 0, 0, 0, NOW()),
			($5, $6, 'emerging', -1.0, 1, 0, 0, 0, NOW())
	`,
		prefix+"-topic-zero", prefix+"-zero",
		prefix+"-topic-multiple", prefix+"-multiple",
		prefix+"-topic-unrelated", prefix+"-unrelated",
	)
	if err != nil {
		t.Fatalf("insert topic momentum fixtures: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO artifacts (
			id, artifact_type, title, content_hash, source_id, user_starred
		) VALUES
			($1, 'test', 'zero topic unstarred', $2, 'test-topic-momentum', FALSE),
			($3, 'test', 'multiple topic starred one', $4, 'test-topic-momentum', TRUE),
			($5, 'test', 'multiple topic starred two', $6, 'test-topic-momentum', TRUE),
			($7, 'test', 'multiple topic unstarred', $8, 'test-topic-momentum', FALSE),
			($9, 'test', 'unrelated topic starred', $10, 'test-topic-momentum', TRUE)
	`,
		prefix+"-artifact-zero", prefix+"-hash-zero",
		prefix+"-artifact-star-one", prefix+"-hash-star-one",
		prefix+"-artifact-star-two", prefix+"-hash-star-two",
		prefix+"-artifact-unstarred", prefix+"-hash-unstarred",
		prefix+"-artifact-unrelated-star", prefix+"-hash-unrelated-star",
	)
	if err != nil {
		t.Fatalf("insert artifact momentum fixtures: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO edges (
			id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata
		) VALUES
			($1, 'artifact', $2, 'topic', $3, 'BELONGS_TO', 1.0, '{}'),
			($4, 'artifact', $5, 'topic', $6, 'BELONGS_TO', 1.0, '{}'),
			($7, 'artifact', $8, 'topic', $9, 'BELONGS_TO', 1.0, '{}'),
			($10, 'artifact', $11, 'topic', $12, 'BELONGS_TO', 1.0, '{}'),
			($13, 'artifact', $14, 'topic', $15, 'BELONGS_TO', 1.0, '{}')
	`,
		prefix+"-edge-zero", prefix+"-artifact-zero", prefix+"-topic-zero",
		prefix+"-edge-star-one", prefix+"-artifact-star-one", prefix+"-topic-multiple",
		prefix+"-edge-star-two", prefix+"-artifact-star-two", prefix+"-topic-multiple",
		prefix+"-edge-unstarred", prefix+"-artifact-unstarred", prefix+"-topic-multiple",
		prefix+"-edge-unrelated-star", prefix+"-artifact-unrelated-star", prefix+"-topic-unrelated",
	)
	if err != nil {
		t.Fatalf("insert edge momentum fixtures: %v", err)
	}
}

func assertDuplicateBelongsToRejected(t *testing.T, ctx context.Context, pool *pgxpool.Pool, prefix string) {
	t.Helper()

	_, err := pool.Exec(ctx, `
		INSERT INTO edges (
			id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata
		) VALUES ($1, 'artifact', $2, 'topic', $3, 'BELONGS_TO', 1.0, '{}')
	`, prefix+"-edge-star-one-duplicate", prefix+"-artifact-star-one", prefix+"-topic-multiple")
	if err == nil {
		t.Fatal("canonical schema accepted a duplicate artifact-to-topic BELONGS_TO relationship")
	}

	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		t.Fatalf("duplicate BELONGS_TO returned non-PostgreSQL error: %v", err)
	}
	if postgresError.Code != "23505" || postgresError.ConstraintName != "edges_src_type_src_id_dst_type_dst_id_edge_type_key" {
		t.Fatalf("duplicate BELONGS_TO error = code %s constraint %s, want canonical relationship uniqueness", postgresError.Code, postgresError.ConstraintName)
	}

	t.Log("PASS: canonical relationship uniqueness rejects duplicate BELONGS_TO edges")
}

func readTopicMomentum(t *testing.T, ctx context.Context, pool *pgxpool.Pool, topicID string) topicMomentumResult {
	t.Helper()

	var result topicMomentumResult
	if err := pool.QueryRow(ctx, `
		SELECT momentum_score, state
		FROM topics
		WHERE id = $1
	`, topicID).Scan(&result.momentum, &result.state); err != nil {
		t.Fatalf("read topic momentum %s: %v", topicID, err)
	}
	return result
}

func assertTopicMomentum(t *testing.T, label string, result topicMomentumResult, expectedMomentum float64, expectedState string) {
	t.Helper()

	if math.Abs(result.momentum-expectedMomentum) > momentumTolerance {
		t.Errorf("%s momentum = %.4f, want %.4f", label, result.momentum, expectedMomentum)
	}
	if result.state != expectedState {
		t.Errorf("%s state = %q, want %q", label, result.state, expectedState)
	}
}

func registerTopicMomentumCleanup(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		likePattern := prefix + "%"
		for _, statement := range []string{
			`DELETE FROM edges WHERE id LIKE $1`,
			`DELETE FROM artifacts WHERE id LIKE $1`,
			`DELETE FROM topics WHERE id LIKE $1`,
		} {
			if _, err := pool.Exec(ctx, statement, likePattern); err != nil {
				t.Errorf("cleanup topic momentum fixture with %q: %v", statement, err)
			}
		}
	})
}
