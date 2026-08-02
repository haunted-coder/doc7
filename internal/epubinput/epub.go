package epubinput

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/archiveinput"
)

type Book struct {
	RootDir  string
	Chapters []Chapter
	cleanup  func()
}

type Chapter struct {
	ID   string
	Href string
	Path string
}

type containerDocument struct {
	Rootfiles []containerRootfile `xml:"rootfiles>rootfile"`
}

type containerRootfile struct {
	FullPath  string `xml:"full-path,attr"`
	MediaType string `xml:"media-type,attr"`
}

type packageDocument struct {
	Manifest struct {
		Items []packageItem `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Items []spineItem `xml:"itemref"`
	} `xml:"spine"`
}

type packageItem struct {
	ID        string `xml:"id,attr"`
	Href      string `xml:"href,attr"`
	MediaType string `xml:"media-type,attr"`
}

type spineItem struct {
	IDRef string `xml:"idref,attr"`
}

func Open(inputPath string) (Book, error) {
	temporary, err := os.MkdirTemp("", "doc7-epub-*")
	if err != nil {
		return Book{}, err
	}
	cleanup := func() { _ = os.RemoveAll(temporary) }
	extracted, err := archiveinput.ExtractZIP(inputPath, temporary, archiveinput.DefaultOptions())
	if err != nil {
		cleanup()
		return Book{}, fmt.Errorf("failed to expand EPUB: %w", err)
	}
	root := extracted.ContentRoot
	opfRelative, err := packagePath(root)
	if err != nil {
		cleanup()
		return Book{}, err
	}
	opfPath, err := resolvePath(root, opfRelative)
	if err != nil {
		cleanup()
		return Book{}, err
	}
	data, err := os.ReadFile(opfPath)
	if err != nil {
		cleanup()
		return Book{}, fmt.Errorf("failed to read EPUB package document: %w", err)
	}
	var document packageDocument
	if err := xml.Unmarshal(data, &document); err != nil {
		cleanup()
		return Book{}, fmt.Errorf("failed to parse EPUB package document: %w", err)
	}
	manifest := make(map[string]packageItem, len(document.Manifest.Items))
	for _, item := range document.Manifest.Items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, exists := manifest[id]; !exists {
			manifest[id] = item
		}
	}
	chapters := make([]Chapter, 0, len(document.Spine.Items))
	packageDir := path.Dir(opfRelative)
	for _, reference := range document.Spine.Items {
		item, exists := manifest[strings.TrimSpace(reference.IDRef)]
		if !exists || !isRenderableItem(item) {
			continue
		}
		relative, err := cleanRelativePath(packageDir, item.Href)
		if err != nil {
			cleanup()
			return Book{}, err
		}
		chapterPath, err := resolvePath(root, relative)
		if err != nil {
			cleanup()
			return Book{}, err
		}
		info, err := os.Stat(chapterPath)
		if err != nil || !info.Mode().IsRegular() {
			cleanup()
			if err == nil {
				err = fmt.Errorf("not a regular file")
			}
			return Book{}, fmt.Errorf("failed to access EPUB chapter %q: %w", item.Href, err)
		}
		chapters = append(chapters, Chapter{ID: item.ID, Href: item.Href, Path: chapterPath})
	}
	if len(chapters) == 0 {
		cleanup()
		return Book{}, fmt.Errorf("EPUB spine does not contain renderable chapters")
	}
	return Book{RootDir: root, Chapters: chapters, cleanup: cleanup}, nil
}

func (book *Book) Close() {
	if book.cleanup != nil {
		book.cleanup()
		book.cleanup = nil
	}
}

func packagePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "META-INF", "container.xml"))
	if err != nil {
		return "", fmt.Errorf("failed to read EPUB container.xml: %w", err)
	}
	var container containerDocument
	if err := xml.Unmarshal(data, &container); err != nil {
		return "", fmt.Errorf("failed to parse EPUB container.xml: %w", err)
	}
	if len(container.Rootfiles) == 0 {
		return "", fmt.Errorf("EPUB container.xml does not contain a rootfile")
	}
	selected := container.Rootfiles[0]
	for _, rootfile := range container.Rootfiles {
		if strings.EqualFold(strings.TrimSpace(rootfile.MediaType), "application/oebps-package+xml") {
			selected = rootfile
			break
		}
	}
	return cleanRelativePath("", selected.FullPath)
}

func cleanRelativePath(base string, value string) (string, error) {
	value, _, _ = strings.Cut(strings.TrimSpace(value), "#")
	value, _, _ = strings.Cut(value, "?")
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return "", fmt.Errorf("invalid EPUB path %q: %w", value, err)
	}
	decoded = strings.ReplaceAll(decoded, "\\", "/")
	if decoded == "" || strings.HasPrefix(decoded, "/") {
		return "", fmt.Errorf("invalid EPUB path: %q", value)
	}
	cleaned := path.Clean(path.Join(base, decoded))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("EPUB path escapes package root: %q", value)
	}
	return cleaned, nil
}

func resolvePath(root string, relative string) (string, error) {
	rootAbsolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(filepath.Join(rootAbsolute, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	inside, err := filepath.Rel(rootAbsolute, candidate)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("EPUB path escapes extracted root: %q", relative)
	}
	return candidate, nil
}

func isRenderableItem(item packageItem) bool {
	switch strings.ToLower(strings.TrimSpace(item.MediaType)) {
	case "application/xhtml+xml", "text/html", "image/svg+xml":
		return true
	}
	switch strings.ToLower(path.Ext(strings.TrimSpace(item.Href))) {
	case ".xhtml", ".html", ".htm", ".svg":
		return true
	default:
		return false
	}
}
