package mail

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

func EncodeMessageID(mailbox string, uid uint32) string {
	raw := fmt.Sprintf("%s|%d", mailbox, uid)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func DecodeMessageID(id string) (mailbox string, uid uint32, err error) {
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", 0, fmt.Errorf("invalid message id")
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid message id")
	}
	u, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("invalid message id")
	}
	return parts[0], uint32(u), nil
}

func EncodeAttachmentID(messageID string, part int) string {
	raw := fmt.Sprintf("%s|%d", messageID, part)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func DecodeAttachmentID(attachmentID string) (messageID string, part int, err error) {
	b, err := base64.RawURLEncoding.DecodeString(attachmentID)
	if err != nil {
		return "", 0, fmt.Errorf("invalid attachment id")
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid attachment id")
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid attachment id")
	}
	if n < 0 {
		return "", 0, fmt.Errorf("invalid attachment id")
	}
	return parts[0], n, nil
}

const (
	scopedIndexedMessageIDPrefix = "v2m_"
	scopedIndexedThreadIDPrefix  = "v2t_"
)

func ScopeIndexedMessageID(accountID, legacyMessageID string) string {
	return scopeIndexedID(scopedIndexedMessageIDPrefix, accountID, legacyMessageID)
}

func IsScopedIndexedMessageID(id string) bool {
	return isScopedIndexedID(scopedIndexedMessageIDPrefix, id)
}

func NormalizeIndexedMessageID(accountID, id string) string {
	if IsScopedIndexedMessageID(id) {
		return strings.TrimSpace(id)
	}
	return ScopeIndexedMessageID(accountID, id)
}

func UnscopeIndexedMessageID(id string) string {
	return unscopeIndexedID(scopedIndexedMessageIDPrefix, id)
}

func ScopeIndexedThreadID(accountID, legacyThreadID string) string {
	return scopeIndexedID(scopedIndexedThreadIDPrefix, accountID, legacyThreadID)
}

func IsScopedIndexedThreadID(id string) bool {
	return isScopedIndexedID(scopedIndexedThreadIDPrefix, id)
}

func NormalizeIndexedThreadID(accountID, id string) string {
	if IsScopedIndexedThreadID(id) {
		return strings.TrimSpace(id)
	}
	return ScopeIndexedThreadID(accountID, id)
}

func UnscopeIndexedThreadID(id string) string {
	return unscopeIndexedID(scopedIndexedThreadIDPrefix, id)
}

func scopeIndexedID(prefix, accountID, legacyID string) string {
	legacyID = strings.TrimSpace(legacyID)
	if legacyID == "" {
		return ""
	}
	if isScopedIndexedID(prefix, legacyID) {
		return legacyID
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return legacyID
	}
	raw := accountID + "\x00" + legacyID
	return prefix + base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func isScopedIndexedID(prefix, id string) bool {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, prefix) {
		return false
	}
	raw := strings.TrimPrefix(id, prefix)
	if raw == "" {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return false
	}
	parts := strings.SplitN(string(decoded), "\x00", 2)
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}

func unscopeIndexedID(prefix, id string) string {
	id = strings.TrimSpace(id)
	if !isScopedIndexedID(prefix, id) {
		return id
	}
	raw := strings.TrimPrefix(id, prefix)
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return id
	}
	parts := strings.SplitN(string(decoded), "\x00", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return id
	}
	return parts[1]
}
