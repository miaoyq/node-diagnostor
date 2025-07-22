package collector

import (
	"testing"
	"time"

	"github.com/miaoyq/node-diagnostor/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectSystemMetrics(t *testing.T) {
	t.Run("successful metrics collection", func(t *testing.T) {
		metrics, err := CollectSystemMetrics()

		assert.NoError(t, err)
		assert.NotNil(t, metrics)
		assert.NotZero(t, metrics.Timestamp)
	})

	t.Run("metrics structure validation", func(t *testing.T) {
		metrics, err := CollectSystemMetrics()

		require.NoError(t, err)
		assert.GreaterOrEqual(t, metrics.CPUUsage, 0.0)
		//assert.LessOrEqual(t, metrics.CPUUsage, 100.0)
		assert.Greater(t, metrics.MemoryTotal, uint64(0))
		assert.GreaterOrEqual(t, metrics.MemoryUsed, uint64(0))
		assert.LessOrEqual(t, metrics.MemoryUsed, metrics.MemoryTotal)
		assert.NotEmpty(t, metrics.DiskUsage)
	})

	t.Run("disk usage validation", func(t *testing.T) {
		metrics, err := CollectSystemMetrics()

		require.NoError(t, err)
		for _, disk := range metrics.DiskUsage {
			assert.NotEmpty(t, disk.MountPoint)
			assert.Greater(t, disk.Total, uint64(0))
			assert.GreaterOrEqual(t, disk.Used, uint64(0))
			assert.LessOrEqual(t, disk.Used, disk.Total)
		}
	})

	t.Run("network stats validation", func(t *testing.T) {
		metrics, err := CollectSystemMetrics()

		require.NoError(t, err)
		assert.GreaterOrEqual(t, metrics.NetworkStats.BytesSent, uint64(0))
		assert.GreaterOrEqual(t, metrics.NetworkStats.BytesRecv, uint64(0))
		assert.GreaterOrEqual(t, metrics.NetworkStats.PacketsSent, uint64(0))
		assert.GreaterOrEqual(t, metrics.NetworkStats.PacketsRecv, uint64(0))
	})
}

func TestCollectSystemMetrics_ErrorHandling(t *testing.T) {
	t.Run("multiple calls consistency", func(t *testing.T) {
		metrics1, err1 := CollectSystemMetrics()
		metrics2, err2 := CollectSystemMetrics()

		assert.NoError(t, err1)
		assert.NoError(t, err2)
		assert.NotNil(t, metrics1)
		assert.NotNil(t, metrics2)

		// 两次调用应该都成功
		assert.InDelta(t, metrics1.CPUUsage, metrics2.CPUUsage, 5.0)
	})

	t.Run("performance test", func(t *testing.T) {
		start := time.Now()
		metrics, err := CollectSystemMetrics()
		duration := time.Since(start)

		assert.NoError(t, err)
		assert.NotNil(t, metrics)
		assert.Less(t, duration, 5*time.Second, "Collection should complete within 5 seconds")
	})
}

func TestMemoryMetrics_Calculation(t *testing.T) {
	metrics, err := CollectSystemMetrics()

	require.NoError(t, err)

	if metrics.MemoryTotal > 0 {
		usagePercent := float64(metrics.MemoryUsed) / float64(metrics.MemoryTotal) * 100
		assert.GreaterOrEqual(t, usagePercent, 0.0)
		assert.LessOrEqual(t, usagePercent, 100.0)
	}
}

func TestDiskUsage_Validation(t *testing.T) {
	metrics, err := CollectSystemMetrics()

	require.NoError(t, err)

	for _, disk := range metrics.DiskUsage {
		if disk.Total > 0 {
			usagePercent := float64(disk.Used) / float64(disk.Total) * 100
			assert.GreaterOrEqual(t, usagePercent, 0.0)
			assert.LessOrEqual(t, usagePercent, 100.0)
		}
	}
}

func TestSystemMetrics_Validation(t *testing.T) {
	tests := []struct {
		name    string
		metrics *types.SystemMetrics
		wantErr bool
	}{
		{
			name: "valid metrics",
			metrics: &types.SystemMetrics{
				Timestamp:   time.Now(),
				CPUUsage:    50.5,
				MemoryTotal: 16384,
				MemoryUsed:  8192,
			},
			wantErr: false,
		},
		{
			name: "zero timestamp",
			metrics: &types.SystemMetrics{
				Timestamp: time.Time{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.metrics == nil {
				t.Skip("nil metrics test")
			}

			// 基本验证：确保时间戳不为零
			if tt.metrics.Timestamp.IsZero() {
				if !tt.wantErr {
					t.Errorf("Expected valid timestamp, got zero")
				}
			} else {
				if tt.wantErr {
					t.Errorf("Expected error for zero timestamp, but got valid")
				}
			}
		})
	}
}
