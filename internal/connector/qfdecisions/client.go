package qfdecisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	clientTimeout = 10 * time.Second

	// defaultUnfetchedPageSize is the page-size value FetchDecisionEvents uses
	// when the QF capability handshake has not yet supplied a max_page_size and
	// the connector did not configure one. Matches the QF spec 063 documented
	// default so an unconfigured client still issues requests QF will accept.
	// This is a documented client-side initial guess, NOT a fallback for
	// missing required configuration — the connector's parseConfig continues
	// to require page_size in source_config.
	defaultUnfetchedPageSize = 200

	// bridgeErrCodePageSizeRange is the QF-side error code surfaced in the
	// BridgeErrorResponse body when the requested page_size falls outside the
	// QF-enforced bounds. Source of truth: QF spec 063 §"GET decision-events"
	// error responses.
	bridgeErrCodePageSizeRange = "PAGE_SIZE_OUT_OF_RANGE"
)

type Client struct {
	baseURL       string
	credentialRef string
	packetVersion int
	pageSize      int
	httpClient    *http.Client

	// capabilityMu guards capability and capabilityStatus, which are set via
	// SetCapability after the connector completes the handshake. Reads happen
	// inside FetchDecisionEvents to clamp the requested page size to the
	// QF-declared bounds. Writes happen exactly once per Connect cycle in
	// production once the connector is wired to call SetCapability; in the
	// interim the fields default to unfetched and FetchDecisionEvents uses the
	// connector-configured page_size verbatim (preserving prior behavior).
	capabilityMu     sync.RWMutex
	capability       *QFBridgeCapability
	capabilityStatus string
}

func NewClient(baseURL, credentialRef string, packetVersion int, pageSize int) *Client {
	return &Client{
		baseURL:       strings.TrimRight(baseURL, "/"),
		credentialRef: credentialRef,
		packetVersion: packetVersion,
		pageSize:      pageSize,
		httpClient:    &http.Client{Timeout: clientTimeout},
	}
}

// SetCapability records the QF capability response and its compatibility
// status on the client so subsequent FetchDecisionEvents calls can clamp the
// requested page size to the QF-declared [1, max_page_size] bounds. The
// connector calls this after a successful handshake (status =
// CapabilityStatusCompatible) AND after a handshake that detected a
// compatibility mismatch (status = CapabilityStatusIncompatible) — the latter
// is recorded so FetchDecisionEvents skips clamping (the connector blocks
// polling at the connector level in that state anyway, but any in-flight
// request before tear-down still uses a sensible value).
//
// Passing capability = nil resets the client to the unfetched state, in which
// FetchDecisionEvents sends the connector-configured page_size verbatim.
// Thread-safe.
func (c *Client) SetCapability(capability *QFBridgeCapability, status string) {
	c.capabilityMu.Lock()
	c.capability = capability
	c.capabilityStatus = status
	c.capabilityMu.Unlock()
}

// effectivePageSize returns the page size FetchDecisionEvents should send on
// the next request, factoring in:
//
//   - The connector-configured page_size (c.pageSize).
//   - The QF-declared capability bounds when the handshake was successful
//     (capabilityStatus == CapabilityStatusCompatible AND capability.MaxPageSize
//     > 0). In that case the configured value is clamped to
//     [1, capability.MaxPageSize].
//   - The documented client-side initial guess (defaultUnfetchedPageSize) when
//     no page size was configured AND the capability has not been fetched.
//
// When the capability is unfetched OR the handshake declared the contract
// incompatible, the configured page size is returned verbatim. Thread-safe.
func (c *Client) effectivePageSize() int {
	c.capabilityMu.RLock()
	capability := c.capability
	status := c.capabilityStatus
	c.capabilityMu.RUnlock()

	pageSize := c.pageSize

	// Capability not usable for clamping — return configured value verbatim,
	// or the documented default if nothing was configured.
	if status != CapabilityStatusCompatible || capability == nil || capability.MaxPageSize <= 0 {
		if pageSize <= 0 {
			return defaultUnfetchedPageSize
		}
		return pageSize
	}

	// Compatible capability — clamp to [1, capability.MaxPageSize].
	if pageSize < 1 {
		slog.Warn("qf-decisions: configured page_size below floor, clamping to 1",
			slog.Int("configured_page_size", pageSize),
			slog.Int("capability_max_page_size", capability.MaxPageSize),
		)
		return 1
	}
	if pageSize > capability.MaxPageSize {
		return capability.MaxPageSize
	}
	return pageSize
}

func (c *Client) Validate(ctx context.Context) error {
	_, err := c.FetchDecisionEvents(ctx, "")
	return err
}

func (c *Client) FetchDecisionEvents(ctx context.Context, cursor string) (DecisionEventsResponse, error) {
	requested := c.effectivePageSize()
	response, err := c.fetchDecisionEventsAttempt(ctx, cursor, requested)
	if err == nil {
		return response, nil
	}

	var oor PageSizeOutOfRangeError
	if !errors.As(err, &oor) {
		return DecisionEventsResponse{}, err
	}

	// QF rejected the requested page size. Compute a retry value clamped to
	// the QF-declared capability bound (when available) and reissue exactly
	// once. If the retry also fails with PAGE_SIZE_OUT_OF_RANGE we surface
	// the error rather than infinite-looping — operators are alerted via the
	// structured WARN log below and the QF-side error message.
	c.capabilityMu.RLock()
	var capMax int
	if c.capability != nil {
		capMax = c.capability.MaxPageSize
	}
	c.capabilityMu.RUnlock()

	retrySize := requested
	if capMax > 0 && (retrySize > capMax || retrySize < 1) {
		retrySize = capMax
	}
	if retrySize < 1 {
		retrySize = 1
	}

	slog.Warn("qf-decisions: page_size_out_of_range",
		slog.String("event", "page_size_out_of_range"),
		slog.Int("requested_page_size", requested),
		slog.Int("capability_max_page_size", capMax),
		slog.Int("retry_page_size", retrySize),
		slog.String("qf_error_message", oor.Message),
	)

	response, retryErr := c.fetchDecisionEventsAttempt(ctx, cursor, retrySize)
	if retryErr == nil {
		return response, nil
	}
	var oor2 PageSizeOutOfRangeError
	if errors.As(retryErr, &oor2) {
		return DecisionEventsResponse{}, fmt.Errorf("qf-decisions: page_size_out_of_range persisted after retry: %w", retryErr)
	}
	return DecisionEventsResponse{}, retryErr
}

// fetchDecisionEventsAttempt issues exactly one GET /decision-events request
// with the supplied page size. Used by FetchDecisionEvents for both the
// initial poll and the post-PAGE_SIZE_OUT_OF_RANGE retry; keeping the
// transport plumbing in one place ensures both attempts share identical
// header, auth, and decode behaviour.
func (c *Client) fetchDecisionEventsAttempt(ctx context.Context, cursor string, pageSize int) (DecisionEventsResponse, error) {
	endpoint, err := c.urlFor(DecisionEventsPath)
	if err != nil {
		return DecisionEventsResponse{}, err
	}
	query := endpoint.Query()
	query.Set("limit", strconv.Itoa(pageSize))
	query.Set("packet_version", strconv.Itoa(c.packetVersion))
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	endpoint.RawQuery = query.Encode()

	var response DecisionEventsResponse
	if err := c.doGet(ctx, endpoint.String(), &response); err != nil {
		return DecisionEventsResponse{}, err
	}
	return response, nil
}

func (c *Client) FetchDecisionPacket(ctx context.Context, packetID string) (QFDecisionPacketEnvelope, error) {
	if strings.TrimSpace(packetID) == "" {
		return QFDecisionPacketEnvelope{}, fmt.Errorf("packet_id is required")
	}
	endpoint, err := c.urlFor(DecisionPacketsPath + "/" + url.PathEscape(packetID))
	if err != nil {
		return QFDecisionPacketEnvelope{}, err
	}
	query := endpoint.Query()
	query.Set("packet_version", strconv.Itoa(c.packetVersion))
	endpoint.RawQuery = query.Encode()

	var response QFDecisionPacketEnvelope
	if err := c.doGet(ctx, endpoint.String(), &response); err != nil {
		return QFDecisionPacketEnvelope{}, err
	}
	return response, nil
}

func (c *Client) urlFor(path string) (*url.URL, error) {
	endpoint, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("build QF request URL: %w", err)
	}
	return endpoint, nil
}

func (c *Client) doGet(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create QF request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.credentialRef)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("QF bridge request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return fmt.Errorf("decode QF bridge response: %w", err)
		}
		return nil
	}

	bridgeErr := BridgeErrorResponse{}
	_ = json.NewDecoder(resp.Body).Decode(&bridgeErr)
	message := bridgeErr.Message
	if message == "" {
		message = resp.Status
	}

	// PAGE_SIZE_OUT_OF_RANGE is surfaced as a typed error so FetchDecisionEvents
	// can retry once with a capability-clamped size. QF may return this on 400
	// OR 422 depending on which validator catches the violation.
	if (resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity) && bridgeErr.Code == bridgeErrCodePageSizeRange {
		return PageSizeOutOfRangeError{
			StatusCode: resp.StatusCode,
			Code:       bridgeErr.Code,
			Message:    message,
		}
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return AuthError{StatusCode: resp.StatusCode, Message: message}
	case http.StatusBadRequest:
		if strings.Contains(strings.ToLower(message), "packet_version") {
			return SchemaCompatibilityError{StatusCode: resp.StatusCode, Message: message}
		}
		return BridgeRequestError{StatusCode: resp.StatusCode, Message: message}
	case http.StatusServiceUnavailable:
		return BridgeUnavailableError{StatusCode: resp.StatusCode, Message: message}
	default:
		return BridgeRequestError{StatusCode: resp.StatusCode, Message: message}
	}
}

type AuthError struct {
	StatusCode int
	Message    string
}

func (e AuthError) Error() string {
	return fmt.Sprintf("QF bridge authorization failed with status %d: %s", e.StatusCode, e.Message)
}

type SchemaCompatibilityError struct {
	StatusCode int
	Message    string
}

func (e SchemaCompatibilityError) Error() string {
	return fmt.Sprintf("QF bridge schema compatibility failed with status %d: %s", e.StatusCode, e.Message)
}

type BridgeUnavailableError struct {
	StatusCode int
	Message    string
}

func (e BridgeUnavailableError) Error() string {
	return fmt.Sprintf("QF bridge unavailable with status %d: %s", e.StatusCode, e.Message)
}

type BridgeRequestError struct {
	StatusCode int
	Message    string
}

func (e BridgeRequestError) Error() string {
	return fmt.Sprintf("QF bridge request failed with status %d: %s", e.StatusCode, e.Message)
}

// PageSizeOutOfRangeError is surfaced when QF rejects a decision-events
// request because the page_size parameter falls outside the QF-enforced
// bounds. FetchDecisionEvents recognises this error and retries the same
// poll exactly once with a capability-clamped page size; persistence after
// retry returns the error to the caller (no infinite loop).
type PageSizeOutOfRangeError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e PageSizeOutOfRangeError) Error() string {
	return fmt.Sprintf("QF page size out of range with status %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

// ClampPageSize returns the request page size clamped to the QF-declared
// capability bounds. When capability is unfetched (max == 0), returns the
// connector-configured default. Caller MUST persist the clamped value into
// the request to avoid PAGE_SIZE_OUT_OF_RANGE rejections from QF.
//
// Round 2C will rewire connector.Sync() to fetch capability first and pass
// max_page_size into this helper before issuing the FetchDecisionEvents call.
func (c *Client) ClampPageSize(requested, capabilityMax int) int {
	if capabilityMax <= 0 {
		return requested
	}
	if requested < 1 {
		return 1
	}
	if requested > capabilityMax {
		return capabilityMax
	}
	return requested
}
