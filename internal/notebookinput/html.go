package notebookinput

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/magicrew/doc7/internal/emailinput"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var markdownRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote))

type documentBuilder struct {
	root       string
	cidAssets  map[string]string
	assetCount int
}

func (builder *documentBuilder) build(cells []rawCell) (string, error) {
	var result strings.Builder
	result.WriteString(`<div class="notebook">`)
	for index, cell := range cells {
		html, err := builder.buildCell(index+1, cell)
		if err != nil {
			return "", fmt.Errorf("failed to render notebook cell %d: %w", index+1, err)
		}
		result.WriteString(html)
	}
	result.WriteString(`</div>`)
	return result.String(), nil
}

func (builder *documentBuilder) buildCell(index int, cell rawCell) (string, error) {
	source, err := textValue(cell.Source)
	if err != nil {
		return "", err
	}
	style := "border:1px solid #d4d4d4;margin:0 0 18px;padding:16px;break-inside:avoid;"
	var result strings.Builder
	result.WriteString(`<section style="` + style + `">`)
	switch cell.CellType {
	case "markdown":
		source, err = builder.prepareMarkdownAttachments(source, cell.Attachments)
		if err != nil {
			return "", err
		}
		rendered, err := renderMarkdown(source)
		if err != nil {
			return "", err
		}
		result.WriteString(rendered)
	case "code":
		result.WriteString(`<div style="font-size:12px;color:#525252;margin-bottom:8px">In [` + stdhtml.EscapeString(executionCount(cell.ExecutionCount)) + `]</div>`)
		result.WriteString(`<pre class="plain-text" style="background:#f5f5f5;padding:12px;overflow-wrap:anywhere"><code>` + stdhtml.EscapeString(source) + `</code></pre>`)
		for outputIndex, output := range cell.Outputs {
			rendered, err := builder.buildOutput(index, outputIndex+1, output)
			if err != nil {
				return "", err
			}
			result.WriteString(rendered)
		}
	case "raw":
		result.WriteString(`<pre class="plain-text">` + stdhtml.EscapeString(source) + `</pre>`)
	default:
		result.WriteString(`<div style="font-size:12px;color:#737373">Cell ` + fmt.Sprintf("%d", index) + ` — ` + stdhtml.EscapeString(cell.CellType) + `</div>`)
		result.WriteString(`<pre class="plain-text">` + stdhtml.EscapeString(source) + `</pre>`)
	}
	result.WriteString(`</section>`)
	return result.String(), nil
}

func (builder *documentBuilder) prepareMarkdownAttachments(source string, attachments map[string]map[string]json.RawMessage) (string, error) {
	names := make([]string, 0, len(attachments))
	for name := range attachments {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		bundle := attachments[name]
		for _, mediaType := range []string{"image/svg+xml", "image/png", "image/jpeg", "image/gif", "image/webp"} {
			raw, exists := bundle[mediaType]
			if !exists {
				continue
			}
			path, cid, err := builder.writeImageOutput(mediaType, raw)
			if err != nil {
				return "", err
			}
			builder.cidAssets[cid] = path
			for _, reference := range []string{"attachment:" + name, "attachment:" + url.PathEscape(name)} {
				source = strings.ReplaceAll(source, reference, "cid:"+cid)
			}
			break
		}
	}
	return source, nil
}

func (builder *documentBuilder) buildOutput(cellIndex int, outputIndex int, output rawOutput) (string, error) {
	label := fmt.Sprintf("Output %d.%d", cellIndex, outputIndex)
	style := "border-left:3px solid #a3a3a3;margin-top:12px;padding:8px 12px;"
	var result strings.Builder
	result.WriteString(`<div style="` + style + `"><div style="font-size:12px;color:#525252;margin-bottom:6px">` + label + `</div>`)
	switch output.OutputType {
	case "stream":
		text, err := textValue(output.Text)
		if err != nil {
			return "", err
		}
		result.WriteString(`<pre class="plain-text">` + stdhtml.EscapeString(text) + `</pre>`)
	case "error":
		result.WriteString(`<div style="color:#b91c1c;font-weight:600">` + stdhtml.EscapeString(strings.TrimSpace(output.EName+": "+output.EValue)) + `</div>`)
		result.WriteString(`<pre class="plain-text" style="color:#991b1b">` + stdhtml.EscapeString(strings.Join(output.Traceback, "\n")) + `</pre>`)
	case "display_data", "execute_result":
		rendered, err := builder.renderData(output.Data, label)
		if err != nil {
			return "", err
		}
		result.WriteString(rendered)
	default:
		text, err := textValue(output.Text)
		if err != nil {
			return "", err
		}
		result.WriteString(`<pre class="plain-text">` + stdhtml.EscapeString(text) + `</pre>`)
	}
	result.WriteString(`</div>`)
	return result.String(), nil
}

func (builder *documentBuilder) renderData(data map[string]json.RawMessage, label string) (string, error) {
	for _, mediaType := range []string{"image/svg+xml", "image/png", "image/jpeg", "image/gif", "image/webp"} {
		if raw, exists := data[mediaType]; exists {
			path, cid, err := builder.writeImageOutput(mediaType, raw)
			if err != nil {
				return "", err
			}
			builder.cidAssets[cid] = path
			return `<figure><img src="cid:` + cid + `" alt="` + stdhtml.EscapeString(label) + `"><figcaption>` + stdhtml.EscapeString(label) + `</figcaption></figure>`, nil
		}
	}
	if raw, exists := data["text/html"]; exists {
		value, err := textValue(raw)
		if err != nil {
			return "", err
		}
		return value, nil
	}
	if raw, exists := data["text/markdown"]; exists {
		value, err := textValue(raw)
		if err != nil {
			return "", err
		}
		return renderMarkdown(value)
	}
	if raw, exists := data["application/json"]; exists {
		return `<pre class="plain-text">` + stdhtml.EscapeString(prettyJSON(raw)) + `</pre>`, nil
	}
	if raw, exists := data["text/plain"]; exists {
		value, err := textValue(raw)
		if err != nil {
			return "", err
		}
		return `<pre class="plain-text">` + stdhtml.EscapeString(value) + `</pre>`, nil
	}
	return `<p class="empty">Output has no renderable representation.</p>`, nil
}

func (builder *documentBuilder) writeImageOutput(mediaType string, raw json.RawMessage) (string, string, error) {
	value, err := textValue(raw)
	if err != nil {
		return "", "", err
	}
	extension := outputExtension(mediaType)
	var data []byte
	if mediaType == "image/svg+xml" && strings.Contains(value, "<svg") {
		cleaned := emailinput.SanitizeHTML(value, nil)
		if !strings.Contains(strings.ToLower(cleaned), "<svg") {
			return "", "", fmt.Errorf("SVG output did not contain a renderable SVG element")
		}
		data = []byte(cleaned)
	} else {
		data, err = base64.StdEncoding.DecodeString(strings.Join(strings.Fields(value), ""))
		if err != nil {
			return "", "", fmt.Errorf("failed to decode %s notebook output: %w", mediaType, err)
		}
	}
	if int64(len(data)) > maxNotebookBytes {
		return "", "", fmt.Errorf("notebook image output exceeds %d bytes", maxNotebookBytes)
	}
	builder.assetCount++
	filename := fmt.Sprintf("output-%03d%s", builder.assetCount, extension)
	assetsDir := filepath.Join(builder.root, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(assetsDir, filename), data, 0o644); err != nil {
		return "", "", err
	}
	cid := fmt.Sprintf("notebook-output-%03d", builder.assetCount)
	return filepath.ToSlash(filepath.Join("assets", filename)), cid, nil
}

func renderMarkdown(source string) (string, error) {
	var output bytes.Buffer
	if err := markdownRenderer.Convert([]byte(source), &output); err != nil {
		return "", err
	}
	return output.String(), nil
}

func executionCount(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return " "
	}
	return strings.Trim(trimmed, `"`)
}

func outputExtension(mediaType string) string {
	switch mediaType {
	case "image/svg+xml":
		return ".svg"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func prettyJSON(raw json.RawMessage) string {
	var output bytes.Buffer
	if json.Indent(&output, raw, "", "  ") == nil {
		return output.String()
	}
	value, err := textValue(raw)
	if err == nil {
		return value
	}
	return string(raw)
}
