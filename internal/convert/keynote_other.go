//go:build !darwin

package convert

import (
	"context"

	"github.com/magicrew/doc7/internal/vlm"
)

func convertWithKeynote(ctx context.Context, inputPath string, outputPath string) error {
	return vlm.NewError(vlm.DependencyError, "Keynote renderer is only available on macOS", false, nil)
}
