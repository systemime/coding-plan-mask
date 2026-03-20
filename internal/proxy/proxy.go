// Package proxy 提供 API 代理转发功能
package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"coding-plan-mask/internal/config"
	"coding-plan-mask/internal/ratelimit"
	"coding-plan-mask/internal/storage"

	"go.uber.org/zap"
)

// Proxy API 代理
type Proxy struct {
	cfg       *config.Config
	rateLimit *ratelimit.GlobalLimiter
	client    *http.Client
	logger    *zap.Logger
	storage   *storage.Storage
}

// New 创建新的代理实例
func New(cfg *config.Config, logger *zap.Logger, store *storage.Storage) *Proxy {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(cfg.Timeout) * time.Second,
	}

	rateLimiter := ratelimit.NewGlobalLimiter(cfg.RateLimitRequests, 5*time.Minute)

	return &Proxy{
		cfg:       cfg,
		rateLimit: rateLimiter,
		client:    client,
		logger:    logger,
		storage:   store,
	}
}

// Close 关闭代理
func (p *Proxy) Close() error {
	p.client.CloseIdleConnections()
	return nil
}

// ChatCompletions 聊天补全代理
func (p *Proxy) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	clientIP := getClientIP(r)

	// 速率限制检查
	if !p.rateLimit.Allow() {
		p.writeError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}

	// 读取请求体
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, p.cfg.MaxRequestBodySize))
	if err != nil {
		p.writeError(w, http.StatusBadRequest, "读取请求体失败")
		return
	}
	defer r.Body.Close()

	// 解析请求以获取模型信息
	var reqBody map[string]interface{}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		p.writeError(w, http.StatusBadRequest, "无效的请求体")
		return
	}

	model, _ := reqBody["model"].(string)
	stream, _ := reqBody["stream"].(bool)

	// 验证本地 API Key
	if !p.validateLocalAPIKey(r) {
		p.writeError(w, http.StatusUnauthorized, "API Key 无效")
		return
	}

	// 获取 Coding Plan API Key
	codingAPIKey := p.cfg.APIKey
	if codingAPIKey == "" {
		p.writeError(w, http.StatusInternalServerError, "服务未配置 API Key")
		return
	}

	// 获取服务商配置
	provider, err := p.cfg.GetProviderConfig()
	if err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 构建目标 URL
	baseURL := provider.CodingBaseURL
	if !p.cfg.UseCodingEndpoint {
		baseURL = provider.GeneralBaseURL
	}
	targetURL := fmt.Sprintf("%s/chat/completions", baseURL)

	// 构建请求头
	headers := p.buildHeaders(provider, codingAPIKey, stream)

	// 日志记录
	p.logger.Info("处理请求",
		zap.String("model", model),
		zap.Bool("stream", stream),
		zap.String("provider", p.cfg.Provider),
	)

	// 创建上游请求
	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		p.writeError(w, http.StatusInternalServerError, "创建请求失败")
		return
	}

	// 设置请求头
	for k, v := range headers {
		upstreamReq.Header.Set(k, v)
	}

	// 发送请求
	resp, err := p.client.Do(upstreamReq)
	if err != nil {
		if strings.Contains(err.Error(), "context canceled") {
			p.logger.Info("请求被取消", zap.String("model", model))
			return
		}
		p.logger.Error("上游请求失败", zap.Error(err))
		p.writeError(w, http.StatusBadGateway, "上游服务不可用")
		return
	}
	defer resp.Body.Close()

	// 计算输入 token
	inputTokens := estimateInputTokens(reqBody)

	// 处理响应
	if stream && resp.StatusCode == http.StatusOK && isEventStream(resp.Header.Get("Content-Type")) {
		p.handleStreamResponseWithStats(w, resp, startTime, model, clientIP, inputTokens, string(body))
	} else {
		p.handleNormalResponseWithStats(w, resp, startTime, model, clientIP, inputTokens, string(body))
	}
}

// Embeddings 向量嵌入代理
func (p *Proxy) Embeddings(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	clientIP := getClientIP(r)

	// 速率限制检查
	if !p.rateLimit.Allow() {
		p.writeError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}

	// 读取请求体
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, p.cfg.MaxRequestBodySize))
	if err != nil {
		p.writeError(w, http.StatusBadRequest, "读取请求体失败")
		return
	}
	defer r.Body.Close()

	// 验证本地 API Key
	if !p.validateLocalAPIKey(r) {
		p.writeError(w, http.StatusUnauthorized, "API Key 无效")
		return
	}

	// 获取 Coding Plan API Key
	codingAPIKey := p.cfg.APIKey
	if codingAPIKey == "" {
		p.writeError(w, http.StatusInternalServerError, "服务未配置 API Key")
		return
	}

	// 获取服务商配置
	provider, err := p.cfg.GetProviderConfig()
	if err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 构建目标 URL
	baseURL := provider.CodingBaseURL
	if !p.cfg.UseCodingEndpoint {
		baseURL = provider.GeneralBaseURL
	}
	targetURL := fmt.Sprintf("%s/embeddings", baseURL)

	// 构建请求头
	headers := p.buildHeaders(provider, codingAPIKey, false)

	// 创建上游请求
	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		p.writeError(w, http.StatusInternalServerError, "创建请求失败")
		return
	}

	for k, v := range headers {
		upstreamReq.Header.Set(k, v)
	}

	// 发送请求
	resp, err := p.client.Do(upstreamReq)
	if err != nil {
		p.writeError(w, http.StatusBadGateway, "上游服务不可用")
		return
	}
	defer resp.Body.Close()

	// 读取响应体用于存储
	respBody, _ := io.ReadAll(resp.Body)

	// 复制响应
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)

	// 保存记录
	duration := time.Since(startTime).Milliseconds()
	record := &storage.RequestRecord{
		Timestamp:    startTime,
		Provider:     p.cfg.Provider,
		Model:        "",
		Stream:       false,
		Method:       "POST",
		Path:         "/v1/embeddings",
		ClientIP:     clientIP,
		RequestBody:  string(body),
		ResponseBody: string(respBody),
		StatusCode:   resp.StatusCode,
		Duration:     float64(duration),
		InputTokens:  0,
		OutputTokens: 0,
		TotalTokens:  0,
		Success:      resp.StatusCode == 200,
	}

	go p.storage.SaveRequest(record)
}

// validateLocalAPIKey 验证本地 API Key
func (p *Proxy) validateLocalAPIKey(r *http.Request) bool {
	localAPIKey := p.cfg.LocalAPIKey

	// 如果未配置本地 API Key，则不验证
	if localAPIKey == "" {
		return true
	}

	// 从请求头获取客户端 API Key
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	// 移除 "Bearer " 前缀
	clientKey := strings.TrimPrefix(authHeader, "Bearer ")
	return clientKey == localAPIKey
}

// buildHeaders 构建请求头
func (p *Proxy) buildHeaders(provider *config.ProviderConfig, apiKey string, stream bool) map[string]string {
	// 获取有效的 User-Agent（基于伪装工具配置）
	userAgent := p.cfg.GetEffectiveUserAgent()
	accept := "application/json"
	if stream {
		accept = "text/event-stream"
	}

	headers := map[string]string{
		"Content-Type":      "application/json",
		provider.AuthHeader: provider.AuthPrefix + apiKey,
		"User-Agent":        userAgent,
		"Accept":            accept,
	}

	// 添加额外头部
	for k, v := range provider.ExtraHeaders {
		headers[k] = v
	}

	return headers
}

// handleStreamResponseWithStats 处理流式响应并统计
func (p *Proxy) handleStreamResponseWithStats(w http.ResponseWriter, resp *http.Response, startTime time.Time, model, clientIP string, inputTokens int, requestBody string) {
	copyHeaders(w.Header(), resp.Header)

	// 设置 SSE 头
	if !isEventStream(w.Header().Get("Content-Type")) {
		w.Header().Set("Content-Type", "text/event-stream")
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(resp.StatusCode)

	// 获取 flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		p.writeError(w, http.StatusInternalServerError, "不支持流式响应")
		return
	}

	// 读取并转发响应，同时收集数据
	var responseBuf bytes.Buffer
	var outputTokens int

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		responseBuf.WriteString(line + "\n")

		if _, err := fmt.Fprintln(w, line); err != nil {
			p.logger.Warn("写入流式响应失败", zap.Error(err))
			break
		}
		flusher.Flush()

		// 解析 SSE 数据提取 token
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			// 提取 usage 中的 token
			if usage, ok := chunk["usage"].(map[string]interface{}); ok {
				if pt, ok := usage["total_tokens"].(float64); ok {
					outputTokens = int(pt)
				}
				if pt, ok := usage["completion_tokens"].(float64); ok {
					outputTokens = int(pt)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		p.logger.Warn("读取流式响应失败", zap.Error(err))
	}

	duration := time.Since(startTime).Milliseconds()
	totalTokens := inputTokens + outputTokens

	p.logger.Info("流式请求完成",
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("output_tokens", outputTokens),
	)

	// 保存记录
	record := &storage.RequestRecord{
		Timestamp:    startTime,
		Provider:     p.cfg.Provider,
		Model:        model,
		Stream:       true,
		Method:       "POST",
		Path:         "/v1/chat/completions",
		ClientIP:     clientIP,
		RequestBody:  requestBody,
		ResponseBody: responseBuf.String(),
		StatusCode:   resp.StatusCode,
		Duration:     float64(duration),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  totalTokens,
		Success:      resp.StatusCode == 200,
	}

	go p.storage.SaveRequest(record)
}

// handleNormalResponseWithStats 处理普通响应并统计
func (p *Proxy) handleNormalResponseWithStats(w http.ResponseWriter, resp *http.Response, startTime time.Time, model, clientIP string, inputTokens int, requestBody string) {
	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logger.Error("读取响应体失败", zap.Error(err))
		return
	}

	// 复制响应头
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)

	duration := time.Since(startTime).Milliseconds()

	// 解析响应获取 token
	var outputTokens int
	var respData map[string]interface{}
	if err := json.Unmarshal(respBody, &respData); err == nil {
		if usage, ok := respData["usage"].(map[string]interface{}); ok {
			if pt, ok := usage["total_tokens"].(float64); ok {
				outputTokens = int(pt)
			}
			if pt, ok := usage["completion_tokens"].(float64); ok {
				outputTokens = int(pt)
			}
		}
	}

	totalTokens := inputTokens + outputTokens

	p.logger.Info("请求完成",
		zap.Int("status", resp.StatusCode),
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("input_tokens", inputTokens),
		zap.Int("output_tokens", outputTokens),
	)

	// 保存记录
	record := &storage.RequestRecord{
		Timestamp:    startTime,
		Provider:     p.cfg.Provider,
		Model:        model,
		Stream:       false,
		Method:       "POST",
		Path:         "/v1/chat/completions",
		ClientIP:     clientIP,
		RequestBody:  requestBody,
		ResponseBody: string(respBody),
		StatusCode:   resp.StatusCode,
		Duration:     float64(duration),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  totalTokens,
		Success:      resp.StatusCode == 200,
	}

	go p.storage.SaveRequest(record)
}

// writeError 写入错误响应
func (p *Proxy) writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	errorResp := map[string]interface{}{
		"error": map[string]string{
			"message": message,
			"type":    "proxy_error",
			"code":    fmt.Sprintf("%d", code),
		},
	}

	json.NewEncoder(w).Encode(errorResp)
}

// Stats 返回统计信息
func (p *Proxy) Stats() map[string]interface{} {
	count, max, remaining := p.rateLimit.Stats()
	stats, err := p.storage.GetStats()
	if err != nil || stats == nil {
		stats = &storage.Stats{}
	}
	return map[string]interface{}{
		"request_count":    count,
		"rate_limit":       max,
		"window_remaining": remaining.String(),
		"total_tokens":     stats.TotalTokens,
		"total_input":      stats.TotalInputTokens,
		"total_output":     stats.TotalOutputTokens,
	}
}

func copyHeaders(dst, src http.Header) {
	for k, values := range src {
		dst[k] = append([]string(nil), values...)
	}
}

func isEventStream(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

// getClientIP 获取客户端 IP
func getClientIP(r *http.Request) string {
	// 检查代理头
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	return r.RemoteAddr
}

// estimateInputTokens 估算输入 token 数量
func estimateInputTokens(reqBody map[string]interface{}) int {
	// 简单估算：每个字符约 0.5 token
	if messages, ok := reqBody["messages"].([]interface{}); ok {
		totalChars := 0
		for _, msg := range messages {
			if m, ok := msg.(map[string]interface{}); ok {
				if content, ok := m["content"].(string); ok {
					totalChars += len(content)
				}
			}
		}
		return totalChars / 2 // 粗略估算
	}
	return 0
}
