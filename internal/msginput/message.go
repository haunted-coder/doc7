package msginput

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AkmalOt/gomsg"
	"github.com/magicrew/doc7/internal/emailinput"
	"golang.org/x/net/html/charset"
)

const maxMSGBytes = int64(64 * 1024 * 1024)

type Message struct {
	RootDir  string
	HTMLPath string
	cleanup  func()
}

func Open(path string) (Message, error) {
	file, err := os.Open(path)
	if err != nil {
		return Message{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Message{}, err
	}
	if !info.Mode().IsRegular() {
		return Message{}, fmt.Errorf("MSG input is not a regular file: %s", path)
	}
	if info.Size() > maxMSGBytes {
		return Message{}, fmt.Errorf("MSG input exceeds %d bytes", maxMSGBytes)
	}
	parsed, err := gomsg.OpenReader(file, info.Size())
	if err != nil {
		return Message{}, fmt.Errorf("failed to parse Outlook MSG: %w", err)
	}

	temporary, err := os.MkdirTemp("", "doc7-msg-*")
	if err != nil {
		return Message{}, err
	}
	cleanup := func() { _ = os.RemoveAll(temporary) }
	attachments, cidAssets, err := extractAttachments(temporary, parsed.Attachments)
	if err != nil {
		cleanup()
		return Message{}, err
	}
	htmlBodies, textBodies := messageBodies(parsed)
	document := emailinput.BuildDocument(emailinput.Document{
		Subject:     cleanText(parsed.Subject),
		Headers:     messageHeaders(parsed),
		HTMLBodies:  htmlBodies,
		TextBodies:  textBodies,
		Attachments: attachments,
		CIDAssets:   cidAssets,
	})
	htmlPath := filepath.Join(temporary, "message.html")
	if err := os.WriteFile(htmlPath, []byte(document), 0o644); err != nil {
		cleanup()
		return Message{}, err
	}
	return Message{RootDir: temporary, HTMLPath: htmlPath, cleanup: cleanup}, nil
}

func (message *Message) Close() {
	if message.cleanup != nil {
		message.cleanup()
		message.cleanup = nil
	}
}

func messageBodies(message *gomsg.Message) ([]string, []string) {
	htmlBody := decodedHTMLBody(message.BodyHTML)
	textBody := cleanText(message.Body)
	var htmlBodies []string
	var textBodies []string
	if htmlBody != "" {
		htmlBodies = append(htmlBodies, htmlBody)
	}
	if textBody != "" {
		textBodies = append(textBodies, textBody)
	}
	return htmlBodies, textBodies
}

func decodedHTMLBody(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if utf8.Valid(data) {
		return cleanText(string(data))
	}
	reader, err := charset.NewReader(bytes.NewReader(data), "text/html")
	if err != nil {
		return cleanText(string(data))
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, maxMSGBytes+1))
	if err != nil || int64(len(decoded)) > maxMSGBytes {
		return cleanText(string(data))
	}
	return cleanText(string(decoded))
}

func messageHeaders(message *gomsg.Message) []emailinput.HeaderField {
	fields := make([]emailinput.HeaderField, 0, 10)
	appendHeader := func(name string, value string) {
		if value = cleanText(value); value != "" {
			fields = append(fields, emailinput.HeaderField{Name: name, Value: value})
		}
	}
	appendHeader("From", formattedAddress(message.SenderName, firstNonEmpty(message.SenderSMTP, message.SenderEmail)))
	appendHeader("To", recipientList(message, gomsg.RecipientTo, message.DisplayTo))
	appendHeader("Cc", recipientList(message, gomsg.RecipientCC, message.DisplayCC))
	appendHeader("Bcc", recipientList(message, gomsg.RecipientBCC, message.DisplayBCC))
	if !message.Date.IsZero() {
		appendHeader("Date", message.Date.Format(time.RFC1123Z))
	}
	if !message.DeliveryTime.IsZero() && !message.DeliveryTime.Equal(message.Date) {
		appendHeader("Delivered", message.DeliveryTime.Format(time.RFC1123Z))
	}
	appendHeader("Message-ID", message.MessageID)
	appendHeader("In-Reply-To", message.InReplyTo)
	appendHeader("Message-Class", message.MessageClass)
	appendHeader("Importance", importanceName(message.Importance))
	return fields
}

func recipientList(message *gomsg.Message, recipientType gomsg.RecipientType, fallback string) string {
	values := make([]string, 0, len(message.Recipients))
	for _, recipient := range message.Recipients {
		if recipient.Type != recipientType {
			continue
		}
		value := formattedAddress(recipient.DisplayName, firstNonEmpty(recipient.SMTPAddress, recipient.EmailAddress))
		if value != "" {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, ", ")
}

func formattedAddress(name string, address string) string {
	name = cleanText(name)
	address = cleanText(address)
	if name == "" {
		return address
	}
	if address == "" || strings.EqualFold(name, address) {
		return name
	}
	return name + " <" + address + ">"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if cleanText(value) != "" {
			return value
		}
	}
	return ""
}

func importanceName(value gomsg.Importance) string {
	switch value {
	case gomsg.ImportanceLow:
		return "Low"
	case gomsg.ImportanceHigh:
		return "High"
	default:
		return "Normal"
	}
}

func cleanText(value string) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	return strings.Trim(value, "\x00 \t\r\n")
}
