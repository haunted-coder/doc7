package render

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/vlm"
)

func renderHTMLSlides(ctx context.Context, path string, imagesDir string, options Options) ([]PageImage, error) {
	index := filepath.Join(path, "index.html")
	if _, err := os.Stat(index); err != nil {
		return nil, vlm.NewError(vlm.RenderError, "HTML slides directory must contain index.html", false, err)
	}
	chrome := options.ChromePath
	if chrome == "" {
		chrome = findChrome()
	}
	if chrome == "" {
		return nil, vlm.NewError(vlm.DependencyError, "Chrome, Chromium, or Edge is required for HTML slide screenshots", false, nil)
	}
	dst := filepath.Join(imagesDir, pageName(1, ".png"))
	cmd := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--hide-scrollbars", "--allow-file-access-from-files", "--window-size=1920,1080", "--screenshot="+dst, localFileURL(index))
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to screenshot HTML slides: "+string(output), false, err)
	}
	hash, err := hashFile(dst)
	if err != nil {
		return nil, err
	}
	width, height := imageSize(dst)
	return []PageImage{{Page: 1, SourcePath: index, ImagePath: dst, Width: width, Height: height, SHA256: hash}}, nil
}

func renderHTMLPage(ctx context.Context, path string, imagesDir string, options Options) ([]PageImage, error) {
	chrome := options.ChromePath
	if chrome == "" {
		chrome = findChrome()
	}
	if chrome == "" {
		return nil, vlm.NewError(vlm.DependencyError, "Chrome, Chromium, or Edge is required for HTML rendering", false, nil)
	}
	tmp, err := os.MkdirTemp("", "doc7-html-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	pdfPath := filepath.Join(tmp, "document.pdf")
	cmd := exec.CommandContext(ctx, chrome,
		"--headless=new",
		"--disable-gpu",
		"--allow-file-access-from-files",
		"--no-pdf-header-footer",
		"--print-to-pdf="+pdfPath,
		localFileURL(path),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to print HTML to PDF: "+strings.TrimSpace(string(output)), false, err)
	}
	if _, err := os.Stat(pdfPath); err != nil {
		return nil, vlm.NewError(vlm.RenderError, "Chrome did not produce an HTML PDF", false, err)
	}
	return renderPDF(ctx, pdfPath, imagesDir, options.DPI, options.TextGrounding)
}

func localFileURL(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	slashPath := filepath.ToSlash(absolute)
	if volume := filepath.VolumeName(absolute); volume != "" && !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	return (&url.URL{Scheme: "file", Path: slashPath}).String()
}

func findChrome() string {
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		"google-chrome",
		"chromium",
		"chrome",
		"msedge",
	}
	for _, candidate := range candidates {
		if filepath.IsAbs(candidate) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}
