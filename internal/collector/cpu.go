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

// CPUCollector collects CPU-related diagnostic data
type CPUCollector struct{}

// NewCPUCollector creates a new CPU collector
func NewCPUCollector() *CPUCollector {
	return &CPUCollector{}
}

// Name returns the collector's unique identifier
func (c *CPUCollector) Name() string {
	return "cpu"
}

// Collect performs CPU data collection
func (c *CPUCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	data := &Data{
		Type:      "cpu",
		Timestamp: time.Now(),
		Source:    "node-local",
		Data:      make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}

	// Collect CPU temperature
	if temp, err := c.collectCPUTemperature(); err == nil {
		data.Data["temperature"] = temp
	}

	// Collect CPU frequency
	if freq, err := c.collectCPUFrequency(); err == nil {
		data.Data["frequency"] = freq
	}

	// Collect CPU throttling events
	if throttling, err := c.collectCPUThrottling(); err == nil {
		data.Data["throttling"] = throttling
	}

	// Collect CPU usage
	if usage, err := c.collectCPUUsage(); err == nil {
		data.Data["usage"] = usage
	}

	return &Result{
		Data:    data,
		Error:   nil,
		Skipped: false,
		Reason:  "",
	}, nil
}

// collectCPUTemperature collects CPU temperature from thermal zones
func (c *CPUCollector) collectCPUTemperature() (map[string]float64, error) {
	temps := make(map[string]float64)

	thermalPath := "/sys/class/thermal"
	entries, err := os.ReadDir(thermalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read thermal zones: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "thermal_zone") {
			continue
		}

		zonePath := filepath.Join(thermalPath, entry.Name())
		typeFile := filepath.Join(zonePath, "type")
		tempFile := filepath.Join(zonePath, "temp")

		typeData, err := os.ReadFile(typeFile)
		if err != nil {
			continue
		}

		tempData, err := os.ReadFile(tempFile)
		if err != nil {
			continue
		}

		tempStr := strings.TrimSpace(string(tempData))
		temp, err := strconv.ParseFloat(tempStr, 64)
		if err != nil {
			continue
		}

		// Convert millidegree celsius to celsius
		temp = temp / 1000.0
		zoneType := strings.TrimSpace(string(typeData))
		temps[zoneType] = temp
	}

	return temps, nil
}

// collectCPUFrequency collects CPU frequency information
func (c *CPUCollector) collectCPUFrequency() (map[string]interface{}, error) {
	freq := make(map[string]interface{})

	// Read current CPU frequencies
	cpuinfoPath := "/proc/cpuinfo"
	file, err := os.Open(cpuinfoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open cpuinfo: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	cpuCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "processor\t:") {
			cpuCount++
		}
		if strings.HasPrefix(line, "cpu MHz\t\t:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				mhz, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				if err == nil {
					freq[fmt.Sprintf("cpu%d_mhz", cpuCount-1)] = mhz
				}
			}
		}
	}

	// Read scaling frequencies if available
	scalingPath := "/sys/devices/system/cpu/cpu0/cpufreq"
	if _, err := os.Stat(scalingPath); err == nil {
		if scalingMax, err := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/scaling_max_freq"); err == nil {
			if max, err := strconv.ParseInt(strings.TrimSpace(string(scalingMax)), 10, 64); err == nil {
				freq["scaling_max_khz"] = max / 1000
			}
		}
		if scalingMin, err := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/scaling_min_freq"); err == nil {
			if min, err := strconv.ParseInt(strings.TrimSpace(string(scalingMin)), 10, 64); err == nil {
				freq["scaling_min_khz"] = min / 1000
			}
		}
	}

	return freq, nil
}

// collectCPUThrottling collects CPU throttling events
func (c *CPUCollector) collectCPUThrottling() (map[string]interface{}, error) {
	throttling := make(map[string]interface{})

	// Read CPU throttling stats from /proc/stat
	statPath := "/proc/stat"
	file, err := os.Open(statPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open stat: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu") && !strings.HasPrefix(line, "cpu ") {
			parts := strings.Fields(line)
			if len(parts) >= 8 {
				cpu := parts[0]
				steal, _ := strconv.ParseInt(parts[8], 10, 64)
				if steal > 0 {
					throttling[cpu+"_steal_time"] = steal
				}
			}
		}
	}

	return throttling, nil
}

// collectCPUUsage collects CPU usage statistics
func (c *CPUCollector) collectCPUUsage() (map[string]interface{}, error) {
	usage := make(map[string]interface{})

	// Read load average
	loadavgPath := "/proc/loadavg"
	if data, err := os.ReadFile(loadavgPath); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			usage["load_avg_1min"], _ = strconv.ParseFloat(parts[0], 64)
			usage["load_avg_5min"], _ = strconv.ParseFloat(parts[1], 64)
			usage["load_avg_15min"], _ = strconv.ParseFloat(parts[2], 64)
		}
	}

	return usage, nil
}

// Validate validates the collector's parameters
func (c *CPUCollector) Validate(params map[string]interface{}) error {
	// CPU collector has no required parameters
	return nil
}

// IsLocalOnly indicates if this collector provides node-local data only
func (c *CPUCollector) IsLocalOnly() bool {
	return true
}

// IsClusterSupplement indicates if this collector provides cluster-level data
func (c *CPUCollector) IsClusterSupplement() bool {
	return false
}

// Priority returns the collector's priority level
func (c *CPUCollector) Priority() int {
	return 1
}

// Timeout returns the recommended timeout for this collector
func (c *CPUCollector) Timeout() time.Duration {
	return 5 * time.Second
}
