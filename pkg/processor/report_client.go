package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/miaoyq/node-diagnostor/pkg/types"
)

// ReportClient 上报客户端
type ReportClient struct {
	client   *retryablehttp.Client
	url      string
	cacheDir string
}

// NewReportClient 创建新的上报客户端
func NewReportClient(url string, timeout time.Duration, retryMax int, cacheDir string) *ReportClient {
	client := retryablehttp.NewClient()
	client.RetryMax = retryMax
	client.RetryWaitMin = 1 * time.Second
	client.RetryWaitMax = 30 * time.Second
	client.HTTPClient.Timeout = timeout

	// 确保缓存目录存在
	if cacheDir != "" {
		os.MkdirAll(cacheDir, 0755)
	}

	return &ReportClient{
		client:   client,
		url:      url,
		cacheDir: cacheDir,
	}
}

// SendReport 异步上报报告，支持本地缓存
func (c *ReportClient) SendReport(ctx context.Context, report types.DiagnosticReport) error {
	jsonData, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("JSON编码失败: %v", err)
	}

	// 先尝试直接上报
	req, err := retryablehttp.NewRequestWithContext(ctx, "POST", c.url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 异步发送
	go func() {
		resp, err := c.client.Do(req)
		if err != nil {
			fmt.Printf("上报失败: %v，保存到本地缓存\n", err)
			c.saveToCache(jsonData)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			fmt.Printf("上报成功: %s\n", resp.Status)
			// 上报成功后清理缓存
			c.clearCache()
		} else {
			fmt.Printf("上报失败: %s，保存到本地缓存\n", resp.Status)
			c.saveToCache(jsonData)
		}
	}()

	return nil
}

// saveToCache 保存数据到本地缓存
func (c *ReportClient) saveToCache(data []byte) error {
	if c.cacheDir == "" {
		return nil
	}

	cacheFile := filepath.Join(c.cacheDir, "report_cache.json")
	err := os.WriteFile(cacheFile, data, 0644)
	if err != nil {
		return fmt.Errorf("保存缓存失败: %v", err)
	}
	fmt.Printf("已保存到本地缓存: %s\n", cacheFile)
	return nil
}

// clearCache 清理本地缓存
func (c *ReportClient) clearCache() error {
	if c.cacheDir == "" {
		return nil
	}

	cacheFile := filepath.Join(c.cacheDir, "report_cache.json")
	if _, err := os.Stat(cacheFile); err == nil {
		err := os.Remove(cacheFile)
		if err != nil {
			return fmt.Errorf("清理缓存失败: %v", err)
		}
		fmt.Println("本地缓存已清理")
	}
	return nil
}

// RetryCachedReports 重试发送缓存的报告
func (c *ReportClient) RetryCachedReports(ctx context.Context) error {
	if c.cacheDir == "" {
		return nil
	}

	cacheFile := filepath.Join(c.cacheDir, "report_cache.json")
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		return nil // 没有缓存文件
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return fmt.Errorf("读取缓存失败: %v", err)
	}

	// 解析缓存的报告
	var report types.DiagnosticReport
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("解析缓存失败: %v", err)
	}

	// 重新发送
	return c.SendReport(ctx, report)
}
