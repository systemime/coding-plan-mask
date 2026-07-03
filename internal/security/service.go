package security

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Settings struct {
	Enabled          bool
	AuditDir         string
	DefaultTrack     string
	DefaultTopK      int
	MaxAuditEntries  int
	HandlingS2       string
	HandlingS3       string
	PlaceholderS3    string
	SessionHeader    string
	RedactionOptions RedactionOptions
	Rules            PrivacyRules
}

type Service struct {
	settings Settings
	logger   *zap.Logger
	runtime  *SessionRuntime
}

type PolicyError struct {
	StatusCode int            `json:"status_code"`
	Decision   PolicyDecision `json:"decision"`
}

func (e *PolicyError) Error() string {
	if e == nil {
		return ""
	}
	return e.Decision.Action + ": " + e.Decision.Reason
}

func NewService(settings Settings, logger *zap.Logger) *Service {
	if strings.TrimSpace(settings.DefaultTrack) == "" {
		settings.DefaultTrack = "clean"
	}
	if settings.DefaultTrack != "full" && settings.DefaultTrack != "clean" {
		settings.DefaultTrack = "clean"
	}
	if settings.DefaultTopK <= 0 {
		settings.DefaultTopK = 5
	}
	if strings.TrimSpace(settings.HandlingS2) == "" {
		settings.HandlingS2 = "redact"
	}
	if strings.TrimSpace(settings.HandlingS3) == "" {
		settings.HandlingS3 = "block"
	}
	if strings.TrimSpace(settings.SessionHeader) == "" {
		settings.SessionHeader = "X-Session-Id"
	}
	if strings.TrimSpace(settings.PlaceholderS3) == "" {
		settings.PlaceholderS3 = "[PRIVATE]"
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		settings: settings,
		logger:   logger,
		runtime:  NewSessionRuntime(settings.AuditDir, settings.MaxAuditEntries),
	}
}

func (s *Service) Enabled() bool {
	return s != nil && s.settings.Enabled
}

func (s *Service) Health() map[string]interface{} {
	sessions, err := s.runtime.ListSessions()
	if err != nil {
		s.logger.Warn("列出安全会话失败", zap.Error(err))
		sessions = nil
	}
	return map[string]interface{}{
		"status":   "ok",
		"data_dir": s.settings.AuditDir,
		"sessions": sessions,
		"enabled":  s.settings.Enabled,
	}
}

func (s *Service) RedactText(text string, options *RedactionOptions) (map[string]interface{}, error) {
	result, err := Redact(text, s.resolveOptions(options), s.privacyConfig().Rules)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"redacted_text": result.RedactedText,
		"mapping":       result.Mapping,
		"restore_token": result.RestoreToken(),
	}, nil
}

func (s *Service) RedactContext(payload map[string]interface{}) (map[string]interface{}, error) {
	options := s.resolveOptionsFromPayload(payload)
	mode, value, err := extractContextPayload(payload)
	if err != nil {
		return nil, err
	}

	if mode == "text" {
		text := value.(string)
		result, err := Redact(text, options, s.privacyConfig().Rules)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"type":             "text",
			"mode":             "text",
			"redacted_context": result.RedactedText,
			"redacted_text":    result.RedactedText,
			"mapping":          result.Mapping,
			"restore_token":    result.RestoreToken(),
			"meta": map[string]interface{}{
				"segments_processed": 1,
				"redacted_values":    len(result.Mapping),
			},
		}, nil
	}

	messages, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("messages must be array")
	}
	redactedMessages, mapping, replacements, processed, err := s.redactMessages(messages, options)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"type":              "messages",
		"mode":              "messages",
		"redacted_context":  redactedMessages,
		"redacted_messages": redactedMessages,
		"mapping":           mapping,
		"restore_token":     encodeRestoreToken(replacements, mapping),
		"meta": map[string]interface{}{
			"segments_processed": processed,
			"redacted_values":    len(mapping),
		},
	}, nil
}

func (s *Service) RestoreContext(payload map[string]interface{}) (map[string]interface{}, error) {
	mode, value, err := extractContextPayload(payload)
	if err != nil {
		return nil, err
	}
	replacements, mapping, err := resolveRestorePayload(payload)
	if err != nil {
		return nil, err
	}
	htmlEscape := false
	if raw, ok := payload["html_escape"].(bool); ok {
		htmlEscape = raw
	}

	if mode == "text" {
		text := value.(string)
		restored := Restore(text, mapping, replacements, htmlEscape)
		return map[string]interface{}{
			"type":             "text",
			"mode":             "text",
			"restored_context": restored,
			"restored_text":    restored,
			"meta": map[string]interface{}{
				"segments_processed": 1,
				"mapping_size":       len(mapping),
			},
		}, nil
	}

	messages := value.([]interface{})
	restored, processed, err := restoreMessages(messages, mapping, replacements, htmlEscape)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"type":              "messages",
		"mode":              "messages",
		"restored_context":  restored,
		"restored_messages": restored,
		"meta": map[string]interface{}{
			"segments_processed": processed,
			"mapping_size":       len(mapping),
		},
	}, nil
}

func (s *Service) DetectPrivacy(payload map[string]interface{}) (map[string]interface{}, error) {
	context, cfg, err := s.privacyRequest(payload)
	if err != nil {
		return nil, err
	}
	result := DetectSensitivity(context, cfg)
	return map[string]interface{}{
		"level":         result.Level,
		"reason":        result.Reason,
		"detector_type": result.DetectorType,
		"confidence":    result.Confidence,
	}, nil
}

func (s *Service) EvaluatePrivacyPolicy(payload map[string]interface{}) (map[string]interface{}, error) {
	context, cfg, err := s.privacyRequest(payload)
	if err != nil {
		return nil, err
	}
	result := EvaluatePrivacyPolicy(context, cfg)
	return map[string]interface{}{
		"level":  result.Level,
		"action": result.Action,
		"reason": result.Reason,
	}, nil
}

func (s *Service) WriteMessage(sessionID string, payload map[string]interface{}) (map[string]interface{}, error) {
	role, ok := payload["role"].(string)
	if !ok || strings.TrimSpace(role) == "" {
		return nil, fmt.Errorf("missing field: role")
	}
	fullText, ok := payload["full_text"].(string)
	if !ok {
		return nil, fmt.Errorf("missing field: full_text")
	}

	var (
		cleanText   *string
		placeholder string
		timestampMS *int64
		metadata    map[string]string
	)

	if raw, ok := payload["clean_text"].(string); ok {
		cleanText = &raw
	}
	if raw, ok := payload["placeholder"].(string); ok {
		placeholder = raw
	}
	if raw, ok := payload["timestamp_ms"].(float64); ok {
		value := int64(raw)
		timestampMS = &value
	}
	metadata = parseMetadata(payload["metadata"])

	if redactMessage, _ := payload["redact"].(bool); redactMessage {
		result, err := Redact(fullText, s.resolveOptionsFromPayload(payload), s.privacyConfig().Rules)
		if err != nil {
			return nil, err
		}
		if cleanText == nil && placeholder == "" {
			clean := result.RedactedText
			cleanText = &clean
		}
		entry, err := s.runtime.Append(sessionID, role, fullText, cleanText, placeholder, timestampMS, metadata)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"entry": map[string]interface{}{
				"role":         entry.Role,
				"full_text":    entry.FullText,
				"clean_text":   entry.CleanText,
				"timestamp_ms": entry.TimestampMS,
				"metadata":     entry.Metadata,
			},
			"mapping":       result.Mapping,
			"redacted_text": result.RedactedText,
		}, nil
	}

	entry, err := s.runtime.Append(sessionID, role, fullText, cleanText, placeholder, timestampMS, metadata)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"entry": map[string]interface{}{
			"role":         entry.Role,
			"full_text":    entry.FullText,
			"clean_text":   entry.CleanText,
			"timestamp_ms": entry.TimestampMS,
			"metadata":     entry.Metadata,
		},
	}, nil
}

func (s *Service) LoadSession(sessionID, track string, limit int) (map[string]interface{}, error) {
	selectedTrack := track
	if strings.TrimSpace(selectedTrack) == "" {
		selectedTrack = s.settings.DefaultTrack
	}
	records, err := s.runtime.LoadTrack(sessionID, selectedTrack, limit)
	if err != nil {
		return nil, err
	}
	messages := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		messages = append(messages, map[string]interface{}{
			"role":         record.Role,
			"content":      record.Content,
			"timestamp_ms": record.TimestampMS,
			"metadata":     record.Metadata,
		})
	}
	return map[string]interface{}{
		"session_id": sessionID,
		"track":      selectedTrack,
		"messages":   messages,
	}, nil
}

func (s *Service) SelectSessionContext(sessionID string, payload map[string]interface{}) (map[string]interface{}, error) {
	query, ok := payload["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("missing field: query")
	}

	track, _ := payload["track"].(string)
	selectedTrack := track
	if strings.TrimSpace(selectedTrack) == "" {
		selectedTrack = s.settings.DefaultTrack
	}

	topK := s.settings.DefaultTopK
	if raw, ok := payload["top_k"].(float64); ok && int(raw) > 0 {
		topK = int(raw)
	}
	minScore := 0.0
	if raw, ok := payload["min_score"].(float64); ok {
		minScore = raw
	}

	records, err := s.runtime.LoadTrack(sessionID, selectedTrack, 0)
	if err != nil {
		return nil, err
	}
	candidates := make([]string, 0, len(records))
	for _, record := range records {
		candidates = append(candidates, record.Content)
	}

	return map[string]interface{}{
		"session_id": sessionID,
		"track":      selectedTrack,
		"query":      query,
		"selected":   SelectRelevantContext(query, candidates, topK, minScore),
	}, nil
}

func (s *Service) ProcessProxyPayload(r *http.Request, payload map[string]interface{}) (map[string]interface{}, *PolicyDecision, error) {
	if !s.settings.Enabled {
		return payload, &PolicyDecision{Level: LevelS1, Action: "allow"}, nil
	}

	nodes := collectPayloadTextNodes(payload)
	if len(nodes) == 0 {
		return payload, &PolicyDecision{Level: LevelS1, Action: "allow"}, nil
	}

	sessionID := s.ResolveSessionID(r, payload)
	privacyCfg := s.privacyConfig()
	metadataBase := map[string]string{
		"path":   r.URL.Path,
		"method": r.Method,
	}

	highest := PolicyDecision{Level: LevelS1, Action: "allow"}
	for _, node := range nodes {
		decision := EvaluatePrivacyPolicy(DetectionContext{Message: node.DetectText, ToolParams: payload}, privacyCfg)
		if levelRank(decision.Level) > levelRank(highest.Level) {
			highest = decision
		} else if decision.Level == highest.Level {
			highest.Action = strongerAction(highest.Action, decision.Action)
		}
	}

	switch highest.Action {
	case "review", "block":
		for _, node := range nodes {
			clean := s.settings.PlaceholderS3
			if highest.Action == "review" {
				clean = "[REVIEW_REQUIRED]"
			}
			metadata := cloneMetadata(metadataBase)
			metadata["decision"] = highest.Action
			metadata["level"] = string(highest.Level)
			metadata["reason"] = highest.Reason
			if _, err := s.runtime.Append(sessionID, node.Role, node.Text, &clean, "", nowMillis(), metadata); err != nil {
				s.logger.Warn("安全审计写入失败", zap.Error(err))
			}
		}
		return payload, &highest, &PolicyError{
			StatusCode: http.StatusForbidden,
			Decision:   highest,
		}
	case "redact":
		for _, node := range nodes {
			redactedText, _, err := s.redactTextNode(node, privacyCfg, true)
			if err != nil {
				return payload, &highest, err
			}
			node.Apply(redactedText)
			metadata := cloneMetadata(metadataBase)
			metadata["decision"] = highest.Action
			metadata["level"] = string(highest.Level)
			if _, err := s.runtime.Append(sessionID, node.Role, node.Text, &redactedText, "", nowMillis(), metadata); err != nil {
				s.logger.Warn("安全审计写入失败", zap.Error(err))
			}
		}
	default:
		redacted := false
		for _, node := range nodes {
			redactedText, replacements, err := s.redactTextNode(node, privacyCfg, false)
			if err != nil {
				return payload, &highest, err
			}
			if replacements > 0 {
				node.Apply(redactedText)
				redacted = true
			} else {
				redactedText = node.Text
			}
			metadata := cloneMetadata(metadataBase)
			metadata["decision"] = "allow"
			if replacements > 0 {
				metadata["decision"] = "redact"
			}
			metadata["level"] = string(highest.Level)
			if _, err := s.runtime.Append(sessionID, node.Role, node.Text, &redactedText, "", nowMillis(), metadata); err != nil {
				s.logger.Warn("安全审计写入失败", zap.Error(err))
			}
		}
		if redacted {
			highest.Action = "redact"
			if highest.Reason == "" {
				highest.Reason = "redaction rule matched"
			}
		}
	}

	return payload, &highest, nil
}

func (s *Service) redactTextNode(node textNode, privacyCfg PrivacyConfig, policyRedact bool) (string, int, error) {
	redactInput := node.Text
	if node.ContextPrefix != "" {
		redactInput = node.ContextPrefix + ": " + node.Text
	}
	options := s.settings.RedactionOptions
	if policyRedact {
		options.InternalIP = true
		options.EnvVar = true
		options.CreditCard = true
	}
	result, err := Redact(redactInput, options, privacyCfg.Rules)
	if err != nil {
		return "", 0, err
	}
	redactedText := result.RedactedText
	if node.ContextPrefix != "" {
		prefix := node.ContextPrefix + ": "
		if strings.HasPrefix(redactedText, prefix) {
			redactedText = strings.TrimPrefix(redactedText, prefix)
		}
	}
	return redactedText, len(result.Replacements), nil
}

func (s *Service) ResolveSessionID(r *http.Request, payload map[string]interface{}) string {
	headers := []string{
		s.settings.SessionHeader,
		"X-Claude-Code-Session-Id",
		"X-Session-Id",
		"x-client-request-id",
	}
	for _, header := range headers {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return safeSessionID(value)
		}
	}
	for _, key := range []string{"session_id", "conversation_id", "thread_id"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return safeSessionID(value)
		}
	}
	return safeSessionID(uuid.NewString())
}

func (s *Service) privacyConfig() PrivacyConfig {
	rules := mergePrivacyRules(DefaultPrivacyRules(), s.settings.Rules)
	return PrivacyConfig{
		Enabled:    s.settings.Enabled,
		HandlingS2: s.settings.HandlingS2,
		HandlingS3: s.settings.HandlingS3,
		Rules:      rules,
	}
}

func mergePrivacyRules(base, override PrivacyRules) PrivacyRules {
	if len(override.KeywordsS2) > 0 {
		base.KeywordsS2 = override.KeywordsS2
	}
	if len(override.KeywordsS3) > 0 {
		base.KeywordsS3 = override.KeywordsS3
	}
	if len(override.PatternsS2) > 0 {
		base.PatternsS2 = override.PatternsS2
	}
	if len(override.PatternsS3) > 0 {
		base.PatternsS3 = override.PatternsS3
	}
	if len(override.ToolsS2.Tools) > 0 {
		base.ToolsS2.Tools = override.ToolsS2.Tools
	}
	if len(override.ToolsS2.Paths) > 0 {
		base.ToolsS2.Paths = override.ToolsS2.Paths
	}
	if len(override.ToolsS3.Tools) > 0 {
		base.ToolsS3.Tools = override.ToolsS3.Tools
	}
	if len(override.ToolsS3.Paths) > 0 {
		base.ToolsS3.Paths = override.ToolsS3.Paths
	}
	return base
}

func (s *Service) privacyRequest(payload map[string]interface{}) (DetectionContext, PrivacyConfig, error) {
	ctx := DetectionContext{}
	if value, ok := payload["message"].(string); ok {
		ctx.Message = value
	}
	if value, ok := payload["tool_name"].(string); ok {
		ctx.ToolName = value
	}
	if value, ok := payload["tool_params"].(map[string]interface{}); ok {
		ctx.ToolParams = value
	}
	if value, ok := payload["tool_result"]; ok {
		ctx.ToolResult = value
	}

	cfg := s.privacyConfig()
	if raw, ok := payload["config"].(map[string]interface{}); ok {
		override, err := parsePrivacyConfig(raw, cfg)
		if err != nil {
			return DetectionContext{}, PrivacyConfig{}, err
		}
		cfg = override
	}
	return ctx, cfg, nil
}

func (s *Service) resolveOptions(options *RedactionOptions) RedactionOptions {
	if options == nil {
		return s.settings.RedactionOptions
	}
	return *options
}

func (s *Service) resolveOptionsFromPayload(payload map[string]interface{}) RedactionOptions {
	options := s.settings.RedactionOptions
	raw, ok := payload["options"].(map[string]interface{})
	if !ok {
		raw, _ = payload["redaction_options"].(map[string]interface{})
	}
	if raw == nil {
		return options
	}
	if value, ok := raw["internal_ip"].(bool); ok {
		options.InternalIP = value
	}
	if value, ok := raw["email"].(bool); ok {
		options.Email = value
	}
	if value, ok := raw["env_var"].(bool); ok {
		options.EnvVar = value
	}
	if value, ok := raw["credit_card"].(bool); ok {
		options.CreditCard = value
	}
	if value, ok := raw["chinese_phone"].(bool); ok {
		options.ChinesePhone = value
	}
	if value, ok := raw["chinese_id"].(bool); ok {
		options.ChineseID = value
	}
	if value, ok := raw["chinese_address"].(bool); ok {
		options.ChineseAddress = value
	}
	if value, ok := raw["pin"].(bool); ok {
		options.PIN = value
	}
	return options
}

type textNode struct {
	Role          string
	Text          string
	DetectText    string
	ContextPrefix string
	Apply         func(string)
}

func collectPayloadTextNodes(payload map[string]interface{}) []textNode {
	nodes := make([]textNode, 0, 8)
	appendStringNode := func(role, text, detectText string, apply func(string)) {
		if strings.TrimSpace(text) == "" {
			return
		}
		if strings.TrimSpace(detectText) == "" {
			detectText = text
		}
		nodes = append(nodes, textNode{
			Role:       role,
			Text:       text,
			DetectText: detectText,
			Apply:      apply,
		})
	}

	if value, ok := payload["system"].(string); ok {
		appendStringNode("system", value, value, func(next string) {
			payload["system"] = next
		})
	}
	if value, ok := payload["system_prompt"].(string); ok {
		appendStringNode("system", value, value, func(next string) {
			payload["system_prompt"] = next
		})
	}
	if value, ok := payload["input"].(string); ok {
		appendStringNode("user", value, value, func(next string) {
			payload["input"] = next
		})
	}
	if value, ok := payload["input"].([]interface{}); ok {
		appendRecursiveStringNodes("user", "", value, func(next interface{}) {
			payload["input"] = next
		}, &nodes)
	}
	if value, ok := payload["input"].(map[string]interface{}); ok {
		appendRecursiveStringNodes("user", "", value, func(next interface{}) {
			payload["input"] = next
		}, &nodes)
	}
	if value, ok := payload["prompt"].(string); ok {
		appendStringNode("user", value, value, func(next string) {
			payload["prompt"] = next
		})
	}
	for _, key := range []string{"tool_params", "parameters", "args"} {
		if value, ok := payload[key]; ok {
			field := key
			appendRecursiveStringNodes("tool", field, value, func(next interface{}) {
				payload[field] = next
			}, &nodes)
		}
	}

	if messages, ok := payload["messages"].([]interface{}); ok {
		for _, item := range messages {
			message, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			role, _ := message["role"].(string)
			switch content := message["content"].(type) {
			case string:
				msgRef := message
				appendStringNode(role, content, content, func(next string) {
					msgRef["content"] = next
				})
			case []interface{}:
				for _, part := range content {
					partMap, ok := part.(map[string]interface{})
					if !ok {
						continue
					}
					if text, ok := partMap["text"].(string); ok {
						partRef := partMap
						appendStringNode(role, text, text, func(next string) {
							partRef["text"] = next
						})
					}
				}
			}
			if toolCalls, ok := message["tool_calls"].([]interface{}); ok {
				for _, item := range toolCalls {
					toolCall, ok := item.(map[string]interface{})
					if !ok {
						continue
					}
					functionMap, ok := toolCall["function"].(map[string]interface{})
					if !ok {
						continue
					}
					if arguments, ok := functionMap["arguments"].(string); ok {
						functionRef := functionMap
						if appendJSONArgumentNodes(role, arguments, func(next string) {
							functionRef["arguments"] = next
						}, &nodes) {
							continue
						}
						appendStringNode(role, arguments, arguments, func(next string) {
							functionRef["arguments"] = next
						})
					}
				}
			}
		}
	}

	return nodes
}

func appendJSONArgumentNodes(role, arguments string, apply func(string), nodes *[]textNode) bool {
	var parsed interface{}
	if err := json.Unmarshal([]byte(arguments), &parsed); err != nil {
		return false
	}
	switch parsed.(type) {
	case map[string]interface{}, []interface{}:
	default:
		return false
	}
	var walk func(path string, value interface{}, set func(interface{}))
	walk = func(path string, value interface{}, set func(interface{})) {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				return
			}
			*nodes = append(*nodes, textNode{
				Role:          role,
				Text:          typed,
				DetectText:    path + ": " + typed,
				ContextPrefix: path,
				Apply: func(next string) {
					set(next)
					if body, err := json.Marshal(parsed); err == nil {
						apply(string(body))
					}
				},
			})
		case []interface{}:
			for i := range typed {
				index := i
				walk(path, typed[index], func(next interface{}) {
					typed[index] = next
				})
			}
		case map[string]interface{}:
			for key, item := range typed {
				field := key
				nextPath := field
				if path != "" {
					nextPath = path + "." + field
				}
				walk(nextPath, item, func(next interface{}) {
					typed[field] = next
				})
			}
		}
	}
	walk("tool_calls.function.arguments", parsed, func(next interface{}) {
		parsed = next
	})
	return true
}

func appendRecursiveStringNodes(role, path string, value interface{}, apply func(interface{}), nodes *[]textNode) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return
		}
		detectText := typed
		if path != "" {
			detectText = path + ": " + typed
		}
		*nodes = append(*nodes, textNode{
			Role:          role,
			Text:          typed,
			DetectText:    detectText,
			ContextPrefix: path,
			Apply: func(next string) {
				apply(next)
			},
		})
	case []interface{}:
		for i := range typed {
			index := i
			appendRecursiveStringNodes(role, path, typed[index], func(next interface{}) {
				typed[index] = next
			}, nodes)
		}
	case map[string]interface{}:
		for key, item := range typed {
			field := key
			nextPath := field
			if path != "" {
				nextPath = path + "." + field
			}
			appendRecursiveStringNodes(role, nextPath, item, func(next interface{}) {
				typed[field] = next
			}, nodes)
		}
	}
}

func parsePrivacyConfig(raw map[string]interface{}, base PrivacyConfig) (PrivacyConfig, error) {
	cfg := base
	if value, ok := raw["enabled"].(bool); ok {
		cfg.Enabled = value
	}
	if value, ok := raw["handling_s2"].(string); ok {
		cfg.HandlingS2 = value
	}
	if value, ok := raw["handling_s3"].(string); ok {
		cfg.HandlingS3 = value
	}

	rulesRaw, _ := raw["rules"].(map[string]interface{})
	if rulesRaw == nil {
		return cfg, nil
	}
	rules := cfg.Rules
	rules.KeywordsS2 = readStringSlice(rulesRaw, "keywords_s2", rules.KeywordsS2)
	rules.KeywordsS3 = readStringSlice(rulesRaw, "keywords_s3", rules.KeywordsS3)
	rules.PatternsS2 = readStringSlice(rulesRaw, "patterns_s2", rules.PatternsS2)
	rules.PatternsS3 = readStringSlice(rulesRaw, "patterns_s3", rules.PatternsS3)
	rules.ToolsS2 = readToolLevel(rulesRaw, "tools_s2", rules.ToolsS2)
	rules.ToolsS3 = readToolLevel(rulesRaw, "tools_s3", rules.ToolsS3)
	if err := validatePrivacyPatterns(rules); err != nil {
		return PrivacyConfig{}, err
	}
	cfg.Rules = rules
	return cfg, nil
}

func validatePrivacyPatterns(rules PrivacyRules) error {
	for _, pattern := range append(append([]string{}, rules.PatternsS2...), rules.PatternsS3...) {
		if _, err := regexp.Compile(`(?i)` + pattern); err != nil {
			return fmt.Errorf("invalid privacy pattern %q: %w", pattern, err)
		}
	}
	return nil
}

func readStringSlice(raw map[string]interface{}, key string, fallback []string) []string {
	value, ok := raw[key].([]interface{})
	if !ok {
		return fallback
	}
	out := make([]string, 0, len(value))
	for _, item := range value {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func readToolLevel(raw map[string]interface{}, key string, fallback ToolRuleLevel) ToolRuleLevel {
	value, ok := raw[key].(map[string]interface{})
	if !ok {
		return fallback
	}
	return ToolRuleLevel{
		Tools: readStringSlice(value, "tools", fallback.Tools),
		Paths: readStringSlice(value, "paths", fallback.Paths),
	}
}

func extractContextPayload(payload map[string]interface{}) (string, interface{}, error) {
	for _, key := range []string{"context", "redacted_context", "redacted_messages", "messages", "redacted_text", "text"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return "text", typed, nil
		case []interface{}:
			return "messages", typed, nil
		default:
			return "", nil, fmt.Errorf("context must be string or list")
		}
	}
	return "", nil, fmt.Errorf("missing field: context")
}

func (s *Service) redactMessages(messages []interface{}, options RedactionOptions) ([]map[string]interface{}, map[string]string, []Replacement, int, error) {
	redacted := make([]map[string]interface{}, 0, len(messages))
	mapping := map[string]string{}
	var replacements []Replacement
	processed := 0

	for _, item := range messages {
		message, ok := item.(map[string]interface{})
		if !ok {
			return nil, nil, nil, 0, fmt.Errorf("each message must be object")
		}
		cloned := cloneMap(message)
		switch content := cloned["content"].(type) {
		case string:
			result, err := Redact(content, options, s.privacyConfig().Rules)
			if err != nil {
				return nil, nil, nil, 0, err
			}
			cloned["content"] = result.RedactedText
			mergeMapping(mapping, result.Mapping)
			replacements = append(replacements, result.Replacements...)
			processed++
		case []interface{}:
			newParts := make([]interface{}, 0, len(content))
			for _, part := range content {
				partMap, ok := part.(map[string]interface{})
				if !ok {
					newParts = append(newParts, part)
					continue
				}
				partClone := cloneMap(partMap)
				if text, ok := partClone["text"].(string); ok {
					result, err := Redact(text, options, s.privacyConfig().Rules)
					if err != nil {
						return nil, nil, nil, 0, err
					}
					partClone["text"] = result.RedactedText
					mergeMapping(mapping, result.Mapping)
					replacements = append(replacements, result.Replacements...)
					processed++
				}
				newParts = append(newParts, partClone)
			}
			cloned["content"] = newParts
		}
		redacted = append(redacted, cloned)
	}

	return redacted, mapping, replacements, processed, nil
}

func restoreMessages(messages []interface{}, mapping map[string]string, replacements []Replacement, htmlEscape bool) ([]map[string]interface{}, int, error) {
	restored := make([]map[string]interface{}, 0, len(messages))
	processed := 0
	for _, item := range messages {
		message, ok := item.(map[string]interface{})
		if !ok {
			return nil, 0, fmt.Errorf("each message must be object")
		}
		cloned := cloneMap(message)
		switch content := cloned["content"].(type) {
		case string:
			cloned["content"] = Restore(content, mapping, replacements, htmlEscape)
			processed++
		case []interface{}:
			newParts := make([]interface{}, 0, len(content))
			for _, part := range content {
				partMap, ok := part.(map[string]interface{})
				if !ok {
					newParts = append(newParts, part)
					continue
				}
				partClone := cloneMap(partMap)
				if text, ok := partClone["text"].(string); ok {
					partClone["text"] = Restore(text, mapping, replacements, htmlEscape)
					processed++
				}
				newParts = append(newParts, partClone)
			}
			cloned["content"] = newParts
		}
		restored = append(restored, cloned)
	}
	return restored, processed, nil
}

func resolveRestorePayload(payload map[string]interface{}) ([]Replacement, map[string]string, error) {
	if raw, ok := payload["restore_token"].(string); ok && strings.TrimSpace(raw) != "" {
		replacements, mapping, err := DecodeRestoreToken(raw)
		if err != nil {
			return nil, nil, err
		}
		return replacements, mapping, nil
	}
	raw, ok := payload["mapping"].(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("missing field: mapping")
	}
	mapping := make(map[string]string, len(raw))
	for key, value := range raw {
		mapping[key] = fmt.Sprint(value)
	}
	return nil, mapping, nil
}

func parseMetadata(raw interface{}) map[string]string {
	value, ok := raw.(map[string]interface{})
	if !ok {
		return map[string]string{}
	}
	out := make(map[string]string, len(value))
	for key, item := range value {
		out[key] = fmt.Sprint(item)
	}
	return out
}

func cloneMap(input map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func mergeMapping(dst, src map[string]string) {
	for key, value := range src {
		dst[key] = value
	}
}

func encodeRestoreToken(replacements []Replacement, mapping map[string]string) string {
	body, _ := json.Marshal(map[string]interface{}{
		"v":            1,
		"replacements": replacements,
		"mapping":      mapping,
	})
	return base64.RawURLEncoding.EncodeToString(body)
}

func strongerAction(current, candidate string) string {
	rank := map[string]int{
		"allow":  1,
		"redact": 2,
		"review": 3,
		"block":  4,
	}
	if rank[candidate] > rank[current] {
		return candidate
	}
	return current
}

func nowMillis() *int64 {
	value := time.Now().UnixMilli()
	return &value
}
