package resource

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func TestNewResourceManager(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	manager := NewResourceManager(logger, monitor, limits)
	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	if manager.monitor != monitor {
		t.Error("Expected monitor to be set correctly")
	}

	if manager.limits.CPUPercent != limits.CPUPercent {
		t.Errorf("Expected cpu limit %f, got %f", limits.CPUPercent, manager.limits.CPUPercent)
	}
}

func TestResourceManager_RegisterCheck(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, _ := NewResourceMonitor(logger, limits)
	manager := NewResourceManager(logger, monitor, limits)

	// 注册检查项
	manager.RegisterCheck("cpu-check", 8)
	manager.RegisterCheck("memory-check", 5)
	manager.RegisterCheck("disk-check", 3)

	// 验证注册
	if !manager.CanExecuteCheck("cpu-check") {
		t.Error("Expected cpu-check to be executable")
	}

	if !manager.CanExecuteCheck("memory-check") {
		t.Error("Expected memory-check to be executable")
	}

	if !manager.CanExecuteCheck("disk-check") {
		t.Error("Expected disk-check to be executable")
	}
}

func TestResourceManager_UnregisterCheck(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, _ := NewResourceMonitor(logger, limits)
	manager := NewResourceManager(logger, monitor, limits)

	// 注册然后注销
	manager.RegisterCheck("test-check", 5)
	manager.UnregisterCheck("test-check")

	if manager.CanExecuteCheck("test-check") {
		t.Error("Expected test-check to be unregistered")
	}
}

func TestResourceManager_Throttling(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  0.1,  // 设置很低的限制
		MemoryBytes: 1024, // 设置很低的限制
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	manager := NewResourceManager(logger, monitor, limits)

	// 注册不同优先级的检查项
	manager.RegisterCheck("high-priority", 9)
	manager.RegisterCheck("medium-priority", 6)
	manager.RegisterCheck("low-priority", 3)

	// 强制触发限流
	monitor.mu.Lock()
	monitor.currentCPU = 1.0
	monitor.currentMemory = 2048
	monitor.mu.Unlock()

	_, err = manager.StartCheck("high-priority", time.Second)
	if err != nil {
		t.Errorf("Expected high-priority to be executable, got error: %v", err)
	}
	_, err = manager.StartCheck("medium-priority", time.Second)
	if err != nil {
		t.Errorf("Expected medium-priority to be executable, got error: %v", err)
	}
	_, err = manager.StartCheck("low-priority", time.Second)
	if err != nil {
		t.Errorf("Expected low-priority to be executable, got error: %v", err)
	}

	// 评估资源
	manager.EvaluateResources()

	// 验证限流效果
	if !manager.CanExecuteCheck("high-priority") {
		t.Error("Expected high-priority to be executable during throttling")
	}

	// 中优先级可能被执行（随机性）
	_ = manager.CanExecuteCheck("medium-priority")

	// 低优先级应该被暂停
	if manager.CanExecuteCheck("low-priority") {
		t.Error("Expected low-priority to be paused during throttling")
	}

	// 验证状态
	status := manager.GetThrottlingStatus()
	if status == nil {
		t.Fatal("Expected non-nil throttling status")
	}

	isThrottled, ok := status["is_throttled"].(bool)
	if !ok {
		t.Error("Expected is_throttled to be bool")
	}

	if !isThrottled {
		t.Error("Expected throttling to be active")
	}
}

func TestResourceManager_Recovery(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	manager := NewResourceManager(logger, monitor, limits)
	manager.recoveryDelay = 100 * time.Millisecond // 缩短恢复延迟用于测试

	// 注册检查项
	manager.RegisterCheck("test-check", 5)

	// 强制触发限流
	monitor.mu.Lock()
	monitor.currentCPU = 10.0
	monitor.currentMemory = 200 * 1024 * 1024
	monitor.mu.Unlock()

	manager.EvaluateResources()

	// 验证限流状态
	if !manager.isThrottled {
		t.Error("Expected throttling to be active")
	}

	// 模拟资源恢复
	monitor.mu.Lock()
	monitor.currentCPU = 1.0
	monitor.currentMemory = 10 * 1024 * 1024
	monitor.mu.Unlock()

	// 等待恢复
	time.Sleep(150 * time.Millisecond)
	manager.EvaluateResources()

	// 验证恢复
	if manager.isThrottled {
		t.Error("Expected throttling to be inactive after recovery")
	}

	if !manager.CanExecuteCheck("test-check") {
		t.Error("Expected test-check to be executable after recovery")
	}
}

func TestResourceManager_GetActiveChecks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, _ := NewResourceMonitor(logger, limits)
	manager := NewResourceManager(logger, monitor, limits)

	// 注册检查项
	manager.RegisterCheck("check1", 8)
	manager.RegisterCheck("check2", 5)
	manager.RegisterCheck("check3", 3)
	manager.StartCheck("check1", time.Second)
	manager.StartCheck("check2", time.Second)
	manager.StartCheck("check3", time.Second)

	active := manager.GetActiveChecks()
	if len(active) != 3 {
		t.Errorf("Expected 3 active checks, got %d", len(active))
	}
}

func TestResourceManager_GetPausedChecks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  0.1,
		MemoryBytes: 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	manager := NewResourceManager(logger, monitor, limits)
	manager.RegisterCheck("low-priority", 2)

	// 强制触发限流
	monitor.mu.Lock()
	monitor.currentCPU = 10.0
	monitor.currentMemory = 2048
	monitor.mu.Unlock()

	_, err = manager.StartCheck("low-priority", time.Second)
	if err != nil {
		t.Fatalf("StartCheck failed: %v", err)
	}
	manager.EvaluateResources()

	paused := manager.GetPausedChecks()
	if len(paused) == 0 {
		t.Error("Expected some paused checks")
	}
}

func TestResourceManager_UpdateCheckStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, _ := NewResourceMonitor(logger, limits)
	manager := NewResourceManager(logger, monitor, limits)

	manager.RegisterCheck("test-check", 5)
	_, err := manager.StartCheck("test-check", time.Second)
	if err != nil {
		t.Fatalf("StartCheck failed: %v", err)
	}
	manager.UpdateCheckStatus("test-check", CheckStatusThrottled)

	// 验证状态更新
	status := manager.GetThrottlingStatus()
	checks := status["checks"].(map[string]string)
	if checks["test-check"] != string(CheckStatusThrottled) {
		t.Errorf("Expected status %s, got %s", CheckStatusThrottled, checks["test-check"])
	}
}

func TestResourceManager_StartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	manager := NewResourceManager(logger, monitor, limits)
	manager.RegisterCheck("test-check", 5)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error)
	go func() {
		done <- manager.Start(ctx)
	}()

	// 等待启动
	time.Sleep(50 * time.Millisecond)

	// 验证管理器正在运行
	summary := manager.GetResourceSummary()
	if summary == nil {
		t.Error("Expected resource summary to be available")
	}

	// 等待context超时
	select {
	case err := <-done:
		if err != context.DeadlineExceeded {
			t.Errorf("Expected context deadline exceeded, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Manager did not stop within expected time")
	}
}

func TestResourceManager_GetResourceSummary(t *testing.T) {
	logger := zaptest.NewLogger(t)
	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	monitor, err := NewResourceMonitor(logger, limits)
	if err != nil {
		t.Fatalf("NewResourceMonitor failed: %v", err)
	}

	manager := NewResourceManager(logger, monitor, limits)
	summary := manager.GetResourceSummary()

	if summary == nil {
		t.Fatal("Expected non-nil resource summary")
	}

	requiredFields := []string{"current_usage", "limits", "throttling"}
	for _, field := range requiredFields {
		if _, ok := summary[field]; !ok {
			t.Errorf("Expected summary to contain field %s", field)
		}
	}
}
