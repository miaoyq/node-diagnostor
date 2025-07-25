package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CacheManager manages the report cache with TTL, size limits, and persistence
type CacheManager struct {
	cacheDir    string
	maxSize     int64
	maxAge      time.Duration
	cleanupFreq time.Duration
	cache       map[string]*CacheEntry
	mutex       sync.RWMutex
	logger      *zap.Logger
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// CacheStats contains cache statistics
type CacheStats struct {
	Size        int
	TotalSize   int64
	Expired     int
	Failed      int
	Pending     int
	Success     int
	Discarded   int
	LastCleanup time.Time
}

// NewCacheManager creates a new cache manager instance
func NewCacheManager(cacheDir string, maxSize int64, maxAge time.Duration, logger *zap.Logger) *CacheManager {
	return &CacheManager{
		cacheDir:    cacheDir,
		maxSize:     maxSize,
		maxAge:      maxAge,
		cleanupFreq: 5 * time.Minute,
		cache:       make(map[string]*CacheEntry),
		logger:      logger,
		stopChan:    make(chan struct{}),
	}
}

// Start starts the cache manager and background cleanup routine
func (cm *CacheManager) Start(ctx context.Context) error {
	cm.logger.Info("Starting cache manager",
		zap.String("cache_dir", cm.cacheDir),
		zap.Int64("max_size_mb", cm.maxSize/1024/1024),
		zap.Duration("max_age", cm.maxAge))

	// Ensure cache directory exists
	if err := os.MkdirAll(cm.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Load persisted cache
	if err := cm.loadPersistedCache(); err != nil {
		cm.logger.Warn("Failed to load persisted cache", zap.Error(err))
	}

	// Start cleanup routine
	cm.wg.Add(1)
	go cm.cleanupRoutine(ctx)

	return nil
}

// Stop stops the cache manager and saves cache to disk
func (cm *CacheManager) Stop() {
	close(cm.stopChan)
	cm.wg.Wait()

	// Persist cache to disk
	if err := cm.persistCache(); err != nil {
		cm.logger.Error("Failed to persist cache", zap.Error(err))
	}

	cm.logger.Info("Cache manager stopped")
}

// Add adds a new entry to the cache
func (cm *CacheManager) Add(id string, entry *CacheEntry) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Check if adding this entry would exceed max size
	if cm.wouldExceedMaxSize(entry) {
		// Remove oldest entries to make space
		if err := cm.makeSpaceForEntry(entry); err != nil {
			return fmt.Errorf("failed to make space for new entry: %w", err)
		}
	}

	cm.cache[id] = entry
	return nil
}

// Get retrieves an entry from the cache
func (cm *CacheManager) Get(id string) (*CacheEntry, bool) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	entry, exists := cm.cache[id]
	if !exists {
		return nil, false
	}

	// Check if entry has expired
	if time.Since(entry.CreatedAt) > cm.maxAge {
		return nil, false
	}

	return entry, true
}

// Remove removes an entry from the cache
func (cm *CacheManager) Remove(id string) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	delete(cm.cache, id)
}

// GetStats returns cache statistics
func (cm *CacheManager) GetStats() CacheStats {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	stats := CacheStats{
		Size: len(cm.cache),
	}

	var totalSize int64
	for _, entry := range cm.cache {
		// Estimate size based on report data
		if entry.Report != nil && entry.Report.Data != nil {
			jsonData, _ := json.Marshal(entry.Report.Data)
			totalSize += int64(len(jsonData))
		}

		switch entry.Status {
		case StatusFailed:
			stats.Failed++
		case StatusPending:
			stats.Pending++
		case StatusSuccess:
			stats.Success++
		case StatusDiscarded:
			stats.Discarded++
		}

		if time.Since(entry.CreatedAt) > cm.maxAge {
			stats.Expired++
		}
	}

	stats.TotalSize = totalSize
	return stats
}

// Clear removes all entries from the cache
func (cm *CacheManager) Clear() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.cache = make(map[string]*CacheEntry)
}

// cleanupRoutine periodically cleans expired and oversized cache entries
func (cm *CacheManager) cleanupRoutine(ctx context.Context) {
	defer cm.wg.Done()

	ticker := time.NewTicker(cm.cleanupFreq)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.cleanup()
		case <-ctx.Done():
			return
		case <-cm.stopChan:
			return
		}
	}
}

// cleanup removes expired entries and enforces size limits
func (cm *CacheManager) cleanup() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	removed := 0
	now := time.Now()

	// Remove expired entries
	for id, entry := range cm.cache {
		if now.Sub(entry.CreatedAt) > cm.maxAge {
			delete(cm.cache, id)
			removed++
		}
	}

	// Enforce size limit
	if cm.getCurrentSize() > cm.maxSize {
		removed += cm.enforceSizeLimit()
	}

	if removed > 0 {
		cm.logger.Debug("Cache cleanup completed",
			zap.Int("removed_entries", removed),
			zap.Int("remaining_entries", len(cm.cache)))
	}
}

// getCurrentSize calculates the current cache size
func (cm *CacheManager) getCurrentSize() int64 {
	var totalSize int64
	for _, entry := range cm.cache {
		if entry.Report != nil && entry.Report.Data != nil {
			jsonData, _ := json.Marshal(entry.Report.Data)
			totalSize += int64(len(jsonData))
		}
	}
	return totalSize
}

// wouldExceedMaxSize checks if adding an entry would exceed max size
func (cm *CacheManager) wouldExceedMaxSize(entry *CacheEntry) bool {
	if cm.maxSize <= 0 {
		return false
	}

	var entrySize int64
	if entry.Report != nil && entry.Report.Data != nil {
		jsonData, _ := json.Marshal(entry.Report.Data)
		entrySize = int64(len(jsonData))
	}

	return cm.getCurrentSize()+entrySize > cm.maxSize
}

// makeSpaceForEntry removes oldest entries to make space
func (cm *CacheManager) makeSpaceForEntry(entry *CacheEntry) error {
	var entrySize int64
	if entry.Report != nil && entry.Report.Data != nil {
		jsonData, _ := json.Marshal(entry.Report.Data)
		entrySize = int64(len(jsonData))
	}

	targetSize := cm.maxSize - entrySize
	if targetSize < 0 {
		return fmt.Errorf("entry size exceeds max cache size")
	}

	// Sort entries by creation time (oldest first)
	type entryWithTime struct {
		id      string
		entry   *CacheEntry
		created time.Time
	}

	var entries []entryWithTime
	for id, e := range cm.cache {
		entries = append(entries, entryWithTime{id, e, e.CreatedAt})
	}

	// Simple sort by creation time
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].created.After(entries[j].created) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Remove oldest entries until we have enough space
	currentSize := cm.getCurrentSize()
	for _, e := range entries {
		if currentSize <= targetSize {
			break
		}

		var eSize int64
		if e.entry.Report != nil && e.entry.Report.Data != nil {
			jsonData, _ := json.Marshal(e.entry.Report.Data)
			eSize = int64(len(jsonData))
		}

		delete(cm.cache, e.id)
		currentSize -= eSize
	}

	return nil
}

// enforceSizeLimit removes entries until size is within limit
func (cm *CacheManager) enforceSizeLimit() int {
	// Similar to makeSpaceForEntry but for existing cache
	targetSize := cm.maxSize
	currentSize := cm.getCurrentSize()

	if currentSize <= targetSize {
		return 0
	}

	// Sort entries by creation time
	type entryWithTime struct {
		id      string
		entry   *CacheEntry
		created time.Time
	}

	var entries []entryWithTime
	for id, e := range cm.cache {
		entries = append(entries, entryWithTime{id, e, e.CreatedAt})
	}

	// Simple sort by creation time
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].created.After(entries[j].created) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	removed := 0
	for _, e := range entries {
		if currentSize <= targetSize {
			break
		}

		var eSize int64
		if e.entry.Report != nil && e.entry.Report.Data != nil {
			jsonData, _ := json.Marshal(e.entry.Report.Data)
			eSize = int64(len(jsonData))
		}

		delete(cm.cache, e.id)
		currentSize -= eSize
		removed++
	}

	return removed
}

// persistCache saves cache to disk
func (cm *CacheManager) persistCache() error {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	if len(cm.cache) == 0 {
		// Remove cache file if empty
		cacheFile := filepath.Join(cm.cacheDir, "cache.json")
		_ = os.Remove(cacheFile)
		return nil
	}

	// Prepare data for persistence
	data := struct {
		Entries   map[string]*CacheEntry `json:"entries"`
		Timestamp time.Time              `json:"timestamp"`
	}{
		Entries:   cm.cache,
		Timestamp: time.Now(),
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	// Write to temporary file first
	cacheFile := filepath.Join(cm.cacheDir, "cache.json")
	tempFile := cacheFile + ".tmp"

	if err := os.WriteFile(tempFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, cacheFile); err != nil {
		_ = os.Remove(tempFile)
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	cm.logger.Debug("Cache persisted to disk",
		zap.String("file", cacheFile),
		zap.Int("entries", len(cm.cache)))

	return nil
}

// loadPersistedCache loads cache from disk
func (cm *CacheManager) loadPersistedCache() error {
	cacheFile := filepath.Join(cm.cacheDir, "cache.json")

	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		return nil // No cache file exists
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return fmt.Errorf("failed to read cache file: %w", err)
	}

	var persisted struct {
		Entries   map[string]*CacheEntry `json:"entries"`
		Timestamp time.Time              `json:"timestamp"`
	}

	if err := json.Unmarshal(data, &persisted); err != nil {
		return fmt.Errorf("failed to unmarshal cache: %w", err)
	}

	// Only load entries that are not too old
	now := time.Now()
	loaded := 0
	for id, entry := range persisted.Entries {
		if now.Sub(entry.CreatedAt) <= cm.maxAge {
			cm.cache[id] = entry
			loaded++
		}
	}

	cm.logger.Info("Cache loaded from disk",
		zap.Int("loaded_entries", loaded),
		zap.Int("total_entries", len(persisted.Entries)))

	return nil
}
