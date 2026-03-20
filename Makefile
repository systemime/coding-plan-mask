# Coding Plan Mask Makefile

# Variables
APP_NAME := mask-ctl
SERVICE_BINARY := coding-plan-mask
VERSION := 0.7.2
BUILD_DIR := build
BIN_DIR := $(BUILD_DIR)/bin
CMD_DIR := cmd/coding-plan-mask
INSTALL_DIR := /opt/project/coding-plan-mask
CONFIG_DIR := /opt/project/coding-plan-mask/config

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod

# Build info
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Linker flags
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -X main.date=$(BUILD_TIME)"

.PHONY: all build clean install uninstall test test-race run fmt vet help release

all: clean build

build:
	@echo "Building $(APP_NAME) v$(VERSION)..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME) ./$(CMD_DIR)
	@echo "Build complete: $(BIN_DIR)/$(APP_NAME)"

release:
	@echo "Cross-compiling all platforms v$(VERSION)..."
	@mkdir -p $(BIN_DIR)
	@echo "Building Linux amd64..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 ./$(CMD_DIR)
	@echo "Building Linux arm64..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-linux-arm64 ./$(CMD_DIR)
	@echo "Building Darwin amd64..."
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-darwin-amd64 ./$(CMD_DIR)
	@echo "Building Darwin arm64..."
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-darwin-arm64 ./$(CMD_DIR)
	@echo "Building Windows amd64..."
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-windows-amd64.exe ./$(CMD_DIR)
	@echo "Building Windows arm64..."
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-windows-arm64.exe ./$(CMD_DIR)
	@echo ""
	@echo "Cross-compilation complete!"
	@ls -la $(BIN_DIR)/

build-linux:
	@echo "Building Linux amd64..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 ./$(CMD_DIR)

build-arm64:
	@echo "Building Linux arm64..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-linux-arm64 ./$(CMD_DIR)

clean:
	@echo "Cleaning build files..."
	@rm -rf $(BUILD_DIR)
	$(GOCLEAN)

install: build
	@echo "Installing $(APP_NAME)..."
	@mkdir -p $(INSTALL_DIR)/bin
	@mkdir -p $(INSTALL_DIR)/deploy
	@mkdir -p $(CONFIG_DIR)
	@mkdir -p /var/log/coding-plan-mask
	@cp $(BIN_DIR)/$(APP_NAME) $(INSTALL_DIR)/bin/$(SERVICE_BINARY)
	@ln -sf $(SERVICE_BINARY) $(INSTALL_DIR)/bin/$(APP_NAME)
	@cp deploy/mask-ctl.sh $(INSTALL_DIR)/deploy/ 2>/dev/null || true
	@chmod +x $(INSTALL_DIR)/deploy/mask-ctl.sh 2>/dev/null || true
	@if [ ! -f $(CONFIG_DIR)/config.toml ]; then \
		cp deploy/config.example.toml $(CONFIG_DIR)/config.toml 2>/dev/null || true; \
	fi
	@ln -sf $(INSTALL_DIR)/bin/$(SERVICE_BINARY) /usr/local/bin/$(APP_NAME)
	@ln -sf $(INSTALL_DIR)/bin/$(SERVICE_BINARY) /usr/local/bin/$(SERVICE_BINARY)
	@echo "Installation complete"
	@echo ""
	@echo "Usage:"
	@echo "  Config: $(CONFIG_DIR)/config.toml"
	@echo "  Start:  $(APP_NAME)"
	@echo "  Info:   $(APP_NAME) info"

uninstall:
	@echo "Uninstalling $(APP_NAME)..."
	@rm -f /usr/local/bin/$(APP_NAME)
	@rm -f /usr/local/bin/$(SERVICE_BINARY)
	@rm -rf $(INSTALL_DIR)
	@rm -rf /var/log/coding-plan-mask
	@echo "Uninstall complete"

test:
	CGO_ENABLED=0 $(GOTEST) -v -coverprofile=coverage.out ./...

test-race:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

run: build
	$(BIN_DIR)/$(APP_NAME) -debug

fmt:
	$(GOCMD) fmt ./...

vet:
	$(GOCMD) vet ./...

deps:
	$(GOMOD) download
	$(GOMOD) verify

tidy:
	$(GOMOD) tidy

help:
	@echo "Coding Plan Mask v$(VERSION)"
	@echo ""
	@echo "Targets: build, release, clean, install, uninstall, test, run"

.DEFAULT_GOAL := help
