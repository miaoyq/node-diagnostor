package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// 状态字符池 - 减少内存分配
var statePool = map[byte]byte{
	'R': 'R', // 运行中
	'S': 'S', // 睡眠(可中断)
	'D': 'D', // 磁盘睡眠(不可中断)
	'Z': 'Z', // 僵尸
	'T': 'T', // 停止
	't': 't', // 跟踪停止
	'X': 'X', // 死亡
	'x': 'x', // 死亡
	'K': 'K', // 唤醒kill
	'W': 'W', // 唤醒
}

type ProcStat struct {
	procMap   map[int]byte // 进程状态缓存: PID -> 状态
	threadMap map[int]byte // 线程状态缓存: TID -> 状态
	mu        sync.Mutex   // 保护并发访问
}

func NewProcStat() *ProcStat {
	return &ProcStat{
		procMap:   make(map[int]byte, 512),  // 预分配内存
		threadMap: make(map[int]byte, 4096), // 预分配内存
	}
}

// 从/proc/PID/status读取进程状态
func (ps *ProcStat) readProcStatus(pid int) (byte, error) {
	statusFile := filepath.Join("/proc", fmt.Sprintf("%d", pid), "status")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return 0, err
	}

	// 复用缓冲区查找状态行
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "State:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				state := fields[1][0] // 取第一个字符
				if s, ok := statePool[state]; ok {
					return s, nil
				}
				return state, nil
			}
		}
	}
	return 0, fmt.Errorf("state not found for PID %d", pid)
}

// 从/proc/PID/task/TID/status读取线程状态
func (ps *ProcStat) readThreadStatus(pid, tid int) (byte, error) {
	statusFile := filepath.Join("/proc", fmt.Sprintf("%d", pid), "task", fmt.Sprintf("%d", tid), "status")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "State:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				state := fields[1][0]
				if s, ok := statePool[state]; ok {
					return s, nil
				}
				return state, nil
			}
		}
	}
	return 0, fmt.Errorf("state not found for TID %d", tid)
}

// 扫描所有进程和线程
func (ps *ProcStat) Scan() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 清空缓存但保留内存空间
	for k := range ps.procMap {
		delete(ps.procMap, k)
	}
	for k := range ps.threadMap {
		delete(ps.threadMap, k)
	}

	// 遍历/proc目录获取所有PID
	pids, err := filepath.Glob("/proc/[0-9]*")
	if err != nil {
		return err
	}

	for _, pidPath := range pids {
		pidStr := filepath.Base(pidPath)
		var pid int
		if _, err := fmt.Sscanf(pidStr, "%d", &pid); err != nil {
			continue
		}

		// 读取进程状态
		state, err := ps.readProcStatus(pid)
		if err != nil {
			continue
		}
		ps.procMap[pid] = state

		// 遍历进程的所有线程
		taskPath := filepath.Join(pidPath, "task")
		tids, err := filepath.Glob(filepath.Join(taskPath, "[0-9]*"))
		if err != nil {
			continue
		}

		for _, tidPath := range tids {
			tidStr := filepath.Base(tidPath)
			var tid int
			if _, err := fmt.Sscanf(tidStr, "%d", &tid); err != nil {
				continue
			}

			// 读取线程状态
			tState, err := ps.readThreadStatus(pid, tid)
			if err != nil {
				continue
			}
			ps.threadMap[tid] = tState
		}
	}
	return nil
}

// 获取统计信息
func (ps *ProcStat) Stats() (int, int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return len(ps.procMap), len(ps.threadMap)
}

// 获取进程状态缓存
func (ps *ProcStat) GetProcMap() map[int]byte {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.procMap
}

// 获取线程状态缓存
func (ps *ProcStat) GetThreadMap() map[int]byte {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.threadMap
}

func main() {
	ps := NewProcStat()
	if err := ps.Scan(); err != nil {
		fmt.Printf("扫描失败: %v\n", err)
		os.Exit(1)
	}

	procCount, threadCount := ps.Stats()
	fmt.Printf("系统进程数: %d\n", procCount)
	fmt.Printf("系统线程数: %d\n", threadCount)
	fmt.Println("\n进程状态缓存:")
	for pid, state := range ps.GetProcMap() {
		fmt.Printf("PID: %d, 状态: %c\n", pid, state)
	}
	fmt.Println("\n线程状态缓存:")
	for tid, state := range ps.GetThreadMap() {
		fmt.Printf("TID: %d, 状态: %c\n", tid, state)
	}
}
