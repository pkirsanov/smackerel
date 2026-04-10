package twitter

import (
	"context"
	"testing"

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
	if len(threads[0].TweetIDs) != 3 {
		t.Errorf("expected 3 tweets in thread, got %d", len(threads[0].TweetIDs))
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
	ts := parseTweetTime("Wed Mar 15 14:30:00 +0000 2026")
	if ts.Year() != 2026 || ts.Month() != 3 || ts.Day() != 15 {
		t.Errorf("unexpected time: %v", ts)
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
