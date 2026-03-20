// Package server 提供 HTTP 服务器
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"coding-plan-mask/internal/config"
	"coding-plan-mask/internal/proxy"
	"coding-plan-mask/internal/storage"

	"go.uber.org/zap"
)

// Server HTTP 服务器
type Server struct {
	cfg     *config.Config
	proxy   *proxy.Proxy
	logger  *zap.Logger
	server  *http.Server
	store   *storage.Storage
	version string
}

// New 创建新服务器
func New(cfg *config.Config, logger *zap.Logger, store *storage.Storage, version string) *Server {
	return &Server{
		cfg:     cfg,
		logger:  logger,
		proxy:   proxy.New(cfg, logger, store),
		store:   store,
		version: version,
	}
}

// SetupRoutes 设置路由
func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// 根路径 - 服务信息
	mux.HandleFunc("/", s.handleRoot)

	// 模型列表
	mux.HandleFunc("/v1/models", s.handleModels)

	// 聊天补全
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)

	// 向量嵌入
	mux.HandleFunc("/v1/embeddings", s.handleEmbeddings)

	// 健康检查
	mux.HandleFunc("/health", s.handleHealth)

	// 就绪检查
	mux.HandleFunc("/ready", s.handleReady)

	// 统计信息
	mux.HandleFunc("/stats", s.handleStats)

	// 带日志的中间件
	handler := s.loggingMiddleware(mux)

	// 安全头中间件
	handler = s.securityMiddleware(handler)

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

// handleModels 模型列表处理器
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}

	provider, err := s.cfg.GetProviderConfig()
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	models := make([]map[string]interface{}, len(provider.Models))
	for i, modelID := range provider.Models {
		models[i] = map[string]interface{}{
			"id":       modelID,
			"object":   "model",
			"created":  1700000000,
			"owned_by": provider.Name,
		}
	}

	resp := map[string]interface{}{
		"object": "list",
		"data":   models,
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleChatCompletions 聊天补全处理器
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	// 支持 OPTIONS 预检请求
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Provider")
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}

	s.proxy.ChatCompletions(w, r)
}

// handleEmbeddings 向量嵌入处理器
func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}

	s.proxy.Embeddings(w, r)
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

		s.logger.Info("HTTP 请求",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", wrapped.statusCode),
			zap.Duration("duration", duration),
			zap.String("remote", r.RemoteAddr),
		)
	})
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

// responseWriter 包装 http.ResponseWriter 以捕获状态码
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
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
