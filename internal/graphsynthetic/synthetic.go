package graphsynthetic

// synthetic.go — the product-owned Knowledge Graph read synthetic.
//
// It exercises the canonical eight-family route manifest over
// PRODUCTION HTTP behavior using a real scoped session, in the fixed
// order design.md §"Synthetic Contract" prescribes:
//
//	1. topics list, then one topic detail when seeded
//	2. people list, then one person detail when seeded
//	3. places list, then one place detail when seeded
//	4. time, over the explicit bounded UTC window
//	5. edges, for the explicit seeded source
//
// Every request is a GET. There is no create, update, delete, refresh,
// or sync path in this file, so the synthetic is READ-ONLY by
// construction rather than by convention.
//
// ACCEPTANCE (SCN-080-001-03): a 401, 403, 404, 5xx, schema, cursor, or
// missing-row outcome fails the family, and a failed REQUIRED family
// makes the aggregate unavailable. The aggregate becomes available ONLY
// from contract-valid populated or explicitly-permitted empty reads.
//
// VALUE SAFETY (SCN-080-001-07): a seed id read from a list row is held
// in a LOCAL variable, used only to build the next request URL, and
// never written into a result, a log, a span attribute, or a metric
// label. Response bodies are decoded into local structs and discarded;
// nothing derived from a body except its contract validity and row
// presence ever leaves this file.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/api/graphapi"
)

// maxBodyBytes bounds how much of a response the synthetic will read
// before declaring a schema failure. It protects the synthetic from an
// unbounded body without ever retaining the bytes.
const maxBodyBytes = 4 << 20 // 4 MiB

// Synthetic is the product-owned read synthetic. Construct it with New.
type Synthetic struct {
	cfg      Config
	observer Observer
	now      func() time.Time
}

// New validates the configuration and returns a runnable synthetic. It
// fails loud on an invalid configuration and on a nil observer — the
// caller must choose NopObserver explicitly rather than receive silent
// telemetry suppression.
func New(cfg Config, observer Observer) (*Synthetic, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if observer == nil {
		return nil, fmt.Errorf("[%s] Observer is required; pass NopObserver{} to run without telemetry", CodeConfigInvalid)
	}
	return &Synthetic{cfg: cfg, observer: observer, now: func() time.Time { return time.Now().UTC() }}, nil
}

// Run executes one fixed-order observation under the supplied explicit
// activation policy and returns the validated aggregate.
//
// When the activation policy is DISABLED the synthetic performs NO
// read: it emits one `disabled` row per canonical family and a
// `policy_disabled` aggregate. That is a truthful non-ready result and
// a valid deployment state — never a fault and never a boot refusal.
//
// When the activation policy is ENABLED, a typed 503
// `capability_disabled` response is a CONTRADICTION between policy and
// runtime, so it fails the family rather than masquerading as a
// disabled state.
func (s *Synthetic) Run(ctx context.Context, activation graphapi.Activation) (AggregateResult, error) {
	s.observer.ObserveActivation(activation)

	started := s.now()
	var rows []GraphFamilyResult

	if activation.Disabled() {
		for _, family := range graphapi.RequiredGraphFamilies() {
			rows = append(rows, newFamilyResult(family, StateDisabled, CodePolicyDisabled, 0))
		}
	} else {
		rows = s.readAllFamilies(ctx)
	}

	for _, row := range rows {
		if err := row.Validate(); err != nil {
			return AggregateResult{}, err
		}
		s.observer.ObserveFamilyRead(row)
	}

	result := aggregate(activation.State, s.cfg.OptionalFamilies, rows, s.now(), s.now().Sub(started))
	if err := result.Validate(); err != nil {
		return AggregateResult{}, err
	}
	s.observer.ObserveAggregate(result)
	return result, nil
}

// readAllFamilies performs the fixed family sequence and returns one
// row per canonical family, in canonical order.
func (s *Synthetic) readAllFamilies(ctx context.Context) []GraphFamilyResult {
	byFamily := make(map[graphapi.GraphRouteFamily]GraphFamilyResult, 8)
	seedIDs := make(map[graphapi.GraphRouteFamily]string, 3)

	// 1..3 — the three list families and their detail reads, in order.
	listPairs := []struct {
		list       graphapi.GraphRouteFamily
		detail     graphapi.GraphRouteFamily
		listPath   string
		detailPath string
	}{
		{graphapi.FamilyTopics, graphapi.FamilyTopicDetail, "/api/topics/", "/api/topics/"},
		{graphapi.FamilyPeople, graphapi.FamilyPersonDetail, "/api/people/", "/api/people/"},
		{graphapi.FamilyPlaces, graphapi.FamilyPlaceDetail, "/api/places/", "/api/places/"},
	}
	for _, pair := range listPairs {
		listRow, seedID := s.readList(ctx, pair.list, pair.listPath)
		byFamily[pair.list] = listRow
		if seedID != "" {
			seedIDs[pair.list] = seedID
		}
		byFamily[pair.detail] = s.readDetail(ctx, pair.detail, pair.detailPath, listRow, seedID)
	}

	// 4 — time, over the explicit bounded UTC window.
	byFamily[graphapi.FamilyTime] = s.readTime(ctx)

	// 5 — edges, for the explicit seeded source.
	byFamily[graphapi.FamilyEdges] = s.readEdges(ctx, seedIDs[s.cfg.edgeSeedFamily()])

	ordered := make([]GraphFamilyResult, 0, 8)
	for _, family := range graphapi.RequiredGraphFamilies() {
		ordered = append(ordered, byFamily[family])
	}
	return ordered
}

// readList performs one list-family read and returns the family result
// plus the first row's id. The returned id is a LOCAL seed for the next
// request URL only; it is never placed in a result.
func (s *Synthetic) readList(ctx context.Context, family graphapi.GraphRouteFamily, path string) (GraphFamilyResult, string) {
	body, state, code, elapsed := s.get(ctx, family, path, nil)
	if state == StateFailed {
		return newFamilyResult(family, state, code, elapsed), ""
	}

	var envelope struct {
		Items      []json.RawMessage `json:"items"`
		NextCursor *string           `json:"nextCursor"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return newFamilyResult(family, StateFailed, CodeSchemaInvalid, elapsed), ""
	}
	// The list contract is `{"items":[...],"nextCursor":"..."}`. A body
	// without the items array, or with a null/absent nextCursor field,
	// is a schema failure rather than an "empty" pass.
	if envelope.Items == nil || envelope.NextCursor == nil {
		return newFamilyResult(family, StateFailed, CodeSchemaInvalid, elapsed), ""
	}
	if len(envelope.Items) == 0 {
		return newFamilyResult(family, s.emptyState(family), s.emptyCode(family), elapsed), ""
	}

	seedID, err := rowID(envelope.Items[0])
	if err != nil {
		return newFamilyResult(family, StateFailed, CodeSchemaInvalid, elapsed), ""
	}
	return newFamilyResult(family, StatePopulated, CodeOK, elapsed), seedID
}

// readDetail performs one detail-family read for the seed id produced
// by its list family.
//
// When the list family read a policy-permitted true-empty there is no
// row to fetch, so the detail family inherits that permitted emptiness
// only when it is ITSELF named in AllowEmptyFamilies. When the list
// family failed, the detail family fails with the same class. When the
// list family was populated but its seed row cannot be fetched, that is
// a MISSING-ROW failure.
func (s *Synthetic) readDetail(
	ctx context.Context,
	family graphapi.GraphRouteFamily,
	basePath string,
	listRow GraphFamilyResult,
	seedID string,
) GraphFamilyResult {
	switch listRow.State {
	case StateFailed:
		return newFamilyResult(family, StateFailed, listRow.Code, 0)
	case StateTrueEmpty:
		return newFamilyResult(family, s.emptyState(family), s.emptyCode(family), 0)
	}
	if seedID == "" {
		return newFamilyResult(family, StateFailed, CodeRowMissing, 0)
	}

	body, state, code, elapsed := s.get(ctx, family, basePath+url.PathEscape(seedID), nil)
	if state == StateFailed {
		// A populated list whose own first row 404s is a missing row,
		// not an absent route.
		if code == CodeRouteAbsent {
			code = CodeRowMissing
		}
		return newFamilyResult(family, StateFailed, code, elapsed)
	}

	id, err := rowID(body)
	if err != nil || id == "" {
		return newFamilyResult(family, StateFailed, CodeSchemaInvalid, elapsed)
	}
	if id != seedID {
		return newFamilyResult(family, StateFailed, CodeRowMissing, elapsed)
	}
	return newFamilyResult(family, StatePopulated, CodeOK, elapsed)
}

// readTime performs the time-family read over the explicit bounded UTC
// window from the configuration.
func (s *Synthetic) readTime(ctx context.Context) GraphFamilyResult {
	family := graphapi.FamilyTime
	query := url.Values{}
	query.Set("from", s.cfg.WindowFrom.Format(time.RFC3339))
	query.Set("to", s.cfg.WindowTo.Format(time.RFC3339))

	body, state, code, elapsed := s.get(ctx, family, "/api/time", query)
	if state == StateFailed {
		return newFamilyResult(family, state, code, elapsed)
	}

	var envelope struct {
		Days []struct {
			Date      *string           `json:"date"`
			Artifacts []json.RawMessage `json:"artifacts"`
		} `json:"days"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Days == nil {
		return newFamilyResult(family, StateFailed, CodeSchemaInvalid, elapsed)
	}
	for _, day := range envelope.Days {
		if day.Date == nil || strings.TrimSpace(*day.Date) == "" {
			return newFamilyResult(family, StateFailed, CodeSchemaInvalid, elapsed)
		}
	}
	if len(envelope.Days) == 0 {
		return newFamilyResult(family, s.emptyState(family), s.emptyCode(family), elapsed)
	}
	return newFamilyResult(family, StatePopulated, CodeOK, elapsed)
}

// readEdges performs the edges-family read for the explicit seeded
// source. The seed id arrives from the configured seed list family and
// is used ONLY to build the `source=kind:id` query value.
func (s *Synthetic) readEdges(ctx context.Context, seedID string) GraphFamilyResult {
	family := graphapi.FamilyEdges
	if seedID == "" {
		// No seed row exists. That is a permitted true-empty only when
		// the edges family is explicitly named in AllowEmptyFamilies;
		// otherwise the synthetic refuses to pass an unexercised family.
		if s.cfg.allowsEmpty(family) {
			return newFamilyResult(family, StateTrueEmpty, CodeEmptyPermitted, 0)
		}
		return newFamilyResult(family, StateFailed, CodeRowMissing, 0)
	}

	query := url.Values{}
	query.Set("source", s.cfg.EdgeSourceKind+":"+seedID)

	body, state, code, elapsed := s.get(ctx, family, "/api/graph/edges", query)
	if state == StateFailed {
		return newFamilyResult(family, state, code, elapsed)
	}

	var envelope struct {
		Items      []json.RawMessage `json:"items"`
		NextCursor *string           `json:"nextCursor"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return newFamilyResult(family, StateFailed, CodeSchemaInvalid, elapsed)
	}
	if envelope.Items == nil || envelope.NextCursor == nil {
		return newFamilyResult(family, StateFailed, CodeSchemaInvalid, elapsed)
	}
	if len(envelope.Items) == 0 {
		return newFamilyResult(family, s.emptyState(family), s.emptyCode(family), elapsed)
	}
	return newFamilyResult(family, StatePopulated, CodeOK, elapsed)
}

// get issues one bounded, authenticated GET and classifies the outcome
// against the closed vocabulary. It returns the response body ONLY for
// a 200; every other status yields StateFailed with a closed code.
//
// The returned error class never names the target host, the request
// URL, the bearer token, or the response body.
func (s *Synthetic) get(
	ctx context.Context,
	family graphapi.GraphRouteFamily,
	path string,
	query url.Values,
) (body []byte, state ReadState, code string, elapsed time.Duration) {
	target := strings.TrimSuffix(s.cfg.BaseURL, "/") + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	reqCtx, cancel := context.WithTimeout(ctx, s.cfg.RequestTimeout)
	defer cancel()

	started := s.now()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	if err != nil {
		return nil, StateFailed, CodeTransport, s.now().Sub(started)
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.BearerToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, StateFailed, CodeTransport, s.now().Sub(started)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	elapsed = s.now().Sub(started)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, StateFailed, CodeTransport, elapsed
	}

	if resp.StatusCode == http.StatusOK {
		return raw, StatePopulated, CodeOK, elapsed
	}
	return nil, StateFailed, classifyStatus(resp.StatusCode, raw), elapsed
}

// classifyStatus maps a non-200 response to a closed value-safe code.
// It reads ONLY the typed graphapi error code from the envelope — never
// the message, the field, or any other body content.
func classifyStatus(status int, raw []byte) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	typed := ""
	if err := json.Unmarshal(raw, &envelope); err == nil {
		typed = envelope.Error.Code
	}

	switch status {
	case http.StatusUnauthorized:
		return CodeUnauthenticated
	case http.StatusForbidden:
		return CodeForbidden
	case http.StatusNotFound:
		return CodeRouteAbsent
	case http.StatusBadRequest:
		if typed == graphapi.CodeInvalidCursor {
			return CodeCursorInvalid
		}
		return CodeSchemaInvalid
	case http.StatusServiceUnavailable:
		switch typed {
		case graphapi.CodeCapabilityDisabled:
			return CodeCapabilityDisabled
		case graphapi.CodeStoreUnavailable:
			return CodeStoreUnavailable
		}
		return CodeServerError
	}
	if status >= 500 {
		if typed == graphapi.CodeSchemaError {
			return CodeSchemaInvalid
		}
		return CodeServerError
	}
	return CodeUnexpectedStatus
}

// emptyState resolves a zero-row read to its policy outcome: a
// permitted true-empty, or a failure when the family is not explicitly
// named in AllowEmptyFamilies.
func (s *Synthetic) emptyState(family graphapi.GraphRouteFamily) ReadState {
	if s.cfg.allowsEmpty(family) {
		return StateTrueEmpty
	}
	return StateFailed
}

// emptyCode is the closed code companion to emptyState.
func (s *Synthetic) emptyCode(family graphapi.GraphRouteFamily) string {
	if s.cfg.allowsEmpty(family) {
		return CodeEmptyPermitted
	}
	return CodeEmptyNotPermitted
}

// rowID extracts the `id` field from a graph row. It returns an error
// when the row is not an object or the id is absent or blank — the
// contract-invalid case the caller reports as a schema failure. The
// returned id is a LOCAL request seed and never reaches a result.
func rowID(raw json.RawMessage) (string, error) {
	var row struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return "", err
	}
	if strings.TrimSpace(row.ID) == "" {
		return "", errors.New("graphsynthetic: row is missing its id")
	}
	return row.ID, nil
}

// durationMillis renders a duration for a value-safe log attribute.
func durationMillis(ms int64) string { return strconv.FormatInt(ms, 10) }
