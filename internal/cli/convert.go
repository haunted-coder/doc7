package cli

import (
	"path/filepath"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/convert"
	"github.com/magicrew/doc7/internal/detect"
	"github.com/spf13/cobra"
)

func newConvertCommand() *cobra.Command {
	var output string
	var pptRenderer string
	cmd := &cobra.Command{
		Use:     "to-pdf <path>",
		Aliases: []string{"convert"},
		Short:   translate("to_pdf.short"),
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(config.FlagConfig{PPTRenderer: pptRenderer}, false)
			if err != nil {
				return err
			}
			input, err := detect.Detect(args[0])
			if err != nil {
				return err
			}
			if output == "" {
				output = input.Name + "_converted"
			}
			result, err := convert.ToPDF(cmd.Context(), input, convert.Options{OutputDir: output, Renderer: cfg.PPTRenderer})
			if err != nil {
				return err
			}
			summary := map[string]interface{}{
				"ok":          true,
				"command":     "convert",
				"input":       input.Path,
				"output":      result.OutputPath,
				"output_dir":  filepath.Dir(result.OutputPath),
				"renderer":    result.Renderer,
				"passthrough": result.Passthrough,
			}
			if cfg.JSONOutput {
				return writeJSON(summary)
			}
			if result.Passthrough {
				writeText("input is already PDF: %s", result.OutputPath)
			} else {
				writeText("converted to %s with %s", result.OutputPath, result.Renderer)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output directory")
	cmd.Flags().StringVar(&pptRenderer, "ppt-renderer", "", "presentation renderer: auto, libreoffice, or keynote")
	return cmd
}
