package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// enrichDateLayout is the only accepted date format on weather.enrich.request.
// Strict layout prevents ambiguity (e.g. "3/15/26" vs "15/3/26") and keeps the
// upstream Open-Meteo archive query string predictable.
const enrichDateLayout = "2006-01-02"

// enrichLatMin and friends bound coordinate ranges. Out-of-range values are
// rejected before any upstream call so a malformed request can never exhaust
// retry budget against the archive API.
const (
	enrichLatMin = -90.0
	enrichLatMax = 90.0
	enrichLonMin = -180.0
	enrichLonMax = 180.0
)

// enrichErrInvalidPayload, enrichErrInvalidDate, enrichErrInvalidLatitude and
// enrichErrInvalidLongitude are the error codes embedded in the response Error
// field. They are stable strings so consumers can branch on them without
// substring-matching free-form text.
const (
	enrichErrInvalidPayload   = "invalid_payload"
	enrichErrInvalidDate      = "invalid_date"
	enrichErrInvalidLatitude  = "invalid_latitude"
	enrichErrInvalidLongitude = "invalid_longitude"
	enrichErrFetchFailed      = "fetch_failed"
)

// enrichFetchTimeout bounds how long a single enrichment request may block
// the upstream archive call. The historical archive is slow under load; 30s
// is well above the p99 observed in TestFetchHistorical_ArchiveURL while
// still surfacing wedged calls long before any caller-side timeout.
const enrichFetchTimeout = 30 * time.Second

// EnrichRequest is the payload published on weather.enrich.request by other
// connectors (Maps, digest generator) that need historical weather context
// for a specific date and location. RequestID is opaque — the subscriber
// echoes it back on the response so the publisher can correlate replies.
type EnrichRequest struct {
	RequestID string  `json:"request_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Date      string  `json:"date"`
}

// EnrichResponse is the payload published on weather.enrich.response. On
// success Weather is populated and Error is empty; on failure Error carries
// one of the enrichErr* codes and Weather is nil. RequestID, Latitude,
// Longitude and Date echo the request fields when they were parseable so the
// caller can correlate replies even on failure.
type EnrichResponse struct {
	RequestID string          `json:"request_id"`
	Latitude  float64         `json:"latitude"`
	Longitude float64         `json:"longitude"`
	Date      string          `json:"date"`
	Weather   *CurrentWeather `json:"weather,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// SetArchiveURL overrides the Open-Meteo archive base URL. Exported so that
// integration tests in other packages can point the connector at an
// httptest.Server instead of the real upstream API.
func (c *Connector) SetArchiveURL(url string) {
	c.mu.Lock()
	c.archiveURL = url
	c.mu.Unlock()
}

// validateEnrichRequest decodes and bounds-checks an enrichment payload.
// Returns the parsed request on success or an EnrichResponse with the Error
// field populated on failure (so the subscriber can publish it as-is).
func validateEnrichRequest(data []byte) (EnrichRequest, *EnrichResponse) {
	var req EnrichRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return req, &EnrichResponse{
			RequestID: req.RequestID,
			Error:     enrichErrInvalidPayload,
		}
	}

	if req.Latitude < enrichLatMin || req.Latitude > enrichLatMax {
		return req, &EnrichResponse{
			RequestID: req.RequestID,
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
			Date:      req.Date,
			Error:     enrichErrInvalidLatitude,
		}
	}
	if req.Longitude < enrichLonMin || req.Longitude > enrichLonMax {
		return req, &EnrichResponse{
			RequestID: req.RequestID,
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
			Date:      req.Date,
			Error:     enrichErrInvalidLongitude,
		}
	}
	if req.Date == "" {
		return req, &EnrichResponse{
			RequestID: req.RequestID,
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
			Error:     enrichErrInvalidDate,
		}
	}
	if _, err := time.Parse(enrichDateLayout, req.Date); err != nil {
		return req, &EnrichResponse{
			RequestID: req.RequestID,
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
			Date:      req.Date,
			Error:     enrichErrInvalidDate,
		}
	}

	return req, nil
}

// handleEnrichmentRequest validates the payload, dispatches to fetchHistorical,
// and returns the JSON-encoded response bytes ready to publish. It always
// returns a non-nil byte slice — failures are encoded into the response body
// rather than dropped, so a publisher subscribed to weather.enrich.response
// always sees a reply (correlated by request_id).
func handleEnrichmentRequest(ctx context.Context, c *Connector, data []byte) []byte {
	req, errResp := validateEnrichRequest(data)
	if errResp != nil {
		return mustMarshalEnrichResponse(errResp)
	}

	fetchCtx, cancel := context.WithTimeout(ctx, enrichFetchTimeout)
	defer cancel()

	cw, err := c.fetchHistorical(fetchCtx, req.Latitude, req.Longitude, req.Date)
	if err != nil {
		return mustMarshalEnrichResponse(&EnrichResponse{
			RequestID: req.RequestID,
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
			Date:      req.Date,
			Error:     enrichErrFetchFailed,
		})
	}

	return mustMarshalEnrichResponse(&EnrichResponse{
		RequestID: req.RequestID,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		Date:      req.Date,
		Weather:   cw,
	})
}

// mustMarshalEnrichResponse encodes an EnrichResponse to JSON. Encoding
// cannot realistically fail for the EnrichResponse shape (only string,
// float64, int, bool fields), but if it ever does the subscriber falls back
// to a hard-coded JSON envelope so the publisher still sees a reply rather
// than a permanent silence.
func mustMarshalEnrichResponse(resp *EnrichResponse) []byte {
	b, err := json.Marshal(resp)
	if err != nil {
		slog.Error("weather enrich response marshal failed", "error", err)
		return []byte(`{"error":"response_marshal_failed"}`)
	}
	return b
}

// StartEnrichmentSubscriber subscribes the connector to
// weather.enrich.request on the supplied NATS client and publishes replies on
// weather.enrich.response. Subscription uses core NATS (nc.Conn.Subscribe)
// for the same reason annotations.go and lists.go do: it is a fire-and-forget
// fan-in where the publisher already encodes correlation in request_id, and
// the JetStream WEATHER stream still captures the messages for inspection.
//
// The returned subscription must be unsubscribed by the caller (typically
// during shutdown) to release the NATS server-side state. Returns an error if
// the supplied client is nil or the underlying Subscribe call fails.
func StartEnrichmentSubscriber(ctx context.Context, nc *smacknats.Client, c *Connector) (*nats.Subscription, error) {
	if nc == nil || nc.Conn == nil {
		return nil, fmt.Errorf("nats client is nil")
	}
	if c == nil {
		return nil, fmt.Errorf("weather connector is nil")
	}

	sub, err := nc.Conn.Subscribe(smacknats.SubjectWeatherEnrichRequest, func(msg *nats.Msg) {
		respBytes := handleEnrichmentRequest(ctx, c, msg.Data)
		if pubErr := nc.Publish(ctx, smacknats.SubjectWeatherEnrichResponse, respBytes); pubErr != nil {
			slog.Warn("weather enrich response publish failed", "error", pubErr)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe to %s: %w", smacknats.SubjectWeatherEnrichRequest, err)
	}

	slog.Info("weather enrichment subscriber started",
		"request_subject", smacknats.SubjectWeatherEnrichRequest,
		"response_subject", smacknats.SubjectWeatherEnrichResponse,
	)
	return sub, nil
}
