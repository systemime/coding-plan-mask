package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"coding-plan-mask/internal/config"
	"coding-plan-mask/internal/storage"

	"go.uber.org/zap"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	cfg := config.DefaultConfig()
	return New(cfg, zap.NewNop(), store, "test")
}

func newSecurityTestServer(t *testing.T) *Server {
	t.Helper()
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	cfg := config.DefaultConfig()
	cfg.Security.Enabled = true
	cfg.Security.HandlingS2 = "redact"
	cfg.Security.HandlingS3 = "block"
	cfg.Security.Redaction.Email = true
	cfg.LocalAPIKey = "sk-test"
	return New(cfg, zap.NewNop(), store, "test")
}

func TestRootRouteStillServesLocalInfo(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for root route, got %d", rec.Code)
	}
}

func TestArbitraryProxyRouteIsHandled(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.SetupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/chat/completions", bytes.NewBufferString(`{"model":"glm-5"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatal("expected arbitrary route to be proxied instead of 404")
	}
}

func TestVersionedProxyRouteIsHandled(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatal("expected versioned route to be proxied instead of 404")
	}
}

func TestReadyFailsWhenSecurityEnabledWithoutLocalKey(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "upstream-test-key"
	cfg.Security.Enabled = true
	srv := New(cfg, zap.NewNop(), store, "test")

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	srv.SetupRoutes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 readiness without local key, got %d", rec.Code)
	}
}

func TestPrivacyPolicyRouteReturnsLocalSecurityDecision(t *testing.T) {
	srv := newSecurityTestServer(t)
	handler := srv.SetupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/privacy/policy", bytes.NewBufferString(`{"message":"read ~/.ssh/id_rsa"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for local privacy route, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"action":"block"`)) {
		t.Fatalf("expected block policy response, got %s", rec.Body.String())
	}
}

func TestRedactRouteUsesLocalSecurityService(t *testing.T) {
	srv := newSecurityTestServer(t)
	handler := srv.SetupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/redact", bytes.NewBufferString(`{"text":"Contact me at user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for local redaction route, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`REDACTED`)) {
		t.Fatalf("expected redacted response body, got %s", rec.Body.String())
	}
}

func TestSecurityRouteRequiresLocalKeyWhenSecurityEnabled(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	cfg := config.DefaultConfig()
	cfg.Security.Enabled = true
	srv := New(cfg, zap.NewNop(), store, "test")

	req := httptest.NewRequest(http.MethodPost, "/redact", bytes.NewBufferString(`{"text":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.SetupRoutes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when security is enabled without local key, got %d", rec.Code)
	}
}

func TestSecurityRulesFromConfigOverridesDefaults(t *testing.T) {
	rules := securityRulesFromConfig(config.SecurityRulesConfig{
		KeywordsS2: []string{"project_secret"},
		ToolsS3: config.SecurityToolRuleConfig{
			Paths: []string{"/vault"},
		},
	})

	if len(rules.KeywordsS2) != 1 || rules.KeywordsS2[0] != "project_secret" {
		t.Fatalf("expected custom S2 keyword, got %+v", rules.KeywordsS2)
	}
	if len(rules.ToolsS3.Paths) != 1 || rules.ToolsS3.Paths[0] != "/vault" {
		t.Fatalf("expected custom S3 path, got %+v", rules.ToolsS3.Paths)
	}
	if len(rules.KeywordsS3) == 0 {
		t.Fatal("expected unspecified defaults to stay enabled")
	}
}
