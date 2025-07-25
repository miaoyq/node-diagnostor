package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/miaoyq/node-diagnostor/internal/collector"
	"github.com/miaoyq/node-diagnostor/internal/config"
	"github.com/miaoyq/node-diagnostor/internal/processor"
	"github.com/miaoyq/node-diagnostor/internal/scheduler"
	"github.com/miaoyq/node-diagnostor/internal/validator"
	"go.uber.org/zap"
)

// App represents the main application
type App struct {
	configManager *config.ConfigManager
	scheduler     scheduler.Scheduler
	validator     validator.Validator
	collector     collector.Registry
	processor     processor.Processor
	reporter      processor.Reporter
	cache         processor.Cache
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewApp creates a new application instance
func NewApp() *App {
	ctx, cancel := context.WithCancel(context.Background())
	logger, _ := zap.NewProduction()
	return &App{
		ctx:           ctx,
		cancel:        cancel,
		configManager: config.NewConfigManager(logger),
	}
}

// Initialize initializes the application components
func (a *App) Initialize() error {
	// Initialize configuration manager
	// TODO: Load configuration from file

	// Initialize validator
	// TODO: Implement concrete validator

	// Initialize collector registry
	// TODO: Implement concrete collector registry

	// Initialize scheduler
	// TODO: Implement concrete scheduler

	// Initialize processor
	// TODO: Implement concrete processor

	// Initialize reporter
	// TODO: Implement concrete reporter

	// Initialize cache
	// TODO: Implement concrete cache

	return nil
}

// Start starts the application
func (a *App) Start() error {
	fmt.Println("Starting K8s Node Diagnostor...")

	// Start configuration watching
	if err := a.configManager.Watch(a.ctx); err != nil {
		return fmt.Errorf("failed to start config watcher: %w", err)
	}

	// Start scheduler
	if err := a.scheduler.Start(a.ctx); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	fmt.Println("K8s Node Diagnostor started successfully")
	return nil
}

// Stop gracefully stops the application
func (a *App) Stop() error {
	fmt.Println("Stopping K8s Node Diagnostor...")

	a.cancel()

	// Stop scheduler
	if err := a.scheduler.Stop(a.ctx); err != nil {
		return fmt.Errorf("failed to stop scheduler: %w", err)
	}

	// Close reporter
	if a.reporter != nil {
		if err := a.reporter.Close(); err != nil {
			return fmt.Errorf("failed to close reporter: %w", err)
		}
	}

	// Close config manager
	if err := a.configManager.Close(); err != nil {
		return fmt.Errorf("failed to close config manager: %w", err)
	}

	fmt.Println("K8s Node Diagnostor stopped successfully")
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
	fmt.Println("\nReceived interrupt signal")

	return a.Stop()
}
