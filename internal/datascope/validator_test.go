package datascope

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDataTypeValidator(t *testing.T) {
	checker, err := NewDataScopeChecker()
	require.NoError(t, err)

	validator := NewDataTypeValidator(checker)
	assert.NotNil(t, validator)
	assert.Equal(t, checker, validator.checker)
}

func TestValidateCollector(t *testing.T) {
	checker, err := NewDataScopeChecker()
	require.NoError(t, err)

	validator := NewDataTypeValidator(checker)

	tests := []struct {
		name             string
		collector        string
		expectedValid    bool
		expectedCategory string
		expectedReason   string
	}{
		{
			name:             "节点本地采集器",
			collector:        "cpu_temperature",
			expectedValid:    true,
			expectedCategory: "node_local",
			expectedReason:   "采集器 'cpu_temperature' 是节点本地独有数据类型，必须在本地采集",
		},
		{
			name:             "集群补充采集器",
			collector:        "process_cpu_usage",
			expectedValid:    true,
			expectedCategory: "cluster_supplement",
			expectedReason:   "采集器 'process_cpu_usage' 是集群监控无法提供的补充数据，需要在本地采集",
		},
		{
			name:             "集群级采集器",
			collector:        "node_cpu_usage",
			expectedValid:    false,
			expectedCategory: "cluster_level",
			expectedReason:   "采集器 'node_cpu_usage' 是集群级数据类型，已由集群监控覆盖，无需本地采集",
		},
		{
			name:             "未知采集器",
			collector:        "unknown_collector",
			expectedValid:    true,
			expectedCategory: "unknown",
			expectedReason:   "采集器 'unknown_collector' 是未知类型，默认作为节点本地数据处理",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.ValidateCollector(tt.collector)

			assert.Equal(t, tt.expectedValid, result.IsValid)
			assert.Equal(t, tt.expectedCategory, result.Category)
			assert.Contains(t, result.Reason, tt.expectedReason)
			assert.NotEmpty(t, result.Suggestion)
			assert.NotNil(t, result.Metadata)
		})
	}
}

func TestValidateCollectors(t *testing.T) {
	checker, err := NewDataScopeChecker()
	require.NoError(t, err)

	validator := NewDataTypeValidator(checker)

	collectors := []string{"cpu_temperature", "node_cpu_usage", "process_cpu_usage"}
	results := validator.ValidateCollectors(collectors)

	assert.Len(t, results, 3)

	// 验证每个结果
	assert.True(t, results[0].IsValid)  // cpu_temperature
	assert.False(t, results[1].IsValid) // node_cpu_usage
	assert.True(t, results[2].IsValid)  // process_cpu_usage
}

func TestValidateCheckConfig(t *testing.T) {
	checker, err := NewDataScopeChecker()
	require.NoError(t, err)

	validator := NewDataTypeValidator(checker)

	tests := []struct {
		name           string
		checkName      string
		collectors     []string
		mode           string
		expectedValid  bool
		expectedErrors int
	}{
		{
			name:           "仅本地模式 - 有效配置",
			checkName:      "cpu_check",
			collectors:     []string{"cpu_temperature", "cpu_frequency"},
			mode:           "仅本地",
			expectedValid:  true,
			expectedErrors: 0,
		},
		{
			name:           "仅本地模式 - 无效配置",
			checkName:      "mixed_check",
			collectors:     []string{"cpu_temperature", "node_cpu_usage"},
			mode:           "仅本地",
			expectedValid:  false,
			expectedErrors: 1,
		},
		{
			name:           "集群补充模式 - 有效配置",
			checkName:      "supplement_check",
			collectors:     []string{"process_cpu_usage", "kernel_dmesg"},
			mode:           "集群补充",
			expectedValid:  true,
			expectedErrors: 0,
		},
		{
			name:           "集群补充模式 - 无效配置",
			checkName:      "invalid_supplement",
			collectors:     []string{"process_cpu_usage", "node_cpu_usage"},
			mode:           "集群补充",
			expectedValid:  false,
			expectedErrors: 1,
		},
		{
			name:           "自动选择模式",
			checkName:      "auto_check",
			collectors:     []string{"cpu_temperature", "process_cpu_usage", "node_cpu_usage"},
			mode:           "自动选择",
			expectedValid:  true,
			expectedErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.ValidateCheckConfig(tt.checkName, tt.collectors, tt.mode)

			assert.Equal(t, tt.expectedValid, result.Valid)
			assert.Len(t, result.Errors, tt.expectedErrors)
			assert.Equal(t, tt.checkName, result.CheckName)
			assert.Equal(t, tt.mode, result.Mode)
			assert.Len(t, result.Collectors, len(tt.collectors))
		})
	}
}

func TestValidationStats(t *testing.T) {
	checker, err := NewDataScopeChecker()
	require.NoError(t, err)

	validator := NewDataTypeValidator(checker)

	collectors := []string{"cpu_temperature", "node_cpu_usage", "process_cpu_usage", "unknown_collector"}
	result := validator.ValidateCheckConfig("test_check", collectors, "自动选择")

	stats := result.Stats
	assert.Equal(t, 4, stats.TotalCount)
	assert.Equal(t, 1, stats.LocalCount)      // cpu_temperature
	assert.Equal(t, 1, stats.SupplementCount) // process_cpu_usage
	assert.Equal(t, 1, stats.SkipCount)       // node_cpu_usage
	assert.Equal(t, 1, stats.UnknownCount)    // unknown_collector
}

func TestGetLocalAlternatives(t *testing.T) {
	checker, err := NewDataScopeChecker()
	require.NoError(t, err)

	validator := NewDataTypeValidator(checker)

	tests := []struct {
		name         string
		clusterType  string
		expectedAlts []string
	}{
		{
			name:         "CPU相关",
			clusterType:  "node_cpu_usage",
			expectedAlts: []string{"cpu_throttling", "cpu_temperature", "cpu_frequency"},
		},
		{
			name:         "内存相关",
			clusterType:  "node_memory_usage",
			expectedAlts: []string{"memory_fragmentation", "swap_usage", "oom_events"},
		},
		{
			name:         "磁盘相关",
			clusterType:  "node_disk_io",
			expectedAlts: []string{"disk_smart", "disk_errors", "disk_performance"},
		},
		{
			name:         "网络相关",
			clusterType:  "node_network_bytes",
			expectedAlts: []string{"network_interface_errors", "tcp_retransmit_rate"},
		},
		{
			name:         "未知类型",
			clusterType:  "unknown_type",
			expectedAlts: []string{"system_load", "file_descriptors", "kernel_logs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alts := validator.getLocalAlternatives(tt.clusterType)
			assert.Equal(t, tt.expectedAlts, alts)
		})
	}
}

func TestGetValidationReport(t *testing.T) {
	checker, err := NewDataScopeChecker()
	require.NoError(t, err)

	validator := NewDataTypeValidator(checker)

	checks := map[string][]string{
		"cpu_check":    {"cpu_temperature", "cpu_frequency"},
		"memory_check": {"memory_fragmentation", "node_memory_usage"},
		"mixed_check":  {"process_cpu_usage", "node_cpu_usage", "unknown_collector"},
	}

	report := validator.GetValidationReport(checks, "自动选择")

	assert.Equal(t, "自动选择", report.Mode)
	assert.Equal(t, 3, report.TotalChecks)
	assert.Len(t, report.Results, 3)

	// 验证总体统计 - 修正期望数量
	stats := report.OverallStats
	assert.Equal(t, 7, stats.TotalCount) // 2+2+3=7个采集器
	assert.True(t, stats.LocalCount > 0)
	assert.True(t, stats.SkipCount > 0)

	// 验证摘要
	summary := report.GetSummary()
	assert.Contains(t, summary, "模式: 自动选择")
	assert.Contains(t, summary, "检查项: 3")
}

func TestGenerateSuggestions(t *testing.T) {
	checker, err := NewDataScopeChecker()
	require.NoError(t, err)

	validator := NewDataTypeValidator(checker)

	tests := []struct {
		name           string
		mode           string
		stats          ValidationStats
		expectedLength int
	}{
		{
			name:           "仅本地模式有跳过",
			mode:           "仅本地",
			stats:          ValidationStats{SkipCount: 1, SupplementCount: 1},
			expectedLength: 3,
		},
		{
			name:           "集群补充模式有本地数据",
			mode:           "集群补充",
			stats:          ValidationStats{LocalCount: 1},
			expectedLength: 1,
		},
		{
			name:           "自动选择模式有未知类型",
			mode:           "自动选择",
			stats:          ValidationStats{UnknownCount: 1},
			expectedLength: 1, // 修正为1个建议
		},
		{
			name:           "无特殊情况的通用建议",
			mode:           "自动选择",
			stats:          ValidationStats{SkipCount: 1},
			expectedLength: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &CheckValidationResult{
				Stats: tt.stats,
			}
			suggestions := validator.generateSuggestions(result, tt.mode)
			assert.Len(t, suggestions, tt.expectedLength)
		})
	}
}

func TestValidationResultFields(t *testing.T) {
	checker, err := NewDataScopeChecker()
	require.NoError(t, err)

	validator := NewDataTypeValidator(checker)

	result := validator.ValidateCollector("cpu_temperature")

	assert.NotEmpty(t, result.Collector)
	assert.NotEmpty(t, result.Category)
	assert.NotEmpty(t, result.Reason)
	assert.NotEmpty(t, result.Suggestion)
	assert.NotNil(t, result.Metadata)
	assert.Contains(t, result.Metadata, "scope")
	assert.Contains(t, result.Metadata, "priority")
}
