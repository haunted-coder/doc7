package render

import (
	"context"

	"github.com/magicrew/doc7/internal/emailinput"
	"github.com/magicrew/doc7/internal/vlm"
)

func renderMHTML(ctx context.Context, path string, imagesDir string, options Options) ([]PageImage, error) {
	message, err := emailinput.OpenMHTML(path)
	if err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to parse MHTML: "+err.Error(), false, err)
	}
	defer message.Close()
	pages, err := renderHTMLPage(ctx, message.HTMLPath, imagesDir, options)
	if err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to render MHTML", false, err)
	}
	return pages, nil
}
