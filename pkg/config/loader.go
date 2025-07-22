package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Config struct {
	MetricsInterval time.Duration `json:"metrics_interval"`
	JournalUnits    []string      `json:"journal_units"`
	CheckConfigs    []CheckConfig `json:"check_configs"`
	ReportURL       string        `json:"report_url"`
	CacheTTL        time.Duration `json:"cache_ttl"`
}

type CheckConfig struct {
	Name   string            `json:"name"`
	Enable bool              `json:"enable"`
	Params map[string]string `json:"params"`
}

// 自定义时间解析
func (c *Config) UnmarshalJSON(data []byte) error {
	type Alias Config
	aux := &struct {
		MetricsInterval string `json:"metrics_interval"`
		CacheTTL        string `json:"cache_ttl"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// 解析时间间隔
	if aux.MetricsInterval != "" {
		duration, err := time.ParseDuration(aux.MetricsInterval)
		if err != nil {
			return fmt.Errorf("解析metrics_interval失败: %v", err)
		}
		c.MetricsInterval = duration
	} else {
		c.MetricsInterval = 5 * time.Second // 默认值
	}

	// 解析缓存TTL
	if aux.CacheTTL != "" {
		duration, err := time.ParseDuration(aux.CacheTTL)
		if err != nil {
			return fmt.Errorf("解析cache_ttl失败: %v", err)
		}
		c.CacheTTL = duration
	} else {
		c.CacheTTL = 1 * time.Minute // 默认值
	}

	return nil
}

func LoadConfig(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Loader 配置加载器
type Loader struct {
	filePath string
	onChange func()
	watcher  *fsnotify.Watcher
}

// NewLoader 创建配置加载器
func NewLoader(filePath string, onChange func()) (*Loader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	loader := &Loader{
		filePath: filePath,
		onChange: onChange,
		watcher:  watcher,
	}

	// 添加配置文件监听
	if err := watcher.Add(filePath); err != nil {
		return nil, err
	}

	log.Printf("开始监听配置文件变化: %s", filePath)

	// 启动监听协程
	go loader.watchChanges()

	return loader, nil
}

func (l *Loader) watchChanges() {
	for {
		select {
		case event, ok := <-l.watcher.Events:
			if !ok {
				return
			}
			log.Printf("配置文件事件: %v", event)

			// 处理多种文件变化事件
			shouldReload := false
			if event.Op&fsnotify.Write == fsnotify.Write {
				shouldReload = true
			}
			if event.Op&fsnotify.Create == fsnotify.Create {
				shouldReload = true
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				// 文件被删除，可能是sed的临时操作，等待文件重新出现
				time.Sleep(100 * time.Millisecond)
				if _, err := os.Stat(l.filePath); err == nil {
					// 文件重新出现，重新添加监听
					l.watcher.Add(l.filePath)
					shouldReload = true
				}
			}

			if shouldReload {
				log.Println("检测到配置文件修改")
				// 延迟加载，避免文件写入不完整
				time.Sleep(1 * time.Second)
				l.onChange()
				log.Println("配置已重新加载")
			}
		case err, ok := <-l.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("配置监听错误: %v", err)
		}
	}
}

// Close 关闭监听器
func (l *Loader) Close() error {
	log.Println("关闭配置监听器")
	return l.watcher.Close()
}
