package security

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

type ContextSelection struct {
	Index int     `json:"index"`
	Text  string  `json:"text"`
	Score float64 `json:"score"`
}

var tokenPattern = regexp.MustCompile(`[a-z0-9_]+|[\p{Han}]+|[\p{Hiragana}\p{Katakana}]+|[\p{Hangul}]+`)

func SelectRelevantContext(query string, candidates []string, topK int, minScore float64) []ContextSelection {
	if topK <= 0 {
		topK = 5
	}

	filtered := make([]string, 0, len(candidates))
	originalIndexes := make([]int, 0, len(candidates))
	for i, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		filtered = append(filtered, candidate)
		originalIndexes = append(originalIndexes, i)
	}
	if len(filtered) == 0 {
		return nil
	}

	docTokens := make([][]string, len(filtered))
	docFreq := map[string]int{}
	for i, doc := range filtered {
		tokens := tokenize(doc)
		docTokens[i] = tokens
		seen := map[string]struct{}{}
		for _, token := range tokens {
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			docFreq[token]++
		}
	}

	queryVec := weightedVector(tokenize(query), docFreq, len(filtered))
	queryNorm := vectorNorm(queryVec)
	if queryNorm == 0 {
		return nil
	}

	ranked := make([]ContextSelection, 0, len(filtered))
	for i, tokens := range docTokens {
		docVec := weightedVector(tokens, docFreq, len(filtered))
		score := cosine(queryVec, queryNorm, docVec)
		if score >= minScore {
			ranked = append(ranked, ContextSelection{
				Index: originalIndexes[i],
				Text:  filtered[i],
				Score: score,
			})
		}
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].Index < ranked[j].Index
		}
		return ranked[i].Score > ranked[j].Score
	})

	if len(ranked) > topK {
		ranked = ranked[:topK]
	}
	return ranked
}

func tokenize(text string) []string {
	text = strings.ToLower(norm.NFKC.String(text))
	var tokens []string
	for _, segment := range tokenPattern.FindAllString(text, -1) {
		if containsHan(segment) {
			runes := []rune(segment)
			for _, r := range runes {
				tokens = append(tokens, "u:"+string(r))
			}
			for i := 0; i < len(runes)-1; i++ {
				tokens = append(tokens, "b:"+string(runes[i])+string(runes[i+1]))
			}
			continue
		}
		token := sanitizeToken(segment)
		if token == "" {
			continue
		}
		tokens = append(tokens, "w:"+token)
		for _, ngram := range charNGrams(token, 3, 8) {
			tokens = append(tokens, "g:"+ngram)
		}
	}
	return tokens
}

func weightedVector(tokens []string, docFreq map[string]int, totalDocs int) map[string]float64 {
	vec := map[string]float64{}
	if len(tokens) == 0 {
		return vec
	}

	totalWeight := 0.0
	for _, token := range tokens {
		weight := featureWeight(token)
		totalWeight += weight
		vec[token] += weight
	}
	if totalWeight == 0 {
		return map[string]float64{}
	}

	for token, weight := range vec {
		idf := math.Log((1+float64(totalDocs))/(1+float64(docFreq[token]))) + 1
		vec[token] = (weight / totalWeight) * idf
	}
	return vec
}

func cosine(a map[string]float64, normA float64, b map[string]float64) float64 {
	normB := vectorNorm(b)
	if normA == 0 || normB == 0 {
		return 0
	}
	dot := 0.0
	if len(a) > len(b) {
		a, b = b, a
		normA, normB = normB, normA
	}
	for token, weight := range a {
		dot += weight * b[token]
	}
	return dot / (normA * normB)
}

func vectorNorm(vec map[string]float64) float64 {
	sum := 0.0
	for _, value := range vec {
		sum += value * value
	}
	return math.Sqrt(sum)
}

func containsHan(value string) bool {
	for _, r := range value {
		if r >= '\u3400' && r <= '\u9fff' {
			return true
		}
	}
	return false
}

func charNGrams(token string, size, limit int) []string {
	runes := []rune(token)
	if len(runes) < size {
		return nil
	}
	out := make([]string, 0, limit)
	for i := 0; i <= len(runes)-size && len(out) < limit; i++ {
		out = append(out, string(runes[i:i+size]))
	}
	return out
}

func featureWeight(token string) float64 {
	switch {
	case strings.HasPrefix(token, "w:"), strings.HasPrefix(token, "b:"):
		return 1
	case strings.HasPrefix(token, "u:"):
		return 0.7
	case strings.HasPrefix(token, "g:"):
		return 0.4
	default:
		return 0.5
	}
}
