package cardrewards

// Card-rewards Google Calendar write client (spec 097).
//
// This is the production implementation of the CalDAVClient interface that the
// CardCalendarBridge (spec 083 Scope 08) writes through. Despite the historical
// "CalDAV" name on the interface, smackerel's Google integration uses the Google
// Calendar REST API v3 (the existing read path in internal/connector/caldav does
// the same), so this client speaks REST v3 — not the CalDAV protocol.
//
// Idempotency (FR-097-02): every recommendation/re-enrollment carries a STABLE
// application UID (e.g. "smackerel-cardrec-2026-06-restaurants"). Google event
// ids must be base32hex (lowercase a-v + digits) and 5..1024 chars, so the UID —
// which contains hyphens — cannot be used directly. We derive a deterministic,
// always-valid event id as the lowercase hex of sha1(uid) (40 chars, all within
// 0-9a-f ⊂ a-v0-9). PutEvent then GETs that id: a 404 inserts, a 200 updates.
// The same UID therefore always maps to the same calendar event — a re-sync
// updates in place rather than duplicating (SCN-083-H02).
//
// Auth (FR-097-03): the card-rewards calendar credential is an operator-supplied
// Google OAuth2 installed-app credential (client_id, client_secret,
// refresh_token, token_uri) delivered as the SST secret
// CARD_REWARDS_GCAL_CREDENTIALS. Google access tokens expire hourly while the
// sync cron is monthly, so the client mints a fresh access token from the
// refresh token on demand (cached until shortly before expiry within a run).
//
// No secret value is ever logged (FR-097-07): logs carry the event UID and the
// calendar id, never the token or credential fields.

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// defaultGCalAPIBase and defaultGCalTokenURL are the live Google endpoints.
// Both are overridable on the client for httptest-driven unit tests.
const (
	defaultGCalAPIBase  = "https://www.googleapis.com/calendar/v3"
	defaultGCalTokenURL = "https://oauth2.googleapis.com/token"

	// gcalCategoryTag mirrors calendarCategoryTag; the card-rewards bridge passes
	// it through PutEvent's categories slice. We surface it as an extended
	// property so events are filterable in Google Calendar.
	gcalExtPropCategories = "smackerel-categories"
	gcalExtPropUID        = "smackerel-uid"
)

// GCalCredential is the operator-supplied Google OAuth2 installed-app credential
// used to mint access tokens for calendar writes. It is parsed from the
// CARD_REWARDS_GCAL_CREDENTIALS secret JSON. Every field is required.
type GCalCredential struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	// TokenURI is optional in the JSON; it defaults to Google's token endpoint.
	TokenURI string `json:"token_uri,omitempty"`
}

// ParseGCalCredential parses and validates the CARD_REWARDS_GCAL_CREDENTIALS
// secret JSON. It fails loud (no defaults) when a required field is absent so a
// misconfigured deployment is rejected at construction time, not at first write.
func ParseGCalCredential(raw string) (GCalCredential, error) {
	var c GCalCredential
	if strings.TrimSpace(raw) == "" {
		return c, fmt.Errorf("card calendar: CARD_REWARDS_GCAL_CREDENTIALS is empty")
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, fmt.Errorf("card calendar: CARD_REWARDS_GCAL_CREDENTIALS is not valid JSON: %w", err)
	}
	var missing []string
	if strings.TrimSpace(c.ClientID) == "" {
		missing = append(missing, "client_id")
	}
	if strings.TrimSpace(c.ClientSecret) == "" {
		missing = append(missing, "client_secret")
	}
	if strings.TrimSpace(c.RefreshToken) == "" {
		missing = append(missing, "refresh_token")
	}
	if len(missing) > 0 {
		return GCalCredential{}, fmt.Errorf("card calendar: CARD_REWARDS_GCAL_CREDENTIALS missing required field(s): %s", strings.Join(missing, ", "))
	}
	if strings.TrimSpace(c.TokenURI) == "" {
		c.TokenURI = defaultGCalTokenURL
	}
	return c, nil
}

// GoogleCalendarClient implements CalDAVClient against the Google Calendar REST
// API v3 for a single target calendar. It is safe for concurrent use; the cached
// access token is guarded by a mutex.
type GoogleCalendarClient struct {
	calendarID string
	cred       GCalCredential
	httpClient *http.Client

	apiBase  string
	tokenURL string

	mu        sync.Mutex
	cachedTok string
	cachedExp time.Time
}

// NewGoogleCalendarClient constructs the production calendar write client.
// calendarID is the operator's target calendar (e.g. a
// "...@group.calendar.google.com" secondary calendar); cred is the validated
// OAuth credential. It fails loud on an empty calendar id or credential.
func NewGoogleCalendarClient(calendarID string, cred GCalCredential, httpClient *http.Client) (*GoogleCalendarClient, error) {
	if strings.TrimSpace(calendarID) == "" {
		return nil, fmt.Errorf("card calendar: calendar id must be non-empty")
	}
	if strings.TrimSpace(cred.ClientID) == "" || strings.TrimSpace(cred.ClientSecret) == "" || strings.TrimSpace(cred.RefreshToken) == "" {
		return nil, fmt.Errorf("card calendar: incomplete Google OAuth credential")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	tokenURL := cred.TokenURI
	if strings.TrimSpace(tokenURL) == "" {
		tokenURL = defaultGCalTokenURL
	}
	return &GoogleCalendarClient{
		calendarID: calendarID,
		cred:       cred,
		httpClient: httpClient,
		apiBase:    defaultGCalAPIBase,
		tokenURL:   tokenURL,
	}, nil
}

// eventID derives the deterministic, Google-valid event id for a stable UID:
// lowercase hex of sha1(uid). Hex digits (0-9a-f) are a subset of the allowed
// base32hex alphabet (0-9a-v), and the 40-char length is within Google's
// 5..1024 bound, so the result is always a valid event id and is identical for
// identical UIDs (idempotency).
func eventID(uid string) string {
	sum := sha1.Sum([]byte(uid))
	return hex.EncodeToString(sum[:])
}

// gcalEvent is the subset of the Google Calendar event resource this client
// writes. Times use RFC3339 with an explicit UTC zone.
type gcalEvent struct {
	ID                 string             `json:"id,omitempty"`
	Summary            string             `json:"summary,omitempty"`
	Description        string             `json:"description,omitempty"`
	Start              gcalEventTime      `json:"start"`
	End                gcalEventTime      `json:"end"`
	ExtendedProperties *gcalExtendedProps `json:"extendedProperties,omitempty"`
}

type gcalEventTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone,omitempty"`
}

type gcalExtendedProps struct {
	Private map[string]string `json:"private,omitempty"`
}

// PutEvent creates or updates the calendar event for a stable UID (idempotent).
func (c *GoogleCalendarClient) PutEvent(ctx context.Context, uid, summary, description string, start, end time.Time, categories []string, extraProps map[string]string) error {
	if strings.TrimSpace(uid) == "" {
		return fmt.Errorf("card calendar: PutEvent requires a non-empty uid")
	}
	id := eventID(uid)

	priv := make(map[string]string, len(extraProps)+2)
	for k, v := range extraProps {
		priv[k] = v
	}
	priv[gcalExtPropUID] = uid
	if len(categories) > 0 {
		priv[gcalExtPropCategories] = strings.Join(categories, ",")
	}

	body := gcalEvent{
		ID:                 id,
		Summary:            summary,
		Description:        description,
		Start:              gcalEventTime{DateTime: start.UTC().Format(time.RFC3339), TimeZone: "UTC"},
		End:                gcalEventTime{DateTime: end.UTC().Format(time.RFC3339), TimeZone: "UTC"},
		ExtendedProperties: &gcalExtendedProps{Private: priv},
	}

	exists, err := c.eventExists(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return c.doEventRequest(ctx, http.MethodPut, "/calendars/"+url.PathEscape(c.calendarID)+"/events/"+id, body, nil)
	}
	return c.doEventRequest(ctx, http.MethodPost, "/calendars/"+url.PathEscape(c.calendarID)+"/events", body, nil)
}

// DeleteEvent removes the calendar event for a stable UID. A missing event
// (404/410) is treated as already-deleted (no error), so cleanup is idempotent.
func (c *GoogleCalendarClient) DeleteEvent(ctx context.Context, uid string) error {
	if strings.TrimSpace(uid) == "" {
		return fmt.Errorf("card calendar: DeleteEvent requires a non-empty uid")
	}
	id := eventID(uid)
	path := "/calendars/" + url.PathEscape(c.calendarID) + "/events/" + id
	return c.doEventRequest(ctx, http.MethodDelete, path, nil, func(status int) bool {
		// Treat not-found / already-gone as success.
		return status == http.StatusNotFound || status == http.StatusGone
	})
}

// eventExists returns true if the deterministic event id is present on the
// calendar. A 404 is "not present" (insert path); any other non-2xx is an error.
func (c *GoogleCalendarClient) eventExists(ctx context.Context, id string) (bool, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return false, err
	}
	apiURL := c.apiBase + "/calendars/" + url.PathEscape(c.calendarID) + "/events/" + id
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false, fmt.Errorf("card calendar: build get request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("card calendar: get event: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusOK:
		return true, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	default:
		detail := readErrBody(resp.Body)
		return false, fmt.Errorf("card calendar: get event returned HTTP %d: %s", resp.StatusCode, detail)
	}
}

// doEventRequest performs a write request (POST/PUT/DELETE) with the given
// JSON body (nil for DELETE). okStatus, when non-nil, lets a caller accept
// additional non-2xx statuses (e.g. 404 on delete) as success.
func (c *GoogleCalendarClient) doEventRequest(ctx context.Context, method, path string, body any, okStatus func(int) bool) error {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("card calendar: marshal event: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiBase+path, reader)
	if err != nil {
		return fmt.Errorf("card calendar: build %s request: %w", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("card calendar: %s event: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if okStatus != nil && okStatus(resp.StatusCode) {
		return nil
	}
	detail := readErrBody(resp.Body)
	return fmt.Errorf("card calendar: %s event returned HTTP %d: %s", method, resp.StatusCode, detail)
}

// accessToken returns a valid access token, refreshing from the refresh token
// when the cached one is absent or within 60s of expiry.
func (c *GoogleCalendarClient) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedTok != "" && time.Now().Add(60*time.Second).Before(c.cachedExp) {
		return c.cachedTok, nil
	}
	form := url.Values{
		"client_id":     {c.cred.ClientID},
		"client_secret": {c.cred.ClientSecret},
		"refresh_token": {c.cred.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("card calendar: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("card calendar: refresh token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The error body may name the OAuth error (e.g. invalid_grant) but never
		// contains the secret values we sent; safe to surface.
		return "", fmt.Errorf("card calendar: token endpoint returned HTTP %d: %s", resp.StatusCode, readErrBody(resp.Body))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&tr); err != nil {
		return "", fmt.Errorf("card calendar: decode token response: %w", err)
	}
	if strings.TrimSpace(tr.AccessToken) == "" {
		return "", fmt.Errorf("card calendar: token endpoint returned an empty access_token")
	}
	c.cachedTok = tr.AccessToken
	ttl := tr.ExpiresIn
	if ttl <= 0 {
		ttl = 3600
	}
	c.cachedExp = time.Now().Add(time.Duration(ttl) * time.Second)
	return c.cachedTok, nil
}

// readErrBody reads a bounded, trimmed error body for diagnostics. Google error
// bodies describe the API error (status/message), never the bearer token.
func readErrBody(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 1<<16))
	return strings.TrimSpace(string(b))
}
