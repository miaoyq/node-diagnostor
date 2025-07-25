package collector

import (
	"context"
	"testing"
	"time"
)

func TestSystemLoadCollector(t *testing.T) {
	collector := NewSystemLoadCollector()

	// Test basic properties
	if collector.Name() != "system-load" {
		t.Errorf("Expected name 'system-load', got %s", collector.Name())
	}

	if !collector.IsLocalOnly() {
		t.Error("Expected IsLocalOnly to be true")
	}

	if collector.IsClusterSupplement() {
		t.Error("Expected IsClusterSupplement to be false")
	}

	// Test collection
	ctx := context.Background()
	result, err := collector.Collect(ctx, nil)
	if err != nil {
		t.Errorf("Collect failed: %v", err)
	}

	if result == nil || result.Data == nil {
		t.Error("Expected non-nil result and data")
		return
	}

	if result.Error != nil {
		t.Logf("Collection error (expected in test environment): %v", result.Error)
	}

	if result.Data != nil {
		// Check if we have load data
		if load1, ok := result.Data.Data["load1"].(float64); ok {
			t.Logf("Load1: %f", load1)
		}
		if load5, ok := result.Data.Data["load5"].(float64); ok {
			t.Logf("Load5: %f", load5)
		}
		if load15, ok := result.Data.Data["load15"].(float64); ok {
			t.Logf("Load15: %f", load15)
		}
	}
}

func TestFileDescriptorCollector(t *testing.T) {
	collector := NewFileDescriptorCollector()

	// Test basic properties
	if collector.Name() != "file-descriptor" {
		t.Errorf("Expected name 'file-descriptor', got %s", collector.Name())
	}

	if !collector.IsLocalOnly() {
		t.Error("Expected IsLocalOnly to be true")
	}

	// Test collection
	ctx := context.Background()
	result, err := collector.Collect(ctx, nil)
	if err != nil {
		t.Errorf("Collect failed: %v", err)
	}

	if result == nil || result.Data == nil {
		t.Error("Expected non-nil result and data")
		return
	}

	if result.Error != nil {
		t.Logf("Collection error (expected in test environment): %v", result.Error)
	}

	if result.Data != nil {
		// Check if we have FD data
		if allocated, ok := result.Data.Data["allocated"].(int); ok {
			t.Logf("Allocated FDs: %d", allocated)
		}
		if max, ok := result.Data.Data["max"].(int); ok {
			t.Logf("Max FDs: %d", max)
		}
		if usage, ok := result.Data.Data["usage_percent"].(float64); ok {
			t.Logf("FD usage: %f%%", usage)
		}
	}
}

func TestKernelLogCollector(t *testing.T) {
	collector := NewKernelLogCollector()

	// Test basic properties
	if collector.Name() != "kernel-log" {
		t.Errorf("Expected name 'kernel-log', got %s", collector.Name())
	}

	if !collector.IsLocalOnly() {
		t.Error("Expected IsLocalOnly to be true")
	}

	// Test collection
	ctx := context.Background()
	result, err := collector.Collect(ctx, nil)
	if err != nil {
		t.Errorf("Collect failed: %v", err)
	}

	if result == nil || result.Data == nil {
		t.Error("Expected non-nil result and data")
		return
	}

	if result.Error != nil {
		t.Logf("Collection error (expected in test environment): %v", result.Error)
	}

	if result.Data != nil {
		// Check if we have log data
		if messages, ok := result.Data.Data["messages"].([]string); ok {
			t.Logf("Found %d kernel messages", len(messages))
			for _, msg := range messages {
				t.Logf("Message: %s", msg)
			}
		}
	}
}

func TestNTPSyncCollector(t *testing.T) {
	collector := NewNTPSyncCollector()

	// Test basic properties
	if collector.Name() != "ntp-sync" {
		t.Errorf("Expected name 'ntp-sync', got %s", collector.Name())
	}

	if !collector.IsLocalOnly() {
		t.Error("Expected IsLocalOnly to be true")
	}

	// Test collection
	ctx := context.Background()
	result, err := collector.Collect(ctx, nil)
	if err != nil {
		t.Errorf("Collect failed: %v", err)
	}

	if result == nil || result.Data == nil {
		t.Error("Expected non-nil result and data")
		return
	}

	if result.Error != nil {
		t.Logf("Collection error (expected in test environment): %v", result.Error)
	}

	if result.Data != nil {
		// Check if we have NTP data
		if syncStatus, ok := result.Data.Data["sync_status"].(string); ok {
			t.Logf("NTP sync status: %s", syncStatus)
		}
		if systemTime, ok := result.Data.Data["system_time"].(string); ok {
			t.Logf("System time: %s", systemTime)
		}
	}
}

func TestSystemCollectorsTimeout(t *testing.T) {
	collectors := []Collector{
		NewSystemLoadCollector(),
		NewFileDescriptorCollector(),
		NewKernelLogCollector(),
		NewNTPSyncCollector(),
	}

	for _, collector := range collectors {
		timeout := collector.Timeout()
		if timeout <= 0 {
			t.Errorf("Collector %s has invalid timeout: %v", collector.Name(), timeout)
		}
		if timeout > 30*time.Second {
			t.Errorf("Collector %s timeout too long: %v", collector.Name(), timeout)
		}
	}
}

func TestSystemCollectorsPriority(t *testing.T) {
	collectors := []Collector{
		NewSystemLoadCollector(),
		NewFileDescriptorCollector(),
		NewKernelLogCollector(),
		NewNTPSyncCollector(),
	}

	for _, collector := range collectors {
		priority := collector.Priority()
		if priority < 1 || priority > 10 {
			t.Errorf("Collector %s has invalid priority: %d", collector.Name(), priority)
		}
	}
}

func TestSystemCollectorsValidation(t *testing.T) {
	collectors := []Collector{
		NewSystemLoadCollector(),
		NewFileDescriptorCollector(),
		NewKernelLogCollector(),
		NewNTPSyncCollector(),
	}

	for _, collector := range collectors {
		err := collector.Validate(nil)
		if err != nil {
			t.Errorf("Collector %s validation failed: %v", collector.Name(), err)
		}

		err = collector.Validate(map[string]interface{}{})
		if err != nil {
			t.Errorf("Collector %s validation failed with empty params: %v", collector.Name(), err)
		}
	}
}
