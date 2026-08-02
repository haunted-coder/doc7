package render

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	embeddedTextVersion  = "1"
	maxEmbeddedTextRunes = 12000
)

func EmbeddedTextVersion() string {
	return embeddedTextVersion
}

func attachEmbeddedText(ctx context.Context, path string, pages []PageImage) {
	if len(pages) == 0 || ctx.Err() != nil {
		return
	}
	texts, ok := extractEmbeddedTextWithMuPDF(ctx, path, len(pages))
	if !ok {
		texts, ok = extractEmbeddedTextWithPDFToText(ctx, path, len(pages))
	}
	if !ok {
		return
	}
	for index := range pages {
		pages[index].EmbeddedTextChecked = true
		if index >= len(texts) {
			break
		}
		text, truncated := normalizeEmbeddedText(texts[index])
		if text == "" {
			continue
		}
		pages[index].EmbeddedText = text
		pages[index].EmbeddedTextSHA256 = embeddedTextHash(text)
		pages[index].EmbeddedTextChars = utf8.RuneCountInString(text)
		pages[index].EmbeddedTextTruncated = truncated
	}
}

func extractEmbeddedTextWithMuPDF(ctx context.Context, path string, pageCount int) ([]string, bool) {
	mutool := FindMuTool()
	if mutool == "" {
		return nil, false
	}
	tmp, err := os.MkdirTemp("", "doc7-pdf-text-*")
	if err != nil {
		return nil, false
	}
	defer os.RemoveAll(tmp)
	pattern := filepath.Join(tmp, "page_%03d.txt")
	cmd := exec.CommandContext(ctx, mutool, "draw", "-q", "-o", pattern, path)
	if err := cmd.Run(); err != nil || ctx.Err() != nil {
		return nil, false
	}
	texts := make([]string, pageCount)
	for index := range texts {
		data, err := os.ReadFile(filepath.Join(tmp, fmt.Sprintf("page_%03d.txt", index+1)))
		if err != nil {
			continue
		}
		texts[index] = string(data)
	}
	return texts, true
}

func extractEmbeddedTextWithPDFToText(ctx context.Context, path string, pageCount int) ([]string, bool) {
	pdftotext, err := exec.LookPath("pdftotext")
	if err != nil {
		return nil, false
	}
	cmd := exec.CommandContext(ctx, pdftotext, "-layout", path, "-")
	output, err := cmd.Output()
	if err != nil || ctx.Err() != nil {
		return nil, false
	}
	parts := strings.Split(string(output), "\f")
	texts := make([]string, pageCount)
	for index := range texts {
		if index < len(parts) {
			texts[index] = parts[index]
		}
	}
	return texts, true
}

func normalizeEmbeddedText(value string) (string, bool) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	runes := []rune(value)
	if len(runes) <= maxEmbeddedTextRunes {
		return value, false
	}
	return strings.TrimSpace(string(runes[:maxEmbeddedTextRunes])), true
}

func embeddedTextHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
