package collector

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DiagnosticCheck 诊断检查结果
type DiagnosticCheck struct {
	Name      string                 `json:"name"`      // 检查项名称
	Status    string                 `json:"status"`    // 检查状态(OK/WARN/ERROR)
	Message   string                 `json:"message"`   // 检查结果消息
	Details   map[string]interface{} `json:"details"`   // 详细检查结果
	Timestamp time.Time              `json:"timestamp"` // 检查时间
}

// DiagnosticChecker 诊断检查器
type DiagnosticChecker struct {
	checks []func() DiagnosticCheck // 检查函数列表
}

// NewDiagnosticChecker 创建诊断检查器
func NewDiagnosticChecker() *DiagnosticChecker {
	return &DiagnosticChecker{
		checks: []func() DiagnosticCheck{
			checkKernelParameters,
			checkDStateProcesses,
			checkFileDescriptors,
		},
	}
}

// RunChecks 执行所有诊断检查
func (c *DiagnosticChecker) RunChecks() []DiagnosticCheck {
	var results []DiagnosticCheck
	for _, check := range c.checks {
		results = append(results, check())
	}
	return results
}

// checkKernelParameters 检查内核参数
func checkKernelParameters() DiagnosticCheck {
	result := DiagnosticCheck{
		Name:      "kernel_parameters",
		Status:    "OK",
		Message:   "All kernel parameters are within expected ranges",
		Details:   make(map[string]interface{}),
		Timestamp: time.Now(),
	}

	// 检查vm.swappiness参数
	swappiness, err := exec.Command("sysctl", "-n", "vm.swappiness").Output()
	if err == nil {
		value, _ := strconv.Atoi(strings.TrimSpace(string(swappiness)))
		result.Details["vm.swappiness"] = value
		if value > 60 {
			result.Status = "WARN"
			result.Message = "vm.swappiness is set too high"
		}
	}

	// 检查net.core.somaxconn参数
	somaxconn, err := exec.Command("sysctl", "-n", "net.core.somaxconn").Output()
	if err == nil {
		value, _ := strconv.Atoi(strings.TrimSpace(string(somaxconn)))
		result.Details["net.core.somaxconn"] = value
		if value < 1024 {
			result.Status = "WARN"
			result.Message = "net.core.somaxconn is set too low"
		}
	}

	return result
}

// checkDStateProcesses 检查D状态进程
func checkDStateProcesses() DiagnosticCheck {
	result := DiagnosticCheck{
		Name:      "d_state_processes",
		Status:    "OK",
		Message:   "No processes in D state found",
		Details:   make(map[string]interface{}),
		Timestamp: time.Now(),
	}

	// 使用ps命令查找D状态进程
	output, err := exec.Command("ps", "-eo", "state,pid,comm").Output()
	if err != nil {
		result.Status = "ERROR"
		result.Message = "Failed to check process states"
		return result
	}

	var dStateProcesses []map[string]string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "D") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				dStateProcesses = append(dStateProcesses, map[string]string{
					"pid":  fields[1],
					"name": fields[2],
				})
			}
		}
	}

	if len(dStateProcesses) > 0 {
		result.Status = "WARN"
		result.Message = "Found processes in D state"
		result.Details["count"] = len(dStateProcesses)
		result.Details["processes"] = dStateProcesses
	}

	return result
}

// checkFileDescriptors 检查文件描述符
func checkFileDescriptors() DiagnosticCheck {
	result := DiagnosticCheck{
		Name:      "file_descriptors",
		Status:    "OK",
		Message:   "File descriptor usage is within limits",
		Details:   make(map[string]interface{}),
		Timestamp: time.Now(),
	}

	// 获取系统级文件描述符限制
	maxFd, err := exec.Command("sysctl", "-n", "fs.file-max").Output()
	if err == nil {
		value, _ := strconv.Atoi(strings.TrimSpace(string(maxFd)))
		result.Details["system_max"] = value
	}

	// 获取当前使用的文件描述符数量
	usedFd, err := exec.Command("sh", "-c", "ls /proc/*/fd 2>/dev/null | wc -l").Output()
	if err == nil {
		value, _ := strconv.Atoi(strings.TrimSpace(string(usedFd)))
		result.Details["used"] = value

		// 计算使用率
		if max, ok := result.Details["system_max"].(int); ok && max > 0 {
			usage := float64(value) / float64(max) * 100
			result.Details["usage_percent"] = usage
			if usage > 80 {
				result.Status = "WARN"
				result.Message = "File descriptor usage is high"
			}
		}
	}

	return result
}
