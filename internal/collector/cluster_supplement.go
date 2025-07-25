package collector

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProcessResourceCollector collects process-level resource usage
type ProcessResourceCollector struct{}

func (c *ProcessResourceCollector) Name() string {
	return "process-resource"
}

func (c *ProcessResourceCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	// 读取/proc/[pid]/stat和/proc/[pid]/status获取进程资源使用
	processes := make([]map[string]interface{}, 0)

	procDir := "/proc"
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return &Result{Error: fmt.Errorf("failed to read /proc: %w", err)}, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid := entry.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue // 跳过非数字目录
		}

		processInfo, err := c.collectProcessInfo(pid)
		if err != nil {
			continue // 跳过无法读取的进程
		}

		processes = append(processes, processInfo)
	}

	data := &Data{
		Type:      "process-resource",
		Timestamp: time.Now(),
		Source:    "node",
		Data: map[string]interface{}{
			"processes": processes,
			"count":     len(processes),
		},
		Metadata: map[string]string{
			"collector": "process-resource",
		},
	}

	return &Result{Data: data}, nil
}

func (c *ProcessResourceCollector) collectProcessInfo(pid string) (map[string]interface{}, error) {
	info := make(map[string]interface{})
	info["pid"] = pid

	// 读取进程状态
	statusPath := filepath.Join("/proc", pid, "status")
	statusData, err := os.ReadFile(statusPath)
	if err != nil {
		return nil, err
	}

	// 解析状态文件
	scanner := bufio.NewScanner(strings.NewReader(string(statusData)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Name":
			info["name"] = value
		case "State":
			info["state"] = value
		case "VmRSS":
			// 内存使用 (KB)
			if rss, err := parseMemoryValue(value); err == nil {
				info["memory_rss_kb"] = rss
			}
		case "VmSize":
			// 虚拟内存 (KB)
			if vms, err := parseMemoryValue(value); err == nil {
				info["memory_vms_kb"] = vms
			}
		case "FDSize":
			// 文件描述符数量
			if fds, err := strconv.Atoi(value); err == nil {
				info["fd_count"] = fds
			}
		case "Threads":
			if threads, err := strconv.Atoi(value); err == nil {
				info["threads"] = threads
			}
		}
	}

	// 读取进程统计信息
	statPath := filepath.Join("/proc", pid, "stat")
	statData, err := os.ReadFile(statPath)
	if err == nil {
		fields := strings.Fields(string(statData))
		if len(fields) >= 17 {
			// CPU时间 (jiffies)
			if utime, err := strconv.ParseUint(fields[13], 10, 64); err == nil {
				info["cpu_user_time"] = utime
			}
			if stime, err := strconv.ParseUint(fields[14], 10, 64); err == nil {
				info["cpu_system_time"] = stime
			}
		}
	}

	// 读取进程命令行
	cmdlinePath := filepath.Join("/proc", pid, "cmdline")
	if cmdline, err := os.ReadFile(cmdlinePath); err == nil {
		info["cmdline"] = strings.ReplaceAll(string(cmdline), "\x00", " ")
	}

	return info, nil
}

func (c *ProcessResourceCollector) Validate(params map[string]interface{}) error {
	return nil // 无特殊参数验证
}

func (c *ProcessResourceCollector) IsLocalOnly() bool {
	return false
}

func (c *ProcessResourceCollector) IsClusterSupplement() bool {
	return true
}

func (c *ProcessResourceCollector) Priority() int {
	return 2 // 中等优先级
}

func (c *ProcessResourceCollector) Timeout() time.Duration {
	return 10 * time.Second
}

// KernelEventCollector collects kernel events and logs
type KernelEventCollector struct{}

func (c *KernelEventCollector) Name() string {
	return "kernel-events"
}

func (c *KernelEventCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	events := make([]map[string]interface{}, 0)

	// 读取内核日志
	logFile := "/var/log/kern.log"
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		logFile = "/var/log/messages"
	}

	file, err := os.Open(logFile)
	if err != nil {
		return &Result{Error: fmt.Errorf("failed to open kernel log: %w", err)}, nil
	}
	defer file.Close()

	// 读取最后100行
	lines := make([]string, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > 100 {
			lines = lines[1:]
		}
	}

	// 解析内核事件
	for _, line := range lines {
		if strings.Contains(line, "error") || strings.Contains(line, "warning") ||
			strings.Contains(line, "fail") || strings.Contains(line, "oops") {

			event := map[string]interface{}{
				"timestamp": extractTimestamp(line),
				"message":   line,
				"severity":  "warning",
			}
			events = append(events, event)
		}
	}

	data := &Data{
		Type:      "kernel-events",
		Timestamp: time.Now(),
		Source:    "node",
		Data: map[string]interface{}{
			"events": events,
			"count":  len(events),
		},
		Metadata: map[string]string{
			"collector": "kernel-events",
			"log_file":  logFile,
		},
	}

	return &Result{Data: data}, nil
}

func (c *KernelEventCollector) Validate(params map[string]interface{}) error {
	return nil
}

func (c *KernelEventCollector) IsLocalOnly() bool {
	return false
}

func (c *KernelEventCollector) IsClusterSupplement() bool {
	return true
}

func (c *KernelEventCollector) Priority() int {
	return 1 // 低优先级
}

func (c *KernelEventCollector) Timeout() time.Duration {
	return 5 * time.Second
}

// ConfigFileCollector collects configuration file status
type ConfigFileCollector struct{}

func (c *ConfigFileCollector) Name() string {
	return "config-files"
}

func (c *ConfigFileCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	configs := make([]map[string]interface{}, 0)

	// 定义要检查的配置文件路径
	configPaths := []string{
		"/etc/kubernetes/manifests",
		"/etc/kubernetes/pki",
		"/etc/containerd",
		"/etc/cni/net.d",
		"/var/lib/kubelet/config.yaml",
		"/var/lib/kubelet/kubeadm-flags.env",
	}

	for _, path := range configPaths {
		info, err := c.collectConfigInfo(path)
		if err != nil {
			continue
		}
		configs = append(configs, info...)
	}

	data := &Data{
		Type:      "config-files",
		Timestamp: time.Now(),
		Source:    "node",
		Data: map[string]interface{}{
			"configs": configs,
			"count":   len(configs),
		},
		Metadata: map[string]string{
			"collector": "config-files",
		},
	}

	return &Result{Data: data}, nil
}

func (c *ConfigFileCollector) collectConfigInfo(path string) ([]map[string]interface{}, error) {
	var configs []map[string]interface{}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		// 处理目录
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				fullPath := filepath.Join(path, entry.Name())
				configInfo, err := c.getFileInfo(fullPath)
				if err == nil {
					configs = append(configs, configInfo)
				}
			}
		}
	} else {
		// 处理单个文件
		configInfo, err := c.getFileInfo(path)
		if err == nil {
			configs = append(configs, configInfo)
		}
	}

	return configs, nil
}

func (c *ConfigFileCollector) getFileInfo(path string) (map[string]interface{}, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	fileInfo := map[string]interface{}{
		"path":     path,
		"exists":   true,
		"size":     info.Size(),
		"modified": info.ModTime(),
		"mode":     info.Mode().String(),
	}

	// 检查文件内容
	if info.Size() < 1024*1024 { // 小于1MB的文件
		content, err := os.ReadFile(path)
		if err == nil {
			fileInfo["content_hash"] = fmt.Sprintf("%x", sha256.Sum256(content))
			fileInfo["line_count"] = strings.Count(string(content), "\n")
		}
	}

	return fileInfo, nil
}

func (c *ConfigFileCollector) Validate(params map[string]interface{}) error {
	return nil
}

func (c *ConfigFileCollector) IsLocalOnly() bool {
	return false
}

func (c *ConfigFileCollector) IsClusterSupplement() bool {
	return true
}

func (c *ConfigFileCollector) Priority() int {
	return 3 // 高优先级
}

func (c *ConfigFileCollector) Timeout() time.Duration {
	return 15 * time.Second
}

// CertificateCollector collects certificate expiration information
type CertificateCollector struct{}

func (c *CertificateCollector) Name() string {
	return "certificates"
}

func (c *CertificateCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	certs := make([]map[string]interface{}, 0)

	// 定义证书路径
	certPaths := []string{
		"/etc/kubernetes/pki/apiserver.crt",
		"/etc/kubernetes/pki/apiserver-kubelet-client.crt",
		"/etc/kubernetes/pki/ca.crt",
		"/etc/kubernetes/pki/front-proxy-ca.crt",
		"/var/lib/kubelet/pki/kubelet-client-current.pem",
	}

	for _, path := range certPaths {
		certInfo, err := c.collectCertInfo(path)
		if err != nil {
			certInfo = map[string]interface{}{
				"path":  path,
				"error": err.Error(),
			}
		}
		certs = append(certs, certInfo)
	}

	data := &Data{
		Type:      "certificates",
		Timestamp: time.Now(),
		Source:    "node",
		Data: map[string]interface{}{
			"certificates": certs,
			"count":        len(certs),
		},
		Metadata: map[string]string{
			"collector": "certificates",
		},
	}

	return &Result{Data: data}, nil
}

func (c *CertificateCollector) collectCertInfo(path string) (map[string]interface{}, error) {
	// 读取证书文件
	certPEM, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate: %w", err)
	}

	// 解析证书
	var cert *x509.Certificate
	if strings.HasSuffix(path, ".pem") || strings.HasSuffix(path, ".crt") {
		// 尝试解析PEM格式
		block, _ := pem.Decode(certPEM)
		if block == nil {
			return nil, fmt.Errorf("failed to decode PEM")
		}
		cert, err = x509.ParseCertificate(block.Bytes)
	} else {
		// 尝试解析DER格式
		cert, err = x509.ParseCertificate(certPEM)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// 计算剩余时间
	now := time.Now()
	daysUntilExpiry := int(cert.NotAfter.Sub(now).Hours() / 24)

	certInfo := map[string]interface{}{
		"path":              path,
		"subject":           cert.Subject.String(),
		"issuer":            cert.Issuer.String(),
		"not_before":        cert.NotBefore,
		"not_after":         cert.NotAfter,
		"days_until_expiry": daysUntilExpiry,
		"is_expired":        now.After(cert.NotAfter),
		"is_valid":          now.After(cert.NotBefore) && now.Before(cert.NotAfter),
	}

	// 检查证书链
	if len(cert.IssuingCertificateURL) > 0 {
		certInfo["issuing_urls"] = cert.IssuingCertificateURL
	}

	return certInfo, nil
}

func (c *CertificateCollector) Validate(params map[string]interface{}) error {
	return nil
}

func (c *CertificateCollector) IsLocalOnly() bool {
	return false
}

func (c *CertificateCollector) IsClusterSupplement() bool {
	return true
}

func (c *CertificateCollector) Priority() int {
	return 3 // 高优先级
}

func (c *CertificateCollector) Timeout() time.Duration {
	return 10 * time.Second
}

// Helper functions
func parseMemoryValue(value string) (int64, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, " kB")
	return strconv.ParseInt(value, 10, 64)
}

// 注册集群补充采集器
func init() {
	// 这里不自动注册，由外部统一管理
}
