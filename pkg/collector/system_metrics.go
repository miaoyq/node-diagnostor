package collector

import (
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

// SystemMetrics 系统指标数据结构
type SystemMetrics struct {
	Timestamp time.Time `json:"timestamp"`
	CPU       float64   `json:"cpu"`     // CPU使用率(%)
	Memory    float64   `json:"memory"`  // 内存使用率(%)
	Disk      DiskUsage `json:"disk"`    // 磁盘使用情况
	Network   NetIO     `json:"network"` // 网络IO
}

// DiskUsage 磁盘使用情况
type DiskUsage struct {
	Total uint64  `json:"total"` // 总空间(字节)
	Used  uint64  `json:"used"`  // 已用空间(字节)
	Usage float64 `json:"usage"` // 使用率(%)
}

// NetIO 网络IO统计
type NetIO struct {
	BytesSent   uint64 `json:"bytes_sent"`   // 发送字节数
	BytesRecv   uint64 `json:"bytes_recv"`   // 接收字节数
	PacketsSent uint64 `json:"packets_sent"` // 发送包数
	PacketsRecv uint64 `json:"packets_recv"` // 接收包数
}

// SystemMetricsCollector 系统指标采集器
type SystemMetricsCollector struct {
	interval time.Duration // 采集间隔
	stopChan chan struct{} // 停止信号
}

// NewSystemMetricsCollector 创建系统指标采集器
func NewSystemMetricsCollector(interval time.Duration) *SystemMetricsCollector {
	return &SystemMetricsCollector{
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// Start 启动指标采集
func (c *SystemMetricsCollector) Start() chan SystemMetrics {
	metricsChan := make(chan SystemMetrics, 10)
	go c.collect(metricsChan)
	return metricsChan
}

// Stop 停止指标采集
func (c *SystemMetricsCollector) Stop() {
	close(c.stopChan)
}

// collect 定时采集系统指标
func (c *SystemMetricsCollector) collect(metricsChan chan SystemMetrics) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics, err := c.getSystemMetrics()
			if err == nil {
				metricsChan <- metrics
			}
		case <-c.stopChan:
			close(metricsChan)
			return
		}
	}
}

// getSystemMetrics 获取系统指标
func (c *SystemMetricsCollector) getSystemMetrics() (SystemMetrics, error) {
	var metrics SystemMetrics
	metrics.Timestamp = time.Now()

	// 获取CPU使用率
	cpuPercents, err := cpu.Percent(time.Second, false)
	if err == nil && len(cpuPercents) > 0 {
		metrics.CPU = cpuPercents[0]
	}

	// 获取内存使用率
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		metrics.Memory = memInfo.UsedPercent
	}

	// 获取磁盘使用情况
	diskUsage, err := disk.Usage("/")
	if err == nil {
		metrics.Disk = DiskUsage{
			Total: diskUsage.Total,
			Used:  diskUsage.Used,
			Usage: diskUsage.UsedPercent,
		}
	}

	// 获取网络IO
	netIO, err := net.IOCounters(false)
	if err == nil && len(netIO) > 0 {
		metrics.Network = NetIO{
			BytesSent:   netIO[0].BytesSent,
			BytesRecv:   netIO[0].BytesRecv,
			PacketsSent: netIO[0].PacketsSent,
			PacketsRecv: netIO[0].PacketsRecv,
		}
	}

	return metrics, nil
}
