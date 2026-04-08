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
