package types

import (
	"encoding/json"
	"os"
	"time"
)

type SystemMetrics struct {
	Timestamp    time.Time    `json:"timestamp"`
	CPUUsage     float64      `json:"cpuUsage"`
	MemoryTotal  uint64       `json:"memoryTotal"`
	MemoryUsed   uint64       `json:"memoryUsed"`
	DiskUsage    []DiskUsage  `json:"diskUsage"`
	NetworkStats NetworkStats `json:"networkStats"`
}

type DiskUsage struct {
	MountPoint string `json:"mountPoint"`
	Total      uint64 `json:"total"`
	Used       uint64 `json:"used"`
}

type NetworkStats struct {
	BytesSent   uint64 `json:"bytesSent"`
	BytesRecv   uint64 `json:"bytesRecv"`
	PacketsSent uint64 `json:"packetsSent"`
	PacketsRecv uint64 `json:"packetsRecv"`
}

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Unit      string    `json:"unit"`
	Message   string    `json:"message"`
	Priority  string    `json:"priority"` // "emerg", "alert", "crit", "err", "warning", "notice", "info", "debug"
}

type DiagnosticReport struct {
	NodeName     string        `json:"nodeName"`
	Timestamp    time.Time     `json:"timestamp"`
	Metrics      SystemMetrics `json:"metrics"`
	ErrorLogs    []LogEntry    `json:"errorLogs"`
	WarningLogs  []LogEntry    `json:"warningLogs"`
	CheckResults []CheckResult `json:"checkResults"`
}

type CheckResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass", "fail", "warning"
	Message string `json:"message"`
}

// SaveReportToFile 将诊断报告保存到JSON文件
func SaveReportToFile(report DiagnosticReport, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
