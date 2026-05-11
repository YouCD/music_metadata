# 项目名称
BINARY_NAME=music_metadata

# Go 相关配置
GO=go
GOCMD=$(GO) build
GOGET=$(GO) get
GOMOD=$(GO) mod
GOTEST=$(GO) test

# 构建配置
OUTPUT_DIR=bin
LDFLAGS=-ldflags "-s -w"

# UPX 压缩配置
UPX=upx
UPX_FLAGS=--lzma --best

# 平台配置
OS=$(shell uname -s | tr A-Z a-z)
ARCH_RAW=$(shell uname -m)
# 将架构名称转换为 Go 的命名规范
ifeq ($(ARCH_RAW),x86_64)
    ARCH=amd64
else ifeq ($(ARCH_RAW),aarch64)
    ARCH=arm64
else
    ARCH=$(ARCH_RAW)
endif

# 默认目标
.PHONY: all
all: help

# 帮助信息
.PHONY: help
help:
	@echo "🎵 Music Metadata Tool - Makefile Commands"
	@echo ""
	@echo "Usage:"
	@echo "  make build        - 构建当前平台的二进制文件"
	@echo "  make build-all    - 构建所有平台的二进制文件"
	@echo "  make compress     - 使用 UPX 压缩当前平台的二进制文件"
	@echo "  make clean        - 清理构建产物"
	@echo "  make deps         - 下载依赖"
	@echo "  make tidy         - 整理依赖"
	@echo "  make install      - 安装到 GOPATH/bin"
	@echo "  make help         - 显示此帮助信息"

# 创建输出目录
$(OUTPUT_DIR):
	mkdir -p $(OUTPUT_DIR)

# 构建当前平台的二进制文件
.PHONY: build
build: $(OUTPUT_DIR)
	@echo "🔨 Building for $(OS)/$(ARCH)..."
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) $(GOCMD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-$(OS)-$(ARCH) .
	@echo "✅ Build completed: $(OUTPUT_DIR)/$(BINARY_NAME)-$(OS)-$(ARCH)"

# 构建多个平台的二进制文件
.PHONY: build-all
build-all: $(OUTPUT_DIR)
	@echo "🔨 Building for multiple platforms..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOCMD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-linux-amd64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOCMD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-linux-arm64 .
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOCMD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-darwin-amd64 .
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOCMD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-darwin-arm64 .
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOCMD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-windows-amd64.exe .
	@echo "✅ Multi-platform build completed"

# 使用 UPX 压缩当前平台的二进制文件
.PHONY: compress
compress: build
	@echo "🗜️ Compressing binary with UPX..."
	@if command -v upx >/dev/null 2>&1; then \
		$(UPX) $(UPX_FLAGS) $(OUTPUT_DIR)/$(BINARY_NAME)-$(OS)-$(ARCH); \
		echo "✅ Compression completed"; \
	else \
		echo "❌ UPX is not installed. Please install UPX first."; \
		exit 1; \
	fi

# 构建并压缩
.PHONY: release
release: compress
	@echo "📦 Release build completed with UPX compression"

# 清理构建产物
.PHONY: clean
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf $(OUTPUT_DIR)
	@echo "✅ Clean completed"

# 下载依赖
.PHONY: deps
deps:
	@echo "📥 Downloading dependencies..."
	$(GOGET) ./...
	@echo "✅ Dependencies downloaded"

# 整理依赖
.PHONY: tidy
tidy:
	@echo "🧹 Tidying dependencies..."
	$(GOMOD) tidy
	@echo "✅ Dependencies tidied"

# 安装到 GOPATH/bin
.PHONY: install
install:
	@echo "📦 Installing to GOPATH/bin..."
	$(GOCMD) -o $(GOPATH)/bin/$(BINARY_NAME) .
	@echo "✅ Installed to $(GOPATH)/bin/$(BINARY_NAME)"