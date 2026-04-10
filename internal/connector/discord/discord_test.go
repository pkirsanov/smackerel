package discord

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestNew(t *testing.T) {
	c := New("discord")
	if c.ID() != "discord" {
		t.Errorf("expected discord, got %s", c.ID())
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("new connector should be disconnected")
	}
	if c.limiter == nil {
		t.Error("new connector should have a rate limiter")
	}
}

func TestConnect_MissingToken(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{},
	})
	if err == nil {
		t.Error("expected error for missing token")
	}
}

func TestConnect_ValidConfig(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": "test-token"},
		SourceConfig: map[string]interface{}{
			"backfill_limit":   float64(500),
			"enable_gateway":   false,
			"include_threads":  false,
			"include_pins":     false,
			"capture_commands": []interface{}{"!grab"},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Error("connected connector should be healthy")
	}
	if c.config.BackfillLimit != 500 {
		t.Errorf("expected backfill_limit 500, got %d", c.config.BackfillLimit)
	}
	if c.config.EnableGateway {
		t.Error("expected enable_gateway false")
	}
	if c.config.IncludeThreads {
		t.Error("expected include_threads false")
	}
	if c.config.IncludePins {
		t.Error("expected include_pins false")
	}
	if len(c.config.CaptureCommands) != 1 || c.config.CaptureCommands[0] != "!grab" {
		t.Errorf("expected capture_commands [!grab], got %v", c.config.CaptureCommands)
	}
}

func TestSync_EmptyChannels(t *testing.T) {
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": "test-token"},
	})
	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(artifacts))
	}
	if cursor == "" {
		t.Error("cursor should not be empty")
	}
}

func TestClose(t *testing.T) {
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": "test-token"},
	})
	err := c.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("closed connector should be disconnected")
	}
}

func TestClassifyMessage(t *testing.T) {
	cmds := []string{"!save", "!capture"}
	tests := []struct {
		name     string
		msg      DiscordMessage
		expected string
	}{
		{"plain text", DiscordMessage{Content: "hello"}, "discord/message"},
		{"with embed", DiscordMessage{Embeds: []Embed{{Title: "Test"}}}, "discord/embed"},
		{"with link", DiscordMessage{Content: "check https://example.com"}, "discord/link"},
		{"with code", DiscordMessage{Content: "```go\nfmt.Println()```"}, "discord/code"},
		{"with attachment", DiscordMessage{Attachments: []Attachment{{ID: "a1", Filename: "img.png"}}}, "discord/attachment"},
		{"reply", DiscordMessage{Content: "I agree", MessageReference: &MessageRef{MessageID: "999"}}, "discord/reply"},
		{"thread starter", DiscordMessage{Content: "Thread topic", ThreadID: "t1"}, "discord/thread"},
		{"bot save command", DiscordMessage{Content: "!save https://example.com Great"}, "discord/capture"},
		{"bot capture command", DiscordMessage{Content: "!capture https://test.com"}, "discord/capture"},
		{"short", DiscordMessage{Content: "ok"}, "discord/message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyMessage(tt.msg, cmds)
			if got != tt.expected {
				t.Errorf("classifyMessage() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestAssignTier(t *testing.T) {
	tests := []struct {
		name     string
		msg      DiscordMessage
		tier     string
		expected string
	}{
		{"pinned", DiscordMessage{Pinned: true}, "light", "full"},
		{"high reactions", DiscordMessage{Content: "great post", Reactions: []Reaction{{Emoji: "👍", Count: 5}}}, "light", "full"},
		{"link", DiscordMessage{Content: "https://example.com"}, "light", "full"},
		{"attachment", DiscordMessage{Attachments: []Attachment{{ID: "a1"}}}, "light", "standard"},
		{"code block", DiscordMessage{Content: "```go\ncode```"}, "light", "standard"},
		{"reply", DiscordMessage{Content: "I agree with this", MessageReference: &MessageRef{MessageID: "999"}}, "light", "standard"},
		{"embed", DiscordMessage{Embeds: []Embed{{}}}, "light", "standard"},
		{"short", DiscordMessage{Content: "ok"}, "light", "metadata"},
		{"normal with default", DiscordMessage{Content: "A normal message here"}, "standard", "standard"},
		{"normal no default", DiscordMessage{Content: "A normal message here"}, "", "light"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assignTier(tt.msg, tt.tier)
			if got != tt.expected {
				t.Errorf("assignTier() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestNormalizeMessage(t *testing.T) {
	msg := DiscordMessage{
		ID:          "123456789",
		Content:     "Check this article https://example.com/go-tips",
		Author:      Author{ID: "user1", Username: "gopher"},
		ChannelID:   "ch1",
		GuildID:     "guild1",
		Timestamp:   time.Now(),
		Pinned:      true,
		ServerName:  "GoLang Community",
		ChannelName: "#resources",
		MentionIDs:  []string{"user2"},
		Reactions:   []Reaction{{Emoji: "👍", Count: 3}},
	}

	artifact := normalizeMessage(msg, "standard", []string{"!save"})
	if artifact.SourceID != "discord" {
		t.Errorf("expected discord source, got %s", artifact.SourceID)
	}
	if artifact.SourceRef != "123456789" {
		t.Errorf("expected source ref 123456789, got %s", artifact.SourceRef)
	}
	if artifact.ContentType != "discord/link" {
		t.Errorf("expected discord/link, got %s", artifact.ContentType)
	}
	if artifact.Metadata["pinned"] != true {
		t.Error("expected pinned=true")
	}
	if artifact.Metadata["server_name"] != "GoLang Community" {
		t.Errorf("expected server_name GoLang Community, got %v", artifact.Metadata["server_name"])
	}
	if artifact.Metadata["channel_name"] != "#resources" {
		t.Errorf("expected channel_name #resources, got %v", artifact.Metadata["channel_name"])
	}
	if artifact.Metadata["reaction_count"] != 3 {
		t.Errorf("expected reaction_count 3, got %v", artifact.Metadata["reaction_count"])
	}
	mentions, ok := artifact.Metadata["mentions"].([]string)
	if !ok || len(mentions) != 1 || mentions[0] != "user2" {
		t.Errorf("expected mentions [user2], got %v", artifact.Metadata["mentions"])
	}
}

func TestNormalizeMessage_ThreadMetadata(t *testing.T) {
	msg := DiscordMessage{
		ID:         "111",
		Content:    "Thread discussion",
		Author:     Author{ID: "u1", Username: "alice"},
		ChannelID:  "ch1",
		GuildID:    "g1",
		Timestamp:  time.Now(),
		ThreadID:   "thread-1",
		ThreadName: "Go Generics Discussion",
	}

	artifact := normalizeMessage(msg, "standard", nil)
	if artifact.Metadata["thread_id"] != "thread-1" {
		t.Errorf("expected thread_id thread-1, got %v", artifact.Metadata["thread_id"])
	}
	if artifact.Metadata["thread_name"] != "Go Generics Discussion" {
		t.Errorf("expected thread_name, got %v", artifact.Metadata["thread_name"])
	}
	if artifact.ContentType != "discord/thread" {
		t.Errorf("expected discord/thread, got %s", artifact.ContentType)
	}
}

func TestNormalizeMessage_ReplyMetadata(t *testing.T) {
	msg := DiscordMessage{
		ID:               "222",
		Content:          "I agree with that point",
		Author:           Author{ID: "u2", Username: "bob"},
		ChannelID:        "ch2",
		GuildID:          "g1",
		Timestamp:        time.Now(),
		MessageReference: &MessageRef{MessageID: "111", ChannelID: "ch2", GuildID: "g1"},
	}

	artifact := normalizeMessage(msg, "standard", nil)
	if artifact.Metadata["reply_to_id"] != "111" {
		t.Errorf("expected reply_to_id 111, got %v", artifact.Metadata["reply_to_id"])
	}
	if artifact.ContentType != "discord/reply" {
		t.Errorf("expected discord/reply, got %s", artifact.ContentType)
	}
}

func TestBuildTitle(t *testing.T) {
	short := buildTitle(DiscordMessage{Content: "Hello"})
	if short != "Hello" {
		t.Errorf("expected Hello, got %s", short)
	}

	long := buildTitle(DiscordMessage{Content: "A" + string(make([]byte, 100))})
	if len(long) > 84 {
		t.Error("title should be truncated")
	}

	empty := buildTitle(DiscordMessage{})
	if empty != "Discord message" {
		t.Errorf("expected 'Discord message', got %s", empty)
	}
}

func TestTotalReactions(t *testing.T) {
	reactions := []Reaction{
		{Emoji: "👍", Count: 3},
		{Emoji: "❤️", Count: 2},
	}
	if total := totalReactions(reactions); total != 5 {
		t.Errorf("expected 5, got %d", total)
	}
	if total := totalReactions(nil); total != 0 {
		t.Errorf("expected 0, got %d", total)
	}
}

func TestParseBotCommand(t *testing.T) {
	cmds := []string{"!save", "!capture"}

	tests := []struct {
		name        string
		content     string
		wantURL     string
		wantComment string
		wantOK      bool
	}{
		{"save with url and comment", "!save https://example.com Great resource", "https://example.com", "Great resource", true},
		{"capture with url only", "!capture https://test.com", "https://test.com", "", true},
		{"save without url", "!save not a url text", "", "not a url text", true},
		{"no command", "hello world", "", "", false},
		{"command only no args", "!save", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, comment, ok := ParseBotCommand(tt.content, cmds)
			if ok != tt.wantOK {
				t.Errorf("ParseBotCommand ok = %v, want %v", ok, tt.wantOK)
			}
			if url != tt.wantURL {
				t.Errorf("ParseBotCommand url = %q, want %q", url, tt.wantURL)
			}
			if comment != tt.wantComment {
				t.Errorf("ParseBotCommand comment = %q, want %q", comment, tt.wantComment)
			}
		})
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter()

	// No bucket — should not wait
	if wait := rl.ShouldWait("channels/123/messages"); wait != 0 {
		t.Errorf("expected 0 wait, got %v", wait)
	}

	// Update with remaining > 1 — should not wait
	rl.Update("channels/123/messages", 5, time.Now().Add(time.Second))
	if wait := rl.ShouldWait("channels/123/messages"); wait != 0 {
		t.Errorf("expected 0 wait, got %v", wait)
	}

	// Update with remaining <= 1 — should wait
	rl.Update("channels/123/messages", 1, time.Now().Add(2*time.Second))
	wait := rl.ShouldWait("channels/123/messages")
	if wait <= 0 {
		t.Error("expected positive wait duration when rate limited")
	}

	// Expired bucket — should not wait
	rl.Update("channels/456/messages", 0, time.Now().Add(-time.Second))
	if wait := rl.ShouldWait("channels/456/messages"); wait != 0 {
		t.Errorf("expected 0 wait for expired bucket, got %v", wait)
	}
}
