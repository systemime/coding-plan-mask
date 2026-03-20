# Coding Plan Mask 功能测试方案

## 1. 背景分析

### 1.1 智谱 GLM 检测机制

根据研究，智谱 GLM 的 Coding Plan API 使用以下方式检测请求来源：

| 检测点 | 说明 |
|--------|------|
| **API 端点** | Coding Plan 使用专属端点 `https://open.bigmodel.cn/api/coding/paas/v4` |
| **User-Agent** | 检测 UA 判断请求是否来自授权工具（Claude Code、Cursor 等） |
| **计费逻辑** | 未伪装的请求会从资源包扣费，而非 Coding Plan 订阅 |

**关键发现**：
- Claude Code CLI 的 User-Agent 格式存在模式差异，当前更接近 `claude-cli/<version> (external, cli)`
- OpenCode 抓包值：`opencode/1.2.27 ai-sdk/provider-utils/3.0.20 runtime/bun/1.3.10`
- OpenClaw 在部分请求路径中使用：`OpenClaw-Gateway/1.0`

### 1.2 当前项目实现分析

**文件**: `internal/proxy/proxy.go`

当前实现会：
- 保留客户端原始请求头
- 覆盖上游认证头
- 覆盖 `User-Agent`
- 在 `claudecode` 模式下补充 `x-app: cli`（如果来包未提供）

**预定义伪装工具** (`internal/config/config.go:104-121`):
```go
var PredefinedDisguiseTools = map[string]DisguiseToolConfig{
    "claudecode": {UserAgent: "claude-cli/2.1.76 (external, cli)"},
    "kimicode":   {UserAgent: "claude-code/0.1.0"},
    "openclaw":   {UserAgent: "OpenClaw-Gateway/1.0"}, // 兼容默认值，可配置覆盖
    "custom":     {UserAgent: ""}, // 使用自定义值
}
```

---

## 2. 测试目标

验证以下核心功能：

1. **请求伪装有效性** - 请求是否能成功通过智谱的 UA 检测
2. **端点路由正确性** - Coding Plan 端点与通用端点的切换
3. **Token 计费验证** - 确认请求从 Coding Plan 扣费而非资源包
4. **流式响应处理** - SSE 流式传输的完整性

---

## 3. 测试方法

### 3.1 方法一：代码对比分析

**对比项目 HTTP 客户端实现与官方工具的差异**

#### Claude Code 实现特征
根据本地抓包与已安装 Claude Code 代码检查：

```
User-Agent: claude-cli/2.1.76 (external, cli)
x-app: cli
```

#### OpenCode 实现特征
根据本地抓包报告：
- 运行时为 Bun
- User-Agent: `opencode/1.2.27 ai-sdk/provider-utils/3.0.20 runtime/bun/1.3.10`
- 无额外特殊认证头

#### 当前项目实现
- User-Agent: 可配置（claudecode/kimicode/opencode/openclaw/custom）
- `opencode` 模式当前无额外专属头

**差异点**:
| 项目 | User-Agent | 额外 Header |
|------|------------|-------------|
| Claude Code | `claude-cli/2.1.76 (external, cli)` | `x-app: cli` |
| OpenCode | `opencode/1.2.27 ai-sdk/provider-utils/3.0.20 runtime/bun/1.3.10` | 无 |
| OpenClaw | `OpenClaw-Gateway/1.0`（部分路径） | 无 |
| **当前项目** | `claude-cli/2.1.76 (external, cli)` / `opencode/1.2.27 ai-sdk/provider-utils/3.0.20 runtime/bun/1.3.10` / `OpenClaw-Gateway/1.0` | `x-app: cli`（仅 `claudecode` 模式） |

**风险点**: Claude Code 的真实请求特征可能继续演进，默认 UA 需要随版本校准。

---

### 3.2 方法二：实际 API 请求测试

#### 测试环境准备

```bash
# 1. 编译项目
cd /opt/project/coding-plan-proxy-go
go build -o coding-plan-mask ./cmd/coding-plan-mask

# 2. 创建测试配置
mkdir -p /tmp/coding-plan-test
cat > /tmp/coding-plan-test/config.toml << 'EOF'
[server]
listen_host = "127.0.0.1"
listen_port = 8787
timeout = 120
rate_limit_requests = 1000

[auth]
provider = "zhipu"
api_key = "YOUR_CODING_PLAN_API_KEY"  # 替换为真实 Key
local_api_key = "test-local-key"

[endpoint]
use_coding_endpoint = true
disguise_tool = "opencode"
EOF
```

#### 测试用例

**测试 1: 基础连接测试**
```bash
# 启动代理
./coding-plan-mask -config /tmp/coding-plan-test/config.toml &

# 测试健康检查
curl http://127.0.0.1:8787/health
# 预期: {"status":"healthy","time":"..."}

# 测试就绪检查
curl http://127.0.0.1:8787/ready
# 预期: {"ready":true}
```

**测试 2: 模型列表测试**
```bash
curl http://127.0.0.1:8787/v1/models \
  -H "Authorization: Bearer test-local-key"
# 预期: 返回支持的模型列表
```

**测试 3: 非流式聊天补全测试**
```bash
curl http://127.0.0.1:8787/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-local-key" \
  -d '{
    "model": "glm-4-flash",
    "messages": [{"role": "user", "content": "说你好"}],
    "stream": false
  }'
# 预期: 返回正常响应，检查 token 使用情况
```

**测试 4: 流式聊天补全测试**
```bash
curl http://127.0.0.1:8787/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-local-key" \
  -d '{
    "model": "glm-4-flash",
    "messages": [{"role": "user", "content": "说你好"}],
    "stream": true
  }'
# 预期: 返回 SSE 流式响应
```

**测试 5: 统计信息验证**
```bash
curl http://127.0.0.1:8787/stats
# 预期: 显示请求计数和 token 统计
```

---

### 3.3 方法三：计费验证测试（关键）

**目的**: 验证请求是否从 Coding Plan 扣费，而非资源包

#### 步骤：

1. **记录初始状态**
   - 登录智谱控制台，记录 Coding Plan 剩余额度
   - 记录资源包剩余额度

2. **发送测试请求**
   ```bash
   # 通过代理发送请求
   curl http://127.0.0.1:8787/v1/chat/completions \
     -H "Authorization: Bearer test-local-key" \
     -H "Content-Type: application/json" \
     -d '{"model":"glm-4-flash","messages":[{"role":"user","content":"测试100字以上内容"}]}'
   ```

3. **验证扣费来源**
   - 检查 Coding Plan 额度是否减少 ✓
   - 检查资源包额度是否保持不变 ✓

4. **对比测试（直接请求通用端点）**
   ```bash
   # 直接请求通用端点（预期会从资源包扣费）
   curl https://open.bigmodel.cn/api/paas/v4/chat/completions \
     -H "Authorization: Bearer YOUR_API_KEY" \
     -H "Content-Type: application/json" \
     -d '{"model":"glm-4-flash","messages":[{"role":"user","content":"测试"}]}'
   ```
   - 检查资源包是否减少

---

### 3.4 方法四：抓包对比分析

**使用 Wireshark/tcpdump 对比请求头**

```bash
# 抓取代理发出的请求
tcpdump -i any -A 'host open.bigmodel.cn and port 443' -w proxy_traffic.pcap &

# 发送测试请求
curl http://127.0.0.1:8787/v1/chat/completions ...

# 停止抓包并分析
# 使用 Wireshark 打开 proxy_traffic.pcap
# 检查 TLS 握手后的 HTTP 头
```

**关注点**:
1. User-Agent 值是否正确设置为抓包报告中的 OpenCode UA
2. 是否存在其他可能暴露身份的 Header
3. 请求体格式是否与官方工具一致

---

## 4. 改进建议

### 4.1 代码改进

**✅ 已修复 - 问题 1**: `X-Client-Type: coding-tool` 是自定义 Header

```go
// 已移除 X-Client-Type header
headers := map[string]string{
    "Content-Type":      "application/json",
    provider.AuthHeader: provider.AuthPrefix + apiKey,
    "User-Agent":        userAgent,
    "Accept":            "text/event-stream",
}
```

**问题 2**: Token 估算不准确 (`proxy.go:499-514`)

```go
// 当前实现
func estimateInputTokens(reqBody map[string]interface{}) int {
    return totalChars / 2 // 粗略估算
}
```

**建议**: 使用 tiktoken 或 API 返回的 usage 数据

### 4.2 配置改进

**✅ 已完成 - 使用有来源依据的伪装工具选项**:

```go
// config.go
// User-Agent 来源说明:
// - claudecode: 当前 Claude Code CLI 请求格式，默认值可通过配置覆盖
// - openclaw: OpenClaw 部分请求路径会发送 OpenClaw-Gateway/1.0，本项目保留该兼容默认值并允许覆盖
// - kimicode: Kimi Code API 订阅认证要求 claude-code/0.1.0
var PredefinedDisguiseTools = map[string]DisguiseToolConfig{
    "claudecode": {UserAgent: "claude-cli/2.1.76 (external, cli)"},
    "kimicode":   {UserAgent: "claude-code/0.1.0"},  // Kimi Code API 订阅认证格式
    "openclaw":   {UserAgent: "OpenClaw-Gateway/1.0"}, // 兼容默认值，可配置覆盖
    "custom":     {UserAgent: ""},                   // 使用自定义值
}
```

**参考来源**:
- [OpenClaw Issue #30099](https://github.com/openclaw/openclaw/issues/30099)
- 本地 Claude Code 请求抓包
- 已安装 Claude Code CLI 代码检查
- [Linux.do 智谱 CC 月包讨论](https://linux.do/t/topic/934378)

---

## 5. 测试检查清单

### 5.1 功能测试
- [ ] 服务正常启动
- [ ] 健康检查端点响应
- [ ] 模型列表返回正确
- [ ] 非流式请求成功
- [ ] 流式请求成功
- [ ] Token 统计记录正确
- [ ] 速率限制生效

### 5.2 伪装测试
- [ ] User-Agent 正确设置
- [ ] 请求到达 Coding Plan 端点
- [ ] 计费从 Coding Plan 扣除
- [ ] 资源包未被扣除

### 5.3 安全测试
- [ ] 本地 API Key 验证生效
- [ ] 无效 Key 被拒绝
- [ ] 请求体大小限制生效

---

## 6. 参考资源

- [智谱 AI Claude Code 文档](https://docs.bigmodel.cn/cn/guide/develop/claude)
- [智谱 API 错误码](https://docs.bigmodel.cn/cn/api/api-code)
- [OpenCode GitHub](https://github.com/opencode-ai/opencode) (已归档)
- [Claude Code User-Agent 格式](https://developer.konghq.com/how-to/use-claude-code-with-ai-gateway-anthropic/)
- [GitHub Issue: Coding Plan 端点配置](https://github.com/agentscope-ai/CoPaw/issues/202)
