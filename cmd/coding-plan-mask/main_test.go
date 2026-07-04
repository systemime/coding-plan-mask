package main

import (
	"testing"

	"coding-plan-mask/internal/config"
)

func TestCollectDoctorChecksFlagsMissingRequiredKeys(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider = "custom"
	cfg.CustomBaseURL = "https://example.com/v1"
	cfg.Security.Enabled = true

	checks := collectDoctorChecks(cfg)
	status := map[string]string{}
	for _, check := range checks {
		status[check.Name] = check.Status
	}

	if status["api_key"] != "error" {
		t.Fatalf("expected missing api_key error, got %q", status["api_key"])
	}
	if status["local_api_key"] != "error" {
		t.Fatalf("expected missing local_api_key error when privacy is enabled, got %q", status["local_api_key"])
	}
	if status["upstream_url"] != "ok" {
		t.Fatalf("expected custom base_url to satisfy upstream_url, got %q", status["upstream_url"])
	}
}

func TestMaskSecret(t *testing.T) {
	if got := maskSecret("sk-1234567890"); got != "sk-1****7890" {
		t.Fatalf("unexpected masked secret: %q", got)
	}
	if got := maskSecret("short"); got != "****" {
		t.Fatalf("expected short secret to be fully masked, got %q", got)
	}
}
