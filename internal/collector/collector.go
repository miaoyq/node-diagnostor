package collector

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Data represents collected diagnostic data
type Data struct {
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"`
	Data      map[string]interface{} `json:"data"`
	Metadata  map[string]string      `json:"metadata"`
}

// Result represents the result of a collection operation
type Result struct {
	Data    *Data
	Error   error
	Skipped bool
	Reason  string
}

// Collector defines the interface for all data collectors
type Collector interface {
	// Name returns the collector's unique identifier
	Name() string

	// Collect performs the data collection
	Collect(ctx context.Context, params map[string]interface{}) (*Result, error)

	// Validate validates the collector's parameters
	Validate(params map[string]interface{}) error

	// IsLocalOnly indicates if this collector provides node-local data only
	IsLocalOnly() bool

	// IsClusterSupplement indicates if this collector provides cluster-level data
	IsClusterSupplement() bool

	// Priority returns the collector's priority level
	Priority() int

	// Timeout returns the recommended timeout for this collector
	Timeout() time.Duration
}

// Registry manages collector registration and lookup
type Registry interface {
	// Register adds a new collector to the registry
	Register(collector Collector) error

	// Get retrieves a collector by name
	Get(name string) (Collector, error)

	// List returns all registered collectors
	List() []Collector

	// GetByType returns collectors filtered by data type
	GetByType(dataType string) []Collector

	// GetLocalOnly returns all local-only collectors
	GetLocalOnly() []Collector

	// GetClusterSupplement returns all cluster supplement collectors
	GetClusterSupplement() []Collector
}

// collectorRegistry implements the Registry interface
type collectorRegistry struct {
	mu         sync.RWMutex
	collectors map[string]Collector
}

// NewRegistry creates a new collector registry
func NewRegistry() Registry {
	return &collectorRegistry{
		collectors: make(map[string]Collector),
	}
}

// Register adds a new collector to the registry
func (r *collectorRegistry) Register(collector Collector) error {
	if collector == nil {
		return errors.New("collector cannot be nil")
	}

	name := collector.Name()
	if name == "" {
		return errors.New("collector name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.collectors[name]; exists {
		return errors.New("collector already registered: " + name)
	}

	r.collectors[name] = collector
	return nil
}

// Get retrieves a collector by name
func (r *collectorRegistry) Get(name string) (Collector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	collector, exists := r.collectors[name]
	if !exists {
		return nil, errors.New("collector not found: " + name)
	}

	return collector, nil
}

// List returns all registered collectors
func (r *collectorRegistry) List() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Collector, 0, len(r.collectors))
	for _, collector := range r.collectors {
		list = append(list, collector)
	}

	return list
}

// GetByType returns collectors filtered by data type
func (r *collectorRegistry) GetByType(dataType string) []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Collector
	for _, collector := range r.collectors {
		if collector.Name() == dataType || collector.Name() == dataType+"-collector" {
			result = append(result, collector)
		}
	}

	return result
}

// GetLocalOnly returns all local-only collectors
func (r *collectorRegistry) GetLocalOnly() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Collector
	for _, collector := range r.collectors {
		if collector.IsLocalOnly() {
			result = append(result, collector)
		}
	}

	return result
}

// GetClusterSupplement returns all cluster supplement collectors
func (r *collectorRegistry) GetClusterSupplement() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Collector
	for _, collector := range r.collectors {
		if collector.IsClusterSupplement() {
			result = append(result, collector)
		}
	}

	return result
}

// Manager handles collector lifecycle management
type Manager struct {
	registry Registry
	mu       sync.RWMutex
	active   map[string]context.CancelFunc
}

// NewManager creates a new collector manager
func NewManager(registry Registry) *Manager {
	return &Manager{
		registry: registry,
		active:   make(map[string]context.CancelFunc),
	}
}

// StartCollector starts a collector with the given parameters
func (m *Manager) StartCollector(ctx context.Context, name string, params map[string]interface{}) error {
	collector, err := m.registry.Get(name)
	if err != nil {
		return err
	}

	if err := collector.Validate(params); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.active[name]; exists {
		return errors.New("collector already running: " + name)
	}

	ctx, cancel := context.WithCancel(ctx)
	m.active[name] = cancel

	// 启动异步收集
	go func() {
		defer func() {
			// 清理：从active中移除
			m.mu.Lock()
			delete(m.active, name)
			m.mu.Unlock()
		}()

		// 设置超时
		timeout := collector.Timeout()
		if timeout > 0 {
			var cancelTimeout context.CancelFunc
			ctx, cancelTimeout = context.WithTimeout(ctx, timeout)
			defer cancelTimeout()
		}

		// 执行收集
		result, err := collector.Collect(ctx, params)

		// 这里可以添加结果处理逻辑
		// 例如：发送到结果通道、记录日志等
		if err != nil {
			// 记录错误日志
			// log.Printf("Collector %s failed: %v", name, err)
		} else if result != nil && !result.Skipped {
			// 处理收集到的数据
			// 例如：发送到分析器或存储
			// if result.Data != nil {
			//     // 处理数据
			// }
		}
	}()

	return nil
}

// StopCollector stops a running collector
func (m *Manager) StopCollector(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cancel, exists := m.active[name]
	if !exists {
		return errors.New("collector not running: " + name)
	}

	cancel()
	delete(m.active, name)
	return nil
}

// IsRunning checks if a collector is currently running
func (m *Manager) IsRunning(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.active[name]
	return exists
}

// ListRunning returns names of all running collectors
func (m *Manager) ListRunning() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.active))
	for name := range m.active {
		names = append(names, name)
	}

	return names
}

// StopAll stops all running collectors
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, cancel := range m.active {
		cancel()
		delete(m.active, name)
	}
}
