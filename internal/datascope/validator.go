package datascope

import (
	"context"
	"sync"
)

// Scope defines the data collection scope
type Scope string

const (
	ScopeLocalOnly         Scope = "local_only"
	ScopeClusterSupplement Scope = "cluster_supplement"
	ScopeAuto              Scope = "auto"
)

// ValidationResult represents the result of a validation check
type ValidationResult struct {
	Valid   bool
	Skipped bool
	Reason  string
	Details map[string]interface{}
}

// Validator defines the interface for data scope validation
type Validator interface {
	// ValidateCollector validates if a collector should run based on scope
	ValidateCollector(ctx context.Context, collectorName string, scope Scope) (*ValidationResult, error)

	// ValidateDataType validates if a data type should be collected
	ValidateDataType(ctx context.Context, dataType string, scope Scope) (*ValidationResult, error)

	// GetLocalOnlyTypes returns all local-only data types
	GetLocalOnlyTypes() []string

	// GetClusterTypes returns all cluster supplement data types
	GetClusterTypes() []string

	// IsLocalOnly checks if a data type is local-only
	IsLocalOnly(dataType string) bool

	// IsClusterSupplement checks if a data type is cluster supplement
	IsClusterSupplement(dataType string) bool
}

// validator implements the Validator interface
type validator struct {
	localOnlyTypes    map[string]bool
	clusterTypes      map[string]bool
	clusterSupplement map[string]bool
	cache             map[string]*ValidationResult
	cacheMutex        sync.RWMutex
}

// NewValidator creates a new validator instance
func NewValidator() Validator {
	v := &validator{
		localOnlyTypes:    make(map[string]bool),
		clusterTypes:      make(map[string]bool),
		clusterSupplement: make(map[string]bool),
		cache:             make(map[string]*ValidationResult),
	}

	// Initialize data type mappings
	v.initializeDataTypes()
	return v
}

// initializeDataTypes sets up the predefined data type mappings
func (v *validator) initializeDataTypes() {
	// Node-local only data types
	localOnlyTypes := []string{
		"cpu_throttling",
		"cpu_temperature",
		"cpu_frequency",
		"memory_fragmentation",
		"swap_usage",
		"oom_events",
		"disk_smart",
		"disk_errors",
		"disk_performance",
		"network_interface_errors",
		"tcp_retransmission",
		"network_buffer",
		"kubelet_process",
		"kubelet_logs",
		"kubelet_config",
		"kubelet_certificates",
		"containerd_process",
		"containerd_memory",
		"containerd_logs",
		"cni_process",
		"cni_config",
		"kube_proxy_process",
		"kube_proxy_logs",
		"system_load",
		"file_descriptors",
		"kernel_logs",
		"ntp_sync",
	}

	// Cluster supplement data types
	clusterSupplementTypes := []string{
		"process_resource_usage",
		"kernel_events",
		"config_file_status",
		"certificate_validity",
	}

	// Populate maps
	for _, t := range localOnlyTypes {
		v.localOnlyTypes[t] = true
	}

	for _, t := range clusterSupplementTypes {
		v.clusterSupplement[t] = true
	}

	// Mark known cluster types (these should be skipped)
	clusterTypes := []string{
		"node_cpu_usage",
		"node_memory_usage",
		"node_disk_usage",
		"node_network_usage",
		"pod_status",
		"container_status",
	}

	for _, t := range clusterTypes {
		v.clusterTypes[t] = true
	}
}

// ValidateCollector validates if a collector should run based on scope
func (v *validator) ValidateCollector(ctx context.Context, collectorName string, scope Scope) (*ValidationResult, error) {
	cacheKey := collectorName + ":" + string(scope)

	// Check cache first
	if cached, exists := v.getCachedResult(cacheKey); exists {
		return cached, nil
	}

	result := &ValidationResult{
		Valid:   true,
		Skipped: false,
		Reason:  "",
		Details: make(map[string]interface{}),
	}

	switch scope {
	case ScopeLocalOnly:
		if !v.IsLocalOnly(collectorName) {
			result.Valid = false
			result.Skipped = true
			result.Reason = "collector is not node-local only"
		}
	case ScopeClusterSupplement:
		if !v.IsClusterSupplement(collectorName) {
			result.Valid = false
			result.Skipped = true
			result.Reason = "collector is not cluster supplement"
		}
	case ScopeAuto:
		if v.IsClusterSupplement(collectorName) {
			result.Details["type"] = "cluster_supplement"
		} else if v.IsLocalOnly(collectorName) {
			result.Details["type"] = "local_only"
		} else {
			result.Skipped = true
			result.Reason = "cluster-level data, use cluster monitoring"
		}
	}

	// Cache the result
	v.cacheResult(cacheKey, result)
	return result, nil
}

// ValidateDataType validates if a data type should be collected
func (v *validator) ValidateDataType(ctx context.Context, dataType string, scope Scope) (*ValidationResult, error) {
	return v.ValidateCollector(ctx, dataType, scope)
}

// GetLocalOnlyTypes returns all local-only data types
func (v *validator) GetLocalOnlyTypes() []string {
	types := make([]string, 0, len(v.localOnlyTypes))
	for t := range v.localOnlyTypes {
		types = append(types, t)
	}
	return types
}

// GetClusterTypes returns all cluster supplement data types
func (v *validator) GetClusterTypes() []string {
	types := make([]string, 0, len(v.clusterSupplement))
	for t := range v.clusterSupplement {
		types = append(types, t)
	}
	return types
}

// IsLocalOnly checks if a data type is local-only
func (v *validator) IsLocalOnly(dataType string) bool {
	return v.localOnlyTypes[dataType]
}

// IsClusterSupplement checks if a data type is cluster supplement
func (v *validator) IsClusterSupplement(dataType string) bool {
	return v.clusterSupplement[dataType]
}

// getCachedResult retrieves a cached validation result
func (v *validator) getCachedResult(key string) (*ValidationResult, bool) {
	v.cacheMutex.RLock()
	defer v.cacheMutex.RUnlock()

	result, exists := v.cache[key]
	return result, exists
}

// cacheResult stores a validation result in cache
func (v *validator) cacheResult(key string, result *ValidationResult) {
	v.cacheMutex.Lock()
	defer v.cacheMutex.Unlock()

	// Simple cache with no eviction for now
	v.cache[key] = result
}
