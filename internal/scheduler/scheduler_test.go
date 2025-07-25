package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestNewCheckScheduler(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)

	scheduler := New(executor, monitor, logger)
	assert.NotNil(t, scheduler)
	assert.Equal(t, 1, scheduler.maxConcurrent)
}

func TestAddTask(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	task := &Task{
		ID:       "test-task",
		Name:     "Test Task",
		Interval: 1 * time.Second,
		Priority: 1,
	}

	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	retrievedTask, err := scheduler.GetTask("test-task")
	assert.NoError(t, err)
	assert.Equal(t, "test-task", retrievedTask.ID)
	assert.Equal(t, 30*time.Second, retrievedTask.Timeout) // Default timeout
	assert.Equal(t, 3, retrievedTask.MaxRetries)           // Default max retries
	assert.Equal(t, TaskStatusPending, retrievedTask.Status)
}

func TestAddTaskValidation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	tests := []struct {
		name    string
		task    *Task
		wantErr bool
	}{
		{
			name:    "empty ID",
			task:    &Task{ID: "", Name: "Test", Interval: 1 * time.Second},
			wantErr: true,
		},
		{
			name:    "zero interval",
			task:    &Task{ID: "test", Name: "Test", Interval: 0},
			wantErr: true,
		},
		{
			name:    "negative interval",
			task:    &Task{ID: "test", Name: "Test", Interval: -1 * time.Second},
			wantErr: true,
		},
		{
			name:    "valid task",
			task:    &Task{ID: "test", Name: "Test", Interval: 1 * time.Second},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := scheduler.Add(context.Background(), tt.task)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRemoveTask(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	task := &Task{
		ID:       "test-task",
		Name:     "Test Task",
		Interval: 1 * time.Second,
	}

	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	err = scheduler.Remove(context.Background(), "test-task")
	assert.NoError(t, err)

	_, err = scheduler.GetTask("test-task")
	assert.Error(t, err)
}

func TestTaskScheduling(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Mock successful execution with call counter
	var executionCount int
	executor.On("Execute", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		executionCount++
	}).Return(&Result{}, nil)
	monitor.On("Record", mock.Anything, mock.Anything).Return(nil)

	task := &Task{
		ID:       "test-task",
		Name:     "Test Task",
		Interval: 100 * time.Millisecond,
		Priority: 1,
	}

	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = scheduler.Start(ctx)
	assert.NoError(t, err)

	// Wait for executions (should get 3-5 executions in 500ms with 100ms interval)
	time.Sleep(500 * time.Millisecond)

	err = scheduler.Stop(context.Background())
	assert.NoError(t, err)

	// Verify execution count
	assert.GreaterOrEqual(t, executionCount, 3, "Expected at least 3 executions")
	assert.LessOrEqual(t, executionCount, 5, "Expected at most 5 executions")

	executor.AssertExpectations(t)
	monitor.AssertExpectations(t)
}

func TestPriorityScheduling(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Mock execution with call order verification
	callOrder := []string{}
	executor.On("Execute", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		task := args.Get(1).(*Task)
		callOrder = append(callOrder, task.ID)
	}).Return(&Result{}, nil)
	monitor.On("Record", mock.Anything, mock.Anything).Return(nil)

	// Add tasks with different priorities
	tasks := []*Task{
		{ID: "low-priority", Name: "Low", Interval: 100 * time.Millisecond, Priority: 1},
		{ID: "high-priority", Name: "High", Interval: 100 * time.Millisecond, Priority: 3},
	}

	for _, task := range tasks {
		err := scheduler.Add(context.Background(), task)
		assert.NoError(t, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := scheduler.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	err = scheduler.Stop(context.Background())
	assert.NoError(t, err)

	// Verify high priority task executed first
	if len(callOrder) > 0 {
		assert.Equal(t, "high-priority", callOrder[0], "High priority task should execute first")
	}
}

func TestPriorityQueue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pq := NewPriorityQueue()

	// 测试高优先级任务先执行
	highPriorityTask := &Task{Priority: 10}
	lowPriorityTask := &Task{Priority: 1}

	pq.Push(lowPriorityTask)
	pq.Push(highPriorityTask)

	assert.Equal(t, highPriorityTask, pq.Pop(), "高优先级任务应该先执行")
	assert.Equal(t, lowPriorityTask, pq.Pop(), "然后是低优先级任务")
}

func TestConcurrencyControl(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Set max concurrent to 1
	scheduler.SetMaxConcurrent(1)

	// Mock slow execution
	executor.On("Execute", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Second)
	}).Return(&Result{}, nil)

	monitor.On("Record", mock.Anything, mock.Anything).Return(nil)

	// Add multiple tasks
	for i := 0; i < 3; i++ {
		task := &Task{
			ID:       fmt.Sprintf("task-%d", i),
			Name:     fmt.Sprintf("Task %d", i),
			Interval: 2 * time.Second,
			Priority: 1,
		}
		err := scheduler.Add(context.Background(), task)
		assert.NoError(t, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := scheduler.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(3 * time.Second) // 调度触发周期为 1s，3个task不能并发执行， 因此等待 4s 后，所有任务都应该执行完毕

	err = scheduler.Stop(context.Background())
	assert.NoError(t, err)

	// Verify only one task was executed at a time
	executor.AssertNumberOfCalls(t, "Execute", 3)
}

func TestErrorRetry(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Mock failing execution
	executor.On("Execute", mock.Anything, mock.Anything).Return((*Result)(nil), errors.New("execution failed"))
	monitor.On("Record", mock.Anything, mock.Anything).Return(nil)

	task := &Task{
		ID:         "retry-task",
		Name:       "Retry Task",
		Interval:   50 * time.Millisecond,
		Priority:   1,
		MaxRetries: 2,
	}

	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = scheduler.Start(ctx)
	assert.NoError(t, err)

	// Wait for retries
	time.Sleep(300 * time.Millisecond)

	err = scheduler.Stop(context.Background())
	assert.NoError(t, err)

	// Verify task was retried and then disabled
	retrievedTask, err := scheduler.GetTask("retry-task")
	assert.NoError(t, err)
	assert.False(t, retrievedTask.Enabled)
	assert.GreaterOrEqual(t, retrievedTask.RetryCount, 2)
}

func TestTaskUpdate(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	task := &Task{
		ID:       "update-task",
		Name:     "Original Name",
		Interval: 1 * time.Second,
	}

	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	// Update task
	updatedTask := &Task{
		ID:       "update-task",
		Name:     "Updated Name",
		Interval: 2 * time.Second,
	}

	err = scheduler.UpdateTask(context.Background(), updatedTask)
	assert.NoError(t, err)

	retrievedTask, err := scheduler.GetTask("update-task")
	assert.NoError(t, err)
	assert.Equal(t, "Updated Name", retrievedTask.Name)
	assert.Equal(t, 2*time.Second, retrievedTask.Interval)
}

func TestSchedulerStatus(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Test stopped status
	status := scheduler.GetStatus()
	assert.Contains(t, status, "stopped")

	// Add some tasks
	task := &Task{
		ID:       "status-task",
		Name:     "Status Task",
		Interval: 1 * time.Second,
	}
	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	// Test running status
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = scheduler.Start(ctx)
	assert.NoError(t, err)

	status = scheduler.GetStatus()
	assert.Contains(t, status, "running")
	assert.Contains(t, status, "1 tasks")

	err = scheduler.Stop(context.Background())
	assert.NoError(t, err)
}

func TestStopTimeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Mock long-running task
	executor.On("Execute", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(15 * time.Second)
	}).Return(&Result{}, nil)
	monitor.On("Record", mock.Anything, mock.Anything).Return(nil)

	task := &Task{
		ID:       "long-task",
		Name:     "Long Task",
		Interval: 5 * time.Second,
		Timeout:  1 * time.Second,
	}

	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	err = scheduler.Start(ctx)
	assert.NoError(t, err)

	// make sure the task is scheduled and is running
	time.Sleep(2 * time.Second)

	// Force stop with timeout
	err = scheduler.Stop(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestMetrics(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	metrics := scheduler.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Contains(t, metrics, "total_tasks")
	assert.Contains(t, metrics, "max_concurrent")
}

func TestConcurrentAccess(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Add tasks concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			task := &Task{
				ID:       fmt.Sprintf("concurrent-task-%d", id),
				Name:     fmt.Sprintf("Concurrent Task %d", id),
				Interval: 1 * time.Second,
			}
			_ = scheduler.Add(context.Background(), task)
		}(i)
	}
	wg.Wait()

	// Verify all tasks were added
	assert.Equal(t, 10, len(scheduler.ListTasks()))
}

func TestTaskRetryLogic(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Mock failing execution
	executor.On("Execute", mock.Anything, mock.Anything).Return((*Result)(nil), errors.New("execution failed"))
	monitor.On("Record", mock.Anything, mock.Anything).Return(nil)

	task := &Task{
		ID:         "retry-task",
		Name:       "Retry Task",
		Interval:   5 * time.Second,
		Priority:   1,
		MaxRetries: 3,
		Enabled:    true,
	}

	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = scheduler.Start(ctx)
	assert.NoError(t, err)

	// Wait for retries
	time.Sleep(2 * time.Second)

	err = scheduler.Stop(context.Background())
	assert.NoError(t, err)

	// Verify task was retried and then disabled
	retrievedTask, err := scheduler.GetTask("retry-task")
	assert.NoError(t, err)
	assert.False(t, retrievedTask.Enabled, "任务应该被禁用")
	assert.Equal(t, task.MaxRetries, retrievedTask.RetryCount, "重试计数应该达到最大值")
}

func TestSchedulerWithTimeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Set timeout to 100ms
	// scheduler.SetTaskTimeout(100 * time.Millisecond)

	task := &Task{
		ID:       "timeout-task",
		Name:     "Timeout Task",
		Interval: 10 * time.Minute,
		Timeout:  3 * time.Second,
	}

	// Mock long running execution
	executor.On("Execute", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		t.Log("executing task")
		time.Sleep(200 * time.Millisecond)
	}).Return(&Result{}, nil)

	monitor.On("Record", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		t.Log("Record metrics")
		time.Sleep(200 * time.Millisecond)
	}).Return(nil)

	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = scheduler.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(1 * time.Second)

	err = scheduler.Stop(ctx)
	assert.NoError(t, err)
}

// TestTaskControl tests the task start/stop/pause/resume functionality
func TestTaskControl(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Mock successful execution
	var executionCount int
	executor.On("Execute", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		executionCount++
	}).Return(&Result{}, nil)
	monitor.On("Record", mock.Anything, mock.Anything).Return(nil)

	task := &Task{
		ID:       "control-task",
		Name:     "Control Task",
		Interval: 5 * time.Second,
		Priority: 1,
	}

	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = scheduler.Start(ctx)
	assert.NoError(t, err)

	// Let it run for a bit
	time.Sleep(2 * time.Second)

	// Test pause
	err = scheduler.PauseTask(ctx, "control-task")
	assert.NoError(t, err)
	status, _ := scheduler.GetTaskStatus("control-task")
	assert.Equal(t, TaskStatusPaused, status)

	// Wait and verify no new executions during pause
	currentCount := executionCount
	time.Sleep(2 * time.Second)
	assert.Equal(t, currentCount, executionCount, "No executions during pause")

	// Test resume
	err = scheduler.ResumeTask(ctx, "control-task")
	assert.NoError(t, err)
	status, _ = scheduler.GetTaskStatus("control-task")
	assert.Equal(t, TaskStatusPending, status)

	// Test stop
	err = scheduler.StopTask(ctx, "control-task")
	assert.NoError(t, err)
	status, _ = scheduler.GetTaskStatus("control-task")
	assert.Equal(t, TaskStatusStopped, status)

	// Test start
	err = scheduler.StartTask(ctx, "control-task")
	assert.NoError(t, err)
	status, _ = scheduler.GetTaskStatus("control-task")
	assert.Equal(t, TaskStatusPending, status)

	err = scheduler.Stop(context.Background())
	assert.NoError(t, err)
}

// TestTaskStatusTracking tests the task status tracking functionality
func TestTaskStatusTracking(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Mock execution
	executor.On("Execute", mock.Anything, mock.Anything).Return(&Result{}, nil)
	monitor.On("Record", mock.Anything, mock.Anything).Return(nil)

	task := &Task{
		ID:       "status-task",
		Name:     "Status Task",
		Interval: 5 * time.Second,
		Priority: 1,
	}

	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	// Check initial status
	status, err := scheduler.GetTaskStatus("status-task")
	assert.NoError(t, err)
	assert.Equal(t, TaskStatusPending, status)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = scheduler.Start(ctx)
	assert.NoError(t, err)

	// Wait for execution
	time.Sleep(2 * time.Second)

	// Check status after execution
	retrievedTask, err := scheduler.GetTask("status-task")
	assert.NoError(t, err)
	assert.Equal(t, TaskStatusCompleted, retrievedTask.Status)

	err = scheduler.Stop(context.Background())
	assert.NoError(t, err)
}

// TestTaskTimer tests the time.Ticker based scheduling
func TestTaskTimer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Mock execution
	var executionTimes []time.Time
	executor.On("Execute", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		executionTimes = append(executionTimes, time.Now())
	}).Return(&Result{}, nil)
	monitor.On("Record", mock.Anything, mock.Anything).Return(nil)

	task := &Task{
		ID:       "timer-task",
		Name:     "Timer Task",
		Interval: 1 * time.Second,
		Priority: 1,
	}

	err := scheduler.Add(context.Background(), task)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = scheduler.Start(ctx)
	assert.NoError(t, err)

	// Wait for multiple executions
	time.Sleep(4 * time.Second)

	err = scheduler.Stop(context.Background())
	assert.NoError(t, err)

	// Verify executions are at regular intervals
	assert.GreaterOrEqual(t, len(executionTimes), 3)
	for i := 1; i < len(executionTimes); i++ {
		interval := executionTimes[i].Sub(executionTimes[i-1])
		assert.InDelta(t, 1*time.Second, interval, float64(20*time.Millisecond))
	}
}

// TestTaskMetrics tests the metrics collection
func TestTaskMetrics(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	executor := new(MockExecutor)
	monitor := new(MockMonitor)
	scheduler := New(executor, monitor, logger)

	// Add tasks with different statuses
	tasks := []*Task{
		{ID: "task1", Name: "Task 1", Interval: 1 * time.Second, Status: TaskStatusPending},
		{ID: "task2", Name: "Task 2", Interval: 1 * time.Second, Status: TaskStatusRunning},
		{ID: "task3", Name: "Task 3", Interval: 1 * time.Second, Status: TaskStatusPaused},
	}

	for _, task := range tasks {
		err := scheduler.Add(context.Background(), task)
		assert.NoError(t, err)
	}

	metrics := scheduler.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Contains(t, metrics, "status_counts")

	statusCounts := metrics["status_counts"].(map[TaskStatus]int)
	assert.Equal(t, 3, statusCounts[TaskStatusPending]) // Adding task will reset status to pending
}
