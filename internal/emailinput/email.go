package emailinput

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"io"
	"mime"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxEmailBytes = int64(64 * 1024 * 1024)
	maxMIMEParts  = 1000
	maxMIMEDepth  = 16
)

type Message struct {
	RootDir     string
	HTMLPath    string
	Attachments []Attachment
	cleanup     func()
}

type Attachment struct {
	Name       string
	MediaType  string
	Bytes      int64
	Inline     bool
	AssetPaths []string
}

type HeaderField struct {
	Name  string
	Value string
}

type Document struct {
	Subject        string
	Headers        []HeaderField
	HTMLBodies     []string
	TextBodies     []string
	Attachments    []Attachment
	CIDAssets      map[string]string
	LocationAssets map[string]string
}

type parserState struct {
	root           string
	label          string
	htmlBodies     []string
	textBodies     []string
	attachments    []Attachment
	cidAssets      map[string]string
	locationAssets map[string]string
	partCount      int
	assetCount     int
}

var headerDecoder = &mime.WordDecoder{}

func Open(inputPath string) (Message, error) {
	return openMIME(inputPath, "EML", "doc7-eml-", "email.html")
}

func OpenMHTML(inputPath string) (Message, error) {
	return openMIME(inputPath, "MHTML", "doc7-mhtml-", "mhtml.html")
}

func openMIME(inputPath string, label string, temporaryPrefix string, htmlName string) (Message, error) {
	data, err := readBoundedFile(inputPath, label)
	if err != nil {
		return Message{}, err
	}
	parsed, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return Message{}, fmt.Errorf("failed to parse %s message: %w", label, err)
	}
	temporary, err := os.MkdirTemp("", temporaryPrefix+"*")
	if err != nil {
		return Message{}, err
	}
	cleanup := func() { _ = os.RemoveAll(temporary) }
	state := &parserState{root: temporary, label: label, cidAssets: map[string]string{}, locationAssets: map[string]string{}}
	body, err := readBoundedReader(parsed.Body, maxEmailBytes, label+" body")
	if err != nil {
		cleanup()
		return Message{}, err
	}
	if err := parseEntity(textproto.MIMEHeader(parsed.Header), body, state, 0); err != nil {
		cleanup()
		return Message{}, err
	}
	htmlPath := filepath.Join(temporary, htmlName)
	document := buildDocument(parsed.Header, state)
	if err := os.WriteFile(htmlPath, []byte(document), 0o644); err != nil {
		cleanup()
		return Message{}, err
	}
	return Message{RootDir: temporary, HTMLPath: htmlPath, Attachments: state.attachments, cleanup: cleanup}, nil
}

func (message *Message) Close() {
	if message.cleanup != nil {
		message.cleanup()
		message.cleanup = nil
	}
}

func readBoundedFile(path string, label string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s input is not a regular file: %s", label, path)
	}
	if info.Size() > maxEmailBytes {
		return nil, fmt.Errorf("%s input exceeds %d bytes", label, maxEmailBytes)
	}
	return readBoundedReader(file, maxEmailBytes, label+" input")
}

func readBoundedReader(reader io.Reader, limit int64, label string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", label, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, limit)
	}
	return data, nil
}

func buildDocument(header mail.Header, state *parserState) string {
	fields := make([]HeaderField, 0, 6)
	for _, name := range []string{"From", "To", "Cc", "Reply-To", "Date", "Message-ID"} {
		if value := decodedHeader(header.Get(name)); value != "" {
			fields = append(fields, HeaderField{Name: name, Value: value})
		}
	}
	return BuildDocument(Document{
		Subject:        decodedHeader(header.Get("Subject")),
		Headers:        fields,
		HTMLBodies:     state.htmlBodies,
		TextBodies:     state.textBodies,
		Attachments:    state.attachments,
		CIDAssets:      state.cidAssets,
		LocationAssets: state.locationAssets,
	})
}

func BuildDocument(document Document) string {
	body := ""
	if len(document.HTMLBodies) > 0 {
		body = sanitizeHTML(strings.Join(document.HTMLBodies, "\n<hr>\n"), document.CIDAssets, document.LocationAssets)
	} else if len(document.TextBodies) > 0 {
		body = "<pre class=\"plain-text\">" + stdhtml.EscapeString(strings.Join(document.TextBodies, "\n\n")) + "</pre>"
	} else {
		body = "<p class=\"empty\">This message has no readable body.</p>"
	}

	var builder strings.Builder
	builder.WriteString("<!doctype html><html><head><meta charset=\"utf-8\">")
	builder.WriteString(`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; font-src 'self' data:">`)
	builder.WriteString(`<style>body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;margin:32px;color:#171717;line-height:1.5}header{border-bottom:1px solid #d4d4d4;margin-bottom:24px;padding-bottom:18px}h1{font-size:24px;margin:0 0 16px}dl{display:grid;grid-template-columns:max-content 1fr;gap:6px 16px;margin:0}dt{font-weight:600;color:#525252}dd{margin:0;overflow-wrap:anywhere}.message{max-width:960px}.plain-text{white-space:pre-wrap;font:inherit}.attachments{border-top:1px solid #d4d4d4;margin-top:28px;padding-top:18px}.attachments img{max-width:100%;height:auto}.empty{color:#737373}</style>`)
	builder.WriteString("</head><body><header>")
	subject := strings.TrimSpace(document.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	builder.WriteString("<h1>" + stdhtml.EscapeString(subject) + "</h1><dl>")
	for _, field := range document.Headers {
		name := strings.TrimSpace(field.Name)
		value := strings.TrimSpace(field.Value)
		if name == "" || value == "" {
			continue
		}
		builder.WriteString("<dt>" + stdhtml.EscapeString(name) + "</dt><dd>" + stdhtml.EscapeString(value) + "</dd>")
	}
	builder.WriteString("</dl></header><main class=\"message\">")
	builder.WriteString(body)
	writeAttachments(&builder, document.Attachments)
	builder.WriteString("</main></body></html>")
	return builder.String()
}

func writeAttachments(builder *strings.Builder, attachments []Attachment) {
	if len(attachments) == 0 {
		return
	}
	builder.WriteString("<section class=\"attachments\"><h2>Attachments</h2><ul>")
	for _, attachment := range attachments {
		name := attachment.Name
		if name == "" {
			name = "unnamed attachment"
		}
		builder.WriteString("<li>" + stdhtml.EscapeString(name))
		if attachment.MediaType != "" {
			builder.WriteString(" — " + stdhtml.EscapeString(attachment.MediaType))
		}
		builder.WriteString(fmt.Sprintf(" — %d bytes</li>", attachment.Bytes))
	}
	builder.WriteString("</ul>")
	for _, attachment := range attachments {
		if len(attachment.AssetPaths) == 0 || attachment.Inline || !strings.HasPrefix(attachment.MediaType, "image/") {
			continue
		}
		for index, assetPath := range attachment.AssetPaths {
			caption := attachment.Name
			if len(attachment.AssetPaths) > 1 {
				caption = fmt.Sprintf("%s — page %d", caption, index+1)
			}
			builder.WriteString("<figure><img src=\"" + stdhtml.EscapeString(assetPath) + "\" alt=\"" + stdhtml.EscapeString(caption) + "\"><figcaption>" + stdhtml.EscapeString(caption) + "</figcaption></figure>")
		}
	}
	builder.WriteString("</section>")
}

func decodedHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	decoded, err := headerDecoder.DecodeHeader(value)
	if err != nil {
		return value
	}
	return strings.TrimSpace(decoded)
}
