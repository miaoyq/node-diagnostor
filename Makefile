# Makefile for Kubernetes Node Diagnostor
# 构建和管理node-diagnostor工具的Makefile

# 变量定义
BINARY_NAME := node-diagnostor
OUTPUT_DIR := bin
INTEGRATION_TEST_DIR := test
INTEGRATION_TEST_SCRIPT := $(INTEGRATION_TEST_DIR)/integration_test.sh

# 默认目标
.PHONY: all
all: build

# 构建应用
.PHONY: build
build: ## 构建node-diagnostor二进制文件
	@echo "Building $(BINARY_NAME)..."
	go build -o $(OUTPUT_DIR)/$(BINARY_NAME) ./cmd/node-diagnostor
	@echo "Build complete: $(OUTPUT_DIR)/$(BINARY_NAME)"

# 运行应用（构建后执行）
.PHONY: run
run: build ## 构建并运行应用（dry-run模式）
	@echo "Running $(BINARY_NAME) in dry-run mode..."
	./$(OUTPUT_DIR)/$(BINARY_NAME) --config config.example.json --dry-run

# 运行所有单元测试
.PHONY: test
test: ## 运行所有单元测试
	@echo "Running unit tests..."
	go test ./...

# 运行单独的单元测试（不运行集成测试）
.PHONY: unit-test
unit-test: ## 仅运行单元测试（排除集成测试）
	@echo "Running unit tests only..."
	go test ./internal/... ./pkg/...

# 运行集成测试
.PHONY: integration-test
integration-test: build ## 运行集成测试
	@echo "Running integration tests..."
	@if [ -f "$(INTEGRATION_TEST_SCRIPT)" ]; then \
		chmod +x $(INTEGRATION_TEST_SCRIPT); \
		$(INTEGRATION_TEST_SCRIPT); \
	else \
		echo "Error: Integration test script not found at $(INTEGRATION_TEST_SCRIPT)"; \
		exit 1; \
	fi

# 清理构建产物
.PHONY: clean
clean: ## 清理构建产物
	@echo "Cleaning build artifacts..."
	rm -rf $(OUTPUT_DIR)

# 显示帮助信息
.PHONY: help
help: ## 显示可用的make目标
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

# 开发常用命令
.PHONY: dev
dev: build run ## 开发模式：构建并运行

# 完整测试（单元测试 + 集成测试）
.PHONY: test-all
test-all: unit-test integration-test ## 运行所有测试（单元测试和集成测试）

# 快速测试（仅单元测试）
.PHONY: test-quick
test-quick: unit-test ## 快速测试（仅单元测试，不包含集成测试）