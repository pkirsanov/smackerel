package graphapi

// Limits is the runtime envelope governing list / edges page sizes
// and time-window clamps for the spec 080 endpoints. All values are
// sourced from SST (config/smackerel.yaml → knowledge_graph_api.*)
// via fail-loud LookupEnv; the zero value is not a valid Limits.
type Limits struct {
	ListDefault       int
	ListMax           int
	EdgesDefault      int
	EdgesMax          int
	TimeWindowMaxDays int
}

// ClampLimit applies the list-endpoint limit clamp described by
// design.md §6 + SCN-080-15. Behavior:
//
//   - req <= 0  → ListDefault (caller did not provide a limit)
//   - req > ListMax → ErrLimitExceeded (rejected, not silently capped,
//     so the client learns it asked for too many)
//   - otherwise → req
//
// Returning ErrLimitExceeded matches the SCN-080-15 use case: the
// caller's intent (10 000 items) is wrong and must be surfaced, not
// silently truncated to 200.
func (l Limits) ClampLimit(req int) (int, error) {
	if l.ListMax <= 0 || l.ListDefault <= 0 {
		// Programming error — Limits was constructed without going
		// through the SST loader. Fail loud rather than return an
		// invented value.
		return 0, ErrLimitExceeded
	}
	if req <= 0 {
		return l.ListDefault, nil
	}
	if req > l.ListMax {
		return 0, ErrLimitExceeded
	}
	return req, nil
}

// ClampEdgesLimit applies the same clamp to the edges endpoint using
// EdgesDefault / EdgesMax.
func (l Limits) ClampEdgesLimit(req int) (int, error) {
	if l.EdgesMax <= 0 || l.EdgesDefault <= 0 {
		return 0, ErrLimitExceeded
	}
	if req <= 0 {
		return l.EdgesDefault, nil
	}
	if req > l.EdgesMax {
		return 0, ErrLimitExceeded
	}
	return req, nil
}
