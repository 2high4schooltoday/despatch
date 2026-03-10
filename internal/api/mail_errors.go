package api

import (
	"net/http"
	"strings"
)

func isInvalidMessageHeaderError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "invalid from email address") || strings.Contains(message, "invalid reply-to address")
}

func classifyMailboxMutationIMAPError(err error) (status int, code string, ok bool) {
	if err == nil {
		return 0, "", false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "already exists"),
		strings.Contains(message, "mailbox exists"),
		strings.Contains(message, "mailbox already exists"),
		strings.Contains(message, "name already exists"),
		strings.Contains(message, "duplicate mailbox"):
		return http.StatusConflict, "mailbox_exists", true
	case strings.Contains(message, "not empty"),
		strings.Contains(message, "non-empty"),
		strings.Contains(message, "mailbox isn't empty"),
		strings.Contains(message, "mailbox is not empty"),
		strings.Contains(message, "has messages"):
		return http.StatusConflict, "mailbox_not_empty", true
	default:
		return 0, "", false
	}
}
