package api

import (
	"fmt"
	"strings"

	"despatch/internal/mail"
)

func normalizeRequiredMailboxAddress(raw, field string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	address, err := mail.NormalizeMailboxAddress(trimmed)
	if err != nil {
		return "", fmt.Errorf("%s must be a valid email address", field)
	}
	return address, nil
}

func normalizeOptionalMailboxAddress(raw, field string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	address, err := mail.NormalizeMailboxAddress(trimmed)
	if err != nil {
		return "", fmt.Errorf("%s must be a valid email address", field)
	}
	return address, nil
}
