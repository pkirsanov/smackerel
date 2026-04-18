package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/annotation"
)

// pendingDisambiguation stores state for /rate disambiguation flow.
type pendingDisambiguation struct {
	Artifacts  []disambiguationOption
	Annotation string
	ExpiresAt  time.Time
}

// disambiguationOption is a single artifact in a disambiguation list.
type disambiguationOption struct {
	ArtifactID string
	Title      string
}

// disambiguationStore provides thread-safe access to pending disambiguation state.
type disambiguationStore struct {
	mu      sync.Mutex
	pending map[int64]*pendingDisambiguation // keyed by chat_id
	timeout time.Duration
}

func newDisambiguationStore(timeoutSeconds int) *disambiguationStore {
	return &disambiguationStore{
		pending: make(map[int64]*pendingDisambiguation),
		timeout: time.Duration(timeoutSeconds) * time.Second,
	}
}

func (ds *disambiguationStore) set(chatID int64, p *pendingDisambiguation) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	p.ExpiresAt = time.Now().Add(ds.timeout)
	ds.pending[chatID] = p
}

func (ds *disambiguationStore) get(chatID int64) *pendingDisambiguation {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	p, ok := ds.pending[chatID]
	if !ok {
		return nil
	}
	if time.Now().After(p.ExpiresAt) {
		delete(ds.pending, chatID)
		return nil
	}
	return p
}

func (ds *disambiguationStore) clear(chatID int64) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	delete(ds.pending, chatID)
}

// handleReplyAnnotation processes a reply-to message as an annotation.
// Returns true if the reply was handled as an annotation (message maps to a known artifact),
// false if it should fall through to normal handling.
func (b *Bot) handleReplyAnnotation(ctx context.Context, msg *tgbotapi.Message) bool {
	if msg.ReplyToMessage == nil {
		return false
	}

	replyToID := msg.ReplyToMessage.MessageID
	chatID := msg.Chat.ID

	artifactID := b.resolveArtifactFromMessage(ctx, replyToID, chatID)
	if artifactID == "" {
		return false // Not a known capture message — fall through
	}

	text := msg.Text
	if text == "" {
		return false
	}

	// Parse the annotation text
	parsed := annotation.Parse(text)

	// Submit annotation via internal API
	created, err := b.submitAnnotation(ctx, artifactID, text)
	if err != nil {
		slog.Error("failed to submit reply annotation", "error", err, "artifact_id", artifactID)
		b.reply(chatID, "? Failed to record annotation")
		return true
	}

	// Format and send confirmation
	confirmation := formatAnnotationConfirmation(created, parsed)
	b.reply(chatID, confirmation)
	return true
}

// handleRate processes the /rate command for annotating artifacts by search.
func (b *Bot) handleRate(ctx context.Context, msg *tgbotapi.Message, args string) {
	chatID := msg.Chat.ID

	if args == "" {
		b.reply(chatID, `Usage: /rate <search terms> <annotation>
Examples:
  /rate pasta carbonara 4/5 great dish
  /rate chicken recipe #weeknight
  /rate that cake I saved 5/5 made it`)
		return
	}

	// Split search terms from annotation by finding annotation markers
	searchTerms, annotationText := splitRateArgs(args)

	if searchTerms == "" {
		b.reply(chatID, "? Please include search terms. Usage: /rate <search terms> <annotation>")
		return
	}

	// Search for matching artifacts
	results, err := b.callSearch(ctx, searchTerms)
	if err != nil {
		b.reply(chatID, "? Search failed. Try again in a moment.")
		return
	}

	resultList, ok := results["results"].([]interface{})
	if !ok || len(resultList) == 0 {
		b.reply(chatID, "No matching artifacts found")
		return
	}

	if len(resultList) == 1 || isStrongMatch(resultList) {
		// Single or strong match — annotate directly
		first, _ := resultList[0].(map[string]interface{})
		artifactID, _ := first["artifact_id"].(string)
		title, _ := first["title"].(string)

		if annotationText == "" {
			b.reply(chatID, fmt.Sprintf("Found \"%s\" but no annotation text provided. Reply with your annotation.", title))
			return
		}

		parsed := annotation.Parse(annotationText)
		created, err := b.submitAnnotation(ctx, artifactID, annotationText)
		if err != nil {
			b.reply(chatID, "? Failed to record annotation")
			return
		}

		confirmation := fmt.Sprintf("📝 \"%s\"\n%s", title, formatAnnotationConfirmation(created, parsed))
		b.reply(chatID, confirmation)
		return
	}

	// Multiple matches — trigger disambiguation
	maxOptions := 3
	if len(resultList) < maxOptions {
		maxOptions = len(resultList)
	}

	var options []disambiguationOption
	var lines []string
	lines = append(lines, "Multiple matches found. Reply with a number:")
	for i := 0; i < maxOptions; i++ {
		r, _ := resultList[i].(map[string]interface{})
		artID, _ := r["artifact_id"].(string)
		title, _ := r["title"].(string)
		artType, _ := r["artifact_type"].(string)
		options = append(options, disambiguationOption{ArtifactID: artID, Title: title})
		lines = append(lines, fmt.Sprintf("%d. %s (%s)", i+1, title, artType))
	}

	if b.disambiguations != nil {
		b.disambiguations.set(chatID, &pendingDisambiguation{
			Artifacts:  options,
			Annotation: annotationText,
		})
	}

	b.reply(chatID, strings.Join(lines, "\n"))
}

// handleDisambiguationReply checks if a message is a numeric reply to a disambiguation prompt.
// Returns true if handled.
func (b *Bot) handleDisambiguationReply(ctx context.Context, msg *tgbotapi.Message) bool {
	if b.disambiguations == nil {
		return false
	}

	chatID := msg.Chat.ID
	pending := b.disambiguations.get(chatID)
	if pending == nil {
		return false
	}

	text := strings.TrimSpace(msg.Text)
	choice, err := strconv.Atoi(text)
	if err != nil || choice < 1 || choice > len(pending.Artifacts) {
		return false // Not a valid number — fall through
	}

	selected := pending.Artifacts[choice-1]
	b.disambiguations.clear(chatID)

	if pending.Annotation == "" {
		b.reply(chatID, fmt.Sprintf("Selected \"%s\" but no annotation was pending.", selected.Title))
		return true
	}

	parsed := annotation.Parse(pending.Annotation)
	created, err := b.submitAnnotation(ctx, selected.ArtifactID, pending.Annotation)
	if err != nil {
		b.reply(chatID, "? Failed to record annotation")
		return true
	}

	confirmation := fmt.Sprintf("📝 \"%s\"\n%s", selected.Title, formatAnnotationConfirmation(created, parsed))
	b.reply(chatID, confirmation)
	return true
}

// submitAnnotation submits an annotation via the internal API and returns the created events.
func (b *Bot) submitAnnotation(ctx context.Context, artifactID, text string) ([]map[string]interface{}, error) {
	url := b.baseURL +
		fmt.Sprintf("/api/artifacts/%s/annotations", artifactID)

	body := map[string]string{"text": text}
	result, err := b.callInternalAPI(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}

	created, _ := result["created"].([]interface{})
	var events []map[string]interface{}
	for _, c := range created {
		if m, ok := c.(map[string]interface{}); ok {
			events = append(events, m)
		}
	}
	return events, nil
}

// callInternalAPI is a generic helper for calling internal JSON APIs.
func (b *Bot) callInternalAPI(ctx context.Context, method, url string, body interface{}) (map[string]interface{}, error) {
	var bodyReader *strings.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = strings.NewReader(string(data))
	} else {
		bodyReader = strings.NewReader("")
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("internal API call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("internal API error %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// splitRateArgs splits /rate arguments into search terms and annotation text.
// It looks for annotation markers (rating pattern, hashtags, interaction keywords)
// and splits at the first one found.
func splitRateArgs(args string) (searchTerms, annotationText string) {
	// Look for rating pattern N/5 as a split point
	words := strings.Fields(args)
	for i, w := range words {
		// Check for rating pattern
		if len(w) == 3 && w[1] == '/' && w[2] == '5' && w[0] >= '1' && w[0] <= '5' {
			searchTerms = strings.TrimSpace(strings.Join(words[:i], " "))
			annotationText = strings.TrimSpace(strings.Join(words[i:], " "))
			return
		}
		// Check for hashtag
		if strings.HasPrefix(w, "#") {
			searchTerms = strings.TrimSpace(strings.Join(words[:i], " "))
			annotationText = strings.TrimSpace(strings.Join(words[i:], " "))
			return
		}
	}

	// Check for interaction keywords
	lower := strings.ToLower(args)
	interactionPhrases := []string{"made it", "bought it", "read it", "visited", "tried it", "used it"}
	for _, phrase := range interactionPhrases {
		idx := strings.Index(lower, phrase)
		if idx > 0 {
			searchTerms = strings.TrimSpace(args[:idx])
			annotationText = strings.TrimSpace(args[idx:])
			return
		}
	}

	// No annotation marker found — treat everything as search terms
	return args, ""
}

// isStrongMatch checks if the first search result is significantly more relevant than others.
func isStrongMatch(results []interface{}) bool {
	if len(results) < 2 {
		return true
	}
	first, ok := results[0].(map[string]interface{})
	if !ok {
		return false
	}
	relevance, _ := first["relevance"].(string)
	return relevance == "high"
}

// formatAnnotationConfirmation renders a human-friendly confirmation for recorded annotations.
func formatAnnotationConfirmation(created []map[string]interface{}, parsed annotation.ParsedAnnotation) string {
	var lines []string

	if parsed.Rating != nil {
		lines = append(lines, "Rated "+renderStars(*parsed.Rating))
	}

	if parsed.InteractionType != "" {
		lines = append(lines, "Logged: "+humanizeInteraction(parsed.InteractionType))
	}

	for _, tag := range parsed.Tags {
		lines = append(lines, "Tagged: #"+tag)
	}

	for _, tag := range parsed.RemovedTags {
		lines = append(lines, "Untagged: #"+tag)
	}

	if parsed.Note != "" {
		note := parsed.Note
		if len(note) > 80 {
			note = note[:77] + "..."
		}
		lines = append(lines, "Note: "+note)
	}

	if len(lines) == 0 {
		return "Annotation recorded"
	}

	return strings.Join(lines, "\n")
}

// renderStars renders a star rating string like "★★★★☆".
func renderStars(rating int) string {
	if rating < 1 {
		rating = 1
	}
	if rating > 5 {
		rating = 5
	}
	return strings.Repeat("★", rating) + strings.Repeat("☆", 5-rating)
}

// humanizeInteraction converts an InteractionType to a human-readable string.
func humanizeInteraction(it annotation.InteractionType) string {
	switch it {
	case annotation.InteractionMadeIt:
		return "Made it"
	case annotation.InteractionBoughtIt:
		return "Bought it"
	case annotation.InteractionReadIt:
		return "Read it"
	case annotation.InteractionVisited:
		return "Visited"
	case annotation.InteractionTriedIt:
		return "Tried it"
	case annotation.InteractionUsedIt:
		return "Used it"
	default:
		return string(it)
	}
}
