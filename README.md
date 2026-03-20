<div align="center">

# 🎭 Coding Plan Mask

**Unlock Your Coding Plan API Key for Any AI Coding Tool**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-0.6.0-green.svg)](https://github.com/systemime/coding-plan-mask)

*Use your Coding Plan subscription with ANY OpenAI-compatible coding tool*

[English](#-english-documentation) | [中文文档](#-中文文档)

</div>

---

## 📖 English Documentation

### 😤 The Problem: Coding Plan Restrictions

Major AI providers (Zhipu GLM, Alibaba Cloud, MiniMax, DeepSeek, Moonshot, etc.) offer **Coding Plan** subscriptions at attractive prices, but with **severe usage restrictions**:

| What You Pay For | What You Actually Get |
|------------------|----------------------|
| ✅ Fixed monthly fee, unlimited coding | ❌ **Only works with specific IDE tools** |
| ✅ Access to powerful models | ❌ **Cannot use in your favorite tools** |
| ✅ Official API Key provided | ❌ **Cannot use for automation/backend** |

#### 🔒 Official Restrictions

| Allowed | Forbidden |
|---------|-----------|
| ✅ Claude Code, Cursor, Cline | ❌ Your own AI tools |
| ✅ VS Code extensions | ❌ Custom scripts |
| ✅ Interactive coding | ❌ Automated workflows |
| | ❌ Backend integration |
| | ❌ Dify, FastGPT platforms |

**Violation Consequence**: Subscription suspension or API Key ban

#### 📊 Provider Comparison

| Provider | Monthly Fee | Models | Can Use in Custom Tools? |
|----------|-------------|--------|-------------------------|
| Zhipu GLM | $3-15+ | GLM-4.7, GLM-5 | ❌ No |
| Alibaba Cloud | $5.80-29 | Qwen, GLM, MiniMax, Kimi | ❌ No |
| MiniMax | Subscription | M2.1 (not M2.5!) | ❌ No |
| DeepSeek | Subscription | DeepSeek V3 | ❌ No |
| Moonshot | Subscription | Kimi | ❌ No |

### 💡 The Solution: Coding Plan Mask

**Coding Plan Mask** acts as a bridge between your Coding Plan API and any OpenAI-compatible tool. It **masks** your requests to appear as if they come from officially supported IDE tools.

```
┌────────────────────┐     ┌──────────────────────┐     ┌─────────────────────┐
│  Your Favorite AI  │────▶│   Coding Plan Mask   │────▶│   LLM Provider      │
│  Tool (Any!)       │◀────│   (Tool Masking)     │◀────│   (Thinks it's OK)  │
└────────────────────┘     └──────────────────────┘     └─────────────────────┘
```

### ✨ Key Features

| Feature | Description |
|---------|-------------|
| 🎭 **Tool Masking** | Mask as Claude Code, Kimi Code, OpenClaw or custom tool |
| 🔀 **Request Relay** | Relay requests to Coding Plan API endpoints |
| 🔌 **Universal Compatibility** | Works with ANY OpenAI-compatible client |
| 🌐 **Multi-Provider** | Support for 6+ major LLM providers |
| 📊 **Usage Analytics** | Track token consumption in real-time with SQLite storage |
| 🔒 **Local Auth** | Protect your proxy with custom API key |
| ⚡ **High Performance** | Built in Go for maximum efficiency |
| 🔧 **Flexible Configuration** | Support TOML config file, environment variables, and custom API URLs |
| 📈 **Rate Limiting** | Built-in rate limiting to prevent abuse |

### 🚀 Quick Start

#### 1. Install

**Download from Releases (Recommended)**

Download the binary for your platform from [GitHub Releases](https://github.com/systemime/coding-plan-mask/releases):

```bash
# Linux amd64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.6.0/mask-ctl-linux-amd64
chmod +x mask-ctl-linux-amd64
sudo mv mask-ctl-linux-amd64 /usr/local/bin/mask-ctl

# Linux arm64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.6.0/mask-ctl-linux-arm64
chmod +x mask-ctl-linux-arm64
sudo mv mask-ctl-linux-arm64 /usr/local/bin/mask-ctl

# macOS (Darwin)
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.6.0/mask-ctl-darwin-arm64
chmod +x mask-ctl-darwin-arm64
sudo mv mask-ctl-darwin-arm64 /usr/local/bin/mask-ctl

# Windows
# Download mask-ctl-windows-amd64.exe from releases
```

**Build from Source**

```bash
git clone https://github.com/systemime/coding-plan-mask.git
cd coding-plan-mask

# Build for current platform
make build

# Cross-compile for all platforms
make release
```

#### 2. First Run

```bash
mask-ctl
```

On first run, a default configuration file will be created at `/opt/project/coding-plan-mask/config/config.toml`. Edit it to fill in your credentials:

```bash
vim /opt/project/coding-plan-mask/config/config.toml
```

#### 3. Configure

Edit the configuration file:

```toml
[server]
listen_host = "127.0.0.1"
listen_port = 8787
timeout = 120                       # Request timeout (seconds)
rate_limit_requests = 100           # Rate limit per 5 minutes

[auth]
provider = "zhipu"                  # Your Coding Plan provider
api_key = "your-coding-plan-api-key"  # Your Coding Plan API Key
local_api_key = "sk-local-secret"   # Key for your tools to use

[endpoint]
use_coding_endpoint = true
disguise_tool = "claudecode"        # Mask as Claude Code (recommended)
```

#### 4. Start

```bash
# Start the proxy server
mask-ctl

# Or with systemd (after make install)
sudo systemctl start coding-plan-mask
```

#### 5. Use with Any Tool

Configure your AI coding tool to use:

```json
{
    "base_url": "http://127.0.0.1:8787/v1",
    "api_key": "sk-local-secret",
    "model": "glm-4-flash"
}
```

### 🤖 Supported Providers

| Provider | Identifier | Models |
|----------|------------|--------|
| **Zhipu GLM** | `zhipu` | glm-4-flash, glm-4-plus, glm-4-air, glm-4-long |
| **Zhipu GLM v2** | `zhipu_v2` | glm-4-flash, glm-4-plus, glm-4-air, glm-4-long, glm-4.7, glm-5 |
| **Alibaba Cloud** | `aliyun` | qwen-turbo, qwen-plus, qwen-max, qwen2.5-coder-32b-instruct |
| **MiniMax** | `minimax` | abab6.5s-chat, abab6.5g-chat, abab6.5-chat |
| **DeepSeek** | `deepseek` | deepseek-chat, deepseek-coder |
| **Moonshot** | `moonshot` | moonshot-v1-8k, moonshot-v1-32k, moonshot-v1-128k |
| **Custom** | `custom` | Use `[api]` section to configure custom URLs |

### 🎭 Tool Masking Options

```toml
[endpoint]
# Mask as officially supported tools
disguise_tool = "claudecode"  # Claude Code (recommended, compatible with Zhipu/Kimi)
# disguise_tool = "kimicode"    # Kimi Code API subscription auth format
# disguise_tool = "openclaw"    # OpenClaw
# disguise_tool = "custom"     # Use custom User-Agent
# custom_user_agent = "YourCustomTool/1.0"
```

| Tool | Identifier | User-Agent | Description |
|------|------------|------------|-------------|
| **Claude Code** | `claudecode` | `claude-code/2.1.63` | Anthropic official terminal coding assistant (recommended) |
| **Kimi Code** | `kimicode` | `claude-code/0.1.0` | Kimi Code API subscription auth format |
| **OpenClaw** | `openclaw` | `OpenClaw-Gateway/1.0` | Open-source AI coding tool |
| **Custom** | `custom` | (custom) | Use `custom_user_agent` config |

> **Note**: User-Agent values are sourced from official documentation and GitHub issues.

### 📡 API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Service information |
| `/v1/chat/completions` | POST | Chat completions (streaming supported) |
| `/v1/embeddings` | POST | Text embeddings |
| `/v1/models` | GET | List available models |
| `/health` | GET | Health check |
| `/ready` | GET | Readiness check |
| `/stats` | GET | Usage statistics (JSON) |

### 📊 Statistics & Management

```bash
# View connection info
mask-ctl info

# View token usage statistics
mask-ctl stats

# View help
mask-ctl help

# View usage statistics via API
curl http://127.0.0.1:8787/stats
```

### 🔧 Environment Variables

You can also configure via environment variables:

| Variable | Description |
|----------|-------------|
| `PROVIDER` | Provider identifier |
| `API_KEY` | Coding Plan API Key |
| `LOCAL_API_KEY` | Local API Key for authentication |
| `HOST` | Listen host |
| `PORT` | Listen port |
| `DEBUG` | Enable debug mode (true/false) |
| `API_BASE_URL` | Custom API base URL |
| `API_CODING_URL` | Custom coding endpoint URL |

### ⚠️ Risk Warning

> **IMPORTANT: Please read carefully before using this project**

This project is provided for **educational and research purposes only**.

| Risk | Description |
|------|-------------|
| 🔴 **Terms of Service** | May violate your provider's Terms of Service |
| 🔴 **Account Risk** | Improper use may result in API key revocation or account suspension |
| 🟡 **No Warranty** | Software provided "as-is" without any warranty |
| 🟡 **Security** | Exposing proxy to public networks may lead to unauthorized access |
| 🟢 **Self-Responsibility** | Users assume full responsibility for compliance |

**By using this software, you agree to:**
- Use it at your own risk
- Comply with all applicable laws and provider terms
- Accept full responsibility for any consequences

---

## 📖 中文文档

### 😤 问题背景：Coding Plan 的使用限制

各大 AI 服务商（智谱 GLM、阿里云百炼、MiniMax、DeepSeek、Moonshot 等）推出的 **Coding Plan（编码套餐）** 虽然价格诱人，但有**严格的使用限制**：

| 你以为买到的 | 实际上只能 |
|-------------|-----------|
| ✅ 固定月费，无限编码 | ❌ **只能在指定的 IDE 工具中使用** |
| ✅ 访问强大的模型 | ❌ **不能在你喜欢的工具里用** |
| ✅ 获得官方 API Key | ❌ **不能用于自动化/后端** |

### 💡 解决方案：Coding Plan Mask

**Coding Plan Mask** 作为你的 Coding Plan API 和任意 OpenAI 兼容工具之间的桥梁。它将你的请求**伪装**成来自官方支持的 IDE 工具。

### ✨ 核心功能

| 功能 | 说明 |
|------|------|
| 🎭 **工具伪装** | 伪装为 Claude Code、Kimi Code、OpenClaw 或自定义工具 |
| 🔀 **请求中转** | 将请求中转到 Coding Plan API 端点 |
| 🔌 **通用兼容** | 兼容任何支持 OpenAI API 的客户端 |
| 🌐 **多供应商** | 支持 6+ 主流大模型供应商 |
| 📊 **用量统计** | 实时追踪 Token 消耗，SQLite 持久化存储 |
| 🔒 **本地认证** | 用自定义密钥保护你的代理 |
| ⚡ **高性能** | Go 语言构建，极致效率 |
| 🔧 **灵活配置** | 支持 TOML 配置文件、环境变量和自定义 API URL |

### 🚀 快速开始

#### 1. 安装

**从 Release 下载（推荐）**

```bash
# Linux amd64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.6.0/mask-ctl-linux-amd64
chmod +x mask-ctl-linux-amd64
sudo mv mask-ctl-linux-amd64 /usr/local/bin/mask-ctl

# Linux arm64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.6.0/mask-ctl-linux-arm64
chmod +x mask-ctl-linux-arm64
sudo mv mask-ctl-linux-arm64 /usr/local/bin/mask-ctl

# macOS
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.6.0/mask-ctl-darwin-arm64
chmod +x mask-ctl-darwin-arm64
sudo mv mask-ctl-darwin-arm64 /usr/local/bin/mask-ctl
```

**从源码编译**

```bash
git clone https://github.com/systemime/coding-plan-mask.git
cd coding-plan-mask

# 编译当前平台
make build

# 交叉编译所有平台
make release
```

#### 2. 首次运行

```bash
mask-ctl
```

首次运行会自动创建配置文件 `/opt/project/coding-plan-mask/config/config.toml`，按提示编辑填写信息：

```bash
vim /opt/project/coding-plan-mask/config/config.toml
```

#### 3. 配置

```toml
[server]
listen_host = "127.0.0.1"
listen_port = 8787
timeout = 120                       # 请求超时(秒)
rate_limit_requests = 100           # 每5分钟请求限制

[auth]
provider = "zhipu"                  # 你的 Coding Plan 供应商
api_key = "your-coding-plan-api-key"  # 你的 Coding Plan API Key
local_api_key = "sk-local-secret"   # 你的工具使用的密钥

[endpoint]
use_coding_endpoint = true
disguise_tool = "claudecode"        # 伪装为 Claude Code (推荐)
```

#### 4. 启动

```bash
# 直接启动
mask-ctl

# 或使用 systemd (make install 后)
sudo systemctl start coding-plan-mask
```

#### 5. 配置你的 AI 工具

```json
{
    "base_url": "http://127.0.0.1:8787/v1",
    "api_key": "sk-local-secret",
    "model": "glm-4-flash"
}
```

### 🤖 支持的供应商

| 供应商 | 标识符 | 支持模型 |
|--------|--------|----------|
| **智谱 GLM** | `zhipu` | glm-4-flash, glm-4-plus, glm-4-air, glm-4-long |
| **智谱 GLM v2** | `zhipu_v2` | glm-4-flash, glm-4-plus, glm-4-air, glm-4-long, glm-4.7, glm-5 |
| **阿里云百炼** | `aliyun` | qwen-turbo, qwen-plus, qwen-max, qwen2.5-coder-32b-instruct |
| **MiniMax** | `minimax` | abab6.5s-chat, abab6.5g-chat, abab6.5-chat |
| **DeepSeek** | `deepseek` | deepseek-chat, deepseek-coder |
| **Moonshot (Kimi)** | `moonshot` | moonshot-v1-8k, moonshot-v1-32k, moonshot-v1-128k |
| **自定义** | `custom` | 使用 `[api]` 配置段自定义 URL |

### 🎭 工具伪装选项

| 工具 | 标识符 | User-Agent | 说明 |
|------|--------|------------|------|
| **Claude Code** | `claudecode` | `claude-code/2.1.63` | Anthropic 官方终端编程助手 (推荐) |
| **Kimi Code** | `kimicode` | `claude-code/0.1.0` | Kimi Code API 订阅认证格式 |
| **OpenClaw** | `openclaw` | `OpenClaw-Gateway/1.0` | 开源 AI 编程工具 |
| **自定义** | `custom` | (自定义) | 使用 `custom_user_agent` 配置 |

### 📡 API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/` | GET | 服务信息 |
| `/v1/chat/completions` | POST | 聊天补全（支持流式） |
| `/v1/embeddings` | POST | 文本向量嵌入 |
| `/v1/models` | GET | 可用模型列表 |
| `/health` | GET | 健康检查 |
| `/ready` | GET | 就绪检查 |
| `/stats` | GET | 使用统计（JSON） |

### 📊 统计与管理

```bash
# 查看连接信息
mask-ctl info

# 查看 Token 使用统计
mask-ctl stats

# 查看帮助
mask-ctl help

# 通过 API 查看使用统计
curl http://127.0.0.1:8787/stats
```

### 🔧 环境变量配置

| 变量 | 说明 |
|------|------|
| `PROVIDER` | 供应商标识符 |
| `API_KEY` | Coding Plan API Key |
| `LOCAL_API_KEY` | 本地认证 API Key |
| `HOST` | 监听地址 |
| `PORT` | 监听端口 |
| `DEBUG` | 启用调试模式 (true/false) |

### ⚠️ 风险预警

> **重要提示：使用前请仔细阅读**

本项目仅供**学习和研究目的**。

**使用本软件即表示您同意：**
- 自行承担使用风险
- 遵守所有适用法律和供应商条款
- 对任何后果承担全部责任

---

## 🛠️ Development

### Build Commands

```bash
# Build for current platform
make build

# Cross-compile for all platforms
make release

# Run tests
make test

# Run locally
make run
```

### Cross-Compilation Output

| Platform | Architecture | Output File |
|----------|-------------|-------------|
| Linux | amd64 | `mask-ctl-linux-amd64` |
| Linux | arm64 | `mask-ctl-linux-arm64` |
| macOS | amd64 | `mask-ctl-darwin-amd64` |
| macOS | arm64 | `mask-ctl-darwin-arm64` |
| Windows | amd64 | `mask-ctl-windows-amd64.exe` |
| Windows | arm64 | `mask-ctl-windows-arm64.exe` |

### Tech Stack

- **Language**: Go 1.21+
- **HTTP Server**: net/http
- **Configuration**: TOML (github.com/BurntSushi/toml)
- **Logging**: Zap (go.uber.org/zap)
- **Storage**: SQLite3 (github.com/mattn/go-sqlite3)
- **Rate Limiting**: golang.org/x/time/rate

---

<div align="center">

**⭐ If this project helps you, please give it a star! ⭐**

Made with ❤️ by the community

</div>
