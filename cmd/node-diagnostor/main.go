package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/miaoyq/node-diagnostor/pkg/collector"
	"github.com/miaoyq/node-diagnostor/pkg/config"
	"github.com/miaoyq/node-diagnostor/pkg/processor"
	"github.com/miaoyq/node-diagnostor/pkg/types"
	"github.com/spf13/cobra"
)

var (
	cfg     *config.Config
	cfgPath string
	loader  *config.Loader
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "node-diagnostor",
		Short: "Kubernetes节点诊断工具",
		Long:  "轻量级节点监控工具，收集指标、日志和进程信息，实现异常检测与上报",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// 加载配置
			var err error
			cfg, err = config.LoadConfig(cfgPath)
			if err != nil {
				return fmt.Errorf("加载配置失败: %v", err)
			}
			fmt.Printf("配置加载成功，监控周期: %v\n", cfg.MetricsInterval)

			// 创建配置加载器
			loader, err = config.NewLoader(cfgPath, func() {
				newCfg, err := config.LoadConfig(cfgPath)
				if err != nil {
					fmt.Printf("配置热加载失败: %v\n", err)
					return
				}
				cfg = newCfg
				fmt.Println("配置已热更新")
			})
			if err != nil {
				return fmt.Errorf("创建配置加载器失败: %v", err)
			}

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			fmt.Printf("节点诊断工具启动，dry-run模式: %v\n", dryRun)

			// 创建数据通道
			metricsChan := make(chan *types.SystemMetrics, 10)
			logChan := make(chan types.LogEntry, 100)
			reportChan := make(chan types.DiagnosticReport, 10)

			// 创建上下文和取消函数
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// 设置信号处理
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			// 启动日志监视器
			if len(cfg.JournalUnits) > 0 {
				go func() {
					if err := collector.MonitorJournal(ctx, cfg.JournalUnits, logChan); err != nil {
						fmt.Printf("日志监视器启动失败: %v\n", err)
					}
				}()
			}

			// 启动数据处理器
			go processor.ProcessData(ctx, cfg, metricsChan, logChan, reportChan)

			// 启动报告处理器（无论是否dry-run模式都保存报告到文件）
			go func() {
				for report := range reportChan {
					outputFile := "test/diagnostic_report.json"
					if dryRun {
						outputFile = "test/diagnostic_report.json"
					}
					if err := saveReportToFile(report, outputFile); err != nil {
						fmt.Printf("保存报告失败: %v\n", err)
					} else {
						fmt.Printf("报告已保存到: %s\n", outputFile)
					}
				}
			}()

			// 主循环
			ticker := time.NewTicker(cfg.MetricsInterval)
			defer ticker.Stop()

			// 用于优雅关闭的等待组
			done := make(chan struct{})

			go func() {
				for {
					select {
					case <-ticker.C:
						fmt.Println("执行数据采集...")
						metrics, err := collector.CollectSystemMetrics()
						if err != nil {
							fmt.Printf("指标采集失败: %v\n", err)
						} else {
							metricsChan <- metrics
							fmt.Printf("CPU使用率: %.2f%%, 内存使用: %d/%d MB\n",
								metrics.CPUUsage,
								metrics.MemoryUsed/(1024*1024),
								metrics.MemoryTotal/(1024*1024))
						}
					case <-ctx.Done():
						close(done)
						return
					}
				}
			}()

			// 等待信号或上下文取消
			select {
			case sig := <-sigChan:
				fmt.Printf("接收到信号 %v，开始优雅关闭...\n", sig)
				cancel()
			case <-done:
				fmt.Println("主循环已结束")
			}

			// 等待所有goroutine完成
			time.Sleep(2 * time.Second)
			fmt.Println("程序已优雅关闭")
		},
	}

	rootCmd.Flags().Bool("dry-run", false, "测试运行模式，不实际上报数据")
	rootCmd.Flags().StringVarP(&cfgPath, "config", "c", "/etc/node-diagnostor/config.json", "配置文件路径")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// 关闭配置加载器
	if loader != nil {
		loader.Close()
	}
}

// saveReportToFile 将报告保存到文件
func saveReportToFile(report types.DiagnosticReport, filename string) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return types.SaveReportToFile(report, filename)
}
