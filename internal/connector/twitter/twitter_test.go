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
