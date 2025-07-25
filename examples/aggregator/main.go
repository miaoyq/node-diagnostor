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
	agg := aggregator.New(config, "example-node")
	ctx := context.Background()

	// 模拟采集结果
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
		{
			Data: &collector.Data{
				Type:      "disk",
				Timestamp: time.Now().Add(-60 * time.Second),
				Source:    "disk-collector",
				Data: map[string]interface{}{
					"total_gb":      100.0,
					"available_gb":  45.3,
					"usage_percent": 54.7,
				},
				Metadata: map[string]string{
					"unit": "gigabytes",
				},
			},
			Error:   nil,
			Skipped: false,
		},
	}

	// 聚合数据
	aggregated, err := agg.Aggregate(ctx, "system-health-check", results)
	if err != nil {
		log.Fatalf("Failed to aggregate data: %v", err)
	}

	// 打印聚合结果
	fmt.Printf("Aggregated Data for: %s\n", aggregated.CheckName)
	fmt.Printf("Node: %s\n", aggregated.NodeName)
	fmt.Printf("Timestamp: %s\n", aggregated.Timestamp.Format(time.RFC3339))
	fmt.Printf("Total Data Points: %d\n", len(aggregated.DataPoints))
	fmt.Printf("Compressed: %t\n", aggregated.Compressed)
	fmt.Printf("Size: %d bytes\n", aggregated.Size)

	// 打印每个数据点
	for i, dp := range aggregated.DataPoints {
		fmt.Printf("\nData Point %d:\n", i+1)
		fmt.Printf("  Collector: %s\n", dp.Collector)
		fmt.Printf("  Source: %s\n", dp.Source)
		fmt.Printf("  Timestamp: %s\n", dp.Timestamp.Format(time.RFC3339))
		fmt.Printf("  Data: %v\n", dp.Data)
	}

	// 获取统计信息
	stats := agg.GetStats()
	fmt.Printf("\nAggregator Statistics:\n")
	fmt.Printf("  Total Checks: %.0f\n", stats["total_checks"])
	fmt.Printf("  Total Data Points: %.0f\n", stats["total_data_points"])
	fmt.Printf("  Total Size: %.0f bytes\n", stats["total_size_bytes"])
	fmt.Printf("  Uptime: %.2f seconds\n", stats["uptime_seconds"])

	// 格式化输出
	formatted, err := agg.FormatData(aggregated)
	if err != nil {
		log.Printf("Failed to format data: %v", err)
	} else {
		fmt.Printf("\nFormatted JSON:\n%s\n", string(formatted))
	}
}
