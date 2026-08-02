package render

import (
	"context"
	"os"

	"github.com/magicrew/doc7/internal/convert"
	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/vlm"
)

func renderOffice(ctx context.Context, path string, imagesDir string, options Options) ([]PageImage, error) {
	tmp, err := os.MkdirTemp("", "doc7-office-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	detected, err := detect.Detect(path)
	if err != nil {
		return nil, vlm.NewError(vlm.ConfigError, "failed to detect Office input", false, err)
	}
	result, err := convert.ToPDF(ctx, detected, convert.Options{OutputDir: tmp, Renderer: options.PresentationRenderer})
	if err != nil {
		return nil, err
	}
	return renderPDF(ctx, result.OutputPath, imagesDir, options.DPI, options.TextGrounding)
}
