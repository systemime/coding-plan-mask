# Coding Plan Proxy Makefile

# 变量
APP_NAME := coding-plan-proxy
VERSION := 0.2.0
BUILD_DIR := build
BIN_DIR := $(BUILD_DIR)/bin
CMD_DIR := cmd/coding-plan-proxy
INSTALL_DIR := /opt/project/$(APP_NAME)
CONFIG_DIR := /opt/project/$(APP_NAME)/config

# Go 参数
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod

# 构建信息
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# 链接参数
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -X main.date=$(BUILD_TIME)"

.PHONY: all build clean install uninstall test run fmt vet help

all: clean build

## build: 构建二进制文件
build:
	@echo "构建 $(APP_NAME) v$(VERSION)..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME) ./$(CMD_DIR)
	@echo "构建完成: $(BIN_DIR)/$(APP_NAME)"

## build-linux: 构建 Linux amd64 版本
build-linux:
	@echo "构建 Linux amd64 版本..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 ./$(CMD_DIR)

## build-arm64: 构建 Linux arm64 版本
build-arm64:
	@echo "构建 Linux arm64 版本..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-linux-arm64 ./$(CMD_DIR)

## clean: 清理构建文件
clean:
	@echo "清理构建文件..."
	@rm -rf $(BUILD_DIR)
	$(GOCLEAN)

## install: 安装到系统
install: build
	@echo "安装 $(APP_NAME)..."
	@mkdir -p $(INSTALL_DIR)/bin
	@mkdir -p $(INSTALL_DIR)/deploy
	@mkdir -p $(CONFIG_DIR)
	@mkdir -p /var/log/$(APP_NAME)
	@cp $(BIN_DIR)/$(APP_NAME) $(INSTALL_DIR)/bin/
	@cp deploy/proxy-ctl.sh $(INSTALL_DIR)/deploy/
	@chmod +x $(INSTALL_DIR)/deploy/proxy-ctl.sh
	@if [ ! -f $(CONFIG_DIR)/config.toml ]; then \
		cp deploy/config.example.toml $(CONFIG_DIR)/config.toml; \
		echo "已创建默认配置文件: $(CONFIG_DIR)/config.toml"; \
	fi
	@cp deploy/$(APP_NAME).service /etc/systemd/system/
	@ln -sf $(INSTALL_DIR)/deploy/proxy-ctl.sh /usr/local/bin/proxy-ctl
	@systemctl daemon-reload
	@echo "安装完成"
	@echo ""
	@echo "使用方法:"
	@echo "  编辑配置: vim $(CONFIG_DIR)/config.toml"
	@echo "  启动服务: proxy-ctl start 或 systemctl start $(APP_NAME)"
	@echo "  查看连接: proxy-ctl info"
	@echo "  开机自启: proxy-ctl enable"
	@echo "  查看日志: proxy-ctl logs"

## uninstall: 从系统卸载
uninstall:
	@echo "卸载 $(APP_NAME)..."
	@systemctl stop $(APP_NAME) 2>/dev/null || true
	@systemctl disable $(APP_NAME) 2>/dev/null || true
	@rm -f /etc/systemd/system/$(APP_NAME).service
	@rm -f /usr/local/bin/proxy-ctl
	@systemctl daemon-reload
	@rm -rf $(INSTALL_DIR)
	@rm -rf /var/log/$(APP_NAME)
	@echo "卸载完成"

## test: 运行测试
test:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

## coverage: 查看测试覆盖率
coverage: test
	$(GOCMD) tool cover -html=coverage.out

## run: 本地运行
run: build
	$(BIN_DIR)/$(APP_NAME) -debug

## fmt: 格式化代码
fmt:
	$(GOCMD) fmt ./...

## vet: 静态检查
vet:
	$(GOCMD) vet ./...

## deps: 下载依赖
deps:
	$(GOMOD) download
	$(GOMOD) verify

## tidy: 整理依赖
tidy:
	$(GOMOD) tidy

## help: 显示帮助信息
help:
	@echo "Coding Plan Proxy v$(VERSION)"
	@echo ""
	@echo "使用方法:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

.DEFAULT_GOAL := help
