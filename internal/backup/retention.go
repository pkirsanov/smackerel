// Package backup implements spec 048 — Backup and Restore Automation.
//
// This package is project-owned. It defines the pure retention policy
// (FR-048-001: 7 daily + 4 weekly), the on-disk status contract that
// `scripts/commands/backup.sh` writes after each run, and a background
// metrics watcher that exposes the most recent backup outcome via the
// existing internal/metrics surface (so spec 049 alert rules can fire
// on missed backups).
//
// Target adapter responsibilities (out of scope for this package):
//
//   - Installing a systemd / cron timer that invokes `./smackerel.sh backup`
//     on the cadence the adapter wants (typically daily).
//   - Shipping the resulting `backups/*.sql.gz` artifact to the operator's
//     real off-host storage backend (rclone to S3, BackBlaze, NFS, etc.).
//     The product surface stays generic — the destination URL is supplied
//     by the deploy adapter via SST (BACKUP_DESTINATION_URL).
//
// The package contains no IO besides reading the status file and emitting
// Prometheus metrics. The pure retention function (SelectKept) is fully
// unit-tested without disk access.
package backup

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Artifact describes one backup file on disk. The retention policy operates
// on this minimal shape so it can be exercised in unit tests without
// touching the filesystem.
//
// The Name field is expected to follow the `smackerel-YYYY-MM-DD-HHMMSS.sql.gz`
// pattern emitted by `scripts/commands/backup.sh`. The CreatedAt field is
// authoritative — the retention policy never re-parses Name. Callers that
// build the slice from disk MUST set CreatedAt from the file's modtime (or,
// preferably, the timestamp embedded in Name parsed via ParseArtifactTime).
type Artifact struct {
	Name      string
	CreatedAt time.Time
}

// RetentionPolicy is the product-owned retention contract.
//
// FR-048-001: 7 daily + 4 weekly retained backups.
//
// Daily slots are the 7 most recent calendar days that contain at least
// one backup. Weekly slots are the most recent backup in each of the 4
// most recent ISO weeks PRIOR to the daily window (so a 4-week-old weekly
// slot does not collide with last week's daily slot).
//
// A backup that lives in both a daily slot and a weekly slot counts only
// once — the policy keeps the artifact, not a duplicate of it.
type RetentionPolicy struct {
	Daily  int
	Weekly int
}

// DefaultPolicy returns the contract policy: 7 daily, 4 weekly.
// FR-048-001.
func DefaultPolicy() RetentionPolicy {
	return RetentionPolicy{Daily: 7, Weekly: 4}
}

// Validate rejects nonsensical retention counts.
func (p RetentionPolicy) Validate() error {
	if p.Daily < 1 {
		return fmt.Errorf("backup retention daily must be >= 1; got %d", p.Daily)
	}
	if p.Weekly < 0 {
		return fmt.Errorf("backup retention weekly must be >= 0; got %d", p.Weekly)
	}
	return nil
}

// SelectKept partitions the artifact list into the set that satisfies the
// retention policy (keep) and the set that should be pruned (prune).
//
// Ordering: artifacts are sorted by CreatedAt descending (newest first)
// before slot assignment so the most-recent artifact in each calendar day /
// ISO week wins. The returned slices are stable in that order.
//
// `now` is supplied by the caller so tests can pin a deterministic clock.
// The function performs no IO.
//
// Adversarial properties exercised by the unit tests:
//
//   - Exactly Daily distinct calendar days are kept (a day with multiple
//     backups keeps only the latest; older copies the same day are pruned).
//   - Weekly slots count ISO weeks, not 7-day windows, so a Sunday and a
//     Monday backup in the same ISO week share a slot.
//   - A backup that lands in both a daily and a weekly slot is kept once
//     and is NOT double-counted against either budget.
//   - When fewer than Daily+Weekly artifacts exist, all of them are kept
//     (no spurious pruning of a fresh repo).
//   - When the input is empty, both returned slices are empty (no panic).
func SelectKept(now time.Time, artifacts []Artifact, p RetentionPolicy) (keep, prune []Artifact) {
	if len(artifacts) == 0 {
		return nil, nil
	}

	// Defensive copy + newest-first sort. We never mutate the caller's
	// slice ordering.
	sorted := make([]Artifact, len(artifacts))
	copy(sorted, artifacts)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	keptIdx := make(map[int]struct{}, p.Daily+p.Weekly)

	// Daily slots: walk newest-first, claim one artifact per distinct
	// calendar day until we hit Daily slots.
	seenDays := make(map[string]struct{}, p.Daily)
	dailyClaimed := 0
	dailyCutoff := -1 // index of the last artifact whose day was claimed by daily
	for i, a := range sorted {
		if dailyClaimed >= p.Daily {
			break
		}
		dayKey := a.CreatedAt.UTC().Format("2006-01-02")
		if _, seen := seenDays[dayKey]; seen {
			continue
		}
		seenDays[dayKey] = struct{}{}
		keptIdx[i] = struct{}{}
		dailyClaimed++
		dailyCutoff = i
	}

	// Weekly slots: walk forward from after the daily cutoff, claim one
	// artifact per distinct ISO week until we hit Weekly slots. We start
	// after the daily cutoff so a daily-kept artifact's week does NOT
	// consume a weekly slot. A week that the daily window already saw
	// is recorded so the weekly walk does not double-claim it.
	if p.Weekly > 0 {
		seenWeeks := make(map[string]struct{}, p.Weekly)
		for i, a := range sorted {
			if i > dailyCutoff {
				break
			}
			seenWeeks[isoWeekKey(a.CreatedAt)] = struct{}{}
		}
		weeklyClaimed := 0
		for i := dailyCutoff + 1; i < len(sorted) && weeklyClaimed < p.Weekly; i++ {
			weekKey := isoWeekKey(sorted[i].CreatedAt)
			if _, seen := seenWeeks[weekKey]; seen {
				continue
			}
			seenWeeks[weekKey] = struct{}{}
			keptIdx[i] = struct{}{}
			weeklyClaimed++
		}
	}

	keep = make([]Artifact, 0, len(keptIdx))
	prune = make([]Artifact, 0, len(sorted)-len(keptIdx))
	for i, a := range sorted {
		if _, ok := keptIdx[i]; ok {
			keep = append(keep, a)
		} else {
			prune = append(prune, a)
		}
	}
	return keep, prune
}

// isoWeekKey returns "YYYY-WW" using ISO 8601 week numbering. Two
// artifacts that share an ISO week (even across a month or year boundary)
// produce the same key.
func isoWeekKey(t time.Time) string {
	year, week := t.UTC().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

// ParseArtifactTime extracts the creation timestamp encoded in a backup
// filename of the form `smackerel-YYYY-MM-DD-HHMMSS.sql.gz`. Returns an
// error when the name does not match the expected shape, so callers can
// fall back to the file modtime without silently mis-classifying.
//
// The format MUST stay in sync with the TIMESTAMP="$(date -u +%Y-%m-%d-%H%M%S)"
// line in scripts/commands/backup.sh.
func ParseArtifactTime(name string) (time.Time, error) {
	const prefix = "smackerel-"
	const suffix = ".sql.gz"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return time.Time{}, fmt.Errorf("backup artifact name %q does not match expected smackerel-YYYY-MM-DD-HHMMSS.sql.gz form", name)
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	t, err := time.Parse("2006-01-02-150405", stamp)
	if err != nil {
		return time.Time{}, fmt.Errorf("backup artifact name %q has malformed timestamp: %w", name, err)
	}
	return t.UTC(), nil
}
