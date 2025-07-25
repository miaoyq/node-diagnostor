package resource

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
	"go.uber.org/zap"
)

// ResourceMonitor 负责监控和管理系统资源使用
type ResourceMonitor struct {
	mu             sync.RWMutex
	logger         *zap.Logger
	cpuLimit       float64
	memoryLimit    uint64
	currentCPU     float64
	currentMemory  uint64
	process        *process.Process
	startTime      time.Time
	lastCheckTime  time.Time
	metrics        *ResourceMetrics
	stopChan       chan struct{}
	updateInterval time.Duration
}

// ResourceMetrics 存储资源使用指标
type ResourceMetrics struct {
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryBytes   uint64    `json:"memory_bytes"`
	MemoryPercent float64   `json:"memory_percent"`
	Timestamp     time.Time `json:"timestamp"`
}

// ResourceLimits 定义资源限制配置
type ResourceLimits struct {
	CPUPercent    float64 `json:"cpu_percent"`    // CPU使用率限制百分比
	MemoryBytes   uint64  `json:"memory_bytes"`   // 内存使用限制字节数
	MaxGoroutines int     `json:"max_goroutines"` // 最大goroutine数量
}

// NewResourceMonitor 创建新的资源监控器
func NewResourceMonitor(logger *zap.Logger, limits ResourceLimits) (*ResourceMonitor, error) {
	pid := int32(os.Getpid())
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("failed to get current process: %w", err)
	}

	monitor := &ResourceMonitor{
		logger:         logger,
		cpuLimit:       limits.CPUPercent,
		memoryLimit:    limits.MemoryBytes,
		process:        proc,
		startTime:      time.Now(),
		metrics:        &ResourceMetrics{},
		stopChan:       make(chan struct{}),
		updateInterval: 1 * time.Second,
	}

	// 初始化资源监控
	if err := monitor.updateMetrics(); err != nil {
		logger.Warn("Failed to initialize resource metrics", zap.Error(err))
	}

	return monitor, nil
}

// Start 启动资源监控
func (rm *ResourceMonitor) Start(ctx context.Context) error {
	rm.logger.Info("Starting resource monitor",
		zap.Float64("cpu_limit", rm.cpuLimit),
		zap.Uint64("memory_limit", rm.memoryLimit))

	ticker := time.NewTicker(rm.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			rm.logger.Info("Resource monitor stopped by context")
			return ctx.Err()
		case <-rm.stopChan:
			rm.logger.Info("Resource monitor stopped")
			return nil
		case <-ticker.C:
			if err := rm.updateMetrics(); err != nil {
				rm.logger.Error("Failed to update resource metrics", zap.Error(err))
			}
		}
	}
}

// Stop 停止资源监控
func (rm *ResourceMonitor) Stop() {
	close(rm.stopChan)
}

// updateMetrics 更新当前资源使用指标
func (rm *ResourceMonitor) updateMetrics() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// 获取CPU使用率
	cpuPercent, err := rm.process.CPUPercent()
	if err != nil {
		return fmt.Errorf("failed to get CPU percent: %w", err)
	}

	// 获取内存使用
	memInfo, err := rm.process.MemoryInfo()
	if err != nil {
		return fmt.Errorf("failed to get memory info: %w", err)
	}

	// 获取系统总内存用于计算百分比
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return fmt.Errorf("failed to get virtual memory: %w", err)
	}

	rm.currentCPU = cpuPercent
	rm.currentMemory = memInfo.RSS
	rm.lastCheckTime = time.Now()

	rm.metrics = &ResourceMetrics{
		CPUPercent:    cpuPercent,
		MemoryBytes:   memInfo.RSS,
		MemoryPercent: float64(memInfo.RSS) / float64(vmStat.Total) * 100,
		Timestamp:     rm.lastCheckTime,
	}

	return nil
}

// GetCurrentMetrics 获取当前资源使用指标
func (rm *ResourceMonitor) GetCurrentMetrics() *ResourceMetrics {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.metrics
}

// IsCPULimitExceeded 检查是否超过CPU限制
func (rm *ResourceMonitor) IsCPULimitExceeded() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.currentCPU > rm.cpuLimit
}

// IsMemoryLimitExceeded 检查是否超过内存限制
func (rm *ResourceMonitor) IsMemoryLimitExceeded() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.currentMemory > rm.memoryLimit
}

// GetResourceUsage 获取详细的资源使用信息
func (rm *ResourceMonitor) GetResourceUsage() (cpuPercent float64, memoryBytes uint64, goroutines int) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.currentCPU, rm.currentMemory, runtime.NumGoroutine()
}

// GetSystemInfo 获取系统信息
func (rm *ResourceMonitor) GetSystemInfo() (totalMemory uint64, availableMemory uint64, cpuCount int, err error) {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get virtual memory: %w", err)
	}

	cpuCount, err = cpu.Counts(true)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get CPU counts: %w", err)
	}

	return vmStat.Total, vmStat.Available, cpuCount, nil
}

// GetProcessInfo 获取进程详细信息
func (rm *ResourceMonitor) GetProcessInfo() (*ProcessInfo, error) {
	name, err := rm.process.Name()
	if err != nil {
		return nil, fmt.Errorf("failed to get process name: %w", err)
	}

	cmdline, err := rm.process.Cmdline()
	if err != nil {
		return nil, fmt.Errorf("failed to get process cmdline: %w", err)
	}

	createTime, err := rm.process.CreateTime()
	if err != nil {
		return nil, fmt.Errorf("failed to get process create time: %w", err)
	}

	return &ProcessInfo{
		PID:        rm.process.Pid,
		Name:       name,
		Cmdline:    cmdline,
		CreateTime: time.Unix(createTime/1000, 0),
		StartTime:  rm.startTime,
	}, nil
}

// ProcessInfo 包含进程详细信息
type ProcessInfo struct {
	PID        int32     `json:"pid"`
	Name       string    `json:"name"`
	Cmdline    string    `json:"cmdline"`
	CreateTime time.Time `json:"create_time"`
	StartTime  time.Time `json:"start_time"`
}

// GetResourceStatus 获取资源状态摘要
func (rm *ResourceMonitor) GetResourceStatus() map[string]interface{} {
	cpu, memory, goroutines := rm.GetResourceUsage()
	totalMem, availMem, cpuCount, _ := rm.GetSystemInfo()

	return map[string]interface{}{
		"cpu_percent":      cpu,
		"cpu_limit":        rm.cpuLimit,
		"cpu_exceeded":     rm.IsCPULimitExceeded(),
		"memory_bytes":     memory,
		"memory_limit":     rm.memoryLimit,
		"memory_exceeded":  rm.IsMemoryLimitExceeded(),
		"goroutines":       goroutines,
		"system_total_mem": totalMem,
		"system_avail_mem": availMem,
		"system_cpu_count": cpuCount,
		"uptime_seconds":   time.Since(rm.startTime).Seconds(),
	}
}
