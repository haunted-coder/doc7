package render

import (
	"context"

	"github.com/magicrew/doc7/internal/notebookinput"
	"github.com/magicrew/doc7/internal/vlm"
)

func renderIPYNB(ctx context.Context, path string, imagesDir string, options Options) ([]PageImage, error) {
	notebook, err := notebookinput.Open(path)
	if err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to parse IPYNB: "+err.Error(), false, err)
	}
	defer notebook.Close()
	pages, err := renderHTMLPage(ctx, notebook.HTMLPath, imagesDir, options)
	if err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to render IPYNB notebook", false, err)
	}
	return pages, nil
}
