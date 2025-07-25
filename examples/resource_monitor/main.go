package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/miaoyq/node-diagnostor/internal/resource"
	"go.uber.org/zap"
)

func main() {
	// 初始化日志
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// 设置资源限制
	limits := resource.ResourceLimits{
		CPUPercent:    5.0,               // 5% CPU使用率限制
		MemoryBytes:   100 * 1024 * 1024, // 100MB内存限制
		MaxGoroutines: 100,
	}

	// 创建资源监控器
	monitor, err := resource.NewResourceMonitor(logger, limits)
	if err != nil {
		logger.Fatal("Failed to create resource monitor", zap.Error(err))
	}

	// 创建资源管理器
	manager := resource.NewResourceManager(logger, monitor, limits)

	// 注册一些示例检查项
	manager.RegisterCheck("cpu-temperature", 9)
	manager.RegisterCheck("memory-usage", 8)
	manager.RegisterCheck("disk-io", 6)
	manager.RegisterCheck("network-stats", 4)
	manager.RegisterCheck("system-load", 2)

	// 创建上下文用于优雅关闭
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动资源监控器
	go func() {
		if err := monitor.Start(ctx); err != nil {
			logger.Error("Resource monitor stopped", zap.Error(err))
		}
	}()

	// 启动资源管理器
	go func() {
		if err := manager.Start(ctx); err != nil {
			logger.Error("Resource manager stopped", zap.Error(err))
		}
	}()

	// 定期打印资源状态
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				printResourceStatus(monitor, manager)
			}
		}
	}()

	// 模拟一些工作负载
	go simulateWorkload()

	logger.Info("Resource monitoring system started")
	logger.Info("Press Ctrl+C to stop")

	// 等待退出信号
	<-sigChan
	logger.Info("Shutting down...")
	cancel()

	// 等待优雅关闭
	time.Sleep(1 * time.Second)
}

func printResourceStatus(monitor *resource.ResourceMonitor, manager *resource.ResourceManager) {
	// 获取当前资源指标
	metrics := monitor.GetCurrentMetrics()
	if metrics == nil {
		return
	}

	// 获取进程信息
	procInfo, err := monitor.GetProcessInfo()
	if err != nil {
		return
	}

	// 获取系统信息
	totalMem, availMem, cpuCount, err := monitor.GetSystemInfo()
	if err != nil {
		return
	}

	// 获取资源状态
	status := monitor.GetResourceStatus()
	_ = manager.GetResourceSummary()

	fmt.Printf("\n=== Resource Status ===\n")
	fmt.Printf("Process: PID=%d, Name=%s, Uptime=%s\n",
		procInfo.PID, procInfo.Name, time.Since(procInfo.StartTime).Round(time.Second))
	fmt.Printf("System: CPUs=%d, TotalMemory=%.1fMB, AvailableMemory=%.1fMB\n",
		cpuCount, float64(totalMem)/1024/1024, float64(availMem)/1024/1024)
	fmt.Printf("Current Usage: CPU=%.2f%%, Memory=%.1fMB (%.2f%%)\n",
		metrics.CPUPercent, float64(metrics.MemoryBytes)/1024/1024, metrics.MemoryPercent)
	fmt.Printf("Limits: CPU=%.1f%%, Memory=%.1fMB\n",
		status["cpu_limit"].(float64), float64(status["memory_limit"].(uint64))/1024/1024)
	fmt.Printf("Throttling: %v\n", status["cpu_exceeded"].(bool) || status["memory_exceeded"].(bool))

	// 打印检查项状态
	activeChecks := manager.GetActiveChecks()
	pausedChecks := manager.GetPausedChecks()
	fmt.Printf("Active Checks: %v\n", activeChecks)
	fmt.Printf("Paused Checks: %v\n", pausedChecks)
}

func simulateWorkload() {
	// 模拟一些CPU和内存使用
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 创建一些内存分配
			data := make([]byte, 1024*1024) // 1MB
			for i := range data {
				data[i] = byte(i % 256)
			}

			// 做一些CPU工作
			sum := 0
			for i := 0; i < 1000000; i++ {
				sum += i
			}

			_ = sum // 避免优化
			_ = data
		}
	}
}
