package aggregator

import (
	"context"
	"testing"
	"time"

	"github.com/miaoyq/node-diagnostor/internal/collector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDataAggregator(t *testing.T) {
	config := Config{
		MaxDataPoints:   100,
		Compression:     true,
		CompressionType: "gzip",
		MaxSizeBytes:    1024 * 1024, // 1MB
		RetentionPeriod: time.Hour,
	}

	aggregator := New(config, "test-node")
	assert.NotNil(t, aggregator)
	assert.Equal(t, "test-node", aggregator.nodeName)
	assert.Equal(t, config, aggregator.config)
}

func TestAggregate(t *testing.T) {
	config := Config{
		MaxDataPoints:   10,
		Compression:     false,
		MaxSizeBytes:    1024 * 1024,
		RetentionPeriod: time.Hour,
	}

	aggregator := New(config, "test-node")
	ctx := context.Background()

	// Create test results
	results := []*collector.Result{
		{
			Data: &collector.Data{
				Type:      "cpu",
				Timestamp: time.Now(),
				Source:    "cpu-collector",
				Data: map[string]interface{}{
					"temperature": 45.5,
					"frequency":   2400,
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
				Timestamp: time.Now().Add(-time.Minute),
				Source:    "memory-collector",
				Data: map[string]interface{}{
					"usage_percent": 65.2,
					"available_mb":  2048,
				},
				Metadata: map[string]string{
					"unit": "percent",
				},
			},
			Error:   nil,
			Skipped: false,
		},
	}

	// Test aggregation
	aggregated, err := aggregator.Aggregate(ctx, "system-check", results)
	require.NoError(t, err)
	assert.NotNil(t, aggregated)
	assert.Equal(t, "system-check", aggregated.CheckName)
	assert.Equal(t, "test-node", aggregated.NodeName)
	assert.Len(t, aggregated.DataPoints, 2)
	assert.Equal(t, 2, len(aggregated.DataPoints))
}

func TestAggregateWithCompression(t *testing.T) {
	config := Config{
		MaxDataPoints:   10,
		Compression:     true,
		CompressionType: "gzip",
		MaxSizeBytes:    1024 * 1024,
		RetentionPeriod: time.Hour,
	}

	aggregator := New(config, "test-node")
	ctx := context.Background()

	// Create test results
	results := []*collector.Result{
		{
			Data: &collector.Data{
				Type:      "cpu",
				Timestamp: time.Now(),
				Source:    "cpu-collector",
				Data: map[string]interface{}{
					"temperature": 45.5,
					"frequency":   2400,
				},
				Metadata: map[string]string{
					"unit": "celsius",
				},
			},
			Error:   nil,
			Skipped: false,
		},
	}

	// Test aggregation with compression
	aggregated, err := aggregator.Aggregate(ctx, "system-check", results)
	require.NoError(t, err)
	assert.NotNil(t, aggregated)
	assert.True(t, aggregated.Compressed)
	assert.Equal(t, "gzip", aggregated.Compression)
	assert.Len(t, aggregated.DataPoints, 1)
	assert.Equal(t, "compressed", aggregated.DataPoints[0].Collector)
}

func TestFilterValidResults(t *testing.T) {
	config := Config{}
	aggregator := New(config, "test-node")

	results := []*collector.Result{
		{
			Data: &collector.Data{
				Type: "valid",
			},
			Error:   nil,
			Skipped: false,
		},
		{
			Data:    nil,
			Error:   nil,
			Skipped: false,
		},
		{
			Data: &collector.Data{
				Type: "skipped",
			},
			Error:   nil,
			Skipped: true,
		},
		{
			Data: &collector.Data{
				Type: "error",
			},
			Error:   assert.AnError,
			Skipped: false,
		},
	}

	valid := aggregator.filterValidResults(results)
	assert.Len(t, valid, 1)
	assert.Equal(t, "valid", valid[0].Data.Type)
}

func TestGetAndGetAll(t *testing.T) {
	config := Config{}
	aggregator := New(config, "test-node")
	ctx := context.Background()

	// Create test results
	results := []*collector.Result{
		{
			Data: &collector.Data{
				Type: "test",
			},
			Error:   nil,
			Skipped: false,
		},
	}

	// Aggregate data
	_, err := aggregator.Aggregate(ctx, "test-check", results)
	require.NoError(t, err)

	// Test Get
	data, exists := aggregator.Get("test-check")
	assert.True(t, exists)
	assert.NotNil(t, data)
	assert.Equal(t, "test-check", data.CheckName)

	// Test GetAll
	allData := aggregator.GetAll()
	assert.Len(t, allData, 1)
	assert.Equal(t, "test-check", allData[0].CheckName)
}

func TestClean(t *testing.T) {
	config := Config{
		RetentionPeriod: time.Second,
	}
	aggregator := New(config, "test-node")
	ctx := context.Background()

	// Create test results
	results := []*collector.Result{
		{
			Data: &collector.Data{
				Type: "test",
			},
			Error:   nil,
			Skipped: false,
		},
	}

	// Aggregate data
	_, err := aggregator.Aggregate(ctx, "test-check", results)
	require.NoError(t, err)

	// Verify data exists
	assert.Len(t, aggregator.GetAll(), 1)

	// Wait for retention period to expire
	time.Sleep(2 * time.Second)

	// Clean old data
	removed := aggregator.Clean()
	assert.Equal(t, 1, removed)
	assert.Len(t, aggregator.GetAll(), 0)
}

func TestGetStats(t *testing.T) {
	config := Config{}
	aggregator := New(config, "test-node")
	ctx := context.Background()

	// Create test results
	results := []*collector.Result{
		{
			Data: &collector.Data{
				Type: "test1",
			},
			Error:   nil,
			Skipped: false,
		},
		{
			Data: &collector.Data{
				Type: "test2",
			},
			Error:   nil,
			Skipped: false,
		},
	}

	// Aggregate data
	_, err := aggregator.Aggregate(ctx, "check1", results[:1])
	require.NoError(t, err)
	_, err = aggregator.Aggregate(ctx, "check2", results[1:])
	require.NoError(t, err)

	// Test stats
	stats := aggregator.GetStats()
	assert.Equal(t, float64(2), stats["total_checks"].(float64))
	assert.Equal(t, float64(2), stats["total_data_points"].(float64))
	assert.Greater(t, stats["uptime_seconds"].(float64), 0.0)
}

func TestValidateSize(t *testing.T) {
	config := Config{
		MaxSizeBytes: 100,
	}
	aggregator := New(config, "test-node")

	// Test valid size
	err := aggregator.ValidateSize([]byte("small data"))
	assert.NoError(t, err)

	// Test invalid size
	largeData := make([]byte, 200)
	err = aggregator.ValidateSize(largeData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds limit")
}

func TestFormatData(t *testing.T) {
	config := Config{}
	aggregator := New(config, "test-node")

	data := map[string]interface{}{
		"key": "value",
		"num": 42,
	}

	formatted, err := aggregator.FormatData(data)
	require.NoError(t, err)
	assert.Contains(t, string(formatted), "\"key\": \"value\"")
	assert.Contains(t, string(formatted), "\"num\": 42")
}

func TestAggregateEmptyResults(t *testing.T) {
	config := Config{}
	aggregator := New(config, "test-node")
	ctx := context.Background()

	// Test with empty results
	_, err := aggregator.Aggregate(ctx, "empty-check", []*collector.Result{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid results to aggregate")
}

func TestAggregateInvalidCheckName(t *testing.T) {
	config := Config{}
	aggregator := New(config, "test-node")
	ctx := context.Background()

	_, err := aggregator.Aggregate(ctx, "", []*collector.Result{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check name cannot be empty")
}
