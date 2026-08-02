package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/pptx"
	"github.com/magicrew/doc7/internal/render"
	"github.com/spf13/cobra"
)

func newExportPPTXCommand() *cobra.Command {
	var output string
	var dpi int
	cmd := &cobra.Command{
		Use:   "export-pptx <slides_dir>",
		Short: translate("export_pptx.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localConfig := config.FlagConfig{DPI: dpi}
			applyChangedNumericFlags(cmd, &localConfig)
			cfg, err := loadConfig(localConfig, false)
			if err != nil {
				return err
			}
			input, err := detect.Detect(args[0])
			if err != nil {
				return err
			}
			if output == "" {
				output = input.Name + ".pptx"
			}
			tmp, err := os.MkdirTemp("", "doc7-export-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)
			result, err := render.Render(cmd.Context(), input, render.Options{OutputDir: tmp, DPI: cfg.DPI, KeepImages: true})
			if err != nil {
				return err
			}
			images := make([]string, 0, len(result.Pages))
			for _, page := range result.Pages {
				images = append(images, page.ImagePath)
			}
			if err := pptx.BuildFromImages(output, images); err != nil {
				return err
			}
			summary := map[string]interface{}{"ok": true, "command": "export-pptx", "input": input.Path, "output": output, "slides": len(images)}
			if cfg.JSONOutput {
				return writeJSON(summary)
			}
			writeText("wrote %s with %d slides", output, len(images))
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output PPTX")
	cmd.Flags().IntVar(&dpi, "dpi", 220, "render DPI")
	return cmd
}

func defaultPPTXName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base)) + ".pptx"
}
