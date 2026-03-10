package api

import (
	"html"
	"net/http"
	"strings"

	"despatch/internal/mail"
	"despatch/internal/middleware"
	"despatch/internal/util"
)

func (h *Handlers) V2GetSessionMailProfile(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	profile, err := h.svc.EnsureSessionMailProfile(r.Context(), u)
	if err != nil {
		util.WriteError(w, 500, "session_mail_profile_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, profile)
}

func (h *Handlers) V2UpdateSessionMailProfile(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	profile, err := h.svc.EnsureSessionMailProfile(r.Context(), u)
	if err != nil {
		util.WriteError(w, 500, "session_mail_profile_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		DisplayName   *string `json:"display_name"`
		ReplyTo       *string `json:"reply_to"`
		SignatureText *string `json:"signature_text"`
		SignatureHTML *string `json:"signature_html"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if req.DisplayName != nil {
		profile.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.ReplyTo != nil {
		profile.ReplyTo = strings.TrimSpace(*req.ReplyTo)
		profile.ReplyTo, err = normalizeOptionalMailboxAddress(profile.ReplyTo, "reply_to")
		if err != nil {
			util.WriteError(w, 400, "session_mail_profile_update_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
	}
	if req.SignatureHTML != nil {
		profile.SignatureHTML = strings.TrimSpace(*req.SignatureHTML)
	}
	if req.SignatureText != nil {
		profile.SignatureText = normalizeStoredSignatureText(*req.SignatureText)
	}
	profile.SignatureHTML, profile.SignatureText = normalizeSignatureFields(profile.SignatureHTML, profile.SignatureText)
	profile, err = h.svc.Store().UpsertSessionMailProfile(r.Context(), profile)
	if err != nil {
		util.WriteError(w, 400, "session_mail_profile_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, profile)
}

func normalizeSignatureFields(signatureHTML, signatureText string) (string, string) {
	htmlValue := strings.TrimSpace(signatureHTML)
	textValue := normalizeStoredSignatureText(signatureText)
	if htmlValue == "" && textValue != "" {
		htmlValue = signatureHTMLFromText(stripSignatureDelimiter(textValue))
	}
	if textValue == "" && htmlValue != "" {
		textValue = signatureTextFromHTML(htmlValue)
	}
	return htmlValue, textValue
}

func signatureTextFromHTML(rawHTML string) string {
	plain := strings.TrimSpace(mail.PlainTextFromHTML(rawHTML))
	if plain == "" {
		return ""
	}
	return "-- \n" + plain
}

func normalizeStoredSignatureText(raw string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "-- \n") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "--\n") {
		return "-- \n" + strings.TrimPrefix(trimmed, "--\n")
	}
	return "-- \n" + stripSignatureDelimiter(trimmed)
}

func stripSignatureDelimiter(raw string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	trimmed = strings.TrimPrefix(trimmed, "-- \n")
	trimmed = strings.TrimPrefix(trimmed, "--\n")
	trimmed = strings.TrimPrefix(trimmed, "-- ")
	return strings.TrimSpace(trimmed)
}

func signatureHTMLFromText(rawText string) string {
	body := strings.TrimSpace(strings.ReplaceAll(rawText, "\r\n", "\n"))
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		rendered = append(rendered, "<p>"+html.EscapeString(line)+"</p>")
	}
	return strings.Join(rendered, "")
}
