package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestConfigManager_Load(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")

	configData := `{
		"version": "1.0",
		"checks": [
			{
				"name": "test_check",
				"interval": 300000000000,
				"collectors": ["cpu", "memory"],
				"mode": "自动选择",
				"enabled": true,
				"timeout": 30000000000,
				"priority": 1
			}
		],
		"resource_limits": {
			"max_cpu_percent": 5.0,
			"max_memory_mb": 100,
			"min_interval": 60000000000,
			"max_concurrent": 1
		},
		"data_scope": {
			"mode": "自动选择",
			"node_local_types": ["cpu_throttling"],
			"supplement_types": ["process_cpu_usage"],
			"skip_types": ["node_cpu_usage"]
		},
		"reporter": {
			"endpoint": "http://localhost:8080/api/v1/diagnostics",
			"timeout": 10000000000,
			"max_retries": 3,
			"retry_delay": 5000000000,
			"compression": true
		}
	}`

	if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// 测试配置加载
	logger, _ := zap.NewProduction()
	cm := NewConfigManager(logger)
	ctx := context.Background()

	if err := cm.Load(ctx, configFile); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	config := cm.Get()
	if config.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", config.Version)
	}

	if len(config.Checks) != 1 {
		t.Errorf("Expected 1 check, got %d", len(config.Checks))
	}

	if config.Checks[0].Name != "test_check" {
		t.Errorf("Expected check name 'test_check', got %s", config.Checks[0].Name)
	}
}

func TestConfigManager_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "empty version",
			config: &Config{
				Version: "",
			},
			wantErr: true,
		},
		{
			name: "empty checks",
			config: &Config{
				Version: "1.0",
				Checks:  []CheckConfig{},
			},
			wantErr: true,
		},
		{
			name: "invalid check mode",
			config: &Config{
				Version: "1.0",
				Checks: []CheckConfig{
					{
						Name:       "test",
						Interval:   5 * time.Minute,
						Collectors: []string{"cpu"},
						Mode:       "invalid",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, _ := zap.NewProduction()
			cm := NewConfigManager(logger)
			cm.config = tt.config
			err := cm.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigManager_Subscribe(t *testing.T) {
	logger, _ := zap.NewProduction()
	cm := NewConfigManager(logger)

	var receivedConfig *Config
	callback := func(config *Config) {
		receivedConfig = config
	}

	if err := cm.Subscribe(callback); err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// 创建临时配置文件
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")

	configData := `{
		"version": "2.0",
		"checks": [
			{
				"name": "test_check",
				"interval": 300000000000,
				"collectors": ["cpu"],
				"mode": "自动选择",
				"enabled": true,
				"timeout": 30000000000,
				"priority": 1
			}
		],
		"resource_limits": {
			"max_cpu_percent": 5.0,
			"max_memory_mb": 100,
			"min_interval": 60000000000,
			"max_concurrent": 1
		},
		"data_scope": {
			"mode": "自动选择",
			"node_local_types": ["cpu_throttling"],
			"supplement_types": ["process_cpu_usage"],
			"skip_types": ["node_cpu_usage"]
		},
		"reporter": {
			"endpoint": "http://localhost:8080/api/v1/diagnostics",
			"timeout": 10000000000,
			"max_retries": 3,
			"retry_delay": 5000000000,
			"compression": true
		}
	}`
	if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	ctx := context.Background()
	if err := cm.Load(ctx, configFile); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 等待回调执行
	time.Sleep(100 * time.Millisecond)

	if receivedConfig == nil {
		t.Error("Callback was not called")
	} else if receivedConfig.Version != "2.0" {
		t.Errorf("Expected version 2.0, got %s", receivedConfig.Version)
	}
}
