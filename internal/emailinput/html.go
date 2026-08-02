package emailinput

import (
	"bytes"
	"net/url"
	"path"
	"strings"

	xhtml "golang.org/x/net/html"
)

func SanitizeHTML(raw string, cidAssets map[string]string) string {
	return sanitizeHTML(raw, cidAssets, nil)
}

func sanitizeHTML(raw string, cidAssets map[string]string, locationAssets map[string]string) string {
	document, err := xhtml.Parse(strings.NewReader(raw))
	if err != nil {
		return ""
	}
	sanitizeChildren(document, cidAssets, locationAssets)
	body := findElement(document, "body")
	if body == nil {
		body = document
	}
	var builder bytes.Buffer
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		_ = xhtml.Render(&builder, child)
	}
	return builder.String()
}

func sanitizeChildren(parent *xhtml.Node, cidAssets map[string]string, locationAssets map[string]string) {
	for child := parent.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == xhtml.ElementNode && unsafeElement(child.Data) {
			parent.RemoveChild(child)
			child = next
			continue
		}
		if child.Type == xhtml.ElementNode {
			attributes := child.Attr[:0]
			for _, attribute := range child.Attr {
				name := strings.ToLower(attribute.Key)
				if strings.HasPrefix(name, "on") {
					continue
				}
				switch name {
				case "src", "background", "poster":
					attribute.Val = safeResourceURL(attribute.Val, cidAssets, locationAssets)
					if attribute.Val == "" {
						continue
					}
				case "href":
					attribute.Val = safeReferenceURL(attribute.Val, cidAssets, locationAssets)
					if attribute.Val == "" {
						continue
					}
				case "srcset":
					continue
				case "style":
					lower := strings.ToLower(attribute.Val)
					if strings.Contains(lower, "url(") || strings.Contains(lower, "expression(") || strings.Contains(lower, "@import") {
						continue
					}
				}
				attributes = append(attributes, attribute)
			}
			child.Attr = attributes
		}
		sanitizeChildren(child, cidAssets, locationAssets)
		child = next
	}
}

func unsafeElement(name string) bool {
	switch strings.ToLower(name) {
	case "script", "iframe", "object", "embed", "form", "input", "button", "textarea", "select", "option", "base", "link", "meta", "style", "audio", "video", "source", "track":
		return true
	default:
		return false
	}
}

func safeResourceURL(value string, cidAssets map[string]string, locationAssets map[string]string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(trimmed), "cid:") {
		return replaceCID(trimmed, cidAssets)
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "data:image/") {
		return trimmed
	}
	for _, key := range locationLookupKeys(trimmed) {
		if asset, exists := locationAssets[key]; exists {
			return asset
		}
	}
	return ""
}

func safeReferenceURL(value string, cidAssets map[string]string, locationAssets map[string]string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "#") {
		return trimmed
	}
	return safeResourceURL(trimmed, cidAssets, locationAssets)
}

func locationLookupKeys(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	keys := []string{strings.ToLower(value)}
	parsed, err := url.Parse(value)
	if err == nil && parsed.Path != "" {
		keys = append(keys, strings.ToLower(parsed.Path), strings.ToLower(path.Base(parsed.Path)))
	}
	return keys
}

func replaceCID(value string, cidAssets map[string]string) string {
	value = strings.TrimSpace(value)
	identifier := strings.TrimSpace(value[4:])
	if decoded, err := url.PathUnescape(identifier); err == nil {
		identifier = decoded
	}
	if asset, exists := cidAssets[normalizedContentID(identifier)]; exists {
		return asset
	}
	return ""
}

func findElement(node *xhtml.Node, name string) *xhtml.Node {
	if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, name) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if match := findElement(child, name); match != nil {
			return match
		}
	}
	return nil
}
