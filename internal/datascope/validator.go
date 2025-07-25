package datascope

import (
	"fmt"
	"strings"
	"sync"
)

// ValidationResult represents the result of validating a single collector
type ValidationResult struct {
	Collector  string
	IsValid    bool
	Category   string
	Reason     string
	Suggestion string
	Metadata   map[string]interface{}
}

// CheckValidationResult represents the result of validating a check configuration
type CheckValidationResult struct {
	CheckName  string
	Valid      bool
	Mode       string
	Collectors []ValidationResult
	Errors     []string
	Stats      ValidationStats
}

// ValidationStats contains statistics about validation results
type ValidationStats struct {
	TotalCount      int
	LocalCount      int
	SupplementCount int
	SkipCount       int
	UnknownCount    int
}

// ValidationReport contains comprehensive validation results
type ValidationReport struct {
	Mode         string
	TotalChecks  int
	Results      map[string]*CheckValidationResult
	OverallStats ValidationStats
}

// DataTypeValidator validates collector configurations based on data scope
type DataTypeValidator struct {
	checker *DataScopeChecker
	mapper  *Mapper
	cache   map[string]*ValidationResult
	mutex   sync.RWMutex
}

// NewDataTypeValidator creates a new data type validator
func NewDataTypeValidator(checker *DataScopeChecker) *DataTypeValidator {
	return &DataTypeValidator{
		checker: checker,
		mapper:  NewMapper(),
		cache:   make(map[string]*ValidationResult),
	}
}

// ValidateCollector validates if a collector should run based on its data type
func (v *DataTypeValidator) ValidateCollector(collector string) *ValidationResult {
	v.mutex.RLock()
	if cached, exists := v.cache[collector]; exists {
		v.mutex.RUnlock()
		return cached
	}
	v.mutex.RUnlock()

	result := &ValidationResult{
		Collector: collector,
		IsValid:   true,
		Metadata:  make(map[string]interface{}),
	}

	// Determine data type using mapper
	dataType, err := v.mapper.GetDataType(collector)
	if err != nil {
		result.IsValid = true
		result.Category = "unknown"
		result.Reason = fmt.Sprintf("采集器 '%s' 是未知类型，默认作为节点本地数据处理", collector)
		result.Suggestion = "建议确认该采集器的数据类型，或将其归类到合适的数据类型"
		result.Metadata["scope"] = "unknown"
		result.Metadata["priority"] = "low"
		v.cacheResult(collector, result)
		return result
	}

	switch dataType {
	case NodeLocal:
		result.IsValid = true
		result.Category = "node_local"
		result.Reason = fmt.Sprintf("采集器 '%s' 是节点本地独有数据类型，必须在本地采集", collector)
		result.Suggestion = "该采集器提供的数据无法通过集群监控获取，建议保留"
		result.Metadata["scope"] = "node_local"
		result.Metadata["priority"] = "high"
	case ClusterLevel:
		result.IsValid = false
		result.Category = "cluster_level"
		result.Reason = fmt.Sprintf("采集器 '%s' 是集群级数据类型，已由集群监控覆盖，无需本地采集", collector)
		result.Suggestion = "建议移除该采集器，使用集群监控数据替代"
		result.Metadata["scope"] = "cluster"
		result.Metadata["priority"] = "skip"
	case ClusterSupplement:
		result.IsValid = true
		result.Category = "cluster_supplement"
		result.Reason = fmt.Sprintf("采集器 '%s' 是集群监控无法提供的补充数据，需要在本地采集", collector)
		result.Suggestion = "该采集器提供集群监控缺失的细节数据，建议保留"
		result.Metadata["scope"] = "supplement"
		result.Metadata["priority"] = "medium"
	}

	v.cacheResult(collector, result)
	return result
}

// ValidateCollectors validates multiple collectors
func (v *DataTypeValidator) ValidateCollectors(collectors []string) []ValidationResult {
	results := make([]ValidationResult, len(collectors))
	for i, collector := range collectors {
		results[i] = *v.ValidateCollector(collector)
	}
	return results
}

// ValidateCheckConfig validates a complete check configuration
func (v *DataTypeValidator) ValidateCheckConfig(checkName string, collectors []string, mode string) *CheckValidationResult {
	result := &CheckValidationResult{
		CheckName:  checkName,
		Mode:       mode,
		Collectors: make([]ValidationResult, len(collectors)),
		Errors:     []string{},
	}

	var localCount, supplementCount, skipCount, unknownCount int

	for i, collector := range collectors {
		validation := v.ValidateCollector(collector)
		result.Collectors[i] = *validation

		switch validation.Category {
		case "node_local":
			localCount++
		case "cluster_supplement":
			supplementCount++
		case "cluster_level":
			skipCount++
		case "unknown":
			unknownCount++
		}

		// Check mode-specific validation
		if !v.isValidForMode(*validation, mode) {
			errMsg := fmt.Sprintf("采集器 '%s' 不适合 %s 模式: %s", collector, mode, validation.Reason)
			result.Errors = append(result.Errors, errMsg)
		}
	}

	result.Stats = ValidationStats{
		TotalCount:      len(collectors),
		LocalCount:      localCount,
		SupplementCount: supplementCount,
		SkipCount:       skipCount,
		UnknownCount:    unknownCount,
	}

	result.Valid = len(result.Errors) == 0
	return result
}

// isValidForMode checks if a validation result is valid for the given mode
func (v *DataTypeValidator) isValidForMode(validation ValidationResult, mode string) bool {
	switch mode {
	case "仅本地":
		return validation.Category == "node_local" || validation.Category == "unknown"
	case "集群补充":
		return validation.Category == "cluster_supplement" || validation.Category == "unknown"
	case "自动选择":
		return true // 自动选择模式下所有类型都有效
	default:
		return true
	}
}

// GetValidationReport generates a comprehensive validation report
func (v *DataTypeValidator) GetValidationReport(checks map[string][]string, mode string) *ValidationReport {
	report := &ValidationReport{
		Mode:        mode,
		TotalChecks: len(checks),
		Results:     make(map[string]*CheckValidationResult),
	}

	var totalLocal, totalSupplement, totalSkip, totalUnknown int

	for checkName, collectors := range checks {
		result := v.ValidateCheckConfig(checkName, collectors, mode)
		report.Results[checkName] = result

		totalLocal += result.Stats.LocalCount
		totalSupplement += result.Stats.SupplementCount
		totalSkip += result.Stats.SkipCount
		totalUnknown += result.Stats.UnknownCount
	}

	report.OverallStats = ValidationStats{
		TotalCount:      totalLocal + totalSupplement + totalSkip + totalUnknown,
		LocalCount:      totalLocal,
		SupplementCount: totalSupplement,
		SkipCount:       totalSkip,
		UnknownCount:    totalUnknown,
	}

	return report
}

// GetLocalAlternatives returns local alternatives for cluster-level data types
func (v *DataTypeValidator) getLocalAlternatives(clusterType string) []string {
	// Map cluster types to local alternatives
	clusterToLocal := map[string][]string{
		"node_cpu_usage":     {"cpu_throttling", "cpu_temperature", "cpu_frequency"},
		"node_memory_usage":  {"memory_fragmentation", "swap_usage", "oom_events"},
		"node_disk_io":       {"disk_smart", "disk_errors", "disk_performance"},
		"node_network_bytes": {"network_interface_errors", "tcp_retransmit_rate"},
	}

	if alts, exists := clusterToLocal[clusterType]; exists {
		return alts
	}

	// Default alternatives for unknown types
	return []string{"system_load", "file_descriptors", "kernel_logs"}
}

// GenerateSuggestions generates suggestions based on validation results
func (v *DataTypeValidator) generateSuggestions(result *CheckValidationResult, mode string) []string {
	suggestions := []string{}

	switch mode {
	case "仅本地":
		if result.Stats.SupplementCount > 0 {
			suggestions = append(suggestions, "当前为仅本地模式，但包含集群补充数据类型，建议检查配置")
		}
		if result.Stats.SkipCount > 0 {
			suggestions = append(suggestions, "检测到集群级数据类型，建议移除或使用本地替代")
		}
	case "集群补充":
		if result.Stats.LocalCount > 0 {
			suggestions = append(suggestions, "当前为集群补充模式，但包含节点本地数据类型，建议确认是否需要")
		}
	case "自动选择":
		if result.Stats.UnknownCount > 0 {
			suggestions = append(suggestions, "检测到未知类型采集器，建议确认数据类型分类")
		}
	}

	// 通用建议
	if result.Stats.SkipCount > 0 {
		suggestions = append(suggestions, "考虑移除被跳过的采集器以简化配置")
	}

	if len(suggestions) == 0 {
		suggestions = append(suggestions, "配置验证通过，当前模式适合所有采集器")
	}

	return suggestions
}

// GetSummary returns a human-readable summary of the validation report
func (r *ValidationReport) GetSummary() string {
	var lines []string
	lines = append(lines, fmt.Sprintf("模式: %s", r.Mode))
	lines = append(lines, fmt.Sprintf("检查项: %d", r.TotalChecks))
	lines = append(lines, fmt.Sprintf("总采集器: %d", r.OverallStats.TotalCount))
	lines = append(lines, fmt.Sprintf("节点本地: %d", r.OverallStats.LocalCount))
	lines = append(lines, fmt.Sprintf("集群补充: %d", r.OverallStats.SupplementCount))
	lines = append(lines, fmt.Sprintf("跳过采集: %d", r.OverallStats.SkipCount))
	lines = append(lines, fmt.Sprintf("未知类型: %d", r.OverallStats.UnknownCount))
	return strings.Join(lines, "\n")
}

// cacheResult stores a validation result in cache
func (v *DataTypeValidator) cacheResult(key string, result *ValidationResult) {
	v.mutex.Lock()
	defer v.mutex.Unlock()
	v.cache[key] = result
}

// ClearCache clears the validation cache
func (v *DataTypeValidator) ClearCache() {
	v.mutex.Lock()
	defer v.mutex.Unlock()
	for key := range v.cache {
		delete(v.cache, key)
	}
}
