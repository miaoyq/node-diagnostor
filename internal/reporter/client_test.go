package reporter

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/miaoyq/node-diagnostor/internal/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewReporterClient(t *testing.T) {
	cfg := config.ReporterConfig{
		Endpoint:    "http://localhost:8080/api/v1/diagnostics",
		Timeout:     10 * time.Second,
		MaxRetries:  3,
		RetryDelay:  1 * time.Second,
		Compression: false,
	}

	logger, _ := zap.NewDevelopment()
	client := NewReporterClient(cfg, logger)

	assert.NotNil(t, client)
	assert.Equal(t, cfg, client.config)
	assert.NotNil(t, client.httpClient)
	assert.NotNil(t, client.cache)
	assert.NotNil(t, client.metrics)
}

func TestReportSuccess(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var report Report
		err := json.NewDecoder(r.Body).Decode(&report)
		assert.NoError(t, err)
		assert.Equal(t, "test-check", report.CheckName)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.ReporterConfig{
		Endpoint:    server.URL,
		Timeout:     5 * time.Second,
		MaxRetries:  1,
		RetryDelay:  100 * time.Millisecond,
		Compression: false,
	}

	logger, _ := zap.NewDevelopment()
	client := NewReporterClient(cfg, logger)

	ctx := context.Background()
	report := &Report{
		CheckName: "test-check",
		Data:      map[string]interface{}{"key": "value"},
		Timestamp: time.Now(),
		NodeName:  "test-node",
	}

	result, err := client.Report(ctx, report)

	assert.NoError(t, err)
	assert.Equal(t, StatusSuccess, result.Status)
	assert.Equal(t, 1, result.Attempts)
	assert.Greater(t, result.Duration, time.Duration(0))

	// Check metrics
	metrics := client.GetMetrics()
	assert.Equal(t, int64(1), metrics.totalReports)
	assert.Equal(t, int64(1), metrics.successReports)
	assert.Equal(t, int64(0), metrics.failedReports)
}

func TestReportWithRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.ReporterConfig{
		Endpoint:    server.URL,
		Timeout:     5 * time.Second,
		MaxRetries:  3,
		RetryDelay:  100 * time.Millisecond,
		Compression: false,
	}

	logger, _ := zap.NewDevelopment()
	client := NewReporterClient(cfg, logger)

	ctx := context.Background()
	report := &Report{
		CheckName: "test-check",
		Data:      map[string]interface{}{"key": "value"},
		Timestamp: time.Now(),
		NodeName:  "test-node",
	}

	result, err := client.Report(ctx, report)

	assert.NoError(t, err)
	assert.Equal(t, StatusSuccess, result.Status)
	assert.Equal(t, 3, result.Attempts)
	assert.Equal(t, 3, attempts)

	// Check metrics
	metrics := client.GetMetrics()
	assert.Equal(t, int64(1), metrics.totalReports)
	assert.Equal(t, int64(1), metrics.successReports)
	assert.Equal(t, int64(0), metrics.failedReports)
	assert.Equal(t, int64(2), metrics.retryReports)
}

func TestReportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	cfg := config.ReporterConfig{
		Endpoint:    server.URL,
		Timeout:     5 * time.Second,
		MaxRetries:  2,
		RetryDelay:  100 * time.Millisecond,
		Compression: false,
	}

	logger, _ := zap.NewDevelopment()
	client := NewReporterClient(cfg, logger)

	ctx := context.Background()
	report := &Report{
		CheckName: "test-check",
		Data:      map[string]interface{}{"key": "value"},
		Timestamp: time.Now(),
		NodeName:  "test-node",
	}

	result, err := client.Report(ctx, report)

	assert.Error(t, err)
	assert.Equal(t, StatusFailed, result.Status)
	assert.Equal(t, 3, result.Attempts) // 1 initial + 2 retries
	assert.Contains(t, result.Error.Error(), "max retries exceeded")

	// Check metrics
	metrics := client.GetMetrics()
	assert.Equal(t, int64(1), metrics.totalReports)
	assert.Equal(t, int64(0), metrics.successReports)
	assert.Equal(t, int64(1), metrics.failedReports)
	assert.Equal(t, int64(2), metrics.retryReports)
}

func TestReportWithCompression(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "gzip", r.Header.Get("Content-Encoding"))

		// Decompress the request body
		gz, err := gzip.NewReader(r.Body)
		assert.NoError(t, err)
		defer gz.Close()

		var report Report
		err = json.NewDecoder(gz).Decode(&report)
		assert.NoError(t, err)
		assert.Equal(t, "test-check", report.CheckName)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.ReporterConfig{
		Endpoint:    server.URL,
		Timeout:     5 * time.Second,
		MaxRetries:  1,
		RetryDelay:  100 * time.Millisecond,
		Compression: true,
	}

	logger, _ := zap.NewDevelopment()
	client := NewReporterClient(cfg, logger)

	ctx := context.Background()
	report := &Report{
		CheckName: "test-check",
		Data:      map[string]interface{}{"key": "value"},
		Timestamp: time.Now(),
		NodeName:  "test-node",
	}

	result, err := client.Report(ctx, report)

	assert.NoError(t, err)
	assert.Equal(t, StatusSuccess, result.Status)
}

func TestCacheManagement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.ReporterConfig{
		Endpoint:    server.URL,
		Timeout:     5 * time.Second,
		CacheMaxAge: 10 * time.Second,
		MaxRetries:  1,
		RetryDelay:  100 * time.Millisecond,
		Compression: false,
	}

	logger, _ := zap.NewDevelopment()
	client := NewReporterClient(cfg, logger)

	ctx := context.Background()
	report := &Report{
		CheckName: "test-check",
		Data:      map[string]interface{}{"key": "value"},
		Timestamp: time.Now(),
		NodeName:  "test-node",
	}

	// Send report
	result, err := client.Report(ctx, report)
	assert.NoError(t, err)
	assert.Equal(t, StatusSuccess, result.Status)

	// Check cache stats
	stats := client.GetCacheStats()
	assert.Equal(t, 0, stats["cache_size"].(int)) // Successful reports are removed from cache

	// Test failed report caching
	server.Close()
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client.config.Endpoint = server.URL

	report2 := &Report{
		CheckName: "test-check-2",
		Data:      map[string]interface{}{"key": "value"},
		Timestamp: time.Now(),
		NodeName:  "test-node",
	}

	result2, err := client.Report(ctx, report2)
	assert.Error(t, err)
	assert.Equal(t, StatusFailed, result2.Status)

	// Check cache stats for failed report
	stats = client.GetCacheStats()
	assert.Equal(t, 1, stats["cache_size"].(int))

	// Clear cache
	client.ClearCache()
	stats = client.GetCacheStats()
	assert.Equal(t, 0, stats["cache_size"].(int))
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.ReporterConfig{
		Endpoint:    server.URL,
		Timeout:     5 * time.Second,
		MaxRetries:  1,
		RetryDelay:  100 * time.Millisecond,
		Compression: false,
	}

	logger, _ := zap.NewDevelopment()
	client := NewReporterClient(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())

	report := &Report{
		CheckName: "test-check",
		Data:      map[string]interface{}{"key": "value"},
		Timestamp: time.Now(),
		NodeName:  "test-node",
	}

	// Cancel context immediately
	cancel()

	_, err := client.Report(ctx, report)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestConcurrentReports(t *testing.T) {
	var mu sync.Mutex
	received := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.ReporterConfig{
		Endpoint:    server.URL,
		Timeout:     5 * time.Second,
		MaxRetries:  1,
		RetryDelay:  100 * time.Millisecond,
		Compression: false,
	}

	logger, _ := zap.NewDevelopment()
	client := NewReporterClient(cfg, logger)

	ctx := context.Background()
	var wg sync.WaitGroup

	// Send 10 concurrent reports
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			report := &Report{
				CheckName: fmt.Sprintf("test-check-%d", index),
				Data:      map[string]interface{}{"index": index},
				Timestamp: time.Now(),
				NodeName:  "test-node",
			}
			_, err := client.Report(ctx, report)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	assert.Equal(t, 10, received)

	// Check metrics
	metrics := client.GetMetrics()
	assert.Equal(t, int64(10), metrics.totalReports)
	assert.Equal(t, int64(10), metrics.successReports)
}

func TestStartStop(t *testing.T) {
	cfg := config.ReporterConfig{
		Endpoint:    "http://localhost:8080/api/v1/diagnostics",
		Timeout:     5 * time.Second,
		MaxRetries:  1,
		RetryDelay:  100 * time.Millisecond,
		Compression: false,
	}

	logger, _ := zap.NewDevelopment()
	client := NewReporterClient(cfg, logger)

	ctx := context.Background()

	// Start client
	err := client.Start(ctx)
	assert.NoError(t, err)

	// Stop client
	client.Stop()

	// Should not panic
	assert.True(t, true)
}
