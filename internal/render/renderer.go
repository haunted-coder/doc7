package render

import (
	"context"
	"encoding/json"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/vlm"
)

type Options struct {
	OutputDir            string
	DPI                  int
	Pages                string
	KeepImages           bool
	ChromePath           string
	LibreOfficePath      string
	PresentationRenderer string
	TextGrounding        bool
}

type PageImage struct {
	Page                  int    `json:"page"`
	SourcePath            string `json:"source_path"`
	ImagePath             string `json:"image_path"`
	Width                 int    `json:"width"`
	Height                int    `json:"height"`
	SHA256                string `json:"sha256"`
	EmbeddedTextSHA256    string `json:"embedded_text_sha256,omitempty"`
	EmbeddedTextChars     int    `json:"embedded_text_chars,omitempty"`
	EmbeddedTextTruncated bool   `json:"embedded_text_truncated,omitempty"`
	EmbeddedTextSupported bool   `json:"embedded_text_supported,omitempty"`
	EmbeddedTextChecked   bool   `json:"embedded_text_checked,omitempty"`
	EmbeddedText          string `json:"-"`
}

type Result struct {
	Input           detect.Input `json:"input"`
	Pages           []PageImage  `json:"pages"`
	SourcePageCount int          `json:"source_page_count"`
	PageSelection   string       `json:"page_selection,omitempty"`
	OutputDir       string       `json:"output_dir"`
	StartedAt       time.Time    `json:"started_at"`
	FinishedAt      time.Time    `json:"finished_at"`
}

type Manifest struct {
	Version string       `json:"version"`
	Command string       `json:"command"`
	Input   detect.Input `json:"input"`
	Render  struct {
		DPI             int    `json:"dpi"`
		PageCount       int    `json:"page_count"`
		SourcePageCount int    `json:"source_page_count,omitempty"`
		PageSelection   string `json:"page_selection,omitempty"`
		ImageFormat     string `json:"image_format"`
		OutputDir       string `json:"output_dir"`
	} `json:"render"`
	Pages      []PageImage `json:"pages"`
	StartedAt  time.Time   `json:"started_at"`
	FinishedAt time.Time   `json:"finished_at"`
}

func Render(ctx context.Context, input detect.Input, options Options) (Result, error) {
	if options.DPI <= 0 {
		options.DPI = 220
	}
	if options.OutputDir == "" {
		options.OutputDir = input.Name + "_output"
	}
	imagesDir := filepath.Join(options.OutputDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return Result{}, err
	}
	started := time.Now()
	var pages []PageImage
	var err error
	switch input.Kind {
	case detect.KindImage:
		pages, err = renderSingleImage(input.Path, imagesDir)
	case detect.KindImageDir:
		pages, err = renderImageDir(input.Files, imagesDir)
	case detect.KindPDF:
		pages, err = renderPDF(ctx, input.Path, imagesDir, options.DPI, options.TextGrounding)
	case detect.KindHTML, detect.KindSVG:
		pages, err = renderHTMLPage(ctx, input.Path, imagesDir, options)
	case detect.KindEPUB:
		pages, err = renderEPUB(ctx, input.Path, imagesDir, options)
	case detect.KindEML:
		pages, err = renderEML(ctx, input.Path, imagesDir, options)
	case detect.KindMHTML:
		pages, err = renderMHTML(ctx, input.Path, imagesDir, options)
	case detect.KindMSG:
		pages, err = renderMSG(ctx, input.Path, imagesDir, options)
	case detect.KindIPYNB:
		pages, err = renderIPYNB(ctx, input.Path, imagesDir, options)
	case detect.KindHTMLSlides:
		pages, err = renderHTMLSlides(ctx, input.Path, imagesDir, options)
	default:
		if detect.IsOffice(input.Kind) {
			pages, err = renderOffice(ctx, input.Path, imagesDir, options)
		} else {
			err = vlm.NewError(vlm.RenderError, "unsupported render input type: "+string(input.Kind), false, nil)
		}
	}
	if err != nil {
		return Result{}, err
	}
	selection, err := ParsePageSelection(options.Pages)
	if err != nil {
		return Result{}, vlm.NewError(vlm.ConfigError, "invalid page selection", false, err)
	}
	sourcePageCount := len(pages)
	pages, err = selection.filter(pages)
	if err != nil {
		return Result{}, vlm.NewError(vlm.ConfigError, "invalid page selection", false, err)
	}
	return Result{Input: input, Pages: pages, SourcePageCount: sourcePageCount, PageSelection: selection.String(), OutputDir: options.OutputDir, StartedAt: started, FinishedAt: time.Now()}, nil
}

func WriteManifest(path string, command string, result Result, dpi int) error {
	pages := make([]PageImage, len(result.Pages))
	for index, page := range result.Pages {
		pages[index] = page
		pages[index].ImagePath = RelativeArtifactPath(result.OutputDir, page.ImagePath)
	}
	manifest := Manifest{Version: "1", Command: command, Input: result.Input, Pages: pages, StartedAt: result.StartedAt, FinishedAt: result.FinishedAt}
	manifest.Render.DPI = dpi
	manifest.Render.PageCount = len(result.Pages)
	manifest.Render.SourcePageCount = result.SourcePageCount
	manifest.Render.PageSelection = result.PageSelection
	manifest.Render.ImageFormat = "png"
	manifest.Render.OutputDir = "images"
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// RelativeArtifactPath keeps persisted artifact references valid after the
// complete output directory is moved to another machine.
func RelativeArtifactPath(root string, path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	absoluteRoot, rootErr := filepath.Abs(root)
	absolutePath, pathErr := filepath.Abs(path)
	if rootErr != nil || pathErr != nil {
		return filepath.ToSlash(path)
	}
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func imageSize(path string) (int, int) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}
