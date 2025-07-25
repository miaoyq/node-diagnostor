package config

import (
	"time"
)

// Config 主配置结构体
type Config struct {
	Version        string          `json:"version"`         // 配置版本
	Checks         []CheckConfig   `json:"checks"`          // 检查项配置
	ResourceLimits ResourceLimits  `json:"resource_limits"` // 资源限制配置
	DataScope      DataScopeConfig `json:"data_scope"`      // 数据范围配置
	Reporter       ReporterConfig  `json:"reporter"`        // 上报配置
}

// CheckConfig 检查项配置
type CheckConfig struct {
	Name       string                 `json:"name"`       // 检查项名称
	Interval   time.Duration          `json:"interval"`   // 检查周期
	Collectors []string               `json:"collectors"` // 采集器ID列表
	Params     map[string]interface{} `json:"params"`     // 采集参数
	Mode       string                 `json:"mode"`       // 数据范围模式："仅本地", "集群补充", "自动选择"
	Enabled    bool                   `json:"enabled"`    // 启用/禁用开关
	Timeout    time.Duration          `json:"timeout"`    // 检查超时时间
	Priority   int                    `json:"priority"`   // 优先级（资源紧张时低优先级先降级）
}

// ResourceLimits 资源限制配置
type ResourceLimits struct {
	MaxCPU              float64       `json:"max_cpu_percent"`       // 最大CPU使用率：5%
	MaxMemory           int64         `json:"max_memory_mb"`         // 最大内存使用：100MB
	MinInterval         time.Duration `json:"min_interval"`          // 最小检查间隔：1分钟
	MaxConcurrent       int           `json:"max_concurrent"`        // 最大并发检查数：1（顺序执行）
	MaxDataPoints       int           `json:"max_data_points"`       // 最大数据点数
	MaxDataSize         int           `json:"max_data_size"`         // 最大数据大小（字节）
	DataRetentionPeriod time.Duration `json:"data_retention_period"` // 数据保留期
	EnableCompression   bool          `json:"enable_compression"`    // 是否启用压缩
}

// DataScopeConfig 数据范围配置
type DataScopeConfig struct {
	Mode            string   `json:"mode"`             // 数据范围模式："仅本地", "集群补充", "自动选择"
	NodeLocalTypes  []string `json:"node_local_types"` // 节点本地独有数据类型
	SupplementTypes []string `json:"supplement_types"` // 集群补充数据类型
	SkipTypes       []string `json:"skip_types"`       // 跳过采集的数据类型
}

// ReporterConfig 上报配置
type ReporterConfig struct {
	Endpoint     string        `json:"endpoint"`       // 上报端点
	Timeout      time.Duration `json:"timeout"`        // 上报超时
	MaxRetries   int           `json:"max_retries"`    // 最大重试次数
	RetryDelay   time.Duration `json:"retry_delay"`    // 重试延迟
	Compression  bool          `json:"compression"`    // 是否压缩数据
	CacheMaxSize int64         `json:"cache_max_size"` // 缓存最大大小（字节）
	CacheMaxAge  time.Duration `json:"cache_max_age"`  // 缓存最大存活时间
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Version: "1.0",
		Checks: []CheckConfig{
			{
				Name:       "node_health",
				Interval:   5 * time.Minute,
				Collectors: []string{"cpu", "memory", "disk"},
				Params: map[string]interface{}{
					"cpu_threshold":    80,
					"memory_threshold": 90,
				},
				Mode:     "自动选择",
				Enabled:  true,
				Timeout:  30 * time.Second,
				Priority: 1,
			},
		},
		ResourceLimits: ResourceLimits{
			MaxCPU:        5.0,
			MaxMemory:     100,
			MinInterval:   1 * time.Minute,
			MaxConcurrent: 1,
		},
		DataScope: DataScopeConfig{
			Mode: "自动选择",
			NodeLocalTypes: []string{
				"cpu_throttling", "cpu_temperature", "memory_fragmentation",
				"disk_smart", "network_interface_errors", "kubelet_process",
			},
			SupplementTypes: []string{
				"process_cpu_usage", "kernel_dmesg", "cert_validity",
			},
			SkipTypes: []string{
				"node_cpu_usage", "node_memory_usage", "node_disk_io",
			},
		},
		Reporter: ReporterConfig{
			Endpoint:     "http://localhost:8080/api/v1/diagnostics",
			Timeout:      10 * time.Second,
			MaxRetries:   3,
			RetryDelay:   5 * time.Second,
			Compression:  true,
			CacheMaxSize: 100 * 1024 * 1024, // 100MB
			CacheMaxAge:  24 * time.Hour,    // 24小时
		},
	}
}
