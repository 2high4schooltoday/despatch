package mail

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"

	gomail "github.com/emersion/go-message/mail"
)

const (
	DefaultPreviewMaxChars = 180
	previewMIMEReadBytes   = 4096
)

var (
	threadPrefixPattern      = regexp.MustCompile(`(?i)^(re|fw|fwd)\s*:\s*`)
	previewHTMLTagPattern    = regexp.MustCompile(`(?is)<[^>]+>`)
	previewHTMLNoisePattern  = regexp.MustCompile(`(?is)<(?:style|script|head|svg|noscript)[^>]*>.*?</(?:style|script|head|svg|noscript)>`)
	previewHeaderLinePattern = regexp.MustCompile(`(?im)^(?:content-[\w-]+|mime-version|content-transfer-encoding|return-path|dkim-signature|received|authentication-results|x-[\w-]+)\s*:[^\n]*$`)
	previewCSSRulePattern    = regexp.MustCompile(`(?is)(?:^|[\s;])(?:@[\w-]+\s+)?[#.\w\[\]\-:,\s>+*()]+\{[^{}]{0,400}\}`)
	previewURLPattern        = regexp.MustCompile(`https?://[^\s<>"']+`)
	previewBase64Token       = regexp.MustCompile(`(?i)^[a-z0-9+/=_-]{24,}$`)
	previewHexToken          = regexp.MustCompile(`(?i)^[a-f0-9]{24,}$`)
	messageIDTokenPattern    = regexp.MustCompile(`<[^>]+>|[^<>,\\s]+@[^<>,\\s]+`)
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

// DeriveThreadID builds a stable conversation-scoped thread ID from normalized
// subject (or sender fallback when subject is empty).
func DeriveThreadID(mailbox, subject, from string) string {
	normalized := NormalizeThreadSubject(subject)
	if normalized == "" {
		normalized = strings.ToLower(strings.TrimSpace(from))
	}
	if normalized == "" {
		normalized = "untitled"
	}
	sum := sha256.Sum256([]byte(normalized))
	return "conv:" + hex.EncodeToString(sum[:10])
}

func NormalizeMessageIDHeader(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	normalized = strings.TrimPrefix(normalized, "<")
	normalized = strings.TrimSuffix(normalized, ">")
	return strings.TrimSpace(normalized)
}

func NormalizeMessageIDHeaders(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := NormalizeMessageIDHeader(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func ParseMessageIDList(raw string) []string {
	items := messageIDTokenPattern.FindAllString(raw, -1)
	if len(items) == 0 {
		return nil
	}
	return NormalizeMessageIDHeaders(items)
}

func FormatMessageIDList(values []string) string {
	normalized := NormalizeMessageIDHeaders(values)
	if len(normalized) == 0 {
		return ""
	}
	return strings.Join(normalized, " ")
}

func DeriveIndexedThreadID(messageID, inReplyTo string, references []string, subject, from string) string {
	root := ""
	normalizedRefs := NormalizeMessageIDHeaders(references)
	switch {
	case len(normalizedRefs) > 0:
		root = normalizedRefs[0]
	case NormalizeMessageIDHeader(inReplyTo) != "":
		root = NormalizeMessageIDHeader(inReplyTo)
	case NormalizeMessageIDHeader(messageID) != "":
		root = NormalizeMessageIDHeader(messageID)
	}
	if root != "" {
		sum := sha256.Sum256([]byte("hdr:" + root))
		return "hdr:" + hex.EncodeToString(sum[:10])
	}
	return DeriveThreadID("", subject, from)
}

// BuildPreviewFromBodySample creates a compact, plain-text snippet from sampled
// message body content.
func BuildPreviewFromBodySample(sample string, max int) string {
	if max <= 0 {
		max = DefaultPreviewMaxChars
	}
	clean := strings.ReplaceAll(sample, "\x00", " ")
	clean = previewHTMLNoisePattern.ReplaceAllString(clean, " ")
	clean = html.UnescapeString(clean)
	clean = previewHeaderLinePattern.ReplaceAllString(clean, " ")
	clean = previewCSSRulePattern.ReplaceAllString(clean, " ")
	if strings.Contains(clean, "<") && strings.Contains(clean, ">") {
		clean = previewHTMLTagPattern.ReplaceAllString(clean, " ")
	}
	clean = previewURLPattern.ReplaceAllStringFunc(clean, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return " "
		}
		host := strings.TrimSpace(parsed.Hostname())
		if host == "" {
			return " "
		}
		return " "
	})
	compact := strings.Join(strings.Fields(clean), " ")
	compact = filterPreviewNoiseTokens(compact)
	if compact == "" {
		return ""
	}
	runes := []rune(compact)
	if len(runes) <= max {
		return compact
	}
	return strings.TrimSpace(string(runes[:max]))
}

func filterPreviewNoiseTokens(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	parts := strings.Fields(input)
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		trimmed := strings.Trim(token, "[](){}<>,;:\"'")
		lower := strings.ToLower(trimmed)
		switch {
		case trimmed == "":
			continue
		case strings.ContainsAny(trimmed, "{}"):
			continue
		case strings.Contains(lower, "border-collapse") || strings.Contains(lower, "font-family") || strings.Contains(lower, "cellpadding") || strings.Contains(lower, "cellspacing"):
			continue
		case previewBase64Token.MatchString(trimmed):
			continue
		case previewHexToken.MatchString(trimmed):
			continue
		case len(trimmed) >= 48 && machinePreviewToken(trimmed):
			continue
		}
		filtered = append(filtered, token)
	}
	return strings.Join(filtered, " ")
}

func machinePreviewToken(token string) bool {
	if token == "" {
		return false
	}
	machineChars := 0
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z':
			machineChars++
		case r >= 'A' && r <= 'Z':
			machineChars++
		case r >= '0' && r <= '9':
			machineChars++
		case r == '+' || r == '/' || r == '=' || r == '_' || r == '-':
			machineChars++
		}
	}
	return machineChars*100/len([]rune(token)) >= 92
}

// BuildPreviewFromMIMERawSample creates a robust snippet from a sampled RFC822
// payload by preferring decoded text/plain and then decoded text/html.
func BuildPreviewFromMIMERawSample(sample []byte, max int) string {
	if len(sample) == 0 {
		return ""
	}
	if max <= 0 {
		max = DefaultPreviewMaxChars
	}

	plain, htmlSnippet := extractPreviewFromMIMEParts(sample)
	if strings.TrimSpace(plain) != "" {
		return BuildPreviewFromBodySample(plain, max)
	}
	if strings.TrimSpace(htmlSnippet) != "" {
		return BuildPreviewFromBodySample(htmlSnippet, max)
	}

	bodySample := sample
	if idx := bytes.Index(sample, []byte("\r\n\r\n")); idx >= 0 {
		bodySample = sample[idx+4:]
	} else if idx := bytes.Index(sample, []byte("\n\n")); idx >= 0 {
		bodySample = sample[idx+2:]
	}
	return BuildPreviewFromBodySample(string(bodySample), max)
}

func extractPreviewFromMIMEParts(raw []byte) (string, string) {
	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return "", ""
	}

	var plain string
	var htmlSnippet string
	for {
		part, nextErr := mr.NextPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			break
		}
		desc := classifyMIMEPart(part.Header)
		switch desc.kind {
		case mimePartTextPlain:
			if plain != "" {
				continue
			}
			body, _ := io.ReadAll(io.LimitReader(part.Body, previewMIMEReadBytes))
			plain = string(body)
		case mimePartTextHTML:
			if htmlSnippet != "" {
				continue
			}
			body, _ := io.ReadAll(io.LimitReader(part.Body, previewMIMEReadBytes))
			htmlSnippet = string(body)
		}
		if plain != "" && htmlSnippet != "" {
			break
		}
	}
	return plain, htmlSnippet
}
