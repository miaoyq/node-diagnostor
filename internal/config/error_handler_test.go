package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestConfigErrorHandler_HandleFormatError(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	logger, _ := zap.NewDevelopment()
	handler := NewConfigErrorHandler(logger, tempDir)

	// 创建无效的配置文件
	configPath := filepath.Join(tempDir, "invalid.json")
	err = os.WriteFile(configPath, []byte("{invalid json}"), 0644)
	require.NoError(t, err)

	// 处理格式错误
	ctx := context.Background()
	config, err := handler.HandleError(ctx,
		NewConfigErrorWithCause(ConfigErrorTypeFormat, "invalid JSON format", nil), configPath)

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "1.0", config.Version)
}

func TestConfigErrorHandler_HandleValidationError(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	logger, _ := zap.NewDevelopment()
	handler := NewConfigErrorHandler(logger, tempDir)

	// 创建无效的配置文件
	configPath := filepath.Join(tempDir, "invalid.json")
	invalidConfig := Config{
		Version: "", // 空版本号
		Checks:  []CheckConfig{},
	}
	data, _ := json.MarshalIndent(invalidConfig, "", "  ")
	err = os.WriteFile(configPath, data, 0644)
	require.NoError(t, err)

	// 处理验证错误
	ctx := context.Background()
	config, err := handler.HandleError(ctx,
		NewConfigErrorWithField(ConfigErrorTypeValidation, "version", "version is required", ""), configPath)

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "1.0", config.Version)
}

func TestConfigErrorHandler_HandleFileError(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	logger, _ := zap.NewDevelopment()
	handler := NewConfigErrorHandler(logger, tempDir)

	// 处理不存在的文件错误
	ctx := context.Background()
	config, err := handler.HandleError(ctx,
		NewConfigError(ConfigErrorTypeFile, "file not found"), "nonexistent.json")

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "1.0", config.Version)
}

func TestConfigErrorHandler_BackupConfig(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	logger, _ := zap.NewDevelopment()
	handler := NewConfigErrorHandler(logger, tempDir)

	// 创建测试配置文件
	configPath := filepath.Join(tempDir, "test.json")
	testConfig := Config{
		Version: "2.0",
		Checks: []CheckConfig{
			{
				Name:     "test",
				Interval: 5 * time.Minute,
				Timeout:  30 * time.Second,
				Mode:     "自动选择",
			},
		},
	}
	data, _ := json.MarshalIndent(testConfig, "", "  ")
	err = os.WriteFile(configPath, data, 0644)
	require.NoError(t, err)

	// 备份配置
	err = handler.backupConfig(configPath)
	assert.NoError(t, err)

	// 检查备份文件是否存在
	backupFiles, err := filepath.Glob(filepath.Join(tempDir, "config-*.json"))
	assert.NoError(t, err)
	assert.Len(t, backupFiles, 1)
}

func TestConfigErrorHandler_CleanupOldBackups(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	logger, _ := zap.NewDevelopment()
	handler := NewConfigErrorHandler(logger, tempDir)
	handler.maxBackups = 2

	// 创建多个备份文件
	for i := 0; i < 5; i++ {
		backupPath := filepath.Join(tempDir, fmt.Sprintf("config-2024010%d-120000.json", i+1))
		err = os.WriteFile(backupPath, []byte("{}"), 0644)
		require.NoError(t, err)
		// 设置不同的修改时间
		time.Sleep(10 * time.Millisecond)
	}

	// 清理旧备份
	handler.cleanupOldBackups()

	// 检查剩余备份文件数量
	backupFiles, err := filepath.Glob(filepath.Join(tempDir, "config-*.json"))
	assert.NoError(t, err)
	assert.Len(t, backupFiles, 2)
}

func TestConfigErrorHandler_LoadAndFixConfig(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	logger, _ := zap.NewDevelopment()
	handler := NewConfigErrorHandler(logger, tempDir)

	// 创建有验证错误的配置文件
	configPath := filepath.Join(tempDir, "fixable.json")
	invalidConfig := Config{
		Version: "", // 空版本号
		Checks: []CheckConfig{
			{
				Name:     "test",
				Interval: 0, // 无效间隔
				Timeout:  30 * time.Second,
				Mode:     "自动选择",
			},
		},
		ResourceLimits: ResourceLimits{
			MaxCPU:    150, // 无效值
			MaxMemory: 0,   // 无效值
		},
	}
	data, _ := json.MarshalIndent(invalidConfig, "", "  ")
	err = os.WriteFile(configPath, data, 0644)
	require.NoError(t, err)

	// 加载并修复配置
	config, err := handler.loadAndFixConfig(configPath)

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "1.0", config.Version)
	assert.Equal(t, 5.0, config.ResourceLimits.MaxCPU)
	assert.Equal(t, int64(100), config.ResourceLimits.MaxMemory)
}

func TestConfigErrorHandler_SaveAndLoadFallbackConfig(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	logger, _ := zap.NewDevelopment()
	handler := NewConfigErrorHandler(logger, tempDir)

	// 测试配置
	testConfig := &Config{
		Version: "3.0",
		Checks: []CheckConfig{
			{
				Name:     "fallback-test",
				Interval: 10 * time.Minute,
				Timeout:  60 * time.Second,
				Mode:     "仅本地",
			},
		},
	}

	// 保存回退配置
	err = handler.saveFallbackConfig(testConfig)
	assert.NoError(t, err)

	// 加载回退配置
	loadedConfig, err := handler.loadFallbackConfig()
	assert.NoError(t, err)
	assert.Equal(t, testConfig.Version, loadedConfig.Version)
	assert.Len(t, loadedConfig.Checks, 1)
	assert.Equal(t, "fallback-test", loadedConfig.Checks[0].Name)
}

func TestConfigErrorHandler_HandleUnknownError(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	logger, _ := zap.NewDevelopment()
	handler := NewConfigErrorHandler(logger, tempDir)

	// 处理未知错误
	ctx := context.Background()
	config, err := handler.HandleError(ctx, fmt.Errorf("unknown error"), "test.json")

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "1.0", config.Version)
}

func TestConfigErrorHandler_ErrorTypes(t *testing.T) {
	tests := []struct {
		name     string
		err      *ConfigError
		expected string
	}{
		{
			name:     "format error",
			err:      NewConfigError(ConfigErrorTypeFormat, "invalid format"),
			expected: "config format_error: invalid format",
		},
		{
			name:     "validation error with field",
			err:      NewConfigErrorWithField(ConfigErrorTypeValidation, "version", "required", ""),
			expected: "config validation_error: field version: required",
		},
		{
			name:     "file error with cause",
			err:      NewConfigErrorWithCause(ConfigErrorTypeFile, "file not found", fmt.Errorf("ENOENT")),
			expected: "config file_error: file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
			if tt.err.Err != nil {
				assert.Equal(t, tt.err.Err, tt.err.Unwrap())
			}
		})
	}
}
