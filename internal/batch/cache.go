package batch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/extract"
)

func manifestArtifactsComplete(manifest extract.Manifest, outputDir string) bool {
	if manifest.Summary.PagesTotal <= 0 || len(manifest.Pages) != manifest.Summary.PagesTotal {
		return false
	}
	seenPages := make(map[int]struct{}, len(manifest.Pages))
	for _, page := range manifest.Pages {
		if page.Page <= 0 || page.Status != extract.StatusSuccess || page.CacheKey == "" {
			return false
		}
		if _, exists := seenPages[page.Page]; exists {
			return false
		}
		seenPages[page.Page] = struct{}{}
		markdownPath, ok := artifactPath(outputDir, page.MarkdownPath)
		if !ok || !nonEmptyRegularFile(markdownPath) {
			return false
		}
		metaPath, ok := artifactPath(outputDir, page.MetaPath)
		if !ok || !nonEmptyRegularFile(metaPath) {
			return false
		}
		meta, ok := readPageMeta(metaPath)
		if !ok || meta.Page != page.Page || meta.Status != extract.StatusSuccess || meta.CacheKey != page.CacheKey {
			return false
		}
		if manifest.Render.ImageFormat == "none" {
			continue
		}
		imagePath, ok := artifactPath(outputDir, page.ImagePath)
		if !ok || !nonEmptyRegularFile(imagePath) {
			return false
		}
		imageHash, err := sha256File(imagePath)
		if err != nil || imageHash != meta.ImageSHA256 {
			return false
		}
	}
	return true
}

func artifactPath(root string, recorded string) (string, bool) {
	recorded = strings.TrimSpace(recorded)
	if recorded == "" {
		return "", false
	}
	if !filepath.IsAbs(recorded) {
		recorded = filepath.Join(root, recorded)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absolutePath, err := filepath.Abs(recorded)
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return absolutePath, true
}

func nonEmptyRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func readPageMeta(path string) (extract.PageMeta, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return extract.PageMeta{}, false
	}
	var meta extract.PageMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return extract.PageMeta{}, false
	}
	return meta, true
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
