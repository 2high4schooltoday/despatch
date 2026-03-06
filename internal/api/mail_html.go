package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"despatch/internal/mail"
)

var errRemoteImageMaxBytesExceeded = errors.New("remote image exceeds byte cap")
var remoteImageHTTPClientFactory = remoteImageHTTPClient

// maxBytesCopyWriter enforces a hard response-size cap while streaming bytes.
type maxBytesCopyWriter struct {
	w        io.Writer
	maxBytes int64
	written  int64
}

func (w *maxBytesCopyWriter) Write(p []byte) (int, error) {
	remaining := w.maxBytes - w.written
	if remaining <= 0 {
		return 0, errRemoteImageMaxBytesExceeded
	}
	if int64(len(p)) > remaining {
		n, err := w.w.Write(p[:remaining])
		w.written += int64(n)
		if err != nil {
			return n, err
		}
		return n, errRemoteImageMaxBytesExceeded
	}
	n, err := w.w.Write(p)
	w.written += int64(n)
	return n, err
}

func remoteImageHTTPClient() *http.Client {
	return &http.Client{
		Timeout: mailRemoteImageFetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= mailRemoteImageMaxRedirects {
				return errors.New("too many redirects")
			}
			_, err := validateRemoteImageTarget(req.Context(), req.URL.String())
			return err
		},
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
}

func validateRemoteImageTarget(ctx context.Context, raw string) (*url.URL, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return nil, fmt.Errorf("url query parameter is required")
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid remote image url")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("remote image url must use http or https")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("remote image url must not include credentials")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return nil, fmt.Errorf("remote image url host is required")
	}
	if isLocalhostStyleHost(host) {
		return nil, fmt.Errorf("remote image host is not allowed")
	}
	if err := validateRemoteImageHost(ctx, host); err != nil {
		return nil, err
	}
	return parsed, nil
}

func validateRemoteImageHost(ctx context.Context, host string) error {
	if ip := parseRemoteImageIP(host); ip != nil {
		if isBlockedRemoteImageIP(ip) {
			return fmt.Errorf("remote image host is not allowed")
		}
		return nil
	}
	resolved, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("remote image host cannot be resolved")
	}
	if len(resolved) == 0 {
		return fmt.Errorf("remote image host cannot be resolved")
	}
	for _, ip := range resolved {
		if isBlockedRemoteImageIP(ip) {
			return fmt.Errorf("remote image host is not allowed")
		}
	}
	return nil
}

func isLocalhostStyleHost(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	return isLoopbackHost(h)
}

func parseRemoteImageIP(host string) net.IP {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return nil
	}
	if i := strings.Index(trimmed, "%"); i > 0 {
		trimmed = trimmed[:i]
	}
	return net.ParseIP(trimmed)
}

func isBlockedRemoteImageIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if !ip.IsGlobalUnicast() {
		return true
	}
	return false
}

func rewriteMessageHTML(messageID, rawHTML string, attachments []mail.AttachmentMeta) string {
	if strings.TrimSpace(rawHTML) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return rawHTML
	}

	cidTargets := map[string]string{}
	for _, item := range attachments {
		key := normalizeCIDToken(item.ContentID)
		if key == "" {
			continue
		}
		if _, exists := cidTargets[key]; exists {
			continue
		}
		cidTargets[key] = "/api/v1/attachments/" + url.PathEscape(item.ID)
	}

	walkHTMLNodes(doc, func(node *html.Node) {
		if node.Type != html.ElementNode {
			return
		}
		tag := strings.ToLower(strings.TrimSpace(node.Data))
		if tag != "img" && tag != "source" {
			return
		}
		for i, attr := range node.Attr {
			key := strings.ToLower(strings.TrimSpace(attr.Key))
			switch key {
			case "src":
				node.Attr[i].Val = rewriteImageSource(messageID, attr.Val, cidTargets)
			case "srcset":
				node.Attr[i].Val = rewriteImageSourceSet(messageID, attr.Val, cidTargets)
			}
		}
	})

	var out bytes.Buffer
	bodyNode := findHTMLElement(doc, "body")
	if bodyNode != nil {
		for child := bodyNode.FirstChild; child != nil; child = child.NextSibling {
			if err := html.Render(&out, child); err != nil {
				return rawHTML
			}
		}
		return out.String()
	}
	if err := html.Render(&out, doc); err != nil {
		return rawHTML
	}
	return out.String()
}

func findHTMLElement(node *html.Node, name string) *html.Node {
	if node == nil {
		return nil
	}
	if node.Type == html.ElementNode && strings.EqualFold(strings.TrimSpace(node.Data), strings.TrimSpace(name)) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func walkHTMLNodes(node *html.Node, visit func(*html.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkHTMLNodes(child, visit)
	}
}

func rewriteImageSource(messageID, raw string, cidTargets map[string]string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "cid:") {
		key := normalizeCIDToken(strings.TrimSpace(trimmed[4:]))
		if target, ok := cidTargets[key]; ok {
			return target
		}
		return raw
	}
	resolved, ok := normalizeRemoteImageURL(trimmed)
	if !ok {
		return raw
	}
	return fmt.Sprintf("/api/v1/messages/%s/remote-image?url=%s", url.PathEscape(messageID), url.QueryEscape(resolved))
}

func rewriteImageSourceSet(messageID, raw string, cidTargets map[string]string) string {
	parts := strings.Split(raw, ",")
	changed := false
	for i, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		fields := strings.Fields(candidate)
		if len(fields) == 0 {
			continue
		}
		rewritten := rewriteImageSource(messageID, fields[0], cidTargets)
		if rewritten != fields[0] {
			fields[0] = rewritten
			changed = true
		}
		parts[i] = strings.Join(fields, " ")
	}
	if !changed {
		return raw
	}
	return strings.Join(parts, ", ")
}

func normalizeRemoteImageURL(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}
	if strings.HasPrefix(value, "//") {
		return "https:" + value, true
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", false
	}
	return parsed.String(), true
}

func normalizeCIDToken(raw string) string {
	value := strings.TrimSpace(raw)
	for {
		next := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(value, ">"), "<"))
		if next == value {
			break
		}
		value = next
	}
	return strings.ToLower(value)
}
