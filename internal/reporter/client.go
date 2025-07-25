package reporter

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/miaoyq/node-diagnostor/internal/config"
	"go.uber.org/zap"
)

// ReportStatus represents the status of a report attempt
type ReportStatus string

const (
	StatusPending   ReportStatus = "pending"
	StatusSuccess   ReportStatus = "success"
	StatusFailed    ReportStatus = "failed"
	StatusRetrying  ReportStatus = "retrying"
	StatusDiscarded ReportStatus = "discarded"
)

// Report represents a diagnostic report to be sent
type Report struct {
	ID        string                 `json:"id"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
	NodeName  string                 `json:"node_name"`
	CheckName string                 `json:"check_name"`
}

// ReportResult contains the result of a report attempt
type ReportResult struct {
	ReportID string
	Status   ReportStatus
	Error    error
	Attempts int
	Duration time.Duration
}

// CacheEntry represents a cached report
type CacheEntry struct {
	Report    *Report
	Status    ReportStatus
	Attempts  int
	LastTry   time.Time
	CreatedAt time.Time
}

// ReporterClient handles sending diagnostic reports to the server
type ReporterClient struct {
	config     config.ReporterConfig
	httpClient *http.Client
	cache      *CacheManager
	logger     *zap.Logger
	metrics    *ReporterMetrics
	stopChan   chan struct{}
	wg         sync.WaitGroup
	mu         sync.RWMutex
}

// ReporterMetrics tracks reporting statistics
type ReporterMetrics struct {
	mu               sync.RWMutex
	totalReports     int64
	successReports   int64
	failedReports    int64
	retryReports     int64
	discardedReports int64
	lastReportTime   time.Time
}

// NewReporterClient creates a new ReporterClient instance
func NewReporterClient(cfg config.ReporterConfig, logger *zap.Logger) *ReporterClient {
	cacheManager := NewCacheManager(
		"/tmp/node-diagnostor-cache",
		cfg.CacheMaxSize,
		cfg.CacheMaxAge,
		logger,
	)

	return &ReporterClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		cache:    cacheManager,
		logger:   logger,
		metrics:  &ReporterMetrics{},
		stopChan: make(chan struct{}),
	}
}

// Start starts the reporter client and background tasks
func (rc *ReporterClient) Start(ctx context.Context) error {
	rc.logger.Info("Starting reporter client", zap.String("endpoint", rc.config.Endpoint))

	// Start cache manager
	if err := rc.cache.Start(ctx); err != nil {
		return fmt.Errorf("failed to start cache manager: %w", err)
	}

	return nil
}

// Stop stops the reporter client and waits for background tasks to complete
func (rc *ReporterClient) Stop() {
	rc.cache.Stop()
	close(rc.stopChan)
	rc.wg.Wait()
	rc.logger.Info("Reporter client stopped")
}

// Report sends a diagnostic report to the server
func (rc *ReporterClient) Report(ctx context.Context, report *Report) (*ReportResult, error) {
	reportID := fmt.Sprintf("%s-%d", report.CheckName, report.Timestamp.Unix())

	// Add to cache
	entry := &CacheEntry{
		Report:    report,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
	if err := rc.cache.Add(reportID, entry); err != nil {
		rc.logger.Error("Failed to add report to cache", zap.Error(err))
	}

	// Update metrics
	rc.updateMetrics(func(m *ReporterMetrics) {
		m.totalReports++
	})

	// Attempt to send report
	result := rc.sendReportWithRetry(ctx, report, reportID)

	// Update cache with result
	if existing, exists := rc.cache.Get(reportID); exists {
		existing.Status = result.Status
		existing.Attempts = result.Attempts
		existing.LastTry = time.Now()

		// Remove successful reports from cache
		if result.Status == StatusSuccess {
			rc.cache.Remove(reportID)
		}
	}

	return result, result.Error
}

// sendReportWithRetry sends the report with exponential backoff retry
func (rc *ReporterClient) sendReportWithRetry(ctx context.Context, report *Report, reportID string) *ReportResult {
	var lastErr error
	maxRetries := rc.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		startTime := time.Now()

		// Calculate exponential backoff delay
		delay := time.Duration(attempt) * rc.config.RetryDelay
		if delay < time.Second {
			delay = time.Second
		}

		if attempt > 0 {
			rc.logger.Debug("Retrying report",
				zap.String("report_id", reportID),
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))

			rc.updateMetrics(func(m *ReporterMetrics) {
				m.retryReports++
			})

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return &ReportResult{
					ReportID: reportID,
					Status:   StatusFailed,
					Error:    ctx.Err(),
					Attempts: attempt,
				}
			}
		}

		// Attempt to send
		err := rc.sendReport(ctx, report)
		duration := time.Since(startTime)

		if err == nil {
			rc.logger.Info("Report sent successfully",
				zap.String("report_id", reportID),
				zap.Int("attempts", attempt+1),
				zap.Duration("duration", duration))

			rc.updateMetrics(func(m *ReporterMetrics) {
				m.successReports++
				m.lastReportTime = time.Now()
			})

			return &ReportResult{
				ReportID: reportID,
				Status:   StatusSuccess,
				Attempts: attempt + 1,
				Duration: duration,
			}
		}

		lastErr = err

		// Check if this is the last attempt
		if attempt == maxRetries {
			rc.logger.Error("Failed to send report after max retries",
				zap.String("report_id", reportID),
				zap.Int("max_retries", maxRetries),
				zap.Error(err))

			rc.updateMetrics(func(m *ReporterMetrics) {
				m.failedReports++
			})

			return &ReportResult{
				ReportID: reportID,
				Status:   StatusFailed,
				Error:    fmt.Errorf("max retries exceeded: %w", err),
				Attempts: attempt + 1,
				Duration: duration,
			}
		}

		rc.logger.Warn("Report attempt failed, will retry",
			zap.String("report_id", reportID),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
	}

	return &ReportResult{
		ReportID: reportID,
		Status:   StatusFailed,
		Error:    lastErr,
		Attempts: maxRetries + 1,
	}
}

// sendReport sends a single report to the server
func (rc *ReporterClient) sendReport(ctx context.Context, report *Report) error {
	// Marshal report data
	jsonData, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	// Compress if enabled
	var body io.Reader = bytes.NewReader(jsonData)
	if rc.config.Compression {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(jsonData); err != nil {
			return fmt.Errorf("failed to compress data: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("failed to close gzip writer: %w", err)
		}
		body = &buf
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", rc.config.Endpoint, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if rc.config.Compression {
		req.Header.Set("Content-Encoding", "gzip")
	}

	// Send request
	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Read error response
	bodyBytes, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
}

// GetMetrics returns current reporting metrics
func (rc *ReporterClient) GetMetrics() ReporterMetrics {
	rc.metrics.mu.RLock()
	defer rc.metrics.mu.RUnlock()

	return ReporterMetrics{
		totalReports:     rc.metrics.totalReports,
		successReports:   rc.metrics.successReports,
		failedReports:    rc.metrics.failedReports,
		retryReports:     rc.metrics.retryReports,
		discardedReports: rc.metrics.discardedReports,
		lastReportTime:   rc.metrics.lastReportTime,
	}
}

// GetCacheStats returns cache statistics
func (rc *ReporterClient) GetCacheStats() map[string]interface{} {
	stats := rc.cache.GetStats()
	return map[string]interface{}{
		"cache_size": stats.Size,
		"total_size": stats.TotalSize,
		"status_counts": map[string]int{
			"pending":   stats.Pending,
			"failed":    stats.Failed,
			"success":   stats.Success,
			"discarded": stats.Discarded,
		},
		"expired": stats.Expired,
	}
}

// ClearCache clears all cached reports
func (rc *ReporterClient) ClearCache() {
	rc.cache.Clear()
	rc.logger.Info("Cache cleared")
}

// updateMetrics safely updates metrics
func (rc *ReporterClient) updateMetrics(update func(*ReporterMetrics)) {
	rc.metrics.mu.Lock()
	defer rc.metrics.mu.Unlock()
	update(rc.metrics)
}

// ConfigSubscriber interface implementation
func (rc *ReporterClient) OnConfigUpdate(newConfig *config.Config) error {
	rc.logger.Info("Reporter received config update")
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Update reporter configuration
	oldConfig := rc.config
	rc.config = newConfig.Reporter

	// Update HTTP client timeout
	rc.httpClient.Timeout = newConfig.Reporter.Timeout

	// Update cache configuration
	rc.cache.UpdateConfig(
		newConfig.Reporter.CacheMaxSize,
		newConfig.Reporter.CacheMaxAge,
	)

	// Log configuration changes
	if oldConfig.Endpoint != newConfig.Reporter.Endpoint {
		rc.logger.Info("Reporter endpoint updated",
			zap.String("old", oldConfig.Endpoint),
			zap.String("new", newConfig.Reporter.Endpoint))
	}

	if oldConfig.Timeout != newConfig.Reporter.Timeout {
		rc.logger.Info("Reporter timeout updated",
			zap.Duration("old", oldConfig.Timeout),
			zap.Duration("new", newConfig.Reporter.Timeout))
	}

	rc.logger.Info("Reporter config update completed")
	return nil
}
