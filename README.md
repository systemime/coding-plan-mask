<div align="center">

# 🚀 Coding Plan Proxy

**Lightning-Fast AI Gateway for Coding Assistants**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

*Transform any OpenAI-compatible client into a powerful coding assistant with enterprise-grade features*

[English](#-english-documentation) | [中文文档](#-中文文档)

</div>

---

## 📖 English Documentation

### What is Coding Plan Proxy?

**Coding Plan Proxy** is a high-performance, production-ready proxy server that bridges your favorite AI coding tools with multiple LLM providers through the **Coding Plan API**. Built in Go for maximum efficiency, it offers enterprise features like rate limiting, usage analytics, and seamless OpenAI API compatibility.

### ✨ Why Choose Coding Plan Proxy?

| Feature | Benefit |
|---------|---------|
| ⚡ **Blazing Fast** | Sub-millisecond latency with Go's native concurrency |
| 🔌 **Drop-in Compatible** | Works with any OpenAI-compatible client instantly |
| 🎭 **Smart Disguise** | Appear as popular coding tools (OpenCode, OpenClaw) |
| 📊 **Rich Analytics** | Real-time token usage tracking and visualization |
| 🔒 **Enterprise Security** | Rate limiting, request validation, and audit logging |
| 🌐 **Multi-Provider** | Support for 6+ major LLM providers out of the box |

### 🏗️ Architecture

```
┌─────────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│  Your AI Client │────▶│  Coding Plan Proxy   │────▶│   LLM Provider  │
│  (OpenAI SDK)   │◀────│  (OpenAI Compatible) │◀────│   (Zhipu, etc)  │
└─────────────────┘     └──────────────────────┘     └─────────────────┘
                               │
                               ▼
                        ┌──────────────┐
                        │   SQLite DB  │
                        │  (Analytics) │
                        └──────────────┘
```

### 🚀 Quick Start

#### Installation

```bash
# Clone the repository
git clone https://github.com/systemime/coding-plan-proxy-go.git
cd coding-plan-proxy-go

# Build
make build

# Install to system (requires root)
sudo make install
```

#### Configuration

Edit `/opt/project/coding-plan-proxy/config/config.toml`:

```toml
[server]
listen_host = "127.0.0.1"
listen_port = 8787

[auth]
provider = "zhipu"                    # zhipu, aliyun, minimax, deepseek, moonshot
api_key = "your-coding-plan-api-key"  # Your Coding Plan API Key
local_api_key = "sk-local-secret"     # Client authentication key

[endpoint]
use_coding_endpoint = true
disguise_tool = "opencode"            # opencode, openclaw, or custom
```

#### Start the Service

```bash
# Using the control script
proxy-ctl start

# Or using systemctl
sudo systemctl start coding-plan-proxy
sudo systemctl enable coding-plan-proxy  # Auto-start on boot
```

### 📡 API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Service information & stats |
| `/v1/models` | GET | List available models |
| `/v1/chat/completions` | POST | Chat completions (streaming supported) |
| `/v1/embeddings` | POST | Text embeddings |
| `/health` | GET | Health check |
| `/ready` | GET | Readiness check |
| `/stats` | GET | Usage statistics |

### 🤖 Supported Providers

| Provider | Identifier | Models |
|----------|------------|--------|
| **Zhipu GLM** | `zhipu` | glm-4-flash, glm-4-plus, glm-4-air, glm-4-long |
| **Zhipu GLM v2** | `zhipu_v2` | glm-4-flash, glm-4-plus, glm-4.7, glm-5 |
| **Alibaba Cloud** | `aliyun` | qwen-turbo, qwen-plus, qwen-max, qwen2.5-coder |
| **MiniMax** | `minimax` | abab6.5s-chat, abab6.5g-chat |
| **DeepSeek** | `deepseek` | deepseek-chat, deepseek-coder |
| **Moonshot** | `moonshot` | moonshot-v1-8k, moonshot-v1-32k, moonshot-v1-128k |

### 💻 Client Configuration

**For OpenAI SDK:**

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8787/v1",
    api_key="sk-local-secret"
)

response = client.chat.completions.create(
    model="glm-4-flash",
    messages=[{"role": "user", "content": "Hello!"}],
    stream=True
)
```

**For cURL:**

```bash
curl http://127.0.0.1:8787/v1/chat/completions \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer sk-local-secret" \
    -d '{
        "model": "glm-4-flash",
        "messages": [{"role": "user", "content": "Hello!"}],
        "stream": true
    }'
```

### 📊 Statistics & Connection Info

```bash
# View statistics
coding-plan-proxy stats

# View connection info
coding-plan-proxy show

# JSON format output (for scripting)
coding-plan-proxy show --json
```

Output example:
```
╔════════════════════════════════════════════════════════════╗
║                    Token Usage Statistics                   ║
╠════════════════════════════════════════════════════════════╣
║  Total Requests:    1,234                                  ║
║  Total Input:       156,789 tokens                         ║
║  Total Output:      89,012 tokens                          ║
║  Today Requests:    56                                     ║
╚════════════════════════════════════════════════════════════╝
```

### 🛠️ Control Script Commands

```bash
proxy-ctl start        # Start service
proxy-ctl stop         # Stop service
proxy-ctl restart      # Restart service
proxy-ctl status       # View status
proxy-ctl logs         # View logs (real-time)
proxy-ctl info         # Show connection info
proxy-ctl test         # Test API connection
proxy-ctl config       # View/modify configuration
proxy-ctl edit         # Edit config file
```

### 🎭 Tool Disguise Feature

The proxy can disguise itself as popular coding tools:

```toml
[endpoint]
disguise_tool = "opencode"  # Options: opencode, openclaw, custom
custom_user_agent = ""      # Used when disguise_tool = "custom"
```

### 🔐 Security Best Practices

1. **Always set `local_api_key`** to prevent unauthorized access
2. **Bind to `127.0.0.1`** unless external access is required
3. **Configure rate limiting** to prevent abuse
4. **Review logs regularly** via `journalctl -u coding-plan-proxy`
5. **Keep updated** with the latest release

### 📁 Project Structure

```
coding-plan-proxy-go/
├── cmd/
│   └── coding-plan-proxy/
│       └── main.go           # Entry point
├── internal/
│   ├── config/
│   │   └── config.go         # Configuration management
│   ├── proxy/
│   │   └── proxy.go          # Proxy logic
│   ├── ratelimit/
│   │   └── ratelimit.go      # Rate limiting
│   ├── server/
│   │   └── server.go         # HTTP server
│   └── storage/
│       └── storage.go        # SQLite storage
├── deploy/
│   ├── coding-plan-proxy.service  # systemd unit
│   ├── config.example.toml        # Example config
│   └── proxy-ctl.sh               # Control script
├── go.mod
├── Makefile
└── README.md
```

### ⚠️ Risk Warning

> **IMPORTANT: Please read carefully before using this project**

This project is provided for **educational and research purposes only**. Users should be aware of the following risks:

| Risk Category | Description |
|---------------|-------------|
| 🔴 **Terms of Service** | Using this proxy may violate the Terms of Service of certain LLM providers. Users are responsible for understanding and complying with provider policies. |
| 🔴 **Account Suspension** | Improper use may result in API key revocation or account suspension by providers. |
| 🟡 **No Warranty** | This software is provided "as-is" without any warranty. The authors are not liable for any damages. |
| 🟡 **Security Risks** | Exposing the proxy to public networks without proper authentication may lead to unauthorized access. |
| 🟢 **Self-Responsibility** | Users assume full responsibility for compliance with applicable laws and regulations. |

**By using this software, you agree to:**
- Use it at your own risk
- Comply with all applicable laws and provider terms
- Not use it for any illegal or unauthorized purposes
- Accept full responsibility for any consequences

**Recommendations:**
1. Only use with APIs you have legitimate access to
2. Always set `local_api_key` to prevent unauthorized access
3. Bind to `127.0.0.1` unless you fully understand the security implications
4. Review and comply with your provider's Terms of Service

---

## 📖 中文文档

### 什么是 Coding Plan Proxy？

**Coding Plan Proxy** 是一个高性能、生产就绪的代理服务器，通过 **Coding Plan API** 将您喜爱的 AI 编程工具与多个大语言模型提供商连接起来。采用 Go 语言构建以实现最高效率，提供速率限制、使用分析、OpenAI API 无缝兼容等企业级功能。

### ✨ 核心优势

| 特性 | 优势 |
|------|------|
| ⚡ **极致性能** | Go 原生并发，亚毫秒级延迟 |
| 🔌 **即插即用** | 与任何 OpenAI 兼容客户端无缝对接 |
| 🎭 **智能伪装** | 伪装为热门编程工具 (OpenCode, OpenClaw) |
| 📊 **丰富统计** | 实时 Token 使用追踪和可视化 |
| 🔒 **企业安全** | 速率限制、请求验证、审计日志 |
| 🌐 **多供应商** | 开箱即用支持 6+ 主流大模型供应商 |

### 🏗️ 工作原理

```
┌─────────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│   您的 AI 客户端 │────▶│   Coding Plan Proxy  │────▶│    大模型供应商  │
│  (OpenAI SDK)   │◀────│   (OpenAI 兼容接口)   │◀────│  (智谱, 阿里等)  │
└─────────────────┘     └──────────────────────┘     └─────────────────┘
                               │
                               ▼
                        ┌──────────────┐
                        │   SQLite 数据库  │
                        │   (统计分析)    │
                        └──────────────┘
```

**工作流程：**
1. 客户端发送 OpenAI 格式的请求到本地代理
2. 代理验证身份并应用速率限制
3. 请求被转换为 Coding Plan API 格式
4. 添加伪装工具的 User-Agent 标识
5. 转发到对应的大模型供应商
6. 响应流式返回给客户端，同时记录统计数据

### 🚀 快速开始

#### 安装

```bash
# 克隆仓库
git clone https://github.com/systemime/coding-plan-proxy-go.git
cd coding-plan-proxy-go

# 编译
make build

# 安装到系统 (需要 root 权限)
sudo make install
```

#### 配置

编辑 `/opt/project/coding-plan-proxy/config/config.toml`:

```toml
[server]
listen_host = "127.0.0.1"    # 监听地址
listen_port = 8787           # 监听端口
timeout = 120                # 请求超时(秒)
rate_limit_requests = 100    # 速率限制(每5分钟)
debug = false                # 调试模式

[auth]
provider = "zhipu"                    # 服务商
api_key = "your-coding-plan-api-key"  # Coding Plan API Key
local_api_key = "sk-local-secret"     # 本地认证密钥

[endpoint]
use_coding_endpoint = true   # 使用 Coding Plan 端点
disguise_tool = "opencode"   # 伪装工具: opencode, openclaw, custom
```

#### 启动服务

```bash
# 使用控制脚本
proxy-ctl start

# 或使用 systemctl
sudo systemctl start coding-plan-proxy
sudo systemctl enable coding-plan-proxy  # 开机自启
```

### 📡 API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/` | GET | 服务信息和统计数据 |
| `/v1/models` | GET | 可用模型列表 |
| `/v1/chat/completions` | POST | 聊天补全 (支持流式) |
| `/v1/embeddings` | POST | 文本向量嵌入 |
| `/health` | GET | 健康检查 |
| `/ready` | GET | 就绪检查 |
| `/stats` | GET | 使用统计 |

### 🤖 支持的供应商

| 供应商 | 标识符 | 支持模型 |
|--------|--------|----------|
| **智谱 GLM** | `zhipu` | glm-4-flash, glm-4-plus, glm-4-air, glm-4-long |
| **智谱 GLM v2** | `zhipu_v2` | glm-4-flash, glm-4-plus, glm-4.7, glm-5 |
| **阿里云百炼** | `aliyun` | qwen-turbo, qwen-plus, qwen-max, qwen2.5-coder |
| **MiniMax** | `minimax` | abab6.5s-chat, abab6.5g-chat |
| **DeepSeek** | `deepseek` | deepseek-chat, deepseek-coder |
| **Moonshot (Kimi)** | `moonshot` | moonshot-v1-8k, moonshot-v1-32k, moonshot-v1-128k |

### 💻 客户端配置

**Python (OpenAI SDK):**

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8787/v1",
    api_key="sk-local-secret"
)

response = client.chat.completions.create(
    model="glm-4-flash",
    messages=[{"role": "user", "content": "你好！"}],
    stream=True
)

for chunk in response:
    print(chunk.choices[0].delta.content, end="")
```

**cURL:**

```bash
curl http://127.0.0.1:8787/v1/chat/completions \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer sk-local-secret" \
    -d '{
        "model": "glm-4-flash",
        "messages": [{"role": "user", "content": "你好！"}],
        "stream": true
    }'
```

### 📊 统计与连接信息

```bash
# 查看统计信息
coding-plan-proxy stats

# 查看连接信息
coding-plan-proxy show

# JSON 格式输出 (便于脚本使用)
coding-plan-proxy show --json
```

输出示例：
```
╔════════════════════════════════════════════════════════════╗
║                    Token 使用统计                           ║
╠════════════════════════════════════════════════════════════╣
║  总请求数:     1,234                                        ║
║  总上传 Token: 156,789                                      ║
║  总下载 Token: 89,012                                       ║
║  今日请求:     56                                           ║
╚════════════════════════════════════════════════════════════╝
```

### 🛠️ 控制脚本命令

```bash
proxy-ctl start        # 启动服务
proxy-ctl stop         # 停止服务
proxy-ctl restart      # 重启服务
proxy-ctl status       # 查看状态
proxy-ctl logs         # 查看日志 (实时)
proxy-ctl info         # 显示连接信息
proxy-ctl test         # 测试 API 连接
proxy-ctl config       # 查看/修改配置
proxy-ctl edit         # 编辑配置文件
proxy-ctl enable       # 开机自启
proxy-ctl disable      # 取消开机自启
```

### 🎭 工具伪装功能

代理可以伪装为热门编程工具，让请求看起来像是来自这些工具：

```toml
[endpoint]
disguise_tool = "opencode"  # 选项: opencode, openclaw, custom
custom_user_agent = ""      # 当 disguise_tool = "custom" 时使用
```

支持的伪装工具：
- **opencode**: 伪装为 OpenCode 编程助手
- **openclaw**: 伪装为 OpenClaw AI 编程工具
- **custom**: 使用自定义 User-Agent

### 🔐 安全最佳实践

1. **始终设置 `local_api_key`** 防止未授权访问
2. **绑定到 `127.0.0.1`** 除非需要外部访问
3. **配置速率限制** 防止滥用
4. **定期检查日志** `journalctl -u coding-plan-proxy`
5. **保持更新** 使用最新版本

### 🔧 高级配置

#### 自定义 API 端点

```toml
[api]
base_url = "https://your-custom-api.com/v1"
coding_url = "https://your-custom-api.com/coding/v1"
auth_header = "Authorization"
auth_prefix = "Bearer "
```

#### 环境变量支持

```bash
export PROVIDER=zhipu
export API_KEY=your-api-key
export LOCAL_API_KEY=sk-local-secret
export HOST=127.0.0.1
export PORT=8787
```

### 📁 项目结构

```
coding-plan-proxy-go/
├── cmd/
│   └── coding-plan-proxy/
│       └── main.go           # 主程序入口
├── internal/
│   ├── config/
│   │   └── config.go         # 配置管理
│   ├── proxy/
│   │   └── proxy.go          # 代理转发逻辑
│   ├── ratelimit/
│   │   └── ratelimit.go      # 速率限制
│   ├── server/
│   │   └── server.go         # HTTP 服务器
│   └── storage/
│       └── storage.go        # SQLite 数据存储
├── deploy/
│   ├── coding-plan-proxy.service  # systemd 服务文件
│   ├── config.example.toml        # 配置示例
│   └── proxy-ctl.sh               # 控制脚本
├── go.mod
├── Makefile
└── README.md
```

### ⚠️ 风险预警

> **重要提示：使用前请仔细阅读**

本项目仅供**学习和研究目的**。用户应当了解以下风险：

| 风险类别 | 说明 |
|----------|------|
| 🔴 **服务条款** | 使用此代理可能违反某些大模型供应商的服务条款。用户有责任了解并遵守供应商的政策。 |
| 🔴 **账户风险** | 不当使用可能导致 API 密钥被吊销或账户被供应商暂停。 |
| 🟡 **无担保** | 本软件按"现状"提供，不提供任何担保。作者不对任何损害承担责任。 |
| 🟡 **安全风险** | 在没有适当认证的情况下将代理暴露到公共网络可能导致未授权访问。 |
| 🟢 **自负责任** | 用户需对遵守适用法律法规承担全部责任。 |

**使用本软件即表示您同意：**
- 自行承担使用风险
- 遵守所有适用法律和供应商条款
- 不将其用于任何非法或未授权的目的
- 对任何后果承担全部责任

**建议：**
1. 仅在您拥有合法访问权限的 API 上使用
2. 始终设置 `local_api_key` 以防止未授权访问
3. 除非您完全了解安全影响，否则绑定到 `127.0.0.1`
4. 审查并遵守您供应商的服务条款

### 🤝 贡献

[MIT License](LICENSE)

---

<div align="center">

**⭐ If you find this project helpful, please give it a star! ⭐**

Made with ❤️ by the community

</div>
