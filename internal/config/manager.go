package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// ConfigManager 实现配置管理接口
type ConfigManager struct {
	mu           sync.RWMutex
	config       *Config
	filePath     string
	watcher      *fsnotify.Watcher
	callbacks    []func(*Config)
	lastReload   time.Time
	errorHandler *ConfigErrorHandler
	logger       *zap.Logger
}

// NewConfigManager 创建新的配置管理器
func NewConfigManager(logger *zap.Logger) *ConfigManager {
	return &ConfigManager{
		config:       DefaultConfig(),
		callbacks:    make([]func(*Config), 0),
		errorHandler: NewConfigErrorHandler(logger, "config/backups"),
		logger:       logger,
	}
}

// Load 从文件加载配置
func (cm *ConfigManager) Load(ctx context.Context, filePath string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			cm.logger.Warn("Config file not found, creating default configuration", zap.String("path", filePath))
			return cm.createDefaultConfig(filePath)
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var newConfig Config
	if err := json.Unmarshal(data, &newConfig); err != nil {
		cm.logger.Error("Failed to parse config file, attempting error recovery", zap.Error(err))
		if recoveredConfig, handleErr := cm.errorHandler.HandleError(ctx,
			NewConfigErrorWithCause(ConfigErrorTypeFormat, "invalid JSON format", err), filePath); handleErr == nil {
			newConfig = *recoveredConfig
		} else {
			return fmt.Errorf("failed to recover from format error: %w", handleErr)
		}
	}

	if err := cm.validateConfig(&newConfig); err != nil {
		cm.logger.Error("Config validation failed, attempting error recovery", zap.Error(err))
		if recoveredConfig, handleErr := cm.errorHandler.HandleError(ctx, err, filePath); handleErr == nil {
			newConfig = *recoveredConfig
		} else {
			return fmt.Errorf("failed to recover from validation error: %w", handleErr)
		}
	}

	cm.config = &newConfig
	cm.filePath = filePath
	cm.lastReload = time.Now()

	// 通知所有订阅者
	cm.notifySubscribers(&newConfig)

	cm.logger.Info("Configuration loaded successfully",
		zap.String("path", filePath),
		zap.Int("checks", len(newConfig.Checks)))

	return nil
}

// createDefaultConfig 创建默认配置文件
func (cm *ConfigManager) createDefaultConfig(filePath string) error {
	defaultConfig := DefaultConfig()

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// 保存默认配置
	data, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal default config: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write default config: %w", err)
	}

	cm.config = defaultConfig
	cm.filePath = filePath
	cm.lastReload = time.Now()

	cm.logger.Info("Default configuration created", zap.String("path", filePath))
	return nil
}

// Get 返回当前配置
func (cm *ConfigManager) Get() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
}

// Watch 开始监控配置文件变化
func (cm *ConfigManager) Watch(ctx context.Context) error {
	if cm.filePath == "" {
		return fmt.Errorf("no config file path set")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	cm.watcher = watcher

	if err := watcher.Add(cm.filePath); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to watch config file: %w", err)
	}

	go cm.handleFileChanges(ctx)

	return nil
}

// handleFileChanges 处理文件变化事件
func (cm *ConfigManager) handleFileChanges(ctx context.Context) {
	defer cm.watcher.Close()

	debounceTimer := time.NewTimer(0)
	if !debounceTimer.Stop() {
		<-debounceTimer.C
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-cm.watcher.Events:
			if !ok {
				return
			}

			// 只处理写入和重命名事件
			if event.Op&(fsnotify.Write|fsnotify.Rename|fsnotify.Create) == 0 {
				continue
			}

			// 防抖处理，避免频繁重载
			debounceTimer.Reset(1 * time.Second)
			select {
			case <-debounceTimer.C:
				if err := cm.Reload(ctx); err != nil {
					cm.logger.Error("Config reload failed", zap.Error(err))
				} else {
					cm.logger.Info("Config reloaded successfully", zap.Time("at", time.Now()))
				}
			case <-ctx.Done():
				return
			}
		case err, ok := <-cm.watcher.Errors:
			if !ok {
				return
			}
			cm.logger.Error("File watcher error", zap.Error(err))
		}
	}
}

// Reload 重新加载配置
func (cm *ConfigManager) Reload(ctx context.Context) error {
	if cm.filePath == "" {
		return fmt.Errorf("no config file path set")
	}

	// 检查文件是否被修改
	info, err := os.Stat(cm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			cm.logger.Warn("Config file disappeared, recreating default")
			return cm.createDefaultConfig(cm.filePath)
		}
		return fmt.Errorf("failed to stat config file: %w", err)
	}

	if info.ModTime().Before(cm.lastReload) {
		return nil // 文件未修改
	}

	return cm.Load(ctx, cm.filePath)
}

// Validate 验证当前配置
func (cm *ConfigManager) Validate() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.validateConfig(cm.config)
}

// validateConfig 验证配置有效性
func (cm *ConfigManager) validateConfig(config *Config) error {
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

// Subscribe 添加配置变更回调
func (cm *ConfigManager) Subscribe(callback func(*Config)) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.callbacks = append(cm.callbacks, callback)
	return nil
}

// Unsubscribe 移除配置变更回调
func (cm *ConfigManager) Unsubscribe(callback func(*Config)) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, cb := range cm.callbacks {
		if &cb == &callback {
			cm.callbacks = append(cm.callbacks[:i], cm.callbacks[i+1:]...)
			break
		}
	}
	return nil
}

// notifySubscribers 通知所有订阅者配置已变更
func (cm *ConfigManager) notifySubscribers(config *Config) {
	for _, callback := range cm.callbacks {
		go callback(config)
	}
}

// Close 关闭配置管理器
func (cm *ConfigManager) Close() error {
	if cm.watcher != nil {
		return cm.watcher.Close()
	}
	return nil
}
