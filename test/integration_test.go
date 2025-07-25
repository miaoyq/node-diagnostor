package test

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/miaoyq/node-diagnostor/internal/config"
	"github.com/miaoyq/node-diagnostor/internal/datascope"
)

// TestConfigHotReload 测试配置热更新功能
func TestConfigHotReload(t *testing.T) {
	// 创建临时目录和文件
	tempDir, err := ioutil.TempDir("", "diagnostor-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "config.json")
	initialConfig := config.Config{
		Version: "1.0",
		Checks: []config.CheckConfig{
			{
				Name:       "cpu_check",
				Interval:   5 * time.Second,
				Timeout:    30 * time.Second,
				Mode:       "仅本地",
				Collectors: []string{"cpu", "memory"},
				Params: map[string]interface{}{
					"threshold": 80,
				},
				Enabled:  true,
				Priority: 1,
			},
		},
		ResourceLimits: config.ResourceLimits{
			MaxCPU:        5.0,
			MaxMemory:     100,
			MaxConcurrent: 10,
		},
	}

	// 写入初始配置
	configData, _ := json.MarshalIndent(initialConfig, "", "  ")
	err = ioutil.WriteFile(configFile, configData, 0644)
	require.NoError(t, err)

	// 创建配置管理器
	logger, _ := zap.NewDevelopment()
	manager := config.NewConfigManager(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 加载配置
	err = manager.Load(ctx, configFile)
	require.NoError(t, err)

	// 验证初始配置
	cfg := manager.Get()
	assert.Equal(t, "1.0", cfg.Version)
	assert.Len(t, cfg.Checks, 1)
	assert.Equal(t, "cpu_check", cfg.Checks[0].Name)

	// 修改配置
	updatedConfig := config.Config{
		Version: "1.1",
		Checks: []config.CheckConfig{
			{
				Name:       "cpu_check",
				Interval:   10 * time.Second,
				Timeout:    30 * time.Second,
				Mode:       "仅本地",
				Collectors: []string{"cpu"},
				Params: map[string]interface{}{
					"threshold": 90,
				},
				Enabled:  true,
				Priority: 1,
			},
			{
				Name:       "memory_check",
				Interval:   15 * time.Second,
				Timeout:    30 * time.Second,
				Mode:       "集群补充",
				Collectors: []string{"memory", "disk"},
				Params: map[string]interface{}{
					"threshold": 85,
				},
				Enabled:  true,
				Priority: 2,
			},
		},
		ResourceLimits: config.ResourceLimits{
			MaxCPU:        3.0,
			MaxMemory:     80,
			MaxConcurrent: 8,
		},
	}

	// 写入更新后的配置
	updatedData, _ := json.MarshalIndent(updatedConfig, "", "  ")
	err = os.WriteFile(configFile, updatedData, 0644)
	require.NoError(t, err)

	// 手动触发重载
	err = manager.Reload(ctx)
	require.NoError(t, err)

	// 验证配置已更新
	updatedCfg := manager.Get()
	assert.Equal(t, "1.1", updatedCfg.Version)
	assert.Len(t, updatedCfg.Checks, 2)
	assert.Equal(t, 10*time.Second, updatedCfg.Checks[0].Interval)
	assert.Equal(t, 15*time.Second, updatedCfg.Checks[1].Interval)
}

// TestDataScopeModes 测试数据范围模式
func TestDataScopeModes(t *testing.T) {
	checker, err := datascope.NewDataScopeChecker()
	require.NoError(t, err)

	tests := []struct {
		name       string
		mode       string
		collectors []string
		expected   bool
	}{
		{
			name:       "本地模式-本地采集器",
			mode:       "仅本地",
			collectors: []string{"cpu_temperature"},
			expected:   true,
		},
		{
			name:       "本地模式-集群采集器",
			mode:       "仅本地",
			collectors: []string{"node_cpu_usage"},
			expected:   false,
		},
		{
			name:       "集群补充模式-补充采集器",
			mode:       "集群补充",
			collectors: []string{"process_cpu_usage"},
			expected:   true,
		},
		{
			name:       "自动选择模式-本地采集器",
			mode:       "自动选择",
			collectors: []string{"cpu_temperature"},
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := checker.ShouldCollectLocally("test-check", tt.collectors, tt.mode)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.ShouldCollect, "mode: %s, collectors: %v", tt.mode, tt.collectors)
		})
	}
}

// TestEndToEndIntegration 测试端到端集成
func TestEndToEndIntegration(t *testing.T) {
	// 创建临时配置
	tempDir, err := ioutil.TempDir("", "diagnostor-e2e")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "config.json")
	configData := config.Config{
		Version: "1.0",
		Checks: []config.CheckConfig{
			{
				Name:       "cpu_check",
				Interval:   100 * time.Millisecond,
				Timeout:    30 * time.Second,
				Mode:       "仅本地",
				Collectors: []string{"cpu"},
				Params: map[string]interface{}{
					"threshold": 80,
				},
				Enabled:  true,
				Priority: 1,
			},
		},
		Reporter: config.ReporterConfig{
			Endpoint: "http://localhost:8080/report",
			Timeout:  5 * time.Second,
		},
	}

	configBytes, _ := json.MarshalIndent(configData, "", "  ")
	err = os.WriteFile(configFile, configBytes, 0644)
	require.NoError(t, err)

	// 验证配置加载
	logger, _ := zap.NewDevelopment()
	manager := config.NewConfigManager(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = manager.Load(ctx, configFile)
	require.NoError(t, err)

	// 验证配置正确加载
	cfg := manager.Get()
	assert.Equal(t, "1.0", cfg.Version)
	assert.Len(t, cfg.Checks, 1)
	assert.Equal(t, "cpu_check", cfg.Checks[0].Name)
}
