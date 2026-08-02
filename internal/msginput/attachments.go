package msginput

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/AkmalOt/gomsg"
	"github.com/magicrew/doc7/internal/emailinput"
)

func extractAttachments(root string, source []gomsg.Attachment) ([]emailinput.Attachment, map[string]string, error) {
	attachments := make([]emailinput.Attachment, 0, len(source))
	cidAssets := make(map[string]string)
	assetCount := 0
	for index := range source {
		item := &source[index]
		name := cleanText(item.DisplayName())
		mediaType := attachmentMediaType(item, name)
		data := item.Data()
		inline := normalizedContentID(item.ContentID) != ""
		attachment := emailinput.Attachment{
			Name:      name,
			MediaType: mediaType,
			Bytes:     int64(len(data)),
			Inline:    inline,
		}
		if item.IsEmbeddedMessage() {
			attachment.MediaType = "application/vnd.ms-outlook"
			if attachment.Name == "" || attachment.Name == "untitled" {
				attachment.Name = "embedded-message.msg"
			}
			attachments = append(attachments, attachment)
			continue
		}
		if len(data) > 0 && (inline || strings.HasPrefix(mediaType, "image/")) {
			assetCount++
			assetPaths, err := emailinput.WriteAttachmentAssets(root, assetCount, name, mediaType, data)
			if err != nil {
				return nil, nil, err
			}
			attachment.AssetPaths = assetPaths
			if contentID := normalizedContentID(item.ContentID); contentID != "" && len(assetPaths) > 0 {
				cidAssets[contentID] = assetPaths[0]
			}
		}
		attachments = append(attachments, attachment)
	}
	return attachments, cidAssets, nil
}

func attachmentMediaType(attachment *gomsg.Attachment, name string) string {
	if value := normalizedMediaType(attachment.MIMEType); value != "" {
		return value
	}
	if value := normalizedMediaType(mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))); value != "" {
		return value
	}
	if data := attachment.Data(); len(data) > 0 {
		return normalizedMediaType(http.DetectContentType(data))
	}
	return "application/octet-stream"
}

func normalizedMediaType(value string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return strings.ToLower(mediaType)
}

func normalizedContentID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "<")
	value = strings.TrimSuffix(value, ">")
	return strings.ToLower(strings.TrimSpace(value))
}
