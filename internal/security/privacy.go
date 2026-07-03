package security

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type SensitivityLevel string

const (
	LevelS1 SensitivityLevel = "S1"
	LevelS2 SensitivityLevel = "S2"
	LevelS3 SensitivityLevel = "S3"
)

type ToolRuleLevel struct {
	Tools []string `json:"tools"`
	Paths []string `json:"paths"`
}

type PrivacyRules struct {
	KeywordsS2 []string      `json:"keywords_s2"`
	KeywordsS3 []string      `json:"keywords_s3"`
	PatternsS2 []string      `json:"patterns_s2"`
	PatternsS3 []string      `json:"patterns_s3"`
	ToolsS2    ToolRuleLevel `json:"tools_s2"`
	ToolsS3    ToolRuleLevel `json:"tools_s3"`
}

type PrivacyConfig struct {
	Enabled    bool         `json:"enabled"`
	HandlingS2 string       `json:"handling_s2"`
	HandlingS3 string       `json:"handling_s3"`
	Rules      PrivacyRules `json:"rules"`
}

type DetectionContext struct {
	Message    string                 `json:"message"`
	ToolName   string                 `json:"tool_name"`
	ToolParams map[string]interface{} `json:"tool_params"`
	ToolResult interface{}            `json:"tool_result"`
}

type DetectionResult struct {
	Level        SensitivityLevel `json:"level"`
	Reason       string           `json:"reason,omitempty"`
	DetectorType string           `json:"detector_type"`
	Confidence   float64          `json:"confidence"`
}

type PolicyDecision struct {
	Level  SensitivityLevel `json:"level"`
	Action string           `json:"action"`
	Reason string           `json:"reason,omitempty"`
}

func DefaultPrivacyRules() PrivacyRules {
	return PrivacyRules{
		KeywordsS2: []string{"password", "api_key", "secret", "token", "credential", "auth_token"},
		KeywordsS3: []string{"ssh", "id_rsa", "private_key", ".pem", ".key", ".env", "master_password"},
		PatternsS2: []string{
			`\b(?:10|172\.(?:1[6-9]|2\d|3[01])|192\.168)\.\d{1,3}\.\d{1,3}\b`,
			`(?:mysql|postgres|postgresql|mongodb|redis|amqp)://[^\s"']+`,
			`\b(?:sk|key|token)-[A-Za-z0-9]{16,}\b`,
		},
		PatternsS3: []string{
			`-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`,
			`AKIA[0-9A-Z]{16}`,
			`ASIA[0-9A-Z]{16}`,
		},
		ToolsS2: ToolRuleLevel{
			Tools: []string{"exec", "shell"},
			Paths: []string{"~/secrets", "~/private"},
		},
		ToolsS3: ToolRuleLevel{
			Tools: []string{"system.run", "sudo", "ssh"},
			Paths: []string{"~/.ssh", "/etc", "~/.aws", "~/.config/credentials", "/root"},
		},
	}
}

func DefaultPrivacyConfig() PrivacyConfig {
	return PrivacyConfig{
		Enabled:    true,
		HandlingS2: "redact",
		HandlingS3: "block",
		Rules:      DefaultPrivacyRules(),
	}
}

func DetectSensitivity(ctx DetectionContext, cfg PrivacyConfig) DetectionResult {
	if !cfg.Enabled {
		return DetectionResult{
			Level:        LevelS1,
			Reason:       "privacy detection disabled",
			DetectorType: "ruleDetector",
			Confidence:   1,
		}
	}

	results := make([]DetectionResult, 0, 6)
	if strings.TrimSpace(ctx.Message) != "" {
		results = append(results, checkKeywords(ctx.Message, cfg.Rules, ""))
		results = append(results, checkPatterns(ctx.Message, cfg.Rules, ""))
	}
	if strings.TrimSpace(ctx.ToolName) != "" {
		results = append(results, checkToolName(ctx.ToolName, cfg.Rules))
	}
	if len(ctx.ToolParams) > 0 {
		results = append(results, checkToolParams(ctx.ToolParams, cfg.Rules))
	}
	if ctx.ToolResult != nil {
		resultText := stringifyToolResult(ctx.ToolResult)
		results = append(results, checkKeywords(resultText, cfg.Rules, "Result: "))
		results = append(results, checkPatterns(resultText, cfg.Rules, "Result: "))
	}

	var detected []DetectionResult
	for _, item := range results {
		if item.Level != LevelS1 {
			detected = append(detected, item)
		}
	}
	if len(detected) == 0 {
		return DetectionResult{
			Level:        LevelS1,
			DetectorType: "ruleDetector",
			Confidence:   1,
		}
	}

	final := detected[0]
	reasons := make([]string, 0, len(detected))
	for _, item := range detected {
		if levelRank(item.Level) > levelRank(final.Level) {
			final = item
		}
		if item.Reason != "" {
			reasons = append(reasons, item.Reason)
		}
	}
	final.Reason = strings.Join(reasons, "; ")
	final.DetectorType = "ruleDetector"
	final.Confidence = 1
	return final
}

func EvaluatePrivacyPolicy(ctx DetectionContext, cfg PrivacyConfig) PolicyDecision {
	detected := DetectSensitivity(ctx, cfg)
	switch detected.Level {
	case LevelS3:
		return PolicyDecision{
			Level:  detected.Level,
			Action: validateAction(cfg.HandlingS3),
			Reason: detected.Reason,
		}
	case LevelS2:
		return PolicyDecision{
			Level:  detected.Level,
			Action: validateAction(cfg.HandlingS2),
			Reason: detected.Reason,
		}
	default:
		return PolicyDecision{
			Level:  LevelS1,
			Action: "allow",
			Reason: detected.Reason,
		}
	}
}

func ExtractPathsFromParams(value interface{}) []string {
	var paths []string
	var walk func(node interface{})
	walk = func(node interface{}) {
		switch v := node.(type) {
		case string:
			if LooksLikePath(v) {
				paths = append(paths, v)
			}
		case map[string]interface{}:
			for _, item := range v {
				walk(item)
			}
		case []interface{}:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(value)
	return paths
}

func checkKeywords(text string, rules PrivacyRules, prefix string) DetectionResult {
	for _, keyword := range rules.KeywordsS3 {
		if containsKeyword(text, keyword) {
			return DetectionResult{Level: LevelS3, Reason: prefix + "S3 keyword detected: " + keyword}
		}
	}
	for _, keyword := range rules.KeywordsS2 {
		if containsKeyword(text, keyword) {
			return DetectionResult{Level: LevelS2, Reason: prefix + "S2 keyword detected: " + keyword}
		}
	}
	return DetectionResult{Level: LevelS1}
}

func checkPatterns(text string, rules PrivacyRules, prefix string) DetectionResult {
	for _, pattern := range rules.PatternsS3 {
		if patternMatches(text, pattern) {
			return DetectionResult{Level: LevelS3, Reason: prefix + "S3 pattern matched: " + pattern}
		}
	}
	for _, pattern := range rules.PatternsS2 {
		if patternMatches(text, pattern) {
			return DetectionResult{Level: LevelS2, Reason: prefix + "S2 pattern matched: " + pattern}
		}
	}
	return DetectionResult{Level: LevelS1}
}

func checkToolName(toolName string, rules PrivacyRules) DetectionResult {
	normalized := strings.ToLower(strings.TrimSpace(toolName))
	for _, tool := range rules.ToolsS3.Tools {
		if toolNameMatches(normalized, tool) {
			return DetectionResult{Level: LevelS3, Reason: "S3 tool detected: " + toolName}
		}
	}
	for _, tool := range rules.ToolsS2.Tools {
		if toolNameMatches(normalized, tool) {
			return DetectionResult{Level: LevelS2, Reason: "S2 tool detected: " + toolName}
		}
	}
	return DetectionResult{Level: LevelS1}
}

func checkToolParams(params map[string]interface{}, rules PrivacyRules) DetectionResult {
	paths := ExtractPathsFromParams(params)
	for _, path := range paths {
		if matchesPathPatterns(path, rules.ToolsS3.Paths) {
			return DetectionResult{Level: LevelS3, Reason: "S3 path detected: " + path}
		}
	}
	for _, path := range paths {
		if matchesPathPatterns(path, rules.ToolsS2.Paths) {
			return DetectionResult{Level: LevelS2, Reason: "S2 path detected: " + path}
		}
	}
	for _, path := range paths {
		if hasSensitiveFileMarker(path) {
			return DetectionResult{Level: LevelS3, Reason: "Sensitive file extension detected: " + path}
		}
	}
	return DetectionResult{Level: LevelS1}
}

func stringifyToolResult(value interface{}) string {
	if raw, err := json.Marshal(value); err == nil {
		return string(raw)
	}
	return fmt.Sprint(value)
}

func patternMatches(text, pattern string) bool {
	re, err := regexp.Compile(`(?i)` + pattern)
	return err == nil && re.MatchString(text)
}

func toolNameMatches(name, segment string) bool {
	segment = strings.ToLower(strings.TrimSpace(segment))
	if name == segment {
		return true
	}
	return regexp.MustCompile(`(?:^|[._-])` + regexp.QuoteMeta(segment) + `(?:$|[._-])`).MatchString(name)
}

func matchesPathPatterns(path string, patterns []string) bool {
	lowerPath := strings.ToLower(path)
	for _, pattern := range patterns {
		candidate := strings.ToLower(pattern)
		if strings.Contains(lowerPath, candidate) {
			return true
		}
		if ok, err := filepath.Match(candidate, lowerPath); err == nil && ok {
			return true
		}
	}
	return false
}

func validateAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "allow":
		return "allow"
	case "redact":
		return "redact"
	case "review":
		return "review"
	case "block":
		return "block"
	default:
		return "block"
	}
}

func levelRank(level SensitivityLevel) int {
	switch level {
	case LevelS3:
		return 3
	case LevelS2:
		return 2
	default:
		return 1
	}
}

func containsKeyword(text, keyword string) bool {
	lowerText := strings.ToLower(text)
	lowerKeyword := strings.ToLower(keyword)
	start := 0
	for {
		index := strings.Index(lowerText[start:], lowerKeyword)
		if index < 0 {
			return false
		}
		index += start
		beforeOK := index == 0 || !isAlphaNum(rune(lowerText[index-1]))
		afterIndex := index + len(lowerKeyword)
		afterOK := afterIndex >= len(lowerText) || !isAlphaNum(rune(lowerText[afterIndex]))
		if strings.HasPrefix(keyword, ".") {
			beforeOK = true
		}
		if beforeOK && afterOK {
			return true
		}
		start = index + 1
	}
}

func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
