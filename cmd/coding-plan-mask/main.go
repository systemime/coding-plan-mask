// Coding Plan Mask - 本地代理转发工具
// 将请求转发到云厂商 Coding Plan API
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"coding-plan-mask/internal/config"
	"coding-plan-mask/internal/server"
	"coding-plan-mask/internal/storage"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	version = "0.8.6"
	commit  = "unknown"
	date    = "unknown"
)

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

func main() {
	// 检查子命令
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "show", "info", "connection":
			showConnection(os.Args[2:])
			return
		case "stats":
			showStats(os.Args[2:])
			return
		case "history":
			showHistory(os.Args[2:])
			return
		case "doctor":
			runDoctor(os.Args[2:])
			return
		case "help", "-h", "--help":
			printHelp()
			return
		}
	}

	// 命令行参数
	configPath := flag.String("config", "", "配置文件路径")
	provider := flag.String("provider", "", "服务商名称")
	apiKey := flag.String("api-key", "", "Coding Plan API Key")
	localAPIKey := flag.String("local-api-key", "", "本地 API Key")
	host := flag.String("host", "", "监听地址")
	port := flag.Int("port", 0, "监听端口")
	debug := flag.Bool("debug", false, "调试模式")
	general := flag.Bool("general", false, "使用通用 API 端点")
	showVersion := flag.Bool("version", false, "显示版本信息")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Coding Plan Mask %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	// 加载配置
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// 命令行参数覆盖
	if *provider != "" {
		cfg.Provider = *provider
	}
	if *apiKey != "" {
		cfg.APIKey = *apiKey
	}
	if *localAPIKey != "" {
		cfg.LocalAPIKey = *localAPIKey
	}
	if *host != "" {
		cfg.ListenHost = *host
	}
	if *port != 0 {
		cfg.ListenPort = *port
	}
	if *debug {
		cfg.Debug = true
	}
	if *general {
		cfg.UseCodingEndpoint = false
	}

	// 初始化日志
	logger := initLogger(cfg.Debug)
	defer logger.Sync()

	// 初始化存储 - 数据库在可执行文件所在目录的 data 子目录
	execDir := getExecutableDir()
	dataDir := filepath.Join(execDir, "data")
	store, err := storage.New(dataDir)
	if err != nil {
		logger.Fatal("初始化存储失败", zap.Error(err))
	}

	// 打印启动信息
	printBanner(cfg, logger)

	// 检查必要配置
	if cfg.APIKey == "" {
		logger.Warn("未配置 Coding Plan API Key，请使用 --api-key 参数或配置文件设置")
	}

	if cfg.LocalAPIKey == "" {
		logger.Warn("未配置本地 API Key，代理将允许任意客户端连接（不推荐）")
	}

	// 创建并启动服务器
	srv := server.New(cfg, logger, store, version)
	if err := srv.Start(); err != nil {
		logger.Fatal("服务器启动失败", zap.Error(err))
	}
}

// showStats 显示统计信息
func showStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	configPath := fs.String("config", "", "配置文件路径")
	_ = fs.Parse(args)

	// 确定数据目录 - 默认在可执行文件所在目录
	var dataDir string
	if *configPath != "" {
		// 从配置文件路径推导数据目录
		dataDir = filepath.Join(filepath.Dir(*configPath), "data")
	} else {
		// 使用可执行文件所在目录
		dataDir = filepath.Join(getExecutableDir(), "data")
	}
	dbPath := filepath.Join(dataDir, "proxy.db")

	// 检查数据库是否存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Println("统计数据库不存在，服务可能还未运行过")
		return
	}

	// 打开存储
	store, err := storage.New(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开存储失败: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// 获取统计
	stats, err := store.GetStats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取统计失败: %v\n", err)
		os.Exit(1)
	}

	// 输出
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    Token 使用统计                           ║")
	fmt.Println("╠════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  总请求数:     %-42d ║\n", stats.TotalRequests)
	fmt.Printf("║  总上传 Token: %-42d ║\n", stats.TotalInputTokens)
	fmt.Printf("║  总下载 Token: %-42d ║\n", stats.TotalOutputTokens)
	fmt.Printf("║  总 Token:     %-42d ║\n", stats.TotalTokens)
	fmt.Println("╠════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  今日请求:     %-42d ║\n", stats.TodayRequests)
	fmt.Printf("║  今日上传:     %-42d ║\n", stats.TodayInputTokens)
	fmt.Printf("║  今日下载:     %-42d ║\n", stats.TodayOutputTokens)
	fmt.Printf("║  今日 Token:   %-42d ║\n", stats.TodayTokens)
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// showConnection 显示连接信息
func showConnection(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	configPath := fs.String("config", "", "配置文件路径")
	jsonOutput := fs.Bool("json", false, "JSON 格式输出")
	_ = fs.Parse(args)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	openAIBaseURL := fmt.Sprintf("http://%s:%d/v1", cfg.ListenHost, cfg.ListenPort)
	anthropicBaseURL := fmt.Sprintf("http://%s:%d", cfg.ListenHost, cfg.ListenPort)

	if *jsonOutput {
		output := map[string]interface{}{
			"base_url":           openAIBaseURL,
			"openai_base_url":    openAIBaseURL,
			"anthropic_base_url": anthropicBaseURL,
			"api_key":            cfg.LocalAPIKey,
			"api_key_configured": cfg.LocalAPIKey != "",
		}
		json.NewEncoder(os.Stdout).Encode(output)
	} else {
		fmt.Println()
		fmt.Println("╔════════════════════════════════════════════════════════════╗")
		fmt.Println("║              本地连接信息 (Local Connection)                ║")
		fmt.Println("╠════════════════════════════════════════════════════════════╣")
		fmt.Printf("║  OpenAI URL:    %-41s ║\n", openAIBaseURL)
		fmt.Printf("║  Anthropic URL: %-41s ║\n", anthropicBaseURL)
		if cfg.LocalAPIKey != "" {
			fmt.Printf("║  API Key:       %-41s ║\n", maskSecret(cfg.LocalAPIKey))
		} else {
			fmt.Printf("║  API Key:       %-41s ║\n", "(未设置，无需认证)")
		}
		fmt.Println("╚════════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println("OpenAI-compatible 客户端示例:")
		fmt.Println("```json")
		if cfg.LocalAPIKey != "" {
			fmt.Printf(`{
    "base_url": "%s",
    "api_key": "<local_api_key>",
    "model": "glm-4-flash"
}`, openAIBaseURL)
		} else {
			fmt.Printf(`{
    "base_url": "%s",
    "model": "glm-4-flash"
}`, openAIBaseURL)
		}
		fmt.Println("\n```")
		fmt.Println()
		fmt.Println("Claude/Anthropic-compatible 客户端：")
		fmt.Printf("ANTHROPIC_BASE_URL=%s\n", anthropicBaseURL)
		if cfg.LocalAPIKey != "" {
			fmt.Println("ANTHROPIC_AUTH_TOKEN=<local_api_key>")
			fmt.Println("实际 Key 可用 `mask-ctl show --json` 给脚本读取。")
		}
	}
}

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func runDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	configPath := fs.String("config", "", "配置文件路径")
	jsonOutput := fs.Bool("json", false, "JSON 格式输出")
	_ = fs.Parse(args)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	checks := collectDoctorChecks(cfg)
	if *jsonOutput {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]interface{}{
			"ok":     !hasDoctorError(checks),
			"checks": checks,
		})
	} else {
		fmt.Println("Coding Plan Mask 配置检查")
		for _, check := range checks {
			fmt.Printf("[%s] %s: %s\n", strings.ToUpper(check.Status), check.Name, check.Message)
		}
	}
	if hasDoctorError(checks) {
		os.Exit(1)
	}
}

func collectDoctorChecks(cfg *config.Config) []doctorCheck {
	var checks []doctorCheck
	add := func(status, name, message string) {
		checks = append(checks, doctorCheck{Name: name, Status: status, Message: message})
	}

	provider, err := cfg.GetProviderConfig()
	if err != nil {
		add("error", "provider", err.Error())
	} else {
		add("ok", "provider", fmt.Sprintf("%s (%s)", cfg.Provider, provider.Name))
		targetURL := provider.CodingBaseURL
		if !cfg.UseCodingEndpoint {
			targetURL = provider.GeneralBaseURL
		}
		if strings.TrimSpace(targetURL) == "" {
			add("error", "upstream_url", "上游 URL 为空；custom provider 需要配置 [api].base_url 或 coding_url")
		} else {
			add("ok", "upstream_url", targetURL)
		}
		if strings.TrimSpace(provider.AuthHeader) == "" {
			add("error", "auth_header", "上游认证头为空")
		} else {
			add("ok", "auth_header", provider.AuthHeader)
		}
	}

	if strings.TrimSpace(cfg.APIKey) == "" {
		add("error", "api_key", "未配置 Coding Plan API Key")
	} else {
		add("ok", "api_key", "已配置")
	}
	if strings.TrimSpace(cfg.LocalAPIKey) == "" {
		if cfg.Security.Enabled {
			add("error", "local_api_key", "启用隐私过滤时必须配置本地 API Key")
		} else {
			add("warn", "local_api_key", "未配置；本地代理将允许任意客户端连接")
		}
	} else {
		add("ok", "local_api_key", "已配置")
	}

	add("ok", "listen", fmt.Sprintf("http://%s:%d", cfg.ListenHost, cfg.ListenPort))
	add("ok", "openai_base_url", fmt.Sprintf("http://%s:%d/v1", cfg.ListenHost, cfg.ListenPort))
	add("ok", "anthropic_base_url", fmt.Sprintf("http://%s:%d", cfg.ListenHost, cfg.ListenPort))
	if cfg.UseAnthropic {
		add("ok", "anthropic_bridge", "/v1/messages 会转换为 OpenAI Chat Completions")
	} else {
		add("info", "anthropic_bridge", "未启用；如需 Claude 风格客户端请设置 use_anthropic=true")
	}
	if cfg.Security.Enabled {
		add("ok", "privacy", "已启用本地隐私过滤")
	} else {
		add("info", "privacy", "未启用本地隐私过滤")
	}
	return checks
}

func hasDoctorError(checks []doctorCheck) bool {
	for _, check := range checks {
		if check.Status == "error" {
			return true
		}
	}
	return false
}

func maskSecret(value string) string {
	if len(value) <= 8 {
		return "****"
	}
	return value[:4] + "****" + value[len(value)-4:]
}

// printHelp 打印帮助信息
func printHelp() {
	fmt.Printf(`Coding Plan Mask v%s - 本地代理转发工具

用法:
  %s [选项]           启动代理服务
  %s show             显示本地连接信息
  %s show --json      JSON 格式输出连接信息
  %s stats            显示 Token 使用统计
  %s history          查看转发历史记录
  %s doctor           检查本地配置

子命令:
  show, info, connection    显示本地连接地址和 API Key
  stats                      显示 Token 使用统计
  history                    查看转发历史记录
  doctor                     检查本地配置

选项:
  -config string         配置文件路径
  -provider string       服务商 (zhipu, zhipu_v2, aliyun, minimax, deepseek, moonshot)
  -api-key string        Coding Plan API Key
  -local-api-key string  本地 API Key
  -host string           监听地址 (默认 127.0.0.1)
  -port int              监听端口 (默认 8787)
  -debug                 调试模式
  -general               使用通用 API 端点
  -version               显示版本信息

伪装工具配置 (在 config.toml 中设置):
  disguise_tool = "claudecode"  伪装为 Claude Code 风格请求
  claude_code_user_agent = "claude-cli/2.1.76 (external, cli)"
  disguise_tool = "kimicode"    Kimi Code API 订阅认证格式
  disguise_tool = "opencode"    兼容旧版 OpenCode 标识
  opencode_user_agent = "opencode/1.2.27 ai-sdk/provider-utils/3.0.20 runtime/bun/1.3.10"
  disguise_tool = "openclaw"    伪装为 OpenClaw
  openclaw_user_agent = "OpenClaw-Gateway/1.0"
  disguise_tool = "custom"      使用自定义 User-Agent

User-Agent 来源说明:
  claudecode: claude-cli/<version> (external, cli) - 可通过 claude_code_user_agent 覆盖
  kimicode:   claude-code/0.1.0 - Kimi Code API 订阅认证要求
  opencode:   opencode/<version> ai-sdk/... runtime/bun/... - 可通过 opencode_user_agent 覆盖
  openclaw:   OpenClaw-Gateway/1.0 - OpenClaw 兼容默认值，可通过 openclaw_user_agent 覆盖

常用环境变量:
  API_KEY, LOCAL_API_KEY, PROVIDER, HOST, PORT, DEBUG
  USE_ANTHROPIC, SECURITY_ENABLED, SECURITY_AUDIT_DIR
  DISGUISE_TOOL, CLAUDE_CODE_USER_AGENT, OPENCODE_USER_AGENT, OPENCLAW_USER_AGENT

示例:
  # 启动服务
  %s -api-key sk-xxx -local-api-key sk-local-xxx

  # 显示连接信息
  %s show

  # 显示统计
  %s stats

  # 查看转发历史
  %s history

  # 检查配置
  %s doctor
`, version, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

// initLogger 初始化日志
func initLogger(debug bool) *zap.Logger {
	var zcfg zap.Config
	if debug {
		zcfg = zap.NewDevelopmentConfig()
		zcfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		zcfg = zap.NewProductionConfig()
		zcfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
		zcfg.EncoderConfig.TimeKey = "time"
		zcfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	logger, err := zcfg.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}

	return logger
}

// printBanner 打印启动横幅
func printBanner(cfg *config.Config, logger *zap.Logger) {
	provider, err := cfg.GetProviderConfig()
	providerName := "未知"
	if err == nil {
		providerName = provider.Name
	}

	endpointType := "Coding Plan"
	if !cfg.UseCodingEndpoint {
		endpointType = "通用 API"
	}

	localAuth := "已配置"
	if cfg.LocalAPIKey == "" {
		localAuth = "未配置 (公开模式)"
	}

	apiKeyStatus := "已配置"
	if cfg.APIKey == "" {
		apiKeyStatus = "未配置"
	}

	debugMode := "关闭"
	if cfg.Debug {
		debugMode = "开启"
	}

	// 获取伪装工具信息
	disguiseTool := cfg.DisguiseTool
	if disguiseTool == "" {
		disguiseTool = "claudecode"
	}
	toolInfo, ok := config.PredefinedDisguiseTools[disguiseTool]
	toolName := "未知"
	if ok {
		toolName = toolInfo.Name
	}
	userAgent := cfg.GetEffectiveUserAgent()

	banner := fmt.Sprintf(`
╔══════════════════════════════════════════════════════════════╗
║                Coding Plan Mask v%s                       ║
╠══════════════════════════════════════════════════════════════╣
║  服务商: %-50s ║
║  端点类型: %-48s ║
║  监听地址: http://%s:%-39d ║
║  本地认证: %-48s ║
║  Coding Key: %-46s ║
║  伪装工具: %-48s ║
║  User-Agent: %-46s ║
║  调试模式: %-48s ║
╚══════════════════════════════════════════════════════════════╝
`, version, padRight(providerName, 50), padRight(endpointType, 48),
		cfg.ListenHost, cfg.ListenPort,
		padRight(localAuth, 48), padRight(apiKeyStatus, 46),
		padRight(toolName, 48), padRight(userAgent, 46), padRight(debugMode, 48))

	fmt.Print(banner)

	logger.Info("服务启动",
		zap.String("provider", cfg.Provider),
		zap.String("listen", fmt.Sprintf("%s:%d", cfg.ListenHost, cfg.ListenPort)),
		zap.String("disguise", disguiseTool),
	)
}

// padRight 右侧填充
func padRight(s string, length int) string {
	if len(s) >= length {
		return s[:length]
	}
	return s + strings.Repeat(" ", length-len(s))
}

func formatDetailContent(r *storage.RequestRecord) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("ID:          %d\n", r.ID))
	b.WriteString(fmt.Sprintf("时间:        %s\n", r.Timestamp.Format("2006-01-02 15:04:05.000")))
	b.WriteString(fmt.Sprintf("供应商:      %s\n", r.Provider))
	b.WriteString(fmt.Sprintf("模型:        %s\n", r.Model))
	b.WriteString(fmt.Sprintf("方法:        %s\n", r.Method))
	b.WriteString(fmt.Sprintf("路径:        %s\n", r.Path))
	b.WriteString(fmt.Sprintf("客户端IP:    %s\n", r.ClientIP))
	b.WriteString(fmt.Sprintf("状态码:      %d\n", r.StatusCode))
	b.WriteString(fmt.Sprintf("耗时:        %.2f ms\n", r.Duration))
	b.WriteString(fmt.Sprintf("输入Token:   %d\n", r.InputTokens))
	b.WriteString(fmt.Sprintf("输出Token:   %d\n", r.OutputTokens))
	b.WriteString(fmt.Sprintf("总Token:     %d\n", r.TotalTokens))
	b.WriteString(fmt.Sprintf("流式:        %v\n", r.Stream))
	b.WriteString(fmt.Sprintf("成功:        %v\n", r.Success))
	if r.ErrorMsg != "" {
		b.WriteString(fmt.Sprintf("错误信息:    %s\n", r.ErrorMsg))
	}
	b.WriteString("\n── 请求Body ──\n")
	if r.RequestBody != "" {
		b.WriteString(indentJSON(r.RequestBody))
	} else {
		b.WriteString("(空)\n")
	}
	b.WriteString("\n── 响应Body ──\n")
	if r.ResponseBody != "" {
		b.WriteString(indentJSON(r.ResponseBody))
	} else {
		b.WriteString("(空)\n")
	}
	return b.String()
}

func indentJSON(s string) string {
	var out bytes.Buffer
	if err := json.Indent(&out, []byte(s), "", "  "); err != nil {
		return s + "\n"
	}
	return out.String() + "\n"
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// showHistory 显示历史记录
func showHistory(args []string) {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	configPath := fs.String("config", "", "配置文件路径")
	limit := fs.Int("limit", 20, "显示最近 N 条记录")
	detailID := fs.Int64("id", 0, "显示指定请求详情")
	_ = fs.Parse(args)

	// 确定数据目录
	var dataDir string
	if *configPath != "" {
		dataDir = filepath.Join(filepath.Dir(*configPath), "data")
	} else {
		dataDir = filepath.Join(getExecutableDir(), "data")
	}
	dbPath := filepath.Join(dataDir, "proxy.db")

	// 检查数据库是否存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Println("历史数据库不存在，服务可能还未运行过")
		return
	}

	store, err := storage.New(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开存储失败: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	if *detailID > 0 {
		detail, err := store.GetRequestDetail(*detailID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取历史详情失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(formatDetailContent(detail))
		return
	}

	records, err := store.GetAllRequestsLite()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取历史记录失败: %v\n", err)
		os.Exit(1)
	}
	if len(records) == 0 {
		fmt.Println("暂无历史记录")
		return
	}
	if *limit <= 0 || *limit > len(records) {
		*limit = len(records)
	}

	fmt.Printf("%-6s %-19s %-16s %-10s %-28s %-6s %-8s\n", "ID", "时间", "模型", "供应商", "路径", "状态", "Tokens")
	for _, r := range records[:*limit] {
		fmt.Printf("%-6d %-19s %-16s %-10s %-28s %-6d %-8d\n",
			r.ID,
			r.Timestamp.Format("2006-01-02 15:04:05"),
			truncate(r.Model, 16),
			truncate(r.Provider, 10),
			truncate(r.Path, 28),
			r.StatusCode,
			r.TotalTokens,
		)
	}
	fmt.Println("\n详情: mask-ctl history -id <ID>")
}
