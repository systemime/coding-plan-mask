package config

import (
	"os"
	"path/filepath"
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
