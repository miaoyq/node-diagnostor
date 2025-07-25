package reporter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewCacheManager(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tempDir := t.TempDir()
	cm := NewCacheManager(tempDir, 1024*1024, time.Hour, logger)

	assert.NotNil(t, cm)
	assert.Equal(t, tempDir, cm.cacheDir)
	assert.Equal(t, int64(1024*1024), cm.maxSize)
	assert.Equal(t, time.Hour, cm.maxAge)
}

func TestCacheManager_AddAndGet(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tempDir := t.TempDir()
	cm := NewCacheManager(tempDir, 1024*1024, time.Hour, logger)

	ctx := context.Background()
	err := cm.Start(ctx)
	require.NoError(t, err)
	defer cm.Stop()

	entry := &CacheEntry{
		Report:    &Report{ID: "test-1"},
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}

	err = cm.Add("test-1", entry)
	require.NoError(t, err)

	retrieved, exists := cm.Get("test-1")
	assert.True(t, exists)
	assert.Equal(t, "test-1", retrieved.Report.ID)
	assert.Equal(t, StatusPending, retrieved.Status)
}

func TestCacheManager_ExpiredEntry(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tempDir := t.TempDir()
	cm := NewCacheManager(tempDir, 1024*1024, time.Millisecond, logger)

	ctx := context.Background()
	err := cm.Start(ctx)
	require.NoError(t, err)
	defer cm.Stop()

	entry := &CacheEntry{
		Report:    &Report{ID: "test-1"},
		Status:    StatusPending,
		CreatedAt: time.Now().Add(-2 * time.Millisecond),
	}

	err = cm.Add("test-1", entry)
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	_, exists := cm.Get("test-1")
	assert.False(t, exists)
}

func TestCacheManager_SizeLimit(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tempDir := t.TempDir()
	cm := NewCacheManager(tempDir, 100, time.Hour, logger)

	ctx := context.Background()
	err := cm.Start(ctx)
	require.NoError(t, err)
	defer cm.Stop()

	// Add a large entry that exceeds size limit
	largeData := make(map[string]interface{})
	for i := 0; i < 1000; i++ {
		largeData[string(rune(i))] = "large data content"
	}

	entry := &CacheEntry{
		Report: &Report{
			ID:   "large-1",
			Data: largeData,
		},
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}

	err = cm.Add("large-1", entry)
	assert.Error(t, err)
}

func TestCacheManager_Remove(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tempDir := t.TempDir()
	cm := NewCacheManager(tempDir, 1024*1024, time.Hour, logger)

	ctx := context.Background()
	err := cm.Start(ctx)
	require.NoError(t, err)
	defer cm.Stop()

	entry := &CacheEntry{
		Report:    &Report{ID: "test-1"},
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}

	err = cm.Add("test-1", entry)
	require.NoError(t, err)

	cm.Remove("test-1")
	_, exists := cm.Get("test-1")
	assert.False(t, exists)
}

func TestCacheManager_Clear(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tempDir := t.TempDir()
	cm := NewCacheManager(tempDir, 1024*1024, time.Hour, logger)

	ctx := context.Background()
	err := cm.Start(ctx)
	require.NoError(t, err)
	defer cm.Stop()

	entry := &CacheEntry{
		Report:    &Report{ID: "test-1"},
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}

	err = cm.Add("test-1", entry)
	require.NoError(t, err)

	cm.Clear()
	stats := cm.GetStats()
	assert.Equal(t, 0, stats.Size)
}

func TestCacheManager_GetStats(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tempDir := t.TempDir()
	cm := NewCacheManager(tempDir, 1024*1024, time.Hour, logger)

	ctx := context.Background()
	err := cm.Start(ctx)
	require.NoError(t, err)
	defer cm.Stop()

	entries := []*CacheEntry{
		{
			Report:    &Report{ID: "test-1", Data: map[string]interface{}{"key": "value1"}},
			Status:    StatusPending,
			CreatedAt: time.Now(),
		},
		{
			Report:    &Report{ID: "test-2", Data: map[string]interface{}{"key": "value2"}},
			Status:    StatusFailed,
			CreatedAt: time.Now(),
		},
		{
			Report:    &Report{ID: "test-3", Data: map[string]interface{}{"key": "value3"}},
			Status:    StatusSuccess,
			CreatedAt: time.Now(),
		},
	}

	for i, entry := range entries {
		err = cm.Add(string(rune('a'+i)), entry)
		require.NoError(t, err)
	}

	stats := cm.GetStats()
	assert.Equal(t, 3, stats.Size)
	assert.Equal(t, 1, stats.Pending)
	assert.Equal(t, 1, stats.Failed)
	assert.Equal(t, 1, stats.Success)
	assert.Greater(t, stats.TotalSize, int64(0))
}

func TestCacheManager_Persistence(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tempDir := t.TempDir()
	cacheFile := filepath.Join(tempDir, "cache.json")

	// First instance
	cm1 := NewCacheManager(tempDir, 1024*1024, time.Hour, logger)
	ctx := context.Background()
	err := cm1.Start(ctx)
	require.NoError(t, err)

	entry := &CacheEntry{
		Report:    &Report{ID: "persist-1"},
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}

	err = cm1.Add("persist-1", entry)
	require.NoError(t, err)

	cm1.Stop()

	// Verify cache file exists
	_, err = os.Stat(cacheFile)
	assert.NoError(t, err)

	// Second instance should load from cache
	cm2 := NewCacheManager(tempDir, 1024*1024, time.Hour, logger)
	err = cm2.Start(ctx)
	require.NoError(t, err)
	defer cm2.Stop()

	retrieved, exists := cm2.Get("persist-1")
	assert.True(t, exists)
	assert.Equal(t, "persist-1", retrieved.Report.ID)
}

func TestCacheManager_Cleanup(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tempDir := t.TempDir()
	cm := NewCacheManager(tempDir, 1024*1024, time.Millisecond, logger)
	cm.cleanupFreq = 10 * time.Millisecond

	ctx := context.Background()
	err := cm.Start(ctx)
	require.NoError(t, err)
	defer cm.Stop()

	entry := &CacheEntry{
		Report:    &Report{ID: "test-1"},
		Status:    StatusPending,
		CreatedAt: time.Now().Add(-2 * time.Millisecond),
	}

	err = cm.Add("test-1", entry)
	require.NoError(t, err)

	// Wait for cleanup
	time.Sleep(50 * time.Millisecond)

	stats := cm.GetStats()
	assert.Equal(t, 0, stats.Size)
}

func TestCacheManager_MakeSpace(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tempDir := t.TempDir()
	cm := NewCacheManager(tempDir, 3000, time.Hour, logger)

	ctx := context.Background()
	err := cm.Start(ctx)
	require.NoError(t, err)
	defer cm.Stop()

	// Add multiple small entries
	for i := 0; i < 5; i++ {
		entry := &CacheEntry{
			Report: &Report{
				ID:   string(rune('a' + i)),
				Data: map[string]interface{}{"data": string(make([]byte, 100))},
			},
			Status:    StatusPending,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		err = cm.Add(string(rune('a'+i)), entry)
		require.NoError(t, err)
	}

	// Add a large entry that requires space
	largeEntry := &CacheEntry{
		Report: &Report{
			ID:   "large",
			Data: map[string]interface{}{"data": string(make([]byte, 300))},
		},
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}

	err = cm.Add("large", largeEntry)
	require.NoError(t, err)

	stats := cm.GetStats()
	assert.LessOrEqual(t, stats.TotalSize, int64(3000))
}

func TestCacheManager_ConcurrentAccess(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tempDir := t.TempDir()
	cm := NewCacheManager(tempDir, 1024*1024, time.Hour, logger)

	ctx := context.Background()
	err := cm.Start(ctx)
	require.NoError(t, err)
	defer cm.Stop()

	// Concurrent add and get
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			entry := &CacheEntry{
				Report:    &Report{ID: string(rune('a' + i%26))},
				Status:    StatusPending,
				CreatedAt: time.Now(),
			}
			_ = cm.Add(string(rune('a'+i%26)), entry)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_, _ = cm.Get(string(rune('a' + i%26)))
		}
		done <- true
	}()

	<-done
	<-done

	stats := cm.GetStats()
	assert.Greater(t, stats.Size, 0)
}
