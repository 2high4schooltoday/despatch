package mail

import (
	"bytes"
	"net/url"
	"strings"

	msgmail "github.com/emersion/go-message/mail"
)

type UnsubscribeAction struct {
	Method   string `json:"method"`
	URL      string `json:"url,omitempty"`
	Email    string `json:"email,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Body     string `json:"body,omitempty"`
	OneClick bool   `json:"one_click,omitempty"`
}

type headerGetter interface {
	Get(string) string
}

func ParsePreferredUnsubscribeActionFromRaw(raw []byte) (*UnsubscribeAction, error) {
	reader, err := msgmail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return ParsePreferredUnsubscribeAction(&reader.Header), nil
}

func ParsePreferredUnsubscribeAction(headers headerGetter) *UnsubscribeAction {
	if headers == nil {
		return nil
	}
	rawLinks := strings.TrimSpace(headers.Get("List-Unsubscribe"))
	if rawLinks == "" {
		return nil
	}
	oneClick := strings.Contains(
		strings.ToLower(strings.TrimSpace(headers.Get("List-Unsubscribe-Post"))),
		"list-unsubscribe=one-click",
	)
	links := splitHeaderLinks(rawLinks)
	if len(links) == 0 {
		return nil
	}
	httpCandidates := make([]UnsubscribeAction, 0, len(links))
	mailtoCandidates := make([]UnsubscribeAction, 0, len(links))
	for _, link := range links {
		trimmed := strings.TrimSpace(strings.Trim(link, "<>"))
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "https://"), strings.HasPrefix(lower, "http://"):
			httpCandidates = append(httpCandidates, UnsubscribeAction{
				Method: "GET",
				URL:    trimmed,
			})
		case strings.HasPrefix(lower, "mailto:"):
			action := parseMailtoUnsubscribe(trimmed)
			if action.Email != "" {
				mailtoCandidates = append(mailtoCandidates, action)
			}
		}
	}
	if oneClick {
		for _, candidate := range httpCandidates {
			if strings.HasPrefix(strings.ToLower(candidate.URL), "https://") {
				candidate.Method = "POST"
				candidate.OneClick = true
				return &candidate
			}
		}
	}
	if len(httpCandidates) > 0 {
		candidate := httpCandidates[0]
		return &candidate
	}
	if len(mailtoCandidates) > 0 {
		candidate := mailtoCandidates[0]
		return &candidate
	}
	return nil
}

func splitHeaderLinks(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	out := make([]string, 0, 4)
	for {
		start := strings.Index(trimmed, "<")
		if start < 0 {
			break
		}
		end := strings.Index(trimmed[start+1:], ">")
		if end < 0 {
			break
		}
		value := strings.TrimSpace(trimmed[start+1 : start+1+end])
		if value != "" {
			out = append(out, value)
		}
		trimmed = trimmed[start+1+end+1:]
	}
	if len(out) > 0 {
		return out
	}
	parts := strings.Split(trimmed, ",")
	out = make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(strings.Trim(part, "<>"))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func parseMailtoUnsubscribe(value string) UnsubscribeAction {
	spec := strings.TrimSpace(value)
	spec = strings.TrimPrefix(spec, "mailto:")
	spec = strings.TrimPrefix(spec, "MAILTO:")
	addressPart, queryPart, _ := strings.Cut(spec, "?")
	addresses := strings.Split(strings.TrimSpace(addressPart), ",")
	email := ""
	for _, address := range addresses {
		address = strings.TrimSpace(address)
		if address != "" {
			email = address
			break
		}
	}
	action := UnsubscribeAction{
		Method: "MAILTO",
		Email:  email,
	}
	if queryPart == "" {
		return action
	}
	values, err := url.ParseQuery(queryPart)
	if err != nil {
		return action
	}
	action.Subject = strings.TrimSpace(values.Get("subject"))
	action.Body = strings.TrimSpace(values.Get("body"))
	return action
}
