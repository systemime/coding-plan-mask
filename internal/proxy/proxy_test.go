package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"coding-plan-mask/internal/config"
	"coding-plan-mask/internal/storage"

	"go.uber.org/zap"
)

type flushingRecorder struct {
	*httptest.ResponseRecorder
}

func (r *flushingRecorder) Flush() {}

func TestBuildHeadersPreservesRequestHeadersAndOverridesAuth(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DisguiseTool = "claudecode"

	p := &Proxy{cfg: cfg}
	provider := &config.ProviderConfig{
		AuthHeader:   "Authorization",
		AuthPrefix:   "Bearer ",
		ExtraHeaders: map[string]string{},
	}

	requestHeaders := http.Header{
		"Accept":        []string{"application/json"},
		"Authorization": []string{"Bearer local-key"},
		"X-Custom":      []string{"custom-value"},
	}

	headers := p.buildHeaders(provider, "test-key", requestHeaders)

	if got := headers.Get("Accept"); got != "application/json" {
		t.Fatalf("expected Accept header to be preserved, got %q", got)
	}
	if got := headers.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("expected upstream Authorization header, got %q", got)
	}
	if got := headers.Get("User-Agent"); got != cfg.GetEffectiveUserAgent() {
		t.Fatalf("expected disguised User-Agent, got %q", got)
	}
	if got := headers.Get("X-App"); got != config.ClaudeCodeAppHeaderValue {
		t.Fatalf("expected Claude Code X-App header, got %q", got)
	}
	if got := headers.Get("X-Custom"); got != "custom-value" {
		t.Fatalf("expected custom header to be preserved, got %q", got)
	}
}

func TestBuildHeadersPreservesExistingXAppHeader(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DisguiseTool = "claudecode"

	p := &Proxy{cfg: cfg}
	provider := &config.ProviderConfig{
		AuthHeader:   "Authorization",
		AuthPrefix:   "Bearer ",
		ExtraHeaders: map[string]string{},
	}

	requestHeaders := http.Header{
		"Authorization": []string{"Bearer local-key"},
		"X-App":         []string{"custom-cli"},
	}

	headers := p.buildHeaders(provider, "test-key", requestHeaders)
	if got := headers.Get("X-App"); got != "custom-cli" {
		t.Fatalf("expected existing X-App header to be preserved, got %q", got)
	}
}

func TestBuildHeadersAddsSessionIdAndClientRequestIdForClaudeCode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DisguiseTool = "claudecode"

	p := &Proxy{cfg: cfg}
	provider := &config.ProviderConfig{
		AuthHeader:   "Authorization",
		AuthPrefix:   "Bearer ",
		ExtraHeaders: map[string]string{},
	}

	requestHeaders := http.Header{
		"Authorization": []string{"Bearer local-key"},
	}

	headers := p.buildHeaders(provider, "test-key", requestHeaders)

	sessionID := headers.Get("X-Claude-Code-Session-Id")
	if sessionID == "" {
		t.Fatal("expected X-Claude-Code-Session-Id to be set for claudecode mode")
	}
	if len(sessionID) != 36 {
		t.Fatalf("expected UUID format for session ID, got %q", sessionID)
	}

	clientReqID := headers.Get("x-client-request-id")
	if clientReqID == "" {
		t.Fatal("expected x-client-request-id to be set for claudecode mode")
	}
	if len(clientReqID) != 36 {
		t.Fatalf("expected UUID format for client request ID, got %q", clientReqID)
	}
}

func TestBuildHeadersPreservesExistingSessionAndClientRequestId(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DisguiseTool = "claudecode"

	p := &Proxy{cfg: cfg}
	provider := &config.ProviderConfig{
		AuthHeader:   "Authorization",
		AuthPrefix:   "Bearer ",
		ExtraHeaders: map[string]string{},
	}

	requestHeaders := http.Header{
		"Authorization":            []string{"Bearer local-key"},
		"X-Claude-Code-Session-Id": []string{"custom-session-id"},
		"x-client-request-id":      []string{"custom-client-req-id"},
	}

	headers := p.buildHeaders(provider, "test-key", requestHeaders)

	if got := headers.Get("X-Claude-Code-Session-Id"); got != "custom-session-id" {
		t.Fatalf("expected existing X-Claude-Code-Session-Id to be preserved, got %q", got)
	}
	if got := headers.Get("x-client-request-id"); got != "custom-client-req-id" {
		t.Fatalf("expected existing x-client-request-id to be preserved, got %q", got)
	}
}

func TestBuildHeadersNoExtraHeadersForNonClaudeCode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DisguiseTool = "openclaw"

	p := &Proxy{cfg: cfg}
	provider := &config.ProviderConfig{
		AuthHeader:   "Authorization",
		AuthPrefix:   "Bearer ",
		ExtraHeaders: map[string]string{},
	}

	requestHeaders := http.Header{
		"Authorization": []string{"Bearer local-key"},
	}

	headers := p.buildHeaders(provider, "test-key", requestHeaders)

	if headers.Get("X-Claude-Code-Session-Id") != "" {
		t.Fatal("expected no X-Claude-Code-Session-Id for non-claudecode mode")
	}
	if headers.Get("x-client-request-id") != "" {
		t.Fatal("expected no x-client-request-id for non-claudecode mode")
	}
}

func TestBuildTargetURLPreservesPathAndQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/chat/completions?foo=bar", nil)

	got := buildTargetURL("https://example.com/api/coding/paas/v4", req, false)
	want := "https://example.com/api/coding/paas/v4/chat/completions?foo=bar"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildTargetURLWithRemoveVersionPath(t *testing.T) {
	tests := []struct {
		name              string
		baseURL           string
		requestPath       string
		removeVersionPath bool
		want              string
	}{
		{
			name:              "remove v1 prefix",
			baseURL:           "https://api.example.com",
			requestPath:       "/v1/models",
			removeVersionPath: true,
			want:              "https://api.example.com/models",
		},
		{
			name:              "remove v1 prefix with longer path",
			baseURL:           "https://api.example.com",
			requestPath:       "/v1/chat/completions",
			removeVersionPath: true,
			want:              "https://api.example.com/chat/completions",
		},
		{
			name:              "do not remove when disabled",
			baseURL:           "https://api.example.com",
			requestPath:       "/v1/models",
			removeVersionPath: false,
			want:              "https://api.example.com/v1/models",
		},
		{
			name:              "remove v2 prefix",
			baseURL:           "https://api.example.com",
			requestPath:       "/v2/assistants",
			removeVersionPath: true,
			want:              "https://api.example.com/assistants",
		},
		{
			name:              "path without version prefix unchanged",
			baseURL:           "https://api.example.com",
			requestPath:       "/models",
			removeVersionPath: true,
			want:              "https://api.example.com/models",
		},
		{
			name:              "preserve query params",
			baseURL:           "https://api.example.com",
			requestPath:       "/v1/models?limit=10",
			removeVersionPath: true,
			want:              "https://api.example.com/models?limit=10",
		},
		{
			name:              "only version path becomes empty",
			baseURL:           "https://api.example.com",
			requestPath:       "/v1",
			removeVersionPath: true,
			want:              "https://api.example.com",
		},
		{
			name:              "remove v1beta prefix",
			baseURL:           "https://api.example.com",
			requestPath:       "/v1beta/files",
			removeVersionPath: true,
			want:              "https://api.example.com/files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.requestPath, nil)
			got := buildTargetURL(tt.baseURL, req, tt.removeVersionPath)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestHandleStreamResponsePreservesEventBoundaries(t *testing.T) {
	cfg := config.DefaultConfig()
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	p := New(cfg, zap.NewNop(), store)

	recorder := &flushingRecorder{ResponseRecorder: httptest.NewRecorder()}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader("data: {\"usage\":{\"completion_tokens\":3}}\n\ndata: [DONE]\n\n")),
	}

	p.handleStreamResponseWithStats(recorder, resp, time.Now(), http.MethodPost, "/chat/completions", "https://api.example.com/chat/completions", "glm-4-flash", "127.0.0.1", 2, "{}", 0)

	body := recorder.Body.String()
	if !strings.Contains(body, "\n\n") {
		t.Fatalf("expected SSE event boundary in body, got %q", body)
	}

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 status code, got %d", recorder.Code)
	}
}

func TestNonDebugLoggingUsesHumanReadableFormat(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Debug = false
	var out bytes.Buffer
	p := &Proxy{cfg: cfg, logger: zap.NewNop(), output: &out}

	p.logForwardRequest("glm-5", 123)
	p.logForwardResponse("glm-5", 456)

	logText := out.String()
	if !strings.Contains(logText, "转发请求：模型：glm-5 token数：123") {
		t.Fatalf("expected human-readable request log, got %q", logText)
	}
	if !strings.Contains(logText, "转发响应：模型：glm-5 token数：456") {
		t.Fatalf("expected human-readable response log, got %q", logText)
	}
}

func TestEstimateOutputTokensFromResponseFallsBackToContent(t *testing.T) {
	respData := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"content": "你好，世界",
				},
			},
		},
	}

	got := estimateOutputTokensFromResponse(respData, nil)
	if got <= 0 {
		t.Fatalf("expected fallback output token estimate to be positive, got %d", got)
	}
}

func TestIsModelsRequest(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "match /models",
			path:     "/models",
			expected: true,
		},
		{
			name:     "match /v1/models",
			path:     "/v1/models",
			expected: true,
		},
		{
			name:     "match /v2/models",
			path:     "/v2/models",
			expected: true,
		},
		{
			name:     "match /v3/models",
			path:     "/v3/models",
			expected: true,
		},
		{
			name:     "match /models/ with trailing slash",
			path:     "/models/",
			expected: true,
		},
		{
			name:     "match /v1/models/ with trailing slash",
			path:     "/v1/models/",
			expected: true,
		},
		{
			name:     "not match /chat/completions",
			path:     "/chat/completions",
			expected: false,
		},
		{
			name:     "not match /v1/chat/completions",
			path:     "/v1/chat/completions",
			expected: false,
		},
		{
			name:     "not match /v4/models (unsupported version)",
			path:     "/v4/models",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			p := &Proxy{cfg: cfg}

			got := p.isModelsRequest(tt.path)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestMockModelsResponse(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	cfg := config.DefaultConfig()
	cfg.MockModels = true
	cfg.RemoveVersionPath = true // 启用后匹配 /models
	cfg.MockModelsResp = `{"object":"list","data":[{"id":"test-model","object":"model","owned_by":"test"}]}`
	cfg.LocalAPIKey = "" // 不验证本地 API Key

	p := New(cfg, zap.NewNop(), store)

	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	recorder := httptest.NewRecorder()

	p.Forward(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", recorder.Code, recorder.Body.String())
	}

	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %s", recorder.Header().Get("Content-Type"))
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "test-model") {
		t.Fatalf("expected mock response to contain 'test-model', got %s", body)
	}
}

func TestMockModelsWithV1Path(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	cfg := config.DefaultConfig()
	cfg.MockModels = true
	cfg.RemoveVersionPath = false // 默认值，匹配 /v1/models
	cfg.MockModelsResp = `{"object":"list","data":[{"id":"v1-model"}]}`
	cfg.LocalAPIKey = ""

	p := New(cfg, zap.NewNop(), store)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	recorder := httptest.NewRecorder()

	p.Forward(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", recorder.Code, recorder.Body.String())
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "v1-model") {
		t.Fatalf("expected mock response to contain 'v1-model', got %s", body)
	}
}

func TestMockModelsDisabled(t *testing.T) {
	// isModelsRequest 只检查路径，不检查 MockModels 配置
	// MockModels 配置在 Forward 函数中检查
	cfg := config.DefaultConfig()
	p := &Proxy{cfg: cfg}

	// isModelsRequest 应该始终匹配路径，不管 MockModels 设置
	if !p.isModelsRequest("/models") {
		t.Fatal("expected isModelsRequest to return true for /models path")
	}
	if !p.isModelsRequest("/v1/models") {
		t.Fatal("expected isModelsRequest to return true for /v1/models path")
	}
}

func TestMockModelsWithRemoveVersionPath(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MockModels = true
	cfg.RemoveVersionPath = true
	cfg.MockModelsResp = `{"object":"list","data":[{"id":"v2-model"}]}`
	cfg.LocalAPIKey = ""

	p := New(cfg, zap.NewNop(), nil)

	tests := []struct {
		path       string
		shouldMock bool
	}{
		{"/models", true},
		{"/v1/models", true}, // 现在也匹配，因为无论 remove_version_path 如何都会 mock
		{"/v2/models", true},
		{"/chat/completions", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if p.isModelsRequest(tt.path) != tt.shouldMock {
				t.Fatalf("path %s: expected shouldMock=%v", tt.path, tt.shouldMock)
			}
		})
	}
}

func TestFixAnthropicSchema(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "fix required null to empty array",
			input: map[string]interface{}{
				"required": nil,
			},
			expected: map[string]interface{}{
				"required": []interface{}{},
			},
		},
		{
			name: "fix enum null to empty array",
			input: map[string]interface{}{
				"enum": nil,
			},
			expected: map[string]interface{}{
				"enum": []interface{}{},
			},
		},
		{
			name: "fix items null to default schema",
			input: map[string]interface{}{
				"items": nil,
			},
			expected: map[string]interface{}{
				"items": map[string]interface{}{"type": "string"},
			},
		},
		{
			name: "fix nested schema",
			input: map[string]interface{}{
				"tools": []interface{}{
					map[string]interface{}{
						"function": map[string]interface{}{
							"parameters": map[string]interface{}{
								"required": nil,
								"properties": map[string]interface{}{
									"query": map[string]interface{}{
										"type": "string",
									},
								},
							},
						},
					},
				},
			},
			expected: map[string]interface{}{
				"tools": []interface{}{
					map[string]interface{}{
						"function": map[string]interface{}{
							"parameters": map[string]interface{}{
								"required": []interface{}{},
								"properties": map[string]interface{}{
									"query": map[string]interface{}{
										"type": "string",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "preserve non-null values",
			input: map[string]interface{}{
				"required": []interface{}{"query"},
				"type":     "object",
			},
			expected: map[string]interface{}{
				"required": []interface{}{"query"},
				"type":     "object",
			},
		},
		{
			name: "fix properties null",
			input: map[string]interface{}{
				"properties": nil,
			},
			expected: map[string]interface{}{
				"properties": map[string]interface{}{},
			},
		},
		{
			name: "fix anyOf/allOf/oneOf null",
			input: map[string]interface{}{
				"anyOf": nil,
				"allOf": nil,
				"oneOf": nil,
			},
			expected: map[string]interface{}{
				"anyOf": []interface{}{},
				"allOf": []interface{}{},
				"oneOf": []interface{}{},
			},
		},
		{
			name: "add missing required for object type",
			input: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			expected: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
				"required":   []interface{}{},
			},
		},
		{
			name: "preserve existing required for object type",
			input: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
				"required":   []interface{}{"name"},
			},
			expected: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
				"required":   []interface{}{"name"},
			},
		},
		{
			name: "do not add required for non-object type",
			input: map[string]interface{}{
				"type": "string",
			},
			expected: map[string]interface{}{
				"type": "string",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := fixAnthropicSchema(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(result, tt.expected) {
				t.Fatalf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestInjectBillingHeaderWithStringSystem(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DisguiseTool = "claudecode"

	p := &Proxy{cfg: cfg}
	reqBody := map[string]interface{}{
		"system": "You are a helpful assistant.",
		"model":  "gpt-4",
	}

	body, _ := json.Marshal(reqBody)
	newBody := p.injectBillingHeader(body, reqBody)

	var result map[string]interface{}
	json.Unmarshal(newBody, &result)

	sys, _ := result["system"].(string)
	if !strings.HasPrefix(sys, "x-anthropic-billing-header:") {
		t.Fatalf("expected system to start with billing header, got %q", sys[:80])
	}
	if !strings.Contains(sys, "cc_version=") {
		t.Fatal("expected cc_version in billing header")
	}
	if !strings.Contains(sys, "cc_entrypoint=cli;") {
		t.Fatal("expected cc_entrypoint=cli in billing header")
	}
	if !strings.Contains(sys, "You are a helpful assistant.") {
		t.Fatal("expected original system content to be preserved")
	}
}

func TestInjectBillingHeaderWithArraySystem(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DisguiseTool = "claudecode"

	p := &Proxy{cfg: cfg}
	reqBody := map[string]interface{}{
		"system": []interface{}{
			map[string]interface{}{"type": "text", "text": "Hello"},
		},
	}

	body, _ := json.Marshal(reqBody)
	newBody := p.injectBillingHeader(body, reqBody)

	var result map[string]interface{}
	json.Unmarshal(newBody, &result)

	sysArr, ok := result["system"].([]interface{})
	if !ok {
		t.Fatal("expected system to remain an array")
	}
	if len(sysArr) != 2 {
		t.Fatalf("expected system array to have 2 elements, got %d", len(sysArr))
	}
	first, _ := sysArr[0].(string)
	if !strings.HasPrefix(first, "x-anthropic-billing-header:") {
		t.Fatalf("expected first element to be billing header, got %q", first)
	}
}

func TestInjectBillingHeaderWithSystemPrompt(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DisguiseTool = "claudecode"

	p := &Proxy{cfg: cfg}
	reqBody := map[string]interface{}{
		"system_prompt": "You are a helpful assistant.",
		"model":         "gpt-4",
	}

	body, _ := json.Marshal(reqBody)
	newBody := p.injectBillingHeader(body, reqBody)

	var result map[string]interface{}
	json.Unmarshal(newBody, &result)

	sysPrompt, _ := result["system_prompt"].(string)
	if !strings.HasPrefix(sysPrompt, "x-anthropic-billing-header:") {
		t.Fatalf("expected system_prompt to start with billing header, got %q", sysPrompt[:80])
	}
}

func TestInjectBillingHeaderNoSystemField(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DisguiseTool = "claudecode"

	p := &Proxy{cfg: cfg}
	reqBody := map[string]interface{}{
		"model": "gpt-4",
	}

	body, _ := json.Marshal(reqBody)
	newBody := p.injectBillingHeader(body, reqBody)

	// 没有 system 字段，body 应保持不变
	if string(newBody) != string(body) {
		t.Fatal("expected body to remain unchanged when no system field exists")
	}
}
