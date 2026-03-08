package mail

import "strings"

func MailboxRole(name string, attributes []string) string {
	attrRole := mailboxRoleFromAttributes(attributes)
	if attrRole != "" {
		return attrRole
	}
	return mailboxRoleFromName(name)
}

func ResolveMailboxByRole(mailboxes []Mailbox, role string) string {
	target := strings.ToLower(strings.TrimSpace(role))
	if target == "" {
		return ""
	}
	for _, mb := range mailboxes {
		if strings.EqualFold(strings.TrimSpace(mb.Role), target) {
			return strings.TrimSpace(mb.Name)
		}
	}
	for _, mb := range mailboxes {
		if mailboxRoleFromName(mb.Name) == target {
			return strings.TrimSpace(mb.Name)
		}
	}
	return ""
}

func mailboxRoleFromAttributes(attributes []string) string {
	for _, attr := range attributes {
		switch strings.ToLower(strings.TrimSpace(attr)) {
		case "\\inbox":
			return "inbox"
		case "\\sent":
			return "sent"
		case "\\trash":
			return "trash"
		case "\\archive", "\\all":
			return "archive"
		case "\\junk", "\\spam":
			return "junk"
		case "\\drafts":
			return "drafts"
		}
	}
	return ""
}

func mailboxRoleFromName(name string) string {
	v := strings.ToLower(strings.TrimSpace(name))
	switch {
	case v == "inbox" || strings.HasSuffix(v, "/inbox"):
		return "inbox"
	case v == "drafts" || strings.Contains(v, "draft"):
		return "drafts"
	case v == "sent" || v == "sent messages" || strings.Contains(v, "sent"):
		return "sent"
	case v == "trash" || v == "deleted messages" || strings.Contains(v, "trash") || strings.Contains(v, "deleted"):
		return "trash"
	case v == "archive" || strings.Contains(v, "archive") || strings.Contains(v, "all mail"):
		return "archive"
	case v == "junk" || v == "spam" || strings.Contains(v, "junk") || strings.Contains(v, "spam"):
		return "junk"
	default:
		return ""
	}
}
