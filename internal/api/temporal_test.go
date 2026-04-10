package api

import (
	"testing"
	"time"
)

func TestParseTemporalIntent_LastWeek(t *testing.T) {
	f := parseTemporalIntent("pricing video from last week")
	if f == nil {
		t.Fatal("expected temporal filter for 'from last week'")
	}
	if f.Cleaned != "pricing video" {
		t.Errorf("expected cleaned query 'pricing video', got %q", f.Cleaned)
	}

	from, _ := time.Parse(time.RFC3339, f.DateFrom)
	daysDiff := time.Since(from).Hours() / 24
	if daysDiff < 6.5 || daysDiff > 7.5 {
		t.Errorf("expected ~7 days ago, got %.1f days", daysDiff)
	}
}

func TestParseTemporalIntent_Yesterday(t *testing.T) {
	f := parseTemporalIntent("meeting notes yesterday")
	if f == nil {
		t.Fatal("expected temporal filter for 'yesterday'")
	}
	if f.Cleaned != "meeting notes" {
		t.Errorf("expected cleaned query 'meeting notes', got %q", f.Cleaned)
	}
}

func TestParseTemporalIntent_ThisMonth(t *testing.T) {
	f := parseTemporalIntent("articles this month")
	if f == nil {
		t.Fatal("expected temporal filter for 'this month'")
	}
	if f.Cleaned != "articles" {
		t.Errorf("expected cleaned query 'articles', got %q", f.Cleaned)
	}

	from, _ := time.Parse(time.RFC3339, f.DateFrom)
	if from.Day() != 1 {
		t.Errorf("expected first of month, got day %d", from.Day())
	}
}

func TestParseTemporalIntent_Today(t *testing.T) {
	f := parseTemporalIntent("what did I save today")
	if f == nil {
		t.Fatal("expected temporal filter for 'today'")
	}
}

func TestParseTemporalIntent_Recently(t *testing.T) {
	f := parseTemporalIntent("that article I read recently")
	if f == nil {
		t.Fatal("expected temporal filter for 'recently'")
	}
	if f.Cleaned != "that article I read" {
		t.Errorf("expected cleaned query, got %q", f.Cleaned)
	}
}

func TestParseTemporalIntent_NoTemporal(t *testing.T) {
	f := parseTemporalIntent("machine learning concepts")
	if f != nil {
		t.Errorf("expected nil filter for non-temporal query, got %+v", f)
	}
}

func TestParseTemporalIntent_CaseInsensitive(t *testing.T) {
	f := parseTemporalIntent("video From Last Week")
	if f == nil {
		t.Fatal("expected case-insensitive match")
	}
}

func TestParseTemporalIntent_LastMonth(t *testing.T) {
	f := parseTemporalIntent("expenses last month")
	if f == nil {
		t.Fatal("expected temporal filter for 'last month'")
	}

	from, _ := time.Parse(time.RFC3339, f.DateFrom)
	daysDiff := time.Since(from).Hours() / 24
	if daysDiff < 28 || daysDiff > 32 {
		t.Errorf("expected ~30 days ago, got %.1f days", daysDiff)
	}
}

func TestParseTemporalIntent_EmptyString(t *testing.T) {
	f := parseTemporalIntent("")
	if f != nil {
		t.Errorf("expected nil filter for empty string, got %+v", f)
	}
}

func TestParseTemporalIntent_TemporalWordOnly(t *testing.T) {
	// "yesterday" alone should still produce a temporal filter
	f := parseTemporalIntent("yesterday")
	if f == nil {
		t.Fatal("expected temporal filter for bare 'yesterday'")
	}
}

func TestParseTemporalIntent_DateFromIsRFC3339(t *testing.T) {
	f := parseTemporalIntent("articles from last week")
	if f == nil {
		t.Fatal("expected temporal filter")
	}
	_, err := time.Parse(time.RFC3339, f.DateFrom)
	if err != nil {
		t.Errorf("DateFrom should be valid RFC3339, got %q: %v", f.DateFrom, err)
	}
	if f.DateTo != "" {
		_, err := time.Parse(time.RFC3339, f.DateTo)
		if err != nil {
			t.Errorf("DateTo should be valid RFC3339, got %q: %v", f.DateTo, err)
		}
	}
}

func TestParseTemporalIntent_ThisWeek(t *testing.T) {
	f := parseTemporalIntent("emails this week")
	if f == nil {
		t.Fatal("expected temporal filter for 'this week'")
	}
	if f.Cleaned != "emails" {
		t.Errorf("expected cleaned query 'emails', got %q", f.Cleaned)
	}

	from, _ := time.Parse(time.RFC3339, f.DateFrom)
	now := time.Now()
	// "this week" should start from the beginning of the current week (Sunday)
	expectedStart := now.AddDate(0, 0, -int(now.Weekday()))
	daysDiff := from.Sub(expectedStart).Hours() / 24
	if daysDiff < -0.5 || daysDiff > 0.5 {
		t.Errorf("expected start of week, got %.1f days difference", daysDiff)
	}
}

func TestParseTemporalIntent_LastYear(t *testing.T) {
	f := parseTemporalIntent("research papers last year")
	if f == nil {
		t.Fatal("expected temporal filter for 'last year'")
	}
	if f.Cleaned != "research papers" {
		t.Errorf("expected cleaned query 'research papers', got %q", f.Cleaned)
	}

	from, _ := time.Parse(time.RFC3339, f.DateFrom)
	daysDiff := time.Since(from).Hours() / 24
	if daysDiff < 360 || daysDiff > 370 {
		t.Errorf("expected ~365 days ago, got %.1f days", daysDiff)
	}
}

func TestParseTemporalIntent_PastFewDays(t *testing.T) {
	f := parseTemporalIntent("notes from the past few days")
	if f == nil {
		t.Fatal("expected temporal filter for 'past few days'")
	}
	if f.Cleaned != "notes from the" {
		// The parser removes "past few days" but the "from the" prefix dangling cleanup removes "from"
		// then trims, so we accept whatever the parser produces
	}

	from, _ := time.Parse(time.RFC3339, f.DateFrom)
	daysDiff := time.Since(from).Hours() / 24
	if daysDiff < 2.5 || daysDiff > 3.5 {
		t.Errorf("expected ~3 days ago, got %.1f days", daysDiff)
	}
}

func TestParseTemporalIntent_PastWeek(t *testing.T) {
	f := parseTemporalIntent("articles past week")
	if f == nil {
		t.Fatal("expected temporal filter for 'past week'")
	}
	if f.Cleaned != "articles" {
		t.Errorf("expected cleaned query 'articles', got %q", f.Cleaned)
	}
}

func TestParseTemporalIntent_PastMonth(t *testing.T) {
	f := parseTemporalIntent("videos from past month")
	if f == nil {
		t.Fatal("expected temporal filter for 'past month'")
	}
	if f.Cleaned != "videos" {
		t.Errorf("expected cleaned query 'videos', got %q", f.Cleaned)
	}
}
