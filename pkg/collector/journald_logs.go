package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// JournaldLog journald日志条目
type JournaldLog struct {
	Timestamp time.Time `json:"__REALTIME_TIMESTAMP"`
	Message   string    `json:"MESSAGE"`
	Unit      string    `json:"_SYSTEMD_UNIT"`
	Priority  string    `json:"PRIORITY"`
}

// JournaldCollector journald日志采集器
type JournaldCollector struct {
	units    []string       // 监控的systemd单元列表
	stopChan chan struct{}  // 停止信号
	wg       sync.WaitGroup // 等待组
}

// NewJournaldCollector 创建journald日志采集器
func NewJournaldCollector(units []string) *JournaldCollector {
	return &JournaldCollector{
		units:    units,
		stopChan: make(chan struct{}),
	}
}

// Start 启动日志采集
func (c *JournaldCollector) Start() chan JournaldLog {
	logChan := make(chan JournaldLog, 100)
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()
		defer close(logChan)

		// 构建journalctl命令参数
		args := []string{"--follow", "--output=json"}
		for _, unit := range c.units {
			args = append(args, "--unit="+unit)
		}

		cmd := exec.CommandContext(context.Background(), "journalctl", args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return
		}

		if err := cmd.Start(); err != nil {
			return
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case <-c.stopChan:
				cmd.Process.Kill()
				return
			default:
				var logEntry JournaldLog
				if err := json.Unmarshal(scanner.Bytes(), &logEntry); err == nil {
					logEntry.Message = strings.TrimSpace(logEntry.Message)
					logChan <- logEntry
				}
			}
		}
	}()

	return logChan
}

// Stop 停止日志采集
func (c *JournaldCollector) Stop() {
	close(c.stopChan)
	c.wg.Wait()
}
