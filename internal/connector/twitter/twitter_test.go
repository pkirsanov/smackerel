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
