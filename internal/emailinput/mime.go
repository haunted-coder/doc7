package emailinput

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html/charset"
)

func parseEntity(header textproto.MIMEHeader, body []byte, state *parserState, depth int) error {
	if depth > maxMIMEDepth {
		return fmt.Errorf("%s MIME nesting exceeds %d levels", state.label, maxMIMEDepth)
	}
	state.partCount++
	if state.partCount > maxMIMEParts {
		return fmt.Errorf("%s MIME part count exceeds %d", state.label, maxMIMEParts)
	}
	mediaType, parameters := parsedMediaType(header.Get("Content-Type"))
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := strings.TrimSpace(parameters["boundary"])
		if boundary == "" {
			return fmt.Errorf("multipart %s part is missing a boundary", state.label)
		}
		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to read %s MIME part: %w", state.label, err)
			}
			partBody, readErr := readBoundedReader(part, maxEmailBytes, state.label+" MIME part")
			closeErr := part.Close()
			if readErr != nil {
				return readErr
			}
			if closeErr != nil {
				return closeErr
			}
			if err := parseEntity(part.Header, partBody, state, depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	decoded, err := decodeTransferEncoding(body, header.Get("Content-Transfer-Encoding"))
	if err != nil {
		return err
	}
	if mediaType == "message/rfc822" {
		nested, err := mail.ReadMessage(bytes.NewReader(decoded))
		if err != nil {
			return fmt.Errorf("failed to parse nested %s message: %w", state.label, err)
		}
		nestedBody, err := readBoundedReader(nested.Body, maxEmailBytes, "nested "+state.label+" body")
		if err != nil {
			return err
		}
		return parseEntity(textproto.MIMEHeader(nested.Header), nestedBody, state, depth+1)
	}

	disposition, dispositionParameters := parsedMediaType(header.Get("Content-Disposition"))
	filename := decodedHeader(dispositionParameters["filename"])
	if filename == "" {
		filename = decodedHeader(parameters["name"])
	}
	contentLocation := strings.TrimSpace(header.Get("Content-Location"))
	if filename == "" && contentLocation != "" {
		filename = locationBaseName(contentLocation)
	}
	contentID := normalizedContentID(header.Get("Content-ID"))
	isAttachment := strings.EqualFold(disposition, "attachment")
	if !isAttachment && (mediaType == "text/plain" || mediaType == "text/html") {
		text, err := decodedText(decoded, parameters["charset"])
		if err != nil {
			return err
		}
		if mediaType == "text/html" {
			state.htmlBodies = append(state.htmlBodies, text)
		} else {
			state.textBodies = append(state.textBodies, text)
		}
		return nil
	}

	inline := strings.EqualFold(disposition, "inline") || contentID != ""
	attachment := Attachment{Name: filename, MediaType: mediaType, Bytes: int64(len(decoded)), Inline: inline}
	if inline || strings.HasPrefix(mediaType, "image/") {
		state.assetCount++
		assetPaths, err := WriteAttachmentAssets(state.root, state.assetCount, filename, mediaType, decoded)
		if err != nil {
			return err
		}
		attachment.AssetPaths = assetPaths
		if contentID != "" && len(assetPaths) > 0 {
			state.cidAssets[contentID] = assetPaths[0]
		}
		if contentLocation != "" && len(assetPaths) > 0 {
			for _, key := range locationKeys(contentLocation) {
				state.locationAssets[key] = assetPaths[0]
			}
		}
	}
	state.attachments = append(state.attachments, attachment)
	return nil
}

func parsedMediaType(value string) (string, map[string]string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "text/plain", map[string]string{}
	}
	mediaType, parameters, err := mime.ParseMediaType(value)
	if err == nil {
		return strings.ToLower(mediaType), parameters
	}
	mediaType, _, _ = strings.Cut(value, ";")
	return strings.ToLower(strings.TrimSpace(mediaType)), map[string]string{}
}

func decodeTransferEncoding(data []byte, encoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return readBoundedReader(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(data)), maxEmailBytes, "base64 MIME part")
	case "quoted-printable":
		return readBoundedReader(quotedprintable.NewReader(bytes.NewReader(data)), maxEmailBytes, "quoted-printable MIME part")
	default:
		return data, nil
	}
}

func decodedText(data []byte, label string) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" || strings.EqualFold(label, "utf-8") || strings.EqualFold(label, "us-ascii") {
		return string(data), nil
	}
	reader, err := charset.NewReaderLabel(label, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("unsupported MIME charset %q: %w", label, err)
	}
	decoded, err := readBoundedReader(reader, maxEmailBytes, "decoded MIME text")
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func normalizedContentID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "<")
	value = strings.TrimSuffix(value, ">")
	return strings.ToLower(strings.TrimSpace(value))
}

func locationBaseName(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err == nil && parsed.Path != "" {
		if name := path.Base(parsed.Path); name != "." && name != "/" {
			return name
		}
	}
	return path.Base(strings.TrimSpace(value))
}

func locationKeys(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	keys := []string{strings.ToLower(value)}
	parsed, err := url.Parse(value)
	if err == nil {
		if parsed.Path != "" {
			keys = append(keys, strings.ToLower(parsed.Path), strings.ToLower(path.Base(parsed.Path)))
		}
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}
