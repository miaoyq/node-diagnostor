package processor

import (
	"context"
	"time"
)

// ProcessedData represents processed diagnostic data ready for reporting
type ProcessedData struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	NodeName  string                 `json:"node_name"`
	Data      map[string]interface{} `json:"data"`
	Metadata  map[string]string      `json:"metadata"`
	Version   string                 `json:"version"`
}

// Processor defines the interface for data processing
type Processor interface {
	// Process processes raw collected data into processed format
	Process(ctx context.Context, rawData interface{}) (*ProcessedData, error)

	// Aggregate aggregates multiple processed data points
	Aggregate(ctx context.Context, data []*ProcessedData) (*ProcessedData, error)

	// Validate validates processed data
	Validate(data *ProcessedData) error

	// GetVersion returns the processor version
	GetVersion() string
}

// Reporter defines the interface for data reporting
type Reporter interface {
	// Report sends processed data to the reporting endpoint
	Report(ctx context.Context, data *ProcessedData) error

	// ReportBatch sends multiple processed data points
	ReportBatch(ctx context.Context, data []*ProcessedData) error

	// GetEndpoint returns the reporting endpoint
	GetEndpoint() string

	// GetStatus returns the reporter status
	GetStatus() string

	// Close closes the reporter and cleans up resources
	Close() error
}

// Cache defines the interface for local data caching
type Cache interface {
	// Store stores data in the cache
	Store(ctx context.Context, key string, data *ProcessedData, ttl time.Duration) error

	// Retrieve retrieves data from the cache
	Retrieve(ctx context.Context, key string) (*ProcessedData, error)

	// Remove removes data from the cache
	Remove(ctx context.Context, key string) error

	// Clean removes expired data from the cache
	Clean(ctx context.Context) error

	// Size returns the current cache size
	Size() int
}
