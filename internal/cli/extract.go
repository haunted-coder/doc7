package cli

import (
	"path/filepath"
	"time"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/extract"
	"github.com/magicrew/doc7/internal/refine"
	"github.com/magicrew/doc7/internal/render"
	"github.com/magicrew/doc7/internal/vlm"
	"github.com/spf13/cobra"
)

func newExtractCommand() *cobra.Command {
	var flags struct {
		Output            string
		Provider          string
		BaseURL           string
		Model             string
		APIKeyEnv         string
		CredentialStore   string
		CredentialAccount string
		CredentialsPath   string
		Workers           int
		Retry             int
		TextGrounding     bool
		Timeout           int
		DPI               int
		Pages             string
		PPTRenderer       string
		ImageDetail       string
		MaxImageMB        int
		MaxTokens         int
		ContextFallbacks  int
		MinImageDimension int
		Prompt            string
		PromptFile        string
		MergedName        string
		Refine            bool
		RefinedName       string
		RefineProfile     string
		RefineLanguage    string
		RefineMaxChars    int
		Merge             bool
		Resume            bool
		KeepImages        bool
	}
	flags.Merge = true
	flags.KeepImages = true
	cmd := &cobra.Command{
		Use:   "extract <path>",
		Short: translate("extract.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.Prompt != "" && flags.PromptFile != "" {
				return vlm.NewError(vlm.ConfigError, "use either --prompt or --prompt-file, not both", false, nil)
			}
			localConfig := config.FlagConfig{
				Provider:          flags.Provider,
				BaseURL:           flags.BaseURL,
				Model:             flags.Model,
				APIKeyEnv:         flags.APIKeyEnv,
				CredentialStore:   flags.CredentialStore,
				CredentialAccount: flags.CredentialAccount,
				CredentialsPath:   flags.CredentialsPath,
				Workers:           flags.Workers,
				DPI:               flags.DPI,
				PPTRenderer:       flags.PPTRenderer,
				ImageDetail:       flags.ImageDetail,
				MaxImageMB:        flags.MaxImageMB,
				MaxTokens:         flags.MaxTokens,
				ContextFallbacks:  &flags.ContextFallbacks,
				MinImageDimension: flags.MinImageDimension,
				TimeoutSeconds:    flags.Timeout,
				RetryCount:        flags.Retry,
				PromptName:        flags.Prompt,
				OutputDir:         flags.Output,
			}
			applyChangedNumericFlags(cmd, &localConfig)
			cfg, err := loadConfig(localConfig, false)
			if err != nil {
				return err
			}
			progress := newProgressReporter(cfg)
			ctx := cmd.Context()
			summary, err := extract.Run(ctx, args[0], extract.Options{
				OutputDir:     flags.Output,
				PromptName:    cfg.PromptName,
				PromptFile:    flags.PromptFile,
				MergedName:    flags.MergedName,
				Merge:         flags.Merge,
				Resume:        flags.Resume,
				Workers:       cfg.Workers,
				RetryCount:    cfg.RetryCount,
				TextGrounding: flags.TextGrounding,
				Progress:      progress.reportExtract,
				Render:        render.Options{OutputDir: flags.Output, DPI: cfg.DPI, Pages: flags.Pages, KeepImages: flags.KeepImages, PresentationRenderer: cfg.PPTRenderer},
				VLMConfig: vlm.Config{
					Provider:          cfg.Provider,
					BaseURL:           cfg.BaseURL,
					Model:             cfg.Model,
					APIKey:            cfg.APIKey,
					ImageDetail:       cfg.ImageDetail,
					MaxImageBytes:     int64(cfg.MaxImageMB) * 1024 * 1024,
					MaxTokens:         cfg.MaxTokens,
					ContextFallbacks:  cfg.ContextFallbacks,
					MinImageDimension: cfg.MinImageDimension,
					Timeout:           time.Duration(cfg.TimeoutSeconds) * time.Second,
				},
			})
			if err != nil && summary.OutputDir == "" {
				return err
			}
			if err == nil && flags.Refine {
				refineInput := summary.MergedMarkdown
				if refineInput == "" {
					return err
				}
				refinedOutput := ""
				if flags.RefinedName != "" {
					refinedOutput = filepath.Join(summary.OutputDir, flags.RefinedName)
				}
				refined, refineErr := refine.Run(ctx, refineInput, refine.Options{
					OutputPath:    refinedOutput,
					Profile:       flags.RefineProfile,
					Language:      flags.RefineLanguage,
					MaxInputChars: flags.RefineMaxChars,
					VLMConfig: vlm.Config{
						Provider:  cfg.Provider,
						BaseURL:   cfg.BaseURL,
						Model:     cfg.Model,
						APIKey:    cfg.APIKey,
						MaxTokens: cfg.MaxTokens,
						Timeout:   time.Duration(cfg.TimeoutSeconds) * time.Second,
					},
				})
				if refineErr != nil {
					err = refineErr
				}
				summary.RefinedMarkdown = refined.Output
			}
			if cfg.JSONOutput {
				if jsonErr := writeJSON(summary); jsonErr != nil {
					return jsonErr
				}
			} else {
				writeText("extracted %d/%d pages to %s", summary.PagesSuccess, summary.PagesTotal, summary.OutputDir)
				if summary.Resumed {
					writeText("resumed %d failed pages; retained %d existing pages", summary.PagesProcessed, summary.PagesRetained)
				}
				if summary.GroundingWarnings > 0 {
					writeText("grounding warnings: %d; inspect page metadata before trusting exact values", summary.GroundingWarnings)
				}
				if summary.RefinedMarkdown != "" {
					writeText("refined markdown: %s", summary.RefinedMarkdown)
				}
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "output directory")
	cmd.Flags().StringVar(&flags.Provider, "provider", "", "VLM provider")
	cmd.Flags().StringVar(&flags.BaseURL, "base-url", "", "OpenAI-compatible base URL")
	cmd.Flags().StringVar(&flags.Model, "model", "", "VLM model name")
	cmd.Flags().StringVar(&flags.APIKeyEnv, "api-key-env", "", "environment variable that stores the API key")
	cmd.Flags().StringVar(&flags.CredentialStore, "credential-store", "", "credential storage: auto, keychain, file, or env")
	cmd.Flags().StringVar(&flags.CredentialAccount, "credential-account", "", "credential account/profile name")
	cmd.Flags().StringVar(&flags.CredentialsPath, "credentials-path", "", "credential file path when credential-store=file")
	cmd.Flags().IntVar(&flags.Workers, "workers", 0, "concurrent page workers")
	cmd.Flags().IntVar(&flags.Retry, "retry", 3, "max retries per page")
	cmd.Flags().BoolVar(&flags.TextGrounding, "text-grounding", false, "use embedded PDF text as secondary visual evidence")
	cmd.Flags().IntVar(&flags.Timeout, "timeout", 120, "request timeout in seconds")
	cmd.Flags().IntVar(&flags.DPI, "dpi", 220, "render DPI")
	cmd.Flags().StringVar(&flags.Pages, "pages", "", "1-based pages to process, for example 1,3-5")
	cmd.Flags().StringVar(&flags.PPTRenderer, "ppt-renderer", "", "presentation renderer: auto, libreoffice, or keynote")
	cmd.Flags().StringVar(&flags.ImageDetail, "image-detail", "", "vision image detail: low, high, or auto")
	cmd.Flags().IntVar(&flags.MaxImageMB, "max-image-mb", 0, "compress request images above this size before sending to the VLM")
	cmd.Flags().IntVar(&flags.MaxTokens, "max-tokens", 0, "maximum model output tokens per page")
	cmd.Flags().IntVar(&flags.ContextFallbacks, "context-fallbacks", 2, "lower request image resolution this many times when the model context truncates output")
	cmd.Flags().IntVar(&flags.MinImageDimension, "min-image-dimension", 720, "minimum longest image side used by context fallback")
	cmd.Flags().StringVar(&flags.Prompt, "prompt", "", "prompt name: auto, document, or slide")
	cmd.Flags().StringVar(&flags.PromptFile, "prompt-file", "", "custom page-conversion prompt file")
	cmd.Flags().StringVar(&flags.MergedName, "merged-name", "", "merged Markdown filename")
	cmd.Flags().BoolVar(&flags.Refine, "refine", false, "refine merged Markdown into a high-signal document")
	cmd.Flags().StringVar(&flags.RefinedName, "refined-name", "", "refined Markdown filename")
	cmd.Flags().StringVar(&flags.RefineProfile, "refine-profile", "report", "refine profile: report, concise, or knowledge")
	cmd.Flags().StringVar(&flags.RefineLanguage, "refine-language", "auto", "refined output language: auto or a specific language name")
	cmd.Flags().IntVar(&flags.RefineMaxChars, "refine-max-input-chars", 120000, "maximum merged Markdown characters sent to the refine model")
	cmd.Flags().BoolVar(&flags.Merge, "merge", true, "write merged Markdown")
	cmd.Flags().BoolVar(&flags.Resume, "resume", false, "retry failed pages from the existing output and preserve successful pages")
	cmd.Flags().BoolVar(&flags.KeepImages, "keep-images", true, "keep rendered page images")
	return cmd
}
