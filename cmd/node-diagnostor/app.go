package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/miaoyq/node-diagnostor/internal/collector"
	"github.com/miaoyq/node-diagnostor/internal/config"
	"github.com/miaoyq/node-diagnostor/internal/datascope"
	"github.com/miaoyq/node-diagnostor/internal/processor"
	"github.com/miaoyq/node-diagnostor/internal/reporter"
	"github.com/miaoyq/node-diagnostor/internal/scheduler"
	"go.uber.org/zap"
)

// App represents the main application
type App struct {
	configManager     *config.ExtendedConfigManager
	scheduler         scheduler.Scheduler
	validator         datascope.Validator
	collectorRegistry collector.Registry
	processor         processor.Processor
	reporter          *reporter.ReporterClient
	cache             *reporter.CacheManager
	ctx               context.Context
	cancel            context.CancelFunc
	logger            *zap.Logger
}

// NewApp creates a new application instance
func NewApp() *App {
	ctx, cancel := context.WithCancel(context.Background())
	logger, _ := zap.NewProduction()
	return &App{
		ctx:           ctx,
		cancel:        cancel,
		configManager: config.NewExtendedConfigManager(logger),
		logger:        logger,
	}
}

// Initialize initializes the application components
func (a *App) Initialize() error {
	// Initialize configuration manager
	if err := a.configManager.Load(a.ctx, "/etc/node-diagnostor/config.json"); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Get configuration
	cfg := a.configManager.Get()
	if cfg == nil {
		return fmt.Errorf("no configuration loaded")
	}

	// Initialize validator
	a.validator = datascope.NewValidator()

	// Initialize collector registry
	a.collectorRegistry = collector.NewRegistry()

	// Register collectors based on configuration
	// Register node-local collectors
	if err := collector.RegisterNodeLocalCollectors(a.collectorRegistry); err != nil {
		return fmt.Errorf("failed to register node-local collectors: %w", err)
	}

	// Register cluster supplement collectors
	if err := collector.RegisterClusterSupplementCollectors(a.collectorRegistry); err != nil {
		return fmt.Errorf("failed to register cluster supplement collectors: %w", err)
	}

	// Initialize scheduler
	executor := scheduler.NewCheckExecutor(a.collectorRegistry, a.validator)

	// Initialize mointor
	// TODO: move to scheduler package
	monitor := scheduler.NewTaskMonitor()

	// Initialize scheduler
	a.scheduler = scheduler.New(executor, monitor, a.logger.With(zap.String("module", "scheduler")))

	// Configure scheduler with resource limits from config
	if cfg.ResourceLimits.MaxConcurrent > 0 {
		a.scheduler.SetMaxConcurrent(cfg.ResourceLimits.MaxConcurrent)
	}

	// Initialize processor
	a.processor = processor.NewDataProcessor()

	// Initialize reporter using configuration
	a.reporter = reporter.NewReporterClient(cfg.Reporter, a.logger.With(zap.String("module", "reporter")))

	// Set processor and reporter for scheduler
	if schedulerImpl, ok := a.scheduler.(*scheduler.CheckScheduler); ok {
		schedulerImpl.SetProcessor(a.processor)
		schedulerImpl.SetReporter(a.reporter)
	}

	// Add tasks from configuration
	if err := a.addTasksFromConfig(cfg); err != nil {
		return fmt.Errorf("failed to add tasks from config: %w", err)
	}

	// Subscribe modules to configuration changes
	if err := a.subscribeModules(); err != nil {
		return fmt.Errorf("failed to subscribe modules: %w", err)
	}

	return nil
}

// subscribeModules subscribes all modules to configuration changes
func (a *App) subscribeModules() error {
	// Subscribe scheduler
	if err := a.configManager.SubscribeModule(a.scheduler); err != nil {
		return fmt.Errorf("failed to subscribe scheduler: %w", err)
	}

	// Subscribe reporter
	if err := a.configManager.SubscribeModule(a.reporter); err != nil {
		return fmt.Errorf("failed to subscribe reporter: %w", err)
	}

	// Note: CacheManager is managed by reporter, so it will be updated via reporter

	a.logger.Info("All modules subscribed to configuration changes")
	return nil
}

// addTasksFromConfig adds tasks based on configuration
func (a *App) addTasksFromConfig(cfg *config.Config) error {
	addedTasks := 0
	for _, check := range cfg.Checks {
		if !check.Enabled {
			continue
		}

		// Create a task for each collector in the check
		for _, collectorID := range check.Collectors {
			taskID := fmt.Sprintf("%s-%s", check.Name, collectorID)
			task := &scheduler.Task{
				ID:         taskID,
				Name:       check.Name,
				Collector:  collectorID,
				Parameters: check.Params,
				Interval:   check.Interval,
				Priority:   check.Priority,
				Timeout:    check.Timeout,
				Enabled:    true,
				MaxRetries: 3,
				Status:     scheduler.TaskStatusPending,
			}

			if err := a.scheduler.Add(a.ctx, task); err != nil {
				return fmt.Errorf("failed to add task %s: %w", taskID, err)
			}
			addedTasks++
		}
	}

	a.logger.Info("Tasks added from configuration", zap.Int("task_count", addedTasks))
	return nil
}

// Start starts the application
func (a *App) Start() error {
	a.logger.Info("Starting K8s Node Diagnostor...")

	// Start configuration watching
	if err := a.configManager.Watch(a.ctx); err != nil {
		return fmt.Errorf("failed to start config watcher: %w", err)
	}

	// Start reporter
	if err := a.reporter.Start(a.ctx); err != nil {
		return fmt.Errorf("failed to start reporter: %w", err)
	}

	// Start scheduler
	if err := a.scheduler.Start(a.ctx); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	a.logger.Info("K8s Node Diagnostor started successfully")
	return nil
}

// Stop gracefully stops the application
func (a *App) Stop() error {
	a.logger.Info("Stopping K8s Node Diagnostor...")

	a.cancel()

	// Stop scheduler
	if err := a.scheduler.Stop(a.ctx); err != nil {
		return fmt.Errorf("failed to stop scheduler: %w", err)
	}

	// Stop reporter
	if a.reporter != nil {
		a.reporter.Stop()
	}

	// Close config manager
	if err := a.configManager.Close(); err != nil {
		return fmt.Errorf("failed to close config manager: %w", err)
	}

	a.logger.Info("K8s Node Diagnostor stopped successfully")
	return nil
}

// Run runs the application
func (a *App) Run() error {
	if err := a.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize app: %w", err)
	}

	if err := a.Start(); err != nil {
		return fmt.Errorf("failed to start app: %w", err)
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	a.logger.Info("Received interrupt signal")

	return a.Stop()
}
