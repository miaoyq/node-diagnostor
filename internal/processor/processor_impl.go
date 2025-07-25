package processor

import (
	"context"
	"fmt"
	"time"

	"github.com/miaoyq/node-diagnostor/internal/collector"
	"go.uber.org/zap"
)

// DataProcessor implements the Processor interface
type DataProcessor struct {
	logger  *zap.Logger
	version string
}

// NewDataProcessor creates a new data processor
func NewDataProcessor() *DataProcessor {
	return &DataProcessor{
		logger:  zap.L(),
		version: "1.0.0",
	}
}

// Process processes raw collected data into processed format
func (p *DataProcessor) Process(ctx context.Context, rawData interface{}) (*ProcessedData, error) {
	p.logger.Debug("Processing raw data", zap.Any("raw_data", rawData))

	// Convert raw data to processed format
	processed := &ProcessedData{
		ID:        fmt.Sprintf("processed-%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Version:   p.version,
		Data:      make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}

	// Extract node name from context or use hostname
	processed.NodeName = "localhost" // TODO: Get actual node name

	// Process the raw data based on its type
	switch data := rawData.(type) {
	case map[string]interface{}:
		// Handle aggregated results from scheduler
		if taskName, ok := data["task_name"].(string); ok {
			processed.Type = taskName
			processed.Data = data
			processed.Metadata["source"] = "scheduler"
		} else {
			processed.Type = "generic"
			processed.Data = data
		}
	case *collector.Data:
		// Handle direct collector data
		processed.Type = data.Type
		processed.Data = data.Data
		processed.Metadata = data.Metadata
		processed.Metadata["source"] = data.Source
	default:
		// Handle unknown data types
		processed.Type = "unknown"
		processed.Data = map[string]interface{}{
			"raw": rawData,
		}
	}

	// Add processing metadata
	processed.Metadata["processor_version"] = p.version
	processed.Metadata["processed_at"] = time.Now().Format(time.RFC3339)

	// Validate processed data
	if err := p.Validate(processed); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return processed, nil
}

// Aggregate aggregates multiple processed data points
func (p *DataProcessor) Aggregate(ctx context.Context, data []*ProcessedData) (*ProcessedData, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("no data to aggregate")
	}

	// Create aggregated result
	aggregated := &ProcessedData{
		ID:        fmt.Sprintf("aggregated-%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Version:   p.version,
		NodeName:  data[0].NodeName, // Use first node's name
		Type:      "aggregated",
		Data:      make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}

	// Aggregate data
	var types []string
	for _, item := range data {
		types = append(types, item.Type)
	}

	aggregated.Data["items"] = data
	aggregated.Data["count"] = len(data)
	aggregated.Data["types"] = types

	// Aggregate metadata
	aggregated.Metadata["processor_version"] = p.version
	aggregated.Metadata["aggregated_at"] = time.Now().Format(time.RFC3339)
	aggregated.Metadata["item_count"] = fmt.Sprintf("%d", len(data))

	return aggregated, nil
}

// Validate validates processed data
func (p *DataProcessor) Validate(data *ProcessedData) error {
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}

	if data.ID == "" {
		return fmt.Errorf("data ID cannot be empty")
	}

	if data.Type == "" {
		return fmt.Errorf("data type cannot be empty")
	}

	if data.NodeName == "" {
		return fmt.Errorf("node name cannot be empty")
	}

	if data.Data == nil {
		return fmt.Errorf("data content cannot be nil")
	}

	return nil
}

// GetVersion returns the processor version
func (p *DataProcessor) GetVersion() string {
	return p.version
}

// HTTPReporter implements the Reporter interface for HTTP reporting
type HTTPReporter struct {
	endpoint string
	client   *HTTPClient
	logger   *zap.Logger
}

// NewHTTPReporter creates a new HTTP reporter
func NewHTTPReporter(endpoint string) *HTTPReporter {
	return &HTTPReporter{
		endpoint: endpoint,
		client:   NewHTTPClient(),
		logger:   zap.L(),
	}
}

// Report sends processed data to the reporting endpoint
func (r *HTTPReporter) Report(ctx context.Context, data *ProcessedData) error {
	r.logger.Debug("Reporting data", zap.String("endpoint", r.endpoint))
	return r.client.Post(ctx, r.endpoint, data)
}

// ReportBatch sends multiple processed data points
func (r *HTTPReporter) ReportBatch(ctx context.Context, data []*ProcessedData) error {
	r.logger.Debug("Reporting batch data",
		zap.String("endpoint", r.endpoint),
		zap.Int("count", len(data)))
	return r.client.Post(ctx, r.endpoint, data)
}

// GetEndpoint returns the reporting endpoint
func (r *HTTPReporter) GetEndpoint() string {
	return r.endpoint
}

// GetStatus returns the reporter status
func (r *HTTPReporter) GetStatus() string {
	return "ready"
}

// Close closes the reporter and cleans up resources
func (r *HTTPReporter) Close() error {
	return nil
}

// HTTPClient handles HTTP communication
type HTTPClient struct {
	timeout time.Duration
}

// NewHTTPClient creates a new HTTP client
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		timeout: 30 * time.Second,
	}
}

// Post sends data via HTTP POST
func (c *HTTPClient) Post(ctx context.Context, endpoint string, data interface{}) error {
	// TODO: Implement actual HTTP POST
	return nil
}
