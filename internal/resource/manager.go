package resource

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CheckContext 存储检查项的执行上下文
type CheckContext struct {
	Name      string
	Priority  int
	StartTime time.Time
	Status    CheckStatus
	CancelFn  context.CancelFunc
	Timeout   time.Duration
}

// CheckStatus 表示检查项状态
type CheckStatus string

const (
	CheckStatusActive    CheckStatus = "active"
	CheckStatusThrottled CheckStatus = "throttled"
	CheckStatusPaused    CheckStatus = "paused"
)

// ResourceManager 负责管理资源使用和检查项调度
type ResourceManager struct {
	mu              sync.RWMutex
	logger          *zap.Logger
	monitor         *ResourceMonitor
	limits          ResourceLimits
	throttler       *Throttler
	checkPriorities map[string]int
	activeChecks    map[string]*CheckContext
	isThrottled     bool
	lastThrottle    time.Time
	recoveryDelay   time.Duration
}

// NewResourceManager 创建新的资源管理器
func NewResourceManager(logger *zap.Logger, monitor *ResourceMonitor, limits ResourceLimits) *ResourceManager {
	return &ResourceManager{
		logger:          logger,
		monitor:         monitor,
		limits:          limits,
		throttler:       NewThrottler(logger),
		checkPriorities: make(map[string]int),
		activeChecks:    make(map[string]*CheckContext),
		recoveryDelay:   30 * time.Second,
	}
}

// RegisterCheck 注册检查项及其优先级
func (rm *ResourceManager) RegisterCheck(name string, priority int) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.checkPriorities[name] = priority
	rm.logger.Info("Registered check",
		zap.String("name", name),
		zap.Int("priority", priority))
}

// UnregisterCheck 注销检查项
func (rm *ResourceManager) UnregisterCheck(name string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	delete(rm.checkPriorities, name)
	rm.logger.Info("Unregistered check", zap.String("name", name))
}

// CanExecuteCheck 检查是否可以执行指定检查项
func (rm *ResourceManager) CanExecuteCheck(name string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.unlockedCanExecuteCheck(name)
}

func (rm *ResourceManager) unlockedCanExecuteCheck(name string) bool {
	// 如果检查项不存在，返回false
	_, exists := rm.checkPriorities[name]
	if !exists {
		return false
	}

	// 如果系统被限流，检查优先级
	if rm.isThrottled {
		checkCtx, exists := rm.activeChecks[name]
		if !exists {
			return false
		}

		// 高优先级检查项可以继续执行
		if checkCtx.Priority >= 8 {
			return true
		}

		// 中等优先级检查项随机允许执行
		if checkCtx.Priority >= 5 {
			return time.Now().UnixNano()%2 == 0
		}

		// 低优先级检查项暂停
		return false
	}

	return true
}

// UpdateCheckStatus 更新检查项状态
func (rm *ResourceManager) UpdateCheckStatus(name string, status CheckStatus) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if checkCtx, exists := rm.activeChecks[name]; exists {
		checkCtx.Status = status
	}
}

// EvaluateResources 评估当前资源状态并调整限流策略
func (rm *ResourceManager) EvaluateResources() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	cpuExceeded := rm.monitor.IsCPULimitExceeded()
	memoryExceeded := rm.monitor.IsMemoryLimitExceeded()

	shouldThrottle := cpuExceeded || memoryExceeded
	wasThrottled := rm.isThrottled

	if shouldThrottle && !wasThrottled {
		// 开始限流
		rm.startThrottling()
	} else if !shouldThrottle && wasThrottled {
		// 检查是否可以恢复
		if time.Since(rm.lastThrottle) > rm.recoveryDelay {
			rm.stopThrottling()
		}
	}

	return nil
}

// startThrottling 开始限流
func (rm *ResourceManager) startThrottling() {
	rm.isThrottled = true
	rm.lastThrottle = time.Now()

	rm.logger.Warn("Resource limits exceeded, starting throttling",
		zap.Bool("cpu_exceeded", rm.monitor.IsCPULimitExceeded()),
		zap.Bool("memory_exceeded", rm.monitor.IsMemoryLimitExceeded()))

	// 应用限流策略
	strategy := rm.throttler.CreateStrategy(rm.monitor.GetCurrentMetrics(), rm.limits)
	rm.applyThrottlingStrategy(strategy)
}

// stopThrottling 停止限流
func (rm *ResourceManager) stopThrottling() {
	rm.isThrottled = false

	rm.logger.Info("Resource usage recovered, stopping throttling")

	// 恢复所有检查项
	for _, checkCtx := range rm.activeChecks {
		checkCtx.Status = CheckStatusActive
	}
}

// applyThrottlingStrategy 应用限流策略
func (rm *ResourceManager) applyThrottlingStrategy(strategy *ThrottlingStrategy) {
	for name, checkCtx := range rm.activeChecks {
		if checkCtx.Priority < strategy.MinPriority {
			checkCtx.Status = CheckStatusPaused
			rm.logger.Debug("Paused check due to throttling",
				zap.String("check", name),
				zap.Int("priority", checkCtx.Priority))
		} else if checkCtx.Priority < strategy.ReducedPriority {
			checkCtx.Status = CheckStatusThrottled
			rm.logger.Debug("Throttled check",
				zap.String("check", name),
				zap.Int("priority", checkCtx.Priority))
		}
	}
}

// GetThrottlingStatus 获取限流状态
func (rm *ResourceManager) GetThrottlingStatus() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	status := map[string]interface{}{
		"is_throttled":   rm.isThrottled,
		"last_throttle":  rm.lastThrottle,
		"recovery_delay": rm.recoveryDelay.Seconds(),
	}

	// 添加检查项状态
	checkStatus := make(map[string]string)
	for name, checkCtx := range rm.activeChecks {
		checkStatus[name] = string(checkCtx.Status)
	}
	status["checks"] = checkStatus

	return status
}

// GetActiveChecks 获取活跃检查项列表
func (rm *ResourceManager) GetActiveChecks() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var active []string
	for name, checkCtx := range rm.activeChecks {
		if checkCtx.Status == CheckStatusActive {
			active = append(active, name)
		}
	}
	return active
}

// GetPausedChecks 获取暂停检查项列表
func (rm *ResourceManager) GetPausedChecks() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var paused []string
	for name, checkCtx := range rm.activeChecks {
		if checkCtx.Status == CheckStatusPaused {
			paused = append(paused, name)
		}
	}
	return paused
}

// StartCheck 开始执行检查项并跟踪上下文
func (rm *ResourceManager) StartCheck(name string, timeout time.Duration) (context.Context, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	priority, exists := rm.checkPriorities[name]
	if !exists {
		return nil, fmt.Errorf("check %s not registered", name)
	}

	// 检查是否已存在相同名称的活动检查项
	if _, exists := rm.activeChecks[name]; exists {
		return nil, fmt.Errorf("check %s is already running", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	checkCtx := &CheckContext{
		Name:      name,
		Priority:  priority,
		StartTime: time.Now(),
		Status:    CheckStatusActive,
		CancelFn:  cancel,
		Timeout:   timeout,
	}

	rm.activeChecks[name] = checkCtx

	rm.logger.Info("Started check execution",
		zap.String("check", name),
		zap.Duration("timeout", timeout))

	return ctx, nil
}

// StopCheck 强制终止正在执行的检查项
func (rm *ResourceManager) StopCheck(name string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	checkCtx, exists := rm.activeChecks[name]
	if !exists {
		return fmt.Errorf("check %s not active", name)
	}

	if checkCtx.Status != CheckStatusActive {
		return fmt.Errorf("check %s not active (status: %s)", name, checkCtx.Status)
	}

	checkCtx.CancelFn()
	checkCtx.Status = CheckStatusPaused

	rm.logger.Warn("Forcefully stopped check",
		zap.String("check", name),
		zap.Duration("runtime", time.Since(checkCtx.StartTime)),
		zap.Duration("timeout", checkCtx.Timeout))

	return nil
}

// LogTimeout 记录检查项超时
func (rm *ResourceManager) LogTimeout(name string) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	checkCtx, exists := rm.activeChecks[name]
	if !exists {
		rm.logger.Error("Timeout for unknown check", zap.String("check", name))
		return
	}

	rm.logger.Error("Check execution timed out",
		zap.String("check", name),
		zap.Duration("timeout", checkCtx.Timeout),
		zap.Duration("runtime", time.Since(checkCtx.StartTime)))
}

// GetActiveCheckContexts 获取所有活动检查项的上下文
func (rm *ResourceManager) GetActiveCheckContexts() map[string]*CheckContext {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	active := make(map[string]*CheckContext)
	for name, ctx := range rm.activeChecks {
		if ctx.Status == CheckStatusActive {
			active[name] = ctx
		}
	}
	return active
}

// CleanupCompletedChecks 清理已完成或超时的检查项
func (rm *ResourceManager) CleanupCompletedChecks() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	now := time.Now()
	for name, ctx := range rm.activeChecks {
		if ctx.Status != CheckStatusActive || now.Sub(ctx.StartTime) > ctx.Timeout {
			delete(rm.activeChecks, name)
		}
	}
}

// Start 启动资源管理器
func (rm *ResourceManager) Start(ctx context.Context) error {
	rm.logger.Info("Starting resource manager")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			rm.logger.Info("Resource manager stopped by context")
			return ctx.Err()
		case <-ticker.C:
			if err := rm.EvaluateResources(); err != nil {
				rm.logger.Error("Failed to evaluate resources", zap.Error(err))
			}
		}
	}
}

// GetResourceSummary 获取资源使用摘要
func (rm *ResourceManager) GetResourceSummary() map[string]interface{} {
	metrics := rm.monitor.GetCurrentMetrics()
	status := rm.GetThrottlingStatus()

	return map[string]interface{}{
		"current_usage": map[string]interface{}{
			"cpu_percent":  metrics.CPUPercent,
			"memory_bytes": metrics.MemoryBytes,
			"memory_pct":   metrics.MemoryPercent,
		},
		"limits": map[string]interface{}{
			"cpu_limit": rm.limits.CPUPercent,
			"mem_limit": rm.limits.MemoryBytes,
		},
		"throttling": status,
	}
}
