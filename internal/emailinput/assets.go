package emailinput

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/magicrew/doc7/internal/rasterinput"
)

func WriteAttachmentAssets(root string, index int, filename string, mediaType string, data []byte) ([]string, error) {
	assetsDir := filepath.Join(root, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return nil, err
	}
	reader := bytes.NewReader(data)
	decodedPages, normalized, err := rasterinput.DecodePages(reader, int64(len(data)), filename, mediaType)
	if err != nil {
		return nil, err
	}
	if normalized {
		return writeNormalizedAssets(assetsDir, index, filename, decodedPages)
	}
	name := SafeAttachmentName(filename)
	if name == "" {
		name = fmt.Sprintf("attachment-%03d%s", index, extensionForMediaType(mediaType))
	} else {
		name = fmt.Sprintf("%03d-%s", index, name)
	}
	path := filepath.Join(assetsDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	return []string{filepath.ToSlash(filepath.Join("assets", name))}, nil
}

func writeNormalizedAssets(assetsDir string, index int, filename string, pages []image.Image) ([]string, error) {
	name := SafeAttachmentName(filename)
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if base == "" {
		base = "attachment"
	}
	paths := make([]string, 0, len(pages))
	for pageIndex, page := range pages {
		filename := fmt.Sprintf("%03d-%s.png", index, base)
		if len(pages) > 1 {
			filename = fmt.Sprintf("%03d-%s-page-%03d.png", index, base, pageIndex+1)
		}
		path := filepath.Join(assetsDir, filename)
		if err := writePNG(path, page); err != nil {
			return nil, err
		}
		paths = append(paths, filepath.ToSlash(filepath.Join("assets", filename)))
	}
	return paths, nil
}

func writePNG(path string, source image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encodeErr := png.Encode(file, source)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func SafeAttachmentName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\\|?*`, r) {
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
	extension := filepath.Ext(name)
	base := strings.TrimSuffix(name, extension)
	if windowsReservedName(base) {
		base = "_" + base
	}
	const maxComponentRunes = 120
	keep := maxComponentRunes - len([]rune(extension))
	if keep < 1 {
		keep = 1
	}
	baseRunes := []rune(base)
	if len(baseRunes) > keep {
		base = string(baseRunes[:keep])
	}
	return base + extension
}

func windowsReservedName(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

func extensionForMediaType(mediaType string) string {
	extensions, err := mime.ExtensionsByType(mediaType)
	if err == nil && len(extensions) > 0 {
		return extensions[0]
	}
	return ""
}
