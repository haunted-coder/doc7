package cli

import (
	"path/filepath"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/render"
	"github.com/spf13/cobra"
)

func newRenderCommand() *cobra.Command {
	var output string
	var dpi int
	var pages string
	var pptRenderer string
	cmd := &cobra.Command{
		Use:   "render <path>",
		Short: translate("render.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localConfig := config.FlagConfig{DPI: dpi, PPTRenderer: pptRenderer}
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
				output = input.Name + "_output"
			}
			result, err := render.Render(cmd.Context(), input, render.Options{OutputDir: output, DPI: cfg.DPI, Pages: pages, KeepImages: true, PresentationRenderer: cfg.PPTRenderer})
			if err != nil {
				return err
			}
			manifest := filepath.Join(output, "manifest.json")
			if err := render.WriteManifest(manifest, "render", result, cfg.DPI); err != nil {
				return err
			}
			summary := map[string]interface{}{"ok": true, "command": "render", "input": input.Path, "output_dir": output, "manifest": manifest, "pages_total": len(result.Pages), "source_pages_total": result.SourcePageCount, "page_selection": result.PageSelection}
			if cfg.JSONOutput {
				return writeJSON(summary)
			}
			if result.SourcePageCount > len(result.Pages) {
				writeText("rendered %d of %d selected pages to %s", len(result.Pages), result.SourcePageCount, output)
			} else {
				writeText("rendered %d pages to %s", len(result.Pages), output)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output directory")
	cmd.Flags().IntVar(&dpi, "dpi", 220, "render DPI")
	cmd.Flags().StringVar(&pages, "pages", "", "1-based pages to render, for example 1,3-5")
	cmd.Flags().StringVar(&pptRenderer, "ppt-renderer", "", "presentation renderer: auto, libreoffice, or keynote")
	return cmd
}
