package collector

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DiskCollector collects disk-related diagnostic data
type DiskCollector struct{}

// NewDiskCollector creates a new disk collector
func NewDiskCollector() *DiskCollector {
	return &DiskCollector{}
}

// Name returns the collector's unique identifier
func (d *DiskCollector) Name() string {
	return "disk"
}

// Collect performs disk data collection
func (d *DiskCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	data := &Data{
		Type:      "disk",
		Timestamp: time.Now(),
		Source:    "node-local",
		Data:      make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}

	// Collect disk usage
	if usage, err := d.collectDiskUsage(); err == nil {
		data.Data["usage"] = usage
	}

	// Collect disk errors
	if errors, err := d.collectDiskErrors(); err == nil {
		data.Data["errors"] = errors
	}

	// Collect SMART information
	if smart, err := d.collectSMARTInfo(); err == nil {
		data.Data["smart"] = smart
	}

	// Collect disk performance
	if performance, err := d.collectDiskPerformance(); err == nil {
		data.Data["performance"] = performance
	}

	return &Result{
		Data:    data,
		Error:   nil,
		Skipped: false,
		Reason:  "",
	}, nil
}

// collectDiskUsage collects disk usage information
func (d *DiskCollector) collectDiskUsage() (map[string]interface{}, error) {
	usage := make(map[string]interface{})

	// Read mount information
	mounts, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/mounts: %w", err)
	}
	defer mounts.Close()

	scanner := bufio.NewScanner(mounts)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			device := parts[0]
			mountPoint := parts[1]
			fsType := parts[2]

			// Skip virtual filesystems
			if strings.HasPrefix(device, "tmpfs") || strings.HasPrefix(device, "devtmpfs") ||
				strings.HasPrefix(device, "sysfs") || strings.HasPrefix(device, "proc") ||
				strings.HasPrefix(device, "devpts") || strings.HasPrefix(device, "cgroup") {
				continue
			}

			// Get disk usage stats
			stat := &syscall.Statfs_t{}
			if err := syscall.Statfs(mountPoint, stat); err == nil {
				total := stat.Blocks * uint64(stat.Bsize)
				free := stat.Bavail * uint64(stat.Bsize)
				used := total - free
				usagePercent := float64(used) * 100.0 / float64(total)

				usage[mountPoint] = map[string]interface{}{
					"device":        device,
					"filesystem":    fsType,
					"total_bytes":   total,
					"free_bytes":    free,
					"used_bytes":    used,
					"usage_percent": usagePercent,
				}
			}
		}
	}

	return usage, nil
}

// collectDiskErrors collects disk error information
func (d *DiskCollector) collectDiskErrors() (map[string]interface{}, error) {
	errors := make(map[string]interface{})

	// Read disk stats
	stats, err := os.Open("/proc/diskstats")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/diskstats: %w", err)
	}
	defer stats.Close()

	scanner := bufio.NewScanner(stats)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 14 {
			device := fields[2]
			readErrors, _ := strconv.ParseInt(fields[3], 10, 64)
			writeErrors, _ := strconv.ParseInt(fields[7], 10, 64)
			readSectors, _ := strconv.ParseInt(fields[5], 10, 64)
			writeSectors, _ := strconv.ParseInt(fields[9], 10, 64)

			errors[device] = map[string]interface{}{
				"read_errors":   readErrors,
				"write_errors":  writeErrors,
				"read_sectors":  readSectors,
				"write_sectors": writeSectors,
			}
		}
	}

	// Collect SMART errors if available
	smartErrors := d.collectSMARTErrors()
	if len(smartErrors) > 0 {
		errors["smart_errors"] = smartErrors
	}

	return errors, nil
}

// collectSMARTInfo collects SMART information from disks
func (d *DiskCollector) collectSMARTInfo() (map[string]interface{}, error) {
	smart := make(map[string]interface{})

	// Find all disk devices
	devices, err := filepath.Glob("/dev/sd*")
	if err != nil {
		return smart, nil
	}

	for _, device := range devices {
		deviceName := filepath.Base(device)
		smartData := make(map[string]interface{})

		// Try to read SMART attributes via smartctl (if available)
		// This is a simplified version - in production, use proper SMART library
		smartPath := fmt.Sprintf("/sys/block/%s/device/smart", deviceName)
		if _, err := os.Stat(smartPath); err == nil {
			if data, err := os.ReadFile(smartPath); err == nil {
				smartData["raw_data"] = string(data)
			}
		}

		// Read basic disk info
		sizePath := fmt.Sprintf("/sys/block/%s/size", deviceName)
		if sizeData, err := os.ReadFile(sizePath); err == nil {
			if size, err := strconv.ParseInt(strings.TrimSpace(string(sizeData)), 10, 64); err == nil {
				smartData["size_bytes"] = size * 512 // Convert sectors to bytes
			}
		}

		if len(smartData) > 0 {
			smart[deviceName] = smartData
		}
	}

	return smart, nil
}

// collectSMARTErrors collects SMART error information
func (d *DiskCollector) collectSMARTErrors() map[string]interface{} {
	errors := make(map[string]interface{})

	// Read kernel logs for disk errors
	logFiles := []string{"/var/log/kern.log", "/var/log/messages"}
	for _, logFile := range logFiles {
		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			continue
		}

		file, err := os.Open(logFile)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "I/O error") || strings.Contains(line, "disk error") ||
				strings.Contains(line, "SMART error") || strings.Contains(line, "hard disk error") {
				errors[line] = time.Now().Format(time.RFC3339)
			}
		}
		break // Only process first available log file
	}

	return errors
}

// collectDiskPerformance collects disk performance metrics
func (d *DiskCollector) collectDiskPerformance() (map[string]interface{}, error) {
	performance := make(map[string]interface{})

	// Read disk stats for performance metrics
	stats, err := os.Open("/proc/diskstats")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/diskstats: %w", err)
	}
	defer stats.Close()

	scanner := bufio.NewScanner(stats)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 14 {
			device := fields[2]

			// Calculate I/O statistics
			reads, _ := strconv.ParseInt(fields[3], 10, 64)
			readsMerged, _ := strconv.ParseInt(fields[4], 10, 64)
			readsSectors, _ := strconv.ParseInt(fields[5], 10, 64)
			readTime, _ := strconv.ParseInt(fields[6], 10, 64)

			writes, _ := strconv.ParseInt(fields[7], 10, 64)
			writesMerged, _ := strconv.ParseInt(fields[8], 10, 64)
			writesSectors, _ := strconv.ParseInt(fields[9], 10, 64)
			writeTime, _ := strconv.ParseInt(fields[10], 10, 64)

			ioInProgress, _ := strconv.ParseInt(fields[11], 10, 64)
			ioTime, _ := strconv.ParseInt(fields[12], 10, 64)
			ioWeightedTime, _ := strconv.ParseInt(fields[13], 10, 64)

			performance[device] = map[string]interface{}{
				"reads":               reads,
				"reads_merged":        readsMerged,
				"reads_sectors":       readsSectors,
				"read_time_ms":        readTime,
				"writes":              writes,
				"writes_merged":       writesMerged,
				"writes_sectors":      writesSectors,
				"write_time_ms":       writeTime,
				"io_in_progress":      ioInProgress,
				"io_time_ms":          ioTime,
				"io_weighted_time_ms": ioWeightedTime,
			}
		}
	}

	return performance, nil
}

// Validate validates the collector's parameters
func (d *DiskCollector) Validate(params map[string]interface{}) error {
	// Disk collector has no required parameters
	return nil
}

// IsLocalOnly indicates if this collector provides node-local data only
func (d *DiskCollector) IsLocalOnly() bool {
	return true
}

// IsClusterSupplement indicates if this collector provides cluster-level data
func (d *DiskCollector) IsClusterSupplement() bool {
	return false
}

// Priority returns the collector's priority level
func (d *DiskCollector) Priority() int {
	return 1
}

// Timeout returns the recommended timeout for this collector
func (d *DiskCollector) Timeout() time.Duration {
	return 10 * time.Second
}
