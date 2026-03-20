package config

import "testing"

func TestGetEffectiveUserAgentSupportsLegacyOpencode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisguiseTool = "opencode"

	if got := cfg.GetEffectiveUserAgent(); got != "opencode/0.3.0 (linux)" {
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
