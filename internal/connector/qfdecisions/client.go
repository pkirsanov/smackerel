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
	// QF-declared bounds. Until a compatible capability is recorded, event
	// polling fails before any HTTP request is issued.
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
// requested page size to the QF-declared [min_page_size, max_page_size]
// bounds. The connector calls this only after a successful compatible
// handshake; incompatible or unfetched status blocks polling.
//
// Passing capability = nil resets the client to the unfetched state, in which
// FetchDecisionEvents fails before transport. Thread-safe.
func (c *Client) SetCapability(capability *QFBridgeCapability, status string) {
	c.capabilityMu.Lock()
	c.capability = capability
	c.capabilityStatus = status
	c.capabilityMu.Unlock()
}

// effectivePageSize returns the page size FetchDecisionEvents should send on
// the next request. It only succeeds after a compatible capability has been
// recorded; missing or incompatible capability blocks polling before transport.
func (c *Client) effectivePageSize() (int, error) {
	c.capabilityMu.RLock()
	capability := c.capability
	status := c.capabilityStatus
	c.capabilityMu.RUnlock()

	if status != CapabilityStatusCompatible {
		return 0, CapabilityUnavailableError{Status: status, Reason: "capability status is not compatible"}
	}
	if capability == nil {
		return 0, CapabilityUnavailableError{Status: CapabilityStatusUnfetched, Reason: "capability response is missing"}
	}
	if capability.MinPageSize < 1 || capability.MaxPageSize < 1 || capability.MinPageSize > capability.MaxPageSize {
		return 0, CapabilityUnavailableError{
			Status: status,
			Reason: fmt.Sprintf("invalid capability page-size range min=%d max=%d", capability.MinPageSize, capability.MaxPageSize),
		}
	}
	pageSize := c.pageSize
	clamped := c.ClampPageSize(pageSize, capability.MinPageSize, capability.MaxPageSize)
	if clamped != pageSize {
		slog.Warn("qf-decisions: configured page_size outside capability range, clamping",
			slog.Int("configured_page_size", pageSize),
			slog.Int("capability_min_page_size", capability.MinPageSize),
			slog.Int("capability_max_page_size", capability.MaxPageSize),
			slog.Int("clamped_page_size", clamped),
		)
	}
	return clamped, nil
}

func (c *Client) Validate(ctx context.Context) error {
	_, err := c.FetchDecisionEvents(ctx, "")
	return err
}

func (c *Client) FetchDecisionEvents(ctx context.Context, cursor string) (DecisionEventsResponse, error) {
	requested, err := c.effectivePageSize()
	if err != nil {
		return DecisionEventsResponse{}, err
	}
	response, err := c.fetchDecisionEventsAttempt(ctx, cursor, requested)
	if err == nil {
		return response, nil
	}

	var oor PageSizeOutOfRangeError
	if errors.As(err, &oor) {
		slog.Warn("qf-decisions: page_size_out_of_range",
			slog.String("event", "page_size_out_of_range"),
			slog.Int("requested_page_size", requested),
			slog.String("qf_error_message", oor.Message),
		)
	}
	return DecisionEventsResponse{}, err
}

// fetchDecisionEventsAttempt issues exactly one GET /decision-events request
// with the supplied page size.
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

	// PAGE_SIZE_OUT_OF_RANGE is surfaced as a typed error so the connector can
	// mark degraded and emit the operator alert metric without retrying with a
	// guessed local limit. QF may return this on 400 OR 422 depending on which
	// validator catches the violation.
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

type CapabilityUnavailableError struct {
	Status string
	Reason string
}

func (e CapabilityUnavailableError) Error() string {
	status := strings.TrimSpace(e.Status)
	if status == "" {
		status = CapabilityStatusUnfetched
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "compatible persisted capability is required before polling"
	}
	return fmt.Sprintf("QF capability unavailable for decision-events polling: status=%s reason=%s", status, reason)
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
// bounds. FetchDecisionEvents returns this error without retrying the same
// sync cycle with any guessed, smaller, or hardcoded limit.
type PageSizeOutOfRangeError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e PageSizeOutOfRangeError) Error() string {
	return fmt.Sprintf("QF page size out of range with status %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

// ClampPageSize returns the request page size clamped to the successfully
// fetched capability bounds.
func (c *Client) ClampPageSize(requested, capabilityMin, capabilityMax int) int {
	if requested < capabilityMin {
		return capabilityMin
	}
	if requested > capabilityMax {
		return capabilityMax
	}
	return requested
}
