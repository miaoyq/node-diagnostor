package scheduler

import (
	"context"
	"sync"
	"time"
)

// TaskMonitor 实现了scheduler.Monitor接口，用于监控任务执行
type TaskMonitor struct {
	mu          sync.RWMutex
	taskMetrics map[string]*TaskExecutionMetrics
	globalStats *GlobalMetrics
}

// TaskExecutionMetrics 存储单个任务的执行指标
type TaskExecutionMetrics struct {
	TaskID           string
	TotalExecutions  int64
	SuccessfulRuns   int64
	FailedRuns       int64
	LastExecution    time.Time
	LastDuration     time.Duration
	LastError        error
	AverageDuration  time.Duration
	TotalDuration    time.Duration
	ErrorRate        float64
	SuccessRate      float64
	RecentExecutions []ExecutionRecord
}

// ExecutionRecord 记录单次执行详情
type ExecutionRecord struct {
	Timestamp time.Time
	Success   bool
	Duration  time.Duration
	Error     error
	Data      interface{}
}

// GlobalMetrics 存储全局执行统计
type GlobalMetrics struct {
	TotalTasks         int
	ActiveTasks        int
	TotalExecutions    int64
	TotalSuccesses     int64
	TotalFailures      int64
	OverallErrorRate   float64
	OverallSuccessRate float64
	LastUpdated        time.Time
}

// NewTaskMonitor 创建新的任务监控器
func NewTaskMonitor() *TaskMonitor {
	return &TaskMonitor{
		taskMetrics: make(map[string]*TaskExecutionMetrics),
		globalStats: &GlobalMetrics{
			LastUpdated: time.Now(),
		},
	}
}

// Record 记录任务执行结果
func (tm *TaskMonitor) Record(ctx context.Context, result *Result) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	taskID := result.TaskID
	if _, exists := tm.taskMetrics[taskID]; !exists {
		tm.taskMetrics[taskID] = &TaskExecutionMetrics{
			TaskID:           taskID,
			RecentExecutions: make([]ExecutionRecord, 0, 10),
		}
	}

	metrics := tm.taskMetrics[taskID]
	metrics.TotalExecutions++
	metrics.LastExecution = result.Timestamp
	metrics.LastDuration = result.Duration
	metrics.LastError = result.Error

	// 添加执行记录
	record := ExecutionRecord{
		Timestamp: result.Timestamp,
		Success:   result.Success,
		Duration:  result.Duration,
		Error:     result.Error,
		Data:      result.Data,
	}

	// 保持最近10条记录
	if len(metrics.RecentExecutions) >= 10 {
		metrics.RecentExecutions = metrics.RecentExecutions[1:]
	}
	metrics.RecentExecutions = append(metrics.RecentExecutions, record)

	// 更新成功/失败计数
	if result.Success {
		metrics.SuccessfulRuns++
	} else {
		metrics.FailedRuns++
	}

	// 计算总持续时间
	metrics.TotalDuration += result.Duration

	// 计算平均持续时间
	if metrics.TotalExecutions > 0 {
		metrics.AverageDuration = time.Duration(int64(metrics.TotalDuration) / metrics.TotalExecutions)
	}

	// 计算成功率
	if metrics.TotalExecutions > 0 {
		metrics.SuccessRate = float64(metrics.SuccessfulRuns) / float64(metrics.TotalExecutions)
		metrics.ErrorRate = float64(metrics.FailedRuns) / float64(metrics.TotalExecutions)
	}

	// 更新全局统计
	tm.updateGlobalStats()

	return nil
}

// GetMetrics 返回全局执行指标
func (tm *TaskMonitor) GetMetrics() map[string]interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return map[string]interface{}{
		"total_tasks":          tm.globalStats.TotalTasks,
		"active_tasks":         tm.globalStats.ActiveTasks,
		"total_executions":     tm.globalStats.TotalExecutions,
		"total_successes":      tm.globalStats.TotalSuccesses,
		"total_failures":       tm.globalStats.TotalFailures,
		"overall_error_rate":   tm.globalStats.OverallErrorRate,
		"overall_success_rate": tm.globalStats.OverallSuccessRate,
		"last_updated":         tm.globalStats.LastUpdated,
	}
}

// GetTaskMetrics 返回特定任务的执行指标
func (tm *TaskMonitor) GetTaskMetrics(taskID string) map[string]interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	metrics, exists := tm.taskMetrics[taskID]
	if !exists {
		return map[string]interface{}{
			"error": "task not found",
		}
	}

	return map[string]interface{}{
		"task_id":          metrics.TaskID,
		"total_executions": metrics.TotalExecutions,
		"successful_runs":  metrics.SuccessfulRuns,
		"failed_runs":      metrics.FailedRuns,
		"last_execution":   metrics.LastExecution,
		"last_duration_ms": metrics.LastDuration.Milliseconds(),
		"last_error": func() string {
			if metrics.LastError != nil {
				return metrics.LastError.Error()
			}
			return ""
		}(),
		"average_duration_ms": metrics.AverageDuration.Milliseconds(),
		"error_rate":          metrics.ErrorRate,
		"success_rate":        metrics.SuccessRate,
		"recent_executions":   len(metrics.RecentExecutions),
	}
}

// updateGlobalStats 更新全局统计信息
func (tm *TaskMonitor) updateGlobalStats() {
	totalExecutions := int64(0)
	totalSuccesses := int64(0)
	totalFailures := int64(0)

	for _, metrics := range tm.taskMetrics {
		totalExecutions += metrics.TotalExecutions
		totalSuccesses += metrics.SuccessfulRuns
		totalFailures += metrics.FailedRuns
	}

	tm.globalStats.TotalTasks = len(tm.taskMetrics)
	tm.globalStats.TotalExecutions = totalExecutions
	tm.globalStats.TotalSuccesses = totalSuccesses
	tm.globalStats.TotalFailures = totalFailures
	tm.globalStats.ActiveTasks = tm.countActiveTasks()

	if totalExecutions > 0 {
		tm.globalStats.OverallSuccessRate = float64(totalSuccesses) / float64(totalExecutions)
		tm.globalStats.OverallErrorRate = float64(totalFailures) / float64(totalExecutions)
	}

	tm.globalStats.LastUpdated = time.Now()
}

// countActiveTasks 计算活跃任务数量
func (tm *TaskMonitor) countActiveTasks() int {
	active := 0
	for _, metrics := range tm.taskMetrics {
		if metrics.TotalExecutions > 0 {
			active++
		}
	}
	return active
}

// GetTaskExecutionMetrics 获取任务的详细执行指标（扩展方法）
func (tm *TaskMonitor) GetTaskExecutionMetrics(taskID string) (*TaskExecutionMetrics, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	metrics, exists := tm.taskMetrics[taskID]
	if !exists {
		return nil, false
	}

	// 返回副本以避免并发问题
	copyMetrics := *metrics
	copyMetrics.RecentExecutions = make([]ExecutionRecord, len(metrics.RecentExecutions))
	copy(copyMetrics.RecentExecutions, metrics.RecentExecutions)
	return &copyMetrics, true
}

// GetAllTaskMetrics 获取所有任务的指标（扩展方法）
func (tm *TaskMonitor) GetAllTaskMetrics() map[string]*TaskExecutionMetrics {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make(map[string]*TaskExecutionMetrics)
	for taskID, metrics := range tm.taskMetrics {
		copyMetrics := *metrics
		copyMetrics.RecentExecutions = make([]ExecutionRecord, len(metrics.RecentExecutions))
		copy(copyMetrics.RecentExecutions, metrics.RecentExecutions)
		result[taskID] = &copyMetrics
	}
	return result
}
