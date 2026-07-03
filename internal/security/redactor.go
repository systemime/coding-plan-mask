package security

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type RedactionOptions struct {
	InternalIP     bool `json:"internal_ip"`
	Email          bool `json:"email"`
	EnvVar         bool `json:"env_var"`
	CreditCard     bool `json:"credit_card"`
	ChinesePhone   bool `json:"chinese_phone"`
	ChineseID      bool `json:"chinese_id"`
	ChineseAddress bool `json:"chinese_address"`
	PIN            bool `json:"pin"`
}

type Replacement struct {
	Original string `json:"original"`
	Tag      string `json:"tag"`
}

type RedactionResult struct {
	RedactedText string            `json:"redacted_text"`
	Mapping      map[string]string `json:"mapping"`
	Replacements []Replacement     `json:"-"`
}

func (r RedactionResult) RestoreToken() string {
	payload := struct {
		Version      int           `json:"v"`
		Replacements []Replacement `json:"replacements"`
		Mapping      interface{}   `json:"mapping,omitempty"`
	}{
		Version:      1,
		Replacements: r.Replacements,
	}
	if len(r.Replacements) == 0 && len(r.Mapping) > 0 {
		payload.Mapping = r.Mapping
	}
	body, _ := json.Marshal(payload)
	return base64.RawURLEncoding.EncodeToString(body)
}

func DecodeRestoreToken(token string) ([]Replacement, map[string]string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid restore_token: %w", err)
	}

	var payload struct {
		Version      int               `json:"v"`
		Replacements []Replacement     `json:"replacements"`
		Mapping      map[string]string `json:"mapping"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("invalid restore_token payload: %w", err)
	}

	return payload.Replacements, payload.Mapping, nil
}

var (
	privateKeyEndPattern = regexp.MustCompile(`-----END (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`)

	phase1Rules = []patternRule{
		{
			Pattern: regexp.MustCompile(
				`-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----[\s\S]*?-----END (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`,
			),
			Tag: "[REDACTED:PRIVATE_KEY]",
		},
		{Pattern: regexp.MustCompile(`\b(?:sk|key|token)-[A-Za-z0-9]{16,}\b`), Tag: "[REDACTED:KEY]"},
		{Pattern: regexp.MustCompile(`\b(?:sk_live|sk_test)_[A-Za-z0-9]{24,}\b`), Tag: "[REDACTED:KEY]"},
		{Pattern: regexp.MustCompile(`\bghp_[A-Za-z0-9]{36,}\b`), Tag: "[REDACTED:KEY]"},
		{Pattern: regexp.MustCompile(`\bgho_[A-Za-z0-9]{36,}\b`), Tag: "[REDACTED:KEY]"},
		{Pattern: regexp.MustCompile(`\bya29\.[A-Za-z0-9_-]{16,}`), Tag: "[REDACTED:KEY]"},
		{Pattern: regexp.MustCompile(`\bxoxb-[A-Za-z0-9-]{10,}`), Tag: "[REDACTED:KEY]"},
		{Pattern: regexp.MustCompile(`\bxoxp-[A-Za-z0-9-]{10,}`), Tag: "[REDACTED:KEY]"},
		{Pattern: regexp.MustCompile(`\bASIA[0-9A-Z]{16}\b`), Tag: "[REDACTED:AWS_KEY]"},
		{Pattern: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), Tag: "[REDACTED:AWS_KEY]"},
		{Pattern: regexp.MustCompile(`(?:mysql|postgres|postgresql|mongodb|redis|amqp)://[^\s"'<>]+`), Tag: "[REDACTED:DB_CONNECTION]"},
		{Pattern: regexp.MustCompile(`(?:快递单号|运单号|取件码)[：:\s]*[A-Za-z0-9]{6,20}`), Tag: "[REDACTED:DELIVERY]"},
		{Pattern: regexp.MustCompile(`(?:门禁码|门禁密码|门锁密码|开门密码)[：:\s]*[A-Za-z0-9#*]{3,12}`), Tag: "[REDACTED:ACCESS_CODE]"},
	}

	optInRules = []optInPatternRule{
		{
			Enabled: func(opts RedactionOptions) bool { return opts.InternalIP },
			Pattern: regexp.MustCompile(`\b(?:10|172\.(?:1[6-9]|2\d|3[01])|192\.168)\.\d{1,3}\.\d{1,3}\b`),
			Tag:     "[REDACTED:INTERNAL_IP]",
		},
		{
			Enabled: func(opts RedactionOptions) bool { return opts.Email },
			Pattern: regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`),
			Tag:     "[REDACTED:EMAIL]",
		},
		{
			Enabled: func(opts RedactionOptions) bool { return opts.EnvVar },
			Pattern: regexp.MustCompile(`(?m)^(?:export\s+)?[A-Z_]{2,}=(?:"|')?[^\s"']+(?:"|')?$`),
			Tag:     "[REDACTED:ENV_VAR]",
		},
		{
			Enabled: func(opts RedactionOptions) bool { return opts.CreditCard },
			Pattern: regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{1,7}\b`),
			Tag:     "[REDACTED:CARD]",
		},
		{
			Enabled: func(opts RedactionOptions) bool { return opts.ChinesePhone },
			Pattern: regexp.MustCompile(`\b1[3-9]\d{9}\b`),
			Tag:     "[REDACTED:PHONE]",
		},
		{
			Enabled: func(opts RedactionOptions) bool { return opts.ChineseID },
			Pattern: regexp.MustCompile(`\b\d{17}[\dXx]\b`),
			Tag:     "[REDACTED:ID]",
		},
		{
			Enabled: func(opts RedactionOptions) bool { return opts.ChineseAddress },
			Pattern: regexp.MustCompile(`[\p{Han}]{2,}(?:省|市|区|县|镇|路|街|巷|弄|号|栋|幢|室|楼|单元|门牌)\d*[\p{Han}\d]*`),
			Tag:     "[REDACTED:ADDRESS]",
		},
	}

	phase2Rules = []contextRule{
		{Pattern: regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|passcode|master[_\s]?password)\b(?:\s+(?:is|are|was|were)(?:\s+(?:in|at|on|of|for)){0,3}\s*|\s*[=:]\s*|\s+)(\S+)`), Tag: "[REDACTED:PASSWORD]"},
		{Pattern: regexp.MustCompile(`(?i)\b(?:credit\s*card|card\s*number)\b(?:\s+(?:is|are|was|were)(?:\s+(?:in|at|on|of|for)){0,3}|\s*[=:])\s*(\S+)`), Tag: "[REDACTED:CARD]"},
		{Pattern: regexp.MustCompile(`(?i)\b(?:api[_\s]?key|access[_\s]?key|secret[_\s]?key)\b(?:\s+(?:is|are|was|were)(?:\s+(?:in|at|on|of|for)){0,3}\s*|\s*[=:]\s*|\s+)(\S+)`), Tag: "[REDACTED:API_KEY]"},
		{Pattern: regexp.MustCompile(`(?i)\bsecret\b(?:\s+(?:is|are|was|were)(?:\s+(?:in|at|on|of|for)){0,3}|\s*[=:])\s*(\S+)`), Tag: "[REDACTED:SECRET]"},
		{Pattern: regexp.MustCompile(`(?i)\b(?:(?:auth[_\s]?)?token|bearer)\b(?:\s+(?:is|are|was|were)(?:\s+(?:in|at|on|of|for)){0,3}\s*|\s*[=:]\s*|\s+)(\S+)`), Tag: "[REDACTED:TOKEN]"},
		{Pattern: regexp.MustCompile(`(?i)\b(?:credential|cred)s?\b(?:\s+(?:is|are|was|were)(?:\s+(?:in|at|on|of|for)){0,3}\s*|\s*[=:]\s*|\s+)(\S+)`), Tag: "[REDACTED:CREDENTIAL]"},
		{Pattern: regexp.MustCompile(`(?i)\b(?:ssn|social\s+security\s+number)\b(?:\s+(?:is|are|was|were)(?:\s+(?:in|at|on|of|for)){0,3}|\s*[=:])\s*(\S+)`), Tag: "[REDACTED:SSN]"},
		{
			Pattern: regexp.MustCompile(`(?i)\b(?:pin|pin\s*code|pin\s*number)\b(?:\s+(?:is|are|was|were)(?:\s+(?:in|at|on|of|for)){0,3}|\s*[=:])\s*(\S+)`),
			Tag:     "[REDACTED:PIN]",
			Enabled: func(opts RedactionOptions) bool { return opts.PIN },
		},
	}

	placeholderPattern = regexp.MustCompile(`\[REDACTED:[A-Z_]+\]`)
	pathTokenPattern   = regexp.MustCompile(`(?:~|\.{1,2}|/)[^\s"'<>]+|[A-Za-z]:\\[^\s"'<>]+`)
)

type patternRule struct {
	Pattern *regexp.Regexp
	Tag     string
}

type optInPatternRule struct {
	Enabled func(RedactionOptions) bool
	Pattern *regexp.Regexp
	Tag     string
}

type contextRule struct {
	Pattern *regexp.Regexp
	Tag     string
	Enabled func(RedactionOptions) bool
}

func Redact(text string, opts RedactionOptions, rules PrivacyRules) (RedactionResult, error) {
	if len(text) > 1024*1024 {
		return RedactionResult{}, fmt.Errorf("input text too long: %d", len(text))
	}

	normalized := normalizeForRedaction(text)
	result := RedactionResult{
		RedactedText: normalized,
		Mapping:      map[string]string{},
	}

	if privateKeyEndPattern.MatchString(result.RedactedText) {
		result.RedactedText = applyPatternRule(result.RedactedText, phase1Rules[0], &result)
	}
	for _, rule := range phase1Rules[1:] {
		result.RedactedText = applyPatternRule(result.RedactedText, rule, &result)
	}
	for _, rule := range optInRules {
		if rule.Enabled != nil && !rule.Enabled(opts) {
			continue
		}
		result.RedactedText = applyPatternRule(result.RedactedText, patternRule{Pattern: rule.Pattern, Tag: rule.Tag}, &result)
	}
	for _, rule := range phase2Rules {
		if rule.Enabled != nil && !rule.Enabled(opts) {
			continue
		}
		result.RedactedText = applyContextRule(result.RedactedText, rule, &result)
	}
	result.RedactedText = redactSensitivePaths(result.RedactedText, rules, &result)

	return result, nil
}

func Restore(text string, mapping map[string]string, replacements []Replacement, htmlEscape bool) string {
	queues := map[string][]string{}
	if len(replacements) > 0 {
		for _, item := range replacements {
			queues[item.Tag] = append(queues[item.Tag], item.Original)
		}
	} else {
		for original, tag := range mapping {
			queues[tag] = append(queues[tag], original)
		}
	}

	return placeholderPattern.ReplaceAllStringFunc(text, func(match string) string {
		values := queues[match]
		if len(values) == 0 {
			return match
		}
		restored := values[0]
		queues[match] = values[1:]
		if htmlEscape {
			return html.EscapeString(restored)
		}
		return restored
	})
}

func normalizeForRedaction(text string) string {
	text = html.UnescapeString(text)
	if decoded, err := url.PathUnescape(text); err == nil {
		text = decoded
	}
	text = norm.NFKC.String(text)
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u200b', '\u200c', '\u200d', '\u200e', '\u200f', '\u00ad', '\u034f', '\u061c', '\u180e', '\u2060', '\u2061', '\u2062', '\u2063', '\u2064', '\u2066', '\u2067', '\u2068', '\u2069', '\u206a', '\u206b', '\u206c', '\u206d', '\u206e', '\u206f', '\ufeff':
			return -1
		default:
			return r
		}
	}, text)
}

func applyPatternRule(text string, rule patternRule, result *RedactionResult) string {
	return rule.Pattern.ReplaceAllStringFunc(text, func(match string) string {
		recordReplacement(result, match, rule.Tag)
		return rule.Tag
	})
}

func applyContextRule(text string, rule contextRule, result *RedactionResult) string {
	return rule.Pattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := rule.Pattern.FindStringSubmatchIndex(match)
		if len(submatches) < 4 {
			return match
		}
		start := submatches[2]
		end := submatches[3]
		if start < 0 || end < 0 || end > len(match) {
			return match
		}

		value := match[start:end]
		if placeholderPattern.MatchString(value) {
			return match
		}

		recordReplacement(result, value, rule.Tag)
		return match[:start] + rule.Tag + match[end:]
	})
}

func redactSensitivePaths(text string, rules PrivacyRules, result *RedactionResult) string {
	return pathTokenPattern.ReplaceAllStringFunc(text, func(match string) string {
		if !LooksLikePath(match) {
			return match
		}
		if matchesPathPatterns(match, rules.ToolsS3.Paths) || matchesPathPatterns(match, rules.ToolsS2.Paths) || hasSensitiveFileMarker(match) {
			recordReplacement(result, match, "[REDACTED:PATH]")
			return "[REDACTED:PATH]"
		}
		return match
	})
}

func recordReplacement(result *RedactionResult, original, tag string) {
	if strings.TrimSpace(original) == "" {
		return
	}
	result.Mapping[original] = tag
	result.Replacements = append(result.Replacements, Replacement{
		Original: original,
		Tag:      tag,
	})
}

func LooksLikePath(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "~/") ||
		strings.HasPrefix(value, "./") ||
		strings.HasPrefix(value, "../") ||
		strings.Contains(value, `\`) ||
		strings.Contains(value, "/")
}

func hasSensitiveFileMarker(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".pem") ||
		strings.HasSuffix(lower, ".key") ||
		strings.HasSuffix(lower, ".p12") ||
		strings.HasSuffix(lower, ".pfx") ||
		strings.HasSuffix(lower, ".env") ||
		strings.Contains(lower, "id_rsa") ||
		strings.Contains(lower, "id_dsa") ||
		strings.Contains(lower, "id_ecdsa") ||
		strings.Contains(lower, "id_ed25519")
}

func sanitizeToken(token string) string {
	if token == "" {
		return token
	}
	var b strings.Builder
	for _, r := range token {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-' {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
