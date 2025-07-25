# Makefile for Kubernetes Node Diagnostor
# 构建和管理node-diagnostor工具的Makefile

# 变量定义
BINARY_NAME := node-diagnostor
OUTPUT_DIR := bin
INTEGRATION_TEST_DIR := test
INTEGRATION_TEST_SCRIPT := $(INTEGRATION_TEST_DIR)/integration_test.sh

# 版本信息
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT ?= $(shell git rev-parse HEAD)
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Go参数
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=$(GOPATH)/bin/golangci-lint

.PHONY: build test clean fmt lint deps help

# 默认目标
all: build

# 构建二进制
build:
	@echo "Building k8s-node-diagnostor..."
	go build -o bin/node-diagnostor ./cmd/node-diagnostor/

# 运行测试
test:
	@echo "Running tests..."
	go test -v ./...

# 运行测试并生成覆盖率报告
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# 格式化代码
fmt:
	@echo "Formatting code..."
	go fmt ./...

# 运行静态检查
lint:
	@echo "Running linter..."
	golangci-lint run

# 安装依赖
deps:
	@echo "Installing dependencies..."
	go mod tidy
	go mod download

# 清理构建产物
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# 安装开发工具
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# 显示帮助
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter"
	@echo "  deps         - Install/update dependencies"
	@echo "  clean        - Clean build artifacts"
	@echo "  install-tools- Install development tools"
	@echo "  help         - Show this help"