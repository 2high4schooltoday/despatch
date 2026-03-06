package mail

import (
	"fmt"
	stdmime "mime"
	"strings"

	gomail "github.com/emersion/go-message/mail"
)

const defaultAttachmentContentType = "application/octet-stream"

type mimePartKind uint8

const (
	mimePartIgnore mimePartKind = iota
	mimePartTextPlain
	mimePartTextHTML
	mimePartAttachment
)

type mimePartDescriptor struct {
	kind        mimePartKind
	contentType string
	filename    string
	inline      bool
	contentID   string
}

func classifyMIMEPart(header gomail.PartHeader) mimePartDescriptor {
	switch h := header.(type) {
	case *gomail.InlineHeader:
		return classifyInlinePart(h)
	case *gomail.AttachmentHeader:
		return classifyAttachmentPart(h)
	default:
		return mimePartDescriptor{kind: mimePartIgnore}
	}
}

func classifyInlinePart(h *gomail.InlineHeader) mimePartDescriptor {
	contentType, ctParams, disposition, dispParams, contentID := readPartHeaderMeta(h)
	filename := firstNonEmpty(strings.TrimSpace(dispParams["filename"]), strings.TrimSpace(ctParams["name"]))

	if disposition == "attachment" {
		return mimePartDescriptor{
			kind:        mimePartAttachment,
			contentType: normalizeAttachmentContentType(contentType),
			filename:    filename,
			inline:      false,
			contentID:   contentID,
		}
	}

	if contentType == "" {
		if contentID != "" || filename != "" || disposition == "inline" {
			return mimePartDescriptor{
				kind:        mimePartAttachment,
				contentType: defaultAttachmentContentType,
				filename:    filename,
				inline:      true,
				contentID:   contentID,
			}
		}
		return mimePartDescriptor{
			kind:        mimePartTextPlain,
			contentType: "text/plain",
		}
	}

	switch contentType {
	case "text/plain":
		return mimePartDescriptor{kind: mimePartTextPlain, contentType: contentType}
	case "text/html":
		return mimePartDescriptor{kind: mimePartTextHTML, contentType: contentType}
	default:
		return mimePartDescriptor{
			kind:        mimePartAttachment,
			contentType: normalizeAttachmentContentType(contentType),
			filename:    filename,
			inline:      true,
			contentID:   contentID,
		}
	}
}

func classifyAttachmentPart(h *gomail.AttachmentHeader) mimePartDescriptor {
	contentType, ctParams, disposition, dispParams, contentID := readPartHeaderMeta(h)
	filename, _ := h.Filename()
	filename = firstNonEmpty(strings.TrimSpace(filename), strings.TrimSpace(dispParams["filename"]), strings.TrimSpace(ctParams["name"]))
	inline := disposition == "inline" || contentID != ""

	return mimePartDescriptor{
		kind:        mimePartAttachment,
		contentType: normalizeAttachmentContentType(contentType),
		filename:    filename,
		inline:      inline,
		contentID:   contentID,
	}
}

type messageHeaderMeta interface {
	ContentType() (string, map[string]string, error)
	ContentDisposition() (string, map[string]string, error)
	Text(string) (string, error)
}

func readPartHeaderMeta(header messageHeaderMeta) (contentType string, ctParams map[string]string, disposition string, dispParams map[string]string, contentID string) {
	contentType, ctParams, _ = header.ContentType()
	contentType = normalizeContentType(contentType)
	if ctParams == nil {
		ctParams = map[string]string{}
	}

	disposition, dispParams, _ = header.ContentDisposition()
	disposition = strings.ToLower(strings.TrimSpace(disposition))
	if dispParams == nil {
		dispParams = map[string]string{}
	}

	if v, err := header.Text("Content-Id"); err == nil {
		contentID = normalizePartContentID(v)
	}
	if contentID == "" {
		if v, err := header.Text("Content-ID"); err == nil {
			contentID = normalizePartContentID(v)
		}
	}
	return contentType, ctParams, disposition, dispParams, contentID
}

func normalizeContentType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	mediaType, _, err := stdmime.ParseMediaType(value)
	if err == nil {
		return strings.ToLower(strings.TrimSpace(mediaType))
	}
	return strings.ToLower(value)
}

func normalizeAttachmentContentType(value string) string {
	normalized := normalizeContentType(value)
	if normalized == "" {
		return defaultAttachmentContentType
	}
	return normalized
}

func normalizePartContentID(value string) string {
	clean := strings.TrimSpace(value)
	for {
		trimmed := strings.TrimPrefix(strings.TrimSuffix(clean, ">"), "<")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == clean {
			break
		}
		clean = trimmed
	}
	return clean
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fallbackAttachmentFilename(contentType string, partIndex int) string {
	ext := ".bin"
	if exts, err := stdmime.ExtensionsByType(strings.TrimSpace(contentType)); err == nil && len(exts) > 0 {
		ext = exts[0]
	}
	return fmt.Sprintf("part-%d%s", partIndex, ext)
}
