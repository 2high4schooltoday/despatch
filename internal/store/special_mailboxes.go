package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

func normalizeSpecialMailboxRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "sent":
		return "sent"
	case "archive":
		return "archive"
	case "trash":
		return "trash"
	default:
		return ""
	}
}

func specialMailboxSettingKey(userID, mailIdentity string) string {
	normalizedUser := strings.TrimSpace(userID)
	normalizedIdentity := strings.ToLower(strings.TrimSpace(mailIdentity))
	sum := sha256.Sum256([]byte(normalizedIdentity))
	return fmt.Sprintf("mail.special.%s.%s", normalizedUser, hex.EncodeToString(sum[:8]))
}

func (s *Store) ListSpecialMailboxMappings(ctx context.Context, userID, mailIdentity string) (map[string]string, error) {
	raw, ok, err := s.GetSetting(ctx, specialMailboxSettingKey(userID, mailIdentity))
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return map[string]string{}, nil
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(payload))
	for role, mailbox := range payload {
		normalizedRole := normalizeSpecialMailboxRole(role)
		name := strings.TrimSpace(mailbox)
		if normalizedRole == "" || name == "" {
			continue
		}
		out[normalizedRole] = name
	}
	return out, nil
}

func (s *Store) UpsertSpecialMailboxMapping(ctx context.Context, userID, mailIdentity, role, mailbox string) error {
	normalizedRole := normalizeSpecialMailboxRole(role)
	name := strings.TrimSpace(mailbox)
	if normalizedRole == "" {
		return fmt.Errorf("unsupported mailbox role")
	}
	if name == "" {
		return fmt.Errorf("mailbox name is required")
	}
	current, err := s.ListSpecialMailboxMappings(ctx, userID, mailIdentity)
	if err != nil {
		return err
	}
	current[normalizedRole] = name
	body, err := json.Marshal(current)
	if err != nil {
		return err
	}
	return s.UpsertSetting(ctx, specialMailboxSettingKey(userID, mailIdentity), string(body))
}

func (s *Store) RenameSpecialMailboxMappings(ctx context.Context, userID, mailIdentity, mailbox, newMailbox string) error {
	mailbox = strings.TrimSpace(mailbox)
	newMailbox = strings.TrimSpace(newMailbox)
	if mailbox == "" || newMailbox == "" || strings.EqualFold(mailbox, newMailbox) {
		return nil
	}
	current, err := s.ListSpecialMailboxMappings(ctx, userID, mailIdentity)
	if err != nil {
		return err
	}
	changed := false
	for role, mapped := range current {
		if strings.EqualFold(strings.TrimSpace(mapped), mailbox) {
			current[role] = newMailbox
			changed = true
		}
	}
	if !changed {
		return nil
	}
	body, err := json.Marshal(current)
	if err != nil {
		return err
	}
	return s.UpsertSetting(ctx, specialMailboxSettingKey(userID, mailIdentity), string(body))
}

func (s *Store) DeleteSpecialMailboxMappingsByMailbox(ctx context.Context, userID, mailIdentity, mailbox string) error {
	mailbox = strings.TrimSpace(mailbox)
	if mailbox == "" {
		return nil
	}
	current, err := s.ListSpecialMailboxMappings(ctx, userID, mailIdentity)
	if err != nil {
		return err
	}
	changed := false
	for role, mapped := range current {
		if strings.EqualFold(strings.TrimSpace(mapped), mailbox) {
			delete(current, role)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if len(current) == 0 {
		return s.DeleteSetting(ctx, specialMailboxSettingKey(userID, mailIdentity))
	}
	body, err := json.Marshal(current)
	if err != nil {
		return err
	}
	return s.UpsertSetting(ctx, specialMailboxSettingKey(userID, mailIdentity), string(body))
}
