package sanitize

import (
	"bytes"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

var allowedTags = map[string]bool{
	"p": true, "div": true, "span": true, "br": true,
	"strong": true, "b": true, "em": true, "i": true, "u": true, "s": true,
	"blockquote": true, "pre": true, "code": true,
	"ul": true, "ol": true, "li": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"a": true, "img": true, "hr": true,
	"table": true, "thead": true, "tbody": true, "tr": true, "th": true, "td": true,
}

var blockedTags = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true, "embed": true,
	"form": true, "input": true, "button": true, "select": true, "textarea": true,
	"meta": true, "link": true, "base": true, "svg": true, "math": true,
}

func SanitizeHTML(input string) string {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return ""
	}

	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return ""
	}

	outRoot := &html.Node{Type: html.DocumentNode}
	for child := doc.FirstChild; child != nil; child = child.NextSibling {
		appendCleanNode(outRoot, child)
	}

	var buf bytes.Buffer
	for child := outRoot.FirstChild; child != nil; child = child.NextSibling {
		_ = html.Render(&buf, child)
	}
	return strings.TrimSpace(buf.String())
}

func appendCleanNode(parent, node *html.Node) {
	switch node.Type {
	case html.TextNode:
		parent.AppendChild(&html.Node{Type: html.TextNode, Data: node.Data})
		return
	case html.ElementNode:
		tag := strings.ToLower(strings.TrimSpace(node.Data))
		if blockedTags[tag] {
			return
		}
		if !allowedTags[tag] {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				appendCleanNode(parent, child)
			}
			return
		}

		clean := &html.Node{Type: html.ElementNode, Data: tag}
		clean.Attr = sanitizeAttrs(tag, node.Attr)
		parent.AppendChild(clean)
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			appendCleanNode(clean, child)
		}
		return
	default:
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			appendCleanNode(parent, child)
		}
	}
}

func sanitizeAttrs(tag string, attrs []html.Attribute) []html.Attribute {
	switch tag {
	case "a":
		return sanitizeAnchorAttrs(attrs)
	case "img":
		return sanitizeImageAttrs(attrs)
	default:
		// Keep no generic attributes on other tags to reduce attack surface.
		return nil
	}
}

func sanitizeAnchorAttrs(attrs []html.Attribute) []html.Attribute {
	out := make([]html.Attribute, 0, len(attrs))
	hasRel := false
	hasTargetBlank := false

	for _, attr := range attrs {
		key, val, ok := normalizedAttr(attr)
		if !ok {
			continue
		}

		switch key {
		case "href":
			if !isSafeHref(val) {
				continue
			}
			out = append(out, html.Attribute{Key: "href", Val: val})
		case "title":
			out = append(out, html.Attribute{Key: "title", Val: val})
		case "target":
			if val != "_blank" {
				continue
			}
			hasTargetBlank = true
			out = append(out, html.Attribute{Key: "target", Val: "_blank"})
		case "rel":
			hasRel = true
			out = append(out, html.Attribute{Key: "rel", Val: val})
		}
	}

	if hasTargetBlank && !hasRel {
		out = append(out, html.Attribute{Key: "rel", Val: "noopener noreferrer"})
	}

	return out
}

func sanitizeImageAttrs(attrs []html.Attribute) []html.Attribute {
	out := make([]html.Attribute, 0, len(attrs))

	for _, attr := range attrs {
		key, val, ok := normalizedAttr(attr)
		if !ok {
			continue
		}

		switch key {
		case "src":
			if !isSafeImageSrc(val) {
				continue
			}
			out = append(out, html.Attribute{Key: "src", Val: val})
		case "alt", "title", "width", "height":
			out = append(out, html.Attribute{Key: key, Val: val})
		}
	}

	return out
}

func normalizedAttr(attr html.Attribute) (key, val string, ok bool) {
	key = strings.ToLower(strings.TrimSpace(attr.Key))
	val = strings.TrimSpace(attr.Val)
	if key == "" || strings.HasPrefix(key, "on") {
		return "", "", false
	}
	return key, val, true
}

func isSafeHref(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme == "" {
		return true
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return true
	default:
		return false
	}
}

func isSafeImageSrc(raw string) bool {
	if raw == "" {
		return false
	}
	l := strings.ToLower(raw)
	if strings.HasPrefix(l, "data:image/") || strings.HasPrefix(l, "cid:") {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme == "" {
		return true
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}
