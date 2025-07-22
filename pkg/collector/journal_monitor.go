package collector

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/miaoyq/node-diagnostor/pkg/types"
)

// MonitorJournal 监视journal日志
func MonitorJournal(ctx context.Context, units []string, logChan chan<- types.LogEntry) error {
	args := []string{"-f"}
	for _, unit := range units {
		args = append(args, "-u", unit)
	}

	cmd := exec.CommandContext(ctx, "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("获取标准输出管道失败: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动journalctl命令失败: %v", err)
	}

	go func() {
		defer cmd.Wait()
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				log.Printf("读取日志失败: %v", err)
				return
			}

			line := string(buf[:n])
			// 解析日志行
			entry := parseJournalLine(line)
			if entry != nil {
				logChan <- *entry
			}
		}
	}()

	return nil
}

func parseJournalLine(line string) *types.LogEntry {
	// 简化解析逻辑
	parts := strings.SplitN(line, " ", 6)
	if len(parts) < 6 {
		return nil
	}

	timestamp, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return nil
	}

	return &types.LogEntry{
		Timestamp: timestamp,
		Unit:      parts[2],
		Priority:  parts[4],
		Message:   parts[5],
	}
}
