package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetEffectiveUserAgentSupportsLegacyOpencodeToolID(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisguiseTool = "opencode"

	if got := cfg.GetEffectiveUserAgent(); got != DefaultOpenCodeUserAgent {
		t.Fatalf("expected legacy opencode user agent, got %q", got)
	}
}

func TestGetEffectiveUserAgentFallsBackToClaudeCode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisguiseTool = "unknown-tool"

	if got := cfg.GetEffectiveUserAgent(); got != PredefinedDisguiseTools["claudecode"].UserAgent {
		t.Fatalf("expected Claude Code fallback user agent, got %q", got)
	}
}

func TestGetEffectiveUserAgentUsesClaudeCodeOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisguiseTool = "claudecode"
	cfg.ClaudeCodeUserAgent = "claude-cli/9.9.9 (external, cli)"

	if got := cfg.GetEffectiveUserAgent(); got != cfg.ClaudeCodeUserAgent {
		t.Fatalf("expected Claude Code override user agent, got %q", got)
	}
}

func TestGetDisguiseHeadersAddsXAppForClaudeCode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisguiseTool = "claudecode"

	headers := cfg.GetDisguiseHeaders()
	if got := headers["X-App"]; got != ClaudeCodeAppHeaderValue {
		t.Fatalf("expected X-App disguise header %q, got %q", ClaudeCodeAppHeaderValue, got)
	}
}

func TestGetDisguiseHeadersAddsSessionIdForClaudeCode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisguiseTool = "claudecode"

	headers := cfg.GetDisguiseHeaders()
	sessionID := headers["X-Claude-Code-Session-Id"]
	if sessionID == "" {
		t.Fatal("expected X-Claude-Code-Session-Id header to be set for claudecode mode")
	}
	if len(sessionID) != 36 {
		t.Fatalf("expected UUID format (36 chars), got %q (%d chars)", sessionID, len(sessionID))
	}
}

func TestGetDisguiseHeadersReturnsNilForNonClaudeCode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisguiseTool = "openclaw"

	headers := cfg.GetDisguiseHeaders()
	if headers != nil {
		t.Fatalf("expected nil headers for openclaw, got %v", headers)
	}
}

func TestGetBillingHeader(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisguiseTool = "claudecode"

	header := cfg.GetBillingHeader()
	if !strings.HasPrefix(header, "x-anthropic-billing-header: cc_version=") {
		t.Fatalf("expected billing header prefix, got %q", header)
	}
	if !strings.Contains(header, "cc_entrypoint=cli;") {
		t.Fatalf("expected cc_entrypoint=cli in billing header, got %q", header)
	}
	if !strings.Contains(header, ClaudeCodeVersion+".") {
		t.Fatalf("expected version %s in billing header, got %q", ClaudeCodeVersion, header)
	}
}

func TestGetClientRequestID(t *testing.T) {
	cfg := DefaultConfig()
	id1 := cfg.GetClientRequestID()
	id2 := cfg.GetClientRequestID()

	if id1 == id2 {
		t.Fatal("expected different UUIDs for consecutive calls")
	}
	if len(id1) != 36 {
		t.Fatalf("expected UUID format (36 chars), got %q", id1)
	}
}

func TestGetEffectiveUserAgentUsesOpenClawOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisguiseTool = "openclaw"
	cfg.OpenClawUserAgent = "OpenClaw-Compatible/9.9"

	if got := cfg.GetEffectiveUserAgent(); got != cfg.OpenClawUserAgent {
		t.Fatalf("expected OpenClaw override user agent, got %q", got)
	}
}

func TestGetEffectiveUserAgentUsesOpenCodeOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisguiseTool = "opencode"
	cfg.OpenCodeUserAgent = "opencode/9.9.9 ai-sdk/provider-utils/9.9.9 runtime/bun/9.9.9"

	if got := cfg.GetEffectiveUserAgent(); got != cfg.OpenCodeUserAgent {
		t.Fatalf("expected OpenCode override user agent, got %q", got)
	}
}

func TestFindConfigInDirPrefersConfigToml(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"config.toml", "config.eg", "config.example.toml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	got, ok := findConfigInDir(dir)
	if !ok {
		t.Fatal("expected config file to be found")
	}

	want := filepath.Join(dir, "config.toml")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFindConfigInDirFallsBackToConfigEg(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "config.eg"), []byte("example"), 0644); err != nil {
		t.Fatalf("write config.eg: %v", err)
	}

	got, ok := findConfigInDir(dir)
	if !ok {
		t.Fatal("expected config file to be found")
	}

	want := filepath.Join(dir, "config.eg")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLoadConfigParsesSecuritySettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[security]
enabled = true
audit_dir = "/tmp/security-audit"
default_track = "full"
default_top_k = 7
max_audit_items = 99
handling_s2 = "review"
handling_s3 = "redact"
placeholder_s3 = "[MASKED]"
session_header = "X-Custom-Session"

[security.redaction]
email = true
pin = true

[security.rules]
keywords_s2 = ["project_secret"]
patterns_s3 = ["TOPSECRET-[0-9]+"]

[security.rules.tools_s3]
paths = ["/vault"]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !cfg.Security.Enabled {
		t.Fatal("expected security to be enabled")
	}
	if cfg.Security.AuditDir != "/tmp/security-audit" {
		t.Fatalf("expected security audit dir to be parsed, got %q", cfg.Security.AuditDir)
	}
	if cfg.Security.DefaultTrack != "full" {
		t.Fatalf("expected default_track=full, got %q", cfg.Security.DefaultTrack)
	}
	if cfg.Security.DefaultTopK != 7 {
		t.Fatalf("expected default_top_k=7, got %d", cfg.Security.DefaultTopK)
	}
	if cfg.Security.MaxAuditItems != 99 {
		t.Fatalf("expected max_audit_items=99, got %d", cfg.Security.MaxAuditItems)
	}
	if cfg.Security.HandlingS2 != "review" || cfg.Security.HandlingS3 != "redact" {
		t.Fatalf("unexpected handling actions: s2=%q s3=%q", cfg.Security.HandlingS2, cfg.Security.HandlingS3)
	}
	if cfg.Security.PlaceholderS3 != "[MASKED]" {
		t.Fatalf("expected placeholder_s3 override, got %q", cfg.Security.PlaceholderS3)
	}
	if cfg.Security.SessionHeader != "X-Custom-Session" {
		t.Fatalf("expected custom session header, got %q", cfg.Security.SessionHeader)
	}
	if !cfg.Security.Redaction.Email || !cfg.Security.Redaction.PIN {
		t.Fatalf("expected redaction flags to be parsed, got %+v", cfg.Security.Redaction)
	}
	if got := cfg.Security.Rules.KeywordsS2; len(got) != 1 || got[0] != "project_secret" {
		t.Fatalf("expected custom S2 keyword, got %+v", got)
	}
	if got := cfg.Security.Rules.PatternsS3; len(got) != 1 || got[0] != "TOPSECRET-[0-9]+" {
		t.Fatalf("expected custom S3 pattern, got %+v", got)
	}
	if got := cfg.Security.Rules.ToolsS3.Paths; len(got) != 1 || got[0] != "/vault" {
		t.Fatalf("expected custom S3 path, got %+v", got)
	}
}

func TestLoadConfigKeepsDefaultSecurityRedactionWhenSectionMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[security]\nenabled = true\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !cfg.Security.Redaction.Email || !cfg.Security.Redaction.ChinesePhone || !cfg.Security.Redaction.ChineseID {
		t.Fatalf("expected default PII redaction options, got %+v", cfg.Security.Redaction)
	}

	if err := os.WriteFile(path, []byte("[security.redaction]\nemail = false\n"), 0644); err != nil {
		t.Fatalf("write override config: %v", err)
	}
	cfg, err = LoadConfig(path)
	if err != nil {
		t.Fatalf("load override config: %v", err)
	}
	if cfg.Security.Redaction.Email {
		t.Fatalf("expected explicit email=false to override default, got %+v", cfg.Security.Redaction)
	}
}

func TestLoadConfigRejectsInvalidSecurityPattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[security.rules]
patterns_s3 = ["["]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected invalid security pattern to fail config load")
	}
}
