package collector

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// K8sComponentCollector 实现K8s组件状态采集
type K8sComponentCollector struct {
	name                string
	isLocalOnly         bool
	isClusterSupplement bool
	priority            int
	timeout             time.Duration
}

// NewKubeletCollector 创建kubelet状态采集器
func NewKubeletCollector() Collector {
	return &K8sComponentCollector{
		name:                "kubelet",
		isLocalOnly:         true,
		isClusterSupplement: false,
		priority:            8,
		timeout:             10 * time.Second,
	}
}

// NewContainerdCollector 创建containerd状态采集器
func NewContainerdCollector() Collector {
	return &K8sComponentCollector{
		name:                "containerd",
		isLocalOnly:         true,
		isClusterSupplement: false,
		priority:            8,
		timeout:             10 * time.Second,
	}
}

// NewCNICollector 创建CNI插件状态采集器
func NewCNICollector() Collector {
	return &K8sComponentCollector{
		name:                "cni",
		isLocalOnly:         true,
		isClusterSupplement: false,
		priority:            7,
		timeout:             8 * time.Second,
	}
}

// NewKubeProxyCollector 创建kube-proxy状态采集器
func NewKubeProxyCollector() Collector {
	return &K8sComponentCollector{
		name:                "kube-proxy",
		isLocalOnly:         true,
		isClusterSupplement: false,
		priority:            7,
		timeout:             8 * time.Second,
	}
}

// Name 返回采集器名称
func (k *K8sComponentCollector) Name() string {
	return k.name
}

// IsLocalOnly 是否为节点本地数据
func (k *K8sComponentCollector) IsLocalOnly() bool {
	return k.isLocalOnly
}

// IsClusterSupplement 是否为集群补充数据
func (k *K8sComponentCollector) IsClusterSupplement() bool {
	return k.isClusterSupplement
}

// Priority 返回优先级
func (k *K8sComponentCollector) Priority() int {
	return k.priority
}

// Timeout 返回超时时间
func (k *K8sComponentCollector) Timeout() time.Duration {
	return k.timeout
}

// Validate 验证参数
func (k *K8sComponentCollector) Validate(params map[string]interface{}) error {
	// K8s组件采集器不需要额外参数
	return nil
}

// Collect 执行数据收集
func (k *K8sComponentCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	data := &Data{
		Type:      k.name,
		Timestamp: time.Now(),
		Source:    "k8s-component",
		Data:      make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}

	switch k.name {
	case "kubelet":
		return k.collectKubelet(ctx, data)
	case "containerd":
		return k.collectContainerd(ctx, data)
	case "cni":
		return k.collectCNI(ctx, data)
	case "kube-proxy":
		return k.collectKubeProxy(ctx, data)
	default:
		return &Result{
			Error: fmt.Errorf("unknown k8s component: %s", k.name),
		}, nil
	}
}

// collectKubelet 收集kubelet状态
func (k *K8sComponentCollector) collectKubelet(ctx context.Context, data *Data) (*Result, error) {
	// 1. 检查进程状态
	processInfo, err := k.checkProcess("kubelet")
	if err != nil {
		data.Data["process_error"] = err.Error()
	} else {
		data.Data["process"] = processInfo
	}

	// 2. 检查证书状态
	certInfo, err := k.checkKubeletCerts()
	if err != nil {
		data.Data["cert_error"] = err.Error()
	} else {
		data.Data["certificates"] = certInfo
	}

	// 3. 异步收集日志（避免阻塞）
	logs, err := k.collectLogsAsync(ctx, "kubelet")
	if err != nil {
		data.Data["logs_error"] = err.Error()
	} else {
		data.Data["logs"] = logs
	}

	// 4. 检查配置文件
	configInfo, err := k.checkKubeletConfig()
	if err != nil {
		data.Data["config_error"] = err.Error()
	} else {
		data.Data["config"] = configInfo
	}

	return &Result{Data: data}, nil
}

// collectContainerd 收集containerd状态
func (k *K8sComponentCollector) collectContainerd(ctx context.Context, data *Data) (*Result, error) {
	// 1. 检查进程状态
	processInfo, err := k.checkProcess("containerd")
	if err != nil {
		data.Data["process_error"] = err.Error()
	} else {
		data.Data["process"] = processInfo
	}

	// 2. 检查内存使用
	memInfo, err := k.checkContainerdMemory()
	if err != nil {
		data.Data["memory_error"] = err.Error()
	} else {
		data.Data["memory"] = memInfo
	}

	// 3. 异步收集日志
	logs, err := k.collectLogsAsync(ctx, "containerd")
	if err != nil {
		data.Data["logs_error"] = err.Error()
	} else {
		data.Data["logs"] = logs
	}

	// 4. 检查配置文件
	configInfo, err := k.checkContainerdConfig()
	if err != nil {
		data.Data["config_error"] = err.Error()
	} else {
		data.Data["config"] = configInfo
	}

	return &Result{Data: data}, nil
}

// collectCNI 收集CNI插件状态
func (k *K8sComponentCollector) collectCNI(ctx context.Context, data *Data) (*Result, error) {
	// 1. 检查CNI配置文件
	cniConfig, err := k.checkCNIConfig()
	if err != nil {
		data.Data["config_error"] = err.Error()
	} else {
		data.Data["config"] = cniConfig
	}

	// 2. 检查CNI二进制文件
	binaries, err := k.checkCNIBinaries()
	if err != nil {
		data.Data["binaries_error"] = err.Error()
	} else {
		data.Data["binaries"] = binaries
	}

	// 3. 检查网络接口
	networkInfo, err := k.checkCNINetwork()
	if err != nil {
		data.Data["network_error"] = err.Error()
	} else {
		data.Data["network"] = networkInfo
	}

	return &Result{Data: data}, nil
}

// collectKubeProxy 收集kube-proxy状态
func (k *K8sComponentCollector) collectKubeProxy(ctx context.Context, data *Data) (*Result, error) {
	// 1. 检查进程状态
	processInfo, err := k.checkProcess("kube-proxy")
	if err != nil {
		data.Data["process_error"] = err.Error()
	} else {
		data.Data["process"] = processInfo
	}

	// 2. 检查iptables规则
	iptablesInfo, err := k.checkKubeProxyIPTables()
	if err != nil {
		data.Data["iptables_error"] = err.Error()
	} else {
		data.Data["iptables"] = iptablesInfo
	}

	// 3. 异步收集日志
	logs, err := k.collectLogsAsync(ctx, "kube-proxy")
	if err != nil {
		data.Data["logs_error"] = err.Error()
	} else {
		data.Data["logs"] = logs
	}

	return &Result{Data: data}, nil
}

// checkProcess 检查进程状态
func (k *K8sComponentCollector) checkProcess(name string) (map[string]interface{}, error) {
	cmd := exec.Command("pgrep", "-f", name)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("process not found: %s", name)
	}

	pid := strings.TrimSpace(string(output))

	// 获取进程详细信息
	statusCmd := exec.Command("ps", "-p", pid, "-o", "pid,ppid,cmd,pcpu,pmem,etime", "--no-headers")
	statusOutput, err := statusCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get process status: %v", err)
	}

	fields := strings.Fields(string(statusOutput))
	if len(fields) < 6 {
		return nil, fmt.Errorf("invalid process status format")
	}

	return map[string]interface{}{
		"pid":     fields[0],
		"ppid":    fields[1],
		"command": strings.Join(fields[2:len(fields)-3], " "),
		"cpu":     fields[len(fields)-3],
		"memory":  fields[len(fields)-2],
		"uptime":  fields[len(fields)-1],
	}, nil
}

// checkKubeletCerts 检查kubelet证书状态
func (k *K8sComponentCollector) checkKubeletCerts() (map[string]interface{}, error) {
	certDir := "/var/lib/kubelet/pki"
	certs := make(map[string]interface{})

	// 检查客户端证书
	clientCert := filepath.Join(certDir, "kubelet-client-current.pem")
	if info, err := os.Stat(clientCert); err == nil {
		certs["client_cert"] = map[string]interface{}{
			"path":    clientCert,
			"modtime": info.ModTime(),
			"size":    info.Size(),
		}
	}

	// 检查服务端证书
	serverCert := filepath.Join(certDir, "kubelet-server-current.pem")
	if info, err := os.Stat(serverCert); err == nil {
		certs["server_cert"] = map[string]interface{}{
			"path":    serverCert,
			"modtime": info.ModTime(),
			"size":    info.Size(),
		}
	}

	// 检查证书有效期
	if output, err := exec.Command("openssl", "x509", "-in", clientCert, "-noout", "-dates").CombinedOutput(); err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "notAfter=") {
				certs["client_cert_expiry"] = strings.TrimPrefix(line, "notAfter=")
			}
		}
	}

	return certs, nil
}

// checkKubeletConfig 检查kubelet配置
func (k *K8sComponentCollector) checkKubeletConfig() (map[string]interface{}, error) {
	configPath := "/var/lib/kubelet/config.yaml"

	info, err := os.Stat(configPath)
	if err != nil {
		return nil, fmt.Errorf("config file not found: %s", configPath)
	}

	return map[string]interface{}{
		"path":    configPath,
		"modtime": info.ModTime(),
		"size":    info.Size(),
	}, nil
}

// checkContainerdMemory 检查containerd内存使用
func (k *K8sComponentCollector) checkContainerdMemory() (map[string]interface{}, error) {
	cmd := exec.Command("pgrep", "-f", "containerd")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("containerd process not found")
	}

	pid := strings.TrimSpace(string(output))

	// 读取内存信息
	memInfo := make(map[string]interface{})

	// 从/proc/[pid]/status获取内存信息
	statusPath := fmt.Sprintf("/proc/%s/status", pid)
	if content, err := os.ReadFile(statusPath); err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					memInfo["rss_kb"], _ = strconv.Atoi(fields[1])
				}
			} else if strings.HasPrefix(line, "VmSize:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					memInfo["vsize_kb"], _ = strconv.Atoi(fields[1])
				}
			}
		}
	}

	return memInfo, nil
}

// checkContainerdConfig 检查containerd配置
func (k *K8sComponentCollector) checkContainerdConfig() (map[string]interface{}, error) {
	configPaths := []string{
		"/etc/containerd/config.toml",
		"/var/lib/containerd/config.toml",
	}

	for _, configPath := range configPaths {
		if info, err := os.Stat(configPath); err == nil {
			return map[string]interface{}{
				"path":    configPath,
				"modtime": info.ModTime(),
				"size":    info.Size(),
			}, nil
		}
	}

	return nil, fmt.Errorf("containerd config file not found")
}

// checkCNIConfig 检查CNI配置
func (k *K8sComponentCollector) checkCNIConfig() (map[string]interface{}, error) {
	cniDir := "/etc/cni/net.d"

	files, err := os.ReadDir(cniDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read CNI config directory: %v", err)
	}

	configs := make([]map[string]interface{}, 0)
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".conflist") || strings.HasSuffix(file.Name(), ".conf") {
			info, _ := file.Info()
			configs = append(configs, map[string]interface{}{
				"name":    file.Name(),
				"modtime": info.ModTime(),
				"size":    info.Size(),
			})
		}
	}

	return map[string]interface{}{
		"config_dir": cniDir,
		"configs":    configs,
	}, nil
}

// checkCNIBinaries 检查CNI二进制文件
func (k *K8sComponentCollector) checkCNIBinaries() (map[string]interface{}, error) {
	cniBinDir := "/opt/cni/bin"

	files, err := os.ReadDir(cniBinDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read CNI bin directory: %v", err)
	}

	binaries := make([]string, 0)
	for _, file := range files {
		if !file.IsDir() {
			binaries = append(binaries, file.Name())
		}
	}

	return map[string]interface{}{
		"bin_dir":  cniBinDir,
		"binaries": binaries,
	}, nil
}

// checkCNINetwork 检查CNI网络状态
func (k *K8sComponentCollector) checkCNINetwork() (map[string]interface{}, error) {
	// 检查CNI网络接口
	cmd := exec.Command("ip", "link", "show")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %v", err)
	}

	interfaces := make([]string, 0)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "cni") || strings.Contains(line, "flannel") || strings.Contains(line, "calico") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				interfaces = append(interfaces, strings.Trim(fields[1], ":"))
			}
		}
	}

	return map[string]interface{}{
		"cni_interfaces": interfaces,
	}, nil
}

// checkKubeProxyIPTables 检查kube-proxy iptables规则
func (k *K8sComponentCollector) checkKubeProxyIPTables() (map[string]interface{}, error) {
	rules := make(map[string]interface{})

	// 检查KUBE-SERVICES链
	cmd := exec.Command("iptables", "-t", "nat", "-L", "KUBE-SERVICES")
	output, err := cmd.CombinedOutput()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		rules["kube_services"] = len(lines) - 3
	}

	// 检查KUBE-NODEPORTS链
	cmd = exec.Command("iptables", "-t", "nat", "-L", "KUBE-NODEPORTS")
	output, err = cmd.CombinedOutput()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		rules["kube_nodeports"] = len(lines) - 3
	}

	return rules, nil
}

// collectLogsAsync 异步收集日志，避免阻塞和资源消耗
func (k *K8sComponentCollector) collectLogsAsync(ctx context.Context, service string) (map[string]interface{}, error) {
	logs := make(map[string]interface{})

	// 使用journalctl异步收集日志，限制时间和大小
	cmd := exec.CommandContext(ctx, "journalctl",
		"-u", service,
		"--since", "-5m", // 只收集最近5分钟
		"--no-pager",
		"--output", "short")

	// 使用管道读取输出，避免内存占用过大
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start journalctl: %v", err)
	}

	// 使用缓冲读取器，限制内存使用
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024), 64*1024) // 限制每行大小

	var logLines []string
	lineCount := 0
	maxLines := 100 // 限制日志行数

	for scanner.Scan() && lineCount < maxLines {
		line := scanner.Text()
		if line != "" {
			logLines = append(logLines, line)
			lineCount++
		}
	}

	// 等待命令完成或超时
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// 超时或取消，强制终止进程
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return nil, ctx.Err()
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("journalctl failed: %v", err)
		}
	}

	// 分析日志内容
	logs["recent_lines"] = len(logLines)
	logs["sample_lines"] = logLines

	// 检查错误模式
	errorCount := 0
	warningCount := 0
	for _, line := range logLines {
		if strings.Contains(strings.ToLower(line), "error") {
			errorCount++
		} else if strings.Contains(strings.ToLower(line), "warn") {
			warningCount++
		}
	}

	logs["error_count"] = errorCount
	logs["warning_count"] = warningCount

	return logs, nil
}
