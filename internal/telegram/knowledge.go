package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/stringutil"
)

// knowledgeGet performs an authenticated GET to the knowledge API and decodes the JSON response.
// Returns the HTTP status code and true if the response was successfully decoded.
// On connection or parse errors, sends an error reply and returns (0, false).
// On 503 Service Unavailable, sends an error reply and returns (503, false).
// On other non-200 status codes, returns (status, false) without replying.
func (b *Bot) knowledgeGet(ctx context.Context, chatID int64, path string, result interface{}) (int, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.knowledgeURL+path, nil)
	if err != nil {
		b.reply(chatID, "? Couldn't reach knowledge service")
		return 0, false
	}
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.reply(chatID, "? Knowledge service unreachable")
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		b.reply(chatID, "? Knowledge layer is not enabled")
		return resp.StatusCode, false
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, false
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponseBytes)).Decode(result); err != nil {
		b.reply(chatID, "? Failed to parse response")
		return resp.StatusCode, false
	}
	return resp.StatusCode, true
}

// handleConcept handles the /concept command.
// No args: list top 10 concepts by citation count.
// With args: search by name and show concept detail.
func (b *Bot) handleConcept(ctx context.Context, msg *tgbotapi.Message, args string) {
	if args == "" {
		b.handleConceptList(ctx, msg)
		return
	}
	b.handleConceptDetail(ctx, msg, args)
}

// handleConceptList fetches and displays the top 10 concepts by citation count.
func (b *Bot) handleConceptList(ctx context.Context, msg *tgbotapi.Message) {
	var result conceptListResponse
	if _, ok := b.knowledgeGet(ctx, msg.Chat.ID, "/concepts?sort=citations&limit=10", &result); !ok {
		return
	}

	if len(result.Concepts) == 0 {
		b.reply(msg.Chat.ID, "> No concept pages yet")
		return
	}

	b.reply(msg.Chat.ID, formatConceptList(result.Concepts))
}

// handleConceptDetail searches for a concept by name and shows its details.
func (b *Bot) handleConceptDetail(ctx context.Context, msg *tgbotapi.Message, name string) {
	if len(name) > maxFindQueryLen {
		name = stringutil.TruncateUTF8(name, maxFindQueryLen)
	}

	var listResult conceptListResponse
	if _, ok := b.knowledgeGet(ctx, msg.Chat.ID, "/concepts?q="+url.QueryEscape(name)+"&limit=1", &listResult); !ok {
		return
	}

	if len(listResult.Concepts) == 0 {
		b.reply(msg.Chat.ID, fmt.Sprintf("? No concept page found for '%s'", name))
		return
	}

	// Fetch full detail for the first match
	var concept conceptDetail
	status, ok := b.knowledgeGet(ctx, msg.Chat.ID, "/concepts/"+url.PathEscape(listResult.Concepts[0].ID), &concept)
	if !ok {
		if status == http.StatusNotFound {
			b.reply(msg.Chat.ID, fmt.Sprintf("? No concept page found for '%s'", name))
		}
		return
	}

	b.reply(msg.Chat.ID, formatConceptDetail(concept))
}

// handlePerson handles the /person command.
// No args: list top 10 entities by mention count.
// With args: search by name and show entity profile.
func (b *Bot) handlePerson(ctx context.Context, msg *tgbotapi.Message, args string) {
	if args == "" {
		b.handlePersonList(ctx, msg)
		return
	}
	b.handlePersonDetail(ctx, msg, args)
}

// handlePersonList fetches and displays the top 10 entities by mention count.
func (b *Bot) handlePersonList(ctx context.Context, msg *tgbotapi.Message) {
	var result entityListResponse
	if _, ok := b.knowledgeGet(ctx, msg.Chat.ID, "/entities?sort=mentions&limit=10", &result); !ok {
		return
	}

	if len(result.Entities) == 0 {
		b.reply(msg.Chat.ID, "> No entity profiles yet")
		return
	}

	b.reply(msg.Chat.ID, formatEntityList(result.Entities))
}

// handlePersonDetail searches for an entity by name and shows its profile.
func (b *Bot) handlePersonDetail(ctx context.Context, msg *tgbotapi.Message, name string) {
	if len(name) > maxFindQueryLen {
		name = stringutil.TruncateUTF8(name, maxFindQueryLen)
	}

	var listResult entityListResponse
	if _, ok := b.knowledgeGet(ctx, msg.Chat.ID, "/entities?q="+url.QueryEscape(name)+"&limit=1", &listResult); !ok {
		return
	}

	if len(listResult.Entities) == 0 {
		b.reply(msg.Chat.ID, fmt.Sprintf("? No entity profile found for '%s'", name))
		return
	}

	// Fetch full detail for the first match
	var entity entityDetail
	status, ok := b.knowledgeGet(ctx, msg.Chat.ID, "/entities/"+url.PathEscape(listResult.Entities[0].ID), &entity)
	if !ok {
		if status == http.StatusNotFound {
			b.reply(msg.Chat.ID, fmt.Sprintf("? No entity profile found for '%s'", name))
		}
		return
	}

	b.reply(msg.Chat.ID, formatEntityProfile(entity))
}

// handleLint handles the /lint command — shows the latest lint report.
func (b *Bot) handleLint(ctx context.Context, msg *tgbotapi.Message) {
	var report lintReportResponse
	status, ok := b.knowledgeGet(ctx, msg.Chat.ID, "/lint", &report)
	if !ok {
		if status == http.StatusNotFound {
			b.reply(msg.Chat.ID, "> No lint report yet")
		}
		return
	}

	b.reply(msg.Chat.ID, formatLintReport(report))
}

// Response types used by knowledge command handlers.
// These mirror the API response shapes for JSON decoding.

type conceptListResponse struct {
	Concepts []conceptListItem `json:"concepts"`
	Total    int               `json:"total"`
}

type conceptListItem struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	CitationCount int      `json:"citation_count"`
	SourceTypes   []string `json:"source_types"`
}

type conceptDetail struct {
	ID                  string          `json:"id"`
	Title               string          `json:"title"`
	Summary             string          `json:"summary"`
	Claims              json.RawMessage `json:"claims"`
	RelatedConceptIDs   []string        `json:"related_concept_ids"`
	SourceArtifactIDs   []string        `json:"source_artifact_ids"`
	SourceTypeDiversity []string        `json:"source_type_diversity"`
}

type entityListResponse struct {
	Entities []entityListItem `json:"entities"`
	Total    int              `json:"total"`
}

type entityListItem struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	EntityType       string   `json:"entity_type"`
	Summary          string   `json:"summary"`
	SourceTypes      []string `json:"source_types"`
	InteractionCount int      `json:"interaction_count"`
}

type entityDetail struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	EntityType       string          `json:"entity_type"`
	Summary          string          `json:"summary"`
	Mentions         json.RawMessage `json:"mentions"`
	SourceTypes      []string        `json:"source_types"`
	InteractionCount int             `json:"interaction_count"`
}

type lintReportResponse struct {
	ID         string          `json:"id"`
	RunAt      string          `json:"run_at"`
	DurationMs int             `json:"duration_ms"`
	Findings   json.RawMessage `json:"findings"`
	Summary    json.RawMessage `json:"summary"`
}

type lintFinding struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	TargetTitle string `json:"target_title"`
	Description string `json:"description"`
}

type lintSummary struct {
	Total  int `json:"total"`
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
}

// knowledgeMatchResponse mirrors the knowledge_match field in search responses.
type knowledgeMatchResponse struct {
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	CitationCount int      `json:"citation_count"`
	SourceTypes   []string `json:"source_types"`
}

// formatConceptList formats a list of concepts for Telegram display.
func formatConceptList(concepts []conceptListItem) string {
	var lines []string
	lines = append(lines, "# Concept Pages")
	for i, c := range concepts {
		sourceInfo := ""
		if len(c.SourceTypes) > 0 {
			sourceInfo = " [" + strings.Join(c.SourceTypes, ", ") + "]"
		}
		lines = append(lines, fmt.Sprintf("- %d. %s (%d citations)%s", i+1, c.Title, c.CitationCount, sourceInfo))
	}
	return strings.Join(lines, "\n")
}

// formatConceptDetail formats a concept detail view for Telegram display.
func formatConceptDetail(c conceptDetail) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("# %s", c.Title))
	lines = append(lines, fmt.Sprintf("> %s", c.Summary))

	// Parse and display claims
	var claims []struct {
		Text       string `json:"text"`
		SourceType string `json:"source_type"`
	}
	if err := json.Unmarshal(c.Claims, &claims); err == nil && len(claims) > 0 {
		lines = append(lines, "")
		lines = append(lines, "# Key Claims")
		for i, cl := range claims {
			if i >= 5 {
				lines = append(lines, fmt.Sprintf("~ ...and %d more claims", len(claims)-5))
				break
			}
			source := ""
			if cl.SourceType != "" {
				source = " (" + cl.SourceType + ")"
			}
			lines = append(lines, fmt.Sprintf("- %s%s", cl.Text, source))
		}
	}

	// Related concepts
	if len(c.RelatedConceptIDs) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("~ %d related concepts", len(c.RelatedConceptIDs)))
	}

	// Source diversity
	if len(c.SourceTypeDiversity) > 0 {
		lines = append(lines, fmt.Sprintf("~ Sources: %s", strings.Join(c.SourceTypeDiversity, ", ")))
	}

	lines = append(lines, fmt.Sprintf("~ %d citations", len(c.SourceArtifactIDs)))

	return strings.Join(lines, "\n")
}

// formatEntityList formats a list of entities for Telegram display.
func formatEntityList(entities []entityListItem) string {
	var lines []string
	lines = append(lines, "# Entity Profiles")
	for i, e := range entities {
		typeLabel := ""
		if e.EntityType != "" {
			typeLabel = " (" + e.EntityType + ")"
		}
		lines = append(lines, fmt.Sprintf("- %d. %s%s - %d mentions", i+1, e.Name, typeLabel, e.InteractionCount))
	}
	return strings.Join(lines, "\n")
}

// formatEntityProfile formats an entity detail view for Telegram display.
func formatEntityProfile(e entityDetail) string {
	var lines []string
	typeLabel := ""
	if e.EntityType != "" {
		typeLabel = " (" + e.EntityType + ")"
	}
	lines = append(lines, fmt.Sprintf("# %s%s", e.Name, typeLabel))

	if e.Summary != "" {
		lines = append(lines, fmt.Sprintf("> %s", e.Summary))
	}

	if len(e.SourceTypes) > 0 {
		lines = append(lines, fmt.Sprintf("~ Source types: %s", strings.Join(e.SourceTypes, ", ")))
	}

	lines = append(lines, fmt.Sprintf("~ %d mentions", e.InteractionCount))

	// Parse and show recent mentions
	var mentions []struct {
		ArtifactTitle string `json:"artifact_title"`
		MentionedAt   string `json:"mentioned_at"`
	}
	if err := json.Unmarshal(e.Mentions, &mentions); err == nil && len(mentions) > 0 {
		lines = append(lines, "")
		lines = append(lines, "# Recent Mentions")
		for i, m := range mentions {
			if i >= 5 {
				lines = append(lines, fmt.Sprintf("~ ...and %d more mentions", len(mentions)-5))
				break
			}
			lines = append(lines, fmt.Sprintf("- %s", m.ArtifactTitle))
		}
	}

	return strings.Join(lines, "\n")
}

// formatLintReport formats a lint report for Telegram display.
func formatLintReport(r lintReportResponse) string {
	var lines []string
	lines = append(lines, "# Knowledge Lint Report")

	// Parse summary
	var summary lintSummary
	if err := json.Unmarshal(r.Summary, &summary); err == nil {
		lines = append(lines, fmt.Sprintf("> Total findings: %d", summary.Total))
		lines = append(lines, fmt.Sprintf("- High: %d", summary.High))
		lines = append(lines, fmt.Sprintf("- Medium: %d", summary.Medium))
		lines = append(lines, fmt.Sprintf("- Low: %d", summary.Low))
	}

	// Parse findings
	var findings []lintFinding
	if err := json.Unmarshal(r.Findings, &findings); err == nil && len(findings) > 0 {
		lines = append(lines, "")
		for i, f := range findings {
			if i >= 10 {
				lines = append(lines, fmt.Sprintf("~ ...and %d more findings", len(findings)-10))
				break
			}
			severityMarker := "- "
			switch f.Severity {
			case "high":
				severityMarker = "! "
			case "medium":
				severityMarker = "? "
			}
			title := f.TargetTitle
			if title == "" {
				title = f.Type
			}
			lines = append(lines, fmt.Sprintf("%s[%s] %s: %s", severityMarker, f.Severity, title, f.Description))
		}
	}

	if r.RunAt != "" {
		lines = append(lines, fmt.Sprintf("~ Report generated: %s", r.RunAt))
	}

	return strings.Join(lines, "\n")
}

// formatKnowledgeMatch formats a knowledge match for inclusion in /find results.
func formatKnowledgeMatch(km knowledgeMatchResponse) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("> From Knowledge Layer: %s", km.Title))
	lines = append(lines, fmt.Sprintf("- %s", km.Summary))
	if km.CitationCount > 0 {
		lines = append(lines, fmt.Sprintf("~ %d citations from %s", km.CitationCount, strings.Join(km.SourceTypes, ", ")))
	}
	lines = append(lines, fmt.Sprintf("~ /concept %s for full page", km.Title))
	return strings.Join(lines, "\n")
}
