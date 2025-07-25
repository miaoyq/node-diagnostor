package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// ConfigErrorType 配置错误类型
type ConfigErrorType string

const (
	ConfigErrorTypeFormat     ConfigErrorType = "format_error"     // 格式错误
	ConfigErrorTypeValidation ConfigErrorType = "validation_error" // 验证错误
	ConfigErrorTypeFile       ConfigErrorType = "file_error"       // 文件错误
	ConfigErrorTypeFallback   ConfigErrorType = "fallback_error"   // 回退错误
)

// ConfigError 配置错误结构
type ConfigError struct {
	Type    ConfigErrorType
	Message string
	Field   string
	Value   interface{}
	Err     error
}

// Error 实现error接口
func (e *ConfigError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("config %s: field %s: %s", e.Type, e.Field, e.Message)
	}
	return fmt.Sprintf("config %s: %s", e.Type, e.Message)
}

// Unwrap 实现错误包装接口
func (e *ConfigError) Unwrap() error {
	return e.Err
}

// ConfigErrorHandler 配置错误处理器
type ConfigErrorHandler struct {
	logger       *zap.Logger
	backupDir    string
	maxBackups   int
	fallbackPath string
}

// NewConfigErrorHandler 创建新的配置错误处理器
func NewConfigErrorHandler(logger *zap.Logger, backupDir string) *ConfigErrorHandler {
	return &ConfigErrorHandler{
		logger:       logger,
		backupDir:    backupDir,
		maxBackups:   5,
		fallbackPath: filepath.Join(backupDir, "fallback.json"),
	}
}

// HandleError 处理配置错误
func (h *ConfigErrorHandler) HandleError(ctx context.Context, err error, configPath string) (*Config, error) {
	var configErr *ConfigError
	if errors.As(err, &configErr) {
		return h.handleTypedError(ctx, configErr, configPath)
	}

	// 处理普通错误
	h.logger.Error("Unknown config error", zap.Error(err))
	return h.createFallbackConfig(ctx, configPath)
}

// handleTypedError 处理特定类型的配置错误
func (h *ConfigErrorHandler) handleTypedError(ctx context.Context, err *ConfigError, configPath string) (*Config, error) {
	h.logger.Error("Configuration error detected",
		zap.String("type", string(err.Type)),
		zap.String("field", err.Field),
		zap.Any("value", err.Value),
		zap.Error(err.Err))

	switch err.Type {
	case ConfigErrorTypeFormat:
		return h.handleFormatError(ctx, configPath, err)
	case ConfigErrorTypeValidation:
		return h.handleValidationError(ctx, configPath, err)
	case ConfigErrorTypeFile:
		return h.handleFileError(ctx, configPath, err)
	case ConfigErrorTypeFallback:
		return h.handleFallbackError(ctx, configPath, err)
	default:
		return h.createFallbackConfig(ctx, configPath)
	}
}

// handleFormatError 处理格式错误
func (h *ConfigErrorHandler) handleFormatError(ctx context.Context, configPath string, err *ConfigError) (*Config, error) {
	h.logger.Warn("Configuration format error, attempting to backup and create fallback")

	// 备份有问题的配置文件
	if err := h.backupConfig(configPath); err != nil {
		h.logger.Error("Failed to backup config file", zap.Error(err))
	}

	// 尝试修复格式错误
	if fixedConfig, fixErr := h.attemptFormatFix(configPath); fixErr == nil {
		h.logger.Info("Successfully fixed configuration format")
		return fixedConfig, nil
	}

	// 创建回退配置
	return h.createFallbackConfig(ctx, configPath)
}

// handleValidationError 处理验证错误
func (h *ConfigErrorHandler) handleValidationError(ctx context.Context, configPath string, err *ConfigError) (*Config, error) {
	h.logger.Warn("Configuration validation error, attempting to apply defaults")

	// 尝试加载原始配置并修复验证错误
	config, loadErr := h.loadAndFixConfig(configPath)
	if loadErr == nil {
		h.logger.Info("Successfully fixed configuration validation errors")
		return config, nil
	}

	// 创建回退配置
	return h.createFallbackConfig(ctx, configPath)
}

// handleFileError 处理文件错误
func (h *ConfigErrorHandler) handleFileError(ctx context.Context, configPath string, err *ConfigError) (*Config, error) {
	h.logger.Warn("Configuration file error, checking for backup")

	// 检查是否存在回退配置
	if _, statErr := os.Stat(h.fallbackPath); statErr == nil {
		h.logger.Info("Loading fallback configuration")
		return h.loadFallbackConfig()
	}

	// 创建新的回退配置
	return h.createFallbackConfig(ctx, configPath)
}

// handleFallbackError 处理回退错误
func (h *ConfigErrorHandler) handleFallbackError(ctx context.Context, configPath string, err *ConfigError) (*Config, error) {
	h.logger.Error("Fallback configuration error, using default configuration")

	// 使用默认配置
	defaultConfig := DefaultConfig()

	// 保存默认配置为回退配置
	if err := h.saveFallbackConfig(defaultConfig); err != nil {
		h.logger.Error("Failed to save fallback configuration", zap.Error(err))
	}

	return defaultConfig, nil
}

// backupConfig 备份配置文件
func (h *ConfigErrorHandler) backupConfig(configPath string) error {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil // 文件不存在，无需备份
	}

	// 确保备份目录存在
	if err := os.MkdirAll(h.backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// 创建带时间戳的备份文件名
	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(h.backupDir, fmt.Sprintf("config-%s.json", timestamp))

	// 复制文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	// 清理旧备份
	h.cleanupOldBackups()

	h.logger.Info("Configuration backed up", zap.String("backup_path", backupPath))
	return nil
}

// cleanupOldBackups 清理旧备份文件
func (h *ConfigErrorHandler) cleanupOldBackups() {
	files, err := filepath.Glob(filepath.Join(h.backupDir, "config-*.json"))
	if err != nil {
		h.logger.Error("Failed to list backup files", zap.Error(err))
		return
	}

	if len(files) <= h.maxBackups {
		return
	}

	// 按修改时间排序
	type fileInfo struct {
		path    string
		modTime time.Time
	}

	fileInfos := make([]fileInfo, 0, len(files))
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		fileInfos = append(fileInfos, fileInfo{file, info.ModTime()})
	}

	// 简单的冒泡排序
	for i := 0; i < len(fileInfos)-1; i++ {
		for j := i + 1; j < len(fileInfos); j++ {
			if fileInfos[i].modTime.Before(fileInfos[j].modTime) {
				fileInfos[i], fileInfos[j] = fileInfos[j], fileInfos[i]
			}
		}
	}

	// 删除最旧的备份
	for i := h.maxBackups; i < len(fileInfos); i++ {
		if err := os.Remove(fileInfos[i].path); err != nil {
			h.logger.Error("Failed to remove old backup", zap.String("path", fileInfos[i].path), zap.Error(err))
		}
	}
}

// attemptFormatFix 尝试修复格式错误
func (h *ConfigErrorHandler) attemptFormatFix(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// 尝试修复常见的JSON格式错误
	fixedData := h.fixCommonJSONErrors(data)

	var config Config
	if err := json.Unmarshal(fixedData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse fixed config: %w", err)
	}

	// 验证修复后的配置
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("fixed config validation failed: %w", err)
	}

	// 保存修复后的配置
	if err := os.WriteFile(configPath, fixedData, 0644); err != nil {
		h.logger.Error("Failed to save fixed config", zap.Error(err))
	}

	return &config, nil
}

// fixCommonJSONErrors 修复常见的JSON格式错误
func (h *ConfigErrorHandler) fixCommonJSONErrors(data []byte) []byte {
	// 这里可以实现一些简单的JSON修复逻辑
	// 例如：修复缺失的引号、逗号等
	// 目前返回原始数据，后续可以添加更复杂的修复逻辑
	return data
}

// loadAndFixConfig 加载并修复配置
func (h *ConfigErrorHandler) loadAndFixConfig(configPath string) (*Config, error) {
	data, errRead := os.ReadFile(configPath)
	if errRead != nil {
		return nil, fmt.Errorf("failed to read config file: %w", errRead)
	}

	var config Config
	if errUnmarshal := json.Unmarshal(data, &config); errUnmarshal != nil {
		return nil, fmt.Errorf("failed to parse config: %w", errUnmarshal)
	}

	// 全面修复所有可能的验证错误
	config = h.applyAllValidationFixes(config)

	// 验证修复后的配置
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("config still invalid after fixes: %w", err)
	}

	// 保存修复后的配置
	fixedData, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(configPath, fixedData, 0644); err != nil {
		h.logger.Error("Failed to save fixed config", zap.Error(err))
	}

	return &config, nil
}

// applyAllValidationFixes 全面应用所有验证修复
func (h *ConfigErrorHandler) applyAllValidationFixes(config Config) Config {
	// 修复版本号
	if config.Version == "" {
		config.Version = "1.0"
	}

	// 修复检查配置
	if len(config.Checks) == 0 {
		config.Checks = DefaultConfig().Checks
	}

	// 修复每个检查的配置
	for i := range config.Checks {
		check := &config.Checks[i]
		if check.Interval <= 0 {
			check.Interval = 5 * time.Minute
		}
		if check.Timeout <= 0 {
			check.Timeout = 30 * time.Second
		}
		if check.Mode != "仅本地" && check.Mode != "集群补充" && check.Mode != "自动选择" {
			check.Mode = "自动选择"
		}
		if len(check.Collectors) == 0 {
			check.Collectors = []string{"cpu", "memory", "disk", "network"}
		}
	}

	// 修复资源限制
	if config.ResourceLimits.MaxCPU <= 0 || config.ResourceLimits.MaxCPU > 100 {
		config.ResourceLimits.MaxCPU = 5.0
	}
	if config.ResourceLimits.MaxMemory <= 0 {
		config.ResourceLimits.MaxMemory = 100
	}
	if config.ResourceLimits.MaxConcurrent <= 0 {
		config.ResourceLimits.MaxConcurrent = 10
	}

	// 修复数据范围
	if config.DataScope.Mode != "仅本地" && config.DataScope.Mode != "集群补充" && config.DataScope.Mode != "自动选择" {
		config.DataScope.Mode = "自动选择"
	}

	// 修复报告器配置
	if config.Reporter.Endpoint == "" {
		config.Reporter.Endpoint = "http://localhost:8080/api/v1/diagnostics"
	}
	if config.Reporter.Timeout <= 0 {
		config.Reporter.Timeout = 30 * time.Second
	}
	if config.Reporter.MaxRetries < 0 {
		config.Reporter.MaxRetries = 3
	}

	return config
}

// createFallbackConfig 创建回退配置
func (h *ConfigErrorHandler) createFallbackConfig(ctx context.Context, configPath string) (*Config, error) {
	h.logger.Info("Creating fallback configuration")

	fallbackConfig := DefaultConfig()

	// 保存回退配置
	if err := h.saveFallbackConfig(fallbackConfig); err != nil {
		h.logger.Error("Failed to save fallback config", zap.Error(err))
	}

	return fallbackConfig, nil
}

// saveFallbackConfig 保存回退配置
func (h *ConfigErrorHandler) saveFallbackConfig(config *Config) error {
	if err := os.MkdirAll(h.backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal fallback config: %w", err)
	}

	if err := os.WriteFile(h.fallbackPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write fallback config: %w", err)
	}

	h.logger.Info("Fallback configuration saved", zap.String("path", h.fallbackPath))
	return nil
}

// loadFallbackConfig 加载回退配置
func (h *ConfigErrorHandler) loadFallbackConfig() (*Config, error) {
	data, err := os.ReadFile(h.fallbackPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read fallback config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse fallback config: %w", err)
	}

	h.logger.Info("Fallback configuration loaded", zap.String("path", h.fallbackPath))
	return &config, nil
}

// NewConfigError 创建新的配置错误
func NewConfigError(errorType ConfigErrorType, message string) *ConfigError {
	return &ConfigError{
		Type:    errorType,
		Message: message,
	}
}

// NewConfigErrorWithField 创建带字段的配置错误
func NewConfigErrorWithField(errorType ConfigErrorType, field string, message string, value interface{}) *ConfigError {
	return &ConfigError{
		Type:    errorType,
		Field:   field,
		Message: message,
		Value:   value,
	}
}

// NewConfigErrorWithCause 创建带原因的配置错误
func NewConfigErrorWithCause(errorType ConfigErrorType, message string, cause error) *ConfigError {
	return &ConfigError{
		Type:    errorType,
		Message: message,
		Err:     cause,
	}
}

// validateConfig 验证配置（独立函数，供错误处理器使用）
func validateConfig(config *Config) error {
	if config.Version == "" {
		return NewConfigErrorWithField(ConfigErrorTypeValidation, "version", "version is required", config.Version)
	}

	if len(config.Checks) == 0 {
		return NewConfigErrorWithField(ConfigErrorTypeValidation, "checks", "at least one check must be configured", len(config.Checks))
	}

	for i, check := range config.Checks {
		if check.Name == "" {
			return NewConfigErrorWithField(ConfigErrorTypeValidation, fmt.Sprintf("checks[%d].name", i), "name is required", check.Name)
		}
		if check.Interval <= 0 {
			return NewConfigErrorWithField(ConfigErrorTypeValidation, fmt.Sprintf("checks[%s].interval", check.Name), "interval must be positive", check.Interval)
		}
		if check.Timeout <= 0 {
			return NewConfigErrorWithField(ConfigErrorTypeValidation, fmt.Sprintf("checks[%s].timeout", check.Name), "timeout must be positive", check.Timeout)
		}
		if len(check.Collectors) == 0 {
			return NewConfigErrorWithField(ConfigErrorTypeValidation, fmt.Sprintf("checks[%s].collectors", check.Name), "at least one collector is required", len(check.Collectors))
		}
		if check.Mode != "仅本地" && check.Mode != "集群补充" && check.Mode != "自动选择" {
			return NewConfigErrorWithField(ConfigErrorTypeValidation, fmt.Sprintf("checks[%s].mode", check.Name), "invalid mode, must be one of: 仅本地, 集群补充, 自动选择", check.Mode)
		}
	}

	if config.ResourceLimits.MaxCPU <= 0 || config.ResourceLimits.MaxCPU > 100 {
		return NewConfigErrorWithField(ConfigErrorTypeValidation, "resource_limits.max_cpu_percent", "max_cpu_percent must be between 0 and 100", config.ResourceLimits.MaxCPU)
	}
	if config.ResourceLimits.MaxMemory <= 0 {
		return NewConfigErrorWithField(ConfigErrorTypeValidation, "resource_limits.max_memory_mb", "max_memory_mb must be positive", config.ResourceLimits.MaxMemory)
	}
	if config.ResourceLimits.MaxConcurrent <= 0 {
		return NewConfigErrorWithField(ConfigErrorTypeValidation, "resource_limits.max_concurrent", "max_concurrent must be positive", config.ResourceLimits.MaxConcurrent)
	}

	if config.DataScope.Mode != "仅本地" && config.DataScope.Mode != "集群补充" && config.DataScope.Mode != "自动选择" {
		return NewConfigErrorWithField(ConfigErrorTypeValidation, "data_scope.mode", "mode must be one of: 仅本地, 集群补充, 自动选择", config.DataScope.Mode)
	}

	if config.Reporter.Endpoint == "" {
		return NewConfigErrorWithField(ConfigErrorTypeValidation, "reporter.endpoint", "endpoint is required", config.Reporter.Endpoint)
	}
	if config.Reporter.Timeout <= 0 {
		return NewConfigErrorWithField(ConfigErrorTypeValidation, "reporter.timeout", "timeout must be positive", config.Reporter.Timeout)
	}
	if config.Reporter.MaxRetries < 0 {
		return NewConfigErrorWithField(ConfigErrorTypeValidation, "reporter.max_retries", "max_retries must be non-negative", config.Reporter.MaxRetries)
	}

	return nil
}
