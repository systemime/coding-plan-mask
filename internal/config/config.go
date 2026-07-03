// Package config 提供配置管理功能
package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
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
	Security SecurityConfig `toml:"security"`
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
	// 删除代理伪装请求的版本控制路径
	// 例如：请求 /v1/models 时，转发时只拼接 /models 部分
	RemoveVersionPath bool `toml:"remove_version_path"`
	// 模拟 /models 响应
	MockModels bool `toml:"mock_models"`
	// 模拟 /models 响应内容 (JSON 字符串)
	MockModelsResp string `toml:"mock_models_resp"`
	// Anthropic 格式兼容模式
	// 启用后会修复请求体中的 schema 字段，将 null 转为正确的默认值
	UseAnthropic bool `toml:"use_anthropic"`
}

type SecurityConfig struct {
	Enabled       bool                 `toml:"enabled"`
	AuditDir      string               `toml:"audit_dir"`
	DefaultTrack  string               `toml:"default_track"`
	DefaultTopK   int                  `toml:"default_top_k"`
	MaxAuditItems int                  `toml:"max_audit_items"`
	HandlingS2    string               `toml:"handling_s2"`
	HandlingS3    string               `toml:"handling_s3"`
	PlaceholderS3 string               `toml:"placeholder_s3"`
	SessionHeader string               `toml:"session_header"`
	Redaction     SecurityRedactConfig `toml:"redaction"`
	Rules         SecurityRulesConfig  `toml:"rules"`
}

type SecurityRedactConfig struct {
	InternalIP     bool `toml:"internal_ip"`
	Email          bool `toml:"email"`
	EnvVar         bool `toml:"env_var"`
	CreditCard     bool `toml:"credit_card"`
	ChinesePhone   bool `toml:"chinese_phone"`
	ChineseID      bool `toml:"chinese_id"`
	ChineseAddress bool `toml:"chinese_address"`
	PIN            bool `toml:"pin"`
}

type SecurityRulesConfig struct {
	KeywordsS2 []string               `toml:"keywords_s2"`
	KeywordsS3 []string               `toml:"keywords_s3"`
	PatternsS2 []string               `toml:"patterns_s2"`
	PatternsS3 []string               `toml:"patterns_s3"`
	ToolsS2    SecurityToolRuleConfig `toml:"tools_s2"`
	ToolsS3    SecurityToolRuleConfig `toml:"tools_s3"`
}

type SecurityToolRuleConfig struct {
	Tools []string `toml:"tools"`
	Paths []string `toml:"paths"`
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
	CustomBaseURL     string
	CustomCodingURL   string
	CustomAuthHeader  string
	CustomAuthPrefix  string
	RemoveVersionPath bool
	MockModels        bool
	MockModelsResp    string
	UseAnthropic      bool // Anthropic 格式兼容模式
	Security          SecurityConfig

	configPath string
}

// DisguiseToolConfig 伪装工具配置
type DisguiseToolConfig struct {
	Name      string
	UserAgent string
	ExtraInfo string
}

const (
	DefaultClaudeCodeUserAgent = "claude-cli/2.1.88 (external, cli)"
	DefaultOpenClawUserAgent   = "OpenClaw-Gateway/1.0"
	DefaultOpenCodeUserAgent   = "opencode/1.2.27 ai-sdk/provider-utils/3.0.20 runtime/bun/1.3.10"
	ClaudeCodeAppHeaderValue   = "cli"
	// ClaudeCodeVersion 用于生成 x-anthropic-billing-header 中的 cc_version 字段
	ClaudeCodeVersion = "2.1.88"
)

// DefaultMockModelsResp 默认的 /models 模拟响应
const DefaultMockModelsResp = `{"object":"list","data":[{"id":"gpt-4","object":"model","owned_by":"organization"}]}`

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

func NormalizeDisguiseTool(tool string) string {
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
		MockModelsResp:     DefaultMockModelsResp,
		Security: SecurityConfig{
			Enabled:       false,
			DefaultTrack:  "clean",
			DefaultTopK:   5,
			MaxAuditItems: 2000,
			HandlingS2:    "redact",
			HandlingS3:    "block",
			PlaceholderS3: "[PRIVATE]",
			SessionHeader: "X-Session-Id",
			Redaction: SecurityRedactConfig{
				Email:        true,
				ChinesePhone: true,
				ChineseID:    true,
			},
		},
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
	metadata, err := toml.Decode(string(data), &cfgFile)
	if err != nil {
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
	cfg.DisguiseTool = NormalizeDisguiseTool(cfgFile.Endpoint.DisguiseTool)

	// 自定义 API 配置
	cfg.CustomBaseURL = cfgFile.API.BaseURL
	cfg.CustomCodingURL = cfgFile.API.CodingURL
	cfg.CustomAuthHeader = cfgFile.API.AuthHeader
	cfg.CustomAuthPrefix = cfgFile.API.AuthPrefix
	cfg.RemoveVersionPath = cfgFile.API.RemoveVersionPath
	cfg.MockModels = cfgFile.API.MockModels
	cfg.UseAnthropic = cfgFile.API.UseAnthropic
	if cfgFile.API.MockModelsResp != "" {
		cfg.MockModelsResp = cfgFile.API.MockModelsResp
	}
	cfg.Security.Enabled = cfgFile.Security.Enabled
	if cfgFile.Security.AuditDir != "" {
		cfg.Security.AuditDir = cfgFile.Security.AuditDir
	}
	if cfgFile.Security.DefaultTrack != "" {
		cfg.Security.DefaultTrack = cfgFile.Security.DefaultTrack
	}
	if cfgFile.Security.DefaultTopK != 0 {
		cfg.Security.DefaultTopK = cfgFile.Security.DefaultTopK
	}
	if cfgFile.Security.MaxAuditItems != 0 {
		cfg.Security.MaxAuditItems = cfgFile.Security.MaxAuditItems
	}
	if cfgFile.Security.HandlingS2 != "" {
		cfg.Security.HandlingS2 = cfgFile.Security.HandlingS2
	}
	if cfgFile.Security.HandlingS3 != "" {
		cfg.Security.HandlingS3 = cfgFile.Security.HandlingS3
	}
	if cfgFile.Security.PlaceholderS3 != "" {
		cfg.Security.PlaceholderS3 = cfgFile.Security.PlaceholderS3
	}
	if cfgFile.Security.SessionHeader != "" {
		cfg.Security.SessionHeader = cfgFile.Security.SessionHeader
	}
	cfg.Security.Redaction = mergeSecurityRedactionConfig(cfg.Security.Redaction, cfgFile.Security.Redaction, metadata)
	cfg.Security.Rules = cfgFile.Security.Rules
	if err := validateSecurityRules(cfg.Security.Rules); err != nil {
		return nil, err
	}

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
		c.DisguiseTool = NormalizeDisguiseTool(v)
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
	if v := os.Getenv("REMOVE_VERSION_PATH"); strings.ToLower(v) == "true" {
		c.RemoveVersionPath = true
	}
	if v := os.Getenv("MOCK_MODELS"); strings.ToLower(v) == "true" {
		c.MockModels = true
	}
	if v := os.Getenv("MOCK_MODELS_RESP"); v != "" {
		c.MockModelsResp = v
	}
	if v := os.Getenv("USE_ANTHROPIC"); strings.ToLower(v) == "true" {
		c.UseAnthropic = true
	}
	if v := os.Getenv("SECURITY_ENABLED"); strings.ToLower(v) == "true" {
		c.Security.Enabled = true
	}
	if v := os.Getenv("SECURITY_AUDIT_DIR"); v != "" {
		c.Security.AuditDir = v
	}
	if v := os.Getenv("SECURITY_HANDLING_S2"); v != "" {
		c.Security.HandlingS2 = v
	}
	if v := os.Getenv("SECURITY_HANDLING_S3"); v != "" {
		c.Security.HandlingS3 = v
	}
	if v := os.Getenv("SECURITY_REDACT_EMAIL"); v != "" {
		c.Security.Redaction.Email = strings.ToLower(v) == "true"
	}
	if v := os.Getenv("SECURITY_REDACT_CHINESE_PHONE"); v != "" {
		c.Security.Redaction.ChinesePhone = strings.ToLower(v) == "true"
	}
	if v := os.Getenv("SECURITY_REDACT_CHINESE_ID"); v != "" {
		c.Security.Redaction.ChineseID = strings.ToLower(v) == "true"
	}
}

func mergeSecurityRedactionConfig(base, override SecurityRedactConfig, metadata toml.MetaData) SecurityRedactConfig {
	fields := []struct {
		key string
		set func()
	}{
		{"internal_ip", func() { base.InternalIP = override.InternalIP }},
		{"email", func() { base.Email = override.Email }},
		{"env_var", func() { base.EnvVar = override.EnvVar }},
		{"credit_card", func() { base.CreditCard = override.CreditCard }},
		{"chinese_phone", func() { base.ChinesePhone = override.ChinesePhone }},
		{"chinese_id", func() { base.ChineseID = override.ChineseID }},
		{"chinese_address", func() { base.ChineseAddress = override.ChineseAddress }},
		{"pin", func() { base.PIN = override.PIN }},
	}
	for _, field := range fields {
		if metadata.IsDefined("security", "redaction", field.key) {
			field.set()
		}
	}
	return base
}

func validateSecurityRules(rules SecurityRulesConfig) error {
	for _, pattern := range append(append([]string{}, rules.PatternsS2...), rules.PatternsS3...) {
		if _, err := regexp.Compile(`(?i)` + pattern); err != nil {
			return fmt.Errorf("security.rules pattern %q invalid: %w", pattern, err)
		}
	}
	return nil
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
		c.DisguiseTool = NormalizeDisguiseTool(value)
	case "api_base_url", "base_url":
		c.CustomBaseURL = value
	case "api_coding_url", "coding_url":
		c.CustomCodingURL = value
	case "auth_header":
		c.CustomAuthHeader = value
	case "auth_prefix":
		c.CustomAuthPrefix = value
	case "remove_version_path":
		c.RemoveVersionPath = strings.ToLower(value) == "true"
	case "mock_models":
		c.MockModels = strings.ToLower(value) == "true"
	case "mock_models_resp":
		c.MockModelsResp = value
	case "use_anthropic":
		c.UseAnthropic = strings.ToLower(value) == "true"
	case "security_enabled":
		c.Security.Enabled = strings.ToLower(value) == "true"
	case "security_audit_dir":
		c.Security.AuditDir = value
	case "security_handling_s2":
		c.Security.HandlingS2 = value
	case "security_handling_s3":
		c.Security.HandlingS3 = value
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

	if NormalizeDisguiseTool(c.DisguiseTool) == "claudecode" && c.ClaudeCodeUserAgent != "" {
		return c.ClaudeCodeUserAgent
	}
	if NormalizeDisguiseTool(c.DisguiseTool) == "openclaw" && c.OpenClawUserAgent != "" {
		return c.OpenClawUserAgent
	}
	if NormalizeDisguiseTool(c.DisguiseTool) == "opencode" && c.OpenCodeUserAgent != "" {
		return c.OpenCodeUserAgent
	}

	// 根据伪装工具选择
	if tool, ok := PredefinedDisguiseTools[NormalizeDisguiseTool(c.DisguiseTool)]; ok && tool.UserAgent != "" {
		return tool.UserAgent
	}

	// 默认使用 claudecode
	return PredefinedDisguiseTools["claudecode"].UserAgent
}

// GetDisguiseHeaders 返回伪装工具额外需要补充的请求头。
func (c *Config) GetDisguiseHeaders() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	switch NormalizeDisguiseTool(c.DisguiseTool) {
	case "claudecode":
		sessionID := uuid.New().String()
		return map[string]string{
			"X-App":                    ClaudeCodeAppHeaderValue,
			"X-Claude-Code-Session-Id": sessionID,
		}
	default:
		return nil
	}
}

// GetClientRequestID 生成一个随机的 x-client-request-id
func (c *Config) GetClientRequestID() string {
	return uuid.New().String()
}

// GetBillingHeader 生成 x-anthropic-billing-header 内容
// 模拟真实 Claude Code 客户端在 system prompt 首行注入的计费属性头
// 格式: x-anthropic-billing-header: cc_version=<ver>.<fingerprint>; cc_entrypoint=cli;
func (c *Config) GetBillingHeader() string {
	// 生成 3 位指纹：基于版本号的确定性哈希
	version := ClaudeCodeVersion
	salt := "59cf53e54c78"
	h := sha256.Sum256([]byte(salt + version))
	fingerprint := fmt.Sprintf("%x", h[:2])[:3]
	return fmt.Sprintf("x-anthropic-billing-header: cc_version=%s.%s; cc_entrypoint=cli;", version, fingerprint)
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
		"remove_version_path":    c.RemoveVersionPath,
		"mock_models":            c.MockModels,
		"mock_models_resp":       c.MockModelsResp,
		"use_anthropic":          c.UseAnthropic,
		"security": map[string]interface{}{
			"enabled":         c.Security.Enabled,
			"audit_dir":       c.Security.AuditDir,
			"default_track":   c.Security.DefaultTrack,
			"default_top_k":   c.Security.DefaultTopK,
			"max_audit_items": c.Security.MaxAuditItems,
			"handling_s2":     c.Security.HandlingS2,
			"handling_s3":     c.Security.HandlingS3,
			"placeholder_s3":  c.Security.PlaceholderS3,
			"session_header":  c.Security.SessionHeader,
		},
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
# 删除代理伪装请求的版本控制路径 (默认 false)
# 例如：请求 /v1/models 时，转发时只拼接 /models 部分
remove_version_path = false
# 模拟 /models 响应 (默认 false)
# 启用后，对 /models 或 /v1/models (取决于 remove_version_path) 返回模拟数据
mock_models = false
# 模拟 /models 响应内容 (JSON 字符串)
mock_models_resp = '{"object":"list","data":[{"id":"gpt-4","object":"model","owned_by":"organization"}]}'
# Anthropic 格式兼容模式 (默认 false)
# 启用后会修复请求体中的 schema 字段，将 null 转为正确的默认值
# 适用于使用 Anthropic 原生协议的 API 供应商
use_anthropic = false

# ============================================================================
# 数据安全过滤配置 (EdgeClaw-Mini Go 集成)
# ============================================================================
[security]
# 是否启用本地数据安全过滤；启用后代理和本地安全接口都要求配置 [auth].local_api_key
enabled = false
# 本地双轨审计目录 (留空则使用 data/edgeclaw-mini)
audit_dir = ""
# 默认审计轨道
default_track = "clean"
# 上下文筛选默认 top_k
default_top_k = 5
# 每个 session 轨道最多保留条数 (0 表示不限制)
max_audit_items = 2000
# S2 默认处置策略: allow/redact/review/block
handling_s2 = "redact"
# S3 默认处置策略: allow/redact/review/block
handling_s3 = "block"
# S3/阻断请求在 clean 轨中的占位符
placeholder_s3 = "[PRIVATE]"
# 用于识别对话会话的请求头
session_header = "X-Session-Id"

[security.redaction]
# 以下规则默认关闭，按需开启
internal_ip = false
email = true
env_var = false
credit_card = false
chinese_phone = true
chinese_id = true
chinese_address = false
pin = false
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
