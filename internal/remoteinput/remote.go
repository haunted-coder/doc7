package remoteinput

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/magicrew/doc7/internal/detect"
)

const (
	defaultMaxBytes = int64(1024 * 1024 * 1024)
	defaultTimeout  = 10 * time.Minute
)

type Options struct {
	MaxBytes int64
	Timeout  time.Duration
	Client   *http.Client
}

type Result struct {
	SourceURL   string `json:"source_url"`
	FinalURL    string `json:"final_url"`
	LocalPath   string `json:"local_path"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
}

func DefaultMaxBytes() int64 {
	return defaultMaxBytes
}

func DefaultTimeout() time.Duration {
	return defaultTimeout
}

func IsHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func SuggestedName(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "download"
	}
	name := decodedBase(parsed.Path)
	if name == "" || name == "." || name == "/" {
		return "download"
	}
	name = sanitizeFilename(name)
	if name == "" {
		return "download"
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func Download(ctx context.Context, rawURL string, outputDir string, options Options) (Result, error) {
	if !IsHTTPURL(rawURL) {
		return Result{}, errors.New("remote input must use an http or https URL")
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = defaultMaxBytes
	}
	if options.Timeout <= 0 {
		options.Timeout = defaultTimeout
	}
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: options.Timeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", "doc7")
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Result{}, fmt.Errorf("remote server returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > options.MaxBytes {
		return Result{}, fmt.Errorf("remote input exceeds maximum size %d bytes", options.MaxBytes)
	}

	contentType := normalizedContentType(resp.Header.Get("Content-Type"))
	filename, err := responseFilename(resp, rawURL, contentType)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Result{}, err
	}
	temporary, err := os.CreateTemp(outputDir, ".doc7-download-*")
	if err != nil {
		return Result{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	written, copyErr := io.Copy(temporary, io.LimitReader(resp.Body, options.MaxBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		return Result{}, copyErr
	}
	if closeErr != nil {
		return Result{}, closeErr
	}
	if written > options.MaxBytes {
		return Result{}, fmt.Errorf("remote input exceeds maximum size %d bytes", options.MaxBytes)
	}

	destination := filepath.Join(outputDir, filename)
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return Result{}, err
	}
	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return Result{
		SourceURL:   RedactedURL(rawURL),
		FinalURL:    RedactedURL(finalURL),
		LocalPath:   destination,
		Filename:    filename,
		ContentType: contentType,
		Bytes:       written,
	}, nil
}

func RedactedURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func responseFilename(resp *http.Response, rawURL string, contentType string) (string, error) {
	name := contentDispositionFilename(resp.Header.Get("Content-Disposition"))
	if name == "" && resp.Request != nil && resp.Request.URL != nil {
		name = decodedBase(resp.Request.URL.Path)
	}
	if name == "" {
		parsed, _ := url.Parse(rawURL)
		if parsed != nil {
			name = decodedBase(parsed.Path)
		}
	}
	name = sanitizeFilename(name)
	ext := strings.ToLower(filepath.Ext(name))
	if !isSupportedExtension(ext) {
		inferred := extensionForContentType(contentType)
		if inferred == "" {
			return "", fmt.Errorf("unsupported remote content type %q and filename %q", contentType, name)
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if stem == "" {
			stem = "download"
		}
		name = stem + inferred
	}
	if name == "" {
		return "", errors.New("failed to determine remote filename")
	}
	return name, nil
}

func contentDispositionFilename(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	if encoded := params["filename*"]; encoded != "" {
		if separator := strings.Index(encoded, "''"); separator >= 0 {
			encoded = encoded[separator+2:]
		}
		if decoded, err := url.PathUnescape(encoded); err == nil {
			return decoded
		}
	}
	return params["filename"]
}

func normalizedContentType(value string) string {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return strings.ToLower(mediaType)
}

func extensionForContentType(value string) string {
	switch value {
	case "application/pdf":
		return ".pdf"
	case "application/msword":
		return ".doc"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.ms-word.document.macroenabled.12":
		return ".docm"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.template":
		return ".dotx"
	case "application/vnd.ms-word.template.macroenabled.12":
		return ".dotm"
	case "application/vnd.ms-powerpoint":
		return ".ppt"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ".pptx"
	case "application/vnd.ms-powerpoint.presentation.macroenabled.12":
		return ".pptm"
	case "application/vnd.openxmlformats-officedocument.presentationml.template":
		return ".potx"
	case "application/vnd.ms-powerpoint.template.macroenabled.12":
		return ".potm"
	case "application/vnd.openxmlformats-officedocument.presentationml.slideshow":
		return ".ppsx"
	case "application/vnd.ms-powerpoint.slideshow.macroenabled.12":
		return ".ppsm"
	case "application/vnd.ms-excel":
		return ".xls"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "application/vnd.ms-excel.sheet.macroenabled.12":
		return ".xlsm"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.template":
		return ".xltx"
	case "application/vnd.ms-excel.template.macroenabled.12":
		return ".xltm"
	case "application/vnd.oasis.opendocument.text":
		return ".odt"
	case "application/vnd.oasis.opendocument.presentation":
		return ".odp"
	case "application/vnd.oasis.opendocument.spreadsheet":
		return ".ods"
	case "application/rtf", "text/rtf":
		return ".rtf"
	case "text/html", "application/xhtml+xml":
		return ".html"
	case "image/png":
		return ".png"
	case "image/bmp", "image/x-ms-bmp":
		return ".bmp"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/tiff":
		return ".tiff"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "text/plain", "text/x-log":
		return ".txt"
	case "text/markdown":
		return ".md"
	case "text/csv":
		return ".csv"
	case "text/tab-separated-values":
		return ".tsv"
	case "application/json", "text/json":
		return ".json"
	case "application/x-ipynb+json":
		return ".ipynb"
	case "application/xml", "text/xml":
		return ".xml"
	case "application/yaml", "text/yaml", "text/x-yaml":
		return ".yaml"
	case "application/epub+zip":
		return ".epub"
	case "message/rfc822":
		return ".eml"
	case "application/x-mimearchive", "multipart/related":
		return ".mhtml"
	case "application/vnd.ms-outlook":
		return ".msg"
	case "application/zip", "application/x-zip-compressed":
		return ".zip"
	default:
		return ""
	}
}

func isSupportedExtension(ext string) bool {
	if ext == ".zip" {
		return true
	}
	return detect.IsSupportedFile("input" + ext)
}

func decodedBase(value string) string {
	name := path.Base(value)
	decoded, err := url.PathUnescape(name)
	if err == nil {
		name = decoded
	}
	return name
}

func sanitizeFilename(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			if !lastUnderscore {
				builder.WriteByte('_')
				lastUnderscore = true
			}
			continue
		}
		builder.WriteRune(r)
		lastUnderscore = false
	}
	name := strings.Trim(builder.String(), " ._")
	if name == "" {
		return ""
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if len([]rune(base)) > 160 {
		base = string([]rune(base)[:160])
	}
	if isWindowsReservedName(base) {
		base = "_" + base
	}
	return base + ext
}

func isWindowsReservedName(value string) bool {
	upper := strings.ToUpper(strings.TrimSpace(value))
	switch upper {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}
