package mail

import (
	"fmt"
	stdmail "net/mail"
	"strings"
)

func NormalizeMailboxAddress(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("email address is required")
	}
	parsed, err := stdmail.ParseAddress(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid email address")
	}
	address := strings.TrimSpace(parsed.Address)
	if address == "" {
		return "", fmt.Errorf("invalid email address")
	}
	return address, nil
}
