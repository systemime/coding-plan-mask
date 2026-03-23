package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
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

	p.handleStreamResponseWithStats(recorder, resp, time.Now(), http.MethodPost, "/chat/completions", "glm-4-flash", "127.0.0.1", 2, "{}")

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
