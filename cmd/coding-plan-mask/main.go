// Coding Plan Mask - 本地代理转发工具
// 将请求转发到云厂商 Coding Plan API
package main

import (
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
	version = "0.7.2"
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

	baseURL := fmt.Sprintf("http://%s:%d/v1", cfg.ListenHost, cfg.ListenPort)

	if *jsonOutput {
		output := map[string]string{
			"base_url": baseURL,
			"api_key":  cfg.LocalAPIKey,
		}
		json.NewEncoder(os.Stdout).Encode(output)
	} else {
		fmt.Println()
		fmt.Println("╔════════════════════════════════════════════════════════════╗")
		fmt.Println("║              本地连接信息 (Local Connection)                ║")
		fmt.Println("╠════════════════════════════════════════════════════════════╣")
		fmt.Printf("║  Base URL:  %-45s ║\n", baseURL)
		if cfg.LocalAPIKey != "" {
			fmt.Printf("║  API Key:   %-45s ║\n", cfg.LocalAPIKey)
		} else {
			fmt.Printf("║  API Key:   %-45s ║\n", "(未设置，无需认证)")
		}
		fmt.Println("╚════════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println("客户端配置示例:")
		fmt.Println("```json")
		if cfg.LocalAPIKey != "" {
			fmt.Printf(`{
    "base_url": "%s",
    "api_key": "%s",
    "model": "glm-4-flash"
}`, baseURL, cfg.LocalAPIKey)
		} else {
			fmt.Printf(`{
    "base_url": "%s",
    "model": "glm-4-flash"
}`, baseURL)
		}
		fmt.Println("\n```")
	}
}

// printHelp 打印帮助信息
func printHelp() {
	fmt.Printf(`Coding Plan Mask v%s - 本地代理转发工具

用法:
  %s [选项]           启动代理服务
  %s show             显示本地连接信息
  %s show --json      JSON 格式输出连接信息
  %s stats            显示 Token 使用统计

子命令:
  show, info, connection    显示本地连接地址和 API Key
  stats                      显示 Token 使用统计

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

示例:
  # 启动服务
  %s -api-key sk-xxx -local-api-key sk-local-xxx

  # 显示连接信息
  %s show

  # 显示统计
  %s stats
`, version, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
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
