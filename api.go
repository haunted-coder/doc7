package doc7

import (
	"context"
	"time"

	"github.com/magicrew/doc7/internal/batch"
	"github.com/magicrew/doc7/internal/extract"
	"github.com/magicrew/doc7/internal/render"
	"github.com/magicrew/doc7/internal/vlm"
)

// Convert converts one local document, image, or supported image/HTML directory
// into Markdown and page-level artifacts.
func Convert(ctx context.Context, inputPath string, options Options) (Summary, error) {
	value, err := extract.Run(ctx, inputPath, extract.Options{
		OutputDir:     options.OutputDir,
		SourceURL:     options.SourceURL,
		InputLabel:    options.InputLabel,
		PromptName:    options.PromptName,
		PromptFile:    options.PromptFile,
		MergedName:    options.MergedName,
		Merge:         options.Merge,
		Resume:        options.Resume,
		Workers:       options.Workers,
		RetryCount:    options.RetryCount,
		TextGrounding: options.TextGrounding,
		Progress:      adaptExtractProgress(options.Progress),
		Render: render.Options{
			OutputDir:            options.OutputDir,
			DPI:                  options.DPI,
			Pages:                options.Pages,
			KeepImages:           options.KeepImages,
			PresentationRenderer: options.PresentationRenderer,
		},
		VLMConfig: vlmConfig(options.Provider, options.BaseURL, options.Model, options.APIKey, options.ImageDetail, options.MaxImageBytes, options.MaxTokens, options.ContextFallbacks, options.MinImageDimension, options.Timeout),
	})
	return summaryFromInternal(value), publicError(err)
}

// ConvertBatch recursively converts every supported document in inputDir.
func ConvertBatch(ctx context.Context, inputDir string, options BatchOptions) (BatchSummary, error) {
	value, err := batch.Run(ctx, inputDir, batch.Options{
		OutputRoot:      options.OutputRoot,
		PromptName:      options.PromptName,
		PromptFile:      options.PromptFile,
		Merge:           options.Merge,
		Resume:          options.Resume,
		DryRun:          options.DryRun,
		FileWorkers:     options.FileWorkers,
		Workers:         options.Workers,
		RetryCount:      options.RetryCount,
		TextGrounding:   options.TextGrounding,
		Progress:        adaptBatchProgress(options.Progress),
		ExtractProgress: adaptExtractProgress(options.Progress),
		Render:          render.Options{DPI: options.DPI, Pages: options.Pages, KeepImages: options.KeepImages, PresentationRenderer: options.PresentationRenderer},
		VLMConfig:       vlmConfig(options.Provider, options.BaseURL, options.Model, options.APIKey, options.ImageDetail, options.MaxImageBytes, options.MaxTokens, options.ContextFallbacks, options.MinImageDimension, options.Timeout),
	})
	return batchSummaryFromInternal(value), publicError(err)
}

func summaryFromInternal(value extract.Summary) Summary {
	return Summary{
		OK:                value.OK,
		Command:           value.Command,
		Mode:              value.Mode,
		Input:             value.Input,
		OutputDir:         value.OutputDir,
		ManifestPath:      value.ManifestPath,
		MergedMarkdown:    value.MergedMarkdown,
		PagesTotal:        value.PagesTotal,
		SourcePagesTotal:  value.SourcePagesTotal,
		PagesProcessed:    value.PagesProcessed,
		PagesRetained:     value.PagesRetained,
		PagesSuccess:      value.PagesSuccess,
		PagesFailed:       value.PagesFailed,
		PagesCached:       value.PagesCached,
		GroundingWarnings: value.GroundingWarnings,
		Resumed:           value.Resumed,
	}
}

func batchSummaryFromInternal(value batch.Summary) BatchSummary {
	items := make([]BatchItem, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, BatchItem{
			Index:             item.Index,
			Input:             item.Input,
			OutputDir:         item.OutputDir,
			Mode:              item.Mode,
			OK:                item.OK,
			Status:            item.Status,
			ManifestPath:      item.ManifestPath,
			MergedMarkdown:    item.MergedMarkdown,
			PagesTotal:        item.PagesTotal,
			SourcePagesTotal:  item.SourcePagesTotal,
			PagesProcessed:    item.PagesProcessed,
			PagesRetained:     item.PagesRetained,
			PagesSuccess:      item.PagesSuccess,
			PagesFailed:       item.PagesFailed,
			PagesCached:       item.PagesCached,
			GroundingWarnings: item.GroundingWarnings,
			Resumed:           item.Resumed,
			Error:             pageError(item.Error),
		})
	}
	return BatchSummary{
		OK:                value.OK,
		Command:           value.Command,
		InputDir:          value.InputDir,
		OutputRoot:        value.OutputRoot,
		Manifest:          value.Manifest,
		DryRun:            value.DryRun,
		FileWorkers:       value.FileWorkers,
		FilesTotal:        value.FilesTotal,
		FilesDone:         value.FilesDone,
		FilesFailed:       value.FilesFailed,
		GroundingWarnings: value.GroundingWarnings,
		StartedAt:         value.StartedAt,
		FinishedAt:        value.FinishedAt,
		Items:             items,
	}
}

func vlmConfig(provider string, baseURL string, model string, apiKey string, imageDetail string, maxImageBytes int64, maxTokens int, contextFallbacks int, minImageDimension int, timeout time.Duration) vlm.Config {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	if provider == "" {
		provider = "openai-compatible"
	}
	return vlm.Config{
		Provider:          provider,
		BaseURL:           baseURL,
		Model:             model,
		APIKey:            apiKey,
		ImageDetail:       imageDetail,
		MaxImageBytes:     maxImageBytes,
		MaxTokens:         maxTokens,
		ContextFallbacks:  contextFallbacks,
		MinImageDimension: minImageDimension,
		Timeout:           timeout,
	}
}
