package ntfy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Event struct {
	ID          string
	Time        *time.Time
	RawTime     string
	EventType   string
	Topic       string
	Title       string
	Message     string
	Priority    string
	RawPriority string
	Tags        []string
	Click       string
	Icon        string
	Markdown    *bool
	Attachment  json.RawMessage
	Attach      string
	Actions     json.RawMessage
	Unknown     map[string]json.RawMessage
	Raw         []byte
}

var unsafeUnknownKeyMarkers = [...]string{"token", "secret", "password", "authorization", "cookie"}

func ParseEvent(raw []byte, maxPayloadBytes int) (Event, error) {
	if maxPayloadBytes <= 0 {
		return Event{}, fmt.Errorf("ntfy event parse: max payload bytes must be positive")
	}
	if len(raw) == 0 {
		return Event{}, fmt.Errorf("ntfy event parse: payload is required")
	}
	if len(raw) > maxPayloadBytes {
		return Event{}, fmt.Errorf("ntfy event parse: payload exceeds max size")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil {
		return Event{}, fmt.Errorf("ntfy event parse: malformed json: %w", err)
	}
	event := Event{Unknown: make(map[string]json.RawMessage), Raw: append([]byte(nil), raw...)}
	for key, value := range fields {
		switch key {
		case "id":
			event.ID = stringField(value)
		case "time":
			event.RawTime = string(value)
			if ts, ok := parseUnixSeconds(value); ok {
				event.Time = &ts
			}
		case "event":
			event.EventType = stringField(value)
		case "topic":
			event.Topic = stringField(value)
		case "title":
			event.Title = stringField(value)
		case "message":
			event.Message = stringField(value)
		case "priority":
			event.RawPriority = string(value)
			event.Priority = priorityText(value)
		case "tags":
			event.Tags = stringArray(value)
		case "click":
			event.Click = stringField(value)
		case "icon":
			event.Icon = stringField(value)
		case "markdown":
			var markdown bool
			if json.Unmarshal(value, &markdown) == nil {
				event.Markdown = &markdown
			}
		case "attachment":
			event.Attachment = canonicalRaw(value)
		case "attach":
			event.Attach = stringField(value)
		case "actions":
			event.Actions = canonicalRaw(value)
		}
		if !isKnownEventField(key) && safeUnknownKey(key) && len(value) <= 2048 {
			event.Unknown[key] = canonicalRaw(value)
		}
	}
	if event.EventType == "" && (event.Message != "" || event.Title != "") {
		event.EventType = "message"
	}
	return event, nil
}

func isKnownEventField(key string) bool {
	switch key {
	case "id", "time", "expires", "event", "topic", "title", "message", "priority", "tags", "click", "icon", "markdown", "attachment", "attach", "actions":
		return true
	default:
		return false
	}
}

func (e Event) ShouldIngest() bool {
	return e.EventType == "message" && (strings.TrimSpace(e.Message) != "" || strings.TrimSpace(e.Title) != "")
}

func (e Event) IsLifecycle() bool {
	switch e.EventType {
	case "open", "keepalive", "poll_request":
		return true
	default:
		return false
	}
}

func stringField(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}
	return ""
}

func stringArray(raw json.RawMessage) []string {
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseUnixSeconds(raw json.RawMessage) (time.Time, bool) {
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return time.Time{}, false
	}
	seconds, err := number.Int64()
	if err != nil || seconds <= 0 {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0).UTC(), true
}

func priorityText(raw json.RawMessage) string {
	text := strings.ToLower(stringField(raw))
	switch text {
	case "1", "min":
		return "min"
	case "2", "low":
		return "low"
	case "3", "default":
		return "default"
	case "4", "high":
		return "high"
	case "5", "urgent":
		return "urgent"
	default:
		return ""
	}
}

func severityHint(priority string) string {
	switch priority {
	case "min":
		return "info"
	case "low":
		return "low"
	case "default":
		return "medium"
	case "high":
		return "high"
	case "urgent":
		return "critical"
	default:
		return ""
	}
}

func canonicalRaw(raw json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return append([]byte(nil), raw...)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return append([]byte(nil), raw...)
	}
	return encoded
}

func safeUnknownKey(key string) bool {
	lowered := strings.ToLower(key)
	for _, marker := range unsafeUnknownKeyMarkers {
		if strings.Contains(lowered, marker) {
			return false
		}
	}
	return true
}

func sortedUnknownJSON(values map[string]json.RawMessage) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	buf := bytes.NewBufferString("{")
	for index, key := range keys {
		if index > 0 {
			buf.WriteByte(',')
		}
		keyJSON, _ := json.Marshal(key)
		buf.Write(keyJSON)
		buf.WriteByte(':')
		buf.Write(values[key])
	}
	buf.WriteByte('}')
	return buf.String()
}
