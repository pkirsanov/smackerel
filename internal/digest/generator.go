package digest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/metrics"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// Generator assembles and generates daily digests.
type Generator struct {
	Pool             *pgxpool.Pool
	NATS             *smacknats.Client
	Registry         *connector.Registry
	KnowledgeEnabled bool
	ExpenseSection   *ExpenseDigestSection
}

// NewGenerator creates a new digest generator.
func NewGenerator(pool *pgxpool.Pool, nats *smacknats.Client, registry *connector.Registry) *Generator {
	return &Generator{Pool: pool, NATS: nats, Registry: registry}
}

// DigestContext is the context payload sent to the ML sidecar for digest generation.
type DigestContext struct {
	DigestDate         string                        `json:"digest_date"`
	ActionItems        []ActionItem                  `json:"action_items"`
	OvernightArtifacts []ArtifactBrief               `json:"overnight_artifacts"`
	HotTopics          []TopicBrief                  `json:"hot_topics"`
	Hospitality        *HospitalityDigestContext     `json:"hospitality,omitempty"`
	KnowledgeHealth    *KnowledgeHealthDigestContext `json:"knowledge_health,omitempty"`
	Expenses           *ExpenseDigestContext         `json:"expenses,omitempty"`
}

// KnowledgeHealthDigestContext holds critical knowledge lint findings for the digest.
type KnowledgeHealthDigestContext struct {
	CriticalFindings []KnowledgeDigestFinding `json:"critical_findings"`
	SynthesisBacklog int                      `json:"synthesis_backlog"`
}

// KnowledgeDigestFinding is a summary of a critical lint finding for the digest.
type KnowledgeDigestFinding struct {
	Type        string `json:"type"`
	Description string `json:"description"`
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
	DigestDate  time.Time `json:"date"`
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

	// Assemble hospitality context if GuestHost connector is active
	if g.isGuestHostActive() {
		hCtx, hErr := AssembleHospitalityContext(ctx, g.Pool)
		if hErr != nil {
			slog.Warn("failed to assemble hospitality digest context", "error", hErr)
		} else if !hCtx.IsEmpty() {
			digestCtx.Hospitality = hCtx
		}
	}

	// Assemble knowledge health context if knowledge layer is enabled
	if g.KnowledgeEnabled {
		khCtx := g.assembleKnowledgeHealthContext(ctx)
		if khCtx != nil {
			digestCtx.KnowledgeHealth = khCtx
		}
	}

	// Assemble expense digest context if expense section producer is configured
	if g.ExpenseSection != nil {
		expCtx, expErr := g.ExpenseSection.Assemble(ctx)
		if expErr != nil {
			slog.Warn("failed to assemble expense digest context", "error", expErr)
		} else if !expCtx.IsEmpty() {
			digestCtx.Expenses = expCtx
		}
	}

	// Check for quiet day
	hasHospitality := digestCtx.Hospitality != nil
	hasKnowledgeHealth := digestCtx.KnowledgeHealth != nil
	hasExpenses := digestCtx.Expenses != nil
	if len(actionItems) == 0 && len(overnight) == 0 && len(hotTopics) == 0 && !hasHospitality && !hasKnowledgeHealth && !hasExpenses {
		metrics.DigestGeneration.WithLabelValues("quiet").Inc()
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
		metrics.DigestGeneration.WithLabelValues("fallback").Inc()
		return digestCtx, g.storeFallbackDigest(ctx, today, digestCtx)
	}

	metrics.DigestGeneration.WithLabelValues("published").Inc()
	return digestCtx, nil
}

// HandleDigestResult processes the generated digest from the ML sidecar.
// Accepts typed fields validated by the caller (pipeline subscriber).
func (g *Generator) HandleDigestResult(ctx context.Context, digestDate, text string, wordCount int, modelUsed string) error {
	if digestDate == "" || text == "" {
		return fmt.Errorf("invalid digest result: missing date or text")
	}

	if g.Pool == nil {
		return fmt.Errorf("database pool is nil")
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
	var args []any

	if date != "" {
		query = "SELECT id, digest_date, digest_text, word_count, is_quiet, COALESCE(model_used, ''), created_at FROM digests WHERE digest_date = $1"
		args = []any{date}
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
			slog.Warn("action item scan failed", "error", err)
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return items, err
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
			slog.Warn("overnight artifact scan failed", "error", err)
			continue
		}
		artifacts = append(artifacts, a)
	}
	if err := rows.Err(); err != nil {
		return artifacts, err
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
			slog.Warn("hot topic scan failed", "error", err)
			continue
		}
		topics = append(topics, t)
	}
	if err := rows.Err(); err != nil {
		return topics, err
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
		lines = append(lines, fmt.Sprintf("> Hot topics: %s", strings.Join(topicNames, ", ")))
	}
	if digestCtx.Hospitality != nil {
		lines = append(lines, formatHospitalityFallback(digestCtx.Hospitality))
	}
	if digestCtx.KnowledgeHealth != nil {
		lines = append(lines, formatKnowledgeHealthFallback(digestCtx.KnowledgeHealth))
	}

	text := strings.Join(lines, "\n")
	wordCount := len(strings.Fields(text))

	id := ulid.Make().String()
	_, err := g.Pool.Exec(ctx, `
		INSERT INTO digests (id, digest_date, digest_text, word_count, model_used, is_quiet)
		VALUES ($1, $2, $3, $4, 'fallback', false)
		ON CONFLICT (digest_date) DO UPDATE SET digest_text = $3, word_count = $4, model_used = 'fallback'
	`, id, date, text, wordCount)
	return err
}

// isGuestHostActive checks whether the GuestHost connector is registered.
func (g *Generator) isGuestHostActive() bool {
	if g.Registry == nil {
		return false
	}
	_, ok := g.Registry.Get("guesthost")
	return ok
}

// formatHospitalityFallback produces a plain-text hospitality section for the
// fallback digest (used when the ML sidecar is unreachable).
func formatHospitalityFallback(h *HospitalityDigestContext) string {
	var parts []string
	parts = append(parts, "--- Hospitality ---")
	if len(h.TodayArrivals) > 0 {
		parts = append(parts, fmt.Sprintf("Arrivals today: %d", len(h.TodayArrivals)))
		for _, a := range h.TodayArrivals {
			parts = append(parts, fmt.Sprintf("  • %s at %s", a.GuestName, a.PropertyName))
		}
	}
	if len(h.TodayDepartures) > 0 {
		parts = append(parts, fmt.Sprintf("Departures today: %d", len(h.TodayDepartures)))
		for _, d := range h.TodayDepartures {
			parts = append(parts, fmt.Sprintf("  • %s from %s", d.GuestName, d.PropertyName))
		}
	}
	if len(h.PendingTasks) > 0 {
		parts = append(parts, fmt.Sprintf("Pending tasks: %d", len(h.PendingTasks)))
	}
	if h.Revenue.DayRevenue > 0 || h.Revenue.WeekRevenue > 0 || h.Revenue.MonthRevenue > 0 {
		parts = append(parts, fmt.Sprintf("Revenue — 24h: $%.2f, week: $%.2f, month: $%.2f", h.Revenue.DayRevenue, h.Revenue.WeekRevenue, h.Revenue.MonthRevenue))
		if len(h.Revenue.ByChannel) > 0 {
			channels := make([]string, 0, len(h.Revenue.ByChannel))
			for ch := range h.Revenue.ByChannel {
				channels = append(channels, ch)
			}
			sort.Strings(channels)
			for _, ch := range channels {
				parts = append(parts, fmt.Sprintf("  • %s: $%.2f", ch, h.Revenue.ByChannel[ch]))
			}
		}
	}
	if len(h.GuestAlerts) > 0 {
		parts = append(parts, fmt.Sprintf("Guest alerts: %d", len(h.GuestAlerts)))
	}
	if len(h.PropertyAlerts) > 0 {
		parts = append(parts, fmt.Sprintf("Property alerts: %d", len(h.PropertyAlerts)))
	}
	return strings.Join(parts, "\n")
}

// assembleKnowledgeHealthContext queries the latest lint report and synthesis backlog.
// Returns nil if no critical findings exist (high-severity lint findings or backlog > 10).
func (g *Generator) assembleKnowledgeHealthContext(ctx context.Context) *KnowledgeHealthDigestContext {
	if g.Pool == nil {
		return nil
	}

	// Query latest lint report summary and findings
	row := g.Pool.QueryRow(ctx, `
		SELECT findings, summary FROM knowledge_lint_reports
		ORDER BY run_at DESC LIMIT 1`)

	var findingsJSON, summaryJSON json.RawMessage
	if err := row.Scan(&findingsJSON, &summaryJSON); err != nil {
		slog.Warn("failed to get latest lint report for digest", "error", err)
		return nil
	}

	// Parse summary to check for high-severity findings
	var summary struct {
		High int `json:"high"`
	}
	if err := json.Unmarshal(summaryJSON, &summary); err != nil {
		slog.Warn("failed to parse lint summary for digest", "error", err)
		return nil
	}

	// Check synthesis backlog
	var backlog int
	if err := g.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM artifacts WHERE synthesis_status = 'pending'").Scan(&backlog); err != nil {
		slog.Warn("failed to count synthesis backlog for digest", "error", err)
	}

	// Only include when critical: high-severity findings or backlog > 10
	if summary.High == 0 && backlog <= 10 {
		return nil
	}

	// Parse individual high-severity findings for the digest
	var findings []struct {
		Type        string `json:"type"`
		Severity    string `json:"severity"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(findingsJSON, &findings); err != nil {
		slog.Warn("failed to parse lint findings for digest", "error", err)
	}

	var critical []KnowledgeDigestFinding
	for _, f := range findings {
		if f.Severity == "high" {
			critical = append(critical, KnowledgeDigestFinding{
				Type:        f.Type,
				Description: f.Description,
			})
		}
	}

	return &KnowledgeHealthDigestContext{
		CriticalFindings: critical,
		SynthesisBacklog: backlog,
	}
}

// formatKnowledgeHealthFallback produces a plain-text knowledge health section for the
// fallback digest (used when the ML sidecar is unreachable).
func formatKnowledgeHealthFallback(kh *KnowledgeHealthDigestContext) string {
	var parts []string
	parts = append(parts, "--- Knowledge Health ---")
	if len(kh.CriticalFindings) > 0 {
		parts = append(parts, fmt.Sprintf("Critical findings: %d", len(kh.CriticalFindings)))
		for _, f := range kh.CriticalFindings {
			parts = append(parts, fmt.Sprintf("  • [%s] %s", f.Type, f.Description))
		}
	}
	if kh.SynthesisBacklog > 10 {
		parts = append(parts, fmt.Sprintf("Synthesis backlog: %d items pending", kh.SynthesisBacklog))
	}
	return strings.Join(parts, "\n")
}
