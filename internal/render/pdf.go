package render

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/magicrew/doc7/internal/toolpath"
	"github.com/magicrew/doc7/internal/vlm"
)

func renderPDF(ctx context.Context, path string, imagesDir string, dpi int, textGrounding bool) ([]PageImage, error) {
	tmp, err := os.MkdirTemp("", "doc7-pdf-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	var attempts []string
	renderedDir := filepath.Join(tmp, "mupdf")
	if err := os.MkdirAll(renderedDir, 0o755); err != nil {
		return nil, err
	}
	if err := renderPDFWithMuPDF(ctx, path, renderedDir, dpi); err != nil {
		attempts = append(attempts, err.Error())
	} else {
		return collectRenderedPages(ctx, path, renderedDir, imagesDir, textGrounding)
	}

	renderedDir = filepath.Join(tmp, "magick")
	if err := os.MkdirAll(renderedDir, 0o755); err != nil {
		return nil, err
	}
	if err := renderPDFWithImageMagick(ctx, path, renderedDir, dpi); err != nil {
		attempts = append(attempts, err.Error())
	} else {
		return collectRenderedPages(ctx, path, renderedDir, imagesDir, textGrounding)
	}

	renderedDir = filepath.Join(tmp, "pdfkit")
	if err := os.MkdirAll(renderedDir, 0o755); err != nil {
		return nil, err
	}
	if err := renderPDFWithPDFKit(ctx, path, renderedDir, dpi); err != nil {
		attempts = append(attempts, err.Error())
	} else {
		return collectRenderedPages(ctx, path, renderedDir, imagesDir, textGrounding)
	}

	return nil, vlm.NewError(vlm.RenderError, "failed to render PDF: "+strings.Join(attempts, " | "), false, nil)
}

func renderPDFWithImageMagick(ctx context.Context, path string, outputDir string, dpi int) error {
	magick, err := exec.LookPath("magick")
	if err != nil {
		return vlm.NewError(vlm.DependencyError, "ImageMagick 'magick' is required for PDF rendering", false, err)
	}
	pattern := filepath.Join(outputDir, "page_%03d.png")
	cmd := exec.CommandContext(ctx, magick, "-density", fmt.Sprint(dpi), path, "-alpha", "remove", "-background", "white", "-quality", "100", pattern)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return vlm.NewError(vlm.RenderError, "ImageMagick PDF renderer failed: "+strings.TrimSpace(string(output)), false, err)
	}
	return nil
}

func renderPDFWithMuPDF(ctx context.Context, path string, outputDir string, dpi int) error {
	mutool := FindMuTool()
	if mutool == "" {
		return vlm.NewError(vlm.DependencyError, "MuPDF 'mutool' is required for portable PDF rendering", false, nil)
	}
	pattern := filepath.Join(outputDir, "page_%03d.png")
	cmd := exec.CommandContext(ctx, mutool, "draw", "-r", fmt.Sprint(dpi), "-o", pattern, path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return vlm.NewError(vlm.RenderError, "MuPDF PDF renderer failed: "+strings.TrimSpace(string(output)), false, err)
	}
	return nil
}

func FindMuTool() string {
	return toolpath.ResolveExecutable(
		toolpath.FromEnv("DOC7_MUTOOL_PATH"),
		toolpath.NearExecutable("tools", "mupdf", "mutool.exe"),
		toolpath.NearExecutable("tools", "MuPDF", "mutool.exe"),
		"/opt/homebrew/opt/mupdf-tools/bin/mutool",
		"/opt/homebrew/opt/mupdf/bin/mutool",
		"/usr/local/opt/mupdf-tools/bin/mutool",
		"/usr/local/opt/mupdf/bin/mutool",
		"mutool",
		"mutool.exe",
	)
}

func collectRenderedPages(ctx context.Context, sourcePath string, renderedDir string, imagesDir string, textGrounding bool) ([]PageImage, error) {
	files, err := filepath.Glob(filepath.Join(renderedDir, "page_*.png"))
	if err != nil {
		return nil, err
	}
	sortRenderedPages(files)
	if len(files) == 0 {
		return nil, vlm.NewError(vlm.RenderError, "PDF renderer produced no images", false, nil)
	}
	pages := make([]PageImage, 0, len(files))
	for i, file := range files {
		dst := filepath.Join(imagesDir, pageName(i+1, ".png"))
		if err := copyFile(file, dst); err != nil {
			return nil, err
		}
		hash, err := hashFile(dst)
		if err != nil {
			return nil, err
		}
		width, height := imageSize(dst)
		pages = append(pages, PageImage{Page: i + 1, SourcePath: sourcePath, ImagePath: dst, Width: width, Height: height, SHA256: hash})
	}
	if textGrounding {
		for index := range pages {
			pages[index].EmbeddedTextSupported = true
		}
		attachEmbeddedText(ctx, sourcePath, pages)
	}
	return pages, nil
}

func sortRenderedPages(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		left, leftOK := renderedPageNumber(paths[i])
		right, rightOK := renderedPageNumber(paths[j])
		if leftOK && rightOK && left != right {
			return left < right
		}
		return paths[i] < paths[j]
	})
}

func renderedPageNumber(path string) (int, bool) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if !strings.HasPrefix(name, "page_") {
		return 0, false
	}
	page, err := strconv.Atoi(strings.TrimPrefix(name, "page_"))
	if err != nil || page < 0 {
		return 0, false
	}
	return page, true
}
