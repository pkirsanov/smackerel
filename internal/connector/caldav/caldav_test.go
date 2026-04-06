package caldav

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestConnector_Interface(t *testing.T) {
	var _ connector.Connector = New("test-caldav")
}

func TestConnector_Connect(t *testing.T) {
	c := New("google-calendar")
	err := c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "oauth2"})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Error("expected healthy")
	}
}

func TestConnector_RequiresOAuth2(t *testing.T) {
	c := New("test")
	err := c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "api_key"})
	if err == nil {
		t.Error("expected error for non-oauth2 auth")
	}
}
