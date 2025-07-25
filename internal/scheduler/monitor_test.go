package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewTaskMonitor(t *testing.T) {
	monitor := NewTaskMonitor()
	if monitor == nil {
		t.Fatal("NewTaskMonitor should not return nil")
	}

	if monitor.taskMetrics == nil {
		t.Error("taskMetrics should be initialized")
	}

	if monitor.globalStats == nil {
		t.Error("globalStats should be initialized")
	}
}

func TestTaskMonitor_Record(t *testing.T) {
	monitor := NewTaskMonitor()
	ctx := context.Background()

	// 测试记录成功的任务
	successResult := &Result{
		TaskID:    "test-task-1",
		Success:   true,
		Duration:  100 * time.Millisecond,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"cpu": 50.5},
	}

	err := monitor.Record(ctx, successResult)
	if err != nil {
		t.Errorf("Record should not return error, got: %v", err)
	}

	// 验证任务指标
	metrics := monitor.GetTaskMetrics("test-task-1")
	if metrics["task_id"] != "test-task-1" {
		t.Errorf("Expected task_id to be 'test-task-1', got: %v", metrics["task_id"])
	}

	if metrics["total_executions"] != int64(1) {
		t.Errorf("Expected total_executions to be 1, got: %v", metrics["total_executions"])
	}

	if metrics["successful_runs"] != int64(1) {
		t.Errorf("Expected successful_runs to be 1, got: %v", metrics["successful_runs"])
	}

	if metrics["failed_runs"] != int64(0) {
		t.Errorf("Expected failed_runs to be 0, got: %v", metrics["failed_runs"])
	}

	// 测试记录失败的任务
	failedResult := &Result{
		TaskID:    "test-task-1",
		Success:   false,
		Duration:  200 * time.Millisecond,
		Timestamp: time.Now(),
		Error:     errors.New("test error"),
	}

	err = monitor.Record(ctx, failedResult)
	if err != nil {
		t.Errorf("Record should not return error, got: %v", err)
	}

	// 验证更新后的指标
	metrics = monitor.GetTaskMetrics("test-task-1")
	if metrics["total_executions"] != int64(2) {
		t.Errorf("Expected total_executions to be 2, got: %v", metrics["total_executions"])
	}

	if metrics["successful_runs"] != int64(1) {
		t.Errorf("Expected successful_runs to be 1, got: %v", metrics["successful_runs"])
	}

	if metrics["failed_runs"] != int64(1) {
		t.Errorf("Expected failed_runs to be 1, got: %v", metrics["failed_runs"])
	}

	if metrics["error_rate"] != 0.5 {
		t.Errorf("Expected error_rate to be 0.5, got: %v", metrics["error_rate"])
	}
}

func TestTaskMonitor_GetMetrics(t *testing.T) {
	monitor := NewTaskMonitor()
	ctx := context.Background()

	// 记录一些任务
	results := []*Result{
		{
			TaskID:    "task-1",
			Success:   true,
			Duration:  100 * time.Millisecond,
			Timestamp: time.Now(),
		},
		{
			TaskID:    "task-2",
			Success:   false,
			Duration:  200 * time.Millisecond,
			Timestamp: time.Now(),
		},
		{
			TaskID:    "task-1",
			Success:   true,
			Duration:  150 * time.Millisecond,
			Timestamp: time.Now(),
		},
	}

	for _, result := range results {
		err := monitor.Record(ctx, result)
		if err != nil {
			t.Errorf("Record should not return error, got: %v", err)
		}
	}

	// 验证全局指标
	globalMetrics := monitor.GetMetrics()
	if globalMetrics["total_tasks"] != 2 {
		t.Errorf("Expected total_tasks to be 2, got: %v", globalMetrics["total_tasks"])
	}

	if globalMetrics["total_executions"] != int64(3) {
		t.Errorf("Expected total_executions to be 3, got: %v", globalMetrics["total_executions"])
	}

	if globalMetrics["total_successes"] != int64(2) {
		t.Errorf("Expected total_successes to be 2, got: %v", globalMetrics["total_successes"])
	}

	if globalMetrics["total_failures"] != int64(1) {
		t.Errorf("Expected total_failures to be 1, got: %v", globalMetrics["total_failures"])
	}
}

func TestTaskMonitor_GetTaskMetrics_NotFound(t *testing.T) {
	monitor := NewTaskMonitor()
	metrics := monitor.GetTaskMetrics("non-existent-task")

	if metrics["error"] != "task not found" {
		t.Errorf("Expected error message for non-existent task, got: %v", metrics["error"])
	}
}

func TestTaskMonitor_ConcurrentAccess(t *testing.T) {
	monitor := NewTaskMonitor()
	ctx := context.Background()

	// 并发记录任务
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			result := &Result{
				TaskID:    "concurrent-task",
				Success:   i%2 == 0,
				Duration:  time.Duration(i) * time.Millisecond,
				Timestamp: time.Now(),
			}
			_ = monitor.Record(ctx, result)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = monitor.GetTaskMetrics("concurrent-task")
			_ = monitor.GetMetrics()
		}
		done <- true
	}()

	// 等待两个goroutine完成
	<-done
	<-done

	// 验证最终状态
	metrics := monitor.GetTaskMetrics("concurrent-task")
	if metrics["total_executions"] != int64(100) {
		t.Errorf("Expected total_executions to be 100, got: %v", metrics["total_executions"])
	}
}

func TestTaskMonitor_GetTaskExecutionMetrics(t *testing.T) {
	monitor := NewTaskMonitor()
	ctx := context.Background()

	// 记录任务
	result := &Result{
		TaskID:    "test-task",
		Success:   true,
		Duration:  100 * time.Millisecond,
		Timestamp: time.Now(),
		Data:      "test data",
	}

	err := monitor.Record(ctx, result)
	if err != nil {
		t.Fatalf("Record should not return error, got: %v", err)
	}

	// 测试扩展方法
	detailedMetrics, exists := monitor.GetTaskExecutionMetrics("test-task")
	if !exists {
		t.Error("GetTaskExecutionMetrics should return true for existing task")
	}

	if detailedMetrics.TaskID != "test-task" {
		t.Errorf("Expected TaskID to be 'test-task', got: %s", detailedMetrics.TaskID)
	}

	if detailedMetrics.TotalExecutions != 1 {
		t.Errorf("Expected TotalExecutions to be 1, got: %d", detailedMetrics.TotalExecutions)
	}

	if len(detailedMetrics.RecentExecutions) != 1 {
		t.Errorf("Expected 1 recent execution, got: %d", len(detailedMetrics.RecentExecutions))
	}
}

func TestTaskMonitor_GetAllTaskMetrics(t *testing.T) {
	monitor := NewTaskMonitor()
	ctx := context.Background()

	// 记录多个任务
	results := []*Result{
		{TaskID: "task-1", Success: true, Duration: 100 * time.Millisecond, Timestamp: time.Now()},
		{TaskID: "task-2", Success: false, Duration: 200 * time.Millisecond, Timestamp: time.Now()},
		{TaskID: "task-3", Success: true, Duration: 300 * time.Millisecond, Timestamp: time.Now()},
	}

	for _, result := range results {
		_ = monitor.Record(ctx, result)
	}

	// 测试获取所有任务指标
	allMetrics := monitor.GetAllTaskMetrics()
	if len(allMetrics) != 3 {
		t.Errorf("Expected 3 tasks in metrics, got: %d", len(allMetrics))
	}

	for taskID, metrics := range allMetrics {
		if metrics.TaskID != taskID {
			t.Errorf("Expected TaskID %s, got: %s", taskID, metrics.TaskID)
		}
	}
}
