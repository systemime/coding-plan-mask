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
