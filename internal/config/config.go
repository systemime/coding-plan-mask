// Package config 提供配置管理功能
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// ProviderConfig 服务商配置
type ProviderConfig struct {
	Name           string            `toml:"name"`
	CodingBaseURL  string            `toml:"coding_base_url"`
	GeneralBaseURL string            `toml:"general_base_url"`
	AuthHeader     string            `toml:"auth_header"`
	AuthPrefix     string            `toml:"auth_prefix"`
	UserAgent      string            `toml:"user_agent"`
	ExtraHeaders   map[string]string `toml:"extra_headers"`
	Models         []string          `toml:"models"`
}

// ConfigFile TOML 配置文件结构
type ConfigFile struct {
	Server   ServerConfig   `toml:"server"`
	Auth     AuthConfig     `toml:"auth"`
	Endpoint EndpointConfig `toml:"endpoint"`
	API      APIConfig      `toml:"api"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	ListenHost         string `toml:"listen_host"`
	ListenPort         int    `toml:"listen_port"`
	Debug              bool   `toml:"debug"`
	Timeout            int    `toml:"timeout"`
	RateLimitRequests  int    `toml:"rate_limit_requests"`
	MaxRequestBodySize int64  `toml:"max_request_body_size"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	Provider    string `toml:"provider"`
	APIKey      string `toml:"api_key"`
	LocalAPIKey string `toml:"local_api_key"`
}

// EndpointConfig 端点配置
type EndpointConfig struct {
	UseCodingEndpoint   bool   `toml:"use_coding_endpoint"`
	CustomUserAgent     string `toml:"custom_user_agent"`
	ClaudeCodeUserAgent string `toml:"claude_code_user_agent"`
	OpenClawUserAgent   string `toml:"openclaw_user_agent"`
	OpenCodeUserAgent   string `toml:"opencode_user_agent"`
	// 伪装工具类型: claudecode, kimicode, openclaw, custom
	// 兼容旧值: opencode
	DisguiseTool string `toml:"disguise_tool"`
}

// APIConfig API URL 配置
type APIConfig struct {
	// 自定义 API 基础 URL（留空使用默认）
	BaseURL string `toml:"base_url"`
	// Coding Plan 端点 URL（留空使用默认）
	CodingURL string `toml:"coding_url"`
	// 认证头名称
	AuthHeader string `toml:"auth_header"`
	// 认证前缀
	AuthPrefix string `toml:"auth_prefix"`
}

// Config 应用配置（运行时使用）
type Config struct {
	mu sync.RWMutex

	Provider            string
	APIKey              string
	LocalAPIKey         string
	ListenHost          string
	ListenPort          int
	UseCodingEndpoint   bool
	CustomUserAgent     string
	ClaudeCodeUserAgent string
	OpenClawUserAgent   string
	OpenCodeUserAgent   string
	DisguiseTool        string // 伪装工具: claudecode, kimicode, openclaw, custom
	Debug               bool
	RateLimitRequests   int
	Timeout             int
	MaxRequestBodySize  int64

	// 自定义 API 配置
	CustomBaseURL    string
	CustomCodingURL  string
	CustomAuthHeader string
	CustomAuthPrefix string

	configPath string
}

// DisguiseToolConfig 伪装工具配置
type DisguiseToolConfig struct {
	Name      string
	UserAgent string
	ExtraInfo string
}

const (
	DefaultClaudeCodeUserAgent = "claude-cli/2.1.76 (external, cli)"
	DefaultOpenClawUserAgent   = "OpenClaw-Gateway/1.0"
	DefaultOpenCodeUserAgent   = "opencode/1.2.27 ai-sdk/provider-utils/3.0.20 runtime/bun/1.3.10"
	ClaudeCodeAppHeaderValue   = "cli"
)

// PredefinedDisguiseTools 预定义的伪装工具
// User-Agent 来源说明:
// - claudecode: 当前 Claude Code CLI 请求格式，默认值可通过配置覆盖
// - openclaw: OpenClaw 部分请求路径会发送 OpenClaw-Gateway/1.0，本项目保留该兼容默认值并允许覆盖
// - opencode: 基于本地实际抓包报告的 OpenCode 1.2.27 请求格式，保留 legacy disguise_tool 标识
// - kimicode: Kimi Code API 订阅认证要求 claude-code/0.1.0
// 参考: 本地 Claude Code 请求抓包与已安装 CLI 代码检查
// 参考: https://github.com/openclaw/openclaw/issues/30099
var PredefinedDisguiseTools = map[string]DisguiseToolConfig{
	"claudecode": {
		Name:      "Claude Code",
		UserAgent: DefaultClaudeCodeUserAgent,
		ExtraInfo: "Anthropic CLI 风格请求头（默认会附加 x-app: cli）",
	},
	"kimicode": {
		Name:      "Kimi Code 兼容",
		UserAgent: "claude-code/0.1.0",
		ExtraInfo: "Kimi Code API 订阅认证格式",
	},
	"openclaw": {
		Name:      "OpenClaw",
		UserAgent: DefaultOpenClawUserAgent,
		ExtraInfo: "OpenClaw 兼容默认值（可通过配置覆盖）",
	},
	"opencode": {
		Name:      "OpenCode (Legacy)",
		UserAgent: DefaultOpenCodeUserAgent,
		ExtraInfo: "Legacy disguise_tool 标识，默认 UA 已按本地抓包报告更新",
	},
	"custom": {
		Name:      "自定义",
		UserAgent: "",
		ExtraInfo: "使用 custom_user_agent 配置",
	},
}

func normalizeDisguiseTool(tool string) string {
	tool = strings.ToLower(strings.TrimSpace(tool))
	if tool == "" {
		return "claudecode"
	}
	return tool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Provider:           "zhipu",
		ListenHost:         "127.0.0.1",
		ListenPort:         8787,
		UseCodingEndpoint:  true,
		Debug:              false,
		RateLimitRequests:  100,
		Timeout:            120,
		MaxRequestBodySize: 10 * 1024 * 1024,
	}
}

// getExecutableDir 获取可执行文件所在目录
func getExecutableDir() string {
	execPath, err := os.Executable()
	if err != nil {
		// 回退到当前工作目录
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Dir(execPath)
}

var defaultConfigNames = []string{
	"config.toml",
	"config.eg",
	"config.example.toml",
}

func findConfigInDir(dir string) (string, bool) {
	for _, name := range defaultConfigNames {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

// getDefaultConfigPath 获取默认配置文件路径（在可执行文件所在目录）
func getDefaultConfigPath() string {
	execDir := getExecutableDir()
	if path, ok := findConfigInDir(execDir); ok {
		return path
	}
	return filepath.Join(execDir, defaultConfigNames[0])
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		path = getDefaultConfigPath()
	}
	cfg.configPath = path

	// 记录配置路径
	absPath, _ := filepath.Abs(path)
	cfg.configPath = absPath

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 配置文件不存在，创建默认配置并提示用户
			if err := createDefaultConfig(path); err != nil {
				fmt.Printf("⚠️  无法创建默认配置文件: %v\n", err)
			} else {
				fmt.Println()
				fmt.Println("╔════════════════════════════════════════════════════════════╗")
				fmt.Println("║           首次运行 - 已创建默认配置文件                      ║")
				fmt.Println("╠════════════════════════════════════════════════════════════╣")
				fmt.Printf("║  配置文件: %-48s ║\n", path)
				fmt.Println("╠════════════════════════════════════════════════════════════╣")
				fmt.Println("║  请编辑配置文件填写以下信息:                                 ║")
				fmt.Println("║  1. [auth].api_key - 你的 Coding Plan API Key               ║")
				fmt.Println("║  2. [auth].local_api_key - 本地认证密钥 (可选)               ║")
				fmt.Println("║  3. [auth].provider - 服务商 (zhipu/aliyun/minimax/...)     ║")
				fmt.Println("╚════════════════════════════════════════════════════════════╝")
				fmt.Println()
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfgFile ConfigFile
	if _, err := toml.Decode(string(data), &cfgFile); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 映射到 Config
	if cfgFile.Server.ListenHost != "" {
		cfg.ListenHost = cfgFile.Server.ListenHost
	}
	if cfgFile.Server.ListenPort != 0 {
		cfg.ListenPort = cfgFile.Server.ListenPort
	}
	cfg.Debug = cfgFile.Server.Debug
	if cfgFile.Server.Timeout != 0 {
		cfg.Timeout = cfgFile.Server.Timeout
	}
	if cfgFile.Server.RateLimitRequests != 0 {
		cfg.RateLimitRequests = cfgFile.Server.RateLimitRequests
	}
	if cfgFile.Server.MaxRequestBodySize != 0 {
		cfg.MaxRequestBodySize = cfgFile.Server.MaxRequestBodySize
	}

	if cfgFile.Auth.Provider != "" {
		cfg.Provider = cfgFile.Auth.Provider
	}
	cfg.APIKey = cfgFile.Auth.APIKey
	cfg.LocalAPIKey = cfgFile.Auth.LocalAPIKey

	cfg.UseCodingEndpoint = cfgFile.Endpoint.UseCodingEndpoint
	cfg.CustomUserAgent = cfgFile.Endpoint.CustomUserAgent
	cfg.ClaudeCodeUserAgent = strings.TrimSpace(cfgFile.Endpoint.ClaudeCodeUserAgent)
	cfg.OpenClawUserAgent = strings.TrimSpace(cfgFile.Endpoint.OpenClawUserAgent)
	cfg.OpenCodeUserAgent = strings.TrimSpace(cfgFile.Endpoint.OpenCodeUserAgent)
	cfg.DisguiseTool = normalizeDisguiseTool(cfgFile.Endpoint.DisguiseTool)

	// 自定义 API 配置
	cfg.CustomBaseURL = cfgFile.API.BaseURL
	cfg.CustomCodingURL = cfgFile.API.CodingURL
	cfg.CustomAuthHeader = cfgFile.API.AuthHeader
	cfg.CustomAuthPrefix = cfgFile.API.AuthPrefix

	cfg.loadFromEnv()
	return cfg, nil
}

func (c *Config) loadFromEnv() {
	if v := os.Getenv("PROVIDER"); v != "" {
		c.Provider = v
	}
	if v := os.Getenv("API_KEY"); v != "" {
		c.APIKey = v
	}
	if v := os.Getenv("LOCAL_API_KEY"); v != "" {
		c.LocalAPIKey = v
	}
	if v := os.Getenv("HOST"); v != "" {
		c.ListenHost = v
	}
	if v := os.Getenv("PORT"); v != "" {
		fmt.Sscanf(v, "%d", &c.ListenPort)
	}
	if v := os.Getenv("DEBUG"); strings.ToLower(v) == "true" {
		c.Debug = true
	}
	if v := os.Getenv("API_BASE_URL"); v != "" {
		c.CustomBaseURL = v
	}
	if v := os.Getenv("API_CODING_URL"); v != "" {
		c.CustomCodingURL = v
	}
	if v := os.Getenv("DISGUISE_TOOL"); v != "" {
		c.DisguiseTool = normalizeDisguiseTool(v)
	}
	if v := os.Getenv("CUSTOM_USER_AGENT"); v != "" {
		c.CustomUserAgent = v
	}
	if v := os.Getenv("CLAUDE_CODE_USER_AGENT"); v != "" {
		c.ClaudeCodeUserAgent = strings.TrimSpace(v)
	}
	if v := os.Getenv("OPENCLAW_USER_AGENT"); v != "" {
		c.OpenClawUserAgent = strings.TrimSpace(v)
	}
	if v := os.Getenv("OPENCODE_USER_AGENT"); v != "" {
		c.OpenCodeUserAgent = strings.TrimSpace(v)
	}
}

// Set 设置配置项
func (c *Config) Set(key string, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch key {
	case "provider":
		c.Provider = value
	case "api_key":
		c.APIKey = value
	case "local_api_key":
		c.LocalAPIKey = value
	case "listen_host":
		c.ListenHost = value
	case "listen_port":
		fmt.Sscanf(value, "%d", &c.ListenPort)
	case "debug":
		c.Debug = strings.ToLower(value) == "true"
	case "rate_limit_requests":
		fmt.Sscanf(value, "%d", &c.RateLimitRequests)
	case "timeout":
		fmt.Sscanf(value, "%d", &c.Timeout)
	case "use_coding_endpoint":
		c.UseCodingEndpoint = strings.ToLower(value) == "true"
	case "custom_user_agent":
		c.CustomUserAgent = value
	case "claude_code_user_agent":
		c.ClaudeCodeUserAgent = strings.TrimSpace(value)
	case "openclaw_user_agent":
		c.OpenClawUserAgent = strings.TrimSpace(value)
	case "opencode_user_agent":
		c.OpenCodeUserAgent = strings.TrimSpace(value)
	case "disguise_tool":
		c.DisguiseTool = normalizeDisguiseTool(value)
	case "api_base_url", "base_url":
		c.CustomBaseURL = value
	case "api_coding_url", "coding_url":
		c.CustomCodingURL = value
	case "auth_header":
		c.CustomAuthHeader = value
	case "auth_prefix":
		c.CustomAuthPrefix = value
	default:
		return fmt.Errorf("未知配置项: %s", key)
	}
	return nil
}

// GetProviderConfig 获取当前服务商配置（支持自定义 URL）
func (c *Config) GetProviderConfig() (*ProviderConfig, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	provider, ok := Providers[c.Provider]
	if !ok {
		return nil, fmt.Errorf("不支持的服务商: %s", c.Provider)
	}

	// 复制配置，以便修改
	cfg := provider

	// 如果配置了自定义 URL，则覆盖默认值
	if c.CustomBaseURL != "" {
		cfg.GeneralBaseURL = c.CustomBaseURL
	}
	if c.CustomCodingURL != "" {
		cfg.CodingBaseURL = c.CustomCodingURL
	}
	// 如果同时设置了 base_url 且没有单独设置 coding_url，则两者都使用 base_url
	if c.CustomBaseURL != "" && c.CustomCodingURL == "" {
		cfg.CodingBaseURL = c.CustomBaseURL
	}
	if c.CustomAuthHeader != "" {
		cfg.AuthHeader = c.CustomAuthHeader
	}
	if c.CustomAuthPrefix != "" {
		cfg.AuthPrefix = c.CustomAuthPrefix
	}

	return &cfg, nil
}

// GetEffectiveUserAgent 获取有效的 User-Agent（基于伪装工具设置）
func (c *Config) GetEffectiveUserAgent() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 优先使用自定义 User-Agent
	if c.CustomUserAgent != "" {
		return c.CustomUserAgent
	}

	if normalizeDisguiseTool(c.DisguiseTool) == "claudecode" && c.ClaudeCodeUserAgent != "" {
		return c.ClaudeCodeUserAgent
	}
	if normalizeDisguiseTool(c.DisguiseTool) == "openclaw" && c.OpenClawUserAgent != "" {
		return c.OpenClawUserAgent
	}
	if normalizeDisguiseTool(c.DisguiseTool) == "opencode" && c.OpenCodeUserAgent != "" {
		return c.OpenCodeUserAgent
	}

	// 根据伪装工具选择
	if tool, ok := PredefinedDisguiseTools[normalizeDisguiseTool(c.DisguiseTool)]; ok && tool.UserAgent != "" {
		return tool.UserAgent
	}

	// 默认使用 claudecode
	return PredefinedDisguiseTools["claudecode"].UserAgent
}

// GetDisguiseHeaders 返回伪装工具额外需要补充的请求头。
func (c *Config) GetDisguiseHeaders() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	switch normalizeDisguiseTool(c.DisguiseTool) {
	case "claudecode":
		return map[string]string{
			"X-App": ClaudeCodeAppHeaderValue,
		}
	default:
		return nil
	}
}

// GetProviderConfigByName 根据名称获取服务商配置
func GetProviderConfigByName(name string) (*ProviderConfig, error) {
	provider, ok := Providers[name]
	if !ok {
		return nil, fmt.Errorf("不支持的服务商: %s", name)
	}
	return &provider, nil
}

// GetSafe 返回安全的配置副本
func (c *Config) GetSafe() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"provider":               c.Provider,
		"api_key":                maskAPIKey(c.APIKey),
		"local_api_key":          maskAPIKey(c.LocalAPIKey),
		"listen_host":            c.ListenHost,
		"listen_port":            c.ListenPort,
		"use_coding_endpoint":    c.UseCodingEndpoint,
		"disguise_tool":          c.DisguiseTool,
		"custom_user_agent":      c.CustomUserAgent,
		"claude_code_user_agent": c.ClaudeCodeUserAgent,
		"openclaw_user_agent":    c.OpenClawUserAgent,
		"opencode_user_agent":    c.OpenCodeUserAgent,
		"debug":                  c.Debug,
		"rate_limit_requests":    c.RateLimitRequests,
		"timeout":                c.Timeout,
		"api_base_url":           c.CustomBaseURL,
		"api_coding_url":         c.CustomCodingURL,
	}
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		if key == "" {
			return "(未设置)"
		}
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// GetConfigPath 获取配置文件路径
func (c *Config) GetConfigPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.configPath
}

// GetProviderNames 获取所有服务商名称
func GetProviderNames() []string {
	names := make([]string, 0, len(Providers))
	for name := range Providers {
		names = append(names, name)
	}
	return names
}

// createDefaultConfig 创建默认配置文件
func createDefaultConfig(path string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	defaultContent := `# Coding Plan Mask 配置文件
# 文档: https://github.com/systemime/coding-plan-mask

# ============================================================================
# 服务器配置
# ============================================================================
[server]
# 监听地址
listen_host = "127.0.0.1"
# 监听端口
listen_port = 8787
# 调试模式
debug = false
# 请求超时(秒)
timeout = 120
# 速率限制(每5分钟窗口)
rate_limit_requests = 100
# 最大请求体大小(字节)
max_request_body_size = 10485760

# ============================================================================
# 认证配置
# ============================================================================
[auth]
# 服务商: zhipu, zhipu_v2, aliyun, minimax, deepseek, moonshot, custom
provider = "zhipu"
# Coding Plan API Key (用于向云厂商发起请求)
# 获取方式: https://open.bigmodel.cn/
api_key = ""
# 本地 API Key (客户端连接代理时使用，留空则不验证)
local_api_key = "sk-local-your-secret-key"

# ============================================================================
# 端点配置
# ============================================================================
[endpoint]
# 是否使用 Coding Plan 端点
use_coding_endpoint = true
# 伪装工具: claudecode, kimicode, openclaw, custom
disguise_tool = "claudecode"
# Claude Code 模式的默认 User-Agent
# 默认值基于当前 Claude Code CLI 的真实请求格式
claude_code_user_agent = "claude-cli/2.1.76 (external, cli)"
# OpenClaw 模式的兼容默认 User-Agent
# 该值用于兼容部分 OpenClaw 请求路径，可按需覆盖
openclaw_user_agent = "OpenClaw-Gateway/1.0"
# OpenCode 模式的默认 User-Agent
# 默认值基于本地抓包报告中的 OpenCode 1.2.27 请求格式
opencode_user_agent = "opencode/1.2.27 ai-sdk/provider-utils/3.0.20 runtime/bun/1.3.10"
# 自定义 User-Agent (留空使用默认，仅当 disguise_tool = "custom" 时生效)
custom_user_agent = ""

# ============================================================================
# API URL 配置 (可选 - 自定义 API 端点)
# ============================================================================
[api]
# 自定义 API 基础 URL (留空使用服务商默认地址)
base_url = ""
# Coding Plan 端点 URL (留空使用服务商默认地址)
coding_url = ""
# 认证头名称 (留空使用默认 "Authorization")
auth_header = ""
# 认证前缀 (留空使用默认 "Bearer ")
auth_prefix = ""
`

	return os.WriteFile(path, []byte(defaultContent), 0644)
}

// Providers 支持的服务商列表（默认配置）
var Providers = map[string]ProviderConfig{
	"zhipu": {
		Name:           "智谱 GLM",
		CodingBaseURL:  "https://open.bigmodel.cn/api/coding/paas/v4",
		GeneralBaseURL: "https://open.bigmodel.cn/api/paas/v4",
		AuthHeader:     "Authorization",
		AuthPrefix:     "Bearer ",
		UserAgent:      DefaultOpenCodeUserAgent,
		ExtraHeaders:   map[string]string{},
		Models:         []string{"glm-4-flash", "glm-4-plus", "glm-4-air", "glm-4-long", "glm-4"},
	},
	"zhipu_v2": {
		Name:           "智谱 GLM (api.z.ai)",
		CodingBaseURL:  "https://api.z.ai/api/coding/paas/v4",
		GeneralBaseURL: "https://api.z.ai/api/paas/v4",
		AuthHeader:     "Authorization",
		AuthPrefix:     "Bearer ",
		UserAgent:      DefaultOpenCodeUserAgent,
		ExtraHeaders:   map[string]string{},
		Models:         []string{"glm-4-flash", "glm-4-plus", "glm-4-air", "glm-4-long", "glm-4", "glm-4.7", "glm-5"},
	},
	"aliyun": {
		Name:           "阿里云百炼",
		CodingBaseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
		GeneralBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		AuthHeader:     "Authorization",
		AuthPrefix:     "Bearer ",
		UserAgent:      DefaultOpenCodeUserAgent,
		ExtraHeaders:   map[string]string{"X-DashScope-SSE": "enable"},
		Models:         []string{"qwen-turbo", "qwen-plus", "qwen-max", "qwen2.5-coder-32b-instruct"},
	},
	"minimax": {
		Name:           "MiniMax",
		CodingBaseURL:  "https://api.minimax.chat/v1",
		GeneralBaseURL: "https://api.minimax.chat/v1",
		AuthHeader:     "Authorization",
		AuthPrefix:     "Bearer ",
		UserAgent:      DefaultOpenCodeUserAgent,
		ExtraHeaders:   map[string]string{},
		Models:         []string{"abab6.5s-chat", "abab6.5g-chat", "abab6.5-chat"},
	},
	"deepseek": {
		Name:           "DeepSeek",
		CodingBaseURL:  "https://api.deepseek.com/v1",
		GeneralBaseURL: "https://api.deepseek.com/v1",
		AuthHeader:     "Authorization",
		AuthPrefix:     "Bearer ",
		UserAgent:      DefaultOpenCodeUserAgent,
		ExtraHeaders:   map[string]string{},
		Models:         []string{"deepseek-chat", "deepseek-coder"},
	},
	"moonshot": {
		Name:           "Moonshot (Kimi)",
		CodingBaseURL:  "https://api.moonshot.cn/v1",
		GeneralBaseURL: "https://api.moonshot.cn/v1",
		AuthHeader:     "Authorization",
		AuthPrefix:     "Bearer ",
		UserAgent:      DefaultOpenCodeUserAgent,
		ExtraHeaders:   map[string]string{},
		Models:         []string{"moonshot-v1-8k", "moonshot-v1-32k", "moonshot-v1-128k"},
	},
	"custom": {
		Name:           "自定义服务商",
		CodingBaseURL:  "",
		GeneralBaseURL: "",
		AuthHeader:     "Authorization",
		AuthPrefix:     "Bearer ",
		UserAgent:      DefaultOpenCodeUserAgent,
		ExtraHeaders:   map[string]string{},
		Models:         []string{},
	},
}
