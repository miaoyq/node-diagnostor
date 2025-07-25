package config

import (
	"context"

	"go.uber.org/zap"
)

// ConfigSubscriber 配置订阅者接口
type ConfigSubscriber interface {
	// OnConfigUpdate 当配置更新时调用
	OnConfigUpdate(newConfig *Config) error
}

// ExtendedConfigManager 扩展配置管理器以支持订阅者模式
type ExtendedConfigManager struct {
	*ConfigManager
	subscribers []ConfigSubscriber
	logger      *zap.Logger
}

// NewExtendedConfigManager 创建扩展的配置管理器
func NewExtendedConfigManager(logger *zap.Logger) *ExtendedConfigManager {
	return &ExtendedConfigManager{
		ConfigManager: NewConfigManager(logger),
		subscribers:   make([]ConfigSubscriber, 0),
		logger:        logger,
	}
}

// SubscribeModule 添加模块订阅者
func (ecm *ExtendedConfigManager) SubscribeModule(subscriber ConfigSubscriber) error {
	ecm.subscribers = append(ecm.subscribers, subscriber)
	return nil
}

// notifySubscribers 通知所有订阅者配置已变更
func (ecm *ExtendedConfigManager) notifySubscribers(config *Config) {
	for _, subscriber := range ecm.subscribers {
		go func(s ConfigSubscriber) {
			if err := s.OnConfigUpdate(config); err != nil {
				// 记录错误但不中断其他模块的更新
				ecm.logger.Error("Module config update failed", zap.Error(err))
			}
		}(subscriber)
	}
}

// Override the Load method to include subscriber notification
func (ecm *ExtendedConfigManager) Load(ctx context.Context, filePath string) error {
	err := ecm.ConfigManager.Load(ctx, filePath)
	if err == nil {
		// 通知所有订阅者
		ecm.notifySubscribers(ecm.Get())
	}
	return err
}
