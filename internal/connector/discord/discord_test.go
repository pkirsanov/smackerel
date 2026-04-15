package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/stringutil"
)

// testBotToken is a fake token for tests that need to pass length validation.
// Uses a format that won't trigger GitHub secret scanning push protection.
const testBotToken = "test-discord-bot-token-placeholder-that-is-long-enough-for-validation"

// newTestDiscordAPI creates an httptest.Server simulating the Discord REST API.
// By default it responds to /users/@me with a valid bot user and returns empty
// arrays for all other endpoints. Custom handler functions can override routes.
func newTestDiscordAPI(t *testing.T, handlers ...func(*http.ServeMux)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/users/@me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"999999999999999999","username":"TestBot"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Thread endpoints expect object responses, all others expect arrays
		path := r.URL.Path
		if strings.Contains(path, "/threads/active") ||
			strings.Contains(path, "/threads/archived/") {
			w.Write([]byte(`{"threads":[],"has_more":false}`))
			return
		}
		w.Write([]byte("[]"))
	})
	for _, h := range handlers {
		h(mux)
	}
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

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
	ts := newTestDiscordAPI(t)
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":          ts.URL,
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
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
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
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
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
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
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
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
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
	ts := newTestDiscordAPI(t)
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
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})
	<-done
}

func TestClose_HealthRaceSafe(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
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
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
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
	ts := newTestDiscordAPI(t)
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "900000000000000001",
					"channel_ids": []interface{}{"100000000000000001", "400000000000000001"},
				},
			},
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

// --- Stabilize Pass 2 Tests ---

func TestSync_AfterClose_ReturnsError(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})
	c.Close()

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error when syncing a closed connector")
	}
}

func TestSync_HealthDegradedOnPartialFailure(t *testing.T) {
	// Verify that after a clean sync, health returns to healthy
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
		},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if h := c.Health(context.Background()); h != connector.HealthHealthy {
		t.Errorf("expected HealthHealthy after clean sync, got %s", h)
	}
}

func TestClose_SetsClosed(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})
	if c.closed {
		t.Error("connector should not be closed before Close()")
	}
	c.Close()
	if !c.closed {
		t.Error("connector should be closed after Close()")
	}
}

func TestSync_HealthTransitionsDuringSyncLifecycle(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})

	if h := c.Health(context.Background()); h != connector.HealthHealthy {
		t.Errorf("expected HealthHealthy before sync, got %s", h)
	}

	c.Sync(context.Background(), "")

	if h := c.Health(context.Background()); h != connector.HealthHealthy {
		t.Errorf("expected HealthHealthy after clean sync, got %s", h)
	}
}

func TestConnect_SetsHealthErrorOnConfigFailure(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"backfill_limit": float64(-1),
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
	if h := c.Health(context.Background()); h != connector.HealthError {
		t.Errorf("expected HealthError after config failure, got %s", h)
	}
}

func TestConnect_SetsHealthErrorOnMissingToken(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if h := c.Health(context.Background()); h != connector.HealthError {
		t.Errorf("expected HealthError after missing token, got %s", h)
	}
}

func TestConnect_SetsHealthErrorOnShortToken(t *testing.T) {
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": "short"},
	})
	if err == nil {
		t.Fatal("expected error for short token")
	}
	if h := c.Health(context.Background()); h != connector.HealthError {
		t.Errorf("expected HealthError after short token, got %s", h)
	}
}

func TestParseDiscordConfig_Defaults(t *testing.T) {
	cfg, err := parseDiscordConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIURL != discordDefaultAPIURL {
		t.Errorf("expected default APIURL %q, got %q", discordDefaultAPIURL, cfg.APIURL)
	}
	if cfg.BackfillLimit != 1000 {
		t.Errorf("expected default backfill_limit 1000, got %d", cfg.BackfillLimit)
	}
	if !cfg.EnableGateway {
		t.Error("expected default enable_gateway true")
	}
	if !cfg.IncludeThreads {
		t.Error("expected default include_threads true")
	}
	if !cfg.IncludePins {
		t.Error("expected default include_pins true")
	}
	if len(cfg.CaptureCommands) != 2 || cfg.CaptureCommands[0] != "!save" || cfg.CaptureCommands[1] != "!capture" {
		t.Errorf("expected default capture_commands [!save, !capture], got %v", cfg.CaptureCommands)
	}
}

func TestSync_RecordsSyncMetadata(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
		},
	})

	c.Sync(context.Background(), "")

	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.lastSyncTime.IsZero() {
		t.Error("expected lastSyncTime to be set after sync")
	}
	if c.lastSyncErrors != 0 {
		t.Errorf("expected 0 sync errors, got %d", c.lastSyncErrors)
	}
}

func TestNormalizeMessage_CaptureCommandContentType(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "!save https://example.com/article Great read",
		Author:    Author{ID: "222222222222222222", Username: "alice"},
		ChannelID: "333333333333333333",
		GuildID:   "444444444444444444",
		Timestamp: time.Now(),
	}

	artifact := normalizeMessage(msg, "standard", []string{"!save"})
	if artifact.ContentType != "discord/capture" {
		t.Errorf("expected discord/capture for bot command, got %s", artifact.ContentType)
	}
}

func TestCompileTimeInterfaceCheck(t *testing.T) {
	// Verify the connector satisfies the interface at runtime too
	var c connector.Connector = New("test")
	if c.ID() != "test" {
		t.Errorf("expected ID 'test', got %s", c.ID())
	}
}

// --- Hardening Pass 2 Tests ---

func TestTotalReactions_NegativeCountIgnored(t *testing.T) {
	reactions := []Reaction{
		{Emoji: "👍", Count: 5},
		{Emoji: "👎", Count: -3},
		{Emoji: "❤️", Count: 2},
	}
	got := totalReactions(reactions)
	if got != 7 {
		t.Errorf("expected 7 (negative counts ignored), got %d", got)
	}
}

func TestNormalizeMessage_NegativeReactionCountClamped(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
		Reactions: []Reaction{{Emoji: "👎", Count: -10}},
	}
	artifact := normalizeMessage(msg, "light", nil)
	reactions := artifact.Metadata["reactions"].([]Reaction)
	if reactions[0].Count != 0 {
		t.Errorf("expected negative count clamped to 0, got %d", reactions[0].Count)
	}
}

func TestNormalizeMessage_NegativeAttachmentSizeClamped(t *testing.T) {
	msg := DiscordMessage{
		ID:          "111111111111111111",
		Content:     "test",
		ChannelID:   "222222222222222222",
		GuildID:     "333333333333333333",
		Timestamp:   time.Now(),
		Attachments: []Attachment{{ID: "a1", Filename: "f.txt", URL: "https://cdn.discordapp.com/f.txt", Size: -100}},
	}
	artifact := normalizeMessage(msg, "light", nil)
	attachments := artifact.Metadata["attachments"].([]Attachment)
	if attachments[0].Size != 0 {
		t.Errorf("expected negative size clamped to 0, got %d", attachments[0].Size)
	}
}

func TestNormalizeMessage_InvalidMsgIDEmptySourceRef(t *testing.T) {
	msg := DiscordMessage{
		ID:        "not-a-snowflake",
		Content:   "test",
		ChannelID: "222222222222222222",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
	}
	artifact := normalizeMessage(msg, "light", nil)
	if artifact.SourceRef != "" {
		t.Errorf("expected empty SourceRef for invalid msg ID, got %q", artifact.SourceRef)
	}
	if _, found := artifact.Metadata["message_id"]; found {
		t.Error("invalid message_id should not be stored in metadata")
	}
}

func TestNormalizeMessage_InvalidChannelIDOmittedFromMetadata(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		ChannelID: "not-valid",
		GuildID:   "333333333333333333",
		Timestamp: time.Now(),
	}
	artifact := normalizeMessage(msg, "light", nil)
	if _, found := artifact.Metadata["channel_id"]; found {
		t.Error("invalid channel_id should not be stored in metadata")
	}
}

func TestNormalizeMessage_InvalidGuildIDOmittedFromMetadata(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "test",
		ChannelID: "222222222222222222",
		GuildID:   "../../inject",
		Timestamp: time.Now(),
	}
	artifact := normalizeMessage(msg, "light", nil)
	if _, found := artifact.Metadata["server_id"]; found {
		t.Error("invalid server_id should not be stored in metadata")
	}
}

func TestNormalizeMessage_LongMetadataStringsTruncated(t *testing.T) {
	longStr := ""
	for len(longStr) < 500 {
		longStr += "abcdefghij"
	}
	msg := DiscordMessage{
		ID:          "111111111111111111",
		Content:     "test",
		ChannelID:   "222222222222222222",
		GuildID:     "333333333333333333",
		Author:      Author{ID: "444444444444444444", Username: longStr},
		Timestamp:   time.Now(),
		ServerName:  longStr,
		ChannelName: longStr,
		ThreadID:    "555555555555555555",
		ThreadName:  longStr,
	}
	artifact := normalizeMessage(msg, "light", nil)
	if len(artifact.Metadata["author_name"].(string)) > 256 {
		t.Errorf("author_name should be capped at 256 bytes, got %d", len(artifact.Metadata["author_name"].(string)))
	}
	if len(artifact.Metadata["server_name"].(string)) > 256 {
		t.Errorf("server_name should be capped at 256 bytes, got %d", len(artifact.Metadata["server_name"].(string)))
	}
	if len(artifact.Metadata["channel_name"].(string)) > 256 {
		t.Errorf("channel_name should be capped at 256 bytes, got %d", len(artifact.Metadata["channel_name"].(string)))
	}
	if len(artifact.Metadata["thread_name"].(string)) > 256 {
		t.Errorf("thread_name should be capped at 256 bytes, got %d", len(artifact.Metadata["thread_name"].(string)))
	}
}

func TestParseDiscordConfig_TotalChannelsCap(t *testing.T) {
	// 5 server entries × 250 channels each = 1250 > maxTotalChannels (1000)
	servers := make([]interface{}, 5)
	for i := range servers {
		channels := make([]interface{}, 250)
		for j := range channels {
			channels[j] = fmt.Sprintf("%018d", (i*250+j)+100000000000000001)
		}
		servers[i] = map[string]interface{}{
			"server_id":   fmt.Sprintf("%018d", i+200000000000000001),
			"channel_ids": channels,
		}
	}
	_, err := parseDiscordConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"monitored_channels": servers,
		},
	})
	if err == nil {
		t.Error("expected error for total channel_ids exceeding maximum")
	}
}

func TestTotalReactions_AllNegative(t *testing.T) {
	reactions := []Reaction{
		{Emoji: "👎", Count: -5},
		{Emoji: "💀", Count: -3},
	}
	got := totalReactions(reactions)
	if got != 0 {
		t.Errorf("expected 0 for all negative counts, got %d", got)
	}
}

// --- Chaos Hardening Tests ---

// C1: Concurrent Connect + Sync must not race.
// go test -race catches this if the config snapshot fix is missing.
func TestChaos_ConcurrentConnectSync(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
		},
	})

	var wg sync.WaitGroup
	wg.Add(2)

	// Concurrent Sync
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			c.Sync(context.Background(), "")
		}
	}()

	// Concurrent re-Connect with different config
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			c.Connect(context.Background(), connector.ConnectorConfig{
				Credentials: map[string]string{"bot_token": testBotToken},
				SourceConfig: map[string]interface{}{
					"api_url": ts.URL,
					"monitored_channels": []interface{}{
						map[string]interface{}{
							"server_id":   "300000000000000001",
							"channel_ids": []interface{}{"400000000000000001"},
						},
					},
				},
			})
		}
	}()

	wg.Wait()
	// If we get here without -race detection, the fix is working
}

// C1b: Concurrent Sync + Close must not race.
func TestChaos_ConcurrentSyncClose(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
		},
	})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			c.Sync(context.Background(), "")
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			c.Close()
			// Re-connect for next iteration
			c.Connect(context.Background(), connector.ConnectorConfig{
				Credentials: map[string]string{"bot_token": testBotToken},
				SourceConfig: map[string]interface{}{
					"api_url": ts.URL,
					"monitored_channels": []interface{}{
						map[string]interface{}{
							"server_id":   "100000000000000001",
							"channel_ids": []interface{}{"200000000000000001"},
						},
					},
				},
			})
		}
	}()

	wg.Wait()
}

// C1c: Concurrent Health reads during Sync and Connect.
func TestChaos_ConcurrentHealthSyncConnect(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			c.Health(context.Background())
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			c.Sync(context.Background(), "")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			c.Connect(context.Background(), connector.ConnectorConfig{
				Credentials:  map[string]string{"bot_token": testBotToken},
				SourceConfig: map[string]interface{}{"api_url": ts.URL},
			})
		}
	}()

	wg.Wait()
}

// C2: Integer overflow in totalReactions must not wrap to negative.
func TestChaos_TotalReactionsOverflow(t *testing.T) {
	reactions := []Reaction{
		{Emoji: "👍", Count: math.MaxInt32},
		{Emoji: "❤️", Count: math.MaxInt32},
		{Emoji: "🔥", Count: math.MaxInt32},
	}
	total := totalReactions(reactions)
	if total < 0 {
		t.Fatalf("totalReactions overflowed to negative: %d", total)
	}
	if total < 5 {
		t.Errorf("total should be ≥5 to trigger full tier, got %d", total)
	}
}

// C2b: Overflow must still trigger "full" tier, not "metadata" or "light".
func TestChaos_OverflowReactionsTriggerFullTier(t *testing.T) {
	msg := DiscordMessage{
		Content: "normal short msg",
		Reactions: []Reaction{
			{Emoji: "👍", Count: math.MaxInt32},
			{Emoji: "❤️", Count: math.MaxInt32},
		},
	}
	tier := assignTier(msg, "light")
	if tier != "full" {
		t.Errorf("expected 'full' tier when reactions overflow, got %q", tier)
	}
}

// C4: Re-Connect clears stale cursors from previous channel config.
func TestChaos_ReConnectClearsUnmonitoredCursors(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")

	// First connect: monitor channel A
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
			"cursors": `{"200000000000000001":"300000000000000001"}`,
		},
	})

	// Verify cursor was set
	c.mu.RLock()
	if _, ok := c.cursors["200000000000000001"]; !ok {
		c.mu.RUnlock()
		t.Fatal("expected cursor for channel A after first connect")
	}
	c.mu.RUnlock()

	// Second connect: monitor channel B (not A)
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"400000000000000001"},
				},
			},
		},
	})

	// Channel A cursor must be gone
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, ok := c.cursors["200000000000000001"]; ok {
		t.Error("stale cursor for channel A should be cleared after re-Connect with different config")
	}
}

// C4b: Re-Connect resets closed flag so Sync works after Close+Connect cycle.
func TestChaos_ReConnectAfterCloseResetsState(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})
	c.Close()

	// Re-connect should reset the closed flag
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})
	if err != nil {
		t.Fatalf("unexpected error on re-connect: %v", err)
	}

	// Sync should work after re-connect
	_, _, err = c.Sync(context.Background(), "")
	if err != nil {
		t.Errorf("Sync should succeed after Close+Connect cycle, got: %v", err)
	}
}

// C5: Rapid successive Syncs must not corrupt cursors.
func TestChaos_RapidSuccessiveSyncs(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001", "200000000000000002"},
				},
			},
		},
	})

	var lastCursor string
	for i := 0; i < 100; i++ {
		_, cursor, err := c.Sync(context.Background(), lastCursor)
		if err != nil {
			t.Fatalf("sync %d failed: %v", i, err)
		}
		// Cursor must always be valid JSON
		var parsed map[string]string
		if err := json.Unmarshal([]byte(cursor), &parsed); err != nil {
			t.Fatalf("sync %d produced invalid cursor JSON: %v", i, err)
		}
		lastCursor = cursor
	}
}

// C6: Double Close must not panic.
func TestChaos_DoubleClose(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})
	if err := c.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second close should not fail: %v", err)
	}
	if h := c.Health(context.Background()); h != connector.HealthDisconnected {
		t.Errorf("expected disconnected after double close, got %s", h)
	}
}

// --- Regression Tests (REG-014-R22) ---

// REG-014-R22-001: Connect() cursor restoration must reject channels
// not in the current MonitoredChannels config. Without the scope check,
// stale channel cursors from a previous config would leak into Sync()
// output, disclosing channel IDs the user is no longer monitoring.
func TestRegression_ConnectCursorScopeEnforcement(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
			// Cursor JSON contains a monitored channel AND a stale channel
			"cursors": `{"200000000000000001":"300000000000000001","999000000000000001":"400000000000000001"}`,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Configured channel cursor should be restored
	if v, ok := c.cursors["200000000000000001"]; !ok || v != "300000000000000001" {
		t.Errorf("monitored channel cursor should be restored, got cursors=%v", c.cursors)
	}
	// Unconfigured channel cursor must NOT be restored
	if _, ok := c.cursors["999000000000000001"]; ok {
		t.Error("cursor for unconfigured channel 999000000000000001 must not be restored (scope enforcement gap)")
	}
}

// REG-014-R22-001b: Connect() stale cursors must not leak into Sync() output.
func TestRegression_ConnectStaleCursorsNotInSyncOutput(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
			// Stale cursor for a channel that is NOT monitored
			"cursors": `{"200000000000000001":"300000000000000001","888000000000000001":"500000000000000001"}`,
		},
	})

	_, cursorOut, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var outputCursors map[string]string
	if err := json.Unmarshal([]byte(cursorOut), &outputCursors); err != nil {
		t.Fatalf("failed to parse output cursor: %v", err)
	}
	if _, found := outputCursors["888000000000000001"]; found {
		t.Error("stale channel cursor from Connect() leaked into Sync() output (scope enforcement bypass)")
	}
}

// REG-014-R22-002: parseDiscordConfig must reject IEEE 754 Inf for backfill_limit.
func TestRegression_BackfillLimitRejectsInf(t *testing.T) {
	_, err := parseDiscordConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"backfill_limit": math.Inf(1),
		},
	})
	if err == nil {
		t.Error("expected error for +Inf backfill_limit")
	}
}

// REG-014-R22-002b: parseDiscordConfig must reject IEEE 754 -Inf for backfill_limit.
func TestRegression_BackfillLimitRejectsNegInf(t *testing.T) {
	_, err := parseDiscordConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"backfill_limit": math.Inf(-1),
		},
	})
	if err == nil {
		t.Error("expected error for -Inf backfill_limit")
	}
}

// REG-014-R22-002c: parseDiscordConfig must reject IEEE 754 NaN for backfill_limit.
func TestRegression_BackfillLimitRejectsNaN(t *testing.T) {
	_, err := parseDiscordConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"backfill_limit": math.NaN(),
		},
	})
	if err == nil {
		t.Error("expected error for NaN backfill_limit")
	}
}

// C7: Craft maximally adversarial cursor input.
func TestChaos_AdversarialCursorJSON(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": ts.URL,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
		},
	})

	adversarial := []string{
		`{}`,
		`{"":""}`,
		`{"\u0000":"123"}`,
		`{"200000000000000001":""}`,
		`{"200000000000000001":"0"}`,
		`not json at all`,
		`{"200000000000000001":"\u0000"}`,
		`{"200000000000000001":"18446744073709551615"}`, // max uint64
		`null`,
	}
	for _, cursor := range adversarial {
		_, _, err := c.Sync(context.Background(), cursor)
		// Must not panic; errors are acceptable
		_ = err
	}
}

// --- Scope 2 & 3: REST Client & Token Validation Tests ---

func TestConnect_TokenValidationSuccess(t *testing.T) {
	ts := newTestDiscordAPI(t)
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Error("expected healthy after valid token")
	}
}

func TestConnect_TokenValidationUnauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users/@me" {
			w.WriteHeader(401)
			w.Write([]byte(`{"message":"401: Unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	t.Cleanup(ts.Close)
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})
	if err == nil {
		t.Fatal("expected error for unauthorized token")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Error("expected error health after failed token validation")
	}
}

func TestFetchChannelMessages_Basic(t *testing.T) {
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/100000000000000001/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"id":"200000000000000001","content":"hello","author":{"id":"300000000000000001","username":"alice"},"channel_id":"100000000000000001","guild_id":"400000000000000001","timestamp":"2024-01-01T00:00:00Z"},
				{"id":"200000000000000002","content":"world","author":{"id":"300000000000000002","username":"bob"},"channel_id":"100000000000000001","guild_id":"400000000000000001","timestamp":"2024-01-01T00:01:00Z"}
			]`))
		})
	})
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})

	msgs, err := c.fetchChannelMessages(context.Background(), ts.URL, testBotToken, "100000000000000001", "", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("expected first message content 'hello', got %q", msgs[0].Content)
	}
	if msgs[1].Author.Username != "bob" {
		t.Errorf("expected second message author 'bob', got %q", msgs[1].Author.Username)
	}
}

func TestFetchChannelMessages_Pagination(t *testing.T) {
	callCount := 0
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/100000000000000001/messages", func(w http.ResponseWriter, r *http.Request) {
			callCount++
			after := r.URL.Query().Get("after")
			limitStr := r.URL.Query().Get("limit")
			w.Header().Set("Content-Type", "application/json")

			// Return a full page on first call (matching limit), then partial on second
			if after == "" {
				// First page: return exactly limit messages
				msgs := make([]apiMessage, 0)
				limit := 5 // default small limit for test
				if limitStr != "" {
					fmt.Sscanf(limitStr, "%d", &limit)
				}
				for i := 0; i < limit; i++ {
					msgs = append(msgs, apiMessage{
						ID:        fmt.Sprintf("%d", 100+i),
						Content:   fmt.Sprintf("msg%d", i),
						Author:    apiUser{ID: "1", Username: "a"},
						ChannelID: "100000000000000001",
						GuildID:   "g1",
						Timestamp: time.Now(),
					})
				}
				json.NewEncoder(w).Encode(msgs)
			} else {
				// Second page: return fewer than limit (end of results)
				w.Write([]byte(`[
					{"id":"999","content":"last","author":{"id":"1","username":"a"},"channel_id":"100000000000000001","guild_id":"g1","timestamp":"2024-01-01T00:03:00Z"}
				]`))
			}
		})
	})
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})

	// Request up to 200 — should paginate: first page returns 100 (full), second returns 1 (partial)
	msgs, err := c.fetchChannelMessages(context.Background(), ts.URL, testBotToken, "100000000000000001", "", 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 101 {
		t.Fatalf("expected 101 messages across pagination (100+1), got %d", len(msgs))
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 API calls for pagination, got %d", callCount)
	}
}

func TestFetchChannelMessages_RespectsBackfillLimit(t *testing.T) {
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/100000000000000001/messages", func(w http.ResponseWriter, r *http.Request) {
			// Return exactly limit messages (simulating a full page always available)
			limitStr := r.URL.Query().Get("limit")
			count := 100
			if limitStr != "" {
				fmt.Sscanf(limitStr, "%d", &count)
			}
			msgs := make([]apiMessage, count)
			for i := range msgs {
				msgs[i] = apiMessage{
					ID:        fmt.Sprintf("%d", 1000+i),
					Content:   fmt.Sprintf("msg%d", i),
					Author:    apiUser{ID: "1", Username: "a"},
					ChannelID: "100000000000000001",
					GuildID:   "g1",
					Timestamp: time.Now(),
				}
			}
			json.NewEncoder(w).Encode(msgs)
		})
	})
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})

	// Limit to 50 messages — should stop after first partial page request
	msgs, err := c.fetchChannelMessages(context.Background(), ts.URL, testBotToken, "100000000000000001", "", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) > 50 {
		t.Errorf("backfill limit not respected: got %d messages, max 50", len(msgs))
	}
}

func TestFetchPinnedMessages_Basic(t *testing.T) {
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/100000000000000001/pins", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"id":"500000000000000001","content":"important pinned","author":{"id":"1","username":"admin"},"channel_id":"100000000000000001","guild_id":"g1","timestamp":"2024-01-01T00:00:00Z","pinned":true}
			]`))
		})
	})
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})

	pins, err := c.fetchPinnedMessages(context.Background(), ts.URL, testBotToken, "100000000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pins) != 1 {
		t.Fatalf("expected 1 pinned message, got %d", len(pins))
	}
	if pins[0].Content != "important pinned" {
		t.Errorf("expected pinned content, got %q", pins[0].Content)
	}
}

func TestDoDiscordRequest_AuthHeader(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(ts.Close)

	c := New("discord")
	c.doDiscordRequest(context.Background(), http.MethodGet, ts.URL, "my-secret-token", "/test")

	if gotAuth != "Bot my-secret-token" {
		t.Errorf("expected 'Bot my-secret-token', got %q", gotAuth)
	}
}

func TestDoDiscordRequest_RateLimitHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "2")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(10*time.Second).Unix()))
		w.Write([]byte(`[]`))
	}))
	t.Cleanup(ts.Close)

	c := New("discord")
	_, err := c.doDiscordRequest(context.Background(), http.MethodGet, ts.URL, testBotToken, "/channels/123/messages")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Rate limiter should now have state for this route
	wait := c.limiter.ShouldWait("/channels/123/messages")
	// remaining=2 > 1, so should not wait
	if wait != 0 {
		t.Errorf("expected no wait (remaining=2), got %v", wait)
	}
}

func TestDoDiscordRequest_429Retry(t *testing.T) {
	attempt := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt <= 1 {
			w.Header().Set("Retry-After", "0.01")
			w.WriteHeader(429)
			w.Write([]byte(`{"message":"rate limited"}`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(ts.Close)

	c := New("discord")
	body, err := c.doDiscordRequest(context.Background(), http.MethodGet, ts.URL, testBotToken, "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("unexpected response: %s", body)
	}
	if attempt != 2 {
		t.Errorf("expected 2 attempts (1 retry), got %d", attempt)
	}
}

func TestSyncEndToEnd_WithMessagesAndPins(t *testing.T) {
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/200000000000000001/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"id":"300000000000000001","content":"hello world","author":{"id":"400000000000000001","username":"alice"},"channel_id":"200000000000000001","guild_id":"100000000000000001","timestamp":"2024-01-01T00:00:00Z"}
			]`))
		})
		mux.HandleFunc("/channels/200000000000000001/pins", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"id":"300000000000000002","content":"pinned msg","author":{"id":"400000000000000001","username":"alice"},"channel_id":"200000000000000001","guild_id":"100000000000000001","timestamp":"2024-01-01T00:00:00Z","pinned":true}
			]`))
		})
	})
	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":         ts.URL,
			"include_pins":    true,
			"include_threads": false,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts (1 message + 1 pin), got %d", len(artifacts))
	}

	// Verify message artifact
	if artifacts[0].SourceID != "discord" {
		t.Errorf("expected source_id 'discord', got %q", artifacts[0].SourceID)
	}
	if artifacts[0].RawContent != "hello world" {
		t.Errorf("expected content 'hello world', got %q", artifacts[0].RawContent)
	}

	// Verify cursor advanced (pins don't advance cursors, only message fetches do)
	var cursors map[string]string
	if err := json.Unmarshal([]byte(cursor), &cursors); err != nil {
		t.Fatalf("failed to parse cursor: %v", err)
	}
	if cursors["200000000000000001"] != "300000000000000001" {
		t.Errorf("expected cursor at 300000000000000001, got %q", cursors["200000000000000001"])
	}
}

func TestSyncEndToEnd_CursorPreventsRefetch(t *testing.T) {
	callCount := 0
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/200000000000000001/messages", func(w http.ResponseWriter, r *http.Request) {
			callCount++
			after := r.URL.Query().Get("after")
			w.Header().Set("Content-Type", "application/json")
			if after == "300000000000000001" {
				// Second sync: no new messages
				w.Write([]byte("[]"))
			} else {
				w.Write([]byte(`[
					{"id":"300000000000000001","content":"first","author":{"id":"1","username":"a"},"channel_id":"200000000000000001","guild_id":"100000000000000001","timestamp":"2024-01-01T00:00:00Z"}
				]`))
			}
		})
	})
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":         ts.URL,
			"include_pins":    false,
			"include_threads": false,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
		},
	})

	// First sync: get 1 message
	artifacts1, cursor1, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("first sync failed: %v", err)
	}
	if len(artifacts1) != 1 {
		t.Fatalf("expected 1 artifact on first sync, got %d", len(artifacts1))
	}

	// Second sync with cursor: no new messages
	artifacts2, _, err := c.Sync(context.Background(), cursor1)
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if len(artifacts2) != 0 {
		t.Errorf("expected 0 artifacts on second sync, got %d", len(artifacts2))
	}
}

func TestApiMessageToInternal_Reactions(t *testing.T) {
	customID := "emoji123"
	msg := apiMessage{
		ID:        "111",
		Content:   "test",
		ChannelID: "222",
		GuildID:   "333",
		Reactions: []apiReaction{
			{Count: 5, Emoji: apiEmoji{Name: "👍"}},
			{Count: 3, Emoji: apiEmoji{Name: "custom", ID: &customID}},
		},
	}
	dm := apiMessageToInternal(msg)
	if len(dm.Reactions) != 2 {
		t.Fatalf("expected 2 reactions, got %d", len(dm.Reactions))
	}
	if dm.Reactions[0].Emoji != "👍" {
		t.Errorf("expected standard emoji '👍', got %q", dm.Reactions[0].Emoji)
	}
	if dm.Reactions[1].Emoji != "custom:emoji123" {
		t.Errorf("expected custom emoji 'custom:emoji123', got %q", dm.Reactions[1].Emoji)
	}
}

func TestApiMessageToInternal_Mentions(t *testing.T) {
	msg := apiMessage{
		ID:       "111",
		Content:  "hey @alice",
		Mentions: []apiMention{{ID: "444"}, {ID: "555"}},
	}
	dm := apiMessageToInternal(msg)
	if len(dm.MentionIDs) != 2 || dm.MentionIDs[0] != "444" || dm.MentionIDs[1] != "555" {
		t.Errorf("expected mention IDs [444, 555], got %v", dm.MentionIDs)
	}
}

func TestApiMessageToInternal_ThreadAndReply(t *testing.T) {
	msg := apiMessage{
		ID:      "111",
		Content: "reply in thread",
		MessageReference: &apiMessageRef{
			MessageID: "100",
			ChannelID: "200",
			GuildID:   "300",
		},
		Thread: &apiThread{
			ID:   "999",
			Name: "Cool Thread",
		},
	}
	dm := apiMessageToInternal(msg)
	if dm.ThreadID != "999" || dm.ThreadName != "Cool Thread" {
		t.Errorf("expected thread metadata, got id=%q name=%q", dm.ThreadID, dm.ThreadName)
	}
	if dm.MessageReference == nil || dm.MessageReference.MessageID != "100" {
		t.Error("expected message reference preserved")
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   time.Duration
	}{
		{"empty", http.Header{}, 0},
		{"1.5 seconds", http.Header{"Retry-After": []string{"1.5"}}, 1500 * time.Millisecond},
		{"invalid", http.Header{"Retry-After": []string{"abc"}}, 0},
		{"negative", http.Header{"Retry-After": []string{"-1"}}, 0},
		{"zero", http.Header{"Retry-After": []string{"0"}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRetryAfter(tt.header)
			if got != tt.want {
				t.Errorf("parseRetryAfter = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseDiscordConfig_APIURLOverride(t *testing.T) {
	cfg, err := parseDiscordConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url": "http://localhost:9999",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIURL != "http://localhost:9999" {
		t.Errorf("expected overridden API URL, got %q", cfg.APIURL)
	}
}

// --- Scope 5: Thread Ingestion Tests ---

func TestFetchActiveThreads_ParsesResponse(t *testing.T) {
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/guilds/900000000000000000/threads/active", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"threads": [
					{"id":"500000000000000001","name":"Thread Alpha","parent_id":"111000000000000000","type":11},
					{"id":"500000000000000002","name":"Thread Beta","parent_id":"222000000000000000","type":11}
				]
			}`))
		})
		mux.HandleFunc("/channels/500000000000000001/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"id":"600000000000000001","content":"thread msg 1","author":{"id":"700000000000000001","username":"alice"},"channel_id":"500000000000000001","guild_id":"900000000000000000","timestamp":"2024-01-01T00:00:00Z"},
				{"id":"600000000000000002","content":"thread msg 2","author":{"id":"700000000000000001","username":"alice"},"channel_id":"500000000000000001","guild_id":"900000000000000000","timestamp":"2024-01-01T00:01:00Z"}
			]`))
		})
		mux.HandleFunc("/channels/500000000000000002/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"id":"600000000000000003","content":"beta msg","author":{"id":"700000000000000002","username":"bob"},"channel_id":"500000000000000002","guild_id":"900000000000000000","timestamp":"2024-01-01T00:02:00Z"}
			]`))
		})
	})

	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})

	monitored := map[string]struct{}{
		"111000000000000000": {},
		"222000000000000000": {},
	}
	cursors := make(ChannelCursors)
	msgs, err := c.fetchActiveThreads(context.Background(), ts.URL, testBotToken, "900000000000000000", monitored, cursors, 1000)
	if err != nil {
		t.Fatalf("fetchActiveThreads: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 thread messages, got %d", len(msgs))
	}
	// Verify thread metadata is set
	for _, msg := range msgs {
		if msg.ThreadID == "" {
			t.Error("expected ThreadID set on thread message")
		}
		if msg.ThreadName == "" {
			t.Error("expected ThreadName set on thread message")
		}
	}
	// Verify cursors were advanced
	if cursors["500000000000000001"] != "600000000000000002" {
		t.Errorf("expected cursor for thread 500000000000000001 = 600000000000000002, got %s", cursors["500000000000000001"])
	}
	if cursors["500000000000000002"] != "600000000000000003" {
		t.Errorf("expected cursor for thread 500000000000000002 = 600000000000000003, got %s", cursors["500000000000000002"])
	}
}

func TestFetchActiveThreads_FiltersToMonitoredChannels(t *testing.T) {
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/guilds/900000000000000000/threads/active", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"threads": [
					{"id":"500000000000000001","name":"Monitored Thread","parent_id":"111000000000000000","type":11},
					{"id":"500000000000000002","name":"Unmonitored Thread","parent_id":"999000000000000000","type":11}
				]
			}`))
		})
		mux.HandleFunc("/channels/500000000000000001/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":"600000000000000001","content":"monitored thread msg","author":{"id":"700000000000000001","username":"alice"},"channel_id":"500000000000000001","guild_id":"900000000000000000","timestamp":"2024-01-01T00:00:00Z"}]`))
		})
		// Should NOT be called since parent_id 999 is not monitored
		mux.HandleFunc("/channels/500000000000000002/messages", func(w http.ResponseWriter, r *http.Request) {
			t.Error("fetchChannelMessages called for unmonitored thread parent")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		})
	})

	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials:  map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{"api_url": ts.URL},
	})

	// Only monitor channel 111, not 999
	monitored := map[string]struct{}{"111000000000000000": {}}
	cursors := make(ChannelCursors)
	msgs, err := c.fetchActiveThreads(context.Background(), ts.URL, testBotToken, "900000000000000000", monitored, cursors, 1000)
	if err != nil {
		t.Fatalf("fetchActiveThreads: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message from monitored thread only, got %d", len(msgs))
	}
	if msgs[0].Content != "monitored thread msg" {
		t.Errorf("unexpected message content: %s", msgs[0].Content)
	}
}

func TestSync_IncludesThreadMessages(t *testing.T) {
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/111000000000000000/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":"200000000000000001","content":"channel msg","author":{"id":"300000000000000001","username":"alice"},"channel_id":"111000000000000000","guild_id":"900000000000000000","timestamp":"2024-01-01T00:00:00Z"}]`))
		})
		mux.HandleFunc("/guilds/900000000000000000/threads/active", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"threads": [{"id":"500000000000000001","name":"Test Thread","parent_id":"111000000000000000","type":11}]
			}`))
		})
		mux.HandleFunc("/channels/500000000000000001/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":"600000000000000001","content":"thread msg","author":{"id":"300000000000000001","username":"alice"},"channel_id":"500000000000000001","guild_id":"900000000000000000","timestamp":"2024-01-01T00:01:00Z"}]`))
		})
		mux.HandleFunc("/channels/111000000000000000/threads/archived/public", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"threads":[],"has_more":false}`))
		})
	})

	c := New("discord")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":         ts.URL,
			"enable_gateway":  false,
			"include_threads": true,
			"include_pins":    false,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "900000000000000000",
					"channel_ids": []interface{}{"111000000000000000"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(artifacts) < 2 {
		t.Fatalf("expected at least 2 artifacts (channel + thread), got %d", len(artifacts))
	}

	foundThread := false
	for _, a := range artifacts {
		if a.SourceRef == "600000000000000001" {
			foundThread = true
			if a.Metadata["thread_id"] != "500000000000000001" {
				t.Errorf("expected thread_id 500000000000000001, got %v", a.Metadata["thread_id"])
			}
			if a.Metadata["thread_name"] != "Test Thread" {
				t.Errorf("expected thread_name 'Test Thread', got %v", a.Metadata["thread_name"])
			}
		}
	}
	if !foundThread {
		t.Error("thread message not found in sync artifacts")
	}
}

func TestSync_ThreadMetadataOnArtifacts(t *testing.T) {
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/111000000000000000/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		})
		mux.HandleFunc("/guilds/900000000000000000/threads/active", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"threads": [{"id":"500000000000000001","name":"Design Thread","parent_id":"111000000000000000","type":11}]
			}`))
		})
		mux.HandleFunc("/channels/500000000000000001/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":"600000000000000001","content":"design discussion","author":{"id":"300000000000000001","username":"carol"},"channel_id":"500000000000000001","guild_id":"900000000000000000","timestamp":"2024-01-01T00:00:00Z"}]`))
		})
		mux.HandleFunc("/channels/111000000000000000/threads/archived/public", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"threads":[],"has_more":false}`))
		})
	})

	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":         ts.URL,
			"enable_gateway":  false,
			"include_threads": true,
			"include_pins":    false,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "900000000000000000",
					"channel_ids": []interface{}{"111000000000000000"},
				},
			},
		},
	})

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Verify thread message metadata
	found := false
	for _, a := range artifacts {
		if a.SourceRef == "600000000000000001" {
			found = true
			if a.Metadata["thread_id"] != "500000000000000001" {
				t.Errorf("expected thread_id, got %v", a.Metadata["thread_id"])
			}
			if a.Metadata["thread_name"] != "Design Thread" {
				t.Errorf("expected thread_name 'Design Thread', got %v", a.Metadata["thread_name"])
			}
			// Thread messages should get discord/thread content type
			if a.ContentType != "discord/thread" {
				t.Errorf("expected content type discord/thread, got %s", a.ContentType)
			}
		}
	}
	if !found {
		t.Error("thread message artifact not found")
	}

	// Verify thread cursor persisted
	var cursorMap map[string]string
	if err := json.Unmarshal([]byte(cursor), &cursorMap); err != nil {
		t.Fatalf("failed to parse cursor: %v", err)
	}
	if cursorMap["500000000000000001"] != "600000000000000001" {
		t.Errorf("expected thread cursor persisted, got %v", cursorMap["500000000000000001"])
	}
}

func TestSync_IncludeThreadsFalse_SkipsThreads(t *testing.T) {
	threadFetched := false
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/111000000000000000/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":"200000000000000001","content":"channel msg","author":{"id":"300000000000000001","username":"alice"},"channel_id":"111000000000000000","guild_id":"900000000000000000","timestamp":"2024-01-01T00:00:00Z"}]`))
		})
		mux.HandleFunc("/guilds/900000000000000000/threads/active", func(w http.ResponseWriter, r *http.Request) {
			threadFetched = true
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"threads":[]}`))
		})
	})

	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":         ts.URL,
			"enable_gateway":  false,
			"include_threads": false,
			"include_pins":    false,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "900000000000000000",
					"channel_ids": []interface{}{"111000000000000000"},
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if threadFetched {
		t.Error("threads should not be fetched when include_threads is false")
	}
	if len(artifacts) != 1 {
		t.Errorf("expected 1 artifact (channel only), got %d", len(artifacts))
	}
}

// --- Scope 6: Bot Command Capture Tests ---

func TestAssignTier_CaptureContentType_ForceFull(t *testing.T) {
	// A short message with no links, no pins, no reactions that would normally get "metadata"
	msg := DiscordMessage{Content: "!save https://example.com"}
	// With "capture" as the tier hint, assignTier should return "full"
	tier := assignTier(msg, "capture")
	if tier != "full" {
		t.Errorf("expected 'full' tier for capture content type, got %q", tier)
	}

	// Even without a link in the message (e.g. "!save some text")
	msg2 := DiscordMessage{Content: "!save no-link"}
	tier2 := assignTier(msg2, "capture")
	if tier2 != "full" {
		t.Errorf("expected 'full' tier for capture without link, got %q", tier2)
	}
}

func TestNormalize_BotCommand_SetsCaptureType(t *testing.T) {
	msg := DiscordMessage{
		ID:        "111111111111111111",
		Content:   "!save https://example.com/article Great read",
		Author:    Author{ID: "222222222222222222", Username: "alice"},
		ChannelID: "333333333333333333",
		GuildID:   "444444444444444444",
		Timestamp: time.Now(),
	}
	artifact := normalizeMessage(msg, "light", []string{"!save", "!capture"})
	if artifact.ContentType != "discord/capture" {
		t.Errorf("expected discord/capture, got %s", artifact.ContentType)
	}
	// Must be "full" tier due to capture escalation
	if artifact.Metadata["processing_tier"] != "full" {
		t.Errorf("expected processing_tier 'full' for capture, got %v", artifact.Metadata["processing_tier"])
	}
	// Must have capture_url metadata
	if artifact.Metadata["capture_url"] != "https://example.com/article" {
		t.Errorf("expected capture_url, got %v", artifact.Metadata["capture_url"])
	}
	if artifact.Metadata["capture_comment"] != "Great read" {
		t.Errorf("expected capture_comment, got %v", artifact.Metadata["capture_comment"])
	}
}

func TestSync_CaptureCommand_ProducesFullTierArtifact(t *testing.T) {
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/111000000000000000/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"id":"200000000000000001","content":"!save https://example.com/article Check this out","author":{"id":"300000000000000001","username":"alice"},"channel_id":"111000000000000000","guild_id":"900000000000000000","timestamp":"2024-01-01T00:00:00Z"},
				{"id":"200000000000000002","content":"ok","author":{"id":"300000000000000002","username":"bob"},"channel_id":"111000000000000000","guild_id":"900000000000000000","timestamp":"2024-01-01T00:01:00Z"}
			]`))
		})
	})

	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":          ts.URL,
			"enable_gateway":   false,
			"include_threads":  false,
			"include_pins":     false,
			"capture_commands": []interface{}{"!save", "!capture"},
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "900000000000000000",
					"channel_ids": []interface{}{"111000000000000000"},
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}

	// First message is capture command
	captureArtifact := artifacts[0]
	if captureArtifact.ContentType != "discord/capture" {
		t.Errorf("expected discord/capture for capture command, got %s", captureArtifact.ContentType)
	}
	if captureArtifact.Metadata["processing_tier"] != "full" {
		t.Errorf("expected 'full' tier for capture artifact, got %v", captureArtifact.Metadata["processing_tier"])
	}
	if captureArtifact.Metadata["capture_url"] != "https://example.com/article" {
		t.Errorf("expected capture_url, got %v", captureArtifact.Metadata["capture_url"])
	}

	// Second message is a short message, should NOT be "full"
	normalArtifact := artifacts[1]
	if normalArtifact.Metadata["processing_tier"] == "full" {
		t.Error("short normal message should not get 'full' tier")
	}
}

// --- IMP-014-IE Improvement Tests ---

// IMP-014-IE-001: HTTP client uses bounded connection pool.
// Adversarial: without the transport limit, >maxConnsPerHost concurrent requests
// would open unlimited connections to Discord's API during high-backfill sync.
func TestNew_HTTPClientHasBoundedTransport(t *testing.T) {
	c := New("discord")
	transport, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected httpClient to use *http.Transport")
	}
	if transport.MaxConnsPerHost != maxConnsPerHost {
		t.Errorf("expected MaxConnsPerHost=%d, got %d", maxConnsPerHost, transport.MaxConnsPerHost)
	}
	if transport.MaxIdleConnsPerHost != maxConnsPerHost {
		t.Errorf("expected MaxIdleConnsPerHost=%d, got %d", maxConnsPerHost, transport.MaxIdleConnsPerHost)
	}
}

// IMP-014-IE-002: Discord API error responses include diagnostic body excerpt.
// Adversarial: without the body excerpt, a 403 "Missing Access" error is
// indistinguishable from a 403 "Missing Permissions" error, making channel
// permission debugging impossible from logs alone.
func TestDoDiscordRequest_ErrorIncludesBodyExcerpt(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantSubstr string
	}{
		{
			name:       "403 with Discord error code",
			status:     403,
			body:       `{"message":"Missing Access","code":50001}`,
			wantSubstr: "Missing Access",
		},
		{
			name:       "401 with details",
			status:     401,
			body:       `{"message":"401: Unauthorized"}`,
			wantSubstr: "Unauthorized",
		},
		{
			name:       "500 server error",
			status:     500,
			body:       `{"message":"Internal Server Error"}`,
			wantSubstr: "Internal Server Error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			t.Cleanup(ts.Close)

			c := New("discord")
			_, err := c.doDiscordRequest(context.Background(), http.MethodGet, ts.URL, testBotToken, "/test")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("error message %q should contain diagnostic body excerpt %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

// IMP-014-IE-002b: Error body excerpt is sanitized and truncated.
// Adversarial: a malicious Discord API response could inject control characters
// or arbitrarily large payloads into error messages, causing log injection or OOM.
func TestDoDiscordRequest_ErrorBodySanitizedAndTruncated(t *testing.T) {
	longBody := strings.Repeat("A", 1000)
	controlBody := "error\x00with\x07control\x1bchars"

	tests := []struct {
		name        string
		body        string
		checkLength bool
		checkSanity bool
	}{
		{"long body truncated", longBody, true, false},
		{"control chars sanitized", controlBody, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(500)
				w.Write([]byte(tt.body))
			}))
			t.Cleanup(ts.Close)

			c := New("discord")
			_, err := c.doDiscordRequest(context.Background(), http.MethodGet, ts.URL, testBotToken, "/test")
			if err == nil {
				t.Fatal("expected error")
			}
			errMsg := err.Error()
			if tt.checkLength && len(errMsg) > maxErrorBodyExcerpt+100 {
				t.Errorf("error message too long (%d bytes), body excerpt should be truncated", len(errMsg))
			}
			if tt.checkSanity {
				for _, r := range errMsg {
					if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
						t.Errorf("error message contains unsanitized control char: %U", r)
						break
					}
				}
			}
		})
	}
}

// IMP-014-IE-003: snowflakeGreater handles variable-length numeric strings.
// Adversarial: raw string comparison "99" > "100" is true (lexicographic),
// but numerically 99 < 100. Without length-first comparison, cursor advancement
// would regress, causing message re-ingestion.
func TestSnowflakeGreater(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		// Same length — lexicographic is correct
		{"200000000000000001", "100000000000000001", true},
		{"100000000000000001", "200000000000000001", false},
		{"100000000000000001", "100000000000000001", false},
		// Different length — longer is bigger
		{"100000000000000001", "99999999999999999", true},  // 18 digits > 17 digits
		{"99999999999999999", "100000000000000001", false}, // 17 digits < 18 digits
		// Edge cases
		{"1", "2", false},
		{"2", "1", true},
		{"10", "9", true},  // length-first: "10" (2 chars) > "9" (1 char)
		{"9", "10", false}, // length-first: "9" (1 char) < "10" (2 chars)
		// Equal
		{"0", "0", false},
		{"18446744073709551615", "18446744073709551615", false},
	}
	for _, tt := range tests {
		got := snowflakeGreater(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("snowflakeGreater(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// IMP-014-IE-003b: Cursor advancement uses snowflakeGreater and handles mixed-length IDs.
// Adversarial: if a hypothetical short snowflake "99" were compared as string
// against "100", the cursor would regress to "99" and re-fetch messages.
func TestCursorAdvancement_MixedLengthSnowflakes(t *testing.T) {
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/200000000000000001/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Return messages with different-length IDs
			w.Write([]byte(`[
				{"id":"100000000000000001","content":"long id","author":{"id":"1","username":"a"},"channel_id":"200000000000000001","guild_id":"100000000000000001","timestamp":"2024-01-01T00:00:00Z"},
				{"id":"9999999999999999","content":"shorter id","author":{"id":"1","username":"a"},"channel_id":"200000000000000001","guild_id":"100000000000000001","timestamp":"2024-01-01T00:01:00Z"}
			]`))
		})
	})
	c := New("discord")
	c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":         ts.URL,
			"include_pins":    false,
			"include_threads": false,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "100000000000000001",
					"channel_ids": []interface{}{"200000000000000001"},
				},
			},
		},
	})

	_, cursorOut, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	var cursors map[string]string
	if err := json.Unmarshal([]byte(cursorOut), &cursors); err != nil {
		t.Fatalf("parse cursor: %v", err)
	}

	// The longer (numerically larger) ID must win as cursor
	if cursors["200000000000000001"] != "100000000000000001" {
		t.Errorf("cursor should be the numerically larger ID '100000000000000001', got %q (snowflakeGreater handles mixed-length correctly)", cursors["200000000000000001"])
	}
}
