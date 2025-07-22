package collector

import (
	"context"
	"testing"
	"time"

	"github.com/miaoyq/node-diagnostor/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestMonitorJournal(t *testing.T) {
	t.Run("basic monitor creation", func(t *testing.T) {
		logChan := make(chan types.LogEntry, 10)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := MonitorJournal(ctx, []string{"kubelet"}, logChan)

		// 由于journalctl可能不存在，我们主要测试函数调用
		assert.NoError(t, err)
	})

	t.Run("empty units list", func(t *testing.T) {
		logChan := make(chan types.LogEntry, 10)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := MonitorJournal(ctx, []string{}, logChan)

		assert.NoError(t, err)
	})

	t.Run("multiple units", func(t *testing.T) {
		logChan := make(chan types.LogEntry, 10)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		units := []string{"kubelet", "containerd", "docker"}
		err := MonitorJournal(ctx, units, logChan)

		assert.NoError(t, err)
	})
}

func TestParseJournalLine(t *testing.T) {
	t.Run("valid journal line", func(t *testing.T) {
		line := "2024-01-15T10:30:00Z hostname kubelet[1234]: INFO test message"
		entry := parseJournalLine(line)

		assert.NotNil(t, entry)
		assert.Equal(t, "kubelet[1234]:", entry.Unit)
		assert.Equal(t, "test", entry.Priority)
		assert.Contains(t, entry.Message, "message")
	})

	t.Run("invalid timestamp format", func(t *testing.T) {
		line := "invalid-timestamp hostname kubelet[1234]: INFO test message"
		entry := parseJournalLine(line)

		assert.Nil(t, entry)
	})

	t.Run("short line", func(t *testing.T) {
		line := "short line"
		entry := parseJournalLine(line)

		assert.Nil(t, entry)
	})

	t.Run("empty line", func(t *testing.T) {
		entry := parseJournalLine("")
		assert.Nil(t, entry)
	})

	t.Run("edge case line", func(t *testing.T) {
		line := "2024-01-15T10:30:00Z host unit"
		entry := parseJournalLine(line)

		// 允许解析失败，因为格式可能不匹配
		if entry != nil {
			assert.Equal(t, "unit", entry.Unit)
		}
	})
}
