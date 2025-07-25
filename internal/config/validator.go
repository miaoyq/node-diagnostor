package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Validator 配置验证器
type Validator struct {
	validCollectors map[string]bool
	minInterval     time.Duration
	maxInterval     time.Duration
}

// NewValidator 创建新的配置验证器
func NewValidator() *Validator {
	return &Validator{
		validCollectors: map[string]bool{
			// CPU相关
			"cpu_throttling":       true,
			"cpu_temperature":      true,
			"cpu_frequency":        true,
			"cpu_cache_misses":     true,
			"cpu_context_switches": true,

			// 内存相关
			"memory_fragmentation": true,
			"swap_usage":           true,
			"oom_events":           true,
			"memory_page_faults":   true,
			"memory_compaction":    true,

			// 磁盘相关
			"disk_smart":               true,
			"disk_errors":              true,
			"disk_performance":         true,
			"disk_temperature":         true,
			"disk_reallocated_sectors": true,

			// 网络相关
			"network_interface_errors": true,
			"tcp_retransmit_rate":      true,
			"network_buffer_usage":     true,
			"network_dropped_packets":  true,
			"network_collisions":       true,

			// K8s组件
			"kubelet_process":     true,
			"kubelet_logs":        true,
			"kubelet_cert":        true,
			"containerd_status":   true,
			"containerd_memory":   true,
			"containerd_logs":     true,
			"cni_plugin_status":   true,
			"kube_proxy_iptables": true,

			// 系统级
			"system_load":      true,
			"file_descriptors": true,
			"kernel_logs":      true,
			"ntp_sync":         true,
			"process_restarts": true,

			// 安全相关
			"cert_validity":    true,
			"file_permissions": true,
			"audit_logs":       true,
			"suid_files":       true,

			// 集群补充
			"process_cpu_usage":        true,
			"process_memory_usage":     true,
			"process_file_descriptors": true,
			"kernel_dmesg":             true,
			"kernel_oom_events":        true,
			"kernel_panic_logs":        true,
			"kubelet_config_status":    true,
			"containerd_config_status": true,
			"cni_config_status":        true,
			"kubelet_cert_validity":    true,
			"containerd_cert_validity": true,
			"etcd_cert_validity":       true,
		},
		minInterval: 1 * time.Minute,
		maxInterval: 60 * time.Minute,
	}
}

// ValidateConfig 验证整个配置
func (v *Validator) ValidateConfig(config *Config) error {
	var errs []error

	// 验证版本
	if config.Version == "" {
		errs = append(errs, errors.New("配置版本不能为空"))
	}

	// 验证检查项
	if len(config.Checks) == 0 {
		errs = append(errs, errors.New("至少需要配置一个检查项"))
	}

	for i, check := range config.Checks {
		if err := v.validateCheck(i, check); err != nil {
			errs = append(errs, fmt.Errorf("检查项[%d]: %w", i+1, err))
		}
	}

	// 验证资源限制
	if err := v.validateResourceLimits(config.ResourceLimits); err != nil {
		errs = append(errs, fmt.Errorf("资源限制: %w", err))
	}

	// 验证数据范围配置
	if err := v.validateDataScope(config.DataScope); err != nil {
		errs = append(errs, fmt.Errorf("数据范围: %w", err))
	}

	// 验证上报配置
	if err := v.validateReporter(config.Reporter); err != nil {
		errs = append(errs, fmt.Errorf("上报配置: %w", err))
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}

// validateCheck 验证单个检查项
func (v *Validator) validateCheck(index int, check CheckConfig) error {
	var errs []error

	// 验证名称
	if check.Name == "" {
		errs = append(errs, errors.New("检查项名称不能为空"))
	}

	// 验证名称格式
	if strings.Contains(check.Name, " ") {
		errs = append(errs, errors.New("检查项名称不能包含空格"))
	}

	// 验证时间间隔
	if check.Interval < v.minInterval {
		errs = append(errs, fmt.Errorf("检查间隔不能小于%v", v.minInterval))
	}
	if check.Interval > v.maxInterval {
		errs = append(errs, fmt.Errorf("检查间隔不能大于%v", v.maxInterval))
	}

	// 验证采集器
	if len(check.Collectors) == 0 {
		errs = append(errs, errors.New("至少需要配置一个采集器"))
	}

	for _, collector := range check.Collectors {
		if !v.validCollectors[collector] {
			errs = append(errs, fmt.Errorf("无效的采集器ID: %s", collector))
		}
	}

	// 验证模式
	validModes := map[string]bool{
		"仅本地":  true,
		"集群补充": true,
		"自动选择": true,
	}
	if !validModes[check.Mode] {
		errs = append(errs, fmt.Errorf("无效的数据范围模式: %s", check.Mode))
	}

	// 验证超时时间
	if check.Timeout <= 0 {
		errs = append(errs, errors.New("超时时间必须大于0"))
	}
	if check.Timeout > check.Interval {
		errs = append(errs, errors.New("超时时间不能大于检查间隔"))
	}

	// 验证优先级
	if check.Priority < 1 || check.Priority > 10 {
		errs = append(errs, errors.New("优先级必须在1-10之间"))
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}

// validateResourceLimits 验证资源限制
func (v *Validator) validateResourceLimits(limits ResourceLimits) error {
	var errs []error

	if limits.MaxCPU <= 0 || limits.MaxCPU > 100 {
		errs = append(errs, errors.New("CPU使用率限制必须在0-100之间"))
	}

	if limits.MaxMemory <= 0 || limits.MaxMemory > 1000 {
		errs = append(errs, errors.New("内存限制必须在0-1000MB之间"))
	}

	if limits.MinInterval < v.minInterval {
		errs = append(errs, fmt.Errorf("最小检查间隔不能小于%v", v.minInterval))
	}

	if limits.MaxConcurrent <= 0 {
		errs = append(errs, errors.New("最大并发数必须大于0"))
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}

// validateDataScope 验证数据范围配置
func (v *Validator) validateDataScope(scope DataScopeConfig) error {
	var errs []error

	validModes := map[string]bool{
		"仅本地":  true,
		"集群补充": true,
		"自动选择": true,
	}
	if !validModes[scope.Mode] {
		errs = append(errs, fmt.Errorf("无效的数据范围模式: %s", scope.Mode))
	}

	// 验证数据类型
	for _, dataType := range scope.NodeLocalTypes {
		if !v.validCollectors[dataType] {
			errs = append(errs, fmt.Errorf("无效的节点本地数据类型: %s", dataType))
		}
	}

	for _, dataType := range scope.SupplementTypes {
		if !v.validCollectors[dataType] {
			errs = append(errs, fmt.Errorf("无效的集群补充数据类型: %s", dataType))
		}
	}

	for _, dataType := range scope.SkipTypes {
		if !v.validCollectors[dataType] {
			errs = append(errs, fmt.Errorf("无效的跳过数据类型: %s", dataType))
		}
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}

// validateReporter 验证上报配置
func (v *Validator) validateReporter(reporter ReporterConfig) error {
	var errs []error

	if reporter.Endpoint == "" {
		errs = append(errs, errors.New("上报端点不能为空"))
	}

	if reporter.Timeout <= 0 {
		errs = append(errs, errors.New("上报超时时间必须大于0"))
	}

	if reporter.MaxRetries < 0 {
		errs = append(errs, errors.New("最大重试次数不能为负数"))
	}

	if reporter.RetryDelay < 0 {
		errs = append(errs, errors.New("重试延迟不能为负数"))
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}

// ValidationError 验证错误集合
type ValidationError struct {
	Errors []error
}

func (e *ValidationError) Error() string {
	if len(e.Errors) == 0 {
		return "配置验证通过"
	}

	var sb strings.Builder
	sb.WriteString("配置验证失败:\n")
	for _, err := range e.Errors {
		sb.WriteString("  - ")
		sb.WriteString(err.Error())
		sb.WriteString("\n")
	}
	return sb.String()
}

// GetCollectorList 获取所有有效的采集器列表
func (v *Validator) GetCollectorList() []string {
	var collectors []string
	for collector := range v.validCollectors {
		collectors = append(collectors, collector)
	}
	return collectors
}

// IsValidCollector 检查采集器ID是否有效
func (v *Validator) IsValidCollector(collector string) bool {
	return v.validCollectors[collector]
}
