package datascope

import (
	"fmt"
	"strings"
)

// DataTypeValidator 数据类型验证器，专门用于验证采集器类型和提供详细的跳过原因
type DataTypeValidator struct {
	checker *DataScopeChecker
}

// ValidationResult 验证结果，包含详细的验证信息
type ValidationResult struct {
	IsValid      bool              // 是否有效
	Collector    string            // 采集器名称
	Category     string            // 数据类型分类
	Reason       string            // 详细原因
	Suggestion   string            // 建议操作
	Alternatives []string          // 替代采集器
	Metadata     map[string]string // 额外元数据
}

// NewDataTypeValidator 创建新的数据类型验证器
func NewDataTypeValidator(checker *DataScopeChecker) *DataTypeValidator {
	return &DataTypeValidator{
		checker: checker,
	}
}

// ValidateCollector 验证单个采集器类型
func (v *DataTypeValidator) ValidateCollector(collector string) *ValidationResult {
	result := &ValidationResult{
		Collector: collector,
		Metadata:  make(map[string]string),
	}

	// 检查是否为节点本地独有数据
	if v.checker.IsNodeLocalType(collector) {
		result.IsValid = true
		result.Category = "node_local"
		result.Reason = fmt.Sprintf("采集器 '%s' 是节点本地独有数据类型，必须在本地采集", collector)
		result.Suggestion = "可以直接使用此采集器"
		result.Metadata["scope"] = "local"
		result.Metadata["priority"] = "high"
		return result
	}

	// 检查是否为集群补充数据
	if v.checker.IsSupplementType(collector) {
		result.IsValid = true
		result.Category = "cluster_supplement"
		result.Reason = fmt.Sprintf("采集器 '%s' 是集群监控无法提供的补充数据，需要在本地采集", collector)
		result.Suggestion = "在集群补充模式下使用此采集器"
		result.Metadata["scope"] = "supplement"
		result.Metadata["priority"] = "medium"
		return result
	}

	// 检查是否为跳过采集的数据类型
	if v.checker.IsSkipType(collector) {
		result.IsValid = false
		result.Category = "cluster_level"
		result.Reason = fmt.Sprintf("采集器 '%s' 是集群级数据类型，已由集群监控覆盖，无需本地采集", collector)
		result.Suggestion = "移除此采集器，依赖集群监控"
		result.Alternatives = v.getLocalAlternatives(collector)
		result.Metadata["scope"] = "cluster"
		result.Metadata["priority"] = "skip"
		return result
	}

	// 未知类型，默认作为节点本地处理
	result.IsValid = true
	result.Category = "unknown"
	result.Reason = fmt.Sprintf("采集器 '%s' 是未知类型，默认作为节点本地数据处理", collector)
	result.Suggestion = "确认此采集器是否确实需要本地采集"
	result.Metadata["scope"] = "local"
	result.Metadata["priority"] = "low"
	result.Metadata["note"] = "unknown_type"

	return result
}

// ValidateCollectors 验证多个采集器类型
func (v *DataTypeValidator) ValidateCollectors(collectors []string) []*ValidationResult {
	results := make([]*ValidationResult, 0, len(collectors))
	for _, collector := range collectors {
		results = append(results, v.ValidateCollector(collector))
	}
	return results
}

// ValidateCheckConfig 验证检查项配置
func (v *DataTypeValidator) ValidateCheckConfig(checkName string, collectors []string, mode string) *CheckValidationResult {
	result := &CheckValidationResult{
		CheckName:  checkName,
		Mode:       mode,
		Valid:      true,
		Collectors: make([]*ValidationResult, 0, len(collectors)),
	}

	// 验证每个采集器
	for _, collector := range collectors {
		validation := v.ValidateCollector(collector)
		result.Collectors = append(result.Collectors, validation)

		// 根据模式判断有效性
		switch mode {
		case "仅本地":
			if !validation.IsValid || validation.Category != "node_local" {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("仅本地模式下不允许使用非节点本地采集器: %s", collector))
			}
		case "集群补充":
			if validation.Category == "cluster_level" {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("集群补充模式下不允许使用集群级采集器: %s", collector))
			}
		}
	}

	// 统计结果
	result.Stats = v.calculateStats(result.Collectors)

	// 生成建议
	result.Suggestions = v.generateSuggestions(result, mode)

	return result
}

// CheckValidationResult 检查项验证结果
type CheckValidationResult struct {
	CheckName   string              // 检查项名称
	Mode        string              // 采集模式
	Valid       bool                // 配置是否有效
	Collectors  []*ValidationResult // 采集器验证结果
	Errors      []string            // 配置错误
	Stats       ValidationStats     // 统计信息
	Suggestions []string            // 配置建议
}

// ValidationStats 验证统计信息
type ValidationStats struct {
	TotalCount      int // 总采集器数量
	LocalCount      int // 节点本地采集器数量
	SupplementCount int // 集群补充采集器数量
	SkipCount       int // 跳过采集器数量
	UnknownCount    int // 未知类型采集器数量
}

// calculateStats 计算验证统计信息
func (v *DataTypeValidator) calculateStats(results []*ValidationResult) ValidationStats {
	stats := ValidationStats{
		TotalCount: len(results),
	}

	for _, result := range results {
		switch result.Category {
		case "node_local":
			stats.LocalCount++
		case "cluster_supplement":
			stats.SupplementCount++
		case "cluster_level":
			stats.SkipCount++
		case "unknown":
			stats.UnknownCount++
		}
	}

	return stats
}

// generateSuggestions 生成配置建议
func (v *DataTypeValidator) generateSuggestions(result *CheckValidationResult, mode string) []string {
	suggestions := []string{}

	switch mode {
	case "仅本地":
		if result.Stats.SkipCount > 0 {
			suggestions = append(suggestions, "仅本地模式下移除了集群级采集器")
		}
		if result.Stats.SupplementCount > 0 {
			suggestions = append(suggestions, "仅本地模式下集群补充采集器也被移除")
		}
	case "集群补充":
		if result.Stats.LocalCount > 0 {
			suggestions = append(suggestions, "集群补充模式下节点本地采集器也被保留")
		}
	case "自动选择":
		if result.Stats.UnknownCount > 0 {
			suggestions = append(suggestions, "建议明确未知类型采集器的数据范围")
		}
	}

	// 通用建议
	if result.Stats.SkipCount > 0 {
		suggestions = append(suggestions, "考虑移除被跳过的采集器以简化配置")
	}

	return suggestions
}

// getLocalAlternatives 获取本地替代采集器
func (v *DataTypeValidator) getLocalAlternatives(clusterType string) []string {
	alternatives := []string{}

	// 根据集群级类型提供对应的本地采集器
	switch {
	case strings.Contains(clusterType, "cpu"):
		alternatives = []string{"cpu_throttling", "cpu_temperature", "cpu_frequency"}
	case strings.Contains(clusterType, "memory"):
		alternatives = []string{"memory_fragmentation", "swap_usage", "oom_events"}
	case strings.Contains(clusterType, "disk"):
		alternatives = []string{"disk_smart", "disk_errors", "disk_performance"}
	case strings.Contains(clusterType, "network"):
		alternatives = []string{"network_interface_errors", "tcp_retransmit_rate"}
	default:
		alternatives = []string{"system_load", "file_descriptors", "kernel_logs"}
	}

	return alternatives
}

// GetValidationReport 生成完整的验证报告
func (v *DataTypeValidator) GetValidationReport(checks map[string][]string, mode string) *ValidationReport {
	report := &ValidationReport{
		Mode:        mode,
		TotalChecks: len(checks),
		Results:     make(map[string]*CheckValidationResult),
	}

	for checkName, collectors := range checks {
		report.Results[checkName] = v.ValidateCheckConfig(checkName, collectors, mode)
	}

	// 计算总体统计
	report.calculateOverallStats()

	return report
}

// ValidationReport 完整验证报告
type ValidationReport struct {
	Mode         string                            // 采集模式
	TotalChecks  int                               // 总检查项数量
	Results      map[string]*CheckValidationResult // 检查项验证结果
	OverallStats ValidationStats                   // 总体统计
}

// calculateOverallStats 计算总体统计信息
func (r *ValidationReport) calculateOverallStats() {
	stats := ValidationStats{}

	for _, result := range r.Results {
		stats.TotalCount += result.Stats.TotalCount
		stats.LocalCount += result.Stats.LocalCount
		stats.SupplementCount += result.Stats.SupplementCount
		stats.SkipCount += result.Stats.SkipCount
		stats.UnknownCount += result.Stats.UnknownCount
	}

	r.OverallStats = stats
}

// GetSummary 获取验证摘要
func (r *ValidationReport) GetSummary() string {
	return fmt.Sprintf(
		"模式: %s, 检查项: %d, 采集器: %d (本地: %d, 补充: %d, 跳过: %d, 未知: %d)",
		r.Mode,
		r.TotalChecks,
		r.OverallStats.TotalCount,
		r.OverallStats.LocalCount,
		r.OverallStats.SupplementCount,
		r.OverallStats.SkipCount,
		r.OverallStats.UnknownCount,
	)
}
