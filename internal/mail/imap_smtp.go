package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"io/fs"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"

	"despatch/internal/config"
)

const (
	defaultDialTimeout = 10 * time.Second
	maxBodyBytes       = 1 << 20  // 1 MiB body preview
	maxAttachmentBytes = 25 << 20 // 25 MiB per attachment
)

type IMAPSMTPClient struct {
	cfg config.Config
}

func NewIMAPSMTPClient(cfg config.Config) *IMAPSMTPClient {
	return &IMAPSMTPClient{cfg: cfg}
}

func (c *IMAPSMTPClient) ListMailboxes(ctx context.Context, user, pass string) ([]Mailbox, error) {
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	ch := make(chan *imap.MailboxInfo, 32)
	done := make(chan error, 1)
	go func() {
		done <- cli.List("", "*", ch)
	}()

	var out []Mailbox
	for mb := range ch {
		status, err := cli.Status(mb.Name, []imap.StatusItem{imap.StatusMessages, imap.StatusUnseen})
		if err != nil {
			continue
		}
		out = append(out, Mailbox{Name: mb.Name, Unread: int(status.Unseen), Messages: int(status.Messages)})
	}
	if err := <-done; err != nil {
		return nil, err
	}
	if len(out) == 0 {
		out = []Mailbox{{Name: "INBOX"}}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == "INBOX" {
			return true
		}
		if out[j].Name == "INBOX" {
			return false
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (c *IMAPSMTPClient) ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]MessageSummary, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	mbox, err := cli.Select(mailbox, true)
	if err != nil {
		return nil, err
	}
	if mbox.Messages == 0 {
		return []MessageSummary{}, nil
	}

	total := int(mbox.Messages)
	end := total - (page-1)*pageSize
	if end < 1 {
		return []MessageSummary{}, nil
	}
	start := end - pageSize + 1
	if start < 1 {
		start = 1
	}

	seq := new(imap.SeqSet)
	seq.AddRange(uint32(start), uint32(end))
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchInternalDate}
	messages := make(chan *imap.Message, pageSize)
	done := make(chan error, 1)
	go func() {
		done <- cli.Fetch(seq, items, messages)
	}()

	out := make([]MessageSummary, 0, pageSize)
	for msg := range messages {
		if msg == nil {
			continue
		}
		from := envelopeFirstAddress(msg.Envelope.From)
		subject := ""
		date := msg.InternalDate
		if msg.Envelope != nil {
			subject = msg.Envelope.Subject
			if !msg.Envelope.Date.IsZero() {
				date = msg.Envelope.Date
			}
		}
		out = append(out, MessageSummary{
			ID:      EncodeMessageID(mailbox, msg.Uid),
			From:    from,
			Subject: subject,
			Date:    date,
			Seen:    hasFlag(msg.Flags, imap.SeenFlag),
		})
	}
	if err := <-done; err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}

func (c *IMAPSMTPClient) GetMessage(ctx context.Context, user, pass, id string) (Message, error) {
	mailbox, uid, err := DecodeMessageID(id)
	if err != nil {
		return Message{}, err
	}
	raw, env, err := c.fetchRawMessage(ctx, user, pass, mailbox, uid)
	if err != nil {
		return Message{}, err
	}
	parsed, err := parseMessage(raw, id, mailbox, uid)
	if err != nil {
		return Message{}, err
	}
	if parsed.Subject == "" && env != nil {
		parsed.Subject = env.Subject
	}
	if parsed.From == "" && env != nil {
		parsed.From = envelopeFirstAddress(env.From)
	}
	if parsed.Date.IsZero() && env != nil {
		parsed.Date = env.Date
	}
	return parsed, nil
}

func (c *IMAPSMTPClient) GetRawMessage(ctx context.Context, user, pass, id string) ([]byte, error) {
	mailbox, uid, err := DecodeMessageID(id)
	if err != nil {
		return nil, err
	}
	raw, _, err := c.fetchRawMessage(ctx, user, pass, mailbox, uid)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *IMAPSMTPClient) Search(ctx context.Context, user, pass, mailbox, query string, page, pageSize int) ([]MessageSummary, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	if strings.TrimSpace(mailbox) == "" {
		mailbox = "INBOX"
	}
	if _, err := cli.Select(mailbox, true); err != nil {
		return nil, err
	}
	criteria := imap.NewSearchCriteria()
	if strings.TrimSpace(query) != "" {
		criteria.Text = []string{query}
	}
	uids, err := cli.UidSearch(criteria)
	if err != nil {
		return nil, err
	}
	if len(uids) == 0 {
		return []MessageSummary{}, nil
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })

	start := (page - 1) * pageSize
	if start >= len(uids) {
		return []MessageSummary{}, nil
	}
	end := start + pageSize
	if end > len(uids) {
		end = len(uids)
	}
	selected := uids[start:end]

	seq := new(imap.SeqSet)
	for _, u := range selected {
		seq.AddNum(u)
	}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchInternalDate}
	messages := make(chan *imap.Message, len(selected))
	done := make(chan error, 1)
	go func() {
		done <- cli.UidFetch(seq, items, messages)
	}()

	msgByUID := make(map[uint32]MessageSummary, len(selected))
	for msg := range messages {
		if msg == nil {
			continue
		}
		s := MessageSummary{
			ID:      EncodeMessageID(mailbox, msg.Uid),
			From:    envelopeFirstAddress(msg.Envelope.From),
			Subject: msg.Envelope.Subject,
			Date:    msg.InternalDate,
			Seen:    hasFlag(msg.Flags, imap.SeenFlag),
		}
		if !msg.Envelope.Date.IsZero() {
			s.Date = msg.Envelope.Date
		}
		msgByUID[msg.Uid] = s
	}
	if err := <-done; err != nil {
		return nil, err
	}

	out := make([]MessageSummary, 0, len(selected))
	for _, uid := range selected {
		if v, ok := msgByUID[uid]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

func (c *IMAPSMTPClient) Send(ctx context.Context, user, pass string, req SendRequest) error {
	req.To = normalizedRecipients(req.To)
	req.CC = normalizedRecipients(req.CC)
	req.BCC = normalizedRecipients(req.BCC)
	if len(mergeRecipients(req.To, req.CC, req.BCC)) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	if req.From == "" {
		req.From = user
	}

	msg, err := buildRFC822(req)
	if err != nil {
		return err
	}

	return c.sendWithSenderFallback(ctx, user, pass, req, msg, c.sendSMTP)
}

func (c *IMAPSMTPClient) sendWithSenderFallback(
	ctx context.Context,
	user,
	pass string,
	req SendRequest,
	raw []byte,
	sendFunc func(context.Context, string, string, string, []string, []byte) error,
) error {
	allRecipients := mergeRecipients(req.To, req.CC, req.BCC)
	if len(allRecipients) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	envelopeFrom := strings.TrimSpace(req.From)
	if envelopeFrom == "" {
		envelopeFrom = strings.TrimSpace(user)
	}
	err := sendFunc(ctx, user, pass, envelopeFrom, allRecipients, raw)
	if err == nil {
		return nil
	}
	if !IsSMTPSenderPolicyError(err) {
		return err
	}
	authIdentity := strings.TrimSpace(user)
	if authIdentity != "" && !strings.EqualFold(envelopeFrom, authIdentity) {
		retryErr := sendFunc(ctx, user, pass, authIdentity, allRecipients, raw)
		if retryErr == nil {
			return nil
		}
		if IsSMTPSenderPolicyError(retryErr) {
			return WrapSMTPSenderRejected(retryErr)
		}
		return retryErr
	}
	return WrapSMTPSenderRejected(err)
}

func (c *IMAPSMTPClient) SetFlags(ctx context.Context, user, pass, id string, flags []string) error {
	mailbox, uid, err := DecodeMessageID(id)
	if err != nil {
		return err
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return err
	}
	defer cli.Logout()
	if _, err := cli.Select(mailbox, false); err != nil {
		return err
	}
	seq := new(imap.SeqSet)
	seq.AddNum(uid)

	values := make([]interface{}, 0, len(flags))
	for _, f := range flags {
		values = append(values, canonicalFlag(f))
	}
	item := imap.FormatFlagsOp(imap.SetFlags, true)
	return cli.UidStore(seq, item, values, nil)
}

func (c *IMAPSMTPClient) Move(ctx context.Context, user, pass, id, mailbox string) error {
	srcMbox, uid, err := DecodeMessageID(id)
	if err != nil {
		return err
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return err
	}
	defer cli.Logout()
	if _, err := cli.Select(srcMbox, false); err != nil {
		return err
	}
	seq := new(imap.SeqSet)
	seq.AddNum(uid)
	return cli.UidMove(seq, mailbox)
}

func (c *IMAPSMTPClient) GetAttachment(ctx context.Context, user, pass, attachmentID string) (AttachmentContent, error) {
	meta, rc, err := c.GetAttachmentStream(ctx, user, pass, attachmentID)
	if err != nil {
		return AttachmentContent{}, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return AttachmentContent{}, err
	}
	return AttachmentContent{Meta: meta, Data: data}, nil
}

func (c *IMAPSMTPClient) GetAttachmentStream(ctx context.Context, user, pass, attachmentID string) (AttachmentMeta, io.ReadCloser, error) {
	msgID, part, err := DecodeAttachmentID(attachmentID)
	if err != nil {
		return AttachmentMeta{}, nil, err
	}
	mailbox, uid, err := DecodeMessageID(msgID)
	if err != nil {
		return AttachmentMeta{}, nil, err
	}
	raw, _, err := c.fetchRawMessage(ctx, user, pass, mailbox, uid)
	if err != nil {
		return AttachmentMeta{}, nil, err
	}
	meta, data, err := extractAttachment(raw, msgID, part)
	if err != nil {
		return AttachmentMeta{}, nil, err
	}
	return meta, io.NopCloser(bytes.NewReader(data)), nil
}

func (c *IMAPSMTPClient) fetchRawMessage(ctx context.Context, user, pass, mailbox string, uid uint32) ([]byte, *imap.Envelope, error) {
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return nil, nil, err
	}
	defer cli.Logout()
	if _, err := cli.Select(mailbox, true); err != nil {
		return nil, nil, err
	}
	seq := new(imap.SeqSet)
	seq.AddNum(uid)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, section.FetchItem()}
	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- cli.UidFetch(seq, items, messages)
	}()
	msg := <-messages
	if err := <-done; err != nil {
		return nil, nil, err
	}
	if msg == nil {
		return nil, nil, fmt.Errorf("message not found")
	}
	body := msg.GetBody(section)
	if body == nil {
		return nil, nil, fmt.Errorf("server did not return message body")
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, nil, err
	}
	return raw, msg.Envelope, nil
}

func (c *IMAPSMTPClient) connectIMAP(ctx context.Context, user, pass string) (*imapclient.Client, error) {
	if pass == "" {
		return nil, fmt.Errorf("missing mail credentials")
	}
	dialer := &net.Dialer{Timeout: defaultDialTimeout}
	addr := net.JoinHostPort(c.cfg.IMAPHost, strconv.Itoa(c.cfg.IMAPPort))
	tlsConfig := &tls.Config{ServerName: c.cfg.IMAPHost, InsecureSkipVerify: c.cfg.IMAPInsecureSkipVerify}

	var cli *imapclient.Client
	var err error
	if c.cfg.IMAPTLS {
		cli, err = imapclient.DialWithDialerTLS(dialer, addr, tlsConfig)
	} else {
		cli, err = imapclient.DialWithDialer(dialer, addr)
		if err == nil && c.cfg.IMAPStartTLS {
			err = cli.StartTLS(tlsConfig)
		}
	}
	if err != nil {
		return nil, err
	}
	if err := cli.Login(user, pass); err != nil {
		_ = cli.Logout()
		return nil, err
	}
	return cli, nil
}

func (c *IMAPSMTPClient) sendSMTP(ctx context.Context, user, pass, from string, rcpt []string, raw []byte) error {
	addr := net.JoinHostPort(c.cfg.SMTPHost, strconv.Itoa(c.cfg.SMTPPort))
	tlsConfig := &tls.Config{ServerName: c.cfg.SMTPHost, InsecureSkipVerify: c.cfg.SMTPInsecureSkipVerify}

	dialer := &net.Dialer{Timeout: defaultDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}

	if c.cfg.SMTPTLS {
		conn = tls.Client(conn, tlsConfig)
	}

	client, err := smtp.NewClient(conn, c.cfg.SMTPHost)
	if err != nil {
		return err
	}
	defer client.Close()

	if c.cfg.SMTPStartTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return err
			}
		}
	}

	if ok, _ := client.Extension("AUTH"); ok {
		auth := smtp.PlainAuth("", user, pass, c.cfg.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(from); err != nil {
		return err
	}
	for _, r := range rcpt {
		if err := client.Rcpt(strings.TrimSpace(r)); err != nil {
			return err
		}
	}

	wc, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(raw); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func buildRFC822(req SendRequest) ([]byte, error) {
	to := normalizedRecipients(req.To)
	cc := normalizedRecipients(req.CC)
	bodyText := strings.TrimSpace(req.Body)
	bodyHTML := strings.TrimSpace(req.BodyHTML)
	if bodyText == "" && bodyHTML != "" {
		bodyText = plainTextFromHTML(bodyHTML)
	}
	if bodyText == "" {
		bodyText = req.Body
	}
	inlineAttachments := make([]SendAttachment, 0, len(req.Attachments))
	regularAttachments := make([]SendAttachment, 0, len(req.Attachments))
	for _, item := range req.Attachments {
		if item.Inline && strings.TrimSpace(item.ContentID) != "" {
			inlineAttachments = append(inlineAttachments, item)
		} else {
			regularAttachments = append(regularAttachments, item)
		}
	}

	var buf bytes.Buffer
	mixed := multipart.NewWriter(&buf)
	boundary := mixed.Boundary()

	fmt.Fprintf(&buf, "From: %s\r\n", req.From)
	fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(to, ", "))
	if len(cc) > 0 {
		fmt.Fprintf(&buf, "Cc: %s\r\n", strings.Join(cc, ", "))
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", req.Subject)
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	if req.InReplyToID != "" {
		fmt.Fprintf(&buf, "In-Reply-To: %s\r\n", req.InReplyToID)
	}
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: multipart/mixed; boundary=%q\r\n", boundary)
	fmt.Fprintf(&buf, "\r\n")

	if bodyHTML != "" {
		altBuf := bytes.Buffer{}
		alt := multipart.NewWriter(&altBuf)
		plainPart, err := alt.CreatePart(textPartHeader("text/plain; charset=utf-8"))
		if err != nil {
			return nil, err
		}
		if err := writeQuotedPrintablePart(plainPart, bodyText); err != nil {
			return nil, err
		}
		htmlPart, err := alt.CreatePart(textPartHeader("text/html; charset=utf-8"))
		if err != nil {
			return nil, err
		}
		if err := writeQuotedPrintablePart(htmlPart, bodyHTML); err != nil {
			return nil, err
		}
		if err := alt.Close(); err != nil {
			return nil, err
		}

		relatedBuf := bytes.Buffer{}
		related := multipart.NewWriter(&relatedBuf)
		altHeader := make(textproto.MIMEHeader)
		altHeader.Set("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", alt.Boundary()))
		altPart, err := related.CreatePart(altHeader)
		if err != nil {
			return nil, err
		}
		if _, err := altPart.Write(altBuf.Bytes()); err != nil {
			return nil, err
		}
		for _, a := range inlineAttachments {
			if err := writeAttachmentPart(related, a, true); err != nil {
				return nil, err
			}
		}
		if err := related.Close(); err != nil {
			return nil, err
		}

		relatedHeader := make(textproto.MIMEHeader)
		relatedHeader.Set("Content-Type", fmt.Sprintf("multipart/related; boundary=%q", related.Boundary()))
		relatedPart, err := mixed.CreatePart(relatedHeader)
		if err != nil {
			return nil, err
		}
		if _, err := relatedPart.Write(relatedBuf.Bytes()); err != nil {
			return nil, err
		}
	} else {
		bodyHeader := textPartHeader("text/plain; charset=utf-8")
		p, err := mixed.CreatePart(bodyHeader)
		if err != nil {
			return nil, err
		}
		if err := writeQuotedPrintablePart(p, bodyText); err != nil {
			return nil, err
		}
		for _, a := range inlineAttachments {
			if err := writeAttachmentPart(mixed, a, false); err != nil {
				return nil, err
			}
		}
	}

	for _, a := range regularAttachments {
		if err := writeAttachmentPart(mixed, a, false); err != nil {
			return nil, err
		}
	}

	if err := mixed.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func textPartHeader(contentType string) textproto.MIMEHeader {
	out := make(textproto.MIMEHeader)
	out.Set("Content-Type", contentType)
	out.Set("Content-Transfer-Encoding", "quoted-printable")
	return out
}

func writeQuotedPrintablePart(part io.Writer, body string) error {
	qp := quotedprintable.NewWriter(part)
	if _, err := qp.Write([]byte(body)); err != nil {
		return err
	}
	return qp.Close()
}

func writeAttachmentPart(writer *multipart.Writer, a SendAttachment, treatInline bool) error {
	h := make(textproto.MIMEHeader)
	ct := strings.TrimSpace(a.ContentType)
	if ct == "" {
		ct = "application/octet-stream"
	}
	filename := strings.TrimSpace(a.Filename)
	if filename == "" {
		if treatInline {
			filename = "inline.bin"
		} else {
			filename = "attachment.bin"
		}
	}
	h.Set("Content-Type", fmt.Sprintf("%s; name=%q", ct, filename))
	disposition := "attachment"
	if treatInline && strings.TrimSpace(a.ContentID) != "" {
		disposition = "inline"
		h.Set("Content-ID", fmt.Sprintf("<%s>", normalizeContentID(a.ContentID)))
	}
	h.Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, filename))
	h.Set("Content-Transfer-Encoding", "base64")
	part, err := writer.CreatePart(h)
	if err != nil {
		return err
	}
	enc := base64NewLineEncoder(part)
	if _, err := enc.Write(a.Data); err != nil {
		return err
	}
	return enc.Close()
}

func normalizeContentID(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "<")
	v = strings.TrimSuffix(v, ">")
	if v == "" {
		return "inline"
	}
	return v
}

func mergeRecipients(to, cc, bcc []string) []string {
	out := make([]string, 0, len(to)+len(cc)+len(bcc))
	seen := make(map[string]struct{}, len(out))
	appendUnique := func(items []string) {
		for _, item := range items {
			value := strings.TrimSpace(item)
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
	}
	appendUnique(to)
	appendUnique(cc)
	appendUnique(bcc)
	return out
}

func normalizedRecipients(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

var (
	htmlTagPattern        = regexp.MustCompile(`(?s)<[^>]+>`)
	htmlBreakTagPattern   = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`)
	htmlCloseBlockPattern = regexp.MustCompile(`(?i)</\s*(p|div|li|h[1-6]|tr|table|blockquote)\s*>`)
	htmlOpenLiPattern     = regexp.MustCompile(`(?i)<\s*li[^>]*>`)
)

func plainTextFromHTML(rawHTML string) string {
	s := strings.ReplaceAll(rawHTML, "\r\n", "\n")
	s = htmlBreakTagPattern.ReplaceAllString(s, "\n")
	s = htmlCloseBlockPattern.ReplaceAllString(s, "\n")
	s = htmlOpenLiPattern.ReplaceAllString(s, "- ")
	s = htmlTagPattern.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	normalized := make([]string, 0, len(lines))
	blankCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			blankCount++
			if blankCount > 1 {
				continue
			}
			normalized = append(normalized, "")
			continue
		}
		blankCount = 0
		normalized = append(normalized, line)
	}
	return strings.TrimSpace(strings.Join(normalized, "\n"))
}

type base64LineEncoder struct {
	w io.Writer
	b []byte
}

func base64NewLineEncoder(w io.Writer) *base64LineEncoder {
	return &base64LineEncoder{w: w}
}

func (e *base64LineEncoder) Write(p []byte) (int, error) {
	e.b = append(e.b, p...)
	return len(p), nil
}

func (e *base64LineEncoder) Close() error {
	enc := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	for len(e.b) > 0 {
		chunk := e.b
		if len(chunk) > 57 {
			chunk = chunk[:57]
		}
		out := make([]byte, ((len(chunk)+2)/3)*4)
		j := 0
		for i := 0; i < len(chunk); i += 3 {
			var a, b, c byte
			a = chunk[i]
			if i+1 < len(chunk) {
				b = chunk[i+1]
			}
			if i+2 < len(chunk) {
				c = chunk[i+2]
			}
			out[j] = enc[a>>2]
			out[j+1] = enc[((a&0x03)<<4)|(b>>4)]
			if i+1 < len(chunk) {
				out[j+2] = enc[((b&0x0f)<<2)|(c>>6)]
			} else {
				out[j+2] = '='
			}
			if i+2 < len(chunk) {
				out[j+3] = enc[c&0x3f]
			} else {
				out[j+3] = '='
			}
			j += 4
		}
		if _, err := e.w.Write(out); err != nil {
			return err
		}
		if _, err := io.WriteString(e.w, "\r\n"); err != nil {
			return err
		}
		e.b = e.b[len(chunk):]
	}
	return nil
}

func parseMessage(raw []byte, messageID, mailbox string, uid uint32) (Message, error) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return Message{}, err
	}

	msg := Message{ID: messageID, Mailbox: mailbox, UID: uid}
	if from, err := mr.Header.AddressList("From"); err == nil && len(from) > 0 {
		msg.From = from[0].String()
	}
	if to, err := mr.Header.AddressList("To"); err == nil {
		msg.To = mailAddressStrings(to)
	}
	if subject, err := mr.Header.Subject(); err == nil {
		msg.Subject = subject
	}
	if date, err := mr.Header.Date(); err == nil {
		msg.Date = date
	}

	partIdx := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := h.ContentType()
			if strings.HasPrefix(ct, "text/plain") || ct == "" {
				body, _ := io.ReadAll(io.LimitReader(part.Body, maxBodyBytes))
				if len(msg.Body) == 0 {
					msg.Body = string(body)
				}
			}
		case *mail.AttachmentHeader:
			partIdx++
			filename, _ := h.Filename()
			ct, _, _ := h.ContentType()
			if ct == "" {
				ct = "application/octet-stream"
			}
			size, _ := io.Copy(io.Discard, io.LimitReader(part.Body, maxAttachmentBytes+1))
			if size > maxAttachmentBytes {
				size = maxAttachmentBytes
			}
			meta := AttachmentMeta{ID: EncodeAttachmentID(messageID, partIdx), Filename: filename, ContentType: ct, Size: size}
			msg.Attachments = append(msg.Attachments, meta)
		}
	}

	return msg, nil
}

func extractAttachment(raw []byte, messageID string, targetPart int) (AttachmentMeta, []byte, error) {
	if targetPart <= 0 {
		return AttachmentMeta{}, nil, fs.ErrNotExist
	}
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return AttachmentMeta{}, nil, err
	}
	partIdx := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		h, ok := part.Header.(*mail.AttachmentHeader)
		if !ok {
			continue
		}
		partIdx++
		if partIdx != targetPart {
			continue
		}
		filename, _ := h.Filename()
		ct, _, _ := h.ContentType()
		if ct == "" {
			ct = "application/octet-stream"
		}
		data, err := io.ReadAll(io.LimitReader(part.Body, maxAttachmentBytes))
		if err != nil {
			return AttachmentMeta{}, nil, err
		}
		meta := AttachmentMeta{
			ID:          EncodeAttachmentID(messageID, targetPart),
			Filename:    filename,
			ContentType: ct,
			Size:        int64(len(data)),
		}
		return meta, data, nil
	}
	return AttachmentMeta{}, nil, fmt.Errorf("attachment not found")
}

func mailAddressStrings(in []*mail.Address) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		if a == nil {
			continue
		}
		out = append(out, a.String())
	}
	return out
}

func envelopeFirstAddress(addrs []*imap.Address) string {
	if len(addrs) == 0 || addrs[0] == nil {
		return ""
	}
	if addrs[0].PersonalName != "" {
		return fmt.Sprintf("%s <%s>", addrs[0].PersonalName, addrs[0].Address())
	}
	return addrs[0].Address()
}

func hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

func canonicalFlag(v string) string {
	s := strings.TrimSpace(strings.ToLower(v))
	switch s {
	case "seen", "\\seen":
		return imap.SeenFlag
	case "answered", "\\answered":
		return imap.AnsweredFlag
	case "flagged", "\\flagged":
		return imap.FlaggedFlag
	case "deleted", "\\deleted":
		return imap.DeletedFlag
	case "draft", "\\draft":
		return imap.DraftFlag
	default:
		if strings.HasPrefix(v, "\\") {
			return v
		}
		if s == "" {
			return imap.SeenFlag
		}
		return "\\" + strings.ToUpper(s[:1]) + s[1:]
	}
}
