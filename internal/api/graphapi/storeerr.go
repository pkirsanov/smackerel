package graphapi

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// BUG-080-001 — closed read outcomes, `store-unavailable` arm.
//
// design.md §"Closed Read Outcomes" narrows the previously generic
// `internal_error` at the graph boundary: PostgreSQL
// connectivity/timeout failures map to 503 `store_unavailable`, and the
// response MUST never degrade into a 404 (which reads as "resource
// missing"/"route absent") or a 200 with an empty `items` array (which
// every client reads as `true-empty`, i.e. "the graph legitimately holds
// nothing"). A broken datastore must not look like "no data".
//
// classifyStoreError is the single classification point. It returns
// ErrStoreUnavailable when err is a store connectivity/timeout class,
// and nil when the error is NOT attributable to the store, in which case
// the caller keeps its existing (narrower or generic) mapping. It never
// reclassifies row/schema/reason invariants — those keep the
// `schema_error` / `internal_reason_missing` paths untouched.
func classifyStoreError(err error) *APIError {
	if err == nil {
		return nil
	}

	// The store call's context expired or was cancelled — a timeout at
	// the data-access boundary, not a client-input problem.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrStoreUnavailable
	}

	// pgx could not establish a connection at all (DNS failure, refused,
	// unreachable host, TLS handshake failure, auth-time transport error).
	var connectErr *pgconn.ConnectError
	if errors.As(err, &connectErr) {
		return ErrStoreUnavailable
	}

	// pgconn-classified timeout (context deadline or net.Error timeout
	// observed inside pgconn).
	if pgconn.Timeout(err) {
		return ErrStoreUnavailable
	}

	// Transport-level failure surfaced as a net error, or a socket that
	// was closed underneath an in-flight query.
	var netErr net.Error
	if errors.As(err, &netErr) || errors.Is(err, net.ErrClosed) {
		return ErrStoreUnavailable
	}

	// Server-reported connectivity/availability SQLSTATE classes:
	//   08*    connection_exception
	//   53*    insufficient_resources (out of memory / too many connections)
	//   57P01  admin_shutdown
	//   57P02  crash_shutdown
	//   57P03  cannot_connect_now
	// Every other SQLSTATE (22*, 23*, 42*, ...) is a data/schema problem
	// and is deliberately NOT reclassified here.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case strings.HasPrefix(pgErr.Code, "08"),
			strings.HasPrefix(pgErr.Code, "53"),
			pgErr.Code == "57P01",
			pgErr.Code == "57P02",
			pgErr.Code == "57P03":
			return ErrStoreUnavailable
		}
	}

	// Acquiring from a pgxpool that has been Close()d surfaces puddle's
	// ErrClosedPool sentinel. puddle is an INDIRECT dependency of this
	// module (go.mod: github.com/jackc/puddle/v2 // indirect); matching
	// its stable sentinel text keeps the dependency graph unchanged
	// rather than promoting puddle to a direct import solely for an
	// errors.Is target. The sentinel text is `closed pool`.
	if msg := err.Error(); msg == closedPoolSentinel ||
		strings.HasSuffix(msg, ": "+closedPoolSentinel) {
		return ErrStoreUnavailable
	}

	return nil
}

// closedPoolSentinel is puddle's ErrClosedPool message, matched by text
// because puddle is an indirect dependency. See classifyStoreError.
const closedPoolSentinel = "closed pool"
