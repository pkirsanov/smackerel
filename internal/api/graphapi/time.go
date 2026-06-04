package graphapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TimeArtifact is one artifact entry inside a TimeDay group.
type TimeArtifact struct {
	ArtifactID string    `json:"artifactId"`
	Title      string    `json:"title"`
	CapturedAt time.Time `json:"capturedAt"`
}

// TimeDay is one calendar-day bucket in the /api/time response.
// Date is the UTC calendar date in YYYY-MM-DD form.
type TimeDay struct {
	Date      string         `json:"date"`
	Artifacts []TimeArtifact `json:"artifacts"`
}

type timeResponse struct {
	Days []TimeDay `json:"days"`
}

// TimeSource is the data-access boundary used by TimeHandlers. The
// window contract is inclusive-start, exclusive-end: from <= ts < to.
type TimeSource interface {
	ArtifactsInWindow(ctx context.Context, from, to time.Time) ([]TimeArtifact, error)
}

// TimeHandlers serves GET /api/time.
type TimeHandlers struct {
	Source TimeSource
	Limits Limits
}

// NewTimeHandlers wires a TimeHandlers against a live pgx pool.
func NewTimeHandlers(pool *pgxpool.Pool, limits Limits) *TimeHandlers {
	return &TimeHandlers{Source: &pgxTimeSource{pool: pool}, Limits: limits}
}

// GetTime handles GET /api/time. Both `from` and `to` are REQUIRED
// (NO-DEFAULTS): missing either parameter returns 400 missing_param
// (SCN-080-13). Window over Limits.TimeWindowMaxDays returns 400
// invalid_window (SCN-080-12).
func (h *TimeHandlers) GetTime(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fromRaw := q.Get("from")
	toRaw := q.Get("to")
	if fromRaw == "" {
		WriteAPIError(w, ErrMissingParam.WithField("from"))
		return
	}
	if toRaw == "" {
		WriteAPIError(w, ErrMissingParam.WithField("to"))
		return
	}
	from, err := time.Parse(time.RFC3339, fromRaw)
	if err != nil {
		WriteError(w, http.StatusBadRequest, CodeMissingParam, "from",
			"from must be an RFC3339 timestamp (e.g. 2026-05-01T00:00:00Z)")
		return
	}
	to, err := time.Parse(time.RFC3339, toRaw)
	if err != nil {
		WriteError(w, http.StatusBadRequest, CodeMissingParam, "to",
			"to must be an RFC3339 timestamp (e.g. 2026-05-08T00:00:00Z)")
		return
	}
	if !to.After(from) {
		WriteError(w, http.StatusBadRequest, CodeInvalidWindow, "window",
			"to must be strictly after from")
		return
	}
	maxDays := h.Limits.TimeWindowMaxDays
	if maxDays <= 0 {
		// Programming error: Limits not populated via SST loader.
		WriteAPIError(w, ErrTimeRangeTooLarge)
		return
	}
	if to.Sub(from) > time.Duration(maxDays)*24*time.Hour {
		WriteError(w, http.StatusBadRequest, CodeInvalidWindow, "window",
			"time window exceeds the configured maximum of "+itoa(maxDays)+" days")
		return
	}

	rows, qerr := h.Source.ArtifactsInWindow(r.Context(), from, to)
	if qerr != nil {
		slog.Error("graphapi: time query failed", "err", qerr)
		WriteError(w, http.StatusInternalServerError, "internal_error", "", "failed to load time window")
		return
	}
	writeJSON(w, http.StatusOK, timeResponse{Days: groupByDayUTC(rows)})
}

// groupByDayUTC buckets artifacts by their UTC calendar date and
// returns the buckets in ascending date order, with each bucket's
// artifacts in capturedAt-ascending order. The day key is YYYY-MM-DD.
func groupByDayUTC(rows []TimeArtifact) []TimeDay {
	if len(rows) == 0 {
		return []TimeDay{}
	}
	idx := map[string]int{}
	out := []TimeDay{}
	for _, r := range rows {
		date := r.CapturedAt.UTC().Format("2006-01-02")
		i, ok := idx[date]
		if !ok {
			out = append(out, TimeDay{Date: date, Artifacts: []TimeArtifact{}})
			i = len(out) - 1
			idx[date] = i
		}
		out[i].Artifacts = append(out[i].Artifacts, r)
	}
	// Date strings are zero-padded, so lexicographic == chronological.
	sortTimeDays(out)
	return out
}

func sortTimeDays(days []TimeDay) {
	// Simple insertion sort — typical N is small (≤ TimeWindowMaxDays).
	for i := 1; i < len(days); i++ {
		j := i
		for j > 0 && days[j-1].Date > days[j].Date {
			days[j-1], days[j] = days[j], days[j-1]
			j--
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// pgxTimeSource is the production TimeSource backed by Postgres.
type pgxTimeSource struct{ pool *pgxpool.Pool }

// artifactsInWindowSQL — inclusive start, exclusive end per design §3.
// Soft cap LIMIT 5000 keeps a single /api/time call bounded; the
// cursor-based slice is a Scope 04 follow-up (existing scope-02 list
// endpoints are the cursor-paginated surface).
const artifactsInWindowSQL = `
SELECT id, COALESCE(title, id), created_at
  FROM artifacts
 WHERE created_at >= $1 AND created_at < $2
 ORDER BY created_at ASC, id ASC
 LIMIT 5000
`

func (s *pgxTimeSource) ArtifactsInWindow(ctx context.Context, from, to time.Time) ([]TimeArtifact, error) {
	if s.pool == nil {
		return nil, errors.New("graphapi: time pool is nil")
	}
	rows, err := s.pool.Query(ctx, artifactsInWindowSQL, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TimeArtifact, 0, 256)
	for rows.Next() {
		var a TimeArtifact
		if err := rows.Scan(&a.ArtifactID, &a.Title, &a.CapturedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
