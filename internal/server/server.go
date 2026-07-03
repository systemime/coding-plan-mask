// Package server 提供 HTTP 服务器
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"coding-plan-mask/internal/config"
	"coding-plan-mask/internal/proxy"
	"coding-plan-mask/internal/security"
	"coding-plan-mask/internal/storage"

	"go.uber.org/zap"
)

// Server HTTP 服务器
type Server struct {
	cfg      *config.Config
	proxy    *proxy.Proxy
	logger   *zap.Logger
	server   *http.Server
	store    *storage.Storage
	security *security.Service
	version  string
}

// New 创建新服务器
func New(cfg *config.Config, logger *zap.Logger, store *storage.Storage, version string) *Server {
	securityDir := cfg.Security.AuditDir
	if strings.TrimSpace(securityDir) == "" {
		securityDir = filepath.Join(store.DataDir(), "edgeclaw-mini")
	}
	securitySvc := security.NewService(security.Settings{
		Enabled:         cfg.Security.Enabled,
		AuditDir:        securityDir,
		DefaultTrack:    cfg.Security.DefaultTrack,
		DefaultTopK:     cfg.Security.DefaultTopK,
		MaxAuditEntries: cfg.Security.MaxAuditItems,
		HandlingS2:      cfg.Security.HandlingS2,
		HandlingS3:      cfg.Security.HandlingS3,
		PlaceholderS3:   cfg.Security.PlaceholderS3,
		SessionHeader:   cfg.Security.SessionHeader,
		RedactionOptions: security.RedactionOptions{
			InternalIP:     cfg.Security.Redaction.InternalIP,
			Email:          cfg.Security.Redaction.Email,
			EnvVar:         cfg.Security.Redaction.EnvVar,
			CreditCard:     cfg.Security.Redaction.CreditCard,
			ChinesePhone:   cfg.Security.Redaction.ChinesePhone,
			ChineseID:      cfg.Security.Redaction.ChineseID,
			ChineseAddress: cfg.Security.Redaction.ChineseAddress,
			PIN:            cfg.Security.Redaction.PIN,
		},
		Rules: securityRulesFromConfig(cfg.Security.Rules),
	}, logger)
	return &Server{
		cfg:      cfg,
		logger:   logger,
		proxy:    proxy.New(cfg, logger, store, securitySvc),
		store:    store,
		security: securitySvc,
		version:  version,
	}
}

func securityRulesFromConfig(cfg config.SecurityRulesConfig) security.PrivacyRules {
	rules := security.DefaultPrivacyRules()
	if len(cfg.KeywordsS2) > 0 {
		rules.KeywordsS2 = cfg.KeywordsS2
	}
	if len(cfg.KeywordsS3) > 0 {
		rules.KeywordsS3 = cfg.KeywordsS3
	}
	if len(cfg.PatternsS2) > 0 {
		rules.PatternsS2 = cfg.PatternsS2
	}
	if len(cfg.PatternsS3) > 0 {
		rules.PatternsS3 = cfg.PatternsS3
	}
	if len(cfg.ToolsS2.Tools) > 0 {
		rules.ToolsS2.Tools = cfg.ToolsS2.Tools
	}
	if len(cfg.ToolsS2.Paths) > 0 {
		rules.ToolsS2.Paths = cfg.ToolsS2.Paths
	}
	if len(cfg.ToolsS3.Tools) > 0 {
		rules.ToolsS3.Tools = cfg.ToolsS3.Tools
	}
	if len(cfg.ToolsS3.Paths) > 0 {
		rules.ToolsS3.Paths = cfg.ToolsS3.Paths
	}
	return rules
}

// SetupRoutes 设置路由
func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// 健康检查
	mux.HandleFunc("/health", s.handleHealth)

	// 就绪检查
	mux.HandleFunc("/ready", s.handleReady)

	// 统计信息
	mux.HandleFunc("/stats", s.handleStats)

	// EdgeClaw-Mini 本地安全接口
	mux.HandleFunc("/redact", s.handleSecurity)
	mux.HandleFunc("/privacy/", s.handleSecurity)
	mux.HandleFunc("/context/", s.handleSecurity)
	mux.HandleFunc("/sessions/", s.handleSecurity)

	// 其余路径全部透传到上游，仅根路径保留本地信息
	mux.HandleFunc("/", s.handleProxy)

	// 带日志的中间件
	handler := s.loggingMiddleware(mux)

	// 安全头中间件
	handler = s.securityMiddleware(handler)

	// panic 兜底，避免单个请求打垮进程
	handler = s.recoverMiddleware(handler)

	return handler
}

// handleRoot 根路径处理器
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	provider, err := s.cfg.GetProviderConfig()
	if err != nil {
		provider = &config.ProviderConfig{Name: "未知"}
	}

	// 获取统计信息
	stats, err := s.store.GetStats()
	if err != nil || stats == nil {
		stats = &storage.Stats{}
	}

	resp := map[string]interface{}{
		"service":       "Coding Plan Proxy",
		"version":       s.version,
		"provider":      provider.Name,
		"status":        "running",
		"models":        provider.Models,
		"request_count": stats.TotalRequests,
		"total_tokens":  stats.TotalTokens,
		"input_tokens":  stats.TotalInputTokens,
		"output_tokens": stats.TotalOutputTokens,
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleProxy 代理所有非保留路径请求
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" && r.Method == http.MethodGet {
		s.handleRoot(w, r)
		return
	}

	s.proxy.Forward(w, r)
}

// handleHealth 健康检查处理器
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// handleReady 就绪检查处理器
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// 检查配置是否完整
	if s.cfg.APIKey == "" {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"ready":  false,
			"reason": "API Key 未配置",
		})
		return
	}
	if s.cfg.Security.Enabled && strings.TrimSpace(s.cfg.LocalAPIKey) == "" {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"ready":  false,
			"reason": "启用安全过滤时必须配置本地 API Key",
		})
		return
	}

	resp := map[string]interface{}{
		"ready": true,
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// handleStats 统计信息处理器
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetStats()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "获取统计信息失败")
		return
	}

	s.writeJSON(w, http.StatusOK, stats)
}

// loggingMiddleware 日志中间件
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 包装 ResponseWriter 以捕获状态码
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		// 只记录本地管理端点的日志，代理请求由 proxy 模块记录详细日志
		if isLocalEndpoint(r.URL.Path) {
			s.logger.Info("HTTP 请求",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", wrapped.statusCode),
				zap.Duration("duration", duration),
				zap.String("remote", r.RemoteAddr),
			)
		}
	})
}

// isLocalEndpoint 判断是否是本地管理端点
func isLocalEndpoint(path string) bool {
	return path == "/" || path == "/health" || path == "/ready" || path == "/stats" ||
		path == "/redact" || strings.HasPrefix(path, "/privacy/") || strings.HasPrefix(path, "/context/") || strings.HasPrefix(path, "/sessions/")
}

// securityMiddleware 安全头中间件
func (s *Server) securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 安全头
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// CORS 头（可选）
		w.Header().Set("Access-Control-Allow-Origin", "*")

		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("HTTP 请求 panic",
					zap.Any("panic", recovered),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
				)
				s.writeError(w, http.StatusInternalServerError, "内部服务错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// responseWriter 包装 http.ResponseWriter 以捕获状态码
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush 实现 http.Flusher 接口，支持流式响应
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// writeJSON 写入 JSON 响应
func (s *Server) writeJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

// writeError 写入错误响应
func (s *Server) writeError(w http.ResponseWriter, code int, message string) {
	s.writeJSON(w, code, map[string]interface{}{
		"error": map[string]string{
			"message": message,
			"code":    fmt.Sprintf("%d", code),
		},
	})
}

func (s *Server) handleSecurity(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Security.Enabled && strings.TrimSpace(s.cfg.LocalAPIKey) == "" {
		s.writeError(w, http.StatusServiceUnavailable, "启用安全过滤时必须配置本地 API Key")
		return
	}
	if !s.validateLocalAPIKey(r) {
		s.writeError(w, http.StatusUnauthorized, "API Key 无效")
		return
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/redact":
		payload, ok := s.readJSONPayload(w, r)
		if !ok {
			return
		}
		text, ok := payload["text"].(string)
		if !ok {
			s.writeError(w, http.StatusBadRequest, "missing field: text")
			return
		}
		resp, err := s.security.RedactText(text, parseRedactionOptions(payload["options"]))
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, resp)
	case r.Method == http.MethodPost && r.URL.Path == "/context/redact":
		payload, ok := s.readJSONPayload(w, r)
		if !ok {
			return
		}
		resp, err := s.security.RedactContext(payload)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, resp)
	case r.Method == http.MethodPost && r.URL.Path == "/context/restore":
		payload, ok := s.readJSONPayload(w, r)
		if !ok {
			return
		}
		resp, err := s.security.RestoreContext(payload)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, resp)
	case r.Method == http.MethodPost && r.URL.Path == "/privacy/detect":
		payload, ok := s.readJSONPayload(w, r)
		if !ok {
			return
		}
		resp, err := s.security.DetectPrivacy(payload)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, resp)
	case r.Method == http.MethodPost && r.URL.Path == "/privacy/policy":
		payload, ok := s.readJSONPayload(w, r)
		if !ok {
			return
		}
		resp, err := s.security.EvaluatePrivacyPolicy(payload)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, resp)
	case strings.HasPrefix(r.URL.Path, "/sessions/"):
		s.handleSecuritySessions(w, r)
	default:
		s.writeError(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) handleSecuritySessions(w http.ResponseWriter, r *http.Request) {
	sessionID, suffix := parseSessionPath(r.URL.Path)
	if sessionID == "" {
		s.writeError(w, http.StatusNotFound, "not found")
		return
	}

	switch {
	case r.Method == http.MethodGet && suffix == "":
		track := r.URL.Query().Get("track")
		limit := 0
		if raw := r.URL.Query().Get("limit"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil {
				s.writeError(w, http.StatusBadRequest, "invalid limit")
				return
			}
			limit = value
		}
		resp, err := s.security.LoadSession(sessionID, track, limit)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, resp)
	case r.Method == http.MethodPost && suffix == "/messages":
		payload, ok := s.readJSONPayload(w, r)
		if !ok {
			return
		}
		resp, err := s.security.WriteMessage(sessionID, payload)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, resp)
	case r.Method == http.MethodPost && suffix == "/context/select":
		payload, ok := s.readJSONPayload(w, r)
		if !ok {
			return
		}
		resp, err := s.security.SelectSessionContext(sessionID, payload)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, resp)
	default:
		s.writeError(w, http.StatusNotFound, "not found")
	}
}

func parseSessionPath(path string) (string, string) {
	if !strings.HasPrefix(path, "/sessions/") {
		return "", ""
	}
	trimmed := strings.TrimPrefix(path, "/sessions/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", ""
	}
	parts := strings.Split(trimmed, "/")
	sessionID := parts[0]
	if len(parts) == 1 {
		return sessionID, ""
	}
	return sessionID, "/" + strings.Join(parts[1:], "/")
}

func (s *Server) readJSONPayload(w http.ResponseWriter, r *http.Request) (map[string]interface{}, bool) {
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxRequestBodySize))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "读取请求体失败")
		return nil, false
	}
	if len(body) == 0 {
		return map[string]interface{}{}, true
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "请求体必须是 JSON object")
		return nil, false
	}
	return payload, true
}

func (s *Server) validateLocalAPIKey(r *http.Request) bool {
	if strings.TrimSpace(s.cfg.LocalAPIKey) == "" {
		return true
	}
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}
	clientKey := strings.TrimPrefix(authHeader, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(clientKey), []byte(s.cfg.LocalAPIKey)) == 1
}

func parseRedactionOptions(raw interface{}) *security.RedactionOptions {
	value, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	options := security.RedactionOptions{}
	if item, ok := value["internal_ip"].(bool); ok {
		options.InternalIP = item
	}
	if item, ok := value["email"].(bool); ok {
		options.Email = item
	}
	if item, ok := value["env_var"].(bool); ok {
		options.EnvVar = item
	}
	if item, ok := value["credit_card"].(bool); ok {
		options.CreditCard = item
	}
	if item, ok := value["chinese_phone"].(bool); ok {
		options.ChinesePhone = item
	}
	if item, ok := value["chinese_id"].(bool); ok {
		options.ChineseID = item
	}
	if item, ok := value["chinese_address"].(bool); ok {
		options.ChineseAddress = item
	}
	if item, ok := value["pin"].(bool); ok {
		options.PIN = item
	}
	return &options
}

// Start 启动服务器
func (s *Server) Start() error {
	handler := s.SetupRoutes()

	addr := fmt.Sprintf("%s:%d", s.cfg.ListenHost, s.cfg.ListenPort)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: time.Duration(s.cfg.Timeout) * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.logger.Info("服务器启动",
		zap.String("address", addr),
		zap.String("provider", s.cfg.Provider),
	)

	// 启动 goroutine 处理信号
	go s.handleShutdown()

	// 启动服务器
	err := s.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

// handleShutdown 处理优雅关闭
func (s *Server) handleShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan

	s.logger.Info("收到关闭信号，开始优雅关闭...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("服务器关闭错误", zap.Error(err))
	}

	if err := s.proxy.Close(); err != nil {
		s.logger.Error("代理关闭错误", zap.Error(err))
	}

	if err := s.store.Close(); err != nil {
		s.logger.Error("存储关闭错误", zap.Error(err))
	}

	s.logger.Info("服务器已关闭")
}

// Stop 停止服务器
func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}
