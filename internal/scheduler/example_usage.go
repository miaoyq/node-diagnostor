package scheduler

import (
	"context"
	"fmt"
	"time"
)

// ExampleUsage 展示了如何使用TaskMonitor
func ExampleUsage() {
	// 创建任务监控器
	monitor := NewTaskMonitor()

	// 创建模拟的执行器 - 使用现有的mock_scheduler.go中的MockExecutor
	executor := &MockExecutor{}

	// 创建调度器
	scheduler := New(executor, monitor, nil)

	// 启动调度器
	ctx := context.Background()
	_ = scheduler.Start(ctx)

	// 添加任务
	task := &Task{
		ID:        "cpu-check",
		Name:      "CPU Usage Check",
		Collector: "cpu",
		Interval:  5 * time.Second,
		Timeout:   30 * time.Second,
		Enabled:   true,
	}

	_ = scheduler.Add(ctx, task)

	// 模拟一些任务执行
	go func() {
		for i := 0; i < 5; i++ {
			result := &Result{
				TaskID:    "cpu-check",
				Success:   i%2 == 0,
				Duration:  time.Duration(100+i*50) * time.Millisecond,
				Timestamp: time.Now(),
				Data:      map[string]interface{}{"cpu_percent": 45.0 + float64(i)},
			}

			_ = monitor.Record(ctx, result)
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// 等待一段时间后查看指标
	time.Sleep(500 * time.Millisecond)

	// 获取全局指标
	globalMetrics := monitor.GetMetrics()
	fmt.Printf("Global Metrics: %+v\n", globalMetrics)

	// 获取特定任务指标
	taskMetrics := monitor.GetTaskMetrics("cpu-check")
	fmt.Printf("Task Metrics: %+v\n", taskMetrics)

	// 获取详细任务指标
	detailedMetrics, _ := monitor.GetTaskExecutionMetrics("cpu-check")
	if detailedMetrics != nil {
		fmt.Printf("Detailed Metrics: %+v\n", detailedMetrics)
	}

	// 停止调度器
	_ = scheduler.Stop(ctx)
}
