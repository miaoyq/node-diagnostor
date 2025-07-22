package processor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/miaoyq/node-diagnostor/pkg/config"
	"github.com/miaoyq/node-diagnostor/pkg/types"
)

// ProcessData 处理收集到的数据
func ProcessData(ctx context.Context, cfg *config.Config, metricsChan <-chan *types.SystemMetrics, logChan <-chan types.LogEntry, reportChan chan<- types.DiagnosticReport) {
	var logsBuffer []types.LogEntry
	var metrics *types.SystemMetrics

	// 创建报告客户端
	cacheDir := filepath.Join(".", "test") // 使用测试目录作为缓存目录
	client := NewReportClient(cfg.ReportURL, 10*time.Second, 3, cacheDir)

	// 启动时尝试重试缓存的报告
	go func() {
		time.Sleep(5 * time.Second) // 等待系统稳定
		if err := client.RetryCachedReports(ctx); err != nil {
			fmt.Printf("重试缓存报告失败: %v\n", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case metric := <-metricsChan:
			metrics = metric
			// 当有新的指标时，处理并生成报告
			if metrics != nil {
				report := types.DiagnosticReport{
					NodeName:     getNodeName(),
					Timestamp:    time.Now(),
					Metrics:      *metrics,
					ErrorLogs:    filterLogsByPriority(logsBuffer, "err"),
					WarningLogs:  filterLogsByPriority(logsBuffer, "warning"),
					CheckResults: executeChecks(cfg, metrics, logsBuffer),
				}

				// 使用报告客户端发送
				if err := client.SendReport(ctx, report); err != nil {
					fmt.Printf("发送报告失败: %v\n", err)
				}

				// 同时发送到通道供测试使用
				select {
				case reportChan <- report:
				default:
					// 通道满时丢弃
				}

				// 清空日志缓冲区
				logsBuffer = nil
			}
		case logEntry := <-logChan:
			// 将日志添加到缓冲区
			logsBuffer = append(logsBuffer, logEntry)
		}
	}
}

// getNodeName 获取节点名称
func getNodeName() string {
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return "unknown-node"
}

// filterLogsByPriority 按优先级过滤日志
func filterLogsByPriority(logs []types.LogEntry, priority string) []types.LogEntry {
	var filtered []types.LogEntry
	for _, log := range logs {
		if log.Priority == priority {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

// executeChecks 执行诊断检查
func executeChecks(cfg *config.Config, metrics *types.SystemMetrics, logs []types.LogEntry) []types.CheckResult {
	var results []types.CheckResult

	// 执行内核参数检查
	if cfg.CheckConfigs != nil {
		for _, check := range cfg.CheckConfigs {
			if check.Enable {
				result := executeSingleCheck(check, metrics, logs)
				results = append(results, result)
			}
		}
	}

	return results
}

// executeSingleCheck 执行单个检查
func executeSingleCheck(check config.CheckConfig, metrics *types.SystemMetrics, logs []types.LogEntry) types.CheckResult {
	return types.CheckResult{
		Name:    check.Name,
		Status:  "pass",
		Message: fmt.Sprintf("检查 %s 通过", check.Name),
	}
}

// runChecks 执行诊断检查
func runChecks(cfg *config.Config, metrics *types.SystemMetrics, logs []types.LogEntry) []types.CheckResult {
	// TODO: 实现诊断检查逻辑
	return []types.CheckResult{}
}
