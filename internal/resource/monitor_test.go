package resource

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestNewResourceMonitor(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:    5.0,
		MemoryBytes:   100 * 1024 * 1024, // 100MB
		MaxGoroutines: 100,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	if monitor == nil {
		t.Fatal("Expected non-nil monitor")
	}

	if monitor.cpuLimit != limits.CPUPercent {
		t.Errorf("Expected cpuLimit %f, got %f", limits.CPUPercent, monitor.cpuLimit)
	}

	if monitor.memoryLimit != limits.MemoryBytes {
		t.Errorf("Expected memoryLimit %d, got %d", limits.MemoryBytes, monitor.memoryLimit)
	}
}

func TestResourceMonitor_GetCurrentMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	metrics := monitor.GetCurrentMetrics()
	if metrics == nil {
		t.Fatal("Expected non-nil metrics")
	}

	if metrics.CPUPercent < 0 {
		t.Errorf("Expected non-negative CPU percent, got %f", metrics.CPUPercent)
	}

	if metrics.MemoryBytes < 0 {
		t.Errorf("Expected non-negative memory bytes, got %d", metrics.MemoryBytes)
	}
}

func TestResourceMonitor_LimitChecks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  0.1,  // 设置很低的限制以触发超限
		MemoryBytes: 1024, // 设置很低的限制以触发超限
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	// 强制设置高值以测试超限检查
	monitor.mu.Lock()
	monitor.currentCPU = 1.0
	monitor.currentMemory = 2048
	monitor.mu.Unlock()

	if !monitor.IsCPULimitExceeded() {
		t.Error("Expected CPU limit to be exceeded")
	}

	if !monitor.IsMemoryLimitExceeded() {
		t.Error("Expected memory limit to be exceeded")
	}
}

func TestResourceMonitor_GetSystemInfo(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	totalMem, availMem, cpuCount, err := monitor.GetSystemInfo()
	if err != nil {
		t.Fatalf("GetSystemInfo failed: %v", err)
	}

	if totalMem <= 0 {
		t.Error("Expected positive total memory")
	}

	if availMem <= 0 {
		t.Error("Expected positive available memory")
	}

	if cpuCount <= 0 {
		t.Error("Expected positive CPU count")
	}
}

func TestResourceMonitor_GetProcessInfo(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	info, err := monitor.GetProcessInfo()
	if err != nil {
		t.Fatalf("GetProcessInfo failed: %v", err)
	}

	if info.PID <= 0 {
		t.Error("Expected positive PID")
	}

	if info.Name == "" {
		t.Error("Expected non-empty process name")
	}

	if info.StartTime.IsZero() {
		t.Error("Expected non-zero start time")
	}
}

func TestResourceMonitor_StartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error)
	go func() {
		done <- monitor.Start(ctx)
	}()

	// 等待启动
	time.Sleep(50 * time.Millisecond)

	// 验证监控器正在运行
	metrics := monitor.GetCurrentMetrics()
	if metrics == nil {
		t.Error("Expected metrics to be available")
	}

	// 等待context超时
	select {
	case err := <-done:
		if err != context.DeadlineExceeded {
			t.Errorf("Expected context deadline exceeded, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Monitor did not stop within expected time")
	}
}

func TestResourceMonitor_GetResourceStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	status := monitor.GetResourceStatus()
	if status == nil {
		t.Fatal("Expected non-nil status")
	}

	requiredFields := []string{
		"cpu_percent", "cpu_limit", "cpu_exceeded",
		"memory_bytes", "memory_limit", "memory_exceeded",
		"goroutines", "system_total_mem", "system_avail_mem",
		"system_cpu_count", "uptime_seconds",
	}

	for _, field := range requiredFields {
		if _, ok := status[field]; !ok {
			t.Errorf("Expected status to contain field %s", field)
		}
	}
}

func TestResourceMonitor_ConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	// 并发访问测试
	const numGoroutines = 10
	done := make(chan bool)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = monitor.GetCurrentMetrics()
				_ = monitor.IsCPULimitExceeded()
				_ = monitor.IsMemoryLimitExceeded()
				_ = monitor.GetResourceStatus()
			}
			done <- true
		}()
	}

	// 等待所有goroutine完成
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
			// 成功完成
		case <-time.After(5 * time.Second):
			t.Fatal("Concurrent access test timed out")
		}
	}
}

func BenchmarkResourceMonitor(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		b.Fatalf("NewResourceMonitor failed: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = monitor.GetCurrentMetrics()
			_ = monitor.IsCPULimitExceeded()
			_ = monitor.GetResourceStatus()
		}
	})
}
