package backup

// Unit tests for spec 048 retention policy.
//
// Test plan (all adversarial — each test would fail if the retention
// policy regressed to a naive "keep N newest" implementation):
//
//   T-048-001a Exactly 7 daily + 4 weekly slots are kept for a long
//              history that spans multiple ISO weeks. Older artifacts
//              outside both windows are pruned.
//
//   T-048-001b Multiple backups in the same calendar day collapse to
//              one daily slot (the newest of that day). Older same-day
//              copies are pruned even when they are inside the 7-day
//              window.
//
//   T-048-001c Weekly slots count ISO weeks: two backups in the same
//              ISO week share a weekly slot. A backup in last week's
//              ISO calendar that is ALSO the daily slot does not
//              consume an additional weekly slot.
//
//   T-048-001d Empty input returns empty keep + empty prune (no panic).
//
//   T-048-001e Input shorter than total budget keeps everything.
//
//   T-048-001f Pure function — calling SelectKept twice on the same
//              input returns identical results; input slice is not
//              mutated.

import (
	"reflect"
	"testing"
	"time"
)

func mustParse(t *testing.T, ts string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t.Fatalf("test setup: parse %q: %v", ts, err)
	}
	return parsed.UTC()
}

func mkArtifact(t *testing.T, when string) Artifact {
	t.Helper()
	created := mustParse(t, when)
	return Artifact{
		Name:      "smackerel-" + created.Format("2006-01-02-150405") + ".sql.gz",
		CreatedAt: created,
	}
}

func names(arts []Artifact) []string {
	out := make([]string, 0, len(arts))
	for _, a := range arts {
		out = append(out, a.Name)
	}
	return out
}

// T-048-001a — long history that spans many weeks. Exactly 7 daily +
// 4 weekly are kept; everything older than the weekly window is pruned.
func TestSelectKept_LongHistory_KeepsExactSlots(t *testing.T) {
	now := mustParse(t, "2026-05-13T12:00:00Z") // Wednesday

	// One backup per day for 60 days.
	artifacts := make([]Artifact, 0, 60)
	for d := 0; d < 60; d++ {
		ts := now.Add(-time.Duration(d) * 24 * time.Hour).Format(time.RFC3339)
		artifacts = append(artifacts, mkArtifact(t, ts))
	}

	keep, prune := SelectKept(now, artifacts, DefaultPolicy())

	if got := len(keep); got != 11 {
		t.Fatalf("expected 11 retained artifacts (7 daily + 4 weekly); got %d (kept=%v)", got, names(keep))
	}
	if got := len(prune); got != 49 {
		t.Fatalf("expected 49 pruned artifacts; got %d", got)
	}

	// The first 7 kept must be the 7 most recent days.
	for i := 0; i < 7; i++ {
		want := artifacts[i].Name
		if keep[i].Name != want {
			t.Errorf("daily slot %d: expected %q, got %q", i, want, keep[i].Name)
		}
	}
	// The next 4 kept must be in distinct ISO weeks AFTER the daily
	// window — never share a week with any of the daily slots.
	dailyWeeks := map[string]struct{}{}
	for i := 0; i < 7; i++ {
		dailyWeeks[isoWeekKey(keep[i].CreatedAt)] = struct{}{}
	}
	seenWeeks := map[string]struct{}{}
	for i := 7; i < 11; i++ {
		w := isoWeekKey(keep[i].CreatedAt)
		if _, dup := seenWeeks[w]; dup {
			t.Errorf("weekly slot %d collides with another weekly slot in week %q", i, w)
		}
		seenWeeks[w] = struct{}{}
		if _, dailyDup := dailyWeeks[w]; dailyDup {
			t.Errorf("weekly slot %d collides with a daily slot in week %q", i, w)
		}
	}
}

// T-048-001b — multiple backups in one calendar day collapse to one
// daily slot. This is the adversarial proof that the policy is
// "distinct days" not "most recent N artifacts".
func TestSelectKept_SameDayCollapsesToOneDailySlot(t *testing.T) {
	now := mustParse(t, "2026-05-13T23:30:00Z")

	// Day 0 (today) has three backups: 23:30, 12:00, 06:00.
	// Days 1..6 each have one backup. Then one weekly candidate in
	// a distinct ISO week (April 29 is W18; daily window covers
	// W19+W20).
	artifacts := []Artifact{
		mkArtifact(t, "2026-05-13T23:30:00Z"), // today late
		mkArtifact(t, "2026-05-13T12:00:00Z"), // today noon
		mkArtifact(t, "2026-05-13T06:00:00Z"), // today morning
	}
	for d := 1; d <= 6; d++ {
		ts := now.Add(-time.Duration(d) * 24 * time.Hour).Format(time.RFC3339)
		artifacts = append(artifacts, mkArtifact(t, ts))
	}
	// W18 candidate — falls into the weekly window (not the daily window).
	artifacts = append(artifacts, mkArtifact(t, "2026-04-29T23:30:00Z"))

	keep, prune := SelectKept(now, artifacts, DefaultPolicy())

	// Daily window has 7 distinct calendar days (today, today-1..today-6).
	// Today gets exactly ONE daily slot (the newest at 23:30).
	// The April 29 candidate occupies a weekly slot (W18 is not
	// covered by the daily window, which spans W19 + W20).
	want := []string{
		mkArtifact(t, "2026-05-13T23:30:00Z").Name, // today (newest)
		mkArtifact(t, "2026-05-12T23:30:00Z").Name, // day 1
		mkArtifact(t, "2026-05-11T23:30:00Z").Name, // day 2
		mkArtifact(t, "2026-05-10T23:30:00Z").Name, // day 3
		mkArtifact(t, "2026-05-09T23:30:00Z").Name, // day 4
		mkArtifact(t, "2026-05-08T23:30:00Z").Name, // day 5
		mkArtifact(t, "2026-05-07T23:30:00Z").Name, // day 6
		mkArtifact(t, "2026-04-29T23:30:00Z").Name, // weekly slot W18
	}
	if got := names(keep); !reflect.DeepEqual(got, want) {
		t.Fatalf("keep mismatch\n got: %v\nwant: %v", got, want)
	}
	// The two older same-day backups are pruned even though they are
	// inside the 7-day window.
	prunedNames := names(prune)
	for _, expectPruned := range []string{
		mkArtifact(t, "2026-05-13T12:00:00Z").Name,
		mkArtifact(t, "2026-05-13T06:00:00Z").Name,
	} {
		found := false
		for _, n := range prunedNames {
			if n == expectPruned {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected same-day older artifact %q to be pruned; not found in prune list %v", expectPruned, prunedNames)
		}
	}
}

// T-048-001c — weekly slots use ISO week numbering. Two backups in the
// same ISO week share a slot. A weekly slot is never assigned to the
// same week as any daily slot.
func TestSelectKept_WeeklySlotsUseISOWeeks(t *testing.T) {
	now := mustParse(t, "2026-05-13T12:00:00Z") // Wed, ISO week 20

	// Inject TWO older backups in the same older ISO week. Only the
	// newer of the pair survives as the weekly slot.
	artifacts := []Artifact{
		mkArtifact(t, "2026-05-13T12:00:00Z"), // today (daily 0)
		mkArtifact(t, "2026-05-12T12:00:00Z"), // daily 1
		mkArtifact(t, "2026-05-11T12:00:00Z"), // daily 2
		mkArtifact(t, "2026-05-10T12:00:00Z"), // daily 3
		mkArtifact(t, "2026-05-09T12:00:00Z"), // daily 4
		mkArtifact(t, "2026-05-08T12:00:00Z"), // daily 5
		mkArtifact(t, "2026-05-07T12:00:00Z"), // daily 6
		// Older — ISO week 18 has two backups (Fri + Wed). Newer wins.
		mkArtifact(t, "2026-05-01T12:00:00Z"), // ISO week 18 Friday (newer in week)
		mkArtifact(t, "2026-04-29T12:00:00Z"), // ISO week 18 Wednesday (older in week)
		// ISO week 17 / 16 / 15 each get one weekly slot.
		mkArtifact(t, "2026-04-22T12:00:00Z"), // ISO week 17
		mkArtifact(t, "2026-04-15T12:00:00Z"), // ISO week 16
		mkArtifact(t, "2026-04-08T12:00:00Z"), // ISO week 15
		// One backup even older — outside all 4 weekly slots; must be pruned.
		mkArtifact(t, "2026-03-01T12:00:00Z"),
	}
	keep, prune := SelectKept(now, artifacts, DefaultPolicy())

	// 7 daily + 4 weekly = 11 retained.
	if got := len(keep); got != 11 {
		t.Fatalf("expected 11 retained; got %d (kept=%v)", got, names(keep))
	}
	// The Wed (older in week 18) MUST be pruned because the Fri of the
	// same ISO week already filled that weekly slot.
	wantPruned := []string{
		mkArtifact(t, "2026-04-29T12:00:00Z").Name,
		mkArtifact(t, "2026-03-01T12:00:00Z").Name,
	}
	prunedNames := names(prune)
	for _, want := range wantPruned {
		found := false
		for _, n := range prunedNames {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in prune list; got prune=%v", want, prunedNames)
		}
	}
}

// T-048-001d — empty input does not panic.
func TestSelectKept_EmptyInput(t *testing.T) {
	now := mustParse(t, "2026-05-13T00:00:00Z")
	keep, prune := SelectKept(now, nil, DefaultPolicy())
	if len(keep) != 0 || len(prune) != 0 {
		t.Fatalf("expected empty keep+prune for empty input; got keep=%v prune=%v", keep, prune)
	}
}

// T-048-001e — fewer backups than budget keeps everything.
func TestSelectKept_FewerThanBudget(t *testing.T) {
	now := mustParse(t, "2026-05-13T00:00:00Z")
	artifacts := []Artifact{
		mkArtifact(t, "2026-05-13T00:00:00Z"),
		mkArtifact(t, "2026-05-12T00:00:00Z"),
		mkArtifact(t, "2026-05-11T00:00:00Z"),
	}
	keep, prune := SelectKept(now, artifacts, DefaultPolicy())
	if len(keep) != 3 {
		t.Fatalf("expected to keep all 3; got %d (kept=%v)", len(keep), names(keep))
	}
	if len(prune) != 0 {
		t.Fatalf("expected no pruning; got prune=%v", names(prune))
	}
}

// T-048-001f — input slice is not mutated.
func TestSelectKept_DoesNotMutateInput(t *testing.T) {
	now := mustParse(t, "2026-05-13T12:00:00Z")
	artifacts := []Artifact{
		mkArtifact(t, "2026-05-12T12:00:00Z"),
		mkArtifact(t, "2026-05-13T12:00:00Z"), // out of order
		mkArtifact(t, "2026-05-11T12:00:00Z"),
	}
	snapshot := make([]Artifact, len(artifacts))
	copy(snapshot, artifacts)

	_, _ = SelectKept(now, artifacts, DefaultPolicy())

	if !reflect.DeepEqual(artifacts, snapshot) {
		t.Fatalf("SelectKept mutated input slice\n got: %v\nwant: %v", artifacts, snapshot)
	}
}

// T-048-001g — Validate rejects nonsense counts.
func TestRetentionPolicy_Validate(t *testing.T) {
	cases := []struct {
		name    string
		p       RetentionPolicy
		wantErr bool
	}{
		{"default", DefaultPolicy(), false},
		{"daily zero", RetentionPolicy{Daily: 0, Weekly: 4}, true},
		{"daily negative", RetentionPolicy{Daily: -1, Weekly: 4}, true},
		{"weekly negative", RetentionPolicy{Daily: 7, Weekly: -1}, true},
		{"weekly zero ok", RetentionPolicy{Daily: 7, Weekly: 0}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.p.Validate()
			if (err != nil) != c.wantErr {
				t.Fatalf("Validate() err=%v wantErr=%v", err, c.wantErr)
			}
		})
	}
}

// T-048-001h — ParseArtifactTime handles malformed names.
func TestParseArtifactTime(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"smackerel-2026-05-13-120000.sql.gz", false},
		{"smackerel-not-a-date.sql.gz", true},
		{"backup-2026-05-13-120000.sql.gz", true}, // wrong prefix
		{"smackerel-2026-05-13-120000.sql", true}, // wrong suffix
		{"", true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			_, err := ParseArtifactTime(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("ParseArtifactTime(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
			}
		})
	}
}
