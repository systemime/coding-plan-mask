package security

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(Settings{
		Enabled:         true,
		AuditDir:        t.TempDir(),
		DefaultTrack:    "clean",
		DefaultTopK:     5,
		HandlingS2:      "redact",
		HandlingS3:      "block",
		PlaceholderS3:   "[PRIVATE]",
		SessionHeader:   "X-Session-Id",
		MaxAuditEntries: 100,
		RedactionOptions: RedactionOptions{
			Email: true,
		},
	}, zap.NewNop())
}

func TestRedactTextWithEmailOption(t *testing.T) {
	svc := newTestService(t)

	resp, err := svc.RedactText("Contact me at user@example.com", &RedactionOptions{Email: true})
	if err != nil {
		t.Fatalf("redact text: %v", err)
	}

	if got := resp["redacted_text"]; got != "Contact me at [REDACTED:EMAIL]" {
		t.Fatalf("unexpected redacted text: %v", got)
	}
}

func TestEvaluatePrivacyPolicyBlocksSSHPath(t *testing.T) {
	decision := EvaluatePrivacyPolicy(DetectionContext{
		Message:    "please inspect ~/.ssh/id_rsa",
		ToolParams: map[string]interface{}{"path": "~/.ssh/id_rsa"},
	}, DefaultPrivacyConfig())

	if decision.Level != LevelS3 {
		t.Fatalf("expected S3, got %s", decision.Level)
	}
	if decision.Action != "block" {
		t.Fatalf("expected block, got %s", decision.Action)
	}
}

func TestProcessProxyPayloadRedactsMessagesAndAuditsTracks(t *testing.T) {
	svc := newTestService(t)
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", nil)
	req.Header.Set("X-Session-Id", "session-1")

	payload := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": "my password is abc123",
			},
		},
	}

	updated, decision, err := svc.ProcessProxyPayload(req, payload)
	if err != nil {
		t.Fatalf("process proxy payload: %v", err)
	}
	if decision == nil || decision.Action != "redact" {
		t.Fatalf("expected redact decision, got %+v", decision)
	}

	messages := updated["messages"].([]interface{})
	content := messages[0].(map[string]interface{})["content"].(string)
	if content != "my password is [REDACTED:PASSWORD]" {
		t.Fatalf("unexpected redacted content: %q", content)
	}

	fullTrack, err := svc.runtime.LoadTrack("session-1", "full", 0)
	if err != nil {
		t.Fatalf("load full track: %v", err)
	}
	cleanTrack, err := svc.runtime.LoadTrack("session-1", "clean", 0)
	if err != nil {
		t.Fatalf("load clean track: %v", err)
	}
	if len(fullTrack) != 1 || len(cleanTrack) != 1 {
		t.Fatalf("expected one audit record per track, got full=%d clean=%d", len(fullTrack), len(cleanTrack))
	}
	if fullTrack[0].Content != "my password is abc123" {
		t.Fatalf("unexpected full track content: %q", fullTrack[0].Content)
	}
	if cleanTrack[0].Content != "my password is [REDACTED:PASSWORD]" {
		t.Fatalf("unexpected clean track content: %q", cleanTrack[0].Content)
	}
}

func TestProcessProxyPayloadRedactsJSONToolCallArguments(t *testing.T) {
	svc := newTestService(t)
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", nil)

	payload := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"type": "function",
						"function": map[string]interface{}{
							"name":      "save_note",
							"arguments": `{"password":"abc123","path":"./notes.txt"}`,
						},
					},
				},
			},
		},
	}

	updated, decision, err := svc.ProcessProxyPayload(req, payload)
	if err != nil {
		t.Fatalf("process proxy payload: %v", err)
	}
	if decision == nil || decision.Action != "redact" {
		t.Fatalf("expected redact decision, got %+v", decision)
	}

	messages := updated["messages"].([]interface{})
	toolCall := messages[0].(map[string]interface{})["tool_calls"].([]interface{})[0].(map[string]interface{})
	functionMap := toolCall["function"].(map[string]interface{})
	if got := functionMap["arguments"].(string); got != `{"password":"[REDACTED:PASSWORD]","path":"./notes.txt"}` {
		t.Fatalf("unexpected redacted arguments: %s", got)
	}
}

func TestWriteLoadAndSelectSessionContext(t *testing.T) {
	svc := newTestService(t)

	if _, err := svc.WriteMessage("session-ctx", map[string]interface{}{
		"role":      "user",
		"full_text": "machine learning deployment strategy",
	}); err != nil {
		t.Fatalf("write first message: %v", err)
	}
	if _, err := svc.WriteMessage("session-ctx", map[string]interface{}{
		"role":      "assistant",
		"full_text": "gardening checklist",
	}); err != nil {
		t.Fatalf("write second message: %v", err)
	}

	session, err := svc.LoadSession("session-ctx", "full", 0)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	messages := session["messages"].([]map[string]interface{})
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	selected, err := svc.SelectSessionContext("session-ctx", map[string]interface{}{
		"query": "machine learning",
		"track": "full",
		"top_k": float64(1),
	})
	if err != nil {
		t.Fatalf("select session context: %v", err)
	}
	items := selected["selected"].([]ContextSelection)
	if len(items) != 1 {
		t.Fatalf("expected 1 selected item, got %d", len(items))
	}
	if items[0].Text != "machine learning deployment strategy" {
		t.Fatalf("unexpected selected text: %q", items[0].Text)
	}
}

func TestCustomPrivacyRulesMergeWithDefaults(t *testing.T) {
	svc := NewService(Settings{
		Enabled:       true,
		AuditDir:      t.TempDir(),
		HandlingS2:    "redact",
		HandlingS3:    "block",
		SessionHeader: "X-Session-Id",
		Rules: PrivacyRules{
			KeywordsS2: []string{"project_secret"},
		},
	}, zap.NewNop())

	custom, err := svc.EvaluatePrivacyPolicy(map[string]interface{}{"message": "project_secret"})
	if err != nil {
		t.Fatalf("evaluate custom policy: %v", err)
	}
	if custom["action"] != "redact" {
		t.Fatalf("expected custom rule to redact, got %+v", custom)
	}

	defaultRule, err := svc.EvaluatePrivacyPolicy(map[string]interface{}{"message": "read ~/.ssh/id_rsa"})
	if err != nil {
		t.Fatalf("evaluate default policy: %v", err)
	}
	if defaultRule["action"] != "block" {
		t.Fatalf("expected default S3 rule to remain active, got %+v", defaultRule)
	}
}

func TestInvalidPrivacyPatternDoesNotPanic(t *testing.T) {
	result := DetectSensitivity(DetectionContext{Message: "anything"}, PrivacyConfig{
		Enabled: true,
		Rules: PrivacyRules{
			PatternsS3: []string{"["},
		},
	})

	if result.Level != LevelS1 {
		t.Fatalf("expected invalid pattern to be ignored, got %+v", result)
	}
}

func TestPolicyRequestRejectsInvalidPattern(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.EvaluatePrivacyPolicy(map[string]interface{}{
		"message": "anything",
		"config": map[string]interface{}{
			"rules": map[string]interface{}{
				"patterns_s3": []interface{}{"["},
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid request pattern to return an error")
	}
}
