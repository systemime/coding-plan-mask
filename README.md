<div align="center">

# 🎭 Coding Plan Mask

**Unlock Your Coding Plan API Key for Any AI Coding Tool**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-0.8.0-green.svg)](https://github.com/systemime/coding-plan-mask)

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

Provider rules, available models, and subscription details can change over time. Treat current provider policy as an external dependency and verify it yourself before use.

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
| 🔀 **Request Relay** | Transparently forward arbitrary upstream API paths |
| 🧩 **Claude CLI Disguise** | `claudecode` mode uses a Claude CLI-style `User-Agent` and injects `x-app: cli` when missing |
| 🔌 **Universal Compatibility** | Works with ANY OpenAI-compatible client |
| 🌐 **Multi-Provider** | Support for 6+ major LLM providers |
| 📊 **Usage Analytics** | Track token consumption in real-time with SQLite storage |
| 📝 **Readable Logs** | Human-friendly token logs in non-debug mode |
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
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.0/mask-ctl-linux-amd64
chmod +x mask-ctl-linux-amd64
sudo mv mask-ctl-linux-amd64 /usr/local/bin/mask-ctl

# Linux arm64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.0/mask-ctl-linux-arm64
chmod +x mask-ctl-linux-arm64
sudo mv mask-ctl-linux-arm64 /usr/local/bin/mask-ctl

# macOS (Darwin amd64)
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.0/mask-ctl-darwin-amd64
chmod +x mask-ctl-darwin-amd64
sudo mv mask-ctl-darwin-amd64 /usr/local/bin/mask-ctl

# macOS (Darwin arm64)
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.0/mask-ctl-darwin-arm64
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

If you run the binary directly from a folder, the default configuration file will be created beside the executable as `config.toml`. The loader also recognizes `config.eg` and `config.example.toml` in the same directory.

If you install into a protected system path such as `/usr/local/bin`, prefer either:

- `make install` so the service uses `/opt/project/coding-plan-mask/config/config.toml`
- Or start with an explicit `-config /path/to/config.toml`

Example when running from the extracted folder:

```bash
vim ./config.toml
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
disguise_tool = "claudecode"        # Mask as Claude Code-style CLI traffic
claude_code_user_agent = "claude-cli/2.1.76 (external, cli)"
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
    "base_url": "http://127.0.0.1:8787",
    "api_key": "sk-local-secret",
    "model": "glm-4-flash"
}
```

If your client hardcodes `/v1`, that still works. The proxy keeps local management endpoints and transparently forwards any other request path upstream.

In non-debug mode, startup keeps the banner output and proxy activity is shown in a human-friendly text format instead of structured JSON logs.

### 🎭 Tool Masking Options

```toml
[endpoint]
# Mask as officially supported tools
disguise_tool = "claudecode"  # Claude Code-style CLI traffic
# claude_code_user_agent = "claude-cli/2.1.76 (external, cli)"
# disguise_tool = "kimicode"    # Kimi Code API subscription auth format
# disguise_tool = "opencode"    # Legacy OpenCode disguise id
# opencode_user_agent = "opencode/1.2.27 ai-sdk/provider-utils/3.0.20 runtime/bun/1.3.10"
# disguise_tool = "openclaw"    # OpenClaw
# openclaw_user_agent = "OpenClaw-Gateway/1.0"
# disguise_tool = "custom"     # Use custom User-Agent
# custom_user_agent = "YourCustomTool/1.0"
```

| Tool | Identifier | User-Agent | Description |
|------|------------|------------|-------------|
| **Claude Code** | `claudecode` | `claude-cli/2.1.76 (external, cli)` | Current default Claude CLI-style UA, configurable via `claude_code_user_agent` |
| **Kimi Code** | `kimicode` | `claude-code/0.1.0` | Kimi Code API subscription auth format |
| **OpenCode** | `opencode` | `opencode/1.2.27 ai-sdk/provider-utils/3.0.20 runtime/bun/1.3.10` | Legacy disguise id with default UA updated from local capture report |
| **OpenClaw** | `openclaw` | `OpenClaw-Gateway/1.0` | Compatibility default, configurable via `openclaw_user_agent` |
| **Custom** | `custom` | (custom) | Use `custom_user_agent` config |

> **Note**: `claudecode` mode also injects `x-app: cli` if the incoming request does not already provide it.
> **Note**: `opencode` mode keeps the legacy disguise id but now defaults to the locally captured OpenCode 1.2.27 UA. Override it with `opencode_user_agent` if needed.
> **Note**: `openclaw` mode keeps `OpenClaw-Gateway/1.0` as a compatibility default, but this does not imply every current OpenClaw request path uses the same UA.

### 📡 API Endpoints

The proxy reserves a small set of local management endpoints and transparently forwards all other request paths to the upstream provider.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Service information |
| `/health` | GET | Health check |
| `/ready` | GET | Readiness check |
| `/stats` | GET | Usage statistics (JSON) |
| `/*` | Any | Forward any other path to the upstream API with disguised headers |

### 📊 Statistics & Management

```bash
# View connection info
mask-ctl info

# View token usage statistics
mask-ctl stats

# View forwarding history (interactive TUI)
mask-ctl history

# View help
mask-ctl help

# View usage statistics via API
curl http://127.0.0.1:8787/stats
```

#### History Command

The `history` command provides an interactive terminal UI for browsing forwarding records:

- **Navigation**: Use `↑/↓` or `j/k` to move through records
- **View Details**: Press `Enter` to expand full request/response details including body
- **Exit**: Press `q` to quit, `Esc` to return from detail view

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
| `DISGUISE_TOOL` | Override disguise tool |
| `CLAUDE_CODE_USER_AGENT` | Override the default UA used by `claudecode` mode |
| `OPENCODE_USER_AGENT` | Override the default UA used by `opencode` mode |
| `OPENCLAW_USER_AGENT` | Override the compatibility UA used by `openclaw` mode |
| `CUSTOM_USER_AGENT` | Override User-Agent directly |

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
| 🔀 **请求中转** | 透传任意上游 API 路径，并附加伪装请求头 |
| 🧩 **Claude CLI 伪装** | `claudecode` 模式默认使用 Claude CLI 风格 `User-Agent`，并在缺失时补 `x-app: cli` |
| 🔌 **通用兼容** | 兼容任何支持 OpenAI API 的客户端 |
| 🌐 **多供应商** | 支持 6+ 主流大模型供应商 |
| 📊 **用量统计** | 实时追踪 Token 消耗，SQLite 持久化存储 |
| 📝 **可读日志** | 非 debug 模式下输出人类友好的 token 日志 |
| 🔒 **本地认证** | 用自定义密钥保护你的代理 |
| ⚡ **高性能** | Go 语言构建，极致效率 |
| 🔧 **灵活配置** | 支持 TOML 配置文件、环境变量和自定义 API URL |

### 🚀 快速开始

#### 1. 安装

**从 Release 下载（推荐）**

```bash
# Linux amd64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.0/mask-ctl-linux-amd64
chmod +x mask-ctl-linux-amd64
sudo mv mask-ctl-linux-amd64 /usr/local/bin/mask-ctl

# Linux arm64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.0/mask-ctl-linux-arm64
chmod +x mask-ctl-linux-arm64
sudo mv mask-ctl-linux-arm64 /usr/local/bin/mask-ctl

# macOS amd64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.0/mask-ctl-darwin-amd64
chmod +x mask-ctl-darwin-amd64
sudo mv mask-ctl-darwin-amd64 /usr/local/bin/mask-ctl

# macOS arm64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.0/mask-ctl-darwin-arm64
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

如果你直接运行下载后的二进制，默认配置会创建在可执行文件同目录下的 `config.toml`，同时也会识别同目录中的 `config.eg` 和 `config.example.toml`。

如果你安装到了 `/usr/local/bin` 这类系统目录，推荐两种方式：

- 使用 `make install`，systemd 配置会使用 `/opt/project/coding-plan-mask/config/config.toml`
- 或显式传入 `-config /path/to/config.toml`

例如在解压目录中直接运行时：

```bash
vim ./config.toml
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
disguise_tool = "claudecode"        # 伪装为 Claude Code 风格 CLI 请求
claude_code_user_agent = "claude-cli/2.1.76 (external, cli)"
openclaw_user_agent = "OpenClaw-Gateway/1.0"
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
    "base_url": "http://127.0.0.1:8787",
    "api_key": "sk-local-secret",
    "model": "glm-4-flash"
}
```

如果你的客户端固定使用 `/v1` 前缀，也可以继续工作。代理会保留本地管理端点，并将其他路径透明转发到上游。

在非 `debug` 模式下，程序会保留启动横幅，并以人类可读的文本格式输出代理 token 日志，而不是结构化 JSON 日志。

### 🎭 工具伪装选项

| 工具 | 标识符 | User-Agent | 说明 |
|------|--------|------------|------|
| **Claude Code** | `claudecode` | `claude-cli/2.1.76 (external, cli)` | 当前默认 Claude CLI 风格 UA，可通过 `claude_code_user_agent` 覆盖 |
| **Kimi Code** | `kimicode` | `claude-code/0.1.0` | Kimi Code API 订阅认证格式 |
| **OpenCode** | `opencode` | `opencode/1.2.27 ai-sdk/provider-utils/3.0.20 runtime/bun/1.3.10` | 保留旧 disguise id，默认 UA 已按本地抓包报告更新 |
| **OpenClaw** | `openclaw` | `OpenClaw-Gateway/1.0` | 兼容默认值，可通过 `openclaw_user_agent` 覆盖 |
| **自定义** | `custom` | (自定义) | 使用 `custom_user_agent` 配置 |

> **说明**：`claudecode` 模式在传入请求未提供时还会补充 `x-app: cli`。
> **说明**：`opencode` 模式保留旧标识，但默认 UA 已更新为本地抓包得到的 OpenCode 1.2.27 请求格式，可通过 `opencode_user_agent` 覆盖。
> **说明**：`openclaw` 模式保留 `OpenClaw-Gateway/1.0` 作为兼容默认值，但这不代表当前 OpenClaw 所有请求路径都统一使用该 UA。

### 📡 API 端点

代理会保留少量本地管理端点，其余任意请求路径都会透明转发到上游服务商。

| 端点 | 方法 | 说明 |
|------|------|------|
| `/` | GET | 服务信息 |
| `/health` | GET | 健康检查 |
| `/ready` | GET | 就绪检查 |
| `/stats` | GET | 使用统计（JSON） |
| `/*` | 任意 | 其余任意路径原样透传到上游 API，并附加伪装请求头 |

### 📊 统计与管理

```bash
# 查看连接信息
mask-ctl info

# 查看 Token 使用统计
mask-ctl stats

# 查看转发历史（交互式 TUI）
mask-ctl history

# 查看帮助
mask-ctl help

# 通过 API 查看使用统计
curl http://127.0.0.1:8787/stats
```

#### History 命令

`history` 命令提供交互式终端界面浏览转发记录：

- **导航**：使用 `↑/↓` 或 `j/k` 在记录间移动
- **查看详情**：按 `Enter` 展开完整的请求/响应详情，包括 body
- **退出**：按 `q` 退出，`Esc` 从详情视图返回

### 🔧 环境变量配置

| 变量 | 说明 |
|------|------|
| `PROVIDER` | 供应商标识符 |
| `API_KEY` | Coding Plan API Key |
| `LOCAL_API_KEY` | 本地认证 API Key |
| `HOST` | 监听地址 |
| `PORT` | 监听端口 |
| `DEBUG` | 启用调试模式 (true/false) |
| `API_BASE_URL` | 自定义通用 API 基础 URL |
| `API_CODING_URL` | 自定义 Coding API URL |
| `DISGUISE_TOOL` | 覆盖伪装工具 |
| `CLAUDE_CODE_USER_AGENT` | 覆盖 `claudecode` 模式默认 User-Agent |
| `OPENCODE_USER_AGENT` | 覆盖 `opencode` 模式默认 User-Agent |
| `OPENCLAW_USER_AGENT` | 覆盖 `openclaw` 模式兼容默认 User-Agent |
| `CUSTOM_USER_AGENT` | 直接覆盖 User-Agent |

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
- **Storage**: SQLite (modernc.org/sqlite)
- **Rate Limiting**: golang.org/x/time/rate

---

<div align="center">

**⭐ If this project helps you, please give it a star! ⭐**

Made with ❤️ by the community

</div>
