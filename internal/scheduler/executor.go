package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/miaoyq/node-diagnostor/internal/collector"
	"github.com/miaoyq/node-diagnostor/internal/datascope"
	"go.uber.org/zap"
)

// CheckExecutor implements the Executor interface for diagnostic checks
type CheckExecutor struct {
	registry  collector.Registry
	validator datascope.Validator
	logger    *zap.Logger
}

// NewCheckExecutor creates a new check executor
func NewCheckExecutor(registry collector.Registry, validator datascope.Validator) *CheckExecutor {
	return &CheckExecutor{
		registry:  registry,
		validator: validator,
		logger:    zap.L(),
	}
}

// Execute executes a single diagnostic check task
func (e *CheckExecutor) Execute(ctx context.Context, task *Task) (*Result, error) {
	e.logger.Debug("Executing check task",
		zap.String("task_id", task.ID),
		zap.String("name", task.Name),
		zap.Duration("timeout", task.Timeout))

	// Validate task parameters
	if task.Name == "" {
		return nil, fmt.Errorf("task name cannot be empty")
	}

	// Get collectors for this task
	collectors := e.getCollectorsForTask(task)
	if len(collectors) == 0 {
		return nil, fmt.Errorf("no collectors found for task: %s", task.Name)
	}

	// Execute collectors and collect results
	results := make([]*collector.Result, 0, len(collectors))
	for _, coll := range collectors {
		// Validate collector based on scope
		validation, err := e.validator.ValidateCollector(ctx, coll.Name(), datascope.ScopeAuto)
		if err != nil {
			e.logger.Warn("Failed to validate collector",
				zap.String("collector", coll.Name()),
				zap.Error(err))
			continue
		}

		if !validation.Valid || validation.Skipped {
			e.logger.Debug("Collector skipped",
				zap.String("collector", coll.Name()),
				zap.String("reason", validation.Reason))
			results = append(results, &collector.Result{
				Skipped: true,
				Reason:  validation.Reason,
			})
			continue
		}

		// Execute collector
		result, err := e.executeCollector(ctx, coll, nil)
		if err != nil {
			e.logger.Error("Collector execution failed",
				zap.String("collector", coll.Name()),
				zap.Error(err))
			results = append(results, &collector.Result{
				Error: err,
			})
			continue
		}

		results = append(results, result)
	}

	// Aggregate results
	aggregated := e.aggregateResults(task, results)
	return &Result{
		TaskID:    task.ID,
		Success:   true,
		Data:      aggregated,
		Duration:  time.Since(time.Now()),
		Timestamp: time.Now(),
	}, nil
}

// ExecuteBatch executes multiple tasks in batch
func (e *CheckExecutor) ExecuteBatch(ctx context.Context, tasks []*Task) ([]*Result, error) {
	results := make([]*Result, 0, len(tasks))
	for _, task := range tasks {
		result, err := e.Execute(ctx, task)
		if err != nil {
			e.logger.Error("Task execution failed",
				zap.String("task_id", task.ID),
				zap.Error(err))
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

// GetConcurrencyLimit returns the maximum concurrent executions
func (e *CheckExecutor) GetConcurrencyLimit() int {
	return 1 // Default to 1 for resource conservation
}

// SetConcurrencyLimit sets the maximum concurrent executions
func (e *CheckExecutor) SetConcurrencyLimit(limit int) error {
	if limit < 1 {
		return fmt.Errorf("concurrency limit must be at least 1")
	}
	return nil
}

// getCollectorsForTask returns collectors for a given task
func (e *CheckExecutor) getCollectorsForTask(task *Task) []collector.Collector {
	var collectors []collector.Collector

	// Use task name as collector name
	if coll, err := e.registry.Get(task.Name); err == nil {
		collectors = append(collectors, coll)
	}

	return collectors
}

// executeCollector executes a single collector with timeout
func (e *CheckExecutor) executeCollector(ctx context.Context, coll collector.Collector, params map[string]interface{}) (*collector.Result, error) {
	// Set collector-specific timeout
	timeout := coll.Timeout()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return coll.Collect(ctx, params)
}

// aggregateResults aggregates multiple collector results into a single result
func (e *CheckExecutor) aggregateResults(task *Task, results []*collector.Result) map[string]interface{} {
	aggregated := map[string]interface{}{
		"task_name": task.Name,
		"task_id":   task.ID,
		"results":   results,
		"timestamp": time.Now(),
	}

	// Count successful, failed, and skipped results
	successCount := 0
	failedCount := 0
	skippedCount := 0

	for _, result := range results {
		if result.Skipped {
			skippedCount++
		} else if result.Error != nil {
			failedCount++
		} else {
			successCount++
		}
	}

	aggregated["summary"] = map[string]int{
		"total":   len(results),
		"success": successCount,
		"failed":  failedCount,
		"skipped": skippedCount,
	}

	return aggregated
}
