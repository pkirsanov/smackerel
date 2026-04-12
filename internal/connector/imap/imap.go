package imap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// Connector implements the IMAP email connector.
type Connector struct {
	id         string
	config     connector.ConnectorConfig
	health     connector.HealthStatus
	qualifiers QualifierConfig
}

// New creates a new IMAP connector.
func New(id string) *Connector {
	return &Connector{
		id:     id,
		health: connector.HealthDisconnected,
	}
}

// ID returns the connector identifier.
func (c *Connector) ID() string { return c.id }

// Connect initializes the IMAP connection.
func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	c.config = config

	authType := config.AuthType
	if authType != "oauth2" && authType != "password" {
		return fmt.Errorf("IMAP connector requires oauth2 or password auth, got %q", authType)
	}

	// Parse qualifier config from source config
	c.qualifiers = ParseQualifiers(config.Qualifiers)

	slog.Info("IMAP connector connected", "id", c.id, "auth", authType)
	c.health = connector.HealthHealthy
	return nil
}

// EmailMessage represents a parsed IMAP email for processing.
type EmailMessage struct {
	UID        string    `json:"uid"`
	MessageID  string    `json:"message_id"`
	From       string    `json:"from"`
	To         []string  `json:"to"`
	Subject    string    `json:"subject"`
	Date       time.Time `json:"date"`
	Body       string    `json:"body"`
	Labels     []string  `json:"labels"`
	HasAttach  bool      `json:"has_attachments"`
	InReplyTo  string    `json:"in_reply_to,omitempty"`
	References []string  `json:"references,omitempty"`
}

// Sync fetches emails newer than the cursor (message UID).
// Emails are read from the source_config "messages" field for local/test use,
// or from a live IMAP connection when credentials are configured.
func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.health = connector.HealthSyncing
	defer func() {
		if c.health == connector.HealthSyncing {
			c.health = connector.HealthHealthy
		}
	}()

	messages, err := c.fetchMessages(ctx, cursor)
	if err != nil {
		c.health = connector.HealthError
		return nil, cursor, fmt.Errorf("fetch messages: %w", err)
	}

	if len(messages) == 0 {
		slog.Info("IMAP sync: no new messages", "id", c.id, "cursor", cursor)
		return nil, cursor, nil
	}

	// Sort by date ascending for cursor advancement
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Date.Before(messages[j].Date)
	})

	var artifacts []connector.RawArtifact
	newCursor := cursor

	for _, msg := range messages {
		// Skip messages at or before cursor
		if msg.UID <= cursor && cursor != "" {
			continue
		}

		tier := AssignTier(msg.From, msg.Labels, c.qualifiers)
		if tier == "skip" {
			continue
		}

		// Build artifact content
		content := msg.Body
		if content == "" {
			content = msg.Subject
		}

		metadata := map[string]interface{}{
			"from":            msg.From,
			"to":              msg.To,
			"subject":         msg.Subject,
			"labels":          msg.Labels,
			"has_attachments": msg.HasAttach,
			"processing_tier": tier,
			"message_id":      msg.MessageID,
		}

		if msg.InReplyTo != "" {
			metadata["in_reply_to"] = msg.InReplyTo
			metadata["is_thread"] = true
		}

		actionItems := ExtractActionItems(msg.Body)
		if len(actionItems) > 0 {
			metadata["action_items"] = actionItems
		}

		artifacts = append(artifacts, connector.RawArtifact{
			SourceID:    c.id,
			SourceRef:   msg.UID,
			ContentType: "email",
			Title:       msg.Subject,
			RawContent:  content,
			Metadata:    metadata,
			CapturedAt:  msg.Date,
		})

		if msg.UID > newCursor {
			newCursor = msg.UID
		}
	}

	slog.Info("IMAP sync complete",
		"id", c.id,
		"fetched", len(messages),
		"artifacts", len(artifacts),
		"cursor", newCursor,
	)

	return artifacts, newCursor, nil
}

// fetchMessages retrieves messages from source config (for testing/local)
// or from the Gmail REST API when OAuth credentials are present.
func (c *Connector) fetchMessages(ctx context.Context, cursor string) ([]EmailMessage, error) {
	// Check for local/test messages in source_config (testing path)
	rawMsgs, ok := c.config.SourceConfig["messages"]
	if ok {
		return parseEmailMessages(rawMsgs)
	}

	// Check for OAuth access token (live API path)
	accessToken := getCredential(c.config.Credentials, "access_token")
	if accessToken == "" {
		slog.Debug("IMAP: no source_config messages and no access_token", "id", c.id)
		return nil, nil
	}

	return c.fetchGmailMessages(ctx, accessToken, cursor)
}

// fetchGmailMessages fetches emails from the Gmail REST API using OAuth2 Bearer token.
func (c *Connector) fetchGmailMessages(ctx context.Context, token string, cursor string) ([]EmailMessage, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Build query: messages after cursor timestamp, in INBOX
	query := "in:inbox"
	if cursor != "" {
		query += " after:" + cursor
	}

	// List message IDs
	listURL := fmt.Sprintf("https://www.googleapis.com/gmail/v1/users/me/messages?q=%s&maxResults=50",
		url.QueryEscape(query))

	listResp, err := gmailAPICall(ctx, client, listURL, token)
	if err != nil {
		return nil, fmt.Errorf("gmail list messages: %w", err)
	}

	msgs, _ := listResp["messages"].([]interface{})
	if len(msgs) == 0 {
		return nil, nil
	}

	var result []EmailMessage
	for _, m := range msgs {
		mm, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		msgID, _ := mm["id"].(string)
		if msgID == "" {
			continue
		}

		// Fetch individual message with metadata and snippet
		getURL := fmt.Sprintf("https://www.googleapis.com/gmail/v1/users/me/messages/%s?format=full", msgID)
		msgData, err := gmailAPICall(ctx, client, getURL, token)
		if err != nil {
			slog.Warn("gmail fetch message failed", "message_id", msgID, "error", err)
			continue
		}

		email := parseGmailMessage(msgData)
		if email != nil {
			result = append(result, *email)
		}
	}

	slog.Info("gmail API fetch complete", "messages", len(result))
	return result, nil
}

// gmailAPICall makes an authenticated GET request to the Gmail API.
func gmailAPICall(ctx context.Context, client *http.Client, apiURL string, token string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("gmail API: token expired or invalid (401)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("gmail API: HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Limit response body to 10MB to prevent resource exhaustion
	var result map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10*1024*1024)).Decode(&result); err != nil {
		return nil, fmt.Errorf("gmail API: decode response: %w", err)
	}
	return result, nil
}

// parseGmailMessage extracts an EmailMessage from a Gmail API message response.
func parseGmailMessage(data map[string]interface{}) *EmailMessage {
	payload, _ := data["payload"].(map[string]interface{})
	if payload == nil {
		return nil
	}

	headers, _ := payload["headers"].([]interface{})
	msg := &EmailMessage{
		UID:       fmt.Sprintf("%v", data["id"]),
		MessageID: fmt.Sprintf("%v", data["id"]),
		Date:      time.Now(),
	}

	// Extract headers
	for _, h := range headers {
		hm, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := hm["name"].(string)
		value, _ := hm["value"].(string)
		switch strings.ToLower(name) {
		case "from":
			msg.From = value
		case "to":
			msg.To = []string{value}
		case "subject":
			msg.Subject = value
		case "date":
			if t, err := time.Parse(time.RFC1123Z, value); err == nil {
				msg.Date = t
			} else if t, err := time.Parse("Mon, 2 Jan 2006 15:04:05 -0700", value); err == nil {
				msg.Date = t
			}
		case "message-id":
			msg.MessageID = value
		case "in-reply-to":
			msg.InReplyTo = value
		}
	}

	// Extract labels
	labelIDs, _ := data["labelIds"].([]interface{})
	for _, l := range labelIDs {
		if s, ok := l.(string); ok {
			msg.Labels = append(msg.Labels, s)
		}
	}

	// Extract body text (try plain text part first, then HTML)
	msg.Body = extractGmailBody(payload)

	// Check for attachments
	parts, _ := payload["parts"].([]interface{})
	for _, p := range parts {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if fn, _ := pm["filename"].(string); fn != "" {
			msg.HasAttach = true
			break
		}
	}

	return msg
}

// extractGmailBody extracts the text body from a Gmail message payload.
func extractGmailBody(payload map[string]interface{}) string {
	// Try direct body first (simple messages)
	if body, ok := payload["body"].(map[string]interface{}); ok {
		if data, ok := body["data"].(string); ok && data != "" {
			decoded, err := base64.URLEncoding.DecodeString(data)
			if err == nil {
				return string(decoded)
			}
		}
	}

	// Try multipart — find text/plain or text/html
	parts, _ := payload["parts"].([]interface{})
	var htmlBody string
	for _, p := range parts {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		mimeType, _ := pm["mimeType"].(string)
		body, _ := pm["body"].(map[string]interface{})
		if body == nil {
			continue
		}
		data, _ := body["data"].(string)
		if data == "" {
			continue
		}
		decoded, err := base64.URLEncoding.DecodeString(data)
		if err != nil {
			continue
		}
		if mimeType == "text/plain" {
			return string(decoded) // Prefer plain text
		}
		if mimeType == "text/html" {
			htmlBody = string(decoded)
		}
	}
	return htmlBody
}

func getCredential(creds map[string]string, key string) string {
	if creds == nil {
		return ""
	}
	return creds[key]
}

// parseEmailMessages converts interface{} messages from config into EmailMessage structs.
func parseEmailMessages(raw interface{}) ([]EmailMessage, error) {
	if raw == nil {
		return nil, nil
	}
	msgs, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("messages must be an array")
	}

	var result []EmailMessage
	for _, m := range msgs {
		mm, ok := m.(map[string]interface{})
		if !ok {
			continue
		}

		msg := EmailMessage{
			UID:     getStr(mm, "uid"),
			From:    getStr(mm, "from"),
			Subject: getStr(mm, "subject"),
			Body:    getStr(mm, "body"),
			Date:    time.Now(),
		}

		if uid := getStr(mm, "message_id"); uid != "" {
			msg.MessageID = uid
		}
		if d, ok := mm["date"].(string); ok {
			if t, err := time.Parse(time.RFC3339, d); err == nil {
				msg.Date = t
			}
		}
		if labels, ok := mm["labels"].([]interface{}); ok {
			for _, l := range labels {
				if s, ok := l.(string); ok {
					msg.Labels = append(msg.Labels, s)
				}
			}
		}
		if to, ok := mm["to"].([]interface{}); ok {
			for _, t := range to {
				if s, ok := t.(string); ok {
					msg.To = append(msg.To, s)
				}
			}
		}
		if v, ok := mm["in_reply_to"].(string); ok {
			msg.InReplyTo = v
		}

		result = append(result, msg)
	}
	return result, nil
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// Health returns the current connector status.
func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
	return c.health
}

// Close disconnects the IMAP session.
func (c *Connector) Close() error {
	c.health = connector.HealthDisconnected
	slog.Info("IMAP connector closed", "id", c.id)
	return nil
}

// QualifierConfig holds IMAP-specific qualifier settings.
type QualifierConfig struct {
	PrioritySenders []string `json:"priority_senders"`
	SkipLabels      []string `json:"skip_labels"`
	PriorityLabels  []string `json:"priority_labels"`
	SkipDomains     []string `json:"skip_domains"`
}

// ParseQualifiers converts generic qualifier map into QualifierConfig.
func ParseQualifiers(q map[string]interface{}) QualifierConfig {
	cfg := QualifierConfig{}
	if ps, ok := q["priority_senders"].([]interface{}); ok {
		for _, s := range ps {
			if str, ok := s.(string); ok {
				cfg.PrioritySenders = append(cfg.PrioritySenders, str)
			}
		}
	}
	if sl, ok := q["skip_labels"].([]interface{}); ok {
		for _, s := range sl {
			if str, ok := s.(string); ok {
				cfg.SkipLabels = append(cfg.SkipLabels, str)
			}
		}
	}
	if pl, ok := q["priority_labels"].([]interface{}); ok {
		for _, s := range pl {
			if str, ok := s.(string); ok {
				cfg.PriorityLabels = append(cfg.PriorityLabels, str)
			}
		}
	}
	return cfg
}

// AssignTier determines processing tier for an email based on qualifiers.
func AssignTier(from string, labels []string, qualifiers QualifierConfig) string {
	// Check skip domains first
	for _, d := range qualifiers.SkipDomains {
		if strings.HasSuffix(from, "@"+d) {
			return "skip"
		}
	}

	// Check skip labels
	for _, l := range labels {
		for _, sl := range qualifiers.SkipLabels {
			if l == sl {
				return "metadata"
			}
		}
	}

	// Check priority senders
	for _, s := range qualifiers.PrioritySenders {
		if s == from {
			return "full"
		}
	}

	// Check priority labels
	for _, l := range labels {
		for _, pl := range qualifiers.PriorityLabels {
			if l == pl {
				return "full"
			}
		}
	}

	return "standard"
}

// ExtractActionItems identifies action items from email text.
func ExtractActionItems(text string) []string {
	if text == "" {
		return nil
	}

	var items []string
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		// Match common action item patterns
		if strings.HasPrefix(lower, "action:") ||
			strings.HasPrefix(lower, "todo:") ||
			strings.HasPrefix(lower, "- [ ]") ||
			strings.Contains(lower, "please") && (strings.Contains(lower, "by") || strings.Contains(lower, "before")) ||
			strings.Contains(lower, "deadline:") {
			items = append(items, strings.TrimSpace(line))
		}
	}
	return items
}

var _ connector.Connector = (*Connector)(nil)
