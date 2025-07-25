package resource

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestNewThrottler(t *testing.T) {
	logger := zaptest.NewLogger(t)
	throttler := NewThrottler(logger)

	if throttler == nil {
		t.Fatal("Expected non-nil throttler")
	}

	if throttler.retryCounts == nil {
		t.Error("Expected retryCounts to be initialized")
	}

	if throttler.lastRetryTime == nil {
		t.Error("Expected lastRetryTime to be initialized")
	}
}

func TestThrottler_CreateStrategy(t *testing.T) {
	logger := zaptest.NewLogger(t)
	throttler := NewThrottler(logger)

	limits := ResourceLimits{
		CPUPercent:  5.0,
		MemoryBytes: 100 * 1024 * 1024,
	}

	tests := []struct {
		name            string
		cpuPercent      float64
		memoryBytes     uint64
		expectedMinPrio int
	}{
		{
			name:            "normal usage",
			cpuPercent:      2.0,
			memoryBytes:     50 * 1024 * 1024,
			expectedMinPrio: 5,
		},
		{
			name:            "mild overload",
			cpuPercent:      7.5,
			memoryBytes:     150 * 1024 * 1024,
			expectedMinPrio: 7,
		},
		{
			name:            "severe overload",
			cpuPercent:      15.0,
			memoryBytes:     300 * 1024 * 1024,
			expectedMinPrio: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &ResourceMetrics{
				CPUPercent:  tt.cpuPercent,
				MemoryBytes: tt.memoryBytes,
			}

			strategy := throttler.CreateStrategy(metrics, limits)
			if strategy.MinPriority != tt.expectedMinPrio {
				t.Errorf("Expected min priority %d, got %d", tt.expectedMinPrio, strategy.MinPriority)
			}
		})
	}
}

func TestThrottler_ShouldRetry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	throttler := NewThrottler(logger)

	// 首次重试应该允许
	if !throttler.ShouldRetry("test-check") {
		t.Error("Expected first retry to be allowed")
	}

	// 记录重试
	throttler.RecordRetry("test-check")

	// 立即重试应该被拒绝
	if throttler.ShouldRetry("test-check") {
		t.Error("Expected immediate retry to be denied")
	}

	// 等待并重试
	time.Sleep(1 * time.Second)
	if !throttler.ShouldRetry("test-check") {
		t.Error("Expected retry to be allowed after delay")
	}
}

func TestThrottler_RecordRetry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	throttler := NewThrottler(logger)

	throttler.RecordRetry("test-check")
	count, lastRetry, _ := throttler.GetRetryInfo("test-check")

	if count != 1 {
		t.Errorf("Expected retry count 1, got %d", count)
	}

	if lastRetry.IsZero() {
		t.Error("Expected non-zero last retry time")
	}

	// 再次记录
	throttler.RecordRetry("test-check")
	count, _, _ = throttler.GetRetryInfo("test-check")
	if count != 2 {
		t.Errorf("Expected retry count 2, got %d", count)
	}
}

func TestThrottler_ResetRetry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	throttler := NewThrottler(logger)

	// 记录一些重试
	throttler.RecordRetry("test-check")
	throttler.RecordRetry("test-check")

	// 重置
	throttler.ResetRetry("test-check")

	count, _, exists := throttler.GetRetryInfo("test-check")
	if count != 0 {
		t.Errorf("Expected retry count 0 after reset, got %d", count)
	}

	if !exists {
		t.Error("Expected retry info to exist")
	}
}

func TestThrottler_GetRetryInfo(t *testing.T) {
	logger := zaptest.NewLogger(t)
	throttler := NewThrottler(logger)

	count, _, shouldRetry := throttler.GetRetryInfo("non-existent")
	if count != 0 {
		t.Errorf("Expected count 0 for non-existent check, got %d", count)
	}

	if !shouldRetry {
		t.Error("Expected should_retry to be true for non-existent check")
	}

	// 记录重试后
	throttler.RecordRetry("test-check")
	count, lastRetry, _ := throttler.GetRetryInfo("test-check")

	if count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}

	if lastRetry.IsZero() {
		t.Error("Expected non-zero last retry time")
	}
}

func TestThrottler_GetAllRetryInfo(t *testing.T) {
	logger := zaptest.NewLogger(t)
	throttler := NewThrottler(logger)

	// 记录多个重试
	throttler.RecordRetry("check1")
	throttler.RecordRetry("check2")
	throttler.RecordRetry("check2")

	allInfo := throttler.GetAllRetryInfo()
	if len(allInfo) != 2 {
		t.Errorf("Expected 2 retry entries, got %d", len(allInfo))
	}

	if info, ok := allInfo["check1"].(map[string]interface{}); ok {
		count := info["retry_count"].(int)
		if count != 1 {
			t.Errorf("Expected check1 count 1, got %d", count)
		}
	} else {
		t.Error("Expected check1 info to be map[string]interface{}")
	}

	if info, ok := allInfo["check2"].(map[string]interface{}); ok {
		count := info["retry_count"].(int)
		if count != 2 {
			t.Errorf("Expected check2 count 2, got %d", count)
		}
	} else {
		t.Error("Expected check2 info to be map[string]interface{}")
	}
}

func TestThrottler_CalculateBackoffDelay(t *testing.T) {
	logger := zaptest.NewLogger(t)
	throttler := NewThrottler(logger)

	// 测试退避计算
	delays := []time.Duration{}
	for i := 0; i < 5; i++ {
		throttler.RecordRetry("test-check")
		delay := throttler.CalculateBackoffDelay("test-check")
		delays = append(delays, delay)
	}

	// 验证退避递增
	for i := 1; i < len(delays); i++ {
		if delays[i] <= delays[i-1] {
			t.Errorf("Expected delay %d > delay %d, got %v <= %v", i, i-1, delays[i], delays[i-1])
		}
	}

	// 验证最大限制
	maxDelay := throttler.CalculateBackoffDelay("test-check")
	if maxDelay > 5*time.Minute {
		t.Errorf("Expected max delay <= 5 minutes, got %v", maxDelay)
	}
}

func TestThrottler_ClearRetryInfo(t *testing.T) {
	logger := zaptest.NewLogger(t)
	throttler := NewThrottler(logger)

	// 记录一些重试
	throttler.RecordRetry("check1")
	throttler.RecordRetry("check2")

	// 清除所有
	throttler.ClearRetryInfo()

	allInfo := throttler.GetAllRetryInfo()
	if len(allInfo) != 0 {
		t.Errorf("Expected 0 retry entries after clear, got %d", len(allInfo))
	}
}

func TestThrottler_ConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	throttler := NewThrottler(logger)

	// 简单的并发测试
	done := make(chan bool)

	go func() {
		for i := 0; i < 50; i++ {
			throttler.RecordRetry("test-check")
			throttler.ShouldRetry("test-check")
			throttler.GetRetryInfo("test-check")
		}
		done <- true
	}()

	select {
	case <-done:
		// 成功完成
	case <-time.After(2 * time.Second):
		t.Fatal("Concurrent access test timed out")
	}
}

func BenchmarkThrottler(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	throttler := NewThrottler(logger)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			throttler.ShouldRetry("test-check")
			throttler.RecordRetry("test-check")
			throttler.GetRetryInfo("test-check")
		}
	})
}
