package collector

import (
	"time"

	"github.com/miaoyq/node-diagnostor/pkg/types"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// CollectSystemMetrics 收集系统指标
func CollectSystemMetrics() (*types.SystemMetrics, error) {
	metrics := &types.SystemMetrics{
		Timestamp: time.Now(),
	}

	// CPU使用率
	cpuPercents, err := cpu.Percent(time.Second, true)
	if err == nil && len(cpuPercents) > 0 {
		for _, percent := range cpuPercents {
			metrics.CPUUsage += percent
		}
	}

	// 内存使用
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		metrics.MemoryTotal = memInfo.Total
		metrics.MemoryUsed = memInfo.Used
	}

	// 磁盘使用
	partitions, err := disk.Partitions(false)
	if err == nil {
		for _, part := range partitions {
			usage, err := disk.Usage(part.Mountpoint)
			if err == nil {
				metrics.DiskUsage = append(metrics.DiskUsage, types.DiskUsage{
					MountPoint: part.Mountpoint,
					Total:      usage.Total,
					Used:       usage.Used,
				})
			}
		}
	}

	// 网络统计
	netStats, err := net.IOCounters(false)
	if err == nil && len(netStats) > 0 {
		metrics.NetworkStats = types.NetworkStats{
			BytesSent:   netStats[0].BytesSent,
			BytesRecv:   netStats[0].BytesRecv,
			PacketsSent: netStats[0].PacketsSent,
			PacketsRecv: netStats[0].PacketsRecv,
		}
	}

	return metrics, nil
}
