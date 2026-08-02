//go:build darwin

package convert

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/magicrew/doc7/internal/vlm"
)

func convertWithKeynote(ctx context.Context, inputPath string, outputPath string) error {
	if _, err := os.Stat("/Applications/Keynote.app"); err != nil {
		return vlm.NewError(vlm.DependencyError, "Keynote is not installed", false, err)
	}
	if _, err := exec.LookPath("osascript"); err != nil {
		return vlm.NewError(vlm.DependencyError, "Keynote renderer requires osascript", false, err)
	}
	script := `
on run argv
  set inputPath to item 1 of argv
  set outputPath to item 2 of argv
  tell application "Keynote"
    set theDoc to open POSIX file inputPath
    export theDoc to POSIX file outputPath as PDF
    close theDoc saving no
  end tell
end run`
	processCtx, cancel := context.WithTimeout(ctx, externalProcessTimeout)
	defer cancel()
	cmd := exec.CommandContext(processCtx, "osascript", "-e", script, inputPath, outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if processCtx.Err() != nil {
			return vlm.NewError(vlm.TimeoutError, "Keynote timed out while converting the presentation", true, processCtx.Err())
		}
		return vlm.NewError(vlm.RenderError, "Keynote failed to convert presentation: "+strings.TrimSpace(string(output)), false, err)
	}
	if _, err := os.Stat(outputPath); err != nil {
		return vlm.NewError(vlm.RenderError, "Keynote did not produce a PDF", false, err)
	}
	return nil
}
