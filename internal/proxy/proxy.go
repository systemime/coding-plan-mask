// Package proxy 提供 API 代理转发功能
package proxy

import (
	"bufio"
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"coding-plan-mask/internal/config"
	"coding-plan-mask/internal/ratelimit"
	"coding-plan-mask/internal/security"
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
	security  *security.Service
	output    io.Writer
}

// New 创建新的代理实例
func New(cfg *config.Config, logger *zap.Logger, store *storage.Storage, securitySvc *security.Service) *Proxy {
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
		security:  securitySvc,
		output:    os.Stdout,
	}
}

// Close 关闭代理
func (p *Proxy) Close() error {
	p.client.CloseIdleConnections()
	return nil
}

// ChatCompletions 聊天补全代理
func (p *Proxy) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	p.Forward(w, r)
}

// Forward 通用透传代理
func (p *Proxy) Forward(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	clientIP := getClientIP(r)

	// 速率限制检查
	if !p.rateLimit.Allow() {
		p.writeError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}

	// 检查是否需要模拟 /models 响应
	if p.cfg.MockModels && p.isModelsRequest(r.URL.Path) {
		p.handleMockModels(w, r, startTime, clientIP)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, p.cfg.MaxRequestBodySize))
	if err != nil {
		p.writeError(w, http.StatusBadRequest, "读取请求体失败")
		return
	}
	defer r.Body.Close()

	reqBody, model, inputTokens := parseRequestMetadata(body)

	// Anthropic 格式兼容模式：修复请求体中的 schema 字段
	if p.cfg.UseAnthropic && reqBody != nil {
		originalLen := len(body)
		if fixedBody, err := fixAnthropicSchema(reqBody); err == nil {
			if newBody, err := json.Marshal(fixedBody); err == nil {
				reqBody = fixedBody
				body = newBody
				if p.cfg.Debug {
					p.logger.Debug("Anthropic schema 修复完成",
						zap.Int("original_len", originalLen),
						zap.Int("fixed_len", len(newBody)),
						zap.Int("diff", len(newBody)-originalLen))
					// 输出 tools 部分的结构用于调试
					if tools, ok := fixedBody["tools"]; ok {
						if toolsBytes, err := json.Marshal(tools); err == nil && len(toolsBytes) < 5000 {
							p.logger.Debug("修复后的 tools", zap.String("tools", string(toolsBytes)))
						}
					}
				}
			}
		}
	}

	// 检查请求是否要求流式响应
	isStreamRequest := false
	if reqBody != nil {
		if stream, ok := reqBody["stream"].(bool); ok {
			isStreamRequest = stream
		}
	}

	// 验证本地 API Key
	if !p.validateLocalAPIKey(r) {
		p.writeError(w, http.StatusUnauthorized, "API Key 无效")
		return
	}

	// 数据安全过滤
	if p.security != nil && reqBody != nil {
		updatedBody, decision, err := p.applySecurityFilter(r, reqBody)
		if err != nil {
			if policyErr, ok := err.(*security.PolicyError); ok {
				p.writeError(w, policyErr.StatusCode, policyErr.Decision.Reason)
				return
			}
			p.logger.Error("安全过滤失败", zap.Error(err))
			p.writeError(w, http.StatusInternalServerError, "安全过滤失败")
			return
		}
		if decision != nil && decision.Action == "redact" {
			body = updatedBody
			reqBody, model, inputTokens = parseRequestMetadata(body)
		}
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
	targetURL := buildTargetURL(baseURL, r, p.cfg.RemoveVersionPath)

	// 构建请求头
	headers := p.buildHeaders(provider, codingAPIKey, r.Header)

	// claudecode 模式：在请求体中注入 x-anthropic-billing-header
	if config.NormalizeDisguiseTool(p.cfg.DisguiseTool) == "claudecode" && reqBody != nil {
		body = p.injectBillingHeader(body, reqBody)
	}

	// 日志记录
	p.logForwardRequest(model, inputTokens)

	// 插入待处理记录到数据库
	pendingRecord := &storage.RequestRecord{
		Timestamp:   startTime,
		Provider:    p.cfg.Provider,
		Model:       model,
		Method:      r.Method,
		Path:        r.URL.Path,
		ClientIP:    clientIP,
		RequestBody: string(body),
		InputTokens: inputTokens,
	}
	recordID, err := p.storage.InsertPendingRequest(pendingRecord)
	if err != nil {
		p.logger.Error("插入待处理记录失败", zap.Error(err))
	}

	// 辅助函数：异步更新记录为失败状态
	updateFailedRecord := func(statusCode int, errMsg string) {
		if recordID > 0 {
			go func() {
				duration := time.Since(startTime).Milliseconds()
				updateRecord := &storage.RequestRecord{
					StatusCode: statusCode,
					Duration:   float64(duration),
					Success:    false,
					ErrorMsg:   errMsg,
				}
				if err := p.storage.UpdateRequestWithResponse(recordID, updateRecord); err != nil {
					p.logger.Error("更新失败记录失败", zap.Error(err))
				}
			}()
		}
	}

	// 创建上游请求
	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		updateFailedRecord(http.StatusInternalServerError, "创建请求失败")
		p.writeError(w, http.StatusInternalServerError, "创建请求失败")
		return
	}

	// 设置请求头
	for k, values := range headers {
		upstreamReq.Header[k] = append([]string(nil), values...)
	}

	// 发送请求
	resp, err := p.client.Do(upstreamReq)
	if err != nil {
		if strings.Contains(err.Error(), "context canceled") {
			updateFailedRecord(499, "请求被取消")
			p.logger.Info("请求被取消", zap.String("model", model))
			return
		}
		updateFailedRecord(http.StatusBadGateway, "上游服务不可用: "+err.Error())
		p.logger.Error("上游请求失败", zap.Error(err))
		p.writeError(w, http.StatusBadGateway, "上游服务不可用")
		return
	}
	defer resp.Body.Close()

	// 处理响应 - 方案A: 同时检查请求中的 stream 参数和响应 Content-Type
	// 如果客户端请求 stream=true，或者上游返回 SSE 格式，都使用流式处理
	isStreamResponse := isStreamRequest || isEventStream(resp.Header.Get("Content-Type"))
	if resp.StatusCode == http.StatusOK && isStreamResponse {
		p.handleStreamResponseWithStats(w, resp, startTime, r.Method, r.URL.Path, targetURL, model, clientIP, inputTokens, string(body), recordID)
	} else {
		p.handleNormalResponseWithStats(w, resp, startTime, r.Method, r.URL.Path, targetURL, model, clientIP, inputTokens, string(body), recordID)
	}
}

// Embeddings 向量嵌入代理
func (p *Proxy) Embeddings(w http.ResponseWriter, r *http.Request) {
	p.Forward(w, r)
}

// isModelsRequest 检查是否是 /models 请求
// 匹配规则:
// - 始终匹配 /models
// - 始终匹配 /v1/models, /v2/models, /v3/models (版本前缀格式)
func (p *Proxy) isModelsRequest(path string) bool {
	path = strings.TrimSuffix(path, "/")

	// 匹配 /models
	if path == "/models" {
		return true
	}

	// 匹配 /v1/models, /v2/models, /v3/models 等
	if strings.HasSuffix(path, "/models") {
		prefix := strings.TrimSuffix(path, "/models")
		return prefix == "/v1" || prefix == "/v2" || prefix == "/v3"
	}

	return false
}

// handleMockModels 处理模拟 /models 响应
func (p *Proxy) handleMockModels(w http.ResponseWriter, r *http.Request, startTime time.Time, clientIP string) {
	duration := time.Since(startTime).Milliseconds()

	// 验证本地 API Key
	if !p.validateLocalAPIKey(r) {
		p.writeError(w, http.StatusUnauthorized, "API Key 无效")
		return
	}

	// 返回模拟响应
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(p.cfg.MockModelsResp))

	// 打印终端日志
	fmt.Fprintf(p.logOutput(), "时间：%s Mock Models: 路径=%s 客户端=%s\n", humanLogTime(), r.URL.Path, clientIP)

	// 打印日志
	p.logResponse(r.Method, r.URL.Path, "mock://models", http.StatusOK, duration, clientIP, p.cfg.MockModelsResp)
}

// validateLocalAPIKey 验证本地 API Key
func (p *Proxy) validateLocalAPIKey(r *http.Request) bool {
	localAPIKey := p.cfg.LocalAPIKey

	// 如果未配置本地 API Key，则不验证
	if localAPIKey == "" {
		return !p.cfg.Security.Enabled
	}

	// 从请求头获取客户端 API Key
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	// 移除 "Bearer " 前缀
	clientKey := strings.TrimPrefix(authHeader, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(clientKey), []byte(localAPIKey)) == 1
}

func (p *Proxy) applySecurityFilter(r *http.Request, reqBody map[string]interface{}) ([]byte, *security.PolicyDecision, error) {
	updated, decision, err := p.security.ProcessProxyPayload(r, reqBody)
	if err != nil {
		return nil, decision, err
	}
	body, marshalErr := json.Marshal(updated)
	return body, decision, marshalErr
}

// buildHeaders 构建请求头
func (p *Proxy) buildHeaders(provider *config.ProviderConfig, apiKey string, requestHeaders http.Header) http.Header {
	// 获取有效的 User-Agent（基于伪装工具配置）
	userAgent := p.cfg.GetEffectiveUserAgent()

	headers := make(http.Header, len(requestHeaders)+len(provider.ExtraHeaders)+2)
	for k, values := range requestHeaders {
		canonicalKey := textproto.CanonicalMIMEHeaderKey(k)
		if isHopByHopHeader(canonicalKey) {
			continue
		}
		if canonicalKey == "Authorization" || canonicalKey == textproto.CanonicalMIMEHeaderKey(provider.AuthHeader) {
			continue
		}
		headers[canonicalKey] = append([]string(nil), values...)
	}

	headers.Set(provider.AuthHeader, provider.AuthPrefix+apiKey)
	headers.Set("User-Agent", userAgent)

	for k, v := range p.cfg.GetDisguiseHeaders() {
		if headers.Get(k) == "" {
			headers.Set(k, v)
		}
	}

	// claudecode 模式：注入 x-client-request-id（仅当客户端未提供时）
	if config.NormalizeDisguiseTool(p.cfg.DisguiseTool) == "claudecode" {
		if headers.Get("x-client-request-id") == "" {
			headers.Set("x-client-request-id", p.cfg.GetClientRequestID())
		}
	}

	// 添加额外头部
	for k, v := range provider.ExtraHeaders {
		headers.Set(k, v)
	}

	return headers
}

// injectBillingHeader 在请求体的 system/system_prompt 字段首行注入 x-anthropic-billing-header
// 模拟真实 Claude Code 客户端行为：将计费属性头作为 system prompt 的第一个块
func (p *Proxy) injectBillingHeader(body []byte, reqBody map[string]interface{}) []byte {
	billingHeader := p.cfg.GetBillingHeader()
	if billingHeader == "" {
		return body
	}

	modified := false

	// 优先处理 "system" 字段（OpenAI 格式，字符串）
	if sys, ok := reqBody["system"].(string); ok {
		reqBody["system"] = billingHeader + "\n" + sys
		modified = true
	} else if sysArr, ok := reqBody["system"].([]interface{}); ok {
		// system 是数组形式（Anthropic 格式）
		newArr := make([]interface{}, len(sysArr)+1)
		newArr[0] = billingHeader
		for i, v := range sysArr {
			newArr[i+1] = v
		}
		reqBody["system"] = newArr
		modified = true
	}

	if !modified {
		// 没有 system 字段，尝试 system_prompt
		if sysPrompt, ok := reqBody["system_prompt"].(string); ok {
			reqBody["system_prompt"] = billingHeader + "\n" + sysPrompt
			modified = true
		}
	}

	if modified {
		if newBody, err := json.Marshal(reqBody); err == nil {
			return newBody
		}
	}

	return body
}

// handleStreamResponseWithStats 处理流式响应并统计
func (p *Proxy) handleStreamResponseWithStats(w http.ResponseWriter, resp *http.Response, startTime time.Time, method, path, targetURL, model, clientIP string, inputTokens int, requestBody string, recordID int64) {
	copyHeaders(w.Header(), resp.Header)

	// 设置 SSE 头
	if !isEventStream(w.Header().Get("Content-Type")) {
		w.Header().Set("Content-Type", "text/event-stream")
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// 获取 flusher（在 WriteHeader 之前检查）
	flusher, ok := w.(http.Flusher)
	if !ok {
		p.writeError(w, http.StatusInternalServerError, "不支持流式响应")
		return
	}

	w.WriteHeader(resp.StatusCode)

	// 读取并转发响应，同时收集数据
	var responseBuf bytes.Buffer
	var outputTokens int
	var responseText strings.Builder

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

			responseText.WriteString(extractResponseText(chunk))

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

	if outputTokens == 0 {
		outputTokens = estimateTextTokens(responseText.String())
	}

	duration := time.Since(startTime).Milliseconds()
	totalTokens := inputTokens + outputTokens

	// 打印响应日志
	p.logResponse(method, path, targetURL, resp.StatusCode, duration, clientIP, responseBuf.String())

	// 异步更新记录（不影响响应）
	if recordID > 0 {
		go func() {
			record := &storage.RequestRecord{
				ResponseBody: responseBuf.String(),
				StatusCode:   resp.StatusCode,
				Duration:     float64(duration),
				OutputTokens: outputTokens,
				TotalTokens:  totalTokens,
				Success:      resp.StatusCode == 200,
			}
			if err := p.storage.UpdateRequestWithResponse(recordID, record); err != nil {
				p.logger.Error("更新请求记录失败", zap.Error(err))
			}
		}()
	}
}

// handleNormalResponseWithStats 处理普通响应并统计
func (p *Proxy) handleNormalResponseWithStats(w http.ResponseWriter, resp *http.Response, startTime time.Time, method, path, targetURL, model, clientIP string, inputTokens int, requestBody string, recordID int64) {
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
	if outputTokens == 0 {
		outputTokens = estimateOutputTokensFromResponse(respData, respBody)
	}

	totalTokens := inputTokens + outputTokens

	// 打印响应日志
	p.logResponse(method, path, targetURL, resp.StatusCode, duration, clientIP, string(respBody))

	// 异步更新记录（不影响响应）
	if recordID > 0 {
		go func() {
			record := &storage.RequestRecord{
				ResponseBody: string(respBody),
				StatusCode:   resp.StatusCode,
				Duration:     float64(duration),
				OutputTokens: outputTokens,
				TotalTokens:  totalTokens,
				Success:      resp.StatusCode == 200,
			}
			if err := p.storage.UpdateRequestWithResponse(recordID, record); err != nil {
				p.logger.Error("更新请求记录失败", zap.Error(err))
			}
		}()
	}
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

func buildTargetURL(baseURL string, r *http.Request, removeVersionPath bool) string {
	targetURL := strings.TrimRight(baseURL, "/")
	if r.URL.Path != "" {
		path := r.URL.Path
		// 如果启用了移除版本路径，则移除 /v1, /v2 等版本前缀
		if removeVersionPath {
			path = removeVersionPrefix(path)
		}
		if path != "" {
			targetURL += "/" + strings.TrimLeft(path, "/")
		}
	}
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}
	return targetURL
}

// versionPrefixRegex 匹配版本前缀的正则表达式
// 匹配: /v1, /v2, /v1beta, /v2alpha, /v3rc 等（可选带尾部斜杠）
var versionPrefixRegex = regexp.MustCompile(`^/?v\d+[a-z]*(?:/|$)`)

// removeVersionPrefix 移除路径中的版本前缀（如 /v1, /v2 等）
func removeVersionPrefix(path string) string {
	// 使用正则匹配：/v 后面跟数字，可选跟 alpha/beta/rc 等后缀
	// 如果匹配到，移除版本前缀部分
	if versionPrefixRegex.MatchString(path) {
		// 移除开头的 / 和版本号部分
		path = versionPrefixRegex.ReplaceAllString(path, "")
		return strings.Trim(path, "/")
	}
	return strings.Trim(path, "/")
}

func isHopByHopHeader(key string) bool {
	switch key {
	case "Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}

func parseRequestMetadata(body []byte) (map[string]interface{}, string, int) {
	if len(body) == 0 {
		return nil, "", 0
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return nil, "", 0
	}

	model, _ := reqBody["model"].(string)
	return reqBody, model, estimateInputTokens(reqBody)
}

func (p *Proxy) logForwardRequest(model string, inputTokens int) {
	fields := []zap.Field{
		zap.Int("input_tokens", inputTokens),
	}
	if model != "" {
		fields = append(fields, zap.String("model", model))
	}
	if p.cfg.Debug {
		fields = append(fields, zap.String("provider", p.cfg.Provider))
		p.logger.Info("处理请求", fields...)
		return
	}
	fmt.Fprintf(p.logOutput(), "时间：%s 转发请求：模型：%s token数：%d\n", humanLogTime(), displayModel(model), inputTokens)
}

func (p *Proxy) logForwardResponse(model string, outputTokens int) {
	fields := []zap.Field{
		zap.Int("output_tokens", outputTokens),
	}
	if model != "" {
		fields = append(fields, zap.String("model", model))
	}
	if p.cfg.Debug {
		p.logger.Info("请求完成", fields...)
		return
	}
	fmt.Fprintf(p.logOutput(), "时间：%s 转发响应：模型：%s token数：%d\n", humanLogTime(), displayModel(model), outputTokens)
}

// logResponse 打印响应日志
func (p *Proxy) logResponse(method, path, targetURL string, statusCode int, duration int64, clientIP, responseBody string) {
	// 判断是否是错误状态码 (4xx 或 5xx)
	isError := statusCode >= 400 && statusCode < 600

	fields := []zap.Field{
		zap.String("method", method),
		zap.String("path", path),
		zap.String("target", targetURL),
		zap.Int("status", statusCode),
		zap.Int64("duration_ms", duration),
		zap.String("remote", clientIP),
	}

	if isError {
		// 限制响应体长度，避免日志过大
		truncatedBody := responseBody
		if len(truncatedBody) > 500 {
			truncatedBody = truncatedBody[:500] + "...(truncated)"
		}
		fields = append(fields, zap.String("response", truncatedBody))
		p.logger.Warn("代理响应", fields...)
	} else {
		p.logger.Info("代理响应", fields...)
	}
}

func (p *Proxy) logOutput() io.Writer {
	if p.output != nil {
		return p.output
	}
	return os.Stdout
}

func humanLogTime() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func displayModel(model string) string {
	if model == "" {
		return "-"
	}
	return model
}

func estimateOutputTokensFromResponse(respData map[string]interface{}, respBody []byte) int {
	if len(respData) != 0 {
		if tokens := estimateTextTokens(extractResponseText(respData)); tokens > 0 {
			return tokens
		}
	}
	return estimateTextTokens(string(respBody))
}

func estimateTextTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return utf8.RuneCountInString(text) / 2
}

func extractResponseText(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []interface{}:
		var b strings.Builder
		for _, item := range v {
			b.WriteString(extractResponseText(item))
		}
		return b.String()
	case map[string]interface{}:
		priorityKeys := []string{"output_text", "content", "text", "message", "delta"}
		var b strings.Builder
		for _, key := range priorityKeys {
			if child, ok := v[key]; ok {
				b.WriteString(extractResponseText(child))
			}
		}
		if choices, ok := v["choices"]; ok {
			b.WriteString(extractResponseText(choices))
		}
		return b.String()
	default:
		return ""
	}
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

// fixAnthropicSchema 修复 Anthropic 格式请求中的 schema 字段
// 将 null 值转换为正确的默认值，以兼容 OpenAI 格式的 API
func fixAnthropicSchema(data map[string]interface{}) (map[string]interface{}, error) {
	// 第一步：修复 null 值
	fixed := fixSchemaValue(data, "").(map[string]interface{})
	// 第二步：转换 Anthropic 格式到 OpenAI 格式
	fixed = convertAnthropicToOpenAI(fixed)
	return fixed, nil
}

// convertAnthropicToOpenAI 将 Anthropic 格式的 tools 转换为 OpenAI 格式
// Anthropic: tools[].input_schema
// OpenAI: tools[].function.parameters
func convertAnthropicToOpenAI(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range data {
		if key == "tools" {
			if tools, ok := value.([]interface{}); ok {
				newTools := make([]interface{}, len(tools))
				for i, tool := range tools {
					newTools[i] = convertToolToOpenAI(tool)
				}
				result[key] = newTools
			} else {
				result[key] = value
			}
		} else {
			result[key] = value
		}
	}

	return result
}

// convertToolToOpenAI 转换单个 tool 定义为 OpenAI 格式
func convertToolToOpenAI(tool interface{}) interface{} {
	toolMap, ok := tool.(map[string]interface{})
	if !ok {
		return tool
	}

	result := make(map[string]interface{})
	for k, v := range toolMap {
		result[k] = v
	}

	// 检查是否有 input_schema 字段（Anthropic 格式）
	if inputSchema, ok := result["input_schema"]; ok {
		// 将 input_schema 移动到 function.parameters
		funcDef := make(map[string]interface{})
		if name, ok := result["name"].(string); ok {
			funcDef["name"] = name
		}
		if desc, ok := result["description"].(string); ok {
			funcDef["description"] = desc
		}

		// 修复 input_schema 中的 null 值
		fixedSchema := fixSchemaValue(inputSchema, "input_schema")
		funcDef["parameters"] = fixedSchema

		// 删除原有的 Anthropic 格式字段
		delete(result, "input_schema")

		result["type"] = "function"
		result["function"] = funcDef

		// 删除可能重复的字段
		delete(result, "name")
		delete(result, "description")
	}

	// 如果有 function 字段，递归处理
	if funcDef, ok := result["function"].(map[string]interface{}); ok {
		fixedFunc := make(map[string]interface{})
		for k, v := range funcDef {
			if k == "parameters" {
				fixedFunc[k] = fixSchemaValue(v, "function.parameters")
			} else {
				fixedFunc[k] = v
			}
		}
		result["function"] = fixedFunc
	}

	return result
}

// fixSchemaValue 递归修复 schema 中的 null 值
// path 用于调试，记录当前处理的路径
func fixSchemaValue(value interface{}, path string) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			currentPath := path + "." + key
			// 处理特定字段的 null 值
			if val == nil {
				switch key {
				case "required", "enum", "examples", "prefixItems", "context":
					// 数组类型字段: null → []
					result[key] = []interface{}{}
				case "items":
					// items: null → items: {"type": "string"}
					result[key] = map[string]interface{}{"type": "string"}
				case "properties", "additionalProperties":
					// 对象类型字段: null → {}
					if key == "additionalProperties" {
						result[key] = true
					} else {
						result[key] = map[string]interface{}{}
					}
				case "anyOf", "allOf", "oneOf":
					// 组合类型: null → []
					result[key] = []interface{}{}
				case "default":
					// default: null → 删除该字段
					continue
				default:
					// 对于 schema 对象中的其他 null 字段，检查是否可能是数组类型
					// JSON Schema 中常见的数组字段
					if isArrayField(key) {
						result[key] = []interface{}{}
					} else if isObjectField(key) {
						// 对象类型字段: null → {}
						result[key] = map[string]interface{}{}
					} else {
						// 保留 null
						result[key] = nil
					}
				}
			} else {
				// 递归处理嵌套对象
				result[key] = fixSchemaValue(val, currentPath)
			}
		}

		// 对于 JSON Schema 对象（有 type 字段的对象），确保 required 字段存在
		if schemaType, ok := result["type"].(string); ok && schemaType == "object" {
			if _, hasRequired := result["required"]; !hasRequired {
				// 添加空的 required 数组
				result["required"] = []interface{}{}
			}
		}

		return result
	case []interface{}:
		// 递归处理数组
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = fixSchemaValue(item, path+"[]")
		}
		return result
	default:
		return value
	}
}

// isArrayField 判断字段名是否通常期望数组类型
func isArrayField(key string) bool {
	arrayFields := map[string]bool{
		"required":         true,
		"enum":             true,
		"examples":         true,
		"prefixItems":      true,
		"anyOf":            true,
		"allOf":            true,
		"oneOf":            true,
		"context":          true,
		"commands":         true,
		"options":          true,
		"arguments":        true,
		"parameters":       true,
		"variables":        true,
		"includes":         true,
		"excludes":         true,
		"items":            true, // 虽然 items 通常是对象，但也可能是数组（tuple items）
		"defs":             true,
		"definitions":      true,
		"$defs":            true,
		"schemas":          true,
		"security":         true,
		"servers":          true,
		"paths":            true,
		"tags":             true,
		"externalDocs":     true,
		"discriminator":    true,
		"xml":              true,
		"deprecated":       true,
		"readOnly":         true,
		"writeOnly":        true,
		"contentMediaType": true,
		"contentEncoding":  true,
		"if":               true,
		"then":             true,
		"else":             true,
	}
	return arrayFields[key]
}

// isObjectField 判断字段名是否通常期望对象类型
func isObjectField(key string) bool {
	objectFields := map[string]bool{
		"properties":            true,
		"additionalProperties":  true,
		"patternProperties":     true,
		"dependencies":          true,
		"dependentSchemas":      true,
		"propertyNames":         true,
		"unevaluatedItems":      true,
		"unevaluatedProperties": true,
		"contentSchema":         true,
	}
	return objectFields[key]
}
