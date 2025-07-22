package processor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/miaoyq/node-diagnostor/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestReportClient_SendReport(t *testing.T) {
	// 创建测试服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// 创建临时缓存目录
	cacheDir := filepath.Join(t.TempDir(), "cache")
	client := NewReportClient(ts.URL, 5*time.Second, 3, cacheDir)

	report := types.DiagnosticReport{
		NodeName:  "test-node",
		Timestamp: time.Now(),
		Metrics: types.SystemMetrics{
			CPUUsage:    10.5,
			MemoryTotal: 16384,
			MemoryUsed:  8192,
		},
	}

	err := client.SendReport(context.Background(), report)
	assert.NoError(t, err)
}

func TestReportClient_CacheAndRetry(t *testing.T) {
	// 创建总是失败的服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	// 创建临时缓存目录
	cacheDir := filepath.Join(t.TempDir(), "cache")
	client := NewReportClient(ts.URL, 5*time.Second, 1, cacheDir)

	report := types.DiagnosticReport{
		NodeName:  "test-node",
		Timestamp: time.Now(),
		Metrics: types.SystemMetrics{
			CPUUsage:    10.5,
			MemoryTotal: 16384,
			MemoryUsed:  8192,
		},
	}

	// 发送报告（会失败并缓存）
	err := client.SendReport(context.Background(), report)
	assert.NoError(t, err)
}
