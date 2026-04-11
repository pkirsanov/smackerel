package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/stringutil"
)

// testBotToken is a fake token for tests that need to pass length validation.
// Uses a format that won't trigger GitHub secret scanning push protection.
const testBotToken = "test-discord-bot-token-placeholder-that-is-long-enough-for-validation"

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
		Credentials: map[string]string{"bot_token": testBotToken},
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
		Credentials: map[string]string{"bot_token": testBotToken},
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
		Credentials: map[string]string{"bot_token": testBotToken},
	})
	err := c.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("closed connector should be disconnected")
	}
}

// --- Security Tests ---

func TestIsValidSnowflake(t *testing.T) {
	valid := []string{"123456789012345678", "0", "18446744073709551615"}
	for _, s := range valid {
		if !isValidSnowflake(s) {
			t.Errorf("expected %q to be a valid snowflake", s)
		}
	}
	invalid := []string{"", "abc", "-1", "12345678901234567890123", "../etc/passwd", "123 456", "123\n456"}
	for _, s := range invalid {
		if isValidSnowflake(s) {
			t.Errorf("expected %q to be an invalid snowflake", s)
		}
	}
}

func TestIsSafeURL(t *testing.T) {
	safe := []string{
		"https://example.com/page",
		"https://discord.com/channels/123/456/789",
		"http://public-site.org",
	}
	for _, u := range safe {
		if !isSafeURL(u) {
			t.Errorf("expected %q to be safe", u)
		}
	}
	unsafe := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost/admin",
		"http://127.0.0.1:8080/secret",
		"http://[::1]/secret",
		"http://0.0.0.0/",
		"http://192.168.1.1/internal",
		"http://10.0.0.1/internal",
		"http://metadata.google.internal/computeMetadata/",
	}
	for _, u := range unsafe {
		if isSafeURL(u) {
			t.Errorf("expected %q to be unsafe (SSRF)", u)
		}
	}
}

func TestConnect_InvalidSnowflakeServerID(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "not-a-snowflake",
					"channel_ids": []interface{}{"123456789012345678"},
				},
			},
		},
	})
	if err == nil {
		t.Error("expected error for invalid server_id snowflake")
	}
}

func TestConnect_InvalidSnowflakeChannelID(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "123456789012345678",
					"channel_ids": []interface{}{"../etc/passwd"},
				},
			},
		},
	})
	if err == nil {
		t.Error("expected error for invalid channel_id snowflake")
	}
}

func TestConnect_InvalidProcessingTier(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":       "123456789012345678",
					"channel_ids":     []interface{}{"123456789012345678"},
					"processing_tier": "evil_tier",
				},
			},
		},
	})
	if err == nil {
		t.Error("expected error for invalid processing_tier")
	}
}

func TestConnect_BackfillLimitUpperBound(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"backfill_limit": float64(999999),
		},
	})
	if err == nil {
		t.Error("expected error for backfill_limit exceeding maximum")
	}
}

func TestConnect_CaptureCommandTooLong(t *testing.T) {
	c := New("discord")
	longCmd := ""
	for i := 0; i < 100; i++ {
		longCmd += "x"
	}
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"capture_commands": []interface{}{longCmd},
		},
	})
	if err == nil {
		t.Error("expected error for capture_command exceeding max length")
	}
}

func TestConnect_CaptureCommandEmpty(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"capture_commands": []interface{}{""},
		},
	})
	if err == nil {
		t.Error("expected error for empty capture_command")
	}
}

func TestParseBotCommand_SSRFProtection(t *testing.T) {
	cmds := []string{"!save"}
	tests := []struct {
		content string
		wantURL string
	}{
		{"!save http://169.254.169.254/latest/meta-data/", ""},
		{"!save http://localhost/admin", ""},
		{"!save http://127.0.0.1:8080/secret", ""},
		{"!save http://10.0.0.1/internal", ""},
		{"!save https://example.com/safe", "https://example.com/safe"},
	}
	for _, tt := range tests {
		gotURL, _, ok := ParseBotCommand(tt.content, cmds)
		if !ok {
			t.Errorf("ParseBotCommand(%q): expected ok=true", tt.content)
			continue
		}
		if gotURL != tt.wantURL {
			t.Errorf("ParseBotCommand(%q): got URL %q, want %q", tt.content, gotURL, tt.wantURL)
		}
	}
}

func TestNormalizeMessage_InvalidSnowflakeOmitsURL(t *testing.T) {
	msg := DiscordMessage{
		ID:        "not-valid",
		Content:   "test",
		GuildID:   "also-not-valid",
		ChannelID: "nope",
		Timestamp: time.Now(),
	}
	artifact := normalizeMessage(msg, "light", nil)
	if artifact.URL != "" {
		t.Errorf("expected empty URL for invalid snowflake IDs, got %q", artifact.URL)
	}
}

func TestNormalizeMessage_ValidSnowflakeBuildsURL(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		GuildID:   "222222222222222222",
		ChannelID: "333333333333333333",
		Timestamp: time.Now(),
	}
	artifact := normalizeMessage(msg, "light", nil)
	expected := "https://discord.com/channels/222222222222222222/333333333333333333/111111111111111111"
	if artifact.URL != expected {
		t.Errorf("expected URL %q, got %q", expected, artifact.URL)
	}
}

func TestBuildTitle_ControlCharsSanitized(t *testing.T) {
	msg := DiscordMessage{
		Content: "hello\x00world\x07test",
	}
	title := buildTitle(msg)
	if title != "helloworldtest" {
		t.Errorf("expected control chars stripped, got %q", title)
	}
}

func TestSyncCursor_InvalidSnowflakeIgnored(t *testing.T) {
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
	})
	// Provide a cursor with an invalid channel ID — should not crash
	_, cursor, err := c.Sync(context.Background(), `{"../etc/passwd":"999","valid_but_not_configured":"111"}`)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cursor == "" {
		t.Error("cursor should not be empty")
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
		Author:      Author{ID: "100000000000000001", Username: "gopher"},
		ChannelID:   "ch1",
		GuildID:     "guild1",
		Timestamp:   time.Now(),
		Pinned:      true,
		ServerName:  "GoLang Community",
		ChannelName: "#resources",
		MentionIDs:  []string{"200000000000000001"},
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
	if !ok || len(mentions) != 1 || mentions[0] != "200000000000000001" {
		t.Errorf("expected mentions [200000000000000001], got %v", artifact.Metadata["mentions"])
	}
}

func TestNormalizeMessage_ThreadMetadata(t *testing.T) {
	msg := DiscordMessage{
		ID:         "111",
		Content:    "Thread discussion",
		Author:     Author{ID: "100000000000000001", Username: "alice"},
		ChannelID:  "ch1",
		GuildID:    "g1",
		Timestamp:  time.Now(),
		ThreadID:   "300000000000000001",
		ThreadName: "Go Generics Discussion",
	}

	artifact := normalizeMessage(msg, "standard", nil)
	if artifact.Metadata["thread_id"] != "300000000000000001" {
		t.Errorf("expected thread_id 300000000000000001, got %v", artifact.Metadata["thread_id"])
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
		Author:           Author{ID: "200000000000000001", Username: "bob"},
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

func TestRateLimiter_PruneExpired(t *testing.T) {
	rl := NewRateLimiter()

	// Add 101 expired buckets to trigger pruning on next Update
	for i := 0; i < 101; i++ {
		route := "channels/" + time.Now().Format("150405") + "/" + fmt.Sprintf("%d", i)
		rl.Update(route, 0, time.Now().Add(-time.Minute))
	}

	// Next Update should trigger pruning of expired entries
	rl.Update("channels/live/messages", 5, time.Now().Add(time.Minute))

	rl.mu.RLock()
	count := len(rl.buckets)
	rl.mu.RUnlock()

	// Only the live bucket should remain (expired ones pruned)
	if count > 2 {
		t.Errorf("expected most expired buckets pruned, got %d remaining", count)
	}
}

func TestSync_ContextCancellation(t *testing.T) {
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001", "200000000000000002"},
				},
			},
		},
	})

	// Cancel context before Sync
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.Sync(ctx, "")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestConnect_HealthRaceSafe(t *testing.T) {
	c := New("discord")
	done := make(chan struct{})

	// Concurrent health reads while connecting
	go func() {
		for i := 0; i < 100; i++ {
			c.Health(context.Background())
		}
		close(done)
	}()

	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
	})
	<-done
}

func TestClose_HealthRaceSafe(t *testing.T) {
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
	})

	done := make(chan struct{})

	// Concurrent health reads while closing
	go func() {
		for i := 0; i < 100; i++ {
			c.Health(context.Background())
		}
		close(done)
	}()

	c.Close()
	<-done
}

func TestBuildTitle_UTF8Safe(t *testing.T) {
	// 30 four-byte emoji runes = 120 bytes but only 30 runes
	emoji := ""
	for i := 0; i < 30; i++ {
		emoji += "🔥"
	}
	// Content is 30 runes; should not be truncated
	title := buildTitle(DiscordMessage{Content: emoji})
	if title != emoji {
		t.Error("30-rune title should not be truncated")
	}

	// 90 emoji runes = 360 bytes; byte-based [:80] would cut mid-rune
	longEmoji := ""
	for i := 0; i < 90; i++ {
		longEmoji += "🔥"
	}
	title = buildTitle(DiscordMessage{Content: longEmoji})
	runes := []rune(title)
	// 80 runes + "..." (3 runes) = 83 runes
	if len(runes) != 83 {
		t.Errorf("expected 83 runes (80 + ...), got %d", len(runes))
	}
}

func TestParseDiscordConfig_NegativeBackfillLimit(t *testing.T) {
	_, err := parseDiscordConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"backfill_limit": float64(-5),
		},
	})
	if err == nil {
		t.Error("expected error for negative backfill limit")
	}
}

func TestParseDiscordConfig_ZeroBackfillLimit(t *testing.T) {
	_, err := parseDiscordConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"backfill_limit": float64(0),
		},
	})
	if err == nil {
		t.Error("expected error for zero backfill limit")
	}
}

// --- Security Pass 2 Tests ---

func TestIsSafeURL_RejectsNonHTTPSchemes(t *testing.T) {
	dangerous := []string{
		"file:///etc/passwd",
		"gopher://evil.com/1",
		"ftp://internal.host/data",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"",
	}
	for _, u := range dangerous {
		if isSafeURL(u) {
			t.Errorf("expected %q to be rejected (non-http scheme)", u)
		}
	}
}

func TestSyncCursor_UnconfiguredChannelRejected(t *testing.T) {
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
		},
	})

	// Provide cursor with a valid snowflake channel ID that is NOT in monitored_channels
	_, cursorOut, err := c.Sync(context.Background(), `{"200000000000000001":"300000000000000001","999000000000000001":"400000000000000001"}`)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Parse output cursor — unconfigured channel 999000000000000001 must NOT appear
	var outputCursors map[string]string
	if err := json.Unmarshal([]byte(cursorOut), &outputCursors); err != nil {
		t.Fatalf("failed to parse output cursor: %v", err)
	}
	if _, found := outputCursors["999000000000000001"]; found {
		t.Error("cursor scope enforcement failed: unconfigured channel ID leaked into output cursor")
	}
	// Configured channel should still be accepted
	if _, found := outputCursors["200000000000000001"]; !found {
		t.Error("configured channel cursor was incorrectly rejected")
	}
}

func TestBuildTitle_NewlinesStripped(t *testing.T) {
	msg := DiscordMessage{
		Content: "line1\r\nline2\nline3\ttab",
	}
	title := buildTitle(msg)
	if title != "line1line2line3tab" {
		t.Errorf("expected newlines and tabs stripped from title, got %q", title)
	}
}

func TestBuildTitle_EmbedTitleNewlinesStripped(t *testing.T) {
	msg := DiscordMessage{
		Embeds: []Embed{{Title: "embed\r\ntitle\twith\ncontrol"}},
	}
	title := buildTitle(msg)
	if title != "embedtitlewithcontrol" {
		t.Errorf("expected newlines stripped from embed title, got %q", title)
	}
}

func TestConnect_MonitoredChannelsLimit(t *testing.T) {
	channels := make([]interface{}, 201)
	for i := range channels {
		channels[i] = map[string]interface{}{
			"server_id":   fmt.Sprintf("%018d", i+100000000000000001),
			"channel_ids": []interface{}{fmt.Sprintf("%018d", i+200000000000000001)},
		}
	}
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"monitored_channels": channels,
		},
	})
	if err == nil {
		t.Error("expected error for monitored_channels exceeding maximum")
	}
}

func TestConnect_BotTokenTooShort(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": "short"},
	})
	if err == nil {
		t.Error("expected error for bot token shorter than minimum length")
	}
}

func TestConnect_BotTokenControlChars(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": "valid-length-token-with\x00null-byte-injected"},
	})
	if err == nil {
		t.Error("expected error for bot token containing control characters")
	}
}

func TestSanitizeSingleLine(t *testing.T) {
	input := "hello\x00\x07\n\r\tworld"
	got := sanitizeSingleLine(input)
	if got != "helloworld" {
		t.Errorf("sanitizeSingleLine: expected %q, got %q", "helloworld", got)
	}
}

func TestSanitizeControlChars_PreservesNewlines(t *testing.T) {
	input := "hello\x00\x07\n\r\tworld"
	got := sanitizeControlChars(input)
	if got != "hello\n\r\tworld" {
		t.Errorf("sanitizeControlChars: expected newlines preserved, got %q", got)
	}
}

// --- Security Pass 3 Tests ---

func TestNormalizeMessage_RawContentSanitized(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "hello\x00world\x07injected\x1bnull",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
	}
	artifact := normalizeMessage(msg, "light", nil)
	if artifact.RawContent != "helloworldinjectednull" {
		t.Errorf("expected control chars stripped from RawContent, got %q", artifact.RawContent)
	}
}

func TestNormalizeMessage_RawContentTruncated(t *testing.T) {
	// Build content larger than maxRawContentLen (8192 bytes)
	long := ""
	for len(long) < 10000 {
		long += "abcdefghij"
	}
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   long,
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
	}
	artifact := normalizeMessage(msg, "light", nil)
	if len(artifact.RawContent) > 8192 {
		t.Errorf("expected RawContent truncated to 8192 bytes, got %d", len(artifact.RawContent))
	}
}

func TestNormalizeMessage_RawContentTruncateUTF8Safe(t *testing.T) {
	// 3000 four-byte emoji runes = 12000 bytes; truncation must not split a rune
	content := ""
	for i := 0; i < 3000; i++ {
		content += "🔥"
	}
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   content,
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
	}
	artifact := normalizeMessage(msg, "light", nil)
	if len(artifact.RawContent) > 8192 {
		t.Errorf("expected truncation to 8192 bytes, got %d", len(artifact.RawContent))
	}
	// Must be valid UTF-8 after truncation
	for i, r := range artifact.RawContent {
		if r == 65533 { // utf8.RuneError
			t.Errorf("invalid UTF-8 at byte %d after truncation", i)
			break
		}
	}
}

func TestNormalizeMessage_MetadataStringSanitized(t *testing.T) {
	msg := DiscordMessage{
		ID:          "111111111111111111",
		Content:     "test",
		Author:      Author{ID: "u1", Username: "alice\x00inject"},
		ChannelID:   "222222222222222222",
		GuildID:     "333333333333333333",
		Timestamp:   time.Now(),
		ServerName:  "Server\x07Name",
		ChannelName: "#chan\x1bnel",
		ThreadID:    "444444444444444444",
		ThreadName:  "Thread\x00Name",
	}
	artifact := normalizeMessage(msg, "light", nil)
	if got := artifact.Metadata["author_name"]; got != "aliceinject" {
		t.Errorf("expected sanitized author_name, got %q", got)
	}
	if got := artifact.Metadata["server_name"]; got != "ServerName" {
		t.Errorf("expected sanitized server_name, got %q", got)
	}
	if got := artifact.Metadata["channel_name"]; got != "#channel" {
		t.Errorf("expected sanitized channel_name, got %q", got)
	}
	if got := artifact.Metadata["thread_name"]; got != "ThreadName" {
		t.Errorf("expected sanitized thread_name, got %q", got)
	}
}

func TestNormalizeMessage_AttachmentURLSchemeSanitized(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
		Attachments: []Attachment{
			{ID: "a1", Filename: "safe.png", URL: "https://cdn.discordapp.com/safe.png", Size: 100},
			{ID: "a2", Filename: "evil.txt", URL: "file:///etc/passwd", Size: 50},
			{ID: "a3", Filename: "data.html", URL: "javascript:alert(1)", Size: 10},
		},
	}
	artifact := normalizeMessage(msg, "light", nil)
	attachments, ok := artifact.Metadata["attachments"].([]Attachment)
	if !ok {
		t.Fatal("expected attachments in metadata")
	}
	if len(attachments) != 3 {
		t.Fatalf("expected 3 attachments, got %d", len(attachments))
	}
	if attachments[0].URL != "https://cdn.discordapp.com/safe.png" {
		t.Errorf("expected safe URL preserved, got %q", attachments[0].URL)
	}
	if attachments[1].URL != "" {
		t.Errorf("expected file:// URL stripped, got %q", attachments[1].URL)
	}
	if attachments[2].URL != "" {
		t.Errorf("expected javascript: URL stripped, got %q", attachments[2].URL)
	}
}

func TestSanitizeEmbedURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/img.png", "https://example.com/img.png"},
		{"http://example.com/file", "http://example.com/file"},
		{"file:///etc/passwd", ""},
		{"javascript:alert(1)", ""},
		{"data:text/html,test", ""},
		{"ftp://evil.com/data", ""},
		{"gopher://evil.com/1", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := sanitizeEmbedURL(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeEmbedURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncateUTF8(t *testing.T) {
	// Short string — no truncation
	if got := stringutil.TruncateUTF8("hello", 10); got != "hello" {
		t.Errorf("expected no truncation, got %q", got)
	}
	// Exact fit
	if got := stringutil.TruncateUTF8("hello", 5); got != "hello" {
		t.Errorf("expected exact fit, got %q", got)
	}
	// ASCII truncation
	if got := stringutil.TruncateUTF8("hello world", 5); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
	// UTF-8 boundary: 🔥 is 4 bytes; maxBytes=5 should yield one emoji (4 bytes)
	emoji := "🔥🔥"
	got := stringutil.TruncateUTF8(emoji, 5)
	if got != "🔥" {
		t.Errorf("expected single emoji, got %q (len=%d)", got, len(got))
	}
}

// --- Hardening Pass Tests ---

func TestSanitizeEmbedURL_SSRFProtection(t *testing.T) {
	// sanitizeEmbedURL now delegates to isSafeURL — must reject private/metadata endpoints
	ssrfTargets := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost/admin",
		"http://127.0.0.1:8080/secret",
		"http://10.0.0.1/internal",
		"http://192.168.1.1/router",
		"http://metadata.google.internal/computeMetadata/",
	}
	for _, u := range ssrfTargets {
		if got := sanitizeEmbedURL(u); got != "" {
			t.Errorf("sanitizeEmbedURL(%q) should reject SSRF target, got %q", u, got)
		}
	}
	// Safe URLs must still pass
	if got := sanitizeEmbedURL("https://cdn.discordapp.com/img.png"); got == "" {
		t.Error("sanitizeEmbedURL should allow safe https URL")
	}
}

func TestNormalizeMessage_EmbedURLsSanitized(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "check this",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
		Embeds: []Embed{
			{Title: "Safe Embed", URL: "https://example.com/article", Description: "Good content"},
			{Title: "SSRF Embed", URL: "http://169.254.169.254/meta", Description: "Metadata steal"},
			{Title: "Scheme Inject", URL: "javascript:alert(1)", Description: "XSS"},
		},
	}
	artifact := normalizeMessage(msg, "light", nil)
	embeds, ok := artifact.Metadata["embeds"].([]Embed)
	if !ok {
		t.Fatal("expected embeds in metadata")
	}
	if len(embeds) != 3 {
		t.Fatalf("expected 3 embeds, got %d", len(embeds))
	}
	if embeds[0].URL != "https://example.com/article" {
		t.Errorf("safe embed URL should be preserved, got %q", embeds[0].URL)
	}
	if embeds[1].URL != "" {
		t.Errorf("SSRF embed URL should be stripped, got %q", embeds[1].URL)
	}
	if embeds[2].URL != "" {
		t.Errorf("scheme-inject embed URL should be stripped, got %q", embeds[2].URL)
	}
}

func TestNormalizeMessage_AttachmentURLSSRF(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
		Attachments: []Attachment{
			{ID: "a1", Filename: "safe.png", URL: "https://cdn.discordapp.com/safe.png", Size: 100},
			{ID: "a2", Filename: "ssrf.txt", URL: "http://169.254.169.254/latest/meta-data/", Size: 50},
			{ID: "a3", Filename: "priv.txt", URL: "http://10.0.0.1/internal", Size: 30},
		},
	}
	artifact := normalizeMessage(msg, "light", nil)
	attachments, ok := artifact.Metadata["attachments"].([]Attachment)
	if !ok {
		t.Fatal("expected attachments in metadata")
	}
	if attachments[0].URL != "https://cdn.discordapp.com/safe.png" {
		t.Errorf("safe attachment URL should be preserved, got %q", attachments[0].URL)
	}
	if attachments[1].URL != "" {
		t.Errorf("SSRF attachment URL should be stripped, got %q", attachments[1].URL)
	}
	if attachments[2].URL != "" {
		t.Errorf("private-IP attachment URL should be stripped, got %q", attachments[2].URL)
	}
}

func TestNormalizeMessage_InvalidMentionIDsFiltered(t *testing.T) {
	msg := DiscordMessage{
		ID:         "111111111111111111",
		Content:    "test mentions",
		ChannelID:  "222222222222222222",
		GuildID:    "333333333333333333",
		Author:     Author{ID: "444444444444444444", Username: "alice"},
		Timestamp:  time.Now(),
		MentionIDs: []string{"555555555555555555", "../etc/passwd", "not-a-snowflake", "666666666666666666"},
	}
	artifact := normalizeMessage(msg, "light", nil)
	mentions, ok := artifact.Metadata["mentions"].([]string)
	if !ok {
		t.Fatal("expected mentions in metadata")
	}
	if len(mentions) != 2 {
		t.Errorf("expected 2 valid mention IDs, got %d: %v", len(mentions), mentions)
	}
	if mentions[0] != "555555555555555555" || mentions[1] != "666666666666666666" {
		t.Errorf("unexpected valid mentions: %v", mentions)
	}
}

func TestNormalizeMessage_InvalidThreadIDOmitted(t *testing.T) {
	msg := DiscordMessage{
		ID:         "111111111111111111",
		Content:    "thread test",
		ChannelID:  "222222222222222222",
		GuildID:    "333333333333333333",
		Timestamp:  time.Now(),
		ThreadID:   "not-a-snowflake",
		ThreadName: "Injected Thread",
	}
	artifact := normalizeMessage(msg, "light", nil)
	if _, found := artifact.Metadata["thread_id"]; found {
		t.Error("invalid thread_id should not be stored in metadata")
	}
}

func TestNormalizeMessage_InvalidReplyToIDOmitted(t *testing.T) {
	msg := DiscordMessage{
		ID:               "111111111111111111",
		Content:          "reply test",
		ChannelID:        "222222222222222222",
		GuildID:          "333333333333333333",
		Timestamp:        time.Now(),
		MessageReference: &MessageRef{MessageID: "../inject", ChannelID: "222222222222222222", GuildID: "333333333333333333"},
	}
	artifact := normalizeMessage(msg, "light", nil)
	if _, found := artifact.Metadata["reply_to_id"]; found {
		t.Error("invalid reply_to_id should not be stored in metadata")
	}
}

func TestNormalizeMessage_InvalidAuthorIDOmitted(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Author:    Author{ID: "not-a-snowflake", Username: "bob"},
		Timestamp: time.Now(),
	}
	artifact := normalizeMessage(msg, "light", nil)
	if _, found := artifact.Metadata["author_id"]; found {
		t.Error("invalid author_id should not be stored in metadata")
	}
	// author_name should still be stored
	if artifact.Metadata["author_name"] != "bob" {
		t.Errorf("author_name should still be stored, got %v", artifact.Metadata["author_name"])
	}
}

func TestNormalizeMessage_MetadataArraysCapped(t *testing.T) {
	// Build message with oversized arrays
	mentions := make([]string, 150)
	for i := range mentions {
		mentions[i] = fmt.Sprintf("%018d", i+100000000000000001)
	}
	reactions := make([]Reaction, 150)
	for i := range reactions {
		reactions[i] = Reaction{Emoji: "👍", Count: 1}
	}
	attachments := make([]Attachment, 60)
	for i := range attachments {
		attachments[i] = Attachment{ID: fmt.Sprintf("a%d", i), Filename: "f.txt", URL: "https://cdn.discordapp.com/f.txt", Size: 10}
	}
	embeds := make([]Embed, 30)
	for i := range embeds {
		embeds[i] = Embed{Title: fmt.Sprintf("Embed %d", i), URL: "https://example.com"}
	}
	msg := DiscordMessage{
		ID:          "111111111111111111",
		Content:     "test caps",
		ChannelID:   "222222222222222222",
		GuildID:     "333333333333333333",
		Timestamp:   time.Now(),
		MentionIDs:  mentions,
		Reactions:   reactions,
		Attachments: attachments,
		Embeds:      embeds,
	}
	artifact := normalizeMessage(msg, "light", nil)

	storedMentions := artifact.Metadata["mentions"].([]string)
	if len(storedMentions) > 100 {
		t.Errorf("mentions should be capped at 100, got %d", len(storedMentions))
	}
	storedReactions := artifact.Metadata["reactions"].([]Reaction)
	if len(storedReactions) > 100 {
		t.Errorf("reactions should be capped at 100, got %d", len(storedReactions))
	}
	storedAttachments := artifact.Metadata["attachments"].([]Attachment)
	if len(storedAttachments) > 50 {
		t.Errorf("attachments should be capped at 50, got %d", len(storedAttachments))
	}
	storedEmbeds := artifact.Metadata["embeds"].([]Embed)
	if len(storedEmbeds) > 25 {
		t.Errorf("embeds should be capped at 25, got %d", len(storedEmbeds))
	}
	// Embed count should reflect the original count, not the capped count
	if artifact.Metadata["embed_count"] != 30 {
		t.Errorf("embed_count should reflect original count 30, got %v", artifact.Metadata["embed_count"])
	}
}

func TestParseBotCommand_CommentTruncated(t *testing.T) {
	cmds := []string{"!save"}
	// Build a comment longer than maxBotCommandCommentLen (2000 bytes)
	longComment := ""
	for len(longComment) < 3000 {
		longComment += "abcdefghij"
	}
	content := "!save " + longComment

	_, comment, ok := ParseBotCommand(content, cmds)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(comment) > maxBotCommandCommentLen {
		t.Errorf("comment should be truncated to %d bytes, got %d", maxBotCommandCommentLen, len(comment))
	}
}

func TestParseBotCommand_CommentWithURLTruncated(t *testing.T) {
	cmds := []string{"!save"}
	longComment := ""
	for len(longComment) < 3000 {
		longComment += "abcdefghij"
	}
	content := "!save https://example.com " + longComment

	parsedURL, comment, ok := ParseBotCommand(content, cmds)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if parsedURL != "https://example.com" {
		t.Errorf("expected URL preserved, got %q", parsedURL)
	}
	if len(comment) > maxBotCommandCommentLen {
		t.Errorf("comment should be truncated to %d bytes, got %d", maxBotCommandCommentLen, len(comment))
	}
}

func TestNormalizeMessage_EmbedFieldsSanitized(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
		Embeds:    []Embed{{Title: "Title\x00Inject", Description: "Desc\x07Bell", URL: "https://example.com"}},
	}
	artifact := normalizeMessage(msg, "light", nil)
	embeds := artifact.Metadata["embeds"].([]Embed)
	if embeds[0].Title != "TitleInject" {
		t.Errorf("expected sanitized embed title, got %q", embeds[0].Title)
	}
	if embeds[0].Description != "DescBell" {
		t.Errorf("expected sanitized embed description, got %q", embeds[0].Description)
	}
}

func TestNormalizeMessage_AttachmentFilenameSanitized(t *testing.T) {
	msg := DiscordMessage{
		ID:          "111111111111111111",
		Content:     "test",
		ChannelID:   "222222222222222222",
		GuildID:     "333333333333333333",
		Timestamp:   time.Now(),
		Attachments: []Attachment{{ID: "a1", Filename: "file\x00name.txt", URL: "https://cdn.discordapp.com/f.txt", Size: 10}},
	}
	artifact := normalizeMessage(msg, "light", nil)
	attachments := artifact.Metadata["attachments"].([]Attachment)
	if attachments[0].Filename != "filename.txt" {
		t.Errorf("expected sanitized filename, got %q", attachments[0].Filename)
	}
}

func TestNormalizeMessage_AttachmentFilenamePathTraversalStripped(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
		Attachments: []Attachment{
			{ID: "a1", Filename: "../../etc/passwd", URL: "https://cdn.discordapp.com/f.txt", Size: 10},
			{ID: "a2", Filename: "../secret/keys.pem", URL: "https://cdn.discordapp.com/k.pem", Size: 20},
			{ID: "a3", Filename: "normal.png", URL: "https://cdn.discordapp.com/n.png", Size: 30},
		},
	}
	artifact := normalizeMessage(msg, "light", nil)
	attachments := artifact.Metadata["attachments"].([]Attachment)
	if attachments[0].Filename != "passwd" {
		t.Errorf("expected path traversal stripped to basename 'passwd', got %q", attachments[0].Filename)
	}
	if attachments[1].Filename != "keys.pem" {
		t.Errorf("expected path traversal stripped to basename 'keys.pem', got %q", attachments[1].Filename)
	}
	if attachments[2].Filename != "normal.png" {
		t.Errorf("expected normal filename preserved, got %q", attachments[2].Filename)
	}
}

func TestNormalizeMessage_EmbedTitleTruncated(t *testing.T) {
	longTitle := ""
	for len(longTitle) < 500 {
		longTitle += "abcdefghij"
	}
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
		Embeds:    []Embed{{Title: longTitle, URL: "https://example.com"}},
	}
	artifact := normalizeMessage(msg, "light", nil)
	embeds := artifact.Metadata["embeds"].([]Embed)
	if len(embeds[0].Title) > maxEmbedTitleLen {
		t.Errorf("embed title should be truncated to %d bytes, got %d", maxEmbedTitleLen, len(embeds[0].Title))
	}
}

func TestNormalizeMessage_EmbedDescriptionTruncated(t *testing.T) {
	longDesc := ""
	for len(longDesc) < 6000 {
		longDesc += "abcdefghij"
	}
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
		Embeds:    []Embed{{Description: longDesc, URL: "https://example.com"}},
	}
	artifact := normalizeMessage(msg, "light", nil)
	embeds := artifact.Metadata["embeds"].([]Embed)
	if len(embeds[0].Description) > maxEmbedDescLen {
		t.Errorf("embed description should be truncated to %d bytes, got %d", maxEmbedDescLen, len(embeds[0].Description))
	}
}

func TestNormalizeMessage_ReactionEmojiSanitized(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
		Reactions: []Reaction{
			{Emoji: "👍\x00injected", Count: 3},
			{Emoji: "❤️", Count: 2},
		},
	}
	artifact := normalizeMessage(msg, "light", nil)
	reactions := artifact.Metadata["reactions"].([]Reaction)
	if reactions[0].Emoji != "👍injected" {
		t.Errorf("expected control chars stripped from emoji, got %q", reactions[0].Emoji)
	}
	if reactions[0].Count != 3 {
		t.Errorf("expected reaction count preserved, got %d", reactions[0].Count)
	}
	if reactions[1].Emoji != "❤️" {
		t.Errorf("expected normal emoji preserved, got %q", reactions[1].Emoji)
	}
}

func TestConnect_CursorRestorationValidatesSnowflakes(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"cursors": `{"100000000000000001":"200000000000000001","../etc/passwd":"300000000000000001","400000000000000001":"not-a-snowflake"}`,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Valid cursor should be restored
	if v, ok := c.cursors["100000000000000001"]; !ok || v != "200000000000000001" {
		t.Errorf("expected valid cursor restored, got %v", c.cursors)
	}
	// Invalid channel ID should be rejected
	if _, ok := c.cursors["../etc/passwd"]; ok {
		t.Error("invalid channel ID should not be restored into cursors")
	}
	// Invalid value should be rejected
	if _, ok := c.cursors["400000000000000001"]; ok {
		t.Error("cursor with invalid snowflake value should not be restored")
	}
}
