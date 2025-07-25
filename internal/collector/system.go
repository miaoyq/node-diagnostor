package collector

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SystemLoadCollector collects system load average data
type SystemLoadCollector struct{}

// NewSystemLoadCollector creates a new system load collector
func NewSystemLoadCollector() *SystemLoadCollector {
	return &SystemLoadCollector{}
}

// Name returns the collector's unique identifier
func (c *SystemLoadCollector) Name() string {
	return "system-load"
}

// Collect performs system load data collection
func (c *SystemLoadCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	data := &Data{
		Type:      "system-load",
		Timestamp: time.Now(),
		Source:    "node-local",
		Data:      make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}

	// Read load average from /proc/loadavg
	loadavgFile := "/proc/loadavg"
	content, err := os.ReadFile(loadavgFile)
	if err != nil {
		return &Result{
			Error: fmt.Errorf("failed to read loadavg: %w", err),
		}, nil
	}

	fields := strings.Fields(string(content))
	if len(fields) >= 3 {
		// Parse 1, 5, 15 minute load averages
		load1, _ := strconv.ParseFloat(fields[0], 64)
		load5, _ := strconv.ParseFloat(fields[1], 64)
		load15, _ := strconv.ParseFloat(fields[2], 64)

		data.Data["load1"] = load1
		data.Data["load5"] = load5
		data.Data["load15"] = load15

		// Get CPU count for load percentage calculation
		cpuCount := getCPUCount()
		if cpuCount > 0 {
			data.Data["load1_percent"] = (load1 / float64(cpuCount)) * 100
			data.Data["load5_percent"] = (load5 / float64(cpuCount)) * 100
			data.Data["load15_percent"] = (load15 / float64(cpuCount)) * 100
		}
	}

	// Get current processes count
	if len(fields) >= 4 {
		processInfo := strings.Split(fields[3], "/")
		if len(processInfo) == 2 {
			runningProcs, _ := strconv.Atoi(processInfo[0])
			totalProcs, _ := strconv.Atoi(processInfo[1])
			data.Data["running_processes"] = runningProcs
			data.Data["total_processes"] = totalProcs
		}
	}

	data.Metadata["unit"] = "load"
	data.Metadata["cpu_count"] = fmt.Sprintf("%d", getCPUCount())

	return &Result{Data: data}, nil
}

// Validate validates the collector's parameters
func (c *SystemLoadCollector) Validate(params map[string]interface{}) error {
	return nil // No parameters required
}

// IsLocalOnly indicates if this collector provides node-local data only
func (c *SystemLoadCollector) IsLocalOnly() bool {
	return true
}

// IsClusterSupplement indicates if this collector provides cluster-level data
func (c *SystemLoadCollector) IsClusterSupplement() bool {
	return false
}

// Priority returns the collector's priority level
func (c *SystemLoadCollector) Priority() int {
	return 5
}

// Timeout returns the recommended timeout for this collector
func (c *SystemLoadCollector) Timeout() time.Duration {
	return 5 * time.Second
}

// FileDescriptorCollector collects file descriptor usage data
type FileDescriptorCollector struct{}

// NewFileDescriptorCollector creates a new file descriptor collector
func NewFileDescriptorCollector() *FileDescriptorCollector {
	return &FileDescriptorCollector{}
}

// Name returns the collector's unique identifier
func (c *FileDescriptorCollector) Name() string {
	return "file-descriptor"
}

// Collect performs file descriptor data collection
func (c *FileDescriptorCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	data := &Data{
		Type:      "file-descriptor",
		Timestamp: time.Now(),
		Source:    "node-local",
		Data:      make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}

	// Read file descriptor limits from /proc/sys/fs/file-nr
	fdFile := "/proc/sys/fs/file-nr"
	content, err := os.ReadFile(fdFile)
	if err != nil {
		return &Result{
			Error: fmt.Errorf("failed to read file-nr: %w", err),
		}, nil
	}

	fields := strings.Fields(string(content))
	if len(fields) >= 3 {
		allocated, _ := strconv.Atoi(fields[0])
		free, _ := strconv.Atoi(fields[1])
		max, _ := strconv.Atoi(fields[2])

		data.Data["allocated"] = allocated
		data.Data["free"] = free
		data.Data["max"] = max
		data.Data["used"] = allocated - free
		data.Data["usage_percent"] = float64(allocated-free) / float64(max) * 100
	}

	// Get per-process file descriptor usage
	procFDs, err := getProcessFDUsage()
	if err == nil {
		data.Data["process_fd_usage"] = procFDs
	}

	data.Metadata["unit"] = "count"

	return &Result{Data: data}, nil
}

// Validate validates the collector's parameters
func (c *FileDescriptorCollector) Validate(params map[string]interface{}) error {
	return nil // No parameters required
}

// IsLocalOnly indicates if this collector provides node-local data only
func (c *FileDescriptorCollector) IsLocalOnly() bool {
	return true
}

// IsClusterSupplement indicates if this collector provides cluster-level data
func (c *FileDescriptorCollector) IsClusterSupplement() bool {
	return false
}

// Priority returns the collector's priority level
func (c *FileDescriptorCollector) Priority() int {
	return 5
}

// Timeout returns the recommended timeout for this collector
func (c *FileDescriptorCollector) Timeout() time.Duration {
	return 5 * time.Second
}

// KernelLogCollector collects kernel log messages
type KernelLogCollector struct{}

// NewKernelLogCollector creates a new kernel log collector
func NewKernelLogCollector() *KernelLogCollector {
	return &KernelLogCollector{}
}

// Name returns the collector's unique identifier
func (c *KernelLogCollector) Name() string {
	return "kernel-log"
}

// Collect performs kernel log data collection
func (c *KernelLogCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	data := &Data{
		Type:      "kernel-log",
		Timestamp: time.Now(),
		Source:    "node-local",
		Data:      make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}

	// Read kernel messages from /var/log/kern.log or use dmesg
	kernLogFile := "/var/log/kern.log"

	var messages []string

	// Try to read from kern.log first
	if _, statErr := os.Stat(kernLogFile); statErr == nil {
		file, err := os.Open(kernLogFile)
		if err == nil {
			defer file.Close()

			scanner := bufio.NewScanner(file)
			lineCount := 0
			maxLines := 100 // Limit to recent messages

			for scanner.Scan() && lineCount < maxLines {
				line := scanner.Text()
				if strings.Contains(line, "error") || strings.Contains(line, "warning") ||
					strings.Contains(line, "fail") || strings.Contains(line, "oops") {
					messages = append(messages, line)
				}
				lineCount++
			}
		}
	}

	// Fallback to dmesg if kern.log not available
	if len(messages) == 0 {
		// Note: In real implementation, we might use exec.Command("dmesg", "--ctime")
		// For now, we'll use a simpler approach
		dmesgFile := "/var/log/dmesg"
		if _, statErr := os.Stat(dmesgFile); statErr == nil {
			content, readErr := os.ReadFile(dmesgFile)
			if readErr == nil {
				lines := strings.Split(string(content), "\n")
				for i := len(lines) - 1; i >= 0 && len(messages) < 20; i-- {
					line := lines[i]
					if strings.Contains(line, "error") || strings.Contains(line, "warning") {
						messages = append(messages, line)
					}
				}
			}
		}
	}

	data.Data["messages"] = messages
	data.Data["count"] = len(messages)
	data.Metadata["source"] = "kernel"
	data.Metadata["type"] = "log"

	return &Result{Data: data}, nil
}

// Validate validates the collector's parameters
func (c *KernelLogCollector) Validate(params map[string]interface{}) error {
	return nil // No parameters required
}

// IsLocalOnly indicates if this collector provides node-local data only
func (c *KernelLogCollector) IsLocalOnly() bool {
	return true
}

// IsClusterSupplement indicates if this collector provides cluster-level data
func (c *KernelLogCollector) IsClusterSupplement() bool {
	return false
}

// Priority returns the collector's priority level
func (c *KernelLogCollector) Priority() int {
	return 3
}

// Timeout returns the recommended timeout for this collector
func (c *KernelLogCollector) Timeout() time.Duration {
	return 10 * time.Second
}

// NTPSyncCollector collects NTP synchronization status
type NTPSyncCollector struct{}

// NewNTPSyncCollector creates a new NTP sync collector
func NewNTPSyncCollector() *NTPSyncCollector {
	return &NTPSyncCollector{}
}

// Name returns the collector's unique identifier
func (c *NTPSyncCollector) Name() string {
	return "ntp-sync"
}

// Collect performs NTP sync data collection
func (c *NTPSyncCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	data := &Data{
		Type:      "ntp-sync",
		Timestamp: time.Now(),
		Source:    "node-local",
		Data:      make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}

	// Check if chronyd is running
	chronyStatus := checkServiceStatus("chronyd")
	data.Data["chronyd_running"] = chronyStatus

	// Check if ntpd is running
	ntpStatus := checkServiceStatus("ntpd")
	data.Data["ntpd_running"] = ntpStatus

	// Check if systemd-timesyncd is running
	timesyncStatus := checkServiceStatus("systemd-timesyncd")
	data.Data["systemd_timesyncd_running"] = timesyncStatus

	// Get NTP synchronization status from /etc/adjtime
	adjtimeFile := "/etc/adjtime"
	if _, err := os.Stat(adjtimeFile); err == nil {
		content, err := os.ReadFile(adjtimeFile)
		if err == nil {
			lines := strings.Split(string(content), "\n")
			if len(lines) >= 3 {
				data.Data["last_adjustment"] = strings.TrimSpace(lines[2])
			}
		}
	}

	// Check system clock synchronization status
	clockFile := "/sys/class/rtc/rtc0/since_epoch"
	if _, err := os.Stat(clockFile); err == nil {
		content, err := os.ReadFile(clockFile)
		if err == nil {
			epoch, _ := strconv.ParseInt(strings.TrimSpace(string(content)), 10, 64)
			data.Data["hardware_clock_epoch"] = epoch
		}
	}

	// Calculate time drift (simplified)
	currentTime := time.Now()
	data.Data["system_time"] = currentTime.Format(time.RFC3339)
	data.Data["timezone"] = currentTime.Location().String()

	// Determine sync status
	syncStatus := "unknown"
	if chronyStatus || ntpStatus || timesyncStatus {
		syncStatus = "active"
	}
	data.Data["sync_status"] = syncStatus

	data.Metadata["unit"] = "status"

	return &Result{Data: data}, nil
}

// Validate validates the collector's parameters
func (c *NTPSyncCollector) Validate(params map[string]interface{}) error {
	return nil // No parameters required
}

// IsLocalOnly indicates if this collector provides node-local data only
func (c *NTPSyncCollector) IsLocalOnly() bool {
	return true
}

// IsClusterSupplement indicates if this collector provides cluster-level data
func (c *NTPSyncCollector) IsClusterSupplement() bool {
	return false
}

// Priority returns the collector's priority level
func (c *NTPSyncCollector) Priority() int {
	return 4
}

// Timeout returns the recommended timeout for this collector
func (c *NTPSyncCollector) Timeout() time.Duration {
	return 5 * time.Second
}

// Helper functions

func getCPUCount() int {
	cpuInfoFile := "/proc/cpuinfo"
	content, err := os.ReadFile(cpuInfoFile)
	if err != nil {
		return 0
	}

	count := 0
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "processor") {
			count++
		}
	}
	return count
}

func getProcessFDUsage() (map[string]interface{}, error) {
	procDir := "/proc"
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid := entry.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue // Skip non-numeric directories
		}

		fdDir := filepath.Join(procDir, pid, "fd")
		fdEntries, err := os.ReadDir(fdDir)
		if err != nil {
			continue // Skip processes we can't read
		}

		fdCount := len(fdEntries)
		result[pid] = map[string]interface{}{
			"fd_count": fdCount,
		}
	}

	return result, nil
}

func checkServiceStatus(serviceName string) bool {
	// Simplified service status check
	// In real implementation, we might use systemctl or service commands
	// For now, we'll check if the process is running
	procDir := "/proc"
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid := entry.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}

		commFile := filepath.Join(procDir, pid, "comm")
		content, err := os.ReadFile(commFile)
		if err != nil {
			continue
		}

		if strings.Contains(strings.ToLower(string(content)), strings.ToLower(serviceName)) {
			return true
		}
	}

	return false
}
