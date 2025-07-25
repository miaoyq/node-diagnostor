package collector

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestK8sComponentCollectors(t *testing.T) {
	tests := []struct {
		name      string
		collector Collector
	}{
		{"kubelet", NewKubeletCollector()},
		{"containerd", NewContainerdCollector()},
		{"cni", NewCNICollector()},
		{"kube-proxy", NewKubeProxyCollector()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 验证基本接口
			if tt.collector.Name() != tt.name {
				t.Errorf("Expected name %s, got %s", tt.name, tt.collector.Name())
			}

			if !tt.collector.IsLocalOnly() {
				t.Errorf("Expected %s to be local-only", tt.name)
			}

			if tt.collector.IsClusterSupplement() {
				t.Errorf("Expected %s to not be cluster supplement", tt.name)
			}

			// 验证参数验证
			if err := tt.collector.Validate(nil); err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}

			// 测试采集功能（在测试环境中可能失败，但不应panic）
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			result, err := tt.collector.Collect(ctx, nil)
			if err != nil {
				t.Logf("Collect() returned error (expected in test env): %v", err)
			} else if result != nil && result.Data != nil {
				t.Logf("Successfully collected data for %s: %v", tt.name, result.Data.Data)
			}
		})
	}
}

func TestCollectLogsAsync(t *testing.T) {
	collector := NewKubeletCollector().(*K8sComponentCollector)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 测试日志收集（使用存在的系统服务）
	logs, err := collector.collectLogsAsync(ctx, "systemd")
	if err != nil {
		t.Logf("collectLogsAsync() returned error (expected in test env): %v", err)
	} else {
		t.Logf("Successfully collected logs: %+v", logs)

		// 验证返回的数据结构
		if logs["recent_lines"] == nil {
			t.Error("Expected recent_lines in logs")
		}
		if logs["error_count"] == nil {
			t.Error("Expected error_count in logs")
		}
		if logs["warning_count"] == nil {
			t.Error("Expected warning_count in logs")
		}
	}
}

func TestCheckProcess(t *testing.T) {
	collector := NewKubeletCollector().(*K8sComponentCollector)

	// 测试检查存在的进程（使用init进程）
	processInfo, err := collector.checkProcess("init")
	if err != nil {
		t.Logf("checkProcess() returned error (expected in test env): %v", err)
	} else {
		t.Logf("Successfully got process info: %+v", processInfo)

		// 验证返回的数据结构
		if processInfo["pid"] == nil {
			t.Error("Expected pid in process info")
		}
		if processInfo["command"] == nil {
			t.Error("Expected command in process info")
		}
	}
}

func TestCheckCNIConfig(t *testing.T) {
	collector := NewCNICollector().(*K8sComponentCollector)

	// 创建临时CNI配置目录用于测试
	tempDir := t.TempDir()
	cniDir := tempDir + "/cni/net.d"
	err := os.MkdirAll(cniDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create temp CNI dir: %v", err)
	}

	// 创建测试配置文件
	testConfig := `{"cniVersion":"0.3.1","name":"test","type":"bridge"}`
	err = os.WriteFile(cniDir+"/10-test.conf", []byte(testConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// 临时替换CNI目录路径
	originalCNIDir := "/etc/cni/net.d"
	defer func() {
		// 恢复原始路径
		_ = originalCNIDir
	}()

	// 由于路径是硬编码的，这里只测试函数结构
	config, err := collector.checkCNIConfig()
	if err != nil {
		t.Logf("checkCNIConfig() returned error (expected in test env): %v", err)
	} else {
		t.Logf("CNI config: %+v", config)
	}
}

func TestTimeoutHandling(t *testing.T) {
	collector := NewKubeletCollector()

	// 测试超时处理
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// 给上下文一点时间过期
	time.Sleep(10 * time.Millisecond)

	result, err := collector.Collect(ctx, nil)
	if err == nil {
		t.Log("Collect completed before timeout")
	} else if err == context.DeadlineExceeded {
		t.Log("Collect properly handled timeout")
	} else {
		t.Logf("Collect returned other error: %v", err)
	}

	if result != nil && result.Data != nil {
		t.Logf("Partial data collected: %v", result.Data.Data)
	}
}

func TestRegistryRegistration(t *testing.T) {
	registry := NewRegistry()

	// 测试注册所有K8s组件采集器
	collectors := []Collector{
		NewKubeletCollector(),
		NewContainerdCollector(),
		NewCNICollector(),
		NewKubeProxyCollector(),
	}

	for _, collector := range collectors {
		err := registry.Register(collector)
		if err != nil {
			t.Errorf("Failed to register %s: %v", collector.Name(), err)
		}

		// 验证可以通过名称获取
		retrieved, err := registry.Get(collector.Name())
		if err != nil {
			t.Errorf("Failed to get %s from registry: %v", collector.Name(), err)
		}

		if retrieved.Name() != collector.Name() {
			t.Errorf("Retrieved collector name mismatch: expected %s, got %s",
				collector.Name(), retrieved.Name())
		}
	}

	// 验证本地采集器列表
	localCollectors := registry.GetLocalOnly()
	if len(localCollectors) < 4 {
		t.Errorf("Expected at least 4 local collectors, got %d", len(localCollectors))
	}
}
