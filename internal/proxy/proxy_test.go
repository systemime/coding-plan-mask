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
	"coding-plan-mask/internal/security"
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

	p := New(cfg, zap.NewNop(), store, nil)

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

func TestForwardRedactsSensitiveMessagesWhenSecurityEnabled(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	var receivedBody map[string]interface{}
	var decodeErr error
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			decodeErr = err
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer upstream.Close()

	cfg := config.DefaultConfig()
	cfg.Provider = "custom"
	cfg.APIKey = "upstream-test-key"
	cfg.CustomBaseURL = upstream.URL
	cfg.CustomCodingURL = upstream.URL
	cfg.Security.Enabled = true
	cfg.LocalAPIKey = "sk-test"
	cfg.Security.HandlingS2 = "redact"
	cfg.Security.HandlingS3 = "block"
	cfg.Security.SessionHeader = "X-Session-Id"
	cfg.Security.Redaction.Email = true

	securitySvc := security.NewService(security.Settings{
		Enabled:         true,
		AuditDir:        t.TempDir(),
		DefaultTrack:    "clean",
		DefaultTopK:     5,
		HandlingS2:      "redact",
		HandlingS3:      "block",
		PlaceholderS3:   "[PRIVATE]",
		SessionHeader:   "X-Session-Id",
		MaxAuditEntries: 100,
	}, zap.NewNop())

	p := New(cfg, zap.NewNop(), store, securitySvc)

	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"my password is abc123"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("X-Session-Id", "proxy-sec-session")
	rec := httptest.NewRecorder()

	p.Forward(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s decodeErr=%v", rec.Code, rec.Body.String(), decodeErr)
	}
	content := receivedBody["messages"].([]interface{})[0].(map[string]interface{})["content"].(string)
	if content != "my password is [REDACTED:PASSWORD]" {
		t.Fatalf("unexpected forwarded content: %q", content)
	}
}

func TestForwardBlocksSensitivePathsWhenSecurityEnabled(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := config.DefaultConfig()
	cfg.Provider = "custom"
	cfg.APIKey = "upstream-test-key"
	cfg.CustomBaseURL = upstream.URL
	cfg.CustomCodingURL = upstream.URL
	cfg.Security.Enabled = true
	cfg.LocalAPIKey = "sk-test"
	cfg.Security.HandlingS2 = "redact"
	cfg.Security.HandlingS3 = "block"

	securitySvc := security.NewService(security.Settings{
		Enabled:         true,
		AuditDir:        t.TempDir(),
		DefaultTrack:    "clean",
		DefaultTopK:     5,
		HandlingS2:      "redact",
		HandlingS3:      "block",
		PlaceholderS3:   "[PRIVATE]",
		SessionHeader:   "X-Session-Id",
		MaxAuditEntries: 100,
	}, zap.NewNop())

	p := New(cfg, zap.NewNop(), store, securitySvc)

	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"read ~/.ssh/id_rsa now"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")
	rec := httptest.NewRecorder()

	p.Forward(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	if upstreamCalled {
		t.Fatal("expected upstream not to be called when security blocks the request")
	}
}

func TestForwardRequiresLocalKeyWhenSecurityEnabled(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	cfg := config.DefaultConfig()
	cfg.Security.Enabled = true
	cfg.APIKey = "upstream-test-key"
	p := New(cfg, zap.NewNop(), store, security.NewService(security.Settings{Enabled: true, AuditDir: t.TempDir()}, zap.NewNop()))

	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	p.Forward(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when security is enabled without local key, got %d", rec.Code)
	}
}

func TestForwardPreservesOriginalHTTPMethod(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	var upstreamMethod string
	var upstreamQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamMethod = r.Method
		upstreamQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer upstream.Close()

	cfg := config.DefaultConfig()
	cfg.Provider = "custom"
	cfg.APIKey = "upstream-test-key"
	cfg.CustomBaseURL = upstream.URL
	cfg.CustomCodingURL = upstream.URL

	p := New(cfg, zap.NewNop(), store, nil)

	req := httptest.NewRequest(http.MethodGet, "/models?limit=10", nil)
	rec := httptest.NewRecorder()

	p.Forward(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if upstreamMethod != http.MethodGet {
		t.Fatalf("expected upstream method GET, got %s", upstreamMethod)
	}
	if upstreamQuery != "limit=10" {
		t.Fatalf("expected upstream query to be preserved, got %q", upstreamQuery)
	}
}

func TestForwardRedactsToolCallArgumentsWhenSecurityEnabled(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	var receivedBody map[string]interface{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer upstream.Close()

	cfg := config.DefaultConfig()
	cfg.Provider = "custom"
	cfg.APIKey = "upstream-test-key"
	cfg.CustomBaseURL = upstream.URL
	cfg.CustomCodingURL = upstream.URL
	cfg.Security.Enabled = true
	cfg.LocalAPIKey = "sk-test"
	cfg.Security.HandlingS2 = "redact"
	cfg.Security.HandlingS3 = "block"

	securitySvc := security.NewService(security.Settings{
		Enabled:         true,
		AuditDir:        t.TempDir(),
		DefaultTrack:    "clean",
		DefaultTopK:     5,
		HandlingS2:      "redact",
		HandlingS3:      "block",
		PlaceholderS3:   "[PRIVATE]",
		SessionHeader:   "X-Session-Id",
		MaxAuditEntries: 100,
	}, zap.NewNop())

	p := New(cfg, zap.NewNop(), store, securitySvc)

	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"messages":[{"role":"assistant","tool_calls":[{"type":"function","function":{"name":"read_file","arguments":"{\"path\":\"~/.ssh/id_rsa\",\"note\":\"password is abc123\"}"}}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")
	rec := httptest.NewRecorder()

	p.Forward(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for sensitive tool arguments, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestForwardRedactsTopLevelToolParamsWhenSecurityEnabled(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	var receivedBody map[string]interface{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer upstream.Close()

	cfg := config.DefaultConfig()
	cfg.Provider = "custom"
	cfg.APIKey = "upstream-test-key"
	cfg.CustomBaseURL = upstream.URL
	cfg.CustomCodingURL = upstream.URL
	cfg.Security.Enabled = true
	cfg.LocalAPIKey = "sk-test"
	cfg.Security.HandlingS2 = "redact"
	cfg.Security.HandlingS3 = "block"

	securitySvc := security.NewService(security.Settings{
		Enabled:         true,
		AuditDir:        t.TempDir(),
		DefaultTrack:    "clean",
		DefaultTopK:     5,
		HandlingS2:      "redact",
		HandlingS3:      "block",
		PlaceholderS3:   "[PRIVATE]",
		SessionHeader:   "X-Session-Id",
		MaxAuditEntries: 100,
	}, zap.NewNop())

	p := New(cfg, zap.NewNop(), store, securitySvc)

	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"run the tool"}],"tool_params":{"password":"abc123","path":"./notes.txt"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")
	rec := httptest.NewRecorder()

	p.Forward(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	toolParams := receivedBody["tool_params"].(map[string]interface{})
	if toolParams["password"].(string) != "[REDACTED:PASSWORD]" {
		t.Fatalf("expected top-level tool_params.password to be redacted, got %q", toolParams["password"].(string))
	}
	if toolParams["path"].(string) != "./notes.txt" {
		t.Fatalf("expected non-sensitive path to be preserved, got %q", toolParams["path"].(string))
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

	p := New(cfg, zap.NewNop(), store, nil)

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

	p := New(cfg, zap.NewNop(), store, nil)

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

	p := New(cfg, zap.NewNop(), nil, nil)

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

func TestForwardConvertsAnthropicMessagesToOpenAIChat(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	var upstreamPath string
	var received map[string]interface{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","model":"glm-5","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`))
	}))
	defer upstream.Close()

	cfg := config.DefaultConfig()
	cfg.Provider = "custom"
	cfg.APIKey = "upstream-test-key"
	cfg.CustomBaseURL = upstream.URL
	cfg.CustomCodingURL = upstream.URL
	cfg.UseAnthropic = true

	p := New(cfg, zap.NewNop(), store, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-3-5-sonnet",
		"max_tokens":64,
		"system":"be terse",
		"messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}],
		"tools":[{"name":"lookup","description":"find","input_schema":{"type":"object","properties":{"q":{"type":"string"}}}}],
		"tool_choice":{"type":"tool","name":"lookup"}
	}`))
	rec := httptest.NewRecorder()

	p.Forward(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamPath != "/chat/completions" {
		t.Fatalf("expected upstream /chat/completions, got %q", upstreamPath)
	}
	if received["model"] != "claude-3-5-sonnet" {
		t.Fatalf("expected custom provider to preserve model, got %v", received["model"])
	}
	messages := received["messages"].([]interface{})
	if messages[0].(map[string]interface{})["role"] != "system" || messages[0].(map[string]interface{})["content"] != "be terse" {
		t.Fatalf("expected system message, got %#v", messages[0])
	}
	if messages[1].(map[string]interface{})["content"] != "ping" {
		t.Fatalf("expected user text to be forwarded, got %#v", messages[1])
	}
	tool := received["tools"].([]interface{})[0].(map[string]interface{})
	if tool["type"] != "function" {
		t.Fatalf("expected OpenAI function tool, got %#v", tool)
	}
	choice := received["tool_choice"].(map[string]interface{})
	if choice["type"] != "function" {
		t.Fatalf("expected OpenAI function tool_choice, got %#v", choice)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["type"] != "message" || body["role"] != "assistant" {
		t.Fatalf("expected Anthropic message response, got %#v", body)
	}
	content := body["content"].([]interface{})[0].(map[string]interface{})
	if content["type"] != "text" || content["text"] != "pong" {
		t.Fatalf("expected Anthropic text block, got %#v", content)
	}
	if body["stop_reason"] != "end_turn" {
		t.Fatalf("expected stop_reason=end_turn, got %v", body["stop_reason"])
	}
}

func TestAnthropicClaudeModelMapsToProviderModel(t *testing.T) {
	provider := &config.ProviderConfig{Models: []string{"cheap-model", "coding-model"}}

	if got := mapAnthropicModelForProvider("claude-3-5-sonnet", provider); got != "coding-model" {
		t.Fatalf("expected provider model, got %q", got)
	}
	if got := mapAnthropicModelForProvider("gpt-4", provider); got != "gpt-4" {
		t.Fatalf("expected non-Claude model to be preserved, got %q", got)
	}
}

func TestConvertAnthropicToolBlocksToOpenAIChat(t *testing.T) {
	got := convertAnthropicMessagesToOpenAIChat(map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "using tool"},
					map[string]interface{}{"type": "tool_use", "id": "toolu_1", "name": "read_file", "input": map[string]interface{}{"path": "README.md"}},
				},
			},
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{"type": "tool_result", "tool_use_id": "toolu_1", "content": "done"},
				},
			},
		},
	})

	messages := got["messages"].([]interface{})
	assistant := messages[0].(map[string]interface{})
	toolCalls := assistant["tool_calls"].([]interface{})
	call := toolCalls[0].(map[string]interface{})
	if call["id"] != "toolu_1" {
		t.Fatalf("expected tool call id, got %#v", call)
	}
	functionMap := call["function"].(map[string]interface{})
	if functionMap["name"] != "read_file" || !strings.Contains(functionMap["arguments"].(string), "README.md") {
		t.Fatalf("unexpected function call: %#v", functionMap)
	}
	toolResult := messages[1].(map[string]interface{})
	if toolResult["role"] != "tool" || toolResult["tool_call_id"] != "toolu_1" || toolResult["content"] != "done" {
		t.Fatalf("unexpected tool result message: %#v", toolResult)
	}
}

func TestHandleAnthropicStreamResponseConvertsOpenAIChunks(t *testing.T) {
	cfg := config.DefaultConfig()
	p := &Proxy{cfg: cfg, logger: zap.NewNop()}
	rec := &flushingRecorder{ResponseRecorder: httptest.NewRecorder()}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"id\":\"chatcmpl-1\",\"model\":\"glm-5\",\"choices\":[{\"delta\":{\"content\":\"he\"}}]}\n\n" +
				"data: {\"choices\":[{\"delta\":{\"content\":\"llo\"},\"finish_reason\":\"stop\"}],\"usage\":{\"completion_tokens\":2}}\n\n" +
				"data: [DONE]\n\n",
		)),
	}

	p.handleAnthropicStreamResponseWithStats(rec, resp, time.Now(), http.MethodPost, "/v1/messages", "https://api.example.com/chat/completions", "glm-5", "127.0.0.1", 2, "{}", 0)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, body)
	}
	if !strings.Contains(body, "event: message_start") || !strings.Contains(body, `"text":"he"`) || !strings.Contains(body, `"text":"llo"`) || !strings.Contains(body, "event: message_stop") {
		t.Fatalf("expected Anthropic SSE events, got %s", body)
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
