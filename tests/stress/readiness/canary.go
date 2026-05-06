package readiness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

type Config struct {
	CoreURL     string
	DatabaseURL string
	NATSURL     string
	AuthToken   string
}

type Probes struct {
	HTTPClient   *http.Client
	PingDatabase func(context.Context, string) error
	ConnectNATS  func(context.Context, string, string) error
}

type healthPayload struct {
	Status   string                   `json:"status"`
	Services map[string]serviceStatus `json:"services"`
}

type serviceStatus struct {
	Status string `json:"status"`
}

func ConfigFromEnv() (Config, error) {
	config := Config{
		CoreURL:     os.Getenv("CORE_EXTERNAL_URL"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		NATSURL:     os.Getenv("NATS_URL"),
		AuthToken:   os.Getenv("SMACKEREL_AUTH_TOKEN"),
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (config Config) Validate() error {
	missing := make([]string, 0, 4)
	if strings.TrimSpace(config.CoreURL) == "" {
		missing = append(missing, "CORE_EXTERNAL_URL")
	}
	if strings.TrimSpace(config.DatabaseURL) == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if strings.TrimSpace(config.NATSURL) == "" {
		missing = append(missing, "NATS_URL")
	}
	if strings.TrimSpace(config.AuthToken) == "" {
		missing = append(missing, "SMACKEREL_AUTH_TOKEN")
	}
	if len(missing) > 0 {
		return fmt.Errorf("stress readiness: missing required env: %s", strings.Join(missing, ", "))
	}
	return nil
}

func Check(ctx context.Context, config Config) error {
	return CheckWithProbes(ctx, config, DefaultProbes())
}

func DefaultProbes() Probes {
	return Probes{
		HTTPClient:   &http.Client{Timeout: 5 * time.Second},
		PingDatabase: pingDatabase,
		ConnectNATS:  connectNATS,
	}
}

func CheckWithProbes(ctx context.Context, config Config, probes Probes) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if probes.HTTPClient == nil {
		probes.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	if probes.PingDatabase == nil {
		probes.PingDatabase = pingDatabase
	}
	if probes.ConnectNATS == nil {
		probes.ConnectNATS = connectNATS
	}

	if err := checkCoreHealth(ctx, config, probes.HTTPClient); err != nil {
		return err
	}
	if err := probes.PingDatabase(ctx, config.DatabaseURL); err != nil {
		return fmt.Errorf("database readiness failed: %w", err)
	}
	if err := probes.ConnectNATS(ctx, config.NATSURL, config.AuthToken); err != nil {
		return fmt.Errorf("nats readiness failed: %w", err)
	}
	return nil
}

func checkCoreHealth(ctx context.Context, config Config, httpClient *http.Client) error {
	endpoint := strings.TrimRight(config.CoreURL, "/") + "/api/health"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("core health readiness failed: build request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+config.AuthToken)

	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("core health readiness failed: GET %s: %w", endpoint, err)
	}
	defer response.Body.Close()

	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return fmt.Errorf("core health readiness failed: read response: %w", readErr)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("core health readiness failed: GET %s returned %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}

	payload := healthPayload{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("core health readiness failed: parse response: %w", err)
	}
	if payload.Services == nil {
		return errors.New("core health readiness failed: authenticated health response did not include service topology; check SMACKEREL_AUTH_TOKEN")
	}
	for _, serviceName := range []string{"postgres", "nats"} {
		service, exists := payload.Services[serviceName]
		if !exists {
			return fmt.Errorf("core health readiness failed: service %s missing from authenticated health response", serviceName)
		}
		if service.Status != "up" {
			return fmt.Errorf("core health readiness failed: service %s status=%q, want up", serviceName, service.Status)
		}
	}
	return nil
}

func pingDatabase(parentContext context.Context, databaseURL string) error {
	ctx, cancel := context.WithTimeout(parentContext, 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

func connectNATS(ctx context.Context, natsURL string, authToken string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	options := []nats.Option{
		nats.Name("smackerel-stress-readiness"),
		nats.Timeout(5 * time.Second),
	}
	if authToken != "" {
		options = append(options, nats.Token(authToken))
	}

	connection, err := nats.Connect(natsURL, options...)
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	connection.Close()
	return nil
}
