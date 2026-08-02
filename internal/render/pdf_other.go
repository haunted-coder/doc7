//go:build !darwin

package render

import (
	"context"

	"github.com/magicrew/doc7/internal/vlm"
)

func renderPDFWithPDFKit(ctx context.Context, path string, outputDir string, dpi int) error {
	return vlm.NewError(vlm.DependencyError, "PDFKit renderer is only available on macOS", false, nil)
}
