package config

// ReportConfig 上报配置
type ReportConfig struct {
	Endpoint string `yaml:"endpoint"` // 上报端点
	Interval int    `yaml:"interval"` // 上报间隔(秒)
}

// CollectorsConfig 采集器配置
type CollectorsConfig struct {
	SystemMetrics SystemMetricsConfig `yaml:"system_metrics"`
	JournaldLogs  JournaldConfig      `yaml:"journald_logs"`
}

// SystemMetricsConfig 系统指标采集配置
type SystemMetricsConfig struct {
	Enabled  bool `yaml:"enabled"`  // 是否启用
	Interval int  `yaml:"interval"` // 采集间隔(秒)
}

// JournaldConfig journald日志采集配置
type JournaldConfig struct {
	Enabled bool     `yaml:"enabled"` // 是否启用
	Units   []string `yaml:"units"`   // 监控的systemd单元列表
}
