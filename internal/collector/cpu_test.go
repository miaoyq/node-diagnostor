package collector

import (
	"context"
	"testing"
	"time"
)

func TestCPUCollector_Name(t *testing.T) {
	collector := NewCPUCollector()
	if name := collector.Name(); name != "cpu" {
		t.Errorf("Expected name 'cpu', got '%s'", name)
	}
}

func TestCPUCollector_IsLocalOnly(t *testing.T) {
	collector := NewCPUCollector()
	if !collector.IsLocalOnly() {
		t.Error("Expected CPU collector to be local-only")
	}
}

func TestCPUCollector_IsClusterSupplement(t *testing.T) {
	collector := NewCPUCollector()
	if collector.IsClusterSupplement() {
		t.Error("Expected CPU collector to not be cluster supplement")
	}
}

func TestCPUCollector_Validate(t *testing.T) {
	collector := NewCPUCollector()
	if err := collector.Validate(nil); err != nil {
		t.Errorf("Expected no validation error, got: %v", err)
	}
}

func TestCPUCollector_Timeout(t *testing.T) {
	collector := NewCPUCollector()
	if timeout := collector.Timeout(); timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", timeout)
	}
}

func TestCPUCollector_Priority(t *testing.T) {
	collector := NewCPUCollector()
	if priority := collector.Priority(); priority != 1 {
		t.Errorf("Expected priority 1, got %d", priority)
	}
}

func TestCPUCollector_Collect(t *testing.T) {
	collector := NewCPUCollector()
	ctx := context.Background()

	result, err := collector.Collect(ctx, nil)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.Data == nil {
		t.Fatal("Expected non-nil data")
	}

	if result.Data.Type != "cpu" {
		t.Errorf("Expected type 'cpu', got '%s'", result.Data.Type)
	}

	if result.Data.Source != "node-local" {
		t.Errorf("Expected source 'node-local', got '%s'", result.Data.Source)
	}

	// Check if at least one of the expected data fields is present
	if len(result.Data.Data) == 0 {
		t.Error("Expected some CPU data to be collected")
	}
}
