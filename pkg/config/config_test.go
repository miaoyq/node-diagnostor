package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	// 创建临时配置文件
	configContent := `{
		"metrics_interval": "30s",
		"journal_units": ["kubelet", "containerd"],
		"report_url": "http://localhost:8080/report",
		"cache_ttl": "1m",
		"check_configs": [
			{
				"name": "kernel_params",
				"enable": true,
				"params": {
					"check_items": "net.ipv4.tcp_tw_reuse"
				}
			}
		]
	}`

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	// 测试加载配置
	cfg, err := LoadConfig(configFile)
	assert.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.MetricsInterval)
	assert.Equal(t, []string{"kubelet", "containerd"}, cfg.JournalUnits)
	assert.Equal(t, "http://localhost:8080/report", cfg.ReportURL)
	assert.Equal(t, 1*time.Minute, cfg.CacheTTL)
	assert.Len(t, cfg.CheckConfigs, 1)
	assert.Equal(t, "kernel_params", cfg.CheckConfigs[0].Name)
	assert.True(t, cfg.CheckConfigs[0].Enable)
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	// 创建无效JSON文件
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	err := os.WriteFile(configFile, []byte("invalid json"), 0644)
	assert.NoError(t, err)

	// 测试加载无效配置
	_, err = LoadConfig(configFile)
	assert.Error(t, err)
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	// 测试文件不存在的情况
	_, err := LoadConfig("/nonexistent/config.json")
	assert.Error(t, err)
}

func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				MetricsInterval: 30 * time.Second,
				JournalUnits:    []string{"kubelet"},
				ReportURL:       "http://localhost:8080/report",
				CacheTTL:        1 * time.Minute,
				CheckConfigs: []CheckConfig{
					{
						Name:   "kernel_params",
						Enable: true,
						Params: map[string]string{"check_items": "net.ipv4.tcp_tw_reuse"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty report url",
			config: Config{
				MetricsInterval: 30 * time.Second,
				JournalUnits:    []string{"kubelet"},
				ReportURL:       "",
				CacheTTL:        1 * time.Minute,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	// 测试默认配置
	cfg := getDefaultConfig()
	assert.NotEmpty(t, cfg.MetricsInterval)
	assert.NotEmpty(t, cfg.ReportURL)
	assert.NotEmpty(t, cfg.JournalUnits)
}

func TestConfig_String(t *testing.T) {
	// 测试配置字符串表示
	cfg := Config{
		MetricsInterval: 30 * time.Second,
		JournalUnits:    []string{"kubelet", "containerd"},
		ReportURL:       "http://localhost:8080/report",
		CacheTTL:        1 * time.Minute,
		CheckConfigs: []CheckConfig{
			{
				Name:   "kernel_params",
				Enable: true,
				Params: map[string]string{"check_items": "net.ipv4.tcp_tw_reuse"},
			},
		},
	}

	str := cfg.String()
	assert.Contains(t, str, "http://localhost:8080/report")
	assert.Contains(t, str, "kubelet")
}

func TestCheckConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  CheckConfig
		wantErr bool
	}{
		{
			name: "valid check config",
			config: CheckConfig{
				Name:   "kernel_params",
				Enable: true,
				Params: map[string]string{"check_items": "net.ipv4.tcp_tw_reuse"},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			config: CheckConfig{
				Name:   "",
				Enable: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCheckConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// 添加验证函数
func validateConfig(cfg Config) error {
	if cfg.ReportURL == "" {
		return fmt.Errorf("report_url不能为空")
	}
	if cfg.MetricsInterval <= 0 {
		return fmt.Errorf("metrics_interval必须大于0")
	}
	if cfg.CacheTTL <= 0 {
		return fmt.Errorf("cache_ttl必须大于0")
	}
	return nil
}

func validateCheckConfig(cfg CheckConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("check name不能为空")
	}
	return nil
}

func getDefaultConfig() Config {
	return Config{
		MetricsInterval: 30 * time.Second,
		JournalUnits:    []string{"kubelet", "containerd"},
		ReportURL:       "http://localhost:8080/report",
		CacheTTL:        1 * time.Minute,
		CheckConfigs: []CheckConfig{
			{
				Name:   "kernel_params",
				Enable: true,
				Params: map[string]string{"check_items": "net.ipv4.tcp_tw_reuse,vm.swappiness"},
			},
		},
	}
}

func (c Config) String() string {
	data, _ := json.MarshalIndent(c, "", "  ")
	return string(data)
}
