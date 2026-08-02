package render

import (
	"context"
	"os"
	"path/filepath"

	"github.com/magicrew/doc7/internal/epubinput"
	"github.com/magicrew/doc7/internal/vlm"
)

func renderEPUB(ctx context.Context, path string, imagesDir string, options Options) ([]PageImage, error) {
	book, err := epubinput.Open(path)
	if err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to parse EPUB: "+err.Error(), false, err)
	}
	defer book.Close()
	temporary, err := os.MkdirTemp("", "doc7-epub-pages-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)

	pages := []PageImage{}
	for chapterIndex, chapter := range book.Chapters {
		if err := ctx.Err(); err != nil {
			return nil, vlm.NewError(vlm.TimeoutError, "EPUB rendering canceled", false, err)
		}
		chapterImages := filepath.Join(temporary, pageName(chapterIndex+1, ""))
		if err := os.MkdirAll(chapterImages, 0o755); err != nil {
			return nil, err
		}
		chapterPages, err := renderHTMLPage(ctx, chapter.Path, chapterImages, options)
		if err != nil {
			return nil, vlm.NewError(vlm.RenderError, "failed to render EPUB chapter "+chapter.Href, false, err)
		}
		for _, chapterPage := range chapterPages {
			number := len(pages) + 1
			destination := filepath.Join(imagesDir, pageName(number, ".png"))
			if err := copyFile(chapterPage.ImagePath, destination); err != nil {
				return nil, err
			}
			hash, err := hashFile(destination)
			if err != nil {
				return nil, err
			}
			width, height := imageSize(destination)
			pages = append(pages, PageImage{
				Page:       number,
				SourcePath: chapter.Path,
				ImagePath:  destination,
				Width:      width,
				Height:     height,
				SHA256:     hash,
			})
		}
	}
	if len(pages) == 0 {
		return nil, vlm.NewError(vlm.RenderError, "EPUB produced no renderable pages", false, nil)
	}
	return pages, nil
}
