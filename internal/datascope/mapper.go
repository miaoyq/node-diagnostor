package datascope

import (
	"fmt"
	"strings"
	"sync"
)

// DataType represents the classification of a data type
type DataType string

const (
	NodeLocal         DataType = "node_local"
	ClusterLevel      DataType = "cluster_level"
	ClusterSupplement DataType = "cluster_supplement"
)

// DataTypeInfo contains information about a data type
type DataTypeInfo struct {
	Category    string   `json:"category"`
	Type        DataType `json:"type"`
	Description string   `json:"description"`
	Examples    []string `json:"examples"`
}

// Mapper manages data type mappings for node-local and cluster-level data
type Mapper struct {
	nodeLocalTypes    map[string]DataTypeInfo
	clusterLevelTypes map[string]DataTypeInfo
	supplementTypes   map[string]DataTypeInfo
	whiteList         map[string]bool
	blackList         map[string]bool
	cache             map[string]DataType
	cacheMutex        sync.RWMutex
}

// NewMapper creates a new data type mapper
func NewMapper() *Mapper {
	m := &Mapper{
		nodeLocalTypes:    make(map[string]DataTypeInfo),
		clusterLevelTypes: make(map[string]DataTypeInfo),
		supplementTypes:   make(map[string]DataTypeInfo),
		whiteList:         make(map[string]bool),
		blackList:         make(map[string]bool),
		cache:             make(map[string]DataType),
	}

	// Initialize data type mappings
	m.Initialize()
	return m
}

// Initialize loads the predefined data type mappings
func (m *Mapper) Initialize() {
	// Initialize node-local only data types
	m.initializeNodeLocalTypes()

	// Initialize cluster-level data types (to be skipped)
	m.initializeClusterLevelTypes()

	// Initialize cluster supplement data types
	m.initializeSupplementTypes()

	// Build white/black lists
	m.buildLists()
}

func (m *Mapper) initializeNodeLocalTypes() {
	// CPU related - node local only
	m.nodeLocalTypes["cpu_throttling"] = DataTypeInfo{
		Category:    "cpu",
		Type:        NodeLocal,
		Description: "CPU throttling events, temperature, and frequency changes visible only at node level",
		Examples:    []string{"cpu_throttle_events", "cpu_temperature_celsius", "cpu_frequency_mhz"},
	}

	m.nodeLocalTypes["cpu_temperature"] = DataTypeInfo{
		Category:    "cpu",
		Type:        NodeLocal,
		Description: "CPU temperature monitoring from node sensors",
		Examples:    []string{"cpu_temp_sensor_0", "cpu_temp_sensor_1"},
	}

	m.nodeLocalTypes["cpu_frequency"] = DataTypeInfo{
		Category:    "cpu",
		Type:        NodeLocal,
		Description: "CPU frequency scaling and governor status",
		Examples:    []string{"cpu_freq_current", "cpu_freq_max", "cpu_governor"},
	}

	m.nodeLocalTypes["cpu_cache_misses"] = DataTypeInfo{
		Category:    "cpu",
		Type:        NodeLocal,
		Description: "CPU cache miss statistics from perf events",
		Examples:    []string{"cache_misses_l1", "cache_misses_l2", "cache_misses_l3"},
	}

	m.nodeLocalTypes["cpu_context_switches"] = DataTypeInfo{
		Category:    "cpu",
		Type:        NodeLocal,
		Description: "CPU context switch statistics",
		Examples:    []string{"context_switches_voluntary", "context_switches_involuntary"},
	}

	// Memory related - node local only
	m.nodeLocalTypes["memory_fragmentation"] = DataTypeInfo{
		Category:    "memory",
		Type:        NodeLocal,
		Description: "Memory fragmentation metrics from node memory management",
		Examples:    []string{"memory_fragmentation_ratio", "memory_compaction_events"},
	}

	m.nodeLocalTypes["swap_usage"] = DataTypeInfo{
		Category:    "memory",
		Type:        NodeLocal,
		Description: "Swap usage statistics and swap in/out events",
		Examples:    []string{"swap_usage_percent", "swap_in_pages", "swap_out_pages"},
	}

	m.nodeLocalTypes["oom_events"] = DataTypeInfo{
		Category:    "memory",
		Type:        NodeLocal,
		Description: "Out of memory killer events and victim processes",
		Examples:    []string{"oom_killer_events", "oom_victim_processes"},
	}

	m.nodeLocalTypes["memory_page_faults"] = DataTypeInfo{
		Category:    "memory",
		Type:        NodeLocal,
		Description: "Memory page fault statistics",
		Examples:    []string{"page_faults_major", "page_faults_minor"},
	}

	m.nodeLocalTypes["memory_compaction"] = DataTypeInfo{
		Category:    "memory",
		Type:        NodeLocal,
		Description: "Memory compaction events and statistics",
		Examples:    []string{"memory_compaction_events", "memory_compaction_success"},
	}

	// Disk related - node local only
	m.nodeLocalTypes["disk_smart"] = DataTypeInfo{
		Category:    "disk",
		Type:        NodeLocal,
		Description: "Disk SMART status and health metrics",
		Examples:    []string{"disk_smart_status", "disk_smart_temperature", "disk_smart_errors"},
	}

	m.nodeLocalTypes["disk_errors"] = DataTypeInfo{
		Category:    "disk",
		Type:        NodeLocal,
		Description: "Disk error logs and I/O error statistics",
		Examples:    []string{"disk_io_errors", "disk_read_errors", "disk_write_errors"},
	}

	m.nodeLocalTypes["disk_performance"] = DataTypeInfo{
		Category:    "disk",
		Type:        NodeLocal,
		Description: "Disk performance metrics and latency distribution",
		Examples:    []string{"disk_io_latency_ms", "disk_throughput_mb", "disk_iops"},
	}

	m.nodeLocalTypes["disk_temperature"] = DataTypeInfo{
		Category:    "disk",
		Type:        NodeLocal,
		Description: "Disk temperature from SMART sensors",
		Examples:    []string{"disk_temp_celsius", "disk_temp_max"},
	}

	m.nodeLocalTypes["disk_reallocated_sectors"] = DataTypeInfo{
		Category:    "disk",
		Type:        NodeLocal,
		Description: "Disk reallocated sector count from SMART",
		Examples:    []string{"disk_reallocated_sectors", "disk_pending_sectors"},
	}

	// Network related - node local only
	m.nodeLocalTypes["network_interface_errors"] = DataTypeInfo{
		Category:    "network",
		Type:        NodeLocal,
		Description: "Network interface error statistics",
		Examples:    []string{"interface_rx_errors", "interface_tx_errors", "interface_dropped"},
	}

	m.nodeLocalTypes["tcp_retransmit_rate"] = DataTypeInfo{
		Category:    "network",
		Type:        NodeLocal,
		Description: "TCP retransmission rate and statistics",
		Examples:    []string{"tcp_retransmit_rate", "tcp_retransmit_segments"},
	}

	m.nodeLocalTypes["network_buffer_usage"] = DataTypeInfo{
		Category:    "network",
		Type:        NodeLocal,
		Description: "Network buffer usage and allocation statistics",
		Examples:    []string{"network_buffer_usage", "network_buffer_errors"},
	}

	m.nodeLocalTypes["network_dropped_packets"] = DataTypeInfo{
		Category:    "network",
		Type:        NodeLocal,
		Description: "Network dropped packet statistics",
		Examples:    []string{"network_rx_dropped", "network_tx_dropped"},
	}

	m.nodeLocalTypes["network_collisions"] = DataTypeInfo{
		Category:    "network",
		Type:        NodeLocal,
		Description: "Network collision statistics",
		Examples:    []string{"network_collisions", "network_collision_rate"},
	}

	// Kubernetes components - node local only
	m.nodeLocalTypes["kubelet_process"] = DataTypeInfo{
		Category:    "kubernetes",
		Type:        NodeLocal,
		Description: "Kubelet process status and health metrics",
		Examples:    []string{"kubelet_process_status", "kubelet_process_memory", "kubelet_process_cpu"},
	}

	m.nodeLocalTypes["kubelet_logs"] = DataTypeInfo{
		Category:    "kubernetes",
		Type:        NodeLocal,
		Description: "Kubelet log errors and warnings",
		Examples:    []string{"kubelet_error_logs", "kubelet_warning_logs"},
	}

	m.nodeLocalTypes["kubelet_cert"] = DataTypeInfo{
		Category:    "kubernetes",
		Type:        NodeLocal,
		Description: "Kubelet certificate validity and expiry",
		Examples:    []string{"kubelet_cert_expiry_days", "kubelet_cert_valid"},
	}

	m.nodeLocalTypes["containerd_status"] = DataTypeInfo{
		Category:    "kubernetes",
		Type:        NodeLocal,
		Description: "Containerd service status and health",
		Examples:    []string{"containerd_service_status", "containerd_socket_response"},
	}

	m.nodeLocalTypes["containerd_memory"] = DataTypeInfo{
		Category:    "kubernetes",
		Type:        NodeLocal,
		Description: "Containerd memory usage statistics",
		Examples:    []string{"containerd_memory_usage_mb", "containerd_memory_rss"},
	}

	m.nodeLocalTypes["containerd_logs"] = DataTypeInfo{
		Category:    "kubernetes",
		Type:        NodeLocal,
		Description: "Containerd log errors and warnings",
		Examples:    []string{"containerd_error_logs", "containerd_warning_logs"},
	}

	m.nodeLocalTypes["cni_plugin_status"] = DataTypeInfo{
		Category:    "kubernetes",
		Type:        NodeLocal,
		Description: "CNI plugin status and configuration",
		Examples:    []string{"cni_plugin_status", "cni_config_valid"},
	}

	m.nodeLocalTypes["kube_proxy_iptables"] = DataTypeInfo{
		Category:    "kubernetes",
		Type:        NodeLocal,
		Description: "Kube-proxy iptables rules and status",
		Examples:    []string{"kube_proxy_iptables_rules", "kube_proxy_iptables_errors"},
	}

	// System level - node local only
	m.nodeLocalTypes["system_load"] = DataTypeInfo{
		Category:    "system",
		Type:        NodeLocal,
		Description: "System load average and process statistics",
		Examples:    []string{"system_load_1min", "system_load_5min", "system_processes"},
	}

	m.nodeLocalTypes["file_descriptors"] = DataTypeInfo{
		Category:    "system",
		Type:        NodeLocal,
		Description: "File descriptor usage and limits",
		Examples:    []string{"file_descriptors_used", "file_descriptors_limit"},
	}

	m.nodeLocalTypes["kernel_logs"] = DataTypeInfo{
		Category:    "system",
		Type:        NodeLocal,
		Description: "Kernel log errors and warnings",
		Examples:    []string{"kernel_error_logs", "kernel_warning_logs"},
	}

	m.nodeLocalTypes["ntp_sync"] = DataTypeInfo{
		Category:    "system",
		Type:        NodeLocal,
		Description: "NTP synchronization status and drift",
		Examples:    []string{"ntp_sync_status", "ntp_time_drift_seconds"},
	}

	m.nodeLocalTypes["process_restarts"] = DataTypeInfo{
		Category:    "system",
		Type:        NodeLocal,
		Description: "Process restart events and counts",
		Examples:    []string{"process_restart_count", "process_restart_events"},
	}

	// Security related - node local only
	m.nodeLocalTypes["cert_validity"] = DataTypeInfo{
		Category:    "security",
		Type:        NodeLocal,
		Description: "Certificate validity and expiry checks",
		Examples:    []string{"cert_expiry_days", "cert_valid_status"},
	}

	m.nodeLocalTypes["file_permissions"] = DataTypeInfo{
		Category:    "security",
		Type:        NodeLocal,
		Description: "File permission checks for sensitive files",
		Examples:    []string{"file_permission_errors", "sensitive_file_perms"},
	}

	m.nodeLocalTypes["audit_logs"] = DataTypeInfo{
		Category:    "security",
		Type:        NodeLocal,
		Description: "Security audit log events",
		Examples:    []string{"audit_log_events", "security_violations"},
	}

	m.nodeLocalTypes["suid_files"] = DataTypeInfo{
		Category:    "security",
		Type:        NodeLocal,
		Description: "SUID/SGID file detection and checks",
		Examples:    []string{"suid_files_count", "suid_file_list"},
	}
}

func (m *Mapper) initializeClusterLevelTypes() {
	// Cluster-level data types that should be skipped (covered by apiserver)
	m.clusterLevelTypes["node_cpu_usage"] = DataTypeInfo{
		Category:    "cpu",
		Type:        ClusterLevel,
		Description: "Node CPU usage (already provided by apiserver)",
		Examples:    []string{"node_cpu_usage_total", "node_cpu_usage_percent"},
	}

	m.clusterLevelTypes["node_memory_usage"] = DataTypeInfo{
		Category:    "memory",
		Type:        ClusterLevel,
		Description: "Node memory usage (already provided by apiserver)",
		Examples:    []string{"node_memory_usage_bytes", "node_memory_usage_percent"},
	}

	m.clusterLevelTypes["node_disk_io"] = DataTypeInfo{
		Category:    "disk",
		Type:        ClusterLevel,
		Description: "Node disk I/O statistics (already provided by apiserver)",
		Examples:    []string{"node_disk_io_time_seconds_total", "node_disk_read_bytes_total"},
	}

	m.clusterLevelTypes["node_network_bytes"] = DataTypeInfo{
		Category:    "network",
		Type:        ClusterLevel,
		Description: "Node network traffic statistics (already provided by apiserver)",
		Examples:    []string{"node_network_receive_bytes_total", "node_network_transmit_bytes_total"},
	}

	m.clusterLevelTypes["pod_cpu_usage"] = DataTypeInfo{
		Category:    "cpu",
		Type:        ClusterLevel,
		Description: "Pod CPU usage (already provided by apiserver)",
		Examples:    []string{"pod_cpu_usage_total", "pod_cpu_usage_percent"},
	}

	m.clusterLevelTypes["pod_memory_usage"] = DataTypeInfo{
		Category:    "memory",
		Type:        ClusterLevel,
		Description: "Pod memory usage (already provided by apiserver)",
		Examples:    []string{"pod_memory_usage_bytes", "pod_memory_usage_percent"},
	}

	m.clusterLevelTypes["container_cpu"] = DataTypeInfo{
		Category:    "cpu",
		Type:        ClusterLevel,
		Description: "Container CPU usage (already provided by apiserver)",
		Examples:    []string{"container_cpu_usage_seconds_total", "container_cpu_usage_percent"},
	}

	m.clusterLevelTypes["container_memory"] = DataTypeInfo{
		Category:    "memory",
		Type:        ClusterLevel,
		Description: "Container memory usage (already provided by apiserver)",
		Examples:    []string{"container_memory_usage_bytes", "container_memory_working_set_bytes"},
	}
}

func (m *Mapper) initializeSupplementTypes() {
	// Cluster supplement data types (details not covered by apiserver)
	m.supplementTypes["process_cpu_usage"] = DataTypeInfo{
		Category:    "process",
		Type:        ClusterSupplement,
		Description: "Process-level CPU usage details not available via apiserver",
		Examples:    []string{"process_cpu_percent", "process_cpu_time"},
	}

	m.supplementTypes["process_memory_usage"] = DataTypeInfo{
		Category:    "process",
		Type:        ClusterSupplement,
		Description: "Process-level memory usage details not available via apiserver",
		Examples:    []string{"process_memory_rss", "process_memory_vms"},
	}

	m.supplementTypes["process_file_descriptors"] = DataTypeInfo{
		Category:    "process",
		Type:        ClusterSupplement,
		Description: "Process file descriptor usage details",
		Examples:    []string{"process_fd_count", "process_fd_limit"},
	}

	m.supplementTypes["kernel_dmesg"] = DataTypeInfo{
		Category:    "kernel",
		Type:        ClusterSupplement,
		Description: "Kernel dmesg logs and events",
		Examples:    []string{"kernel_dmesg_errors", "kernel_dmesg_warnings"},
	}

	m.supplementTypes["kernel_oom_events"] = DataTypeInfo{
		Category:    "kernel",
		Type:        ClusterSupplement,
		Description: "Kernel OOM killer events and details",
		Examples:    []string{"kernel_oom_events", "kernel_oom_victims"},
	}

	m.supplementTypes["kernel_panic_logs"] = DataTypeInfo{
		Category:    "kernel",
		Type:        ClusterSupplement,
		Description: "Kernel panic logs and stack traces",
		Examples:    []string{"kernel_panic_logs", "kernel_panic_traces"},
	}

	m.supplementTypes["kubelet_config_status"] = DataTypeInfo{
		Category:    "kubernetes",
		Type:        ClusterSupplement,
		Description: "Kubelet configuration file status and validation",
		Examples:    []string{"kubelet_config_valid", "kubelet_config_errors"},
	}

	m.supplementTypes["containerd_config_status"] = DataTypeInfo{
		Category:    "kubernetes",
		Type:        ClusterSupplement,
		Description: "Containerd configuration file status and validation",
		Examples:    []string{"containerd_config_valid", "containerd_config_errors"},
	}

	m.supplementTypes["cni_config_status"] = DataTypeInfo{
		Category:    "kubernetes",
		Type:        ClusterSupplement,
		Description: "CNI configuration file status and validation",
		Examples:    []string{"cni_config_valid", "cni_config_errors"},
	}

	m.supplementTypes["kubelet_cert_validity"] = DataTypeInfo{
		Category:    "security",
		Type:        ClusterSupplement,
		Description: "Kubelet certificate validity and expiry",
		Examples:    []string{"kubelet_cert_expiry_days", "kubelet_cert_valid"},
	}

	m.supplementTypes["containerd_cert_validity"] = DataTypeInfo{
		Category:    "security",
		Type:        ClusterSupplement,
		Description: "Containerd certificate validity and expiry",
		Examples:    []string{"containerd_cert_expiry_days", "containerd_cert_valid"},
	}

	m.supplementTypes["etcd_cert_validity"] = DataTypeInfo{
		Category:    "security",
		Type:        ClusterSupplement,
		Description: "Etcd certificate validity and expiry",
		Examples:    []string{"etcd_cert_expiry_days", "etcd_cert_valid"},
	}
}

func (m *Mapper) buildLists() {
	// Build white list for node-local types
	for key := range m.nodeLocalTypes {
		m.whiteList[key] = true
	}

	// Build black list for cluster-level types
	for key := range m.clusterLevelTypes {
		m.blackList[key] = true
	}
}

// GetDataType returns the classification for a given collector ID
func (m *Mapper) GetDataType(collectorID string) (DataType, error) {
	m.cacheMutex.RLock()
	if dataType, exists := m.cache[collectorID]; exists {
		m.cacheMutex.RUnlock()
		return dataType, nil
	}
	m.cacheMutex.RUnlock()

	// Determine data type
	var dataType DataType

	if _, exists := m.nodeLocalTypes[collectorID]; exists {
		dataType = NodeLocal
	} else if _, exists := m.clusterLevelTypes[collectorID]; exists {
		dataType = ClusterLevel
	} else if _, exists := m.supplementTypes[collectorID]; exists {
		dataType = ClusterSupplement
	} else {
		return "", fmt.Errorf("unknown collector ID: %s", collectorID)
	}

	// Cache the result
	m.cacheMutex.Lock()
	m.cache[collectorID] = dataType
	m.cacheMutex.Unlock()

	return dataType, nil
}

// IsNodeLocal checks if a collector is node-local only
func (m *Mapper) IsNodeLocal(collectorID string) bool {
	_, exists := m.nodeLocalTypes[collectorID]
	return exists
}

// IsClusterLevel checks if a collector is cluster-level (should be skipped)
func (m *Mapper) IsClusterLevel(collectorID string) bool {
	_, exists := m.clusterLevelTypes[collectorID]
	return exists
}

// IsClusterSupplement checks if a collector provides cluster supplement data
func (m *Mapper) IsClusterSupplement(collectorID string) bool {
	_, exists := m.supplementTypes[collectorID]
	return exists
}

// GetNodeLocalTypes returns all node-local data types
func (m *Mapper) GetNodeLocalTypes() []string {
	types := make([]string, 0, len(m.nodeLocalTypes))
	for key := range m.nodeLocalTypes {
		types = append(types, key)
	}
	return types
}

// GetClusterLevelTypes returns all cluster-level data types
func (m *Mapper) GetClusterLevelTypes() []string {
	types := make([]string, 0, len(m.clusterLevelTypes))
	for key := range m.clusterLevelTypes {
		types = append(types, key)
	}
	return types
}

// GetSupplementTypes returns all cluster supplement data types
func (m *Mapper) GetSupplementTypes() []string {
	types := make([]string, 0, len(m.supplementTypes))
	for key := range m.supplementTypes {
		types = append(types, key)
	}
	return types
}

// GetDataTypeInfo returns detailed information about a data type
func (m *Mapper) GetDataTypeInfo(collectorID string) (*DataTypeInfo, error) {
	if info, exists := m.nodeLocalTypes[collectorID]; exists {
		return &info, nil
	}

	if info, exists := m.clusterLevelTypes[collectorID]; exists {
		return &info, nil
	}

	if info, exists := m.supplementTypes[collectorID]; exists {
		return &info, nil
	}

	return nil, fmt.Errorf("unknown collector ID: %s", collectorID)
}

// ValidateCollectors validates a list of collectors against the data type mappings
func (m *Mapper) ValidateCollectors(collectors []string) error {
	var invalid []string

	for _, collector := range collectors {
		if !m.IsValid(collector) {
			invalid = append(invalid, collector)
		}
	}

	if len(invalid) > 0 {
		return fmt.Errorf("invalid collectors: %s", strings.Join(invalid, ", "))
	}

	return nil
}

// IsValid checks if a collector ID is valid
func (m *Mapper) IsValid(collectorID string) bool {
	return m.IsNodeLocal(collectorID) || m.IsClusterLevel(collectorID) || m.IsClusterSupplement(collectorID)
}

// ClearCache clears the internal cache
func (m *Mapper) ClearCache() {
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()

	for key := range m.cache {
		delete(m.cache, key)
	}
}

// GetCollectorList returns a comprehensive list of all valid collectors
func (m *Mapper) GetCollectorList() []string {
	allCollectors := make([]string, 0)

	// Add node-local collectors
	allCollectors = append(allCollectors, m.GetNodeLocalTypes()...)

	// Add cluster supplement collectors
	allCollectors = append(allCollectors, m.GetSupplementTypes()...)

	// Note: cluster-level collectors are not included as they should be skipped
	return allCollectors
}
