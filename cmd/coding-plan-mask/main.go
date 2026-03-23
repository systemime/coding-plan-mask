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

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	version = "0.8.0"
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
  %s history          查看转发历史记录

子命令:
  show, info, connection    显示本地连接地址和 API Key
  stats                      显示 Token 使用统计
  history                    交互式查看转发历史记录

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

  # 查看转发历史
  %s history
`, version, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
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

// ========== History TUI ==========

// history keybindings
type historyKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Esc      key.Binding
	Quit     key.Binding
	Help     key.Binding
}

var historyKeys = historyKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "上移"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "下移"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "查看详情"),
	),
	Esc: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "返回列表"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "退出"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "帮助"),
	),
}

func (k historyKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Esc, k.Quit}
}

func (k historyKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Enter, k.Esc},
		{k.Quit, k.Help},
	}
}

// historyModel TUI model
type historyModel struct {
	records        []storage.RequestRecordLite
	selected       int
	store          *storage.Storage
	viewMode       bool          // 是否在详情查看模式
	detailRecord   *storage.RequestRecord
	viewport       viewport.Model
	help           help.Model
	showHelp       bool
	ready          bool
	width, height  int
	err            error
}

func newHistoryModel(records []storage.RequestRecordLite, store *storage.Storage) historyModel {
	return historyModel{
		records:  records,
		store:    store,
		help:     help.New(),
		showHelp: false,
	}
}

func (m historyModel) Init() tea.Cmd {
	return nil
}

func (m historyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.ready = true
		if m.viewMode {
			m.viewport.Width = msg.Width - 4
			m.viewport.Height = msg.Height - 6
		}
		return m, nil

	case tea.KeyMsg:
		if m.viewMode {
			// 详情模式
			switch {
			case key.Matches(msg, historyKeys.Esc):
				m.viewMode = false
				m.detailRecord = nil
				return m, nil
			case key.Matches(msg, historyKeys.Quit):
				return m, tea.Quit
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		// 列表模式
		switch {
		case key.Matches(msg, historyKeys.Quit):
			return m, tea.Quit
		case key.Matches(msg, historyKeys.Up):
			if m.selected > 0 {
				m.selected--
			}
		case key.Matches(msg, historyKeys.Down):
			if m.selected < len(m.records)-1 {
				m.selected++
			}
		case key.Matches(msg, historyKeys.Enter):
			if len(m.records) > 0 && m.selected < len(m.records) {
				record := m.records[m.selected]
				detail, err := m.store.GetRequestDetail(record.ID)
				if err != nil {
					m.err = err
					return m, nil
				}
				m.detailRecord = detail
				m.viewMode = true
				content := formatDetailContent(detail)
				m.viewport = viewport.New(m.width-4, m.height-6)
				m.viewport.SetContent(content)
			}
		case key.Matches(msg, historyKeys.Help):
			m.showHelp = !m.showHelp
		}
	}

	return m, tea.Batch(cmds...)
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62"))

	normalStyle = lipgloss.NewStyle()

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86"))

	detailStyle = lipgloss.NewStyle().
			Padding(0, 1)
)

func (m historyModel) View() string {
	if !m.ready {
		return "加载中..."
	}

	if m.err != nil {
		return fmt.Sprintf("错误: %v", m.err)
	}

	if m.viewMode && m.detailRecord != nil {
		// 详情模式
		title := titleStyle.Render(fmt.Sprintf(" 请求详情 #%d ", m.detailRecord.ID))
		helpBar := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("按 Esc 返回列表 | q 退出")
		return fmt.Sprintf("%s\n%s\n\n%s", title, m.viewport.View(), helpBar)
	}

	// 列表模式
	var b strings.Builder

	// 标题
	title := titleStyle.Render(" 转发历史记录 ")
	b.WriteString(title + "\n\n")

	// 表头
	header := fmt.Sprintf("  %-6s %-20s %-18s %-12s %-30s %-10s",
		"ID", "时间", "模型", "供应商", "路径", "状态")
	b.WriteString(headerStyle.Render(header) + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Repeat("─", min(m.width, 120))) + "\n")

	// 计算可见区域
	visibleStart := max(0, m.selected-10)
	visibleEnd := min(len(m.records), visibleStart+20)

	for i := visibleStart; i < visibleEnd; i++ {
		r := m.records[i]
		timeStr := r.Timestamp.Format("2006-01-02 15:04:05")
		model := truncate(r.Model, 16)
		provider := truncate(r.Provider, 10)
		path := truncate(r.Path, 28)

		line := fmt.Sprintf("  %-6d %-20s %-18s %-12s %-30s %-10d",
			r.ID, timeStr, model, provider, path, r.StatusCode)

		if i == m.selected {
			b.WriteString(selectedStyle.Render(line) + "\n")
		} else {
			b.WriteString(normalStyle.Render(line) + "\n")
		}
	}

	// 底部状态
	if len(m.records) == 0 {
		b.WriteString("\n  暂无历史记录\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Repeat("─", min(m.width, 120))) + "\n")
		status := fmt.Sprintf(" %d/%d 条记录 | ↑/↓ 移动 | Enter 查看详情 | q 退出 ",
			m.selected+1, len(m.records))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(status))
	}

	return b.String()
}

func formatDetailContent(r *storage.RequestRecord) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Render("═══════════════════════════════════════════════════════════════") + "\n\n")

	// 基本信息
	b.WriteString(fmt.Sprintf("  ID:          %d\n", r.ID))
	b.WriteString(fmt.Sprintf("  时间:        %s\n", r.Timestamp.Format("2006-01-02 15:04:05.000")))
	b.WriteString(fmt.Sprintf("  供应商:      %s\n", r.Provider))
	b.WriteString(fmt.Sprintf("  模型:        %s\n", r.Model))
	b.WriteString(fmt.Sprintf("  方法:        %s\n", r.Method))
	b.WriteString(fmt.Sprintf("  路径:        %s\n", r.Path))
	b.WriteString(fmt.Sprintf("  客户端IP:    %s\n", r.ClientIP))
	b.WriteString(fmt.Sprintf("  状态码:      %d\n", r.StatusCode))
	b.WriteString(fmt.Sprintf("  耗时:        %.2f ms\n", r.Duration))
	b.WriteString(fmt.Sprintf("  输入Token:   %d\n", r.InputTokens))
	b.WriteString(fmt.Sprintf("  输出Token:   %d\n", r.OutputTokens))
	b.WriteString(fmt.Sprintf("  总Token:     %d\n", r.TotalTokens))
	b.WriteString(fmt.Sprintf("  流式:        %v\n", r.Stream))
	b.WriteString(fmt.Sprintf("  成功:        %v\n", r.Success))
	if r.ErrorMsg != "" {
		b.WriteString(fmt.Sprintf("  错误信息:    %s\n", r.ErrorMsg))
	}

	b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).Render("── 请求Body ──") + "\n\n")
	if r.RequestBody != "" {
		b.WriteString(indentJSON(r.RequestBody))
	} else {
		b.WriteString("  (空)\n")
	}

	b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).Render("── 响应Body ──") + "\n\n")
	if r.ResponseBody != "" {
		b.WriteString(indentJSON(r.ResponseBody))
	} else {
		b.WriteString("  (空)\n")
	}

	b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render("═══════════════════════════════════════════════════════════════") + "\n")

	return b.String()
}

func indentJSON(s string) string {
	var buf strings.Builder
	indent := 0
	inString := false
	escaped := false

	for _, c := range s {
		if escaped {
			buf.WriteRune(c)
			escaped = false
			continue
		}

		switch c {
		case '\\':
			buf.WriteRune(c)
			escaped = true
		case '"':
			buf.WriteRune(c)
			inString = !inString
		case '{', '[':
			buf.WriteRune(c)
			if !inString {
				buf.WriteRune('\n')
				indent++
				buf.WriteString(strings.Repeat("  ", indent))
			}
		case '}', ']':
			if !inString {
				buf.WriteRune('\n')
				indent--
				buf.WriteString(strings.Repeat("  ", indent))
			}
			buf.WriteRune(c)
		case ',':
			buf.WriteRune(c)
			if !inString {
				buf.WriteRune('\n')
				buf.WriteString(strings.Repeat("  ", indent))
			}
		case ':':
			buf.WriteRune(c)
			if !inString {
				buf.WriteRune(' ')
			}
		case ' ', '\t', '\n', '\r':
			if inString {
				buf.WriteRune(c)
			}
		default:
			buf.WriteRune(c)
		}
	}

	return "  " + strings.ReplaceAll(buf.String(), "\n", "\n  ")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// showHistory 显示历史记录
func showHistory(args []string) {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	configPath := fs.String("config", "", "配置文件路径")
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

	// 打开存储
	store, err := storage.New(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开存储失败: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// 获取所有记录
	records, err := store.GetAllRequestsLite()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取历史记录失败: %v\n", err)
		os.Exit(1)
	}

	if len(records) == 0 {
		fmt.Println("\n暂无历史记录")
		return
	}

	// 启动TUI
	m := newHistoryModel(records, store)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "启动TUI失败: %v\n", err)
		os.Exit(1)
	}
}
