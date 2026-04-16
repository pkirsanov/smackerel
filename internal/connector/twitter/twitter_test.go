package twitter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestNew(t *testing.T) {
	c := New("twitter")
	if c.ID() != "twitter" {
		t.Errorf("expected twitter, got %s", c.ID())
	}
}

func TestConnect_MissingArchiveDir(t *testing.T) {
	c := New("twitter")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": ""},
	})
	if err == nil {
		t.Error("expected error for missing archive_dir")
	}
}

func TestConnect_NonexistentArchiveDir(t *testing.T) {
	c := New("twitter")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": "/nonexistent/path"},
	})
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestParseTweetsJS(t *testing.T) {
	data := []byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"100","full_text":"Hello world","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":5,"retweet_count":2,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}},{"tweet":{"id":"101","full_text":"Second tweet","created_at":"Wed Mar 15 15:00:00 +0000 2026","favorite_count":10,"retweet_count":0,"entities":{"urls":[],"hashtags":[{"text":"test"}],"user_mentions":[]}}}]`)

	tweets, err := parseTweetsJS(data)
	if err != nil {
		t.Fatalf("parseTweetsJS failed: %v", err)
	}
	if len(tweets) != 2 {
		t.Fatalf("expected 2 tweets, got %d", len(tweets))
	}
	if tweets[0].ID != "100" {
		t.Errorf("expected ID 100, got %s", tweets[0].ID)
	}
	if tweets[0].FavoriteCount != 5 {
		t.Errorf("expected 5 favorites, got %d", tweets[0].FavoriteCount)
	}
	if len(tweets[1].Entities.Hashtags) != 1 {
		t.Errorf("expected 1 hashtag, got %d", len(tweets[1].Entities.Hashtags))
	}
}

func TestParseTweetsJS_InvalidJSON(t *testing.T) {
	_, err := parseTweetsJS([]byte("not json at all"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestBuildThreads(t *testing.T) {
	tweets := []ArchiveTweet{
		{ID: "100", FullText: "Thread start", InReplyToStatusID: ""},
		{ID: "101", FullText: "Reply 1", InReplyToStatusID: "100"},
		{ID: "102", FullText: "Reply 2", InReplyToStatusID: "101"},
		{ID: "200", FullText: "Standalone", InReplyToStatusID: ""},
	}

	threads := buildThreads(tweets)
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if threads[0].RootID != "100" {
		t.Errorf("expected root ID 100, got %s", threads[0].RootID)
	}
	if len(threads[0].Tweets) != 3 {
		t.Errorf("expected 3 tweets in thread, got %d", len(threads[0].Tweets))
	}
}

func TestClassifyTweet(t *testing.T) {
	tests := []struct {
		name     string
		tweet    ArchiveTweet
		thread   *Thread
		expected string
	}{
		{"text", ArchiveTweet{FullText: "Hello"}, nil, "tweet/text"},
		{"retweet", ArchiveTweet{FullText: "RT @user: text"}, nil, "tweet/retweet"},
		{"link", ArchiveTweet{Entities: TweetEntities{URLs: []TweetURL{{ExpandedURL: "https://x.com"}}}}, nil, "tweet/link"},
		{"thread", ArchiveTweet{}, &Thread{RootID: "1"}, "tweet/thread"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyTweet(tt.tweet, tt.thread)
			if got != tt.expected {
				t.Errorf("classifyTweet() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestAssignTweetTier(t *testing.T) {
	tests := []struct {
		name       string
		tweet      ArchiveTweet
		bookmarked bool
		liked      bool
		thread     *Thread
		expected   string
	}{
		{"bookmarked", ArchiveTweet{}, true, false, nil, "full"},
		{"liked", ArchiveTweet{}, false, true, nil, "full"},
		{"thread", ArchiveTweet{}, false, false, &Thread{}, "full"},
		{"with url", ArchiveTweet{Entities: TweetEntities{URLs: []TweetURL{{ExpandedURL: "https://x.com"}}}}, false, false, nil, "full"},
		{"high engagement", ArchiveTweet{FavoriteCount: 200}, false, false, nil, "standard"},
		{"retweet", ArchiveTweet{FullText: "RT @user: text"}, false, false, nil, "light"},
		{"short", ArchiveTweet{FullText: "ok"}, false, false, nil, "metadata"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assignTweetTier(tt.tweet, tt.bookmarked, tt.liked, tt.thread)
			if got != tt.expected {
				t.Errorf("assignTweetTier() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestNormalizeTweet(t *testing.T) {
	tweet := ArchiveTweet{
		ID:       "123",
		FullText: "Great article about Go: https://example.com",
		Entities: TweetEntities{
			URLs:     []TweetURL{{ExpandedURL: "https://example.com"}},
			Hashtags: []TweetHashtag{{Text: "golang"}},
		},
	}

	artifact := normalizeTweet(tweet, true, false, nil)
	if artifact.SourceID != "twitter" {
		t.Errorf("expected twitter, got %s", artifact.SourceID)
	}
	if artifact.ContentType != "tweet/link" {
		t.Errorf("expected tweet/link, got %s", artifact.ContentType)
	}
	if artifact.Metadata["is_bookmarked"] != true {
		t.Error("expected bookmarked=true")
	}
}

func TestParseTweetTime(t *testing.T) {
	ts, err := parseTweetTime("Wed Mar 15 14:30:00 +0000 2026")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts.Year() != 2026 || ts.Month() != 3 || ts.Day() != 15 {
		t.Errorf("unexpected time: %v", ts)
	}
}

func TestParseTweetTime_MalformedReturnsError(t *testing.T) {
	_, err := parseTweetTime("not a date")
	if err == nil {
		t.Error("expected error for malformed timestamp")
	}
}

func TestParseTweetTime_EmptyReturnsError(t *testing.T) {
	_, err := parseTweetTime("")
	if err == nil {
		t.Error("expected error for empty timestamp")
	}
}

func TestClose(t *testing.T) {
	c := New("twitter")
	c.health = connector.HealthHealthy
	c.Close()
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("should be disconnected after close")
	}
}

// --- Chaos hardening tests ---

func TestClose_ConcurrentWithHealth(t *testing.T) {
	c := New("twitter")
	c.mu.Lock()
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Health(context.Background())
		}()
	}
	// Close concurrently with health reads — previously a data race
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Close()
	}()
	wg.Wait()
}

func TestConnect_InvalidSyncMode(t *testing.T) {
	c := New("twitter")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "garbage"},
	})
	if err == nil {
		t.Error("expected error for invalid sync_mode")
	}
}

func TestConnect_ConcurrentWithHealth(t *testing.T) {
	c := New("twitter")
	dir := t.TempDir()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Health(context.Background())
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Connect(context.Background(), connector.ConnectorConfig{
			SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
		})
	}()
	wg.Wait()
}

func TestBuildThreads_BranchingReplies(t *testing.T) {
	// Two replies to the same parent (branching conversation)
	tweets := []ArchiveTweet{
		{ID: "100", FullText: "Thread root", InReplyToStatusID: ""},
		{ID: "101", FullText: "Branch A reply", InReplyToStatusID: "100"},
		{ID: "102", FullText: "Branch B reply", InReplyToStatusID: "100"},
	}

	threads := buildThreads(tweets)
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if len(threads[0].Tweets) != 3 {
		t.Errorf("expected 3 tweets in branching thread, got %d (data loss on branch)", len(threads[0].Tweets))
	}
}

func TestBuildThreads_EmptyInput(t *testing.T) {
	threads := buildThreads(nil)
	if len(threads) != 0 {
		t.Errorf("expected 0 threads for nil input, got %d", len(threads))
	}
	threads = buildThreads([]ArchiveTweet{})
	if len(threads) != 0 {
		t.Errorf("expected 0 threads for empty input, got %d", len(threads))
	}
}

func TestBuildThreads_AllStandalone(t *testing.T) {
	tweets := []ArchiveTweet{
		{ID: "1", FullText: "Hello"},
		{ID: "2", FullText: "World"},
	}
	threads := buildThreads(tweets)
	if len(threads) != 0 {
		t.Errorf("expected 0 threads for all standalone tweets, got %d", len(threads))
	}
}

func TestSyncArchive_CancelledContext(t *testing.T) {
	c := New("twitter")
	dir := t.TempDir()
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, _, err := c.syncArchive(ctx, "")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestNormalizeTweet_EmptyFullText(t *testing.T) {
	tweet := ArchiveTweet{
		ID:       "999",
		FullText: "",
	}
	artifact := normalizeTweet(tweet, false, false, nil)
	if artifact.Title != "" {
		t.Errorf("expected empty title for empty tweet, got %q", artifact.Title)
	}
	if artifact.CapturedAt.After(time.Now()) {
		t.Error("captured_at should not be in the future for a zero-time fallback")
	}
}

func TestParseTweetsJS_EmptyArray(t *testing.T) {
	data := []byte("window.YTD.tweet.part0 = []")
	tweets, err := parseTweetsJS(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tweets) != 0 {
		t.Errorf("expected 0 tweets for empty array, got %d", len(tweets))
	}
}

func TestParseTweetsJS_NoArrayBracket(t *testing.T) {
	data := []byte("window.YTD.tweet.part0 = {}")
	_, err := parseTweetsJS(data)
	if err == nil {
		t.Error("expected error when no JSON array found")
	}
}

// --- Security hardening tests ---

func TestConnect_ArchiveDirSymlinkResolution(t *testing.T) {
	// Create a real directory and a symlink to it
	realDir := t.TempDir()
	symlinkDir := filepath.Join(t.TempDir(), "symlink")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Skipf("cannot create symlinks on this OS: %v", err)
	}

	c := New("twitter")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"sync_mode":   "archive",
			"archive_dir": symlinkDir,
		},
	})
	if err != nil {
		t.Fatalf("expected connect to succeed with symlink dir: %v", err)
	}

	// After connect, the config should hold the resolved (real) path
	c.mu.RLock()
	resolved := c.config.ArchiveDir
	c.mu.RUnlock()
	if resolved != realDir {
		t.Errorf("archive_dir should be resolved to %s, got %s", realDir, resolved)
	}
}

func TestConnect_ArchiveDirNotADirectory(t *testing.T) {
	// Point archive_dir at a regular file, not a directory
	tmpFile := filepath.Join(t.TempDir(), "not_a_dir")
	if err := os.WriteFile(tmpFile, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := New("twitter")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"sync_mode":   "archive",
			"archive_dir": tmpFile,
		},
	})
	if err == nil {
		t.Error("expected error when archive_dir is a file, not a directory")
	}
}

func TestSyncArchive_FileSizeLimit(t *testing.T) {
	// Verify that the file size limit constant is enforced.
	// We cannot easily create a 500MB file in a unit test, so verify
	// the constant is set to a sane value.
	if maxArchiveFileSize != 500*1024*1024 {
		t.Errorf("maxArchiveFileSize should be 500 MiB, got %d", maxArchiveFileSize)
	}
}

func TestSyncArchive_SymlinkTraversal(t *testing.T) {
	// Create archive dir structure where data/tweets.js is a symlink
	// pointing outside the archive directory.
	archiveDir := t.TempDir()
	dataDir := filepath.Join(archiveDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a file outside the archive directory
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret data"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Create a symlink data/tweets.js → outside file
	symlinkPath := filepath.Join(dataDir, "tweets.js")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skipf("cannot create symlinks on this OS: %v", err)
	}

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: archiveDir}

	_, _, err := c.syncArchive(context.Background(), "")
	if err == nil {
		t.Error("expected error when tweets.js is a symlink escaping archive directory")
	}
	if err != nil && !strings.Contains(err.Error(), "escapes archive directory") {
		t.Errorf("expected 'escapes archive directory' error, got: %v", err)
	}
}

func TestNormalizeTweet_InvalidIDNoURL(t *testing.T) {
	// A tweet ID with non-digit characters should not produce a URL
	tweet := ArchiveTweet{
		ID:       "abc/../../../etc/passwd",
		FullText: "Malicious tweet",
	}
	artifact := normalizeTweet(tweet, false, false, nil)
	if artifact.URL != "" {
		t.Errorf("expected empty URL for non-numeric tweet ID, got %q", artifact.URL)
	}
}

func TestNormalizeTweet_ValidIDProducesURL(t *testing.T) {
	tweet := ArchiveTweet{
		ID:       "1234567890",
		FullText: "Normal tweet",
	}
	artifact := normalizeTweet(tweet, false, false, nil)
	expected := "https://x.com/i/status/1234567890"
	if artifact.URL != expected {
		t.Errorf("expected URL %q, got %q", expected, artifact.URL)
	}
}

func TestTwitterConfig_StringRedactsToken(t *testing.T) {
	cfg := TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: "super-secret-token-123",
		APIEnabled:  true,
	}
	s := cfg.String()
	if strings.Contains(s, "super-secret-token-123") {
		t.Error("String() must not contain the bearer token")
	}
	if !strings.Contains(s, "<redacted>") {
		t.Error("String() should show <redacted> for set token")
	}
}

func TestTwitterConfig_StringNoToken(t *testing.T) {
	cfg := TwitterConfig{
		SyncMode: SyncModeArchive,
	}
	s := cfg.String()
	if !strings.Contains(s, "<not set>") {
		t.Error("String() should show <not set> for empty token")
	}
}

func TestParseTwitterConfig_CleansDirPath(t *testing.T) {
	cfg, err := parseTwitterConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"sync_mode":   "archive",
			"archive_dir": "/some/path/../other/./dir",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Clean("/some/path/../other/./dir")
	if cfg.ArchiveDir != expected {
		t.Errorf("expected cleaned path %q, got %q", expected, cfg.ArchiveDir)
	}
}

// --- Chaos hardening: Sync lifecycle race conditions ---

func TestSync_OnDisconnectedConnector(t *testing.T) {
	// Syncing a connector that was never connected (or was closed) must fail
	// rather than silently proceeding with zero config.
	c := New("twitter")
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error when syncing a disconnected connector")
	}
	if !strings.Contains(err.Error(), "disconnected") {
		t.Errorf("expected 'disconnected' in error, got: %v", err)
	}
}

func TestSync_AfterClose(t *testing.T) {
	// Connect, close, then sync — must reject.
	c := New("twitter")
	dir := t.TempDir()
	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}
	c.Close()

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error when syncing after Close()")
	}
}

func TestSync_CloseDoesNotRestoreHealthy(t *testing.T) {
	// Previously, Sync's defer unconditionally set HealthHealthy,
	// overwriting HealthDisconnected from a concurrent Close().
	// After fix: if Close() runs during sync, health stays Disconnected.
	c := New("twitter")
	dir := t.TempDir()

	// Create a valid archive so sync takes a moment
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"1","full_text":"hello","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	// Sync completes first, then close
	c.Sync(context.Background(), "")
	c.Close()

	health := c.Health(context.Background())
	if health != connector.HealthDisconnected {
		t.Errorf("expected disconnected after close, got %s", health)
	}
}

func TestSync_ConcurrentDoubleSync(t *testing.T) {
	// Two concurrent Sync() calls — one must succeed, one must get
	// "sync already in progress" error.
	c := New("twitter")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"1","full_text":"hello","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	// Force syncing=true to simulate concurrent sync
	c.mu.Lock()
	c.syncing = true
	c.mu.Unlock()

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error for concurrent sync attempt")
	}
	if !strings.Contains(err.Error(), "already in progress") {
		t.Errorf("expected 'already in progress' in error, got: %v", err)
	}

	// Release the guard
	c.mu.Lock()
	c.syncing = false
	c.mu.Unlock()
}

func TestSync_HealthDegradedAfterFailure(t *testing.T) {
	// When syncArchive fails, health should be Degraded, not Healthy.
	c := New("twitter")
	dir := t.TempDir()
	// No data/ subdir — syncArchive will fail trying to find tweets.js

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	c.Sync(context.Background(), "")
	health := c.Health(context.Background())
	if health != connector.HealthDegraded {
		t.Errorf("expected degraded health after sync failure, got %s", health)
	}
}

// --- Stability regression: error propagation and cursor correctness ---

func TestSync_ReturnsErrorOnArchiveFailure(t *testing.T) {
	// Sync must return a non-nil error when archive sync fails completely.
	// Previously Sync swallowed the error and returned nil, making failures
	// invisible to the supervisor and preventing retries.
	c := New("twitter")
	dir := t.TempDir()
	// No data/ subdir — syncArchive will fail

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected non-nil error when archive sync fails, got nil (error swallowed)")
	}
	if err != nil && !strings.Contains(err.Error(), "archive sync") {
		t.Errorf("expected 'archive sync' in error, got: %v", err)
	}
}

func TestSync_PreservesCursorOnFailure(t *testing.T) {
	// When sync fails, the original cursor must be returned unchanged so that
	// the next retry reprocesses from the same position. No data loss.
	c := New("twitter")
	dir := t.TempDir()

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	originalCursor := "2026-03-15T10:00:00Z"
	_, returnedCursor, _ := c.Sync(context.Background(), originalCursor)
	if returnedCursor != originalCursor {
		t.Errorf("cursor must not advance on failure: expected %q, got %q", originalCursor, returnedCursor)
	}
}

func TestSync_RecoveryAfterFailure(t *testing.T) {
	// After a failed sync (health=Degraded), a subsequent successful sync
	// must restore health to Healthy and return artifacts.
	c := New("twitter")
	dir := t.TempDir()

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	// First sync fails — no data/ directory
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected first sync to fail")
	}
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Fatal("expected degraded health after failed sync")
	}

	// Create the archive so next sync succeeds
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"1","full_text":"recovery tweet","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("expected recovery sync to succeed, got: %v", err)
	}
	if len(artifacts) != 1 {
		t.Errorf("expected 1 artifact from recovery sync, got %d", len(artifacts))
	}
	if cursor == "" {
		t.Error("expected non-empty cursor after successful recovery sync")
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy after recovery, got %s", c.Health(context.Background()))
	}
}

func TestSync_CursorNotAdvancedOnContextCancel(t *testing.T) {
	// When context is cancelled during sync, cursor must stay at its original
	// position to prevent skipping unprocessed tweets on the next attempt.
	c := New("twitter")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"1","full_text":"hello","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before sync starts

	originalCursor := "2026-01-01T00:00:00Z"
	_, returnedCursor, err := c.Sync(ctx, originalCursor)
	if err == nil {
		// Context was cancelled — sync should fail
		t.Error("expected error for cancelled context sync")
	}
	if returnedCursor != originalCursor {
		t.Errorf("cursor must not advance on context cancel: expected %q, got %q", originalCursor, returnedCursor)
	}
}

func TestSync_ReturnsNilArtifactsOnFailure(t *testing.T) {
	// When sync fails, returned artifacts must be nil (not an empty slice)
	// so callers can distinguish "nothing new" from "failure".
	c := New("twitter")
	dir := t.TempDir()

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if artifacts != nil {
		t.Errorf("expected nil artifacts on failure, got %d items", len(artifacts))
	}
}

func TestSync_HealthRestoredAfterSuccess(t *testing.T) {
	// Successful sync should restore Healthy.
	c := New("twitter")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"1","full_text":"hello","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	c.Sync(context.Background(), "")
	health := c.Health(context.Background())
	if health != connector.HealthHealthy {
		t.Errorf("expected healthy after successful sync, got %s", health)
	}
}

func TestSync_ConcurrentSyncAndClose(t *testing.T) {
	// Stress test: many goroutines calling Sync and Close concurrently.
	// Must not panic, deadlock, or produce data races.
	c := New("twitter")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = []`), 0o600)

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			c.Sync(context.Background(), "")
		}()
		go func() {
			defer wg.Done()
			c.Close()
		}()
	}
	// Also read health concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Health(context.Background())
		}()
	}
	wg.Wait()
}

// --- Security hardening: Round R15 findings ---

func TestIsSafeURL_AllowsHTTPS(t *testing.T) {
	if !isSafeURL("https://example.com/article") {
		t.Error("https URL should be safe")
	}
}

func TestIsSafeURL_AllowsHTTP(t *testing.T) {
	if !isSafeURL("http://example.com/article") {
		t.Error("http URL should be safe")
	}
}

func TestIsSafeURL_RejectsJavascript(t *testing.T) {
	if isSafeURL("javascript:alert(1)") {
		t.Error("javascript: URL must be rejected (CWE-79)")
	}
}

func TestIsSafeURL_RejectsData(t *testing.T) {
	if isSafeURL("data:text/html,<script>alert(1)</script>") {
		t.Error("data: URL must be rejected (CWE-79)")
	}
}

func TestIsSafeURL_RejectsVBScript(t *testing.T) {
	if isSafeURL("vbscript:MsgBox(1)") {
		t.Error("vbscript: URL must be rejected")
	}
}

func TestIsSafeURL_RejectsEmpty(t *testing.T) {
	if isSafeURL("") {
		t.Error("empty URL should not be considered safe")
	}
}

func TestIsSafeURL_RejectsRelativePath(t *testing.T) {
	// Relative paths have no scheme and should be rejected.
	if isSafeURL("../../etc/passwd") {
		t.Error("relative path should not be considered safe")
	}
}

func TestNormalizeTweet_FiltersUnsafeURLs(t *testing.T) {
	// A tweet with a mix of safe and unsafe URLs should only include safe ones.
	tweet := ArchiveTweet{
		ID:       "555",
		FullText: "Check this out!",
		Entities: TweetEntities{
			URLs: []TweetURL{
				{ExpandedURL: "https://example.com/safe"},
				{ExpandedURL: "javascript:alert('xss')"},
				{ExpandedURL: "https://another.com/also-safe"},
				{ExpandedURL: "data:text/html,evil"},
			},
		},
	}

	artifact := normalizeTweet(tweet, false, false, nil)
	urls, ok := artifact.Metadata["urls"].([]string)
	if !ok {
		t.Fatal("expected urls metadata to be []string")
	}
	if len(urls) != 2 {
		t.Errorf("expected 2 safe URLs, got %d: %v", len(urls), urls)
	}
	for _, u := range urls {
		if strings.HasPrefix(u, "javascript:") || strings.HasPrefix(u, "data:") {
			t.Errorf("unsafe URL leaked through filter: %s", u)
		}
	}
	// url_count should reflect only safe URLs
	count, _ := artifact.Metadata["url_count"].(int)
	if count != 2 {
		t.Errorf("expected url_count=2, got %d", count)
	}
}

func TestConnect_APIModeRequiresBearerToken(t *testing.T) {
	// sync_mode=api without bearer_token must fail-loud (CWE-287).
	c := New("twitter")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "api"},
	})
	if err == nil {
		t.Error("expected error when bearer_token missing for API mode")
	}
	if err != nil && !strings.Contains(err.Error(), "bearer_token") {
		t.Errorf("expected 'bearer_token' in error, got: %v", err)
	}
}

func TestConnect_HybridModeWithoutTokenAllowed(t *testing.T) {
	// Hybrid mode without token should connect (archive is primary).
	c := New("twitter")
	dir := t.TempDir()
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"sync_mode":   "hybrid",
			"archive_dir": dir,
		},
	})
	if err != nil {
		t.Errorf("hybrid mode should allow missing token (archive is primary): %v", err)
	}
}

func TestTruncateUTF8_ASCIIOnly(t *testing.T) {
	got := truncateUTF8("Hello, World!", 5)
	if got != "Hello" {
		t.Errorf("expected 'Hello', got %q", got)
	}
}

func TestTruncateUTF8_MultiByteBoundary(t *testing.T) {
	// "é" is 2 bytes in UTF-8. "café" = 5 bytes. Truncating at 4 should not
	// split the "é" — should truncate to "caf" (3 bytes).
	s := "café"
	got := truncateUTF8(s, 4)
	if got != "caf" {
		t.Errorf("expected 'caf', got %q", got)
	}
}

func TestTruncateUTF8_ThreeByteRune(t *testing.T) {
	// "日" is 3 bytes. "AB日" = 5 bytes. Truncating at 4 should → "AB" (2 bytes).
	s := "AB日"
	got := truncateUTF8(s, 4)
	if got != "AB" {
		t.Errorf("expected 'AB', got %q", got)
	}
}

func TestTruncateUTF8_FourByteEmoji(t *testing.T) {
	// "🐦" is 4 bytes. "X🐦" = 5 bytes. Truncating at 3 → "X" (1 byte).
	s := "X🐦"
	got := truncateUTF8(s, 3)
	if got != "X" {
		t.Errorf("expected 'X', got %q", got)
	}
}

// --- Improve R16: IMP-015-R16-001 — bookmarked/liked signal file parsing ---

func TestSyncArchive_LikeSignalElevatesTier(t *testing.T) {
	// Before the fix, like.js was never parsed — all tweets got
	// bookmarked=false, liked=false, so the full-tier fast path for
	// user-curated content was dead code.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)

	// tweets.js with two tweets
	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"10","full_text":"short","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}},{"tweet":{"id":"20","full_text":"also short","created_at":"Wed Mar 15 15:00:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	// like.js marks tweet 10 as liked
	os.WriteFile(filepath.Join(dataDir, "like.js"),
		[]byte(`window.YTD.like.part0 = [{"like":{"tweetId":"10","fullText":"short"}}]`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	artifacts, _, err := c.syncArchive(context.Background(), "")
	if err != nil {
		t.Fatalf("syncArchive failed: %v", err)
	}

	for _, a := range artifacts {
		id := a.Metadata["tweet_id"].(string)
		tier := a.Metadata["processing_tier"].(string)
		liked := a.Metadata["is_liked"].(bool)
		if id == "10" {
			if !liked {
				t.Error("tweet 10 must be liked=true from like.js")
			}
			if tier != "full" {
				t.Errorf("liked tweet must be full tier, got %q", tier)
			}
		}
		if id == "20" {
			if liked {
				t.Error("tweet 20 must not be liked")
			}
			// Short tweet with 0 engagement: should be metadata tier
			if tier != "metadata" {
				t.Errorf("unloved short tweet expected metadata tier, got %q", tier)
			}
		}
	}
}

func TestSyncArchive_BookmarkSignalElevatesTier(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)

	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"30","full_text":"tiny","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	// bookmark.js marks tweet 30 as bookmarked
	os.WriteFile(filepath.Join(dataDir, "bookmark.js"),
		[]byte(`window.YTD.bookmark.part0 = [{"bookmark":{"tweetId":"30"}}]`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	artifacts, _, err := c.syncArchive(context.Background(), "")
	if err != nil {
		t.Fatalf("syncArchive failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	a := artifacts[0]
	if a.Metadata["is_bookmarked"] != true {
		t.Error("tweet 30 must be bookmarked=true from bookmark.js")
	}
	if a.Metadata["processing_tier"] != "full" {
		t.Errorf("bookmarked tweet must be full tier, got %q", a.Metadata["processing_tier"])
	}
}

func TestSyncArchive_MissingSignalFilesGraceful(t *testing.T) {
	// When like.js and bookmark.js don't exist (older exports), sync must
	// still succeed — signals are best-effort.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)

	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"1","full_text":"hello world tweet","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	artifacts, _, err := c.syncArchive(context.Background(), "")
	if err != nil {
		t.Fatalf("syncArchive must succeed without signal files: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
}

func TestSyncArchive_CorruptSignalFileGraceful(t *testing.T) {
	// A corrupt signal file must not crash or block the main tweet import.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)

	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"1","full_text":"hello world tweet","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	os.WriteFile(filepath.Join(dataDir, "like.js"),
		[]byte(`THIS IS NOT VALID JSON OR JS`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	artifacts, _, err := c.syncArchive(context.Background(), "")
	if err != nil {
		t.Fatalf("corrupt signal file must not break sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
}

func TestParseSignalFile_SymlinkEscapeBlocked(t *testing.T) {
	// Signal file that is a symlink escaping archive_dir must be rejected.
	archiveDir := t.TempDir()
	dataDir := filepath.Join(archiveDir, "data")
	os.MkdirAll(dataDir, 0o755)

	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.json")
	os.WriteFile(outsideFile, []byte(`window.YTD.like.part0 = [{"like":{"tweetId":"999"}}]`), 0o600)

	symlinkPath := filepath.Join(dataDir, "like.js")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skipf("cannot create symlinks on this OS: %v", err)
	}

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: archiveDir}

	ids := c.parseSignalFile(context.Background(), "like.js", "like")
	if len(ids) != 0 {
		t.Error("signal file escaping archive_dir must return empty set")
	}
}

// --- Improve R16: IMP-015-R16-002 — mentions in metadata ---

func TestNormalizeTweet_MentionsInMetadata(t *testing.T) {
	// Before the fix, mentions were parsed from JSON but never stored
	// in the artifact metadata — an R-005 compliance gap.
	tweet := ArchiveTweet{
		ID:       "400",
		FullText: "Thanks @alice and @bob for the help!",
		Entities: TweetEntities{
			Mentions: []TweetMention{
				{ScreenName: "alice"},
				{ScreenName: "bob"},
			},
		},
	}

	artifact := normalizeTweet(tweet, false, false, nil)
	mentions, ok := artifact.Metadata["mentions"].([]string)
	if !ok {
		t.Fatal("expected mentions metadata to be []string")
	}
	if len(mentions) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(mentions))
	}
	if mentions[0] != "alice" || mentions[1] != "bob" {
		t.Errorf("unexpected mentions: %v", mentions)
	}
}

func TestNormalizeTweet_EmptyMentions(t *testing.T) {
	tweet := ArchiveTweet{
		ID:       "401",
		FullText: "No mentions here",
	}
	artifact := normalizeTweet(tweet, false, false, nil)
	mentions, ok := artifact.Metadata["mentions"].([]string)
	if !ok {
		t.Fatal("expected mentions metadata to be []string")
	}
	if len(mentions) != 0 {
		t.Errorf("expected 0 mentions, got %d", len(mentions))
	}
}

// --- Improve R16: IMP-015-R16-003 — control char sanitization in title (CWE-116) ---

func TestBuildTweetTitle_NewlinesSanitized(t *testing.T) {
	// Before the fix, control characters passed through verbatim into the
	// artifact title, enabling potential log/output injection (CWE-116).
	tweet := ArchiveTweet{
		ID:       "500",
		FullText: "Line one\nLine two\rLine three\tTabbed",
	}
	title := buildTweetTitle(tweet)
	if strings.ContainsAny(title, "\n\r\t") {
		t.Errorf("title must not contain control characters, got %q", title)
	}
	if !strings.Contains(title, "Line one") || !strings.Contains(title, "Line two") {
		t.Error("title content must be preserved after sanitization")
	}
}

func TestBuildTweetTitle_NullByteSanitized(t *testing.T) {
	tweet := ArchiveTweet{
		ID:       "501",
		FullText: "Hello\x00World",
	}
	title := buildTweetTitle(tweet)
	if strings.Contains(title, "\x00") {
		t.Error("null byte must be sanitized from title")
	}
	if !strings.Contains(title, "Hello") || !strings.Contains(title, "World") {
		t.Error("content around null byte must be preserved")
	}
}

func TestSanitizeControlChars_PreservesUnicode(t *testing.T) {
	input := "日本語のツイート 🐦"
	got := sanitizeControlChars(input)
	if got != input {
		t.Errorf("Unicode content must be preserved, got %q", got)
	}
}

func TestSanitizeControlChars_C1Range(t *testing.T) {
	// C1 control characters (U+007F to U+009F) must be replaced.
	input := "before\x7Fafter\u0085more"
	got := sanitizeControlChars(input)
	if strings.ContainsRune(got, 0x7F) {
		t.Error("DEL (0x7F) must be sanitized")
	}
	if strings.ContainsRune(got, 0x85) {
		t.Error("NEL (U+0085) must be sanitized")
	}
}

func TestTruncateUTF8_ShortString(t *testing.T) {
	got := truncateUTF8("hi", 80)
	if got != "hi" {
		t.Errorf("expected 'hi', got %q", got)
	}
}

func TestBuildTweetTitle_UTF8Safe(t *testing.T) {
	// Title with multi-byte chars near the boundary must not produce invalid UTF-8.
	// 78 ASCII + "日本" = 78 + 6 = 84 bytes; truncation should not split a rune.
	tweet := ArchiveTweet{
		FullText: strings.Repeat("a", 78) + "日本",
	}
	title := buildTweetTitle(tweet)
	if !strings.HasSuffix(title, "...") {
		t.Error("expected truncated title to end with ...")
	}
	// Verify the title is valid UTF-8
	if strings.ToValidUTF8(title, "\xff") != title {
		t.Error("truncated title contains invalid UTF-8")
	}
	// The title (minus "...") should be exactly the ASCII portion since the rune
	// at byte 78 is a 3-byte char that can't fit in 2 remaining bytes.
	trimmed := strings.TrimSuffix(title, "...")
	if len(trimmed) > 80 {
		t.Errorf("truncated title body exceeds 80 bytes: %d", len(trimmed))
	}
}

func TestMaxTweetCount_ConstantSet(t *testing.T) {
	if maxTweetCount != 500_000 {
		t.Errorf("maxTweetCount should be 500000, got %d", maxTweetCount)
	}
}

// --- Improve: IMP-001 — Graduated health escalation with consecutive error tracking ---

func TestSync_ConsecutiveErrorsEscalateToDegraded(t *testing.T) {
	c := New("twitter")
	dir := t.TempDir()
	// No data/ subdir — every sync will fail

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	// First failure: should be degraded (consecutiveErrors=1, <5)
	c.Sync(context.Background(), "")
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected degraded after 1 failure, got %s", c.Health(context.Background()))
	}
}

func TestSync_ConsecutiveErrorsEscalateToFailing(t *testing.T) {
	c := New("twitter")
	dir := t.TempDir()

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	// 5 consecutive failures: should escalate to failing
	for i := 0; i < 5; i++ {
		c.Sync(context.Background(), "")
	}
	if c.Health(context.Background()) != connector.HealthFailing {
		t.Errorf("expected failing after 5 consecutive failures, got %s", c.Health(context.Background()))
	}
}

func TestSync_ConsecutiveErrorsEscalateToError(t *testing.T) {
	c := New("twitter")
	dir := t.TempDir()

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	// 10 consecutive failures: should escalate to error
	for i := 0; i < 10; i++ {
		c.Sync(context.Background(), "")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected error after 10 consecutive failures, got %s", c.Health(context.Background()))
	}
}

func TestSync_SuccessResetsConsecutiveErrors(t *testing.T) {
	c := New("twitter")
	dir := t.TempDir()

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	// 4 consecutive failures
	for i := 0; i < 4; i++ {
		c.Sync(context.Background(), "")
	}
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Fatalf("expected degraded after 4 failures, got %s", c.Health(context.Background()))
	}

	// Create data so next sync succeeds
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"1","full_text":"recovery","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	c.Sync(context.Background(), "")
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy after successful sync, got %s", c.Health(context.Background()))
	}

	// Verify consecutive errors reset — next failure should be degraded, not failing
	os.Remove(filepath.Join(dataDir, "tweets.js"))
	os.Remove(dataDir)
	c.Sync(context.Background(), "")
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected degraded (reset counter) after new failure, got %s", c.Health(context.Background()))
	}
}

// --- Improve: IMP-002 — Sync metrics for operational observability ---

func TestSyncMetrics_TracksSuccessfulSync(t *testing.T) {
	c := New("twitter")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"1","full_text":"hello world tweet","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	before := time.Now()
	c.Sync(context.Background(), "")
	lastSync, count, errors, consec := c.SyncMetrics()

	if lastSync.Before(before) {
		t.Error("lastSyncTime should be after sync started")
	}
	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
	if errors != 0 {
		t.Errorf("expected errors=0, got %d", errors)
	}
	if consec != 0 {
		t.Errorf("expected consecutiveErrors=0, got %d", consec)
	}
}

func TestSyncMetrics_TracksFailedSync(t *testing.T) {
	c := New("twitter")
	dir := t.TempDir()

	if err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": dir},
	}); err != nil {
		t.Fatal(err)
	}

	c.Sync(context.Background(), "")
	_, count, errors, consec := c.SyncMetrics()

	if count != 0 {
		t.Errorf("expected count=0 on failure, got %d", count)
	}
	if errors != 1 {
		t.Errorf("expected errors=1, got %d", errors)
	}
	if consec != 1 {
		t.Errorf("expected consecutiveErrors=1, got %d", consec)
	}
}

// --- Improve: IMP-003 — Connect sets HealthError on config validation failure ---

func TestConnect_SetsHealthErrorOnFailure(t *testing.T) {
	c := New("twitter")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": ""},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected HealthError after failed connect, got %s", c.Health(context.Background()))
	}
}

func TestConnect_NonexistentDir_SetsHealthError(t *testing.T) {
	c := New("twitter")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "archive", "archive_dir": "/nonexistent/path"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected HealthError after nonexistent dir, got %s", c.Health(context.Background()))
	}
}

// --- Improve: IMP-004 — Media content type detection (tweet/image, tweet/video) ---

func TestClassifyTweet_Image(t *testing.T) {
	tweet := ArchiveTweet{
		FullText: "Check out this photo!",
		Entities: TweetEntities{
			Media: []TweetMedia{{Type: "photo"}},
		},
	}
	got := classifyTweet(tweet, nil)
	if got != "tweet/image" {
		t.Errorf("expected tweet/image, got %s", got)
	}
}

func TestClassifyTweet_Video(t *testing.T) {
	tweet := ArchiveTweet{
		FullText: "Watch this video!",
		Entities: TweetEntities{
			Media: []TweetMedia{{Type: "video"}},
		},
	}
	got := classifyTweet(tweet, nil)
	if got != "tweet/video" {
		t.Errorf("expected tweet/video, got %s", got)
	}
}

func TestClassifyTweet_AnimatedGif(t *testing.T) {
	tweet := ArchiveTweet{
		FullText: "Funny gif!",
		Entities: TweetEntities{
			Media: []TweetMedia{{Type: "animated_gif"}},
		},
	}
	got := classifyTweet(tweet, nil)
	if got != "tweet/video" {
		t.Errorf("expected tweet/video for animated_gif, got %s", got)
	}
}

func TestClassifyTweet_MediaPrecedenceOverURL(t *testing.T) {
	// When a tweet has both media and URLs, media takes precedence.
	tweet := ArchiveTweet{
		FullText: "Photo with link",
		Entities: TweetEntities{
			URLs:  []TweetURL{{ExpandedURL: "https://example.com"}},
			Media: []TweetMedia{{Type: "photo"}},
		},
	}
	got := classifyTweet(tweet, nil)
	if got != "tweet/image" {
		t.Errorf("expected tweet/image (media precedence), got %s", got)
	}
}

func TestClassifyTweet_ThreadPrecedenceOverMedia(t *testing.T) {
	// Threads take highest precedence even with media.
	tweet := ArchiveTweet{
		FullText: "Thread with photo",
		Entities: TweetEntities{
			Media: []TweetMedia{{Type: "photo"}},
		},
	}
	thread := &Thread{RootID: "1"}
	got := classifyTweet(tweet, thread)
	if got != "tweet/thread" {
		t.Errorf("expected tweet/thread (thread precedence), got %s", got)
	}
}

func TestNormalizeTweet_MediaMetadata(t *testing.T) {
	tweet := ArchiveTweet{
		ID:       "700",
		FullText: "Photo tweet",
		Entities: TweetEntities{
			Media: []TweetMedia{{Type: "photo"}, {Type: "photo"}},
		},
	}
	artifact := normalizeTweet(tweet, false, false, nil)
	if artifact.ContentType != "tweet/image" {
		t.Errorf("expected tweet/image, got %s", artifact.ContentType)
	}
	mediaTypes, ok := artifact.Metadata["media_types"].([]string)
	if !ok {
		t.Fatal("expected media_types metadata to be []string")
	}
	if len(mediaTypes) != 2 {
		t.Errorf("expected 2 media types, got %d", len(mediaTypes))
	}
	count, ok := artifact.Metadata["media_count"].(int)
	if !ok || count != 2 {
		t.Errorf("expected media_count=2, got %v", artifact.Metadata["media_count"])
	}
}

func TestNormalizeTweet_NoMediaNoMetadata(t *testing.T) {
	tweet := ArchiveTweet{
		ID:       "701",
		FullText: "Text only tweet",
	}
	artifact := normalizeTweet(tweet, false, false, nil)
	if _, ok := artifact.Metadata["media_types"]; ok {
		t.Error("tweets without media should not have media_types metadata")
	}
	if _, ok := artifact.Metadata["media_count"]; ok {
		t.Error("tweets without media should not have media_count metadata")
	}
}

// --- Test coverage gap closure ---

func TestSyncArchive_UnparseableTimestampSkipped(t *testing.T) {
	// Tweets with unparseable timestamps must be skipped; valid tweets processed.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)

	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"1","full_text":"good tweet with enough text to be standard","created_at":"Wed Mar 15 14:30:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}},{"tweet":{"id":"2","full_text":"bad timestamp tweet","created_at":"NOT-A-DATE","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	artifacts, _, err := c.syncArchive(context.Background(), "")
	if err != nil {
		t.Fatalf("syncArchive should succeed despite bad timestamps: %v", err)
	}
	if len(artifacts) != 1 {
		t.Errorf("expected 1 artifact (bad timestamp skipped), got %d", len(artifacts))
	}
	if len(artifacts) > 0 && artifacts[0].SourceRef != "1" {
		t.Errorf("expected tweet ID 1, got %s", artifacts[0].SourceRef)
	}
}

func TestSyncArchive_CursorSkipsOlderTweets(t *testing.T) {
	// Tweets at or before the cursor must be skipped.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)

	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [{"tweet":{"id":"1","full_text":"old tweet with enough chars for standard tier assignment","created_at":"Wed Mar 15 10:00:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}},{"tweet":{"id":"2","full_text":"new tweet with enough chars for standard tier assignment","created_at":"Wed Mar 15 20:00:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}}]`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	// Cursor set after tweet 1, before tweet 2
	artifacts, newCursor, err := c.syncArchive(context.Background(), "2026-03-15T15:00:00Z")
	if err != nil {
		t.Fatalf("syncArchive failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 new artifact, got %d", len(artifacts))
	}
	if artifacts[0].SourceRef != "2" {
		t.Errorf("expected tweet 2 (newer than cursor), got %s", artifacts[0].SourceRef)
	}
	if newCursor <= "2026-03-15T15:00:00Z" {
		t.Error("cursor should advance past the new tweet")
	}
}

func TestParseSignalFile_ContextCancelled(t *testing.T) {
	// When context is cancelled, parseSignalFile must return empty set.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "like.js"),
		[]byte(`window.YTD.like.part0 = [{"like":{"tweetId":"10"}}]`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ids := c.parseSignalFile(ctx, "like.js", "like")
	if len(ids) != 0 {
		t.Errorf("expected empty set on cancelled context, got %d IDs", len(ids))
	}
}

func TestParseSignalFile_EmptyTweetIDSkipped(t *testing.T) {
	// Signal entries with empty tweetId should be ignored.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "like.js"),
		[]byte(`window.YTD.like.part0 = [{"like":{"tweetId":""}},{"like":{"tweetId":"42"}}]`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	ids := c.parseSignalFile(context.Background(), "like.js", "like")
	if len(ids) != 1 {
		t.Errorf("expected 1 ID (empty tweetId skipped), got %d", len(ids))
	}
	if !ids["42"] {
		t.Error("expected tweet 42 in signal set")
	}
}

func TestParseSignalFile_MalformedSignalEntry(t *testing.T) {
	// Entries that don't match the expected signal key are silently skipped.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "like.js"),
		[]byte(`window.YTD.like.part0 = [{"wrong_key":{"tweetId":"10"}},{"like":{"tweetId":"20"}}]`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	ids := c.parseSignalFile(context.Background(), "like.js", "like")
	if len(ids) != 1 {
		t.Errorf("expected 1 ID (wrong key skipped), got %d", len(ids))
	}
	if !ids["20"] {
		t.Error("expected tweet 20 in signal set")
	}
}

func TestParseSignalFile_NoArrayBracket(t *testing.T) {
	// Signal file without JSON array bracket returns empty set.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "like.js"),
		[]byte(`window.YTD.like.part0 = {}`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	ids := c.parseSignalFile(context.Background(), "like.js", "like")
	if len(ids) != 0 {
		t.Errorf("expected empty set for non-array signal file, got %d", len(ids))
	}
}

func TestAssignTweetTier_StandardDefault(t *testing.T) {
	// Medium-length tweet with no special attributes gets standard tier.
	tweet := ArchiveTweet{
		FullText:      "This is a regular tweet that is definitely longer than fifty characters in length",
		FavoriteCount: 5,
		RetweetCount:  0,
	}
	tier := assignTweetTier(tweet, false, false, nil)
	if tier != "standard" {
		t.Errorf("expected standard tier for default tweet, got %s", tier)
	}
}

func TestNormalizeTweet_InReplyToMetadata(t *testing.T) {
	// Verify in_reply_to metadata is set for reply tweets.
	tweet := ArchiveTweet{
		ID:                "300",
		FullText:          "This is a reply to another tweet",
		InReplyToStatusID: "200",
	}
	artifact := normalizeTweet(tweet, false, false, nil)
	replyTo, ok := artifact.Metadata["in_reply_to"].(string)
	if !ok || replyTo != "200" {
		t.Errorf("expected in_reply_to=200, got %v", artifact.Metadata["in_reply_to"])
	}
}

func TestNormalizeTweet_ThreadMetadata(t *testing.T) {
	// Verify thread metadata fields are correctly set.
	tweet := ArchiveTweet{
		ID:       "301",
		FullText: "Part of a thread",
	}
	thread := &Thread{RootID: "300"}
	artifact := normalizeTweet(tweet, false, false, thread)
	if artifact.Metadata["is_thread"] != true {
		t.Error("expected is_thread=true")
	}
	if artifact.Metadata["thread_id"] != "300" {
		t.Errorf("expected thread_id=300, got %v", artifact.Metadata["thread_id"])
	}
}

func TestConnect_APIModeWithBearerToken(t *testing.T) {
	// API mode with bearer token should connect successfully (no archive validation).
	c := New("twitter")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "api"},
		Credentials:  map[string]string{"bearer_token": "test-token-value"},
	})
	if err != nil {
		t.Errorf("API mode with bearer token should succeed: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy after API connect, got %s", c.Health(context.Background()))
	}
}

func TestConnect_HybridModeWithBearerToken(t *testing.T) {
	// Hybrid mode with bearer token and valid archive dir.
	c := New("twitter")
	dir := t.TempDir()
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "hybrid", "archive_dir": dir},
		Credentials:  map[string]string{"bearer_token": "test-token"},
	})
	if err != nil {
		t.Errorf("hybrid mode with token and dir should succeed: %v", err)
	}
}

func TestIsSafeURL_SchemeOnly(t *testing.T) {
	// URL with no host — still has a scheme, but it's safe by scheme check.
	if !isSafeURL("https:") {
		t.Error("https: with no host should still be scheme-safe")
	}
}

func TestIsSafeURL_FTPRejected(t *testing.T) {
	if isSafeURL("ftp://example.com/file") {
		t.Error("ftp: scheme must be rejected")
	}
}

func TestIsSafeURL_MixedCaseScheme(t *testing.T) {
	if !isSafeURL("HTTPS://example.com") {
		t.Error("HTTPS (uppercase) should be accepted")
	}
}

func TestSyncArchive_TweetsJSNotFound(t *testing.T) {
	// Archive dir exists with data/ subdir but no tweets.js.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	// data/ exists but tweets.js does not

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	_, _, err := c.syncArchive(context.Background(), "")
	if err == nil {
		t.Error("expected error when tweets.js is missing")
	}
	if !strings.Contains(err.Error(), "tweets.js") {
		t.Errorf("expected error mentioning tweets.js, got: %v", err)
	}
}

func TestSync_APIModeSkipsArchive(t *testing.T) {
	// API-only mode should not try to read archive files.
	c := New("twitter")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{"sync_mode": "api"},
		Credentials:  map[string]string{"bearer_token": "test-token"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Sync in API mode with no API implementation yet — should return empty, no error.
	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Errorf("API mode sync should not error (no archive access): %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts from API-only sync (not implemented), got %d", len(artifacts))
	}
	if cursor != "" {
		t.Errorf("expected empty cursor, got %q", cursor)
	}
}

func TestParseSignalFile_MalformedInnerJSON(t *testing.T) {
	// Signal entry where the inner JSON (under the signal key) is malformed.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "like.js"),
		[]byte(`window.YTD.like.part0 = [{"like":"not-an-object"},{"like":{"tweetId":"50"}}]`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	ids := c.parseSignalFile(context.Background(), "like.js", "like")
	if len(ids) != 1 {
		t.Errorf("expected 1 ID (malformed entry skipped), got %d", len(ids))
	}
	if !ids["50"] {
		t.Error("expected tweet 50 in signal set")
	}
}

func TestIsSafeURL_InvalidIPv6(t *testing.T) {
	// Unterminated IPv6 literal triggers a url.Parse error.
	if isSafeURL("http://[::1") {
		t.Error("malformed URL (unterminated IPv6) should not be safe")
	}
}

func TestParseSignalFile_DirectoryInsteadOfFile(t *testing.T) {
	// If the signal "file" is actually a directory, ReadFile will fail.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)
	// Create like.js as a directory instead of a file
	os.MkdirAll(filepath.Join(dataDir, "like.js"), 0o755)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	ids := c.parseSignalFile(context.Background(), "like.js", "like")
	if len(ids) != 0 {
		t.Errorf("expected empty set when signal path is a directory, got %d", len(ids))
	}
}

func TestSyncArchive_FullRoundTrip(t *testing.T) {
	// End-to-end: archive with tweets, likes, bookmarks, threads; verify all
	// metadata propagation and cursor advancement in a single sync.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o755)

	os.WriteFile(filepath.Join(dataDir, "tweets.js"),
		[]byte(`window.YTD.tweet.part0 = [`+
			`{"tweet":{"id":"100","full_text":"Thread start with enough text for standard","created_at":"Wed Mar 15 14:00:00 +0000 2026","favorite_count":0,"retweet_count":0,"in_reply_to_status_id":"","entities":{"urls":[],"hashtags":[],"user_mentions":[]}}},`+
			`{"tweet":{"id":"101","full_text":"Thread reply with enough text to be standard tier","created_at":"Wed Mar 15 14:05:00 +0000 2026","favorite_count":0,"retweet_count":0,"in_reply_to_status_id":"100","entities":{"urls":[],"hashtags":[],"user_mentions":[]}}},`+
			`{"tweet":{"id":"200","full_text":"RT @other: retweeted content that should be light tier","created_at":"Wed Mar 15 15:00:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[],"hashtags":[],"user_mentions":[]}}},`+
			`{"tweet":{"id":"300","full_text":"A tweet with a link for you","created_at":"Wed Mar 15 16:00:00 +0000 2026","favorite_count":0,"retweet_count":0,"entities":{"urls":[{"expanded_url":"https://example.com/article"}],"hashtags":[{"text":"golang"}],"user_mentions":[{"screen_name":"gopher"}]}}}`+
			`]`), 0o600)

	os.WriteFile(filepath.Join(dataDir, "like.js"),
		[]byte(`window.YTD.like.part0 = [{"like":{"tweetId":"200"}}]`), 0o600)
	os.WriteFile(filepath.Join(dataDir, "bookmark.js"),
		[]byte(`window.YTD.bookmark.part0 = [{"bookmark":{"tweetId":"300"}}]`), 0o600)

	c := New("twitter")
	c.config = TwitterConfig{SyncMode: SyncModeArchive, ArchiveDir: dir}

	artifacts, cursor, err := c.syncArchive(context.Background(), "")
	if err != nil {
		t.Fatalf("syncArchive failed: %v", err)
	}
	if len(artifacts) != 4 {
		t.Fatalf("expected 4 artifacts, got %d", len(artifacts))
	}
	if cursor == "" {
		t.Error("expected non-empty cursor")
	}

	// Check thread tweets have thread metadata
	for _, a := range artifacts {
		id := a.SourceRef
		switch id {
		case "100", "101":
			if a.Metadata["is_thread"] != true {
				t.Errorf("tweet %s should have thread metadata", id)
			}
		case "200":
			if a.Metadata["is_liked"] != true {
				t.Errorf("tweet 200 should be liked")
			}
		case "300":
			if a.Metadata["is_bookmarked"] != true {
				t.Error("tweet 300 should be bookmarked")
			}
			urls := a.Metadata["urls"].([]string)
			if len(urls) != 1 || urls[0] != "https://example.com/article" {
				t.Errorf("expected 1 URL, got %v", urls)
			}
		}
	}
}
