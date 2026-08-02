package render

import (
	"context"

	"github.com/magicrew/doc7/internal/msginput"
	"github.com/magicrew/doc7/internal/vlm"
)

func renderMSG(ctx context.Context, path string, imagesDir string, options Options) ([]PageImage, error) {
	message, err := msginput.Open(path)
	if err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to parse MSG: "+err.Error(), false, err)
	}
	defer message.Close()
	pages, err := renderHTMLPage(ctx, message.HTMLPath, imagesDir, options)
	if err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to render MSG message", false, err)
	}
	return pages, nil
}
