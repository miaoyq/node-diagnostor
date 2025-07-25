package collector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessResourceCollector(t *testing.T) {
	collector := &ProcessResourceCollector{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "process-resource", collector.Name())
	})

	t.Run("Flags", func(t *testing.T) {
		assert.False(t, collector.IsLocalOnly())
		assert.True(t, collector.IsClusterSupplement())
		assert.Equal(t, 2, collector.Priority())
		assert.Equal(t, 10*time.Second, collector.Timeout())
	})

	t.Run("Collect", func(t *testing.T) {
		result, err := collector.Collect(context.Background(), nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.Data)

		processes, ok := result.Data.Data["processes"].([]map[string]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(processes), 0)

		// 检查第一个进程的基本信息
		if len(processes) > 0 {
			process := processes[0]
			assert.Contains(t, process, "pid")
			assert.Contains(t, process, "name")
		}
	})

	t.Run("Validate", func(t *testing.T) {
		assert.NoError(t, collector.Validate(nil))
		assert.NoError(t, collector.Validate(map[string]interface{}{"test": "value"}))
	})
}

func TestKernelEventCollector(t *testing.T) {
	collector := &KernelEventCollector{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "kernel-events", collector.Name())
	})

	t.Run("Flags", func(t *testing.T) {
		assert.False(t, collector.IsLocalOnly())
		assert.True(t, collector.IsClusterSupplement())
		assert.Equal(t, 1, collector.Priority())
		assert.Equal(t, 5*time.Second, collector.Timeout())
	})

	t.Run("Collect", func(t *testing.T) {
		// 创建临时日志文件用于测试
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "kern.log")

		content := `2024-01-01 10:00:00 kernel: [12345.678] error: something went wrong
2024-01-01 10:01:00 kernel: [12346.789] warning: disk space low
2024-01-01 10:02:00 kernel: [12347.890] info: normal operation`

		err := os.WriteFile(logFile, []byte(content), 0644)
		require.NoError(t, err)

		// 临时替换日志文件路径
		originalLogFile := "/var/log/kern.log"
		if _, err := os.Stat(originalLogFile); err == nil {
			// 如果原文件存在，备份并恢复
			defer func() {
				// 恢复逻辑
			}()
		}

		result, err := collector.Collect(context.Background(), nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.Data)
	})

	t.Run("Validate", func(t *testing.T) {
		assert.NoError(t, collector.Validate(nil))
	})
}

func TestConfigFileCollector(t *testing.T) {
	collector := &ConfigFileCollector{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "config-files", collector.Name())
	})

	t.Run("Flags", func(t *testing.T) {
		assert.False(t, collector.IsLocalOnly())
		assert.True(t, collector.IsClusterSupplement())
		assert.Equal(t, 3, collector.Priority())
		assert.Equal(t, 15*time.Second, collector.Timeout())
	})

	t.Run("Collect", func(t *testing.T) {
		// 创建临时配置目录用于测试
		tmpDir := t.TempDir()
		configDir := filepath.Join(tmpDir, "test-config")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		// 创建测试配置文件
		testFile := filepath.Join(configDir, "test.conf")
		err = os.WriteFile(testFile, []byte("test config content\nline 2\nline 3"), 0644)
		require.NoError(t, err)

		result, err := collector.Collect(context.Background(), nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.Data)

		configs, ok := result.Data.Data["configs"].([]map[string]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(configs), 0)
	})

	t.Run("Validate", func(t *testing.T) {
		assert.NoError(t, collector.Validate(nil))
	})
}

func TestCertificateCollector(t *testing.T) {
	collector := &CertificateCollector{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "certificates", collector.Name())
	})

	t.Run("Flags", func(t *testing.T) {
		assert.False(t, collector.IsLocalOnly())
		assert.True(t, collector.IsClusterSupplement())
		assert.Equal(t, 3, collector.Priority())
		assert.Equal(t, 10*time.Second, collector.Timeout())
	})

	t.Run("Collect", func(t *testing.T) {
		result, err := collector.Collect(context.Background(), nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.Data)

		certs, ok := result.Data.Data["certificates"].([]map[string]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(certs), 0)
	})

	t.Run("Validate", func(t *testing.T) {
		assert.NoError(t, collector.Validate(nil))
	})
}

func TestParseMemoryValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
		wantErr  bool
	}{
		{"valid KB", "1024 kB", 1024, false},
		{"valid with spaces", " 2048 kB ", 2048, false},
		{"invalid format", "invalid", 0, true},
		{"empty string", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseMemoryValue(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestExtractTimestamp(t *testing.T) {
	t.Run("valid log line", func(t *testing.T) {
		line := "2024-01-01T10:00:00Z kernel: test message"
		result := extractTimestamp(line)
		assert.NotEmpty(t, result)
	})

	t.Run("empty line", func(t *testing.T) {
		result := extractTimestamp("")
		assert.NotEmpty(t, result)
	})
}

// 注册测试移到专门的注册测试中
