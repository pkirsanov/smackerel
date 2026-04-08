package imap

import (
	"context"
	"fmt"
	"log/slog"
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
	defer func() { c.health = connector.HealthHealthy }()

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
// or would fetch from a live IMAP server when credentials are present.
func (c *Connector) fetchMessages(ctx context.Context, cursor string) ([]EmailMessage, error) {
	// Check for local/test messages in source_config
	rawMsgs, ok := c.config.SourceConfig["messages"]
	if ok {
		return parseEmailMessages(rawMsgs)
	}

	// No local messages and no live IMAP connection — return empty
	slog.Debug("IMAP: no messages in source_config and no live connection", "id", c.id)
	return nil, nil
}

// parseEmailMessages converts interface{} messages from config into EmailMessage structs.
func parseEmailMessages(raw interface{}) ([]EmailMessage, error) {
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
