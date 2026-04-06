package digest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// Generator assembles and generates daily digests.
type Generator struct {
	Pool *pgxpool.Pool
	NATS *smacknats.Client
}

// NewGenerator creates a new digest generator.
func NewGenerator(pool *pgxpool.Pool, nats *smacknats.Client) *Generator {
	return &Generator{Pool: pool, NATS: nats}
}

// DigestContext is the context payload sent to the ML sidecar for digest generation.
type DigestContext struct {
	DigestDate         string          `json:"digest_date"`
	ActionItems        []ActionItem    `json:"action_items"`
	OvernightArtifacts []ArtifactBrief `json:"overnight_artifacts"`
	HotTopics          []TopicBrief    `json:"hot_topics"`
}

// ActionItem is a pending action item for the digest.
type ActionItem struct {
	Text        string `json:"text"`
	Person      string `json:"person"`
	DaysWaiting int    `json:"days_waiting"`
}

// ArtifactBrief is a summary of an artifact for the digest.
type ArtifactBrief struct {
	Title string `json:"title"`
	Type  string `json:"type"`
}

// TopicBrief is a summary of a hot topic for the digest.
type TopicBrief struct {
	Name             string `json:"name"`
	CapturesThisWeek int    `json:"captures_this_week"`
}

// Digest is a generated digest record.
type Digest struct {
	ID          string    `json:"id"`
	DigestDate  string    `json:"date"`
	DigestText  string    `json:"text"`
	WordCount   int       `json:"word_count"`
	ActionItems []byte    `json:"action_items,omitempty"`
	HotTopics   []byte    `json:"hot_topics,omitempty"`
	IsQuiet     bool      `json:"is_quiet"`
	ModelUsed   string    `json:"model_used,omitempty"`
	CreatedAt   time.Time `json:"generated_at"`
}

// Generate assembles the context and triggers digest generation via NATS.
func (g *Generator) Generate(ctx context.Context) (*DigestContext, error) {
	today := time.Now().Format("2006-01-02")

	// Assemble action items
	actionItems, err := g.getPendingActionItems(ctx)
	if err != nil {
		slog.Warn("failed to get action items for digest", "error", err)
	}

	// Assemble overnight artifacts
	overnight, err := g.getOvernightArtifacts(ctx)
	if err != nil {
		slog.Warn("failed to get overnight artifacts for digest", "error", err)
	}

	// Assemble hot topics
	hotTopics, err := g.getHotTopics(ctx)
	if err != nil {
		slog.Warn("failed to get hot topics for digest", "error", err)
	}

	digestCtx := &DigestContext{
		DigestDate:         today,
		ActionItems:        actionItems,
		OvernightArtifacts: overnight,
		HotTopics:          hotTopics,
	}

	// Check for quiet day
	if len(actionItems) == 0 && len(overnight) == 0 && len(hotTopics) == 0 {
		return digestCtx, g.storeQuietDigest(ctx, today)
	}

	// Publish to NATS for LLM generation
	data, err := json.Marshal(digestCtx)
	if err != nil {
		return nil, fmt.Errorf("marshal digest context: %w", err)
	}

	if err := g.NATS.Publish(ctx, smacknats.SubjectDigestGenerate, data); err != nil {
		// Fallback: generate plain-text digest without LLM
		slog.Warn("NATS publish failed, generating fallback digest", "error", err)
		return digestCtx, g.storeFallbackDigest(ctx, today, digestCtx)
	}

	return digestCtx, nil
}

// HandleDigestResult processes the generated digest from the ML sidecar.
func (g *Generator) HandleDigestResult(ctx context.Context, digest map[string]interface{}) error {
	digestDate, _ := digest["digest_date"].(string)
	text, _ := digest["text"].(string)
	wordCount := 0
	if wc, ok := digest["word_count"].(float64); ok {
		wordCount = int(wc)
	}
	modelUsed, _ := digest["model_used"].(string)

	if digestDate == "" || text == "" {
		return fmt.Errorf("invalid digest result: missing date or text")
	}

	id := ulid.Make().String()
	_, err := g.Pool.Exec(ctx, `
		INSERT INTO digests (id, digest_date, digest_text, word_count, model_used, is_quiet)
		VALUES ($1, $2, $3, $4, $5, false)
		ON CONFLICT (digest_date) DO UPDATE SET digest_text = $3, word_count = $4, model_used = $5
	`, id, digestDate, text, wordCount, modelUsed)
	if err != nil {
		return fmt.Errorf("store digest: %w", err)
	}

	slog.Info("digest stored", "date", digestDate, "words", wordCount, "model", modelUsed)
	return nil
}

// GetLatest returns the latest digest, optionally for a specific date.
func (g *Generator) GetLatest(ctx context.Context, date string) (*Digest, error) {
	var d Digest
	var query string
	var args []interface{}

	if date != "" {
		query = "SELECT id, digest_date, digest_text, word_count, is_quiet, COALESCE(model_used, ''), created_at FROM digests WHERE digest_date = $1"
		args = []interface{}{date}
	} else {
		query = "SELECT id, digest_date, digest_text, word_count, is_quiet, COALESCE(model_used, ''), created_at FROM digests ORDER BY digest_date DESC LIMIT 1"
	}

	err := g.Pool.QueryRow(ctx, query, args...).Scan(
		&d.ID, &d.DigestDate, &d.DigestText, &d.WordCount, &d.IsQuiet, &d.ModelUsed, &d.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get digest: %w", err)
	}

	return &d, nil
}

func (g *Generator) getPendingActionItems(ctx context.Context) ([]ActionItem, error) {
	rows, err := g.Pool.Query(ctx, `
		SELECT ai.text, COALESCE(p.name, 'unknown'), EXTRACT(DAY FROM NOW() - ai.created_at)::int
		FROM action_items ai
		LEFT JOIN people p ON p.id = ai.person_id
		WHERE ai.status = 'open'
		ORDER BY ai.created_at
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ActionItem
	for rows.Next() {
		var item ActionItem
		if err := rows.Scan(&item.Text, &item.Person, &item.DaysWaiting); err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (g *Generator) getOvernightArtifacts(ctx context.Context) ([]ArtifactBrief, error) {
	rows, err := g.Pool.Query(ctx, `
		SELECT title, artifact_type FROM artifacts
		WHERE created_at > NOW() - INTERVAL '24 hours'
		ORDER BY created_at DESC
		LIMIT 20
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []ArtifactBrief
	for rows.Next() {
		var a ArtifactBrief
		if err := rows.Scan(&a.Title, &a.Type); err != nil {
			continue
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, nil
}

func (g *Generator) getHotTopics(ctx context.Context) ([]TopicBrief, error) {
	rows, err := g.Pool.Query(ctx, `
		SELECT name, capture_count_30d FROM topics
		WHERE state IN ('hot', 'active')
		ORDER BY momentum_score DESC
		LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []TopicBrief
	for rows.Next() {
		var t TopicBrief
		if err := rows.Scan(&t.Name, &t.CapturesThisWeek); err != nil {
			continue
		}
		topics = append(topics, t)
	}
	return topics, nil
}

func (g *Generator) storeQuietDigest(ctx context.Context, date string) error {
	id := ulid.Make().String()
	_, err := g.Pool.Exec(ctx, `
		INSERT INTO digests (id, digest_date, digest_text, word_count, is_quiet)
		VALUES ($1, $2, 'All quiet. Nothing needs your attention today.', 9, true)
		ON CONFLICT (digest_date) DO NOTHING
	`, id, date)
	return err
}

func (g *Generator) storeFallbackDigest(ctx context.Context, date string, digestCtx *DigestContext) error {
	var lines []string
	if len(digestCtx.ActionItems) > 0 {
		lines = append(lines, fmt.Sprintf("! %d action items need attention.", len(digestCtx.ActionItems)))
	}
	if len(digestCtx.OvernightArtifacts) > 0 {
		lines = append(lines, fmt.Sprintf("> %d items processed overnight.", len(digestCtx.OvernightArtifacts)))
	}
	if len(digestCtx.HotTopics) > 0 {
		topicNames := make([]string, 0, len(digestCtx.HotTopics))
		for _, t := range digestCtx.HotTopics {
			topicNames = append(topicNames, t.Name)
		}
		lines = append(lines, fmt.Sprintf("> Hot topics: %s", joinStrings(topicNames, ", ")))
	}

	text := joinStrings(lines, "\n")
	wordCount := len(splitWords(text))

	id := ulid.Make().String()
	_, err := g.Pool.Exec(ctx, `
		INSERT INTO digests (id, digest_date, digest_text, word_count, model_used, is_quiet)
		VALUES ($1, $2, $3, $4, 'fallback', false)
		ON CONFLICT (digest_date) DO UPDATE SET digest_text = $3, word_count = $4, model_used = 'fallback'
	`, id, date, text, wordCount)
	return err
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func splitWords(s string) []string {
	var words []string
	word := ""
	for _, c := range s {
		if c == ' ' || c == '\n' || c == '\t' {
			if word != "" {
				words = append(words, word)
				word = ""
			}
		} else {
			word += string(c)
		}
	}
	if word != "" {
		words = append(words, word)
	}
	return words
}
