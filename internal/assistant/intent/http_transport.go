package intent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const (
	compileRoute         = "/assistant/intent/compile"
	errorResponseBodyMax = 512
)

// HTTPTransport calls the ML sidecar's schema-bound intent compiler route.
type HTTPTransport struct {
	baseURL         string
	authToken       string
	responseBodyMax int64
	httpClient      *http.Client
}

// NewHTTPTransport constructs the production core-to-ML compiler transport.
func NewHTTPTransport(baseURL, authToken string, timeout time.Duration, responseBodyMax int) (*HTTPTransport, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("intent: NewHTTPTransport requires a non-empty baseURL")
	}
	if strings.TrimSpace(authToken) == "" {
		return nil, errors.New("intent: NewHTTPTransport requires a non-empty authToken")
	}
	if timeout <= 0 {
		return nil, errors.New("intent: NewHTTPTransport requires timeout > 0")
	}
	if responseBodyMax <= 0 {
		return nil, errors.New("intent: NewHTTPTransport requires responseBodyMax > 0")
	}
	return &HTTPTransport{
		baseURL:         strings.TrimRight(baseURL, "/"),
		authToken:       authToken,
		responseBodyMax: int64(responseBodyMax),
		httpClient:      &http.Client{Timeout: timeout},
	}, nil
}

// Compile implements Transport through POST /assistant/intent/compile.
func (t *HTTPTransport) Compile(ctx context.Context, payload CompileRequest) (CompileResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return CompileResponse{}, fmt.Errorf("intent transport: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+compileRoute, bytes.NewReader(body))
	if err != nil {
		return CompileResponse{}, fmt.Errorf("intent transport: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.authToken)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	response, err := t.httpClient.Do(req)
	if err != nil {
		return CompileResponse{}, fmt.Errorf("intent transport: POST %s: %w", compileRoute, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, errorResponseBodyMax))
		return CompileResponse{}, fmt.Errorf(
			"intent transport: %s returned HTTP %d: %s",
			compileRoute,
			response.StatusCode,
			strings.TrimSpace(string(snippet)),
		)
	}

	limited := io.LimitReader(response.Body, t.responseBodyMax+1)
	encoded, err := io.ReadAll(limited)
	if err != nil {
		return CompileResponse{}, fmt.Errorf("intent transport: read response: %w", err)
	}
	if int64(len(encoded)) > t.responseBodyMax {
		return CompileResponse{}, fmt.Errorf(
			"intent transport: response exceeds configured max of %d bytes",
			t.responseBodyMax,
		)
	}

	var parsed CompileResponse
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return CompileResponse{}, fmt.Errorf("intent transport: decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CompileResponse{}, errors.New("intent transport: response contains trailing JSON")
	}
	if parsed.SchemaVersion != payload.SchemaVersion {
		return CompileResponse{}, fmt.Errorf(
			"intent transport: response schema_version %q does not match request %q",
			parsed.SchemaVersion,
			payload.SchemaVersion,
		)
	}
	if len(parsed.CompiledIntent) == 0 {
		return CompileResponse{}, errors.New("intent transport: response compiled_intent is required")
	}
	return parsed, nil
}
