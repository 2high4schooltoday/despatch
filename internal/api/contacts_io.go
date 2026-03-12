package api

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"

	"despatch/internal/models"
)

type importedContact struct {
	Name               string
	Nicknames          []string
	Emails             []models.ContactEmail
	Notes              string
	Groups             []string
	PreferredAccountID string
	PreferredSenderID  string
}

func splitPipeValues(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, "|")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func parseCSVBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func importedContactCSVKey(item importedContact) string {
	nicknames := append([]string(nil), item.Nicknames...)
	groups := append([]string(nil), item.Groups...)
	sort.Slice(nicknames, func(i, j int) bool { return strings.ToLower(nicknames[i]) < strings.ToLower(nicknames[j]) })
	sort.Slice(groups, func(i, j int) bool { return strings.ToLower(groups[i]) < strings.ToLower(groups[j]) })
	return strings.Join([]string{
		strings.TrimSpace(item.Name),
		strings.Join(nicknames, "|"),
		strings.TrimSpace(item.Notes),
		strings.Join(groups, "|"),
		strings.TrimSpace(item.PreferredAccountID),
		strings.TrimSpace(item.PreferredSenderID),
	}, "\x1f")
}

func parseContactsCSV(data []byte) ([]importedContact, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []importedContact{}, nil
	}
	headers := map[string]int{}
	for i, cell := range rows[0] {
		headers[strings.ToLower(strings.TrimSpace(cell))] = i
	}
	get := func(row []string, key string) string {
		idx, ok := headers[key]
		if !ok || idx < 0 || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}
	out := make([]importedContact, 0, len(rows)-1)
	indexByKey := map[string]int{}
	for _, row := range rows[1:] {
		item := importedContact{
			Name:               get(row, "name"),
			Nicknames:          splitPipeValues(get(row, "nicknames")),
			Notes:              get(row, "notes"),
			Groups:             splitPipeValues(get(row, "groups")),
			PreferredAccountID: get(row, "preferred_account_id"),
			PreferredSenderID:  get(row, "preferred_sender_id"),
		}
		email := get(row, "email")
		if email != "" {
			item.Emails = []models.ContactEmail{{
				Email:     email,
				Label:     get(row, "email_label"),
				IsPrimary: parseCSVBool(get(row, "is_primary")),
			}}
		}
		key := importedContactCSVKey(item)
		if idx, ok := indexByKey[key]; ok {
			out[idx].Emails = append(out[idx].Emails, item.Emails...)
			continue
		}
		indexByKey[key] = len(out)
		out = append(out, item)
	}
	return out, nil
}

func exportContactsCSV(contacts []models.Contact, groupsByID map[string]models.ContactGroup) ([]byte, error) {
	buf := &bytes.Buffer{}
	writer := csv.NewWriter(buf)
	if err := writer.Write([]string{
		"name",
		"nicknames",
		"email",
		"email_label",
		"is_primary",
		"groups",
		"notes",
		"preferred_account_id",
		"preferred_sender_id",
	}); err != nil {
		return nil, err
	}
	for _, contact := range contacts {
		nicknames := strings.Join(contact.Nicknames, "|")
		groupNames := make([]string, 0, len(contact.GroupIDs))
		for _, groupID := range contact.GroupIDs {
			if group, ok := groupsByID[groupID]; ok && strings.TrimSpace(group.Name) != "" {
				groupNames = append(groupNames, group.Name)
			}
		}
		sort.Slice(groupNames, func(i, j int) bool { return strings.ToLower(groupNames[i]) < strings.ToLower(groupNames[j]) })
		if len(contact.Emails) == 0 {
			if err := writer.Write([]string{
				strings.TrimSpace(contact.Name),
				nicknames,
				"",
				"",
				"",
				strings.Join(groupNames, "|"),
				strings.TrimSpace(contact.Notes),
				strings.TrimSpace(contact.PreferredAccountID),
				strings.TrimSpace(contact.PreferredSenderID),
			}); err != nil {
				return nil, err
			}
			continue
		}
		for _, email := range contact.Emails {
			if err := writer.Write([]string{
				strings.TrimSpace(contact.Name),
				nicknames,
				strings.TrimSpace(email.Email),
				strings.TrimSpace(email.Label),
				fmt.Sprintf("%t", email.IsPrimary),
				strings.Join(groupNames, "|"),
				strings.TrimSpace(contact.Notes),
				strings.TrimSpace(contact.PreferredAccountID),
				strings.TrimSpace(contact.PreferredSenderID),
			}); err != nil {
				return nil, err
			}
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func unfoldVCardLines(data string) []string {
	rawLines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if len(out) > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			out[len(out)-1] += strings.TrimLeft(line, " \t")
			continue
		}
		out = append(out, line)
	}
	return out
}

func parseVCardProperty(line string) (string, []string, string, bool) {
	if strings.TrimSpace(line) == "" {
		return "", nil, "", false
	}
	idx := strings.IndexByte(line, ':')
	if idx <= 0 {
		return "", nil, "", false
	}
	head := line[:idx]
	value := line[idx+1:]
	parts := strings.Split(head, ";")
	name := strings.ToUpper(strings.TrimSpace(parts[0]))
	params := make([]string, 0, len(parts)-1)
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part != "" {
			params = append(params, part)
		}
	}
	return name, params, unescapeVCardValue(value), true
}

func unescapeVCardValue(raw string) string {
	replacer := strings.NewReplacer(
		`\\n`, "\n",
		`\\N`, "\n",
		`\\,`, ",",
		`\\;`, ";",
		`\\\\`, `\`,
	)
	return replacer.Replace(strings.TrimSpace(raw))
}

func escapeVCardValue(raw string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		"\n", `\n`,
		",", `\,`,
		";", `\;`,
	)
	return replacer.Replace(strings.TrimSpace(raw))
}

func parseContactsVCF(data []byte) ([]importedContact, error) {
	lines := unfoldVCardLines(string(data))
	out := make([]importedContact, 0, 16)
	var current *importedContact
	for _, line := range lines {
		name := strings.ToUpper(strings.TrimSpace(line))
		switch name {
		case "BEGIN:VCARD":
			current = &importedContact{}
			continue
		case "END:VCARD":
			if current != nil {
				out = append(out, *current)
			}
			current = nil
			continue
		}
		if current == nil {
			continue
		}
		propName, params, value, ok := parseVCardProperty(line)
		if !ok {
			continue
		}
		switch propName {
		case "FN":
			current.Name = value
		case "NICKNAME":
			current.Nicknames = splitPipeValues(strings.ReplaceAll(value, ",", "|"))
		case "NOTE":
			current.Notes = value
		case "CATEGORIES":
			current.Groups = splitPipeValues(strings.ReplaceAll(value, ",", "|"))
		case "EMAIL":
			label := ""
			for _, param := range params {
				upper := strings.ToUpper(param)
				if strings.HasPrefix(upper, "TYPE=") {
					label = strings.TrimSpace(param[len("TYPE="):])
					break
				}
			}
			current.Emails = append(current.Emails, models.ContactEmail{
				Email:     value,
				Label:     label,
				IsPrimary: len(current.Emails) == 0,
			})
		case "X-DESPATCH-PREFERRED-ACCOUNT-ID":
			current.PreferredAccountID = value
		case "X-DESPATCH-PREFERRED-SENDER-ID":
			current.PreferredSenderID = value
		}
	}
	return out, nil
}

func exportContactsVCF(contacts []models.Contact, groupsByID map[string]models.ContactGroup) ([]byte, error) {
	var buf strings.Builder
	for _, contact := range contacts {
		buf.WriteString("BEGIN:VCARD\r\n")
		buf.WriteString("VERSION:3.0\r\n")
		displayName := strings.TrimSpace(contact.Name)
		if displayName == "" && len(contact.Emails) > 0 {
			displayName = strings.TrimSpace(contact.Emails[0].Email)
		}
		buf.WriteString("FN:" + escapeVCardValue(displayName) + "\r\n")
		if len(contact.Nicknames) > 0 {
			buf.WriteString("NICKNAME:" + escapeVCardValue(strings.Join(contact.Nicknames, ",")) + "\r\n")
		}
		for _, email := range contact.Emails {
			line := "EMAIL"
			if strings.TrimSpace(email.Label) != "" {
				line += ";TYPE=" + escapeVCardValue(strings.TrimSpace(email.Label))
			}
			line += ":" + escapeVCardValue(strings.TrimSpace(email.Email))
			buf.WriteString(line + "\r\n")
		}
		if strings.TrimSpace(contact.Notes) != "" {
			buf.WriteString("NOTE:" + escapeVCardValue(contact.Notes) + "\r\n")
		}
		if len(contact.GroupIDs) > 0 {
			groupNames := make([]string, 0, len(contact.GroupIDs))
			for _, groupID := range contact.GroupIDs {
				if group, ok := groupsByID[groupID]; ok && strings.TrimSpace(group.Name) != "" {
					groupNames = append(groupNames, group.Name)
				}
			}
			sort.Slice(groupNames, func(i, j int) bool { return strings.ToLower(groupNames[i]) < strings.ToLower(groupNames[j]) })
			if len(groupNames) > 0 {
				buf.WriteString("CATEGORIES:" + escapeVCardValue(strings.Join(groupNames, ",")) + "\r\n")
			}
		}
		if strings.TrimSpace(contact.PreferredAccountID) != "" {
			buf.WriteString("X-DESPATCH-PREFERRED-ACCOUNT-ID:" + escapeVCardValue(contact.PreferredAccountID) + "\r\n")
		}
		if strings.TrimSpace(contact.PreferredSenderID) != "" {
			buf.WriteString("X-DESPATCH-PREFERRED-SENDER-ID:" + escapeVCardValue(contact.PreferredSenderID) + "\r\n")
		}
		buf.WriteString("END:VCARD\r\n")
	}
	return []byte(buf.String()), nil
}
