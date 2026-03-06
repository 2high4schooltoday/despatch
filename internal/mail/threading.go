package mail

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"regexp"
	"strings"
)

const (
	DefaultPreviewMaxChars = 180
)

var (
	threadPrefixPattern   = regexp.MustCompile(`(?i)^(re|fw|fwd)\s*:\s*`)
	previewHTMLTagPattern = regexp.MustCompile(`<[^>]+>`)
)

// NormalizeThreadSubject strips repeated reply/forward prefixes and lowercases
// the remaining text for stable mailbox-scoped thread grouping.
func NormalizeThreadSubject(subject string) string {
	normalized := strings.TrimSpace(strings.ToLower(subject))
	for normalized != "" {
		next := threadPrefixPattern.ReplaceAllString(normalized, "")
		next = strings.TrimSpace(next)
		if next == normalized {
			break
		}
		normalized = next
	}
	return normalized
}

// DeriveThreadID builds a stable mailbox-scoped thread ID from normalized
// subject (or sender fallback when subject is empty).
func DeriveThreadID(mailbox, subject, from string) string {
	scope := strings.ToLower(strings.TrimSpace(mailbox))
	if scope == "" {
		scope = "unknown"
	}
	normalized := NormalizeThreadSubject(subject)
	if normalized == "" {
		normalized = strings.ToLower(strings.TrimSpace(from))
	}
	if normalized == "" {
		normalized = "untitled"
	}
	sum := sha256.Sum256([]byte(scope + "\x00" + normalized))
	return scope + ":" + hex.EncodeToString(sum[:10])
}

// BuildPreviewFromBodySample creates a compact, plain-text snippet from sampled
// message body content.
func BuildPreviewFromBodySample(sample string, max int) string {
	if max <= 0 {
		max = DefaultPreviewMaxChars
	}
	clean := strings.ReplaceAll(sample, "\x00", " ")
	clean = html.UnescapeString(clean)
	if strings.Contains(clean, "<") && strings.Contains(clean, ">") {
		clean = previewHTMLTagPattern.ReplaceAllString(clean, " ")
	}
	compact := strings.Join(strings.Fields(clean), " ")
	if compact == "" {
		return ""
	}
	runes := []rune(compact)
	if len(runes) <= max {
		return compact
	}
	return strings.TrimSpace(string(runes[:max]))
}
