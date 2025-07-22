package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/miaoyq/node-diagnostor/pkg/types"
)

func StartReporter(ctx context.Context, reportURL string, reportChan <-chan types.DiagnosticReport, dryRun bool) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for {
		select {
		case <-ctx.Done():
			return

		case report := <-reportChan:
			if dryRun {
				fmt.Printf("[dry-run] 诊断报告生成: 时间=%s, 错误日志数=%d\n",
					report.Timestamp.Format(time.RFC3339), len(report.ErrorLogs))
				continue
			}

			// 转换为JSON
			jsonData, err := json.Marshal(report)
			if err != nil {
				fmt.Printf("JSON编码失败: %v\n", err)
				continue
			}

			// 发送HTTP POST请求
			resp, err := client.Post(reportURL, "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				fmt.Printf("上报失败: %v\n", err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				fmt.Printf("上报成功: %s\n", resp.Status)
			} else {
				fmt.Printf("上报失败: %s\n", resp.Status)
			}
		}
	}
}
