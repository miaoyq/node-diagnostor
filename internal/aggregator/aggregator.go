package aggregator

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/miaoyq/node-diagnostor/internal/collector"
)

// AggregatedData represents aggregated diagnostic data for a check item
type AggregatedData struct {
	CheckName   string            `json:"check_name"`
	Timestamp   time.Time         `json:"timestamp"`
	NodeName    string            `json:"node_name"`
	DataPoints  []DataPoint       `json:"data_points"`
	Metadata    map[string]string `json:"metadata"`
	Compressed  bool              `json:"compressed"`
	Compression string            `json:"compression,omitempty"`
	Size        int               `json:"size_bytes"`
}

// DataPoint represents a single data point from a collector
type DataPoint struct {
	Collector string                 `json:"collector"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	Metadata  map[string]string      `json:"metadata"`
	Source    string                 `json:"source"`
}

// Config holds configuration for the data aggregator
type Config struct {
	MaxDataPoints   int           `json:"max_data_points"`
	Compression     bool          `json:"compression"`
	CompressionType string        `json:"compression_type"`
	MaxSizeBytes    int           `json:"max_size_bytes"`
	RetentionPeriod time.Duration `json:"retention_period"`
}

// DataAggregator handles data aggregation, formatting, and compression
type DataAggregator struct {
	config    Config
	mu        sync.RWMutex
	cache     map[string]*AggregatedData
	nodeName  string
	startTime time.Time
}

// New creates a new DataAggregator instance
func New(config Config, nodeName string) *DataAggregator {
	return &DataAggregator{
		config:    config,
		cache:     make(map[string]*AggregatedData),
		nodeName:  nodeName,
		startTime: time.Now(),
	}
}

// Aggregate aggregates collected data by check item
func (da *DataAggregator) Aggregate(ctx context.Context, checkName string, results []*collector.Result) (*AggregatedData, error) {
	if checkName == "" {
		return nil, fmt.Errorf("check name cannot be empty")
	}

	da.mu.Lock()
	defer da.mu.Unlock()

	// Filter valid results
	validResults := da.filterValidResults(results)
	if len(validResults) == 0 {
		return nil, fmt.Errorf("no valid results to aggregate")
	}

	// Create data points
	dataPoints := make([]DataPoint, 0, len(validResults))
	for _, result := range validResults {
		if result.Data == nil {
			continue
		}

		dataPoint := DataPoint{
			Collector: result.Data.Type,
			Timestamp: result.Data.Timestamp,
			Data:      result.Data.Data,
			Metadata:  result.Data.Metadata,
			Source:    result.Data.Source,
		}
		dataPoints = append(dataPoints, dataPoint)
	}

	// Sort data points by timestamp
	sort.Slice(dataPoints, func(i, j int) bool {
		return dataPoints[i].Timestamp.Before(dataPoints[j].Timestamp)
	})

	// Limit data points if configured
	if da.config.MaxDataPoints > 0 && len(dataPoints) > da.config.MaxDataPoints {
		dataPoints = dataPoints[len(dataPoints)-da.config.MaxDataPoints:]
	}

	// Create aggregated data
	aggregated := &AggregatedData{
		CheckName:  checkName,
		Timestamp:  time.Now(),
		NodeName:   da.nodeName,
		DataPoints: dataPoints,
		Metadata: map[string]string{
			"total_data_points": fmt.Sprintf("%d", len(dataPoints)),
			"node_name":         da.nodeName,
			"aggregation_time":  time.Now().Format(time.RFC3339),
		},
	}

	// Add compression if enabled
	if da.config.Compression {
		if err := da.compress(aggregated); err != nil {
			return nil, fmt.Errorf("failed to compress data: %w", err)
		}
	}

	// Calculate size
	size, err := da.calculateSize(aggregated)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate size: %w", err)
	}
	aggregated.Size = size

	// Cache the result
	da.cache[checkName] = aggregated

	return aggregated, nil
}

// filterValidResults filters out invalid or skipped results
func (da *DataAggregator) filterValidResults(results []*collector.Result) []*collector.Result {
	valid := make([]*collector.Result, 0, len(results))
	for _, result := range results {
		if result == nil || result.Skipped || result.Error != nil {
			continue
		}
		if result.Data == nil {
			continue
		}
		valid = append(valid, result)
	}
	return valid
}

// compress compresses the aggregated data
func (da *DataAggregator) compress(aggregated *AggregatedData) error {
	if len(aggregated.DataPoints) == 0 {
		return nil
	}

	// Marshal data points to JSON
	jsonData, err := json.Marshal(aggregated.DataPoints)
	if err != nil {
		return fmt.Errorf("failed to marshal data points: %w", err)
	}

	// Compress using gzip
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(jsonData); err != nil {
		return fmt.Errorf("failed to compress data: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Replace data points with compressed data
	aggregated.DataPoints = []DataPoint{
		{
			Collector: "compressed",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"compressed_data": buf.Bytes(),
				"original_size":   len(jsonData),
				"compressed_size": buf.Len(),
			},
			Metadata: map[string]string{
				"compression_type":  "gzip",
				"compression_ratio": fmt.Sprintf("%.2f", float64(buf.Len())/float64(len(jsonData))),
			},
			Source: "aggregator",
		},
	}
	aggregated.Compressed = true
	aggregated.Compression = "gzip"

	return nil
}

// calculateSize calculates the size of the aggregated data in bytes
func (da *DataAggregator) calculateSize(aggregated *AggregatedData) (int, error) {
	jsonData, err := json.Marshal(aggregated)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal aggregated data: %w", err)
	}
	return len(jsonData), nil
}

// Get returns aggregated data for a specific check
func (da *DataAggregator) Get(checkName string) (*AggregatedData, bool) {
	da.mu.RLock()
	defer da.mu.RUnlock()

	data, exists := da.cache[checkName]
	return data, exists
}

// GetAll returns all aggregated data
func (da *DataAggregator) GetAll() []*AggregatedData {
	da.mu.RLock()
	defer da.mu.RUnlock()

	result := make([]*AggregatedData, 0, len(da.cache))
	for _, data := range da.cache {
		result = append(result, data)
	}

	// Sort by check name for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].CheckName < result[j].CheckName
	})

	return result
}

// Clean removes old data based on retention period
func (da *DataAggregator) Clean() int {
	da.mu.Lock()
	defer da.mu.Unlock()

	if da.config.RetentionPeriod <= 0 {
		return 0
	}

	cutoffTime := time.Now().Add(-da.config.RetentionPeriod)
	removed := 0

	for checkName, data := range da.cache {
		if data.Timestamp.Before(cutoffTime) {
			delete(da.cache, checkName)
			removed++
		}
	}

	return removed
}

// GetStats returns statistics about the aggregator
func (da *DataAggregator) GetStats() map[string]interface{} {
	da.mu.RLock()
	defer da.mu.RUnlock()

	totalSize := 0
	totalDataPoints := 0
	for _, data := range da.cache {
		totalSize += data.Size
		totalDataPoints += len(data.DataPoints)
	}

	return map[string]interface{}{
		"total_checks":      float64(len(da.cache)),
		"total_size_bytes":  float64(totalSize),
		"total_data_points": float64(totalDataPoints),
		"uptime_seconds":    time.Since(da.startTime).Seconds(),
	}
}

// FormatData formats data according to standard format
func (da *DataAggregator) FormatData(data interface{}) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

// ValidateSize checks if data size is within limits
func (da *DataAggregator) ValidateSize(data []byte) error {
	if da.config.MaxSizeBytes > 0 && len(data) > da.config.MaxSizeBytes {
		return fmt.Errorf("data size %d exceeds limit %d", len(data), da.config.MaxSizeBytes)
	}
	return nil
}
