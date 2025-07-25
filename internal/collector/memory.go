package collector

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// MemoryCollector collects memory-related diagnostic data
type MemoryCollector struct{}

// NewMemoryCollector creates a new memory collector
func NewMemoryCollector() *MemoryCollector {
	return &MemoryCollector{}
}

// Name returns the collector's unique identifier
func (m *MemoryCollector) Name() string {
	return "memory"
}

// Collect performs memory data collection
func (m *MemoryCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	data := &Data{
		Type:      "memory",
		Timestamp: time.Now(),
		Source:    "node-local",
		Data:      make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}

	// Collect memory usage
	if memInfo, err := m.collectMemoryInfo(); err == nil {
		data.Data["memory_info"] = memInfo
	}

	// Collect swap information
	if swapInfo, err := m.collectSwapInfo(); err == nil {
		data.Data["swap_info"] = swapInfo
	}

	// Collect memory fragmentation
	if fragmentation, err := m.collectMemoryFragmentation(); err == nil {
		data.Data["fragmentation"] = fragmentation
	}

	// Collect OOM events
	if oomEvents, err := m.collectOOMEvents(); err == nil {
		data.Data["oom_events"] = oomEvents
	}

	return &Result{
		Data:    data,
		Error:   nil,
		Skipped: false,
		Reason:  "",
	}, nil
}

// collectMemoryInfo collects memory usage information from /proc/meminfo
func (m *MemoryCollector) collectMemoryInfo() (map[string]interface{}, error) {
	memInfo := make(map[string]interface{})

	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/meminfo: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			key := strings.TrimSuffix(parts[0], ":")
			value, err := strconv.ParseInt(parts[1], 10, 64)
			if err == nil {
				memInfo[key] = value * 1024 // Convert KB to bytes
			}
		}
	}

	// Calculate usage percentages
	if total, ok := memInfo["MemTotal"].(int64); ok && total > 0 {
		if available, ok := memInfo["MemAvailable"].(int64); ok {
			used := total - available
			memInfo["MemUsed"] = used
			memInfo["MemUsagePercent"] = float64(used) * 100.0 / float64(total)
		}
	}

	return memInfo, nil
}

// collectSwapInfo collects swap usage information
func (m *MemoryCollector) collectSwapInfo() (map[string]interface{}, error) {
	swapInfo := make(map[string]interface{})

	file, err := os.Open("/proc/swaps")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/swaps: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		if lineNum == 1 {
			continue // Skip header
		}

		parts := strings.Fields(line)
		if len(parts) >= 5 {
			device := parts[0]
			swapInfo[device] = map[string]interface{}{
				"type":       parts[1],
				"size":       parts[2],
				"used":       parts[3],
				"priority":   parts[4],
				"size_bytes": parseInt64(parts[2]) * 1024,
				"used_bytes": parseInt64(parts[3]) * 1024,
			}
		}
	}

	return swapInfo, nil
}

// collectMemoryFragmentation collects memory fragmentation information
func (m *MemoryCollector) collectMemoryFragmentation() (map[string]interface{}, error) {
	fragmentation := make(map[string]interface{})

	// Try to read from /proc/buddyinfo
	file, err := os.Open("/proc/buddyinfo")
	if err != nil {
		return fragmentation, nil // Return empty if not available
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) >= 5 {
			zone := parts[2]
			fragmentation[zone] = parts[4:]
		}
	}

	// Read /proc/zoneinfo for more detailed fragmentation info
	zoneFile, err := os.Open("/proc/zoneinfo")
	if err == nil {
		defer zoneFile.Close()
		zoneScanner := bufio.NewScanner(zoneFile)
		currentZone := ""
		for zoneScanner.Scan() {
			line := zoneScanner.Text()
			if strings.HasPrefix(line, "Node") {
				currentZone = strings.TrimSpace(line)
			} else if strings.Contains(line, "free pages") {
				fragmentation[currentZone+"_free_pages"] = strings.TrimSpace(line)
			}
		}
	}

	return fragmentation, nil
}

// collectOOMEvents collects Out of Memory events from kernel logs
func (m *MemoryCollector) collectOOMEvents() ([]map[string]interface{}, error) {
	var oomEvents []map[string]interface{}

	// Read kernel messages for OOM events
	dmesgPath := "/var/log/kern.log"
	if _, err := os.Stat(dmesgPath); os.IsNotExist(err) {
		dmesgPath = "/var/log/messages"
	}

	file, err := os.Open(dmesgPath)
	if err != nil {
		return oomEvents, nil // Return empty if log file not accessible
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Out of memory") || strings.Contains(line, "Killed process") {
			event := map[string]interface{}{
				"message":   line,
				"timestamp": extractTimestamp(line),
			}
			oomEvents = append(oomEvents, event)
		}
	}

	// Limit to last 10 events
	if len(oomEvents) > 10 {
		oomEvents = oomEvents[len(oomEvents)-10:]
	}

	return oomEvents, nil
}

// Validate validates the collector's parameters
func (m *MemoryCollector) Validate(params map[string]interface{}) error {
	// Memory collector has no required parameters
	return nil
}

// IsLocalOnly indicates if this collector provides node-local data only
func (m *MemoryCollector) IsLocalOnly() bool {
	return true
}

// IsClusterSupplement indicates if this collector provides cluster-level data
func (m *MemoryCollector) IsClusterSupplement() bool {
	return false
}

// Priority returns the collector's priority level
func (m *MemoryCollector) Priority() int {
	return 1
}

// Timeout returns the recommended timeout for this collector
func (m *MemoryCollector) Timeout() time.Duration {
	return 5 * time.Second
}

// Helper functions
func parseInt64(s string) int64 {
	val, _ := strconv.ParseInt(s, 10, 64)
	return val
}

func extractTimestamp(line string) string {
	// 简单的日志时间戳提取
	// TODO: 更复杂的日志时间戳提取
	parts := strings.Fields(line)
	if len(parts) > 0 {
		return parts[0]
	}
	return time.Now().Format(time.RFC3339)
}
