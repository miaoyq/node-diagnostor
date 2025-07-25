package resource

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TimeoutManager manages timeouts for check items.
type TimeoutManager struct {
	logger   *zap.Logger
	timeouts map[string]time.Duration
	mu       sync.Mutex
}

// NewTimeoutManager creates a new TimeoutManager.
func NewTimeoutManager(logger *zap.Logger) *TimeoutManager {
	return &TimeoutManager{
		logger:   logger,
		timeouts: make(map[string]time.Duration),
	}
}

// SetTimeout sets the timeout for a specific check item.
func (tm *TimeoutManager) SetTimeout(checkID string, timeout time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.timeouts[checkID] = timeout
}

// GetTimeout gets the timeout for a specific check item.
func (tm *TimeoutManager) GetTimeout(checkID string) time.Duration {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.timeouts[checkID]
}

// RunWithTimeout executes a function with a timeout.
func (tm *TimeoutManager) RunWithTimeout(ctx context.Context, checkID string, fn func(context.Context) error) error {
	timeout := tm.GetTimeout(checkID)
	if timeout == 0 {
		return fn(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		tm.logger.Warn("Check item timed out", zap.String("checkID", checkID), zap.Duration("timeout", timeout))
		return ctx.Err()
	}
}

// Cleanup cleans up resources after a timeout.
func (tm *TimeoutManager) Cleanup(checkID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.timeouts, checkID)
}
