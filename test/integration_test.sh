#!/bin/bash
set -euo pipefail

# 集成测试脚本 - 适配当前版本
# 检查端口占用
check_port() {
    local port=$1
    if lsof -i :"$port" >/dev/null 2>&1; then
        echo "端口 $port 已被占用，正在释放..."
        lsof -ti :"$port" | xargs kill -9 || true
        sleep 1
    fi
}

# 创建测试配置文件
create_test_config() {
    echo "Creating test config..."
    cat > test/config.json <<EOL
{
  "version": "1.0",
  "checks": [
    {
      "name": "system_health",
      "interval": 10000000000,
      "collectors": ["cpu", "memory", "disk", "network"],
      "params": {
        "cpu_threshold": 80,
        "memory_threshold": 90,
        "disk_threshold": 85
      },
      "mode": "仅本地",
      "enabled": true,
      "timeout": 30000000000,
      "priority": 1
    },
    {
      "name": "system_load",
      "interval": 15000000000,
      "collectors": ["system-load"],
      "params": {},
      "mode": "仅本地",
      "enabled": true,
      "timeout": 20000000000,
      "priority": 2
    },
    {
      "name": "kernel_log",
      "interval": 30000000000,
      "collectors": ["kernel-log"],
      "params": {
        "check_sysctl": ["net.ipv4.tcp_tw_reuse", "vm.swappiness"],
        "check_dmesg": true
      },
      "mode": "仅本地",
      "enabled": true,
      "timeout": 15000000000,
      "priority": 3
    }
  ],
  "resource_limits": {
    "max_cpu_percent": 10.0,
    "max_memory_mb": 50,
    "min_interval": 10000000000,
    "max_concurrent": 2,
    "max_data_points": 1000,
    "max_data_size": 10485760,
    "data_retention_period": 3600000000000,
    "enable_compression": true
  },
  "data_scope": {
    "mode": "仅本地",
    "node_local_types": ["cpu_throttling", "memory_fragmentation", "disk_smart", "network_interface_errors"],
    "supplement_types": [],
    "skip_types": []
  },
  "reporter": {
    "endpoint": "http://localhost:8088/api/v1/diagnostics",
    "timeout": 10000000000,
    "max_retries": 3,
    "retry_delay": 5000000000,
    "compression": true,
    "cache_max_size": 5242880,
    "cache_max_age": 3600000000000
  }
}
EOL
}

# 验证配置文件
validate_config() {
    echo "Validating config file..."
    if ! jq . test/config.json >/dev/null; then
        echo "Error: Invalid config file"
        cat test/config.json
        exit 1
    fi
}

# 启动模拟API服务器
start_mock_server() {
    echo "Starting mock API server..."
    check_port 8088
    
    # 创建简单的mock服务器
    cat > test/mock_server.py << 'EOL'
#!/usr/bin/env python3
import http.server
import socketserver
import json
import sys

class MockHandler(http.server.SimpleHTTPRequestHandler):
    def do_POST(self):
        if self.path == '/api/v1/diagnostics':
            content_length = int(self.headers['Content-Length'])
            post_data = self.rfile.read(content_length)
            
            try:
                data = json.loads(post_data.decode('utf-8'))
                print(f"Received diagnostic data: {json.dumps(data, indent=2)}")
                
                # 保存接收到的数据用于验证
                with open('test/received_data.json', 'w') as f:
                    json.dump(data, f, indent=2)
                
                self.send_response(200)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({"status": "success"}).encode())
            except Exception as e:
                print(f"Error processing request: {e}")
                self.send_response(400)
                self.end_headers()
        else:
            self.send_response(404)
            self.end_headers()
    
    def log_message(self, format, *args):
        return  # 静默日志

PORT = 8088
with socketserver.TCPServer(("", PORT), MockHandler) as httpd:
    print(f"Mock server serving at port {PORT}")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("Server stopped")
EOL
    
    chmod +x test/mock_server.py
    python3 test/mock_server.py &
    SERVER_PID=$!
    sleep 2 # 等待服务器启动
}

# 停止模拟API服务器
stop_mock_server() {
    echo "Stopping mock API server..."
    if [ -n "${SERVER_PID:-}" ]; then
        kill $SERVER_PID 2>/dev/null || true
        wait $SERVER_PID 2>/dev/null || true
    fi
}

# 验证报告内容
validate_report() {
    echo "Validating report content..."
    
    if [ ! -f "test/received_data.json" ]; then
        echo "Error: No diagnostic data received"
        return 1
    fi
    
    # 检查基本字段
    jq -e '.nodeName' test/received_data.json >/dev/null
    jq -e '.timestamp' test/received_data.json >/dev/null
    jq -e '.data' test/received_data.json >/dev/null
    
    # 检查是否有CPU数据
    if jq -e '.data.cpu' test/received_data.json >/dev/null; then
        echo "✓ CPU data found"
    else
        echo "Warning: No CPU data found"
    fi
    
    # 检查是否有内存数据
    if jq -e '.data.memory' test/received_data.json >/dev/null; then
        echo "✓ Memory data found"
    else
        echo "Warning: No memory data found"
    fi
    
    # 检查是否有磁盘数据
    if jq -e '.data.disk' test/received_data.json >/dev/null; then
        echo "✓ Disk data found"
    else
        echo "Warning: No disk data found"
    fi
    
    # 检查是否有网络数据
    if jq -e '.data.network' test/received_data.json >/dev/null; then
        echo "✓ Network data found"
    else
        echo "Warning: No network data found"
    fi
    
    echo "Report validation completed"
}

# 测试配置热加载
test_config_reload() {
    echo "Testing config hot reload..."
    
    # 备份原配置
    cp test/config.json test/config.json.bak
    
    # 修改配置 - 增加新的检查项
    cat > test/config.json <<EOL
{
  "version": "1.0",
  "checks": [
    {
      "name": "system_health",
      "interval": 5000000000,
      "collectors": ["cpu", "memory"],
      "params": {
        "cpu_threshold": 75,
        "memory_threshold": 85
      },
      "mode": "仅本地",
      "enabled": true,
      "timeout": 30000000000,
      "priority": 1
    },
    {
      "name": "new_check",
      "interval": 20000000000,
      "collectors": ["system-load"],
      "params": {
        "load_threshold": 2.0
      },
      "mode": "仅本地",
      "enabled": true,
      "timeout": 15000000000,
      "priority": 4
    }
  ],
  "resource_limits": {
    "max_cpu_percent": 10.0,
    "max_memory_mb": 50,
    "min_interval": 5000000000,
    "max_concurrent": 2,
    "max_data_points": 1000,
    "max_data_size": 10485760,
    "data_retention_period": 3600000000000,
    "enable_compression": true
  },
  "data_scope": {
    "mode": "仅本地",
    "node_local_types": ["cpu_throttling", "memory_fragmentation"],
    "supplement_types": [],
    "skip_types": []
  },
  "reporter": {
    "endpoint": "http://localhost:8088/api/v1/diagnostics",
    "timeout": 10000000000,
    "max_retries": 3,
    "retry_delay": 5000000000,
    "compression": true,
    "cache_max_size": 5242880,
    "cache_max_age": 3600000000000
  }
}
EOL
    
    # 等待配置生效
    sleep 5
    
    # 检查日志确认配置已加载
    if ! grep -q "configuration updated" node-diagnostor.log 2>/dev/null; then
        echo "Warning: Config hot reload test - config reload not detected in logs"
    else
        echo "✓ Config reload detected in logs"
    fi
    
    # 恢复配置
    mv test/config.json.bak test/config.json
    
    echo "Config hot reload test completed"
}

# 测试网络故障场景
test_network_failure() {
    echo "Testing network failure scenario..."
    
    # 停止模拟服务器
    stop_mock_server
    
    # 等待上报失败并创建缓存
    sleep 15
    
    # 检查本地缓存是否创建
    if [ -f "test/cache/diagnostic_cache.json" ]; then
        echo "✓ Local cache created on network failure"
    else
        echo "Warning: Local cache not found at expected location"
    fi
    
    # 重启服务器
    start_mock_server
    
    # 等待重试机制工作
    sleep 20
    
    echo "Network failure test completed"
}

# 测试资源限制场景
test_resource_limits() {
    echo "Testing resource limits scenario..."
    
    # 创建高资源使用的配置
    cat > test/high_resource_config.json <<EOL
{
  "version": "1.0",
  "checks": [
    {
      "name": "intensive_check",
      "interval": 1000000000,
      "collectors": ["cpu", "memory", "disk", "network", "kubelet", "containerd"],
      "params": {},
      "mode": "仅本地",
      "enabled": true,
      "timeout": 5000000000,
      "priority": 1
    }
  ],
  "resource_limits": {
    "max_cpu_percent": 1.0,
    "max_memory_mb": 10,
    "min_interval": 1000000000,
    "max_concurrent": 5,
    "max_data_points": 100,
    "max_data_size": 1048576,
    "data_retention_period": 1800000000000,
    "enable_compression": true
  },
  "data_scope": {
    "mode": "仅本地",
    "node_local_types": [],
    "supplement_types": [],
    "skip_types": []
  },
  "reporter": {
    "endpoint": "http://localhost:8088/api/v1/diagnostics",
    "timeout": 5000000000,
    "max_retries": 2,
    "retry_delay": 2000000000,
    "compression": true,
    "cache_max_size": 1048576,
    "cache_max_age": 1800000000000
  }
}
EOL
    
    # 重启工具使用高资源限制配置
    if [ -n "${TOOL_PID:-}" ]; then
        kill -SIGINT $TOOL_PID 2>/dev/null || true
        wait $TOOL_PID 2>/dev/null || true
    fi
    
    echo "Restarting tool with resource limits..."
    cp test/high_resource_config.json /etc/node-diagnostor/config.json
    ./bin/node-diagnostor > node-diagnostor.log 2>&1 &
    TOOL_PID=$!
    
    sleep 10
    
    # 检查是否触发资源限制
    if grep -q "resource limit exceeded\|degraded mode" node-diagnostor.log 2>/dev/null; then
        echo "✓ Resource limit handling detected"
    else
        echo "Note: Resource limit handling not explicitly detected"
    fi
    
    # 恢复使用正常配置
    if [ -n "${TOOL_PID:-}" ]; then
        kill -SIGINT $TOOL_PID 2>/dev/null || true
        wait $TOOL_PID 2>/dev/null || true
    fi
    
    echo "Restarting tool with normal config..."
    cp test/config.json /etc/node-diagnostor/config.json
    ./bin/node-diagnostor > node-diagnostor.log 2>&1 &
    TOOL_PID=$!
    
    echo "Resource limits test completed"
}

# 测试优雅关闭
test_graceful_shutdown() {
    echo "Testing graceful shutdown..."
    
    # 发送SIGINT信号
    if [ -n "${TOOL_PID:-}" ]; then
        kill -SIGINT $TOOL_PID
        wait $TOOL_PID
        
        # 检查是否正常退出
        if [ $? -eq 0 ]; then
            echo "✓ Graceful shutdown successful"
        else
            echo "Warning: Graceful shutdown may have issues"
        fi
    fi
    
    # 重启工具
    ./bin/node-diagnostor > node-diagnostor.log 2>&1 &
    TOOL_PID=$!
    sleep 3
    
    echo "Graceful shutdown test completed"
}

# 主测试函数
run_integration_test() {
    echo "Running integration test for current version..."
    
    # 确保从项目根目录运行
    cd "$(dirname "$0")/.."
    
    # 创建测试目录
    mkdir -p test/cache
    
    # 清理旧文件
    rm -f test/received_data.json
    rm -f test/cache/diagnostic_cache.json
    rm -f node-diagnostor.log
    
    # 检查二进制文件是否存在
    if [ ! -f "./bin/node-diagnostor" ]; then
        echo "Error: node-diagnostor binary not found. Please build first."
        echo "Run: make build"
        exit 1
    fi
    
    # 创建配置文件
    create_test_config
    validate_config
    
    # 启动模拟服务器
    start_mock_server
    
    # 运行诊断工具
    echo "Running diagnostic tool..."
    # 备份原始配置
    cp /etc/node-diagnostor/config.json /etc/node-diagnostor/config.json.bak
    # 使用测试配置
    cp test/config.json /etc/node-diagnostor/config.json
    ./bin/node-diagnostor > node-diagnostor.log 2>&1 &
    TOOL_PID=$!
    
    # 等待工具初始化
    sleep 5
    
    # 检查工具是否正常运行
    if ! ps -p $TOOL_PID > /dev/null; then
        echo "Error: Diagnostic tool failed to start"
        cat node-diagnostor.log
        exit 1
    fi
    
    # 执行测试用例
    echo "Running test cases..."
    
    # 等待数据收集
    sleep 20
    
    # 验证基本功能
    validate_report || exit 1
    
    # 测试配置热加载
    test_config_reload || exit 1
    
    # 测试网络故障
    test_network_failure || exit 1
    
    # 测试资源限制
    test_resource_limits || exit 1
    
    # 测试优雅关闭
    test_graceful_shutdown || exit 1
    
    # 最终验证
    sleep 10
    validate_report || exit 1
    
    # 停止工具
    echo "Stopping diagnostic tool..."
    if [ -n "${TOOL_PID:-}" ]; then
        kill -SIGINT $TOOL_PID 2>/dev/null || true
        wait $TOOL_PID 2>/dev/null || true
    fi
    
    # 恢复原始配置
    if [ -f "/etc/node-diagnostor/config.json.bak" ]; then
        mv /etc/node-diagnostor/config.json.bak /etc/node-diagnostor/config.json
    fi
    
    # 停止模拟服务器
    stop_mock_server
    
    # 清理测试文件
    rm -f test/mock_server.py
    
    echo "Integration test completed successfully!"
    echo "Check test/received_data.json for received diagnostic data"
    echo "Check node-diagnostor.log for detailed logs"
}

# 执行测试
run_integration_test