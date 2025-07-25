package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/miaoyq/node-diagnostor/internal/aggregator"
	"github.com/miaoyq/node-diagnostor/internal/collector"
)

func main() {
	// 创建聚合器配置
	config := aggregator.Config{
		MaxDataPoints:   100,
		Compression:     true,
		CompressionType: "gzip",
		MaxSizeBytes:    1024 * 1024, // 1MB
		RetentionPeriod: 24 * time.Hour,
	}

	// 创建数据聚合器
	agg := aggregator.New(config, "test-node")
	ctx := context.Background()

	// 模拟从scheduler收集的结果
	results := []*collector.Result{
		{
			Data: &collector.Data{
				Type:      "cpu",
				Timestamp: time.Now(),
				Source:    "cpu-collector",
				Data: map[string]interface{}{
					"temperature": 45.5,
					"frequency":   2400.0,
					"usage":       23.4,
				},
				Metadata: map[string]string{
					"unit": "celsius",
				},
			},
			Error:   nil,
			Skipped: false,
		},
		{
			Data: &collector.Data{
				Type:      "memory",
				Timestamp: time.Now().Add(-30 * time.Second),
				Source:    "memory-collector",
				Data: map[string]interface{}{
					"total_mb":      8192.0,
					"available_mb":  2048.0,
					"usage_percent": 75.0,
				},
				Metadata: map[string]string{
					"unit": "megabytes",
				},
			},
			Error:   nil,
			Skipped: false,
		},
	}

	// 聚合数据 - 这是现在主程序中会调用的方式
	aggregated, err := agg.Aggregate(ctx, "system-health-check", results)
	if err != nil {
		log.Fatalf("Failed to aggregate data: %v", err)
	}

	fmt.Printf("✅ DataAggregator 已成功集成！\n")
	fmt.Printf("聚合数据详情:\n")
	fmt.Printf("- 检查名称: %s\n", aggregated.CheckName)
	fmt.Printf("- 节点名称: %s\n", aggregated.NodeName)
	fmt.Printf("- 时间戳: %s\n", aggregated.Timestamp.Format(time.RFC3339))
	fmt.Printf("- 数据点数: %d\n", len(aggregated.DataPoints))
	fmt.Printf("- 是否压缩: %t\n", aggregated.Compressed)
	fmt.Printf("- 数据大小: %d bytes\n", aggregated.Size)

	// 获取统计信息
	stats := agg.GetStats()
	fmt.Printf("\n聚合器统计:\n")
	fmt.Printf("- 总检查数: %.0f\n", stats["total_checks"])
	fmt.Printf("- 总数据点数: %.0f\n", stats["total_data_points"])
	fmt.Printf("- 总大小: %.0f bytes\n", stats["total_size_bytes"])

	// 演示清理功能
	removed := agg.Clean()
	fmt.Printf("- 清理过期数据: %d 条\n", removed)
}
