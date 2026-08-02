package detect

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Kind string

const (
	KindPDF        Kind = "pdf"
	KindDOC        Kind = "doc"
	KindDOCX       Kind = "docx"
	KindODT        Kind = "odt"
	KindRTF        Kind = "rtf"
	KindPPT        Kind = "ppt"
	KindPPTX       Kind = "pptx"
	KindODP        Kind = "odp"
	KindXLS        Kind = "xls"
	KindXLSX       Kind = "xlsx"
	KindODS        Kind = "ods"
	KindHTML       Kind = "html"
	KindSVG        Kind = "svg"
	KindMarkdown   Kind = "markdown"
	KindText       Kind = "text"
	KindCSV        Kind = "csv"
	KindTSV        Kind = "tsv"
	KindJSON       Kind = "json"
	KindXML        Kind = "xml"
	KindYAML       Kind = "yaml"
	KindEPUB       Kind = "epub"
	KindEML        Kind = "eml"
	KindMHTML      Kind = "mhtml"
	KindMSG        Kind = "msg"
	KindIPYNB      Kind = "ipynb"
	KindImage      Kind = "image"
	KindImageDir   Kind = "image_dir"
	KindHTMLSlides Kind = "html_slides"
)

type Input struct {
	Path      string   `json:"path"`
	SourceURL string   `json:"source_url,omitempty"`
	Name      string   `json:"name"`
	Kind      Kind     `json:"kind"`
	Files     []string `json:"files,omitempty"`
	SHA256    string   `json:"sha256"`
}

func Detect(path string) (Input, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Input{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return Input{}, err
	}
	if info.IsDir() {
		return detectDir(absolute)
	}
	return detectFile(absolute)
}

func detectFile(path string) (Input, error) {
	ext := strings.ToLower(filepath.Ext(path))
	kind, ok := kindForExtension(ext)
	if !ok {
		return Input{}, errors.New("unsupported input type: " + ext)
	}
	hash, err := hashFile(path)
	if err != nil {
		return Input{}, err
	}
	return Input{Path: path, Name: trimExt(filepath.Base(path)), Kind: kind, SHA256: hash}, nil
}

func IsSupportedFile(path string) bool {
	_, ok := KindForExtension(filepath.Ext(path))
	return ok
}

func KindForExtension(extension string) (Kind, bool) {
	return kindForExtension(strings.ToLower(strings.TrimSpace(extension)))
}

func IsOffice(kind Kind) bool {
	switch kind {
	case KindDOC, KindDOCX, KindODT, KindRTF, KindPPT, KindPPTX, KindODP, KindXLS, KindXLSX, KindODS:
		return true
	default:
		return false
	}
}

func IsPresentation(kind Kind) bool {
	switch kind {
	case KindPPT, KindPPTX, KindODP:
		return true
	default:
		return false
	}
}

func IsNative(kind Kind) bool {
	switch kind {
	case KindMarkdown, KindText, KindCSV, KindTSV, KindJSON, KindXML, KindYAML:
		return true
	default:
		return false
	}
}

func kindForExtension(ext string) (Kind, bool) {
	switch ext {
	case ".pdf":
		return KindPDF, true
	case ".doc", ".dot":
		return KindDOC, true
	case ".docx", ".docm", ".dotx", ".dotm":
		return KindDOCX, true
	case ".odt":
		return KindODT, true
	case ".rtf":
		return KindRTF, true
	case ".ppt", ".pot", ".pps":
		return KindPPT, true
	case ".pptx", ".pptm", ".potx", ".potm", ".ppsx", ".ppsm":
		return KindPPTX, true
	case ".odp":
		return KindODP, true
	case ".xls", ".xlt":
		return KindXLS, true
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return KindXLSX, true
	case ".ods":
		return KindODS, true
	case ".html", ".htm":
		return KindHTML, true
	case ".svg":
		return KindSVG, true
	case ".md", ".markdown":
		return KindMarkdown, true
	case ".txt", ".text", ".log":
		return KindText, true
	case ".csv":
		return KindCSV, true
	case ".tsv":
		return KindTSV, true
	case ".json":
		return KindJSON, true
	case ".xml":
		return KindXML, true
	case ".yaml", ".yml":
		return KindYAML, true
	case ".epub":
		return KindEPUB, true
	case ".eml":
		return KindEML, true
	case ".mht", ".mhtml":
		return KindMHTML, true
	case ".msg":
		return KindMSG, true
	case ".ipynb":
		return KindIPYNB, true
	case ".bmp", ".png", ".jpg", ".jpeg", ".gif", ".tif", ".tiff", ".webp":
		return KindImage, true
	default:
		return "", false
	}
}

func detectDir(path string) (Input, error) {
	if exists(filepath.Join(path, "index.html")) || exists(filepath.Join(path, "magic.project.js")) {
		files, _ := dirFiles(path, nil)
		hash, err := hashFiles(files)
		if err != nil {
			return Input{}, err
		}
		return Input{Path: path, Name: filepath.Base(path), Kind: KindHTMLSlides, Files: files, SHA256: hash}, nil
	}
	files, err := dirFiles(path, isImage)
	if err != nil {
		return Input{}, err
	}
	if len(files) == 0 {
		return Input{}, errors.New("directory does not contain supported images or HTML slides")
	}
	sortStableImages(files)
	hash, err := hashFiles(files)
	if err != nil {
		return Input{}, err
	}
	return Input{Path: path, Name: filepath.Base(path), Kind: KindImageDir, Files: files, SHA256: hash}, nil
}

func IsImage(path string) bool {
	return isImage(path)
}

func isImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".bmp", ".png", ".jpg", ".jpeg", ".gif", ".tif", ".tiff", ".webp":
		return true
	default:
		return false
	}
}

func dirFiles(path string, filter func(string) bool) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		full := filepath.Join(path, entry.Name())
		if filter == nil || filter(full) {
			files = append(files, full)
		}
	}
	sort.Strings(files)
	return files, nil
}

func sortStableImages(files []string) {
	sort.SliceStable(files, func(i, j int) bool {
		ni, oi := firstNumber(filepath.Base(files[i]))
		nj, oj := firstNumber(filepath.Base(files[j]))
		if oi && oj && ni != nj {
			return ni < nj
		}
		return filepath.Base(files[i]) < filepath.Base(files[j])
	})
}

var numberPattern = regexp.MustCompile(`\d+`)

func firstNumber(name string) (int, bool) {
	match := numberPattern.FindString(name)
	if match == "" {
		return 0, false
	}
	value, err := strconv.Atoi(match)
	if err != nil {
		return 0, false
	}
	return value, true
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func trimExt(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func hashFile(path string) (string, error) {
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

func hashFiles(files []string) (string, error) {
	hash := sha256.New()
	for _, path := range files {
		hash.Write([]byte(filepath.Base(path)))
		hash.Write([]byte{0})
		value, err := hashFile(path)
		if err != nil {
			return "", err
		}
		hash.Write([]byte(value))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
