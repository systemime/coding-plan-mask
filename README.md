<div align="center">

# 🎭 Coding Plan Mask

**Local proxy for coding-client disguise and optional privacy filtering**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-0.8.6-green.svg)](https://github.com/systemime/coding-plan-mask)

*Mask request origin locally, then optionally redact or block sensitive content before it leaves your machine*

[English](#-english-documentation) | [中文文档](#-中文文档)

</div>

> 📚 **使用教程**: [Coding‐plan‐mask使用案例](https://github.com/systemime/coding-plan-mask/wiki/Coding%E2%80%90plan%E2%80%90mask%E4%BD%BF%E7%94%A8%E6%A1%88%E4%BE%8B) - 如何让 Trae 等 IDE 使用本地/其他大模型

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

Scope is intentionally small: local proxy, request-origin disguise, OpenAI/Anthropic compatibility, and optional local privacy protection. It is not a GUI app, cloud sync service, MCP/Skills panel, or multi-tenant billing platform.

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
| 🛡️ **Local Privacy Filter** | Off by default; when enabled, applies local S1/S2/S3 policy, redaction, block, full/clean audit, and context selection |
| ⚡ **High Performance** | Built in Go for maximum efficiency |
| 🔧 **Flexible Configuration** | Support TOML config file, environment variables, and custom API URLs |
| 📈 **Rate Limiting** | Built-in rate limiting to prevent abuse |
| 🌊 **Streaming Support** | Real-time streaming response forwarding with intelligent detection |
| 💾 **Two-Phase Storage** | Request saved immediately on arrival, updated on response completion |

### 🚀 Quick Start

#### 1. Install

**Download from Releases (Recommended)**

Download the binary for your platform from [GitHub Releases](https://github.com/systemime/coding-plan-mask/releases):

```bash
# Linux amd64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.6/mask-ctl-linux-amd64
chmod +x mask-ctl-linux-amd64
sudo mv mask-ctl-linux-amd64 /usr/local/bin/mask-ctl

# Linux arm64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.6/mask-ctl-linux-arm64
chmod +x mask-ctl-linux-arm64
sudo mv mask-ctl-linux-arm64 /usr/local/bin/mask-ctl

# macOS (Darwin amd64)
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.6/mask-ctl-darwin-amd64
chmod +x mask-ctl-darwin-amd64
sudo mv mask-ctl-darwin-amd64 /usr/local/bin/mask-ctl

# macOS (Darwin arm64)
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.6/mask-ctl-darwin-arm64
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
claude_code_user_agent = "claude-cli/2.1.88 (external, cli)"

[api]
# Optional: Remove version prefix (e.g., /v1) from request path when forwarding
# Example: Request to /v1/models will be forwarded as /models only
remove_version_path = false
# Mock /models endpoint response (default: false)
# When enabled, returns mock data instead of forwarding to upstream
# Matches: /models, /v1/models, /v2/models, /v3/models
mock_models = false
# Mock /models response content (JSON string)
mock_models_resp = '{"object":"list","data":[{"id":"gpt-4","object":"model","owned_by":"organization"}]}'
# Anthropic/Claude client compatibility mode (default: false)
# Local Anthropic /v1/messages -> upstream OpenAI-compatible /chat/completions
# Also converts upstream OpenAI JSON/SSE responses back to Anthropic format
# Built-in providers map claude-* model names to their preferred coding model
use_anthropic = false

[security]
# Disabled by default. Enable to redact/block before forwarding upstream.
enabled = false  # When true, set [auth].local_api_key for proxy/security APIs
handling_s2 = "redact"
handling_s3 = "block"
default_track = "clean"
max_audit_items = 2000

[security.redaction]
email = true
chinese_phone = true
chinese_id = true
```

Privacy switch:

- `enabled = false` (default): proxy only; request bodies are not changed by the privacy filter.
- `enabled = true`: redact or block locally before forwarding upstream. Configure `[auth].local_api_key` when this is enabled.

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

Privacy filtering is a low-CPU local rules baseline, not a full DLP or ML PII detector. Extend `[security.rules]` for project-specific secrets.

### 🔁 Protocol Compatibility

`disguise_tool` and `use_anthropic` do different things:

- `disguise_tool` changes outbound headers/User-Agent so the upstream sees a supported coding client.
- `use_anthropic` changes the local API protocol accepted by this proxy.

| Local client sends | Local URL/path | Upstream request | Status |
|--------------------|----------------|------------------|--------|
| OpenAI Chat Completions | `http://127.0.0.1:8787/v1/chat/completions` | OpenAI-compatible path/body | Default |
| Anthropic Messages / Claude-style | `http://127.0.0.1:8787/v1/messages` with `use_anthropic=true` | OpenAI-compatible `/chat/completions` | Supported |
| OpenAI client → native Anthropic/Claude upstream | OpenAI `/v1/chat/completions` | Anthropic `/v1/messages` | Not implemented |

When `use_anthropic=true`, Claude-style clients can call the local proxy with Anthropic Messages format while the upstream remains OpenAI-compatible. Requests, tools, tool results, normal responses, and SSE streams are translated both ways for that path.

Minimal client examples:

```json
// OpenAI-compatible client
{
  "base_url": "http://127.0.0.1:8787/v1",
  "api_key": "sk-local-secret",
  "model": "glm-4-flash"
}
```

```bash
# Claude/Anthropic-style client
# config.toml: [api] use_anthropic = true
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ANTHROPIC_API_KEY=sk-local-secret
```

### 🎭 Tool Masking Options

```toml
[endpoint]
# Mask as officially supported tools
disguise_tool = "claudecode"  # Claude Code-style CLI traffic
# claude_code_user_agent = "claude-cli/2.1.88 (external, cli)"
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
| **Claude Code** | `claudecode` | `claude-cli/2.1.88 (external, cli)` | Current default Claude CLI-style UA, configurable via `claude_code_user_agent` |
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
| `/redact` | POST | Local text redaction |
| `/privacy/detect` | POST | Local sensitivity detection |
| `/privacy/policy` | POST | Local allow/redact/review/block decision |
| `/context/redact` / `/context/restore` | POST | Redact or restore text/messages context |
| `/sessions/{id}` / `/sessions/{id}/messages` / `/sessions/{id}/context/select` | GET/POST | full/clean audit tracks and local context selection |
| `/*` | Any | Forward any other path to the upstream API with disguised headers |

### 📊 Statistics & Management

```bash
# View connection info
mask-ctl info

# View token usage statistics
mask-ctl stats

# Check local configuration
mask-ctl doctor

# View forwarding history
mask-ctl history

# View one request detail
mask-ctl history -id 123

# View help
mask-ctl help

# View usage statistics via API
curl http://127.0.0.1:8787/stats
```

#### Connection & Check Commands

- `mask-ctl show` prints OpenAI-compatible and Anthropic-compatible local URLs.
- `mask-ctl show --json` keeps machine-readable connection output for scripts.
- `mask-ctl doctor` checks required keys, provider routing, local auth, Anthropic bridge, and privacy mode.
- `mask-ctl history -id <ID>` prints one full request/response record.

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
| `REMOVE_VERSION_PATH` | Remove version prefix (e.g., `/v1`) from request path when forwarding (true/false) |
| `MOCK_MODELS` | Enable mock /models endpoint response (true/false) |
| `MOCK_MODELS_RESP` | Mock /models response content (JSON string) |
| `USE_ANTHROPIC` | Enable Anthropic format compatibility mode (true/false) |
| `SECURITY_ENABLED` | Enable local privacy filter (true/false) |
| `SECURITY_AUDIT_DIR` | Override full/clean audit directory |
| `SECURITY_HANDLING_S2` / `SECURITY_HANDLING_S3` | Override S2/S3 handling action |
| `SECURITY_REDACT_EMAIL` / `SECURITY_REDACT_CHINESE_PHONE` / `SECURITY_REDACT_CHINESE_ID` | Override default PII redaction toggles |

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

项目边界很明确：本地代理、请求来源伪装、OpenAI/Anthropic 协议兼容，以及可选的本地隐私保护。它不是 GUI、云同步、MCP/Skills 面板，也不是多租户计费平台。

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
| 🛡️ **本地隐私过滤** | 默认关闭；开启后在本地执行 S1/S2/S3 策略、脱敏、阻断、full/clean 审计和上下文筛选 |
| ⚡ **高性能** | Go 语言构建，极致效率 |
| 🔧 **灵活配置** | 支持 TOML 配置文件、环境变量和自定义 API URL |
| 🌊 **流式响应** | 实时流式转发响应，智能检测流式请求 |
| 💾 **两阶段存储** | 请求到达时立即保存，响应完成时更新记录 |

### 🚀 快速开始

#### 1. 安装

**从 Release 下载（推荐）**

```bash
# Linux amd64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.6/mask-ctl-linux-amd64
chmod +x mask-ctl-linux-amd64
sudo mv mask-ctl-linux-amd64 /usr/local/bin/mask-ctl

# Linux arm64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.6/mask-ctl-linux-arm64
chmod +x mask-ctl-linux-arm64
sudo mv mask-ctl-linux-arm64 /usr/local/bin/mask-ctl

# macOS amd64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.6/mask-ctl-darwin-amd64
chmod +x mask-ctl-darwin-amd64
sudo mv mask-ctl-darwin-amd64 /usr/local/bin/mask-ctl

# macOS arm64
wget https://github.com/systemime/coding-plan-mask/releases/download/v0.8.6/mask-ctl-darwin-arm64
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
claude_code_user_agent = "claude-cli/2.1.88 (external, cli)"
openclaw_user_agent = "OpenClaw-Gateway/1.0"

[api]
# 可选：转发时移除请求路径中的版本前缀（如 /v1）
# 例如：请求 /v1/models 时，转发时只拼接 /models 部分
remove_version_path = false
# 模拟 /models 端点响应 (默认: false)
# 启用后返回模拟数据，不转发到上游
# 匹配路径: /models, /v1/models, /v2/models, /v3/models
mock_models = false
# 模拟 /models 响应内容 (JSON 字符串)
mock_models_resp = '{"object":"list","data":[{"id":"gpt-4","object":"model","owned_by":"organization"}]}'
# Anthropic/Claude 客户端兼容模式 (默认: false)
# 本地 Anthropic /v1/messages -> 上游 OpenAI 兼容 /chat/completions
# 同时把上游 OpenAI JSON/SSE 响应转回 Anthropic 格式
# 内置服务商会将 claude-* 模型名映射到其推荐编码模型
use_anthropic = false

[security]
# 默认关闭。启用后会在转发上游前本地脱敏/阻断；代理和本地安全接口都需配置 [auth].local_api_key。
enabled = false
handling_s2 = "redact"
handling_s3 = "block"
default_track = "clean"
max_audit_items = 2000

[security.redaction]
email = true
chinese_phone = true
chinese_id = true
```

隐私保护开关：

- `enabled = false`（默认）：只做代理和伪装，隐私过滤不会改写请求体。
- `enabled = true`：转发上游前先在本地脱敏或阻断。开启后请配置 `[auth].local_api_key`。

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

隐私过滤是低 CPU 的本地规则基线，不是完整 DLP 或机器学习 PII 检测器。项目专属敏感规则请通过 `[security.rules]` 扩展。

### 🔁 协议兼容

`disguise_tool` 和 `use_anthropic` 是两件事：

- `disguise_tool` 只改转发到上游时的请求头/User-Agent，让上游看到受支持的编码客户端。
- `use_anthropic` 才是本地协议转换开关。

| 本地客户端请求 | 本地地址/路径 | 转发到上游 | 状态 |
|----------------|---------------|------------|------|
| OpenAI Chat Completions | `http://127.0.0.1:8787/v1/chat/completions` | OpenAI 兼容路径/请求体 | 默认支持 |
| Anthropic Messages / Claude 风格 | `http://127.0.0.1:8787/v1/messages`，并设置 `use_anthropic=true` | OpenAI 兼容 `/chat/completions` | 已支持 |
| OpenAI 客户端 → 原生 Anthropic/Claude 上游 | OpenAI `/v1/chat/completions` | Anthropic `/v1/messages` | 暂未实现 |

也就是说：Claude/Anthropic 协议的本地客户端可以接入 OpenAI 兼容上游；请求、工具调用、工具结果、普通响应和 SSE 流都会在 `/v1/messages` 这条路径上转换。反向的“OpenAI 本地协议转原生 Claude/Anthropic 上游”目前没有做；如果上游本身提供 OpenAI 兼容接口，直接走默认 OpenAI 路径即可。

最小客户端配置示例：

```json
// OpenAI 兼容客户端
{
  "base_url": "http://127.0.0.1:8787/v1",
  "api_key": "sk-local-secret",
  "model": "glm-4-flash"
}
```

```bash
# Claude/Anthropic 风格客户端
# config.toml: [api] use_anthropic = true
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ANTHROPIC_API_KEY=sk-local-secret
```

### 🎭 工具伪装选项

| 工具 | 标识符 | User-Agent | 说明 |
|------|--------|------------|------|
| **Claude Code** | `claudecode` | `claude-cli/2.1.88 (external, cli)` | 当前默认 Claude CLI 风格 UA，可通过 `claude_code_user_agent` 覆盖 |
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
| `/redact` | POST | 本地文本脱敏 |
| `/privacy/detect` | POST | 本地敏感级别检测 |
| `/privacy/policy` | POST | 本地 allow/redact/review/block 策略判断 |
| `/context/redact` / `/context/restore` | POST | 文本/消息上下文脱敏与还原 |
| `/sessions/{id}` / `/sessions/{id}/messages` / `/sessions/{id}/context/select` | GET/POST | full/clean 审计轨和本地上下文筛选 |
| `/*` | 任意 | 其余任意路径原样透传到上游 API，并附加伪装请求头 |

### 📊 统计与管理

```bash
# 查看连接信息
mask-ctl info

# 查看 Token 使用统计
mask-ctl stats

# 检查本地配置
mask-ctl doctor

# 查看转发历史
mask-ctl history

# 查看单条请求详情
mask-ctl history -id 123

# 查看帮助
mask-ctl help

# 通过 API 查看使用统计
curl http://127.0.0.1:8787/stats
```

#### 连接与检查命令

- `mask-ctl show` 输出 OpenAI 兼容和 Anthropic 兼容本地地址。
- `mask-ctl show --json` 保持脚本可读的连接信息。
- `mask-ctl doctor` 检查必要 Key、服务商路由、本地认证、Anthropic 转换和隐私模式。
- `mask-ctl history -id <ID>` 输出单条完整请求/响应记录。

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
| `REMOVE_VERSION_PATH` | 转发时移除请求路径中的版本前缀（如 `/v1`）(true/false) |
| `MOCK_MODELS` | 启用模拟 /models 端点响应 (true/false) |
| `MOCK_MODELS_RESP` | 模拟 /models 响应内容 (JSON 字符串) |
| `USE_ANTHROPIC` | 启用 Anthropic 格式兼容模式 (true/false) |
| `SECURITY_ENABLED` | 启用本地隐私过滤 (true/false) |
| `SECURITY_AUDIT_DIR` | 覆盖 full/clean 审计目录 |
| `SECURITY_HANDLING_S2` / `SECURITY_HANDLING_S3` | 覆盖 S2/S3 处置策略 |
| `SECURITY_REDACT_EMAIL` / `SECURITY_REDACT_CHINESE_PHONE` / `SECURITY_REDACT_CHINESE_ID` | 覆盖默认 PII 脱敏开关 |

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
