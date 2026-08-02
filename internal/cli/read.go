package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/magicrew/doc7/internal/archiveinput"
	"github.com/magicrew/doc7/internal/config"
	readinput "github.com/magicrew/doc7/internal/read"
	"github.com/magicrew/doc7/internal/remoteinput"
	"github.com/magicrew/doc7/internal/vlm"
	"github.com/spf13/cobra"
)

type readFlags struct {
	Output            string
	Provider          string
	BaseURL           string
	Model             string
	APIKeyEnv         string
	CredentialStore   string
	CredentialAccount string
	CredentialsPath   string
	FileWorkers       int
	Workers           int
	Retry             int
	TextGrounding     bool
	Timeout           int
	DPI               int
	Pages             string
	OfficeRenderer    string
	ImageDetail       string
	MaxImageMB        int
	MaxTokens         int
	ContextFallbacks  int
	MinImageDimension int
	Prompt            string
	PromptFile        string
	StdinName         string
	StdinMaxMB        int64
	Stdout            bool
	Merge             bool
	Resume            bool
	KeepImages        bool
	ArchiveMaxFiles   int
	ArchiveMaxMB      int64
	DownloadMaxMB     int64
	DownloadTimeout   int
	result            *readinput.Result
}

func newReadCommand() *cobra.Command {
	flags := defaultReadFlags()
	cmd := &cobra.Command{
		Use:   "read <path|->",
		Short: translate("read.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeRead(cmd, args[0], &flags)
		},
	}
	flags.bind(cmd)
	return cmd
}

func defaultReadFlags() readFlags {
	archiveDefaults := archiveinput.DefaultOptions()
	return readFlags{
		Merge:           true,
		KeepImages:      true,
		ArchiveMaxFiles: archiveDefaults.MaxFiles,
		ArchiveMaxMB:    archiveDefaults.MaxBytes / bytesPerMiB,
		DownloadMaxMB:   remoteinput.DefaultMaxBytes() / bytesPerMiB,
		DownloadTimeout: int(remoteinput.DefaultTimeout().Seconds()),
		StdinMaxMB:      remoteinput.DefaultMaxBytes() / bytesPerMiB,
	}
}

func executeRead(cmd *cobra.Command, inputPath string, flags *readFlags) error {
	sourceURL := ""
	ctx := cmd.Context()
	localConfig := flags.config()
	applyChangedNumericFlags(cmd, &localConfig)
	cfg, err := loadConfig(localConfig, false)
	if err != nil {
		return err
	}
	if flags.Stdout && cfg.JSONOutput {
		return vlm.NewError(vlm.ConfigError, "--stdout cannot be combined with --json", false, nil)
	}
	if flags.Stdout && !flags.Merge {
		return vlm.NewError(vlm.ConfigError, "--stdout requires --merge", false, nil)
	}
	if flags.Prompt != "" && flags.PromptFile != "" {
		return vlm.NewError(vlm.ConfigError, "use either --prompt or --prompt-file, not both", false, nil)
	}
	if inputPath != "-" && flags.StdinName != "" {
		return vlm.NewError(vlm.ConfigError, "--stdin-name is only valid when input is -", false, nil)
	}
	if inputPath == "-" {
		stagedPath, stagedName, cleanup, stageErr := stageStdin(os.Stdin, flags.StdinName, flags.StdinMaxMB)
		if stageErr != nil {
			return vlm.NewError(vlm.ConfigError, "failed to read stdin", false, stageErr)
		}
		defer cleanup()
		inputPath = stagedPath
		flags.StdinName = stagedName
		sourceURL = "stdin://" + stagedName
	}
	cfg, err = ensureVisionConfig(ctx, cfg, inputPath)
	if err != nil {
		return err
	}
	if !cfg.JSONOutput && !flags.Stdout {
		writeText("%s", translate("input.processing", inputPath))
	}
	progress := newProgressReporter(cfg)
	result, runErr := readinput.Run(ctx, inputPath, readinput.Options{
		OutputDir:            flags.Output,
		SourceURL:            sourceURL,
		InputLabel:           flags.StdinName,
		PromptName:           cfg.PromptName,
		PromptFile:           flags.PromptFile,
		Merge:                flags.Merge,
		Resume:               flags.Resume,
		FileWorkers:          cfg.FileWorkers,
		Workers:              cfg.Workers,
		RetryCount:           cfg.RetryCount,
		TextGrounding:        flags.TextGrounding,
		DPI:                  cfg.DPI,
		Pages:                flags.Pages,
		KeepImages:           flags.KeepImages,
		PresentationRenderer: cfg.PPTRenderer,
		ArchiveMaxFiles:      flags.ArchiveMaxFiles,
		ArchiveMaxBytes:      flags.ArchiveMaxMB * bytesPerMiB,
		DownloadMaxBytes:     flags.DownloadMaxMB * bytesPerMiB,
		DownloadTimeout:      time.Duration(flags.DownloadTimeout) * time.Second,
		SingleDocument:       flags.Stdout,
		ExtractProgress:      progress.reportExtract,
		BatchProgress:        progress.reportBatch,
		VLMConfig:            readVLMConfig(cfg),
	})
	if flags.result != nil {
		*flags.result = result
	}
	if result.Document == nil && result.Batch == nil {
		return runErr
	}
	if cfg.JSONOutput {
		var writeErr error
		if result.Document != nil {
			writeErr = writeJSON(result.Document)
		} else {
			writeErr = writeJSON(result.Batch)
		}
		if writeErr != nil {
			return writeErr
		}
		return runErr
	}
	if result.Document != nil {
		if flags.Stdout {
			if stdoutErr := writeMarkdownFile(result.Document.MergedMarkdown); stdoutErr != nil {
				return stdoutErr
			}
		} else {
			writeText("%s", translate("output.complete"))
			if result.Document.MergedMarkdown != "" {
				writeText("%s", translate("output.markdown", result.Document.MergedMarkdown))
			}
			writeText("%s", translate("output.artifacts", result.Document.OutputDir))
			writeText("%s", translate("output.pages", result.Document.PagesSuccess, result.Document.PagesTotal))
		}
		if !flags.Stdout && result.Document.Resumed {
			writeText("resumed %d failed pages; retained %d existing pages", result.Document.PagesProcessed, result.Document.PagesRetained)
		}
		if !flags.Stdout && result.Document.GroundingWarnings > 0 {
			writeText("grounding warnings: %d; inspect page metadata before trusting exact values", result.Document.GroundingWarnings)
		}
	} else {
		writeText("%s", translate("output.batch", result.Batch.FilesDone, result.Batch.FilesTotal, result.Batch.OutputRoot))
		if result.Batch.GroundingWarnings > 0 {
			writeText("grounding warnings: %d; inspect page metadata before trusting exact values", result.Batch.GroundingWarnings)
		}
	}
	return runErr
}

func (flags readFlags) config() config.FlagConfig {
	return config.FlagConfig{
		Provider:          flags.Provider,
		BaseURL:           flags.BaseURL,
		Model:             flags.Model,
		APIKeyEnv:         flags.APIKeyEnv,
		CredentialStore:   flags.CredentialStore,
		CredentialAccount: flags.CredentialAccount,
		CredentialsPath:   flags.CredentialsPath,
		FileWorkers:       flags.FileWorkers,
		Workers:           flags.Workers,
		DPI:               flags.DPI,
		PPTRenderer:       flags.OfficeRenderer,
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
}

func (flags *readFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "output directory")
	cmd.Flags().StringVar(&flags.Provider, "provider", "", "VLM provider")
	cmd.Flags().StringVar(&flags.BaseURL, "base-url", "", "OpenAI-compatible base URL")
	cmd.Flags().StringVar(&flags.Model, "model", "", "VLM model name")
	cmd.Flags().StringVar(&flags.APIKeyEnv, "api-key-env", "", "environment variable that stores the API key")
	cmd.Flags().StringVar(&flags.CredentialStore, "credential-store", "", "credential storage: auto, keychain, file, or env")
	cmd.Flags().StringVar(&flags.CredentialAccount, "credential-account", "", "credential account/profile name")
	cmd.Flags().StringVar(&flags.CredentialsPath, "credentials-path", "", "credential file path when credential-store=file")
	cmd.Flags().IntVar(&flags.FileWorkers, "file-workers", 0, "concurrent document workers")
	cmd.Flags().IntVar(&flags.Workers, "workers", 0, "concurrent page workers")
	cmd.Flags().IntVar(&flags.Retry, "retry", 3, "max retries per page")
	cmd.Flags().BoolVar(&flags.TextGrounding, "text-grounding", false, "use embedded PDF text as secondary visual evidence")
	cmd.Flags().IntVar(&flags.Timeout, "timeout", 120, "request timeout in seconds")
	cmd.Flags().IntVar(&flags.DPI, "dpi", 220, "render DPI")
	cmd.Flags().StringVar(&flags.Pages, "pages", "", "1-based pages to process, for example 1,3-5")
	cmd.Flags().StringVar(&flags.OfficeRenderer, "office-renderer", "", "Office renderer: auto, libreoffice, or keynote")
	cmd.Flags().StringVar(&flags.ImageDetail, "image-detail", "", "vision image detail: low, high, or auto")
	cmd.Flags().IntVar(&flags.MaxImageMB, "max-image-mb", 0, "compress request images above this size before sending to the VLM")
	cmd.Flags().IntVar(&flags.MaxTokens, "max-tokens", 0, "maximum model output tokens per page")
	cmd.Flags().IntVar(&flags.ContextFallbacks, "context-fallbacks", 2, "lower request image resolution this many times when the model context truncates output")
	cmd.Flags().IntVar(&flags.MinImageDimension, "min-image-dimension", 720, "minimum longest image side used by context fallback")
	cmd.Flags().StringVar(&flags.Prompt, "prompt", "", "prompt name: auto, document, or slide")
	cmd.Flags().StringVar(&flags.PromptFile, "prompt-file", "", "custom page-conversion prompt file")
	cmd.Flags().StringVar(&flags.StdinName, "stdin-name", "", "filename with extension for input read from stdin")
	cmd.Flags().Int64Var(&flags.StdinMaxMB, "stdin-max-mb", flags.StdinMaxMB, "maximum stdin input size in MB")
	cmd.Flags().BoolVar(&flags.Stdout, "stdout", false, "write merged Markdown to stdout")
	cmd.Flags().BoolVar(&flags.Merge, "merge", true, "write merged Markdown")
	cmd.Flags().BoolVar(&flags.Resume, "resume", false, "retry failed pages from the existing output and preserve successful pages")
	cmd.Flags().BoolVar(&flags.KeepImages, "keep-images", true, "keep rendered page images")
	cmd.Flags().IntVar(&flags.ArchiveMaxFiles, "archive-max-files", flags.ArchiveMaxFiles, "maximum files expanded from an archive")
	cmd.Flags().Int64Var(&flags.ArchiveMaxMB, "archive-max-mb", flags.ArchiveMaxMB, "maximum archive expanded size in MB")
	cmd.Flags().Int64Var(&flags.DownloadMaxMB, "download-max-mb", flags.DownloadMaxMB, "maximum remote input size in MB")
	cmd.Flags().IntVar(&flags.DownloadTimeout, "download-timeout", flags.DownloadTimeout, "remote input timeout in seconds")
}

func writeMarkdownFile(path string) error {
	if path == "" {
		return vlm.NewError(vlm.PartialError, "merged Markdown was not generated", false, nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return vlm.NewError(vlm.ConfigError, "failed to read merged Markdown", false, err)
	}
	if _, err := os.Stdout.Write(data); err != nil {
		return err
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		_, err = fmt.Fprintln(os.Stdout)
	}
	return err
}

func readVLMConfig(cfg config.AppConfig) vlm.Config {
	return vlm.Config{
		Provider:          cfg.Provider,
		BaseURL:           cfg.BaseURL,
		Model:             cfg.Model,
		APIKey:            cfg.APIKey,
		ImageDetail:       cfg.ImageDetail,
		MaxImageBytes:     int64(cfg.MaxImageMB) * bytesPerMiB,
		MaxTokens:         cfg.MaxTokens,
		ContextFallbacks:  cfg.ContextFallbacks,
		MinImageDimension: cfg.MinImageDimension,
		Timeout:           time.Duration(cfg.TimeoutSeconds) * time.Second,
	}
}
