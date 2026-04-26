package mail

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"html"
	"io"
	stdmail "net/mail"
	"net/textproto"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"

	gomail "github.com/emersion/go-message/mail"
)

const (
	DefaultPreviewMaxChars = 180
	previewMIMEReadBytes   = 4096
)

var (
	threadPrefixPattern      = regexp.MustCompile(`(?i)^(?:(?:re|aw|sv|wg|fw|fwd)\s*(?:\[[0-9]+\])?\s*:|(?:回复|答复|转发)\s*:)\s*`)
	threadListTagPattern     = regexp.MustCompile(`^(?:\[[^\[\]]{1,40}\]\s*)+`)
	threadTrailerPattern     = regexp.MustCompile(`(?i)\s*\((?:fwd|fw)\)\s*$`)
	threadReplyLikePattern   = regexp.MustCompile(`(?i)^\s*(?:\[[^\[\]]{1,40}\]\s*)*(?:(?:re|aw|sv|wg|fw|fwd)\s*(?:\[[0-9]+\])?\s*:|(?:回复|答复|转发)\s*:)|\((?:fwd|fw)\)\s*$`)
	previewHTMLTagPattern    = regexp.MustCompile(`(?is)<[^>]+>`)
	previewHTMLNoisePattern  = regexp.MustCompile(`(?is)<(?:style|script|head|svg|noscript)[^>]*>.*?</(?:style|script|head|svg|noscript)>`)
	previewHeaderLinePattern = regexp.MustCompile(`(?im)^(?:content-[\w-]+|mime-version|content-transfer-encoding|return-path|dkim-signature|received|authentication-results|x-[\w-]+)\s*:[^\n]*$`)
	previewCSSRulePattern    = regexp.MustCompile(`(?is)(?:^|[\s;])(?:@[\w-]+\s+)?[#.\w\[\]\-:,\s>+*()]+\{[^{}]{0,400}\}`)
	previewURLPattern        = regexp.MustCompile(`https?://[^\s<>"']+`)
	previewBase64Token       = regexp.MustCompile(`(?i)^[a-z0-9+/=_-]{24,}$`)
	previewHexToken          = regexp.MustCompile(`(?i)^[a-f0-9]{24,}$`)
	previewInlineNoiseStart  = regexp.MustCompile(`(?i)\s+(?:@(?:font-face|media|supports|import|page|keyframes|charset|viewport|counter-style|property|layer)\b|(?:font-family|font-size|font-weight|font-style|font-display|line-height|letter-spacing|border-collapse|border-spacing|background(?:-color|-image)?|color|display|src|unicode-range|cellpadding|cellspacing|mso-[\w-]+)\b\s*[:=]|mime-version\s*:|content-type\s*:|content-transfer-encoding\s*:|quoted-printable\b|multipart/|text/html\b|charset=)`)
	previewResidualNoise     = regexp.MustCompile(`(?i)(?:@font-face|@media|@supports|@import|\b(?:border-collapse|border-spacing|font-family|font-size|font-weight|font-style|font-display|line-height|letter-spacing|cellpadding|cellspacing|mso-[\w-]*|mime-version|content-transfer-encoding|content-type|return-path|dkim-signature|authentication-results|multipart/|text/html|charset=|quoted-printable|unicode-range)\b|src\s*:)`)
	messageIDTokenPattern    = regexp.MustCompile(`<[^>]+>|[^<>,[:space:]]+@[^<>,[:space:]]+`)
)

// NormalizeThreadSubject strips repeated reply/forward prefixes and lowercases
// the remaining text for stable mailbox-scoped thread grouping.
func NormalizeThreadSubject(subject string) string {
	normalized := strings.TrimSpace(strings.ToLower(subject))
	for normalized != "" {
		next := strings.TrimSpace(threadListTagPattern.ReplaceAllString(normalized, ""))
		next = strings.TrimSpace(threadPrefixPattern.ReplaceAllString(next, ""))
		next = strings.TrimSpace(threadTrailerPattern.ReplaceAllString(next, ""))
		if next == normalized {
			break
		}
		normalized = next
	}
	normalized = strings.Join(strings.Fields(normalized), " ")
	return normalized
}

func ThreadSubjectLooksReply(subject string) bool {
	return threadReplyLikePattern.MatchString(strings.TrimSpace(subject))
}

func NormalizeThreadParticipants(values ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if parsed, err := stdmail.ParseAddressList(raw); err == nil && len(parsed) > 0 {
			for _, addr := range parsed {
				if addr == nil {
					continue
				}
				email := strings.ToLower(strings.TrimSpace(addr.Address))
				if email == "" {
					continue
				}
				if _, ok := seen[email]; ok {
					continue
				}
				seen[email] = struct{}{}
				out = append(out, email)
			}
			return
		}
		if parsed, err := stdmail.ParseAddress(raw); err == nil && parsed != nil {
			email := strings.ToLower(strings.TrimSpace(parsed.Address))
			if email != "" {
				if _, ok := seen[email]; !ok {
					seen[email] = struct{}{}
					out = append(out, email)
				}
			}
			return
		}
		for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
			switch r {
			case ',', ';':
				return true
			default:
				return false
			}
		}) {
			email := strings.ToLower(strings.TrimSpace(strings.Trim(token, ` <>"'`)))
			if email == "" || !strings.Contains(email, "@") {
				continue
			}
			if _, ok := seen[email]; ok {
				continue
			}
			seen[email] = struct{}{}
			out = append(out, email)
		}
	}
	for _, value := range values {
		add(value)
	}
	sort.Strings(out)
	return out
}

func ThreadParticipantsOverlap(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(a))
	for _, item := range NormalizeThreadParticipants(strings.Join(a, ", ")) {
		seen[item] = struct{}{}
	}
	count := 0
	for _, item := range NormalizeThreadParticipants(strings.Join(b, ", ")) {
		if _, ok := seen[item]; ok {
			count++
		}
	}
	return count
}

func NormalizeConversationIndexRoot(raw string) string {
	value := strings.Join(strings.Fields(strings.TrimSpace(raw)), "")
	if value == "" {
		return ""
	}
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) >= 22 {
		return hex.EncodeToString(decoded[:22])
	}
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		decoded, err := encoding.DecodeString(value)
		if err != nil || len(decoded) < 22 {
			continue
		}
		return hex.EncodeToString(decoded[:22])
	}
	return ""
}

func ParseThreadingHeaderFields(raw []byte) (textproto.MIMEHeader, error) {
	header, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw))).ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	return header, nil
}

func normalizeMessageIDToken(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	normalized = strings.TrimPrefix(normalized, "<")
	normalized = strings.TrimSuffix(normalized, ">")
	return strings.TrimSpace(normalized)
}

func NormalizeThreadTopic(value string) string {
	return NormalizeThreadSubject(DecodeHeaderText(value))
}

func deriveHintThreadID(subject string, participants []string, listID string) string {
	baseSubject := NormalizeThreadSubject(subject)
	if baseSubject == "" {
		baseSubject = "untitled"
	}
	normalizedParticipants := NormalizeThreadParticipants(strings.Join(participants, ", "))
	payload := "hint:" + baseSubject
	if len(normalizedParticipants) > 0 {
		payload += "|" + strings.Join(normalizedParticipants, ",")
	}
	listID = strings.ToLower(strings.TrimSpace(listID))
	if listID != "" {
		payload += "|list:" + listID
	}
	sum := sha256.Sum256([]byte(payload))
	return "hint:" + hex.EncodeToString(sum[:10])
}

func DeriveIndexedThreadIDWithHints(messageID, inReplyTo string, references []string, subject, from string, participants []string, threadTopic, conversationIndexRoot, listID string) string {
	if root := NormalizeConversationIndexRoot(conversationIndexRoot); root != "" {
		sum := sha256.Sum256([]byte("cidx:" + root))
		return "cidx:" + hex.EncodeToString(sum[:10])
	}
	normalizedRefs := NormalizeMessageIDHeaders(references)
	normalizedInReplyTo := NormalizeMessageIDHeader(inReplyTo)
	normalizedMessageID := NormalizeMessageIDHeader(messageID)
	switch {
	case len(normalizedRefs) > 0:
		sum := sha256.Sum256([]byte("hdr:" + normalizedRefs[0]))
		return "hdr:" + hex.EncodeToString(sum[:10])
	case normalizedInReplyTo != "":
		sum := sha256.Sum256([]byte("hdr:" + normalizedInReplyTo))
		return "hdr:" + hex.EncodeToString(sum[:10])
	}
	hintSubject := firstNonEmptyThreadValue(NormalizeThreadTopic(threadTopic), NormalizeThreadSubject(subject))
	if hintSubject != "" && (ThreadSubjectLooksReply(subject) || strings.TrimSpace(threadTopic) != "" || strings.TrimSpace(listID) != "") {
		return deriveHintThreadID(hintSubject, append([]string{from}, participants...), listID)
	}
	if normalizedMessageID != "" {
		sum := sha256.Sum256([]byte("hdr:" + normalizedMessageID))
		return "hdr:" + hex.EncodeToString(sum[:10])
	}
	return deriveHintThreadID(subject, append([]string{from}, participants...), listID)
}

func firstNonEmptyThreadValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
	if token := strings.TrimSpace(messageIDTokenPattern.FindString(strings.TrimSpace(value))); token != "" {
		return normalizeMessageIDToken(token)
	}
	return normalizeMessageIDToken(value)
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

func BuildReplyHeaders(parentMessageID, parentInReplyTo string, parentReferences []string) (string, []string) {
	inReplyTo := NormalizeMessageIDHeader(parentMessageID)
	references := NormalizeMessageIDHeaders(parentReferences)
	if len(references) == 0 {
		if seeded := NormalizeMessageIDHeader(parentInReplyTo); seeded != "" {
			references = append(references, seeded)
		}
	}
	if inReplyTo != "" {
		references = append(references, inReplyTo)
	}
	return inReplyTo, NormalizeMessageIDHeaders(references)
}

func DeriveIndexedThreadID(messageID, inReplyTo string, references []string, subject, from string) string {
	return DeriveIndexedThreadIDWithHints(messageID, inReplyTo, references, subject, from, nil, "", "", "")
}

func DeriveLiveThreadID(mailbox, messageID, inReplyTo string, references []string, subject, from string) string {
	if NormalizeMessageIDHeader(messageID) == "" &&
		NormalizeMessageIDHeader(inReplyTo) == "" &&
		len(NormalizeMessageIDHeaders(references)) == 0 {
		return DeriveThreadID(mailbox, subject, from)
	}
	return DeriveIndexedThreadID(messageID, inReplyTo, references, subject, from)
}

func PopulateLiveMessageThreadID(msg *Message) {
	if msg == nil {
		return
	}
	msg.ThreadID = DeriveIndexedThreadIDWithHints(
		msg.MessageID,
		msg.InReplyTo,
		msg.References,
		msg.Subject,
		msg.From,
		append(append([]string{}, msg.To...), append(msg.CC, msg.BCC...)...),
		msg.ThreadTopic,
		msg.ThreadIndex,
		msg.ListID,
	)
}

// BuildPreviewFromBodySample creates a compact, plain-text snippet from sampled
// message body content.
func BuildPreviewFromBodySample(sample string, max int) string {
	if max <= 0 {
		max = DefaultPreviewMaxChars
	}
	clean := strings.ReplaceAll(sample, "\x00", " ")
	clean = strings.ReplaceAll(clean, "\r\n", "\n")
	clean = strings.ReplaceAll(clean, "\r", "\n")
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
	clean = filterPreviewNoiseLines(clean)
	compact := strings.Join(strings.Fields(clean), " ")
	compact = stripInlinePreviewNoise(compact)
	compact = filterPreviewNoiseTokens(compact)
	compact = stripInlinePreviewNoise(compact)
	if compact == "" {
		return ""
	}
	runes := []rune(compact)
	if len(runes) <= max {
		return compact
	}
	return strings.TrimSpace(string(runes[:max]))
}

func normalizePreviewCandidate(sample string, max int) string {
	candidate := BuildPreviewFromBodySample(sample, max)
	if !previewAppearsUseful(candidate) {
		return ""
	}
	return candidate
}

func previewAppearsUseful(sample string) bool {
	trimmed := strings.TrimSpace(sample)
	if trimmed == "" {
		return false
	}
	if previewResidualNoise.MatchString(trimmed) {
		return false
	}
	return previewContainsText(trimmed)
}

func previewContainsText(sample string) bool {
	for _, r := range sample {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func stripInlinePreviewNoise(sample string) string {
	trimmed := strings.TrimSpace(sample)
	if trimmed == "" {
		return ""
	}
	loc := previewInlineNoiseStart.FindStringIndex(trimmed)
	if loc == nil {
		return trimmed
	}
	prefix := strings.TrimSpace(trimmed[:loc[0]])
	if previewContainsText(prefix) {
		return prefix
	}
	return trimmed
}

func filterPreviewNoiseLines(sample string) string {
	if strings.TrimSpace(sample) == "" {
		return ""
	}
	lines := strings.Split(sample, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		cleaned := strings.Join(strings.Fields(stripInlinePreviewNoise(line)), " ")
		if cleaned == "" {
			continue
		}
		if previewResidualNoise.MatchString(cleaned) {
			continue
		}
		filtered = append(filtered, cleaned)
	}
	return strings.Join(filtered, "\n")
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
		case previewResidualNoise.MatchString(lower):
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

// BestAvailablePreview normalizes a preview candidate and falls back through
// body text, sanitized html, and raw RFC822 when earlier candidates are noisy
// or low-signal.
func BestAvailablePreview(snippet, bodyText, bodyHTML, rawSource string, max int) string {
	if max <= 0 {
		max = DefaultPreviewMaxChars
	}
	for _, candidate := range []string{snippet, bodyText, bodyHTML} {
		if preview := normalizePreviewCandidate(candidate, max); preview != "" {
			return preview
		}
	}
	if strings.TrimSpace(rawSource) == "" {
		return ""
	}
	preview := BuildPreviewFromMIMERawSample([]byte(rawSource), max)
	if !previewAppearsUseful(preview) {
		return ""
	}
	return preview
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
