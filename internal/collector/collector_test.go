package collector

import (
	"context"
	"testing"
	"time"
)

// mockCollector is a mock implementation of Collector for testing
type mockCollector struct {
	name              string
	localOnly         bool
	clusterSupplement bool
	priority          int
	timeout           time.Duration
	validateError     error
	collectError      error
}

func (m *mockCollector) Name() string {
	return m.name
}

func (m *mockCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	if m.collectError != nil {
		return nil, m.collectError
	}
	return &Result{
		Data: &Data{
			Type:      m.name,
			Timestamp: time.Now(),
			Source:    "test",
			Data:      map[string]interface{}{"value": "test"},
			Metadata:  map[string]string{"collector": m.name},
		},
	}, nil
}

func (m *mockCollector) Validate(params map[string]interface{}) error {
	return m.validateError
}

func (m *mockCollector) IsLocalOnly() bool {
	return m.localOnly
}

func (m *mockCollector) IsClusterSupplement() bool {
	return m.clusterSupplement
}

func (m *mockCollector) Priority() int {
	return m.priority
}

func (m *mockCollector) Timeout() time.Duration {
	return m.timeout
}

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name      string
		collector Collector
		wantErr   bool
	}{
		{
			name:      "valid collector",
			collector: &mockCollector{name: "test-collector"},
			wantErr:   false,
		},
		{
			name:      "nil collector",
			collector: nil,
			wantErr:   true,
		},
		{
			name:      "empty name",
			collector: &mockCollector{name: ""},
			wantErr:   true,
		},
		{
			name:      "duplicate registration",
			collector: &mockCollector{name: "duplicate"},
			wantErr:   false,
		},
	}

	registry := NewRegistry()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.Register(tt.collector)
			if (err != nil) != tt.wantErr {
				t.Errorf("Register() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Test duplicate registration
			if tt.name == "duplicate registration" {
				err := registry.Register(tt.collector)
				if err == nil {
					t.Error("Expected error for duplicate registration, got nil")
				}
			}
		})
	}
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()
	collector := &mockCollector{name: "test-collector"}

	err := registry.Register(collector)
	if err != nil {
		t.Fatalf("Failed to register collector: %v", err)
	}

	tests := []struct {
		name    string
		want    Collector
		wantErr bool
	}{
		{
			name:    "test-collector",
			want:    collector,
			wantErr: false,
		},
		{
			name:    "non-existent",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := registry.Get(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Get() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()

	collector1 := &mockCollector{name: "collector1"}
	collector2 := &mockCollector{name: "collector2"}

	registry.Register(collector1)
	registry.Register(collector2)

	list := registry.List()
	if len(list) != 2 {
		t.Errorf("Expected 2 collectors, got %d", len(list))
	}
}

func TestRegistry_GetLocalOnly(t *testing.T) {
	registry := NewRegistry()

	localCollector := &mockCollector{name: "local", localOnly: true}
	clusterCollector := &mockCollector{name: "cluster", clusterSupplement: true}

	registry.Register(localCollector)
	registry.Register(clusterCollector)

	localCollectors := registry.GetLocalOnly()
	if len(localCollectors) != 1 || localCollectors[0].Name() != "local" {
		t.Errorf("Expected 1 local collector, got %d", len(localCollectors))
	}
}

func TestRegistry_GetClusterSupplement(t *testing.T) {
	registry := NewRegistry()

	localCollector := &mockCollector{name: "local", localOnly: true}
	clusterCollector := &mockCollector{name: "cluster", clusterSupplement: true}

	registry.Register(localCollector)
	registry.Register(clusterCollector)

	clusterCollectors := registry.GetClusterSupplement()
	if len(clusterCollectors) != 1 || clusterCollectors[0].Name() != "cluster" {
		t.Errorf("Expected 1 cluster supplement collector, got %d", len(clusterCollectors))
	}
}

func TestManager_StartCollector(t *testing.T) {
	registry := NewRegistry()
	manager := NewManager(registry)

	collector := &mockCollector{name: "test-collector"}
	registry.Register(collector)

	ctx := context.Background()
	params := map[string]interface{}{"param": "value"}

	err := manager.StartCollector(ctx, "test-collector", params)
	if err != nil {
		t.Errorf("StartCollector() error = %v", err)
	}

	if !manager.IsRunning("test-collector") {
		t.Error("Expected collector to be running")
	}

	// Test duplicate start
	err = manager.StartCollector(ctx, "test-collector", params)
	if err == nil {
		t.Error("Expected error for duplicate start")
	}
}

func TestManager_StopCollector(t *testing.T) {
	registry := NewRegistry()
	manager := NewManager(registry)

	collector := &mockCollector{name: "test-collector"}
	registry.Register(collector)

	ctx := context.Background()
	params := map[string]interface{}{"param": "value"}

	manager.StartCollector(ctx, "test-collector", params)

	err := manager.StopCollector("test-collector")
	if err != nil {
		t.Errorf("StopCollector() error = %v", err)
	}

	if manager.IsRunning("test-collector") {
		t.Error("Expected collector to be stopped")
	}

	// Test stop non-existent
	err = manager.StopCollector("non-existent")
	if err == nil {
		t.Error("Expected error for stopping non-existent collector")
	}
}

func TestManager_ListRunning(t *testing.T) {
	registry := NewRegistry()
	manager := NewManager(registry)

	collector1 := &mockCollector{name: "collector1"}
	collector2 := &mockCollector{name: "collector2"}

	registry.Register(collector1)
	registry.Register(collector2)

	ctx := context.Background()

	manager.StartCollector(ctx, "collector1", nil)
	manager.StartCollector(ctx, "collector2", nil)

	running := manager.ListRunning()
	if len(running) != 2 {
		t.Errorf("Expected 2 running collectors, got %d", len(running))
	}
}

func TestManager_StopAll(t *testing.T) {
	registry := NewRegistry()
	manager := NewManager(registry)

	collector1 := &mockCollector{name: "collector1"}
	collector2 := &mockCollector{name: "collector2"}

	registry.Register(collector1)
	registry.Register(collector2)

	ctx := context.Background()

	manager.StartCollector(ctx, "collector1", nil)
	manager.StartCollector(ctx, "collector2", nil)

	manager.StopAll()

	running := manager.ListRunning()
	if len(running) != 0 {
		t.Errorf("Expected 0 running collectors after StopAll, got %d", len(running))
	}
}

func TestCollectorInterface(t *testing.T) {
	var _ Collector = (*mockCollector)(nil)
}
