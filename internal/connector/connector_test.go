package connector

import (
	"context"
	"fmt"
	"testing"
)

// testConnector is a mock connector for testing.
type testConnector struct {
	id     string
	health HealthStatus
	items  []RawArtifact
	closed bool
}

func newTestConnector(id string) *testConnector {
	return &testConnector{id: id, health: HealthHealthy}
}

func (c *testConnector) ID() string                                         { return c.id }
func (c *testConnector) Connect(_ context.Context, _ ConnectorConfig) error { return nil }
func (c *testConnector) Sync(_ context.Context, _ string) ([]RawArtifact, string, error) {
	return c.items, "cursor-1", nil
}
func (c *testConnector) Health(_ context.Context) HealthStatus { return c.health }
func (c *testConnector) Close() error {
	c.closed = true
	return nil
}

func TestConnectorInterface(t *testing.T) {
	var _ Connector = newTestConnector("test")
}

func TestRegistry_Register(t *testing.T) {
	reg := NewRegistry()
	conn := newTestConnector("test-1")

	if err := reg.Register(conn); err != nil {
		t.Fatalf("register: %v", err)
	}

	if reg.Count() != 1 {
		t.Errorf("expected 1 connector, got %d", reg.Count())
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	reg := NewRegistry()
	conn := newTestConnector("test-1")

	reg.Register(conn)
	err := reg.Register(conn)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := NewRegistry()
	conn := newTestConnector("test-1")
	reg.Register(conn)

	got, ok := reg.Get("test-1")
	if !ok {
		t.Fatal("expected to find connector")
	}
	if got.ID() != "test-1" {
		t.Errorf("expected ID test-1, got %s", got.ID())
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := NewRegistry()
	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	reg := NewRegistry()
	conn := newTestConnector("test-1")
	reg.Register(conn)

	if err := reg.Unregister("test-1"); err != nil {
		t.Fatalf("unregister: %v", err)
	}

	if reg.Count() != 0 {
		t.Errorf("expected 0 connectors, got %d", reg.Count())
	}
	if !conn.closed {
		t.Error("expected connector to be closed")
	}
}

func TestRegistry_Unregister_NotFound(t *testing.T) {
	reg := NewRegistry()
	err := reg.Unregister("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent connector")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry()
	for i := 0; i < 3; i++ {
		reg.Register(newTestConnector(fmt.Sprintf("test-%d", i)))
	}

	ids := reg.List()
	if len(ids) != 3 {
		t.Errorf("expected 3 IDs, got %d", len(ids))
	}
}

func TestConnectorSync(t *testing.T) {
	conn := newTestConnector("test")
	conn.items = []RawArtifact{
		{SourceID: "test", SourceRef: "ref-1", Title: "Item 1"},
		{SourceID: "test", SourceRef: "ref-2", Title: "Item 2"},
	}

	items, cursor, err := conn.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if cursor != "cursor-1" {
		t.Errorf("expected cursor-1, got %s", cursor)
	}
}

func TestConnectorHealth(t *testing.T) {
	conn := newTestConnector("test")

	if conn.Health(context.Background()) != HealthHealthy {
		t.Error("expected healthy status")
	}

	conn.health = HealthError
	if conn.Health(context.Background()) != HealthError {
		t.Error("expected error status")
	}
}
