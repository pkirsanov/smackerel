package imap

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestConnector_Interface(t *testing.T) {
	var _ connector.Connector = New("test-imap")
}

func TestConnector_Connect(t *testing.T) {
	c := New("gmail-imap")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Error("expected healthy after connect")
	}
}

func TestConnector_Connect_InvalidAuth(t *testing.T) {
	c := New("test-imap")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "invalid",
	})
	if err == nil {
		t.Error("expected error for invalid auth type")
	}
}

func TestConnector_Close(t *testing.T) {
	c := New("test-imap")
	c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "oauth2"})
	c.Close()
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("expected disconnected after close")
	}
}

func TestAssignTier_PrioritySender(t *testing.T) {
	q := QualifierConfig{PrioritySenders: []string{"boss@example.com"}}
	tier := AssignTier("boss@example.com", nil, q)
	if tier != "full" {
		t.Errorf("expected full tier for priority sender, got %q", tier)
	}
}

func TestAssignTier_SkipLabel(t *testing.T) {
	q := QualifierConfig{SkipLabels: []string{"promotions"}}
	tier := AssignTier("someone@example.com", []string{"promotions"}, q)
	if tier != "metadata" {
		t.Errorf("expected metadata tier for skip label, got %q", tier)
	}
}

func TestAssignTier_Default(t *testing.T) {
	tier := AssignTier("someone@example.com", nil, QualifierConfig{})
	if tier != "standard" {
		t.Errorf("expected standard tier by default, got %q", tier)
	}
}
