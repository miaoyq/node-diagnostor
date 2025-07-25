package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/miaoyq/node-diagnostor/internal/aggregator"
	"github.com/miaoyq/node-diagnostor/internal/collector"
	"github.com/miaoyq/node-diagnostor/internal/config"
	"github.com/miaoyq/node-diagnostor/internal/processor"
	"github.com/miaoyq/node-diagnostor/internal/reporter"
	"go.uber.org/zap"
)

// Task represents a scheduled diagnostic task
type Task struct {
	ID         string
	Name       string
	Collector  string
	Parameters map[string]interface{}
	Interval   time.Duration
	Priority   int
	Timeout    time.Duration
	LastRun    time.Time
	NextRun    time.Time
	Enabled    bool
	RetryCount int
	MaxRetries int
	Status     TaskStatus    // 新增：任务状态
	StopChan   chan struct{} // 新增：停止通道
	Ticker     *time.Ticker  // 新增：定时器
}

// TaskStatus represents the current status of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusPaused    TaskStatus = "paused"
	TaskStatusStopped   TaskStatus = "stopped"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCompleted TaskStatus = "completed"
)

// Result represents the result of a task execution
type Result struct {
	TaskID    string
	Success   bool
	Error     error
	Data      interface{}
	Duration  time.Duration
	Timestamp time.Time
}

// Scheduler defines the interface for task scheduling
type Scheduler interface {
	// Add adds a new task to the scheduler
	Add(ctx context.Context, task *Task) error

	// Remove removes a task from the scheduler
	Remove(ctx context.Context, taskID string) error

	// Start starts the scheduler
	Start(ctx context.Context) error

	// Stop stops the scheduler
	Stop(ctx context.Context) error

	// GetTask retrieves a task by ID
	GetTask(taskID string) (*Task, error)

	// ListTasks returns all scheduled tasks
	ListTasks() []*Task

	// UpdateTask updates an existing task
	UpdateTask(ctx context.Context, task *Task) error

	// GetStatus returns the scheduler status
	GetStatus() string

	// StartTask starts a specific task
	StartTask(ctx context.Context, taskID string) error

	// StopTask stops a specific task
	StopTask(ctx context.Context, taskID string) error

	// PauseTask pauses a specific task
	PauseTask(ctx context.Context, taskID string) error

	// ResumeTask resumes a specific task
	ResumeTask(ctx context.Context, taskID string) error

	// GetTaskStatus returns the status of a specific task
	GetTaskStatus(taskID string) (TaskStatus, error)

	// SetMaxConcurrent sets the maximum number of concurrent executions
	SetMaxConcurrent(maxConcurrent int)

	// SetProcessor sets the data processor
	SetProcessor(processor processor.Processor)

	// SetReporter sets the data reporter
	SetReporter(reporter *reporter.ReporterClient)

	// OnConfigUpdate add ConfigSubscriber interface implementation for scheduler
	OnConfigUpdate(newConfig *config.Config) error
}

// Executor defines the interface for task execution
type Executor interface {
	// Execute executes a single task
	Execute(ctx context.Context, task *Task) (*Result, error)

	// ExecuteBatch executes multiple tasks
	ExecuteBatch(ctx context.Context, tasks []*Task) ([]*Result, error)

	// GetConcurrencyLimit returns the maximum concurrent executions
	GetConcurrencyLimit() int

	// SetConcurrencyLimit sets the maximum concurrent executions
	SetConcurrencyLimit(limit int) error
}

// Monitor defines the interface for monitoring task execution
type Monitor interface {
	// Record records task execution metrics
	Record(ctx context.Context, result *Result) error

	// GetMetrics returns execution metrics
	GetMetrics() map[string]interface{}

	// GetTaskMetrics returns metrics for a specific task
	GetTaskMetrics(taskID string) map[string]interface{}
}

// CheckScheduler implements the Scheduler interface
type CheckScheduler struct {
	mu            sync.RWMutex
	tasks         map[string]*Task
	taskQueue     *PriorityQueue
	executor      Executor
	monitor       Monitor
	logger        *zap.Logger
	running       bool
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	maxConcurrent int
	semaphore     chan struct{}
	taskTimers    map[string]*time.Timer     // 新增：任务定时器映射
	processor     processor.Processor        // 新增：数据处理器
	reporter      *reporter.ReporterClient   // 新增：数据上报器
	aggregator    *aggregator.DataAggregator // 新增：数据聚合器
}

// New creates a new CheckScheduler instance
func New(executor Executor, monitor Monitor, logger *zap.Logger) *CheckScheduler {
	return &CheckScheduler{
		tasks:         make(map[string]*Task),
		taskQueue:     NewPriorityQueue(),
		executor:      executor,
		monitor:       monitor,
		logger:        logger,
		maxConcurrent: 1,
		semaphore:     make(chan struct{}, 1),
		taskTimers:    make(map[string]*time.Timer),
	}
}

// SetProcessor sets the data processor
func (cs *CheckScheduler) SetProcessor(processor processor.Processor) {
	cs.processor = processor
}

// SetReporter sets the data reporter
func (cs *CheckScheduler) SetReporter(reporter *reporter.ReporterClient) {
	cs.reporter = reporter
}

// SetAggregator sets the data aggregator
func (cs *CheckScheduler) SetAggregator(aggregator *aggregator.DataAggregator) {
	cs.aggregator = aggregator
}

// Add adds a new task to the scheduler
func (cs *CheckScheduler) Add(ctx context.Context, task *Task) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if task.ID == "" {
		return errors.New("task ID cannot be empty")
	}

	if task.Interval <= 0 {
		return errors.New("task interval must be positive")
	}

	if task.Timeout <= 0 {
		task.Timeout = 30 * time.Second // Default timeout
	}

	if task.MaxRetries <= 0 {
		task.MaxRetries = 3 // Default max retries
	}

	// Initialize task fields
	task.Status = TaskStatusPending
	task.StopChan = make(chan struct{})
	task.NextRun = time.Now()
	task.Enabled = true
	task.RetryCount = 0

	cs.tasks[task.ID] = task
	cs.logger.Info("Task added to scheduler",
		zap.String("task_id", task.ID),
		zap.String("name", task.Name),
		zap.Duration("interval", task.Interval))

	// 如果调度器正在运行，立即启动任务定时器
	if cs.running {
		cs.startTaskTimer(task)
	}

	return nil
}

// Remove removes a task from the scheduler
func (cs *CheckScheduler) Remove(ctx context.Context, taskID string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if _, exists := cs.tasks[taskID]; !exists {
		return fmt.Errorf("task %s not found", taskID)
	}

	// 停止任务定时器
	cs.stopTaskTimer(taskID)

	delete(cs.tasks, taskID)
	cs.logger.Info("Task removed from scheduler", zap.String("task_id", taskID))
	return nil
}

// Start starts the scheduler
func (cs *CheckScheduler) Start(ctx context.Context) error {
	cs.mu.Lock()
	if cs.running {
		cs.mu.Unlock()
		return errors.New("scheduler is already running")
	}

	cs.ctx, cs.cancel = context.WithCancel(ctx)
	cs.running = true
	cs.mu.Unlock()

	// Start the scheduling loop
	// go cs.schedulingLoop()

	// Start the execution loop
	go cs.executionLoop()

	// 启动所有任务的定时器
	cs.mu.RLock()
	for _, task := range cs.tasks {
		if task.Enabled {
			cs.startTaskTimer(task)
		}
	}
	cs.mu.RUnlock()

	cs.logger.Info("Check scheduler started")
	return nil
}

// Stop stops the scheduler
func (cs *CheckScheduler) Stop(ctx context.Context) error {
	cs.mu.Lock()
	if !cs.running {
		cs.mu.Unlock()
		return errors.New("scheduler is not running")
	}

	// 停止所有任务定时器
	for taskID := range cs.taskTimers {
		cs.stopTaskTimer(taskID)
	}

	cs.cancel()
	cs.running = false
	cs.mu.Unlock()

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		cs.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		cs.logger.Info("Check scheduler stopped gracefully")
		return nil
	case <-time.After(10 * time.Second):
		cs.logger.Warn("Check scheduler stopped with timeout")
		return errors.New("scheduler stop timeout")
	}
}

// startTaskTimer starts a timer for a specific task
func (cs *CheckScheduler) startTaskTimer(task *Task) {
	if task.Ticker != nil {
		task.Ticker.Stop()
	}

	task.Ticker = time.NewTicker(task.Interval)
	// 在任务启动时，如果任务已经过了下次运行时间，则立即触发一次任务执行
	if task.Enabled && task.Status != TaskStatusPaused {
		cs.taskQueue.Push(task)
	}
	go func() {
		for {
			select {
			case <-task.Ticker.C:
				if task.Enabled && task.Status != TaskStatusPaused {
					cs.taskQueue.Push(task)
				}
			case <-task.StopChan:
				if task.Ticker != nil {
					task.Ticker.Stop()
				}
				return
			case <-cs.ctx.Done():
				if task.Ticker != nil {
					task.Ticker.Stop()
				}
				return
			}
		}
	}()
}

// stopTaskTimer stops the timer for a specific task
func (cs *CheckScheduler) stopTaskTimer(taskID string) {
	if task, exists := cs.tasks[taskID]; exists {
		if task.StopChan != nil {
			close(task.StopChan)
			task.StopChan = nil
		}
		if task.Ticker != nil {
			task.Ticker.Stop()
			task.Ticker = nil
		}
		if timer, exists := cs.taskTimers[taskID]; exists {
			timer.Stop()
			delete(cs.taskTimers, taskID)
		}
	}
}

// schedulingLoop manages the scheduling of tasks based on their intervals
func (cs *CheckScheduler) schedulingLoop() {
	cs.wg.Add(1)
	defer cs.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cs.ctx.Done():
			return
		case <-ticker.C:
			cs.scheduleTasks()
		}
	}
}

// scheduleTasks checks and schedules tasks that are due for execution
func (cs *CheckScheduler) scheduleTasks() {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	now := time.Now()
	for _, task := range cs.tasks {
		if !task.Enabled || task.Status == TaskStatusPaused {
			continue
		}

		cs.logger.Debug("Scheduling task", zap.String("task_id", task.ID), zap.String("nextRunTime", task.NextRun.Format(time.RFC3339)))
		if now.After(task.NextRun) || now.Equal(task.NextRun) {
			// Add to priority queue for execution
			cs.logger.Debug("Task is due for execution", zap.String("task_id", task.ID))
			cs.taskQueue.Push(task)

			// Update next run time
			task.NextRun = now.Add(task.Interval)
			task.LastRun = now
		}
	}
}

// executionLoop manages the execution of scheduled tasks
func (cs *CheckScheduler) executionLoop() {
	cs.wg.Add(1)
	defer cs.wg.Done()

	for {
		select {
		case <-cs.ctx.Done():
			return
		default:
			task := cs.taskQueue.Pop()
			if task == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Acquire semaphore for concurrency control
			select {
			case cs.semaphore <- struct{}{}:
				cs.wg.Add(1)
				go cs.executeTask(task)
			default:
				// Queue is full, re-queue the task
				cs.taskQueue.Push(task)
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

// executeTask executes a single task with error handling and retry logic
func (cs *CheckScheduler) executeTask(task *Task) {
	defer func() {
		<-cs.semaphore
		cs.wg.Done()
	}()

	ctx, cancel := context.WithTimeout(cs.ctx, task.Timeout)
	defer cancel()

	// 更新任务状态为运行中
	cs.mu.Lock()
	task.Status = TaskStatusRunning
	cs.mu.Unlock()

	start := time.Now()

	result, err := cs.executor.Execute(ctx, task)

	duration := time.Since(start)

	// Create result object
	execResult := &Result{
		TaskID:    task.ID,
		Success:   err == nil,
		Error:     err,
		Data:      result,
		Duration:  duration,
		Timestamp: time.Now(),
	}

	// Record metrics
	if cs.monitor != nil {
		if err := cs.monitor.Record(ctx, execResult); err != nil {
			cs.logger.Error("Failed to record task metrics", zap.Error(err))
		}
	}

	// Process and report data if successful
	if err == nil && result != nil {
		cs.processAndReportData(ctx, task, result)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	if err != nil {
		task.RetryCount++
		task.Status = TaskStatusFailed
		cs.logger.Error("Task execution failed",
			zap.String("task_id", task.ID),
			zap.String("name", task.Name),
			zap.Error(err),
			zap.Int("retry_count", task.RetryCount),
			zap.Int("max_retries", task.MaxRetries))

		// Retry logic
		if task.RetryCount < task.MaxRetries {
			// Exponential backoff: 2^retryCount * interval
			backoff := time.Duration(1<<uint(task.RetryCount)) * task.Interval
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute // Cap at 5 minutes
			}

			task.NextRun = time.Now().Add(backoff)
			cs.taskQueue.Push(task)
		} else {
			cs.logger.Error("Task exceeded max retries, disabling",
				zap.String("task_id", task.ID),
				zap.String("name", task.Name))
			task.Enabled = false
			task.Status = TaskStatusStopped
		}
	} else {
		// Reset retry count on success
		task.RetryCount = 0
		task.Status = TaskStatusCompleted
		cs.logger.Debug("Task executed successfully",
			zap.String("task_id", task.ID),
			zap.String("name", task.Name),
			zap.Duration("duration", duration))
	}
}

// processAndReportData processes collected data and reports it
func (cs *CheckScheduler) processAndReportData(ctx context.Context, task *Task, result interface{}) {
	if cs.processor == nil || cs.reporter == nil {
		cs.logger.Warn("Processor or reporter not configured, skipping data processing and reporting")
		return
	}

	// 尝试从结果中提取collector结果
	var collectorResults []*collector.Result

	// 处理不同类型的结果数据
	switch v := result.(type) {
	case map[string]interface{}:
		// 从scheduler executor的结果中提取
		if results, ok := v["results"].([]*collector.Result); ok {
			collectorResults = results
		} else {
			// 单个结果的情况
			collectorResults = []*collector.Result{
				{
					Data: &collector.Data{
						Type:      task.Collector,
						Timestamp: time.Now(),
						Source:    task.Collector,
						Data:      v,
						Metadata:  make(map[string]string),
					},
					Error:   nil,
					Skipped: false,
				},
			}
		}
	default:
		// 处理其他类型的结果
		collectorResults = []*collector.Result{
			{
				Data: &collector.Data{
					Type:      task.Collector,
					Timestamp: time.Now(),
					Source:    task.Collector,
					Data:      map[string]interface{}{"raw": v},
					Metadata:  make(map[string]string),
				},
				Error:   nil,
				Skipped: false,
			},
		}
	}

	// 使用DataAggregator聚合数据
	if cs.aggregator != nil && len(collectorResults) > 0 {
		aggregatedData, err := cs.aggregator.Aggregate(ctx, task.Name, collectorResults)
		if err != nil {
			cs.logger.Error("Failed to aggregate data", zap.Error(err))
			// 如果聚合失败，继续使用原始数据
		} else {
			// 使用聚合后的数据
			result = map[string]interface{}{
				"aggregated": aggregatedData,
				"task_name":  task.Name,
				"task_id":    task.ID,
				"timestamp":  time.Now(),
			}
		}
	}

	// Process the collected/aggregated data
	processedData, err := cs.processor.Process(ctx, result)
	if err != nil {
		cs.logger.Error("Failed to process data", zap.Error(err))
		return
	}

	// Create report from processed data
	report := &reporter.Report{
		ID:        fmt.Sprintf("%s-%d", task.Name, time.Now().Unix()),
		Data:      processedData.Data,
		Timestamp: processedData.Timestamp,
		NodeName:  processedData.NodeName,
		CheckName: task.Name,
	}

	// Report the data
	_, err = cs.reporter.Report(ctx, report)
	if err != nil {
		cs.logger.Error("Failed to report data", zap.Error(err))
	} else {
		cs.logger.Debug("Data reported successfully", zap.String("task_id", task.ID))
	}
}

// GetTask retrieves a task by ID
func (cs *CheckScheduler) GetTask(taskID string) (*Task, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	task, exists := cs.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	return task, nil
}

// ListTasks returns all scheduled tasks
func (cs *CheckScheduler) ListTasks() []*Task {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	tasks := make([]*Task, 0, len(cs.tasks))
	for _, task := range cs.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// UpdateTask updates an existing task
func (cs *CheckScheduler) UpdateTask(ctx context.Context, task *Task) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if _, exists := cs.tasks[task.ID]; !exists {
		return fmt.Errorf("task %s not found", task.ID)
	}

	// Stop existing timer
	cs.stopTaskTimer(task.ID)

	// Update task
	cs.tasks[task.ID] = task

	// Restart timer if enabled and running
	if task.Enabled && cs.running {
		cs.startTaskTimer(task)
	}

	cs.logger.Info("Task updated", zap.String("task_id", task.ID))
	return nil
}

// GetStatus returns the scheduler status
func (cs *CheckScheduler) GetStatus() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.running {
		return "running"
	}
	return "stopped"
}

// StartTask starts a specific task
func (cs *CheckScheduler) StartTask(ctx context.Context, taskID string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	task, exists := cs.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %s not found", taskID)
	}

	if task.Enabled {
		return fmt.Errorf("task %s is already enabled", taskID)
	}

	task.Enabled = true
	task.Status = TaskStatusPending

	if cs.running {
		cs.startTaskTimer(task)
	}

	cs.logger.Info("Task started", zap.String("task_id", taskID))
	return nil
}

// StopTask stops a specific task
func (cs *CheckScheduler) StopTask(ctx context.Context, taskID string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	task, exists := cs.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %s not found", taskID)
	}

	if !task.Enabled {
		return fmt.Errorf("task %s is already disabled", taskID)
	}

	task.Enabled = false
	task.Status = TaskStatusStopped
	cs.stopTaskTimer(taskID)

	cs.logger.Info("Task stopped", zap.String("task_id", taskID))
	return nil
}

// PauseTask pauses a specific task
func (cs *CheckScheduler) PauseTask(ctx context.Context, taskID string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	task, exists := cs.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %s not found", taskID)
	}

	if task.Status == TaskStatusPaused {
		return fmt.Errorf("task %s is already paused", taskID)
	}

	task.Status = TaskStatusPaused
	cs.stopTaskTimer(taskID)

	cs.logger.Info("Task paused", zap.String("task_id", taskID))
	return nil
}

// ResumeTask resumes a specific task
func (cs *CheckScheduler) ResumeTask(ctx context.Context, taskID string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	task, exists := cs.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %s not found", taskID)
	}

	if task.Status != TaskStatusPaused {
		return fmt.Errorf("task %s is not paused", taskID)
	}

	task.Status = TaskStatusPending

	if cs.running && task.Enabled {
		cs.startTaskTimer(task)
	}

	cs.logger.Info("Task resumed", zap.String("task_id", taskID))
	return nil
}

// GetTaskStatus returns the status of a specific task
func (cs *CheckScheduler) GetTaskStatus(taskID string) (TaskStatus, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	task, exists := cs.tasks[taskID]
	if !exists {
		return "", fmt.Errorf("task %s not found", taskID)
	}
	return task.Status, nil
}

// SetMaxConcurrent sets the maximum number of concurrent executions
func (cs *CheckScheduler) SetMaxConcurrent(maxConcurrent int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	// Update semaphore
	oldSemaphore := cs.semaphore
	cs.semaphore = make(chan struct{}, maxConcurrent)
	cs.maxConcurrent = maxConcurrent

	// Fill new semaphore with existing permits
	for i := 0; i < len(oldSemaphore); i++ {
		select {
		case <-oldSemaphore:
			cs.semaphore <- struct{}{}
		default:
			break
		}
	}

	cs.logger.Info("Max concurrent executions updated", zap.Int("max_concurrent", maxConcurrent))
}

// GetMetrics returns scheduler metrics
func (cs *CheckScheduler) GetMetrics() map[string]interface{} {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// 统计各种状态的任务数量
	statusCounts := make(map[TaskStatus]int)
	for _, task := range cs.tasks {
		statusCounts[task.Status]++
	}

	return map[string]interface{}{
		"running":        cs.running,
		"total_tasks":    len(cs.tasks),
		"queued_tasks":   cs.taskQueue.Len(),
		"max_concurrent": cs.maxConcurrent,
		"status_counts":  statusCounts,
	}
}

// Add ConfigSubscriber interface implementation for scheduler
func (cs *CheckScheduler) OnConfigUpdate(newConfig *config.Config) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.logger.Info("Scheduler received config update")

	// Update scheduler configuration
	if newConfig.ResourceLimits.MaxConcurrent > 0 {
		cs.SetMaxConcurrent(newConfig.ResourceLimits.MaxConcurrent)
	}

	// Remove all existing tasks
	tasks := cs.ListTasks()
	for _, task := range tasks {
		if err := cs.Remove(context.Background(), task.ID); err != nil {
			cs.logger.Error("Failed to remove task during config update",
				zap.String("task_id", task.ID), zap.Error(err))
		}
	}

	// Add new tasks from configuration
	addedTasks := 0
	for _, check := range newConfig.Checks {
		if !check.Enabled {
			continue
		}

		// Create a task for each collector in the check
		for _, collectorID := range check.Collectors {
			taskID := fmt.Sprintf("%s-%s", check.Name, collectorID)
			task := &Task{
				ID:         taskID,
				Name:       check.Name,
				Collector:  collectorID,
				Parameters: check.Params,
				Interval:   check.Interval,
				Priority:   check.Priority,
				Timeout:    check.Timeout,
				Enabled:    true,
				MaxRetries: 3,
				Status:     TaskStatusPending,
			}

			if err := cs.Add(context.Background(), task); err != nil {
				cs.logger.Error("Failed to add task from updated config",
					zap.String("task_id", taskID), zap.Error(err))
				continue
			}
			addedTasks++
		}
	}

	cs.logger.Info("Scheduler config update completed", zap.Int("task_count", addedTasks))
	return nil
}
