package qfdecisions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const clientTimeout = 10 * time.Second

type Client struct {
	baseURL       string
	credentialRef string
	packetVersion int
	pageSize      int
	httpClient    *http.Client
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

func (c *Client) Validate(ctx context.Context) error {
	_, err := c.FetchDecisionEvents(ctx, "")
	return err
}

func (c *Client) FetchDecisionEvents(ctx context.Context, cursor string) (DecisionEventsResponse, error) {
	endpoint, err := c.urlFor(DecisionEventsPath)
	if err != nil {
		return DecisionEventsResponse{}, err
	}
	query := endpoint.Query()
	query.Set("limit", strconv.Itoa(c.pageSize))
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
