package datascope

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2"
)

// DataScopeChecker 数据范围检查器，根据需求17实现完全不依赖apiserver的数据范围检查
type DataScopeChecker struct {
	nodeLocalTypes  map[string]bool // 节点本地独有数据类型
	supplementTypes map[string]bool // 集群补充数据类型
	skipTypes       map[string]bool // 跳过采集的数据类型
	cache           *lru.Cache[string, *ScopeCheckResult]
	lastUpdate      time.Time
	updateMutex     sync.RWMutex
}

// ScopeCheckResult 范围检查结果
type ScopeCheckResult struct {
	ShouldCollect bool   // 是否需要在本地采集
	Reason        string // 采集或跳过的详细原因
	Source        string // 数据来源：local/supplement/skipped
	Mode          string // 当前使用的模式
	DataType      string // 数据类型分类
}

// NewDataScopeChecker 创建新的数据范围检查器
func NewDataScopeChecker() (*DataScopeChecker, error) {
	cache, err := lru.New[string, *ScopeCheckResult](1000)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	checker := &DataScopeChecker{
		cache: cache,
	}

	// 初始化数据类型映射
	checker.initializeDataTypes()
	return checker, nil
}

// initializeDataTypes 初始化数据类型映射
func (d *DataScopeChecker) initializeDataTypes() {
	d.nodeLocalTypes = map[string]bool{
		// CPU相关 - 仅节点本地可见
		"cpu_throttling":          true,
		"cpu_temperature":         true,
		"cpu_frequency":           true,
		"cpu_cache_misses":        true,
		"cpu_context_switches":    true,
		"cpu_throttle_events":     true,
		"cpu_temperature_celsius": true,
		"cpu_frequency_mhz":       true,

		// 内存相关 - 仅节点本地可见
		"memory_fragmentation":       true,
		"swap_usage":                 true,
		"oom_events":                 true,
		"memory_page_faults":         true,
		"memory_compaction":          true,
		"memory_fragmentation_ratio": true,
		"swap_usage_percent":         true,
		"oom_killer_events":          true,

		// 磁盘相关 - 仅节点本地可见
		"disk_smart":               true,
		"disk_errors":              true,
		"disk_performance":         true,
		"disk_temperature":         true,
		"disk_reallocated_sectors": true,
		"disk_io_latency_ms":       true,
		"disk_smart_status":        true,
		"disk_error_count":         true,

		// 网络相关 - 仅节点本地可见
		"network_interface_errors": true,
		"tcp_retransmit_rate":      true,
		"network_buffer_usage":     true,
		"network_dropped_packets":  true,
		"network_collisions":       true,

		// K8s组件本地状态 - 仅节点本地可见
		"kubelet_process":          true,
		"kubelet_logs":             true,
		"kubelet_cert":             true,
		"containerd_status":        true,
		"containerd_memory":        true,
		"containerd_logs":          true,
		"cni_plugin_status":        true,
		"kube_proxy_iptables":      true,
		"kubelet_process_status":   true,
		"kubelet_cert_expiry_days": true,
		"containerd_memory_mb":     true,

		// 系统级 - 仅节点本地可见
		"system_load":            true,
		"file_descriptors":       true,
		"kernel_logs":            true,
		"ntp_sync":               true,
		"process_restarts":       true,
		"system_load_average":    true,
		"file_descriptors_usage": true,
		"kernel_log_errors":      true,
		"ntp_sync_status":        true,

		// 安全相关 - 仅节点本地可见
		"cert_validity":    true,
		"file_permissions": true,
		"audit_logs":       true,
		"suid_files":       true,
	}

	d.supplementTypes = map[string]bool{
		// 进程级资源使用（集群监控无法提供）
		"process_cpu_usage":        true,
		"process_memory_usage":     true,
		"process_file_descriptors": true,

		// 内核日志和事件
		"kernel_dmesg":      true,
		"kernel_oom_events": true,
		"kernel_panic_logs": true,

		// 本地配置文件状态
		"kubelet_config_status":    true,
		"containerd_config_status": true,
		"cni_config_status":        true,

		// 证书有效期（本地检查）
		"kubelet_cert_validity":    true,
		"containerd_cert_validity": true,
		"etcd_cert_validity":       true,
	}

	d.skipTypes = map[string]bool{
		// 集群级数据类型（apiserver已覆盖）
		"node_cpu_usage":     true,
		"node_memory_usage":  true,
		"node_disk_io":       true,
		"node_network_bytes": true,
		"pod_cpu_usage":      true,
		"pod_memory_usage":   true,
		"container_cpu":      true,
		"container_memory":   true,
	}
}

// ShouldCollectLocally 根据三种模式判断是否需要本地采集
func (d *DataScopeChecker) ShouldCollectLocally(checkName string, collectors []string, mode string) (*ScopeCheckResult, error) {
	// 根据需求17的三种模式进行精确控制
	switch mode {
	case "仅本地":
		return d.handleLocalOnlyMode(checkName, collectors)
	case "集群补充":
		return d.handleClusterSupplementMode(checkName, collectors)
	case "自动选择":
		return d.handleAutoSelectMode(checkName, collectors)
	default:
		return nil, fmt.Errorf("invalid collection mode: %s, must be one of: 仅本地, 集群补充, 自动选择", mode)
	}
}

// handleLocalOnlyMode 处理"仅本地"模式
func (d *DataScopeChecker) handleLocalOnlyMode(checkName string, collectors []string) (*ScopeCheckResult, error) {
	// 严格验证所有采集器必须是节点本地独有
	invalidCollectors := []string{}

	for _, collector := range collectors {
		if !d.nodeLocalTypes[collector] {
			invalidCollectors = append(invalidCollectors, collector)
		}
	}

	if len(invalidCollectors) > 0 {
		return &ScopeCheckResult{
			ShouldCollect: false,
			Reason:        fmt.Sprintf("collectors %v are not node-local-only data, rejected in 仅本地 mode", invalidCollectors),
			Source:        "skipped",
			Mode:          "仅本地",
			DataType:      "cluster_level",
		}, nil
	}

	return &ScopeCheckResult{
		ShouldCollect: true,
		Reason:        "all collectors are node-local-only data, forced local collection",
		Source:        "local",
		Mode:          "仅本地",
		DataType:      "node_local",
	}, nil
}

// handleClusterSupplementMode 处理"集群补充"模式
func (d *DataScopeChecker) handleClusterSupplementMode(checkName string, collectors []string) (*ScopeCheckResult, error) {
	// 仅采集集群监控无法提供的细节数据
	supplementCollectors := []string{}

	for _, collector := range collectors {
		if d.supplementTypes[collector] {
			supplementCollectors = append(supplementCollectors, collector)
		}
	}

	if len(supplementCollectors) > 0 {
		return &ScopeCheckResult{
			ShouldCollect: true,
			Reason:        fmt.Sprintf("collecting %d cluster-supplement collectors: %v", len(supplementCollectors), supplementCollectors),
			Source:        "supplement",
			Mode:          "集群补充",
			DataType:      "cluster_supplement",
		}, nil
	}

	return &ScopeCheckResult{
		ShouldCollect: false,
		Reason:        "no cluster-supplement data to collect",
		Source:        "skipped",
		Mode:          "集群补充",
		DataType:      "cluster_level",
	}, nil
}

// handleAutoSelectMode 处理"自动选择"模式
func (d *DataScopeChecker) handleAutoSelectMode(checkName string, collectors []string) (*ScopeCheckResult, error) {
	// 检查缓存
	cacheKey := fmt.Sprintf("%s:%s", checkName, strings.Join(collectors, ","))
	if cached, ok := d.cache.Get(cacheKey); ok {
		return cached, nil
	}

	// 基于预定义的数据类型映射进行智能判断
	nodeLocalCollectors := []string{}
	supplementCollectors := []string{}
	clusterLevelCollectors := []string{}

	for _, collector := range collectors {
		switch {
		case d.nodeLocalTypes[collector]:
			nodeLocalCollectors = append(nodeLocalCollectors, collector)
		case d.supplementTypes[collector]:
			supplementCollectors = append(supplementCollectors, collector)
		case d.skipTypes[collector]:
			clusterLevelCollectors = append(clusterLevelCollectors, collector)
		default:
			// 未知类型，默认作为节点本地处理
			nodeLocalCollectors = append(nodeLocalCollectors, collector)
		}
	}

	// 优先采集节点本地独有数据
	if len(nodeLocalCollectors) > 0 {
		result := &ScopeCheckResult{
			ShouldCollect: true,
			Reason:        fmt.Sprintf("collecting %d node-local-only collectors: %v", len(nodeLocalCollectors), nodeLocalCollectors),
			Source:        "local",
			Mode:          "自动选择",
			DataType:      "node_local",
		}
		d.cache.Add(cacheKey, result)
		return result, nil
	}

	// 其次采集集群补充数据
	if len(supplementCollectors) > 0 {
		result := &ScopeCheckResult{
			ShouldCollect: true,
			Reason:        fmt.Sprintf("collecting %d cluster-supplement collectors: %v", len(supplementCollectors), supplementCollectors),
			Source:        "supplement",
			Mode:          "自动选择",
			DataType:      "cluster_supplement",
		}
		d.cache.Add(cacheKey, result)
		return result, nil
	}

	// 无可采集数据
	result := &ScopeCheckResult{
		ShouldCollect: false,
		Reason:        fmt.Sprintf("no node-local data to collect, skipped cluster-level collectors: %v", clusterLevelCollectors),
		Source:        "skipped",
		Mode:          "自动选择",
		DataType:      "cluster_level",
	}
	d.cache.Add(cacheKey, result)
	return result, nil
}

// GetNodeLocalTypes 获取节点本地独有数据类型列表
func (d *DataScopeChecker) GetNodeLocalTypes() []string {
	types := make([]string, 0, len(d.nodeLocalTypes))
	for t := range d.nodeLocalTypes {
		types = append(types, t)
	}
	return types
}

// GetSupplementTypes 获取集群补充数据类型列表
func (d *DataScopeChecker) GetSupplementTypes() []string {
	types := make([]string, 0, len(d.supplementTypes))
	for t := range d.supplementTypes {
		types = append(types, t)
	}
	return types
}

// GetSkipTypes 获取跳过采集的数据类型列表
func (d *DataScopeChecker) GetSkipTypes() []string {
	types := make([]string, 0, len(d.skipTypes))
	for t := range d.skipTypes {
		types = append(types, t)
	}
	return types
}

// IsNodeLocalType 判断是否为节点本地独有数据类型
func (d *DataScopeChecker) IsNodeLocalType(collector string) bool {
	return d.nodeLocalTypes[collector]
}

// IsSupplementType 判断是否为集群补充数据类型
func (d *DataScopeChecker) IsSupplementType(collector string) bool {
	return d.supplementTypes[collector]
}

// IsSkipType 判断是否为跳过采集的数据类型
func (d *DataScopeChecker) IsSkipType(collector string) bool {
	return d.skipTypes[collector]
}

// ClearCache 清除缓存
func (d *DataScopeChecker) ClearCache() {
	d.cache.Purge()
}
