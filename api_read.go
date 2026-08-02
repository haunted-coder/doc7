package doc7

import (
	"context"
	"time"

	"github.com/magicrew/doc7/internal/archiveinput"
	"github.com/magicrew/doc7/internal/read"
	"github.com/magicrew/doc7/internal/remoteinput"
)

// ReadOptions controls the unified document, directory, URL, and ZIP reader.
// The embedded Options contains model, rendering, prompt, and page settings.
type ReadOptions struct {
	Options
	FileWorkers      int
	ArchiveMaxFiles  int
	ArchiveMaxBytes  int64
	DownloadMaxBytes int64
	DownloadTimeout  time.Duration
}

// ReadMode identifies the result shape returned by Read.
type ReadMode string

const (
	ReadDocument ReadMode = "document"
	ReadBatch    ReadMode = "batch"
)

// ReadResult contains either one document result or a recursive batch result.
// ZIP archives and directories may produce a batch result.
type ReadResult struct {
	OK        bool          `json:"ok"`
	Mode      ReadMode      `json:"mode,omitempty"`
	Input     string        `json:"input"`
	OutputDir string        `json:"output_dir"`
	Document  *Summary      `json:"document,omitempty"`
	Batch     *BatchSummary `json:"batch,omitempty"`
}

// DefaultReadOptions returns defaults for the unified Read API.
func DefaultReadOptions() ReadOptions {
	archiveDefaults := archiveinput.DefaultOptions()
	return ReadOptions{
		Options:          DefaultOptions(),
		FileWorkers:      1,
		ArchiveMaxFiles:  archiveDefaults.MaxFiles,
		ArchiveMaxBytes:  archiveDefaults.MaxBytes,
		DownloadMaxBytes: remoteinput.DefaultMaxBytes(),
		DownloadTimeout:  remoteinput.DefaultTimeout(),
	}
}

// Read converts a local document, directory, HTTP(S) URL, or ZIP archive.
// It uses the same orchestration path as the CLI read command.
func Read(ctx context.Context, input string, options ReadOptions) (ReadResult, error) {
	value, err := read.Run(ctx, input, read.Options{
		OutputDir:            options.OutputDir,
		SourceURL:            options.SourceURL,
		InputLabel:           options.InputLabel,
		PromptName:           options.PromptName,
		PromptFile:           options.PromptFile,
		Merge:                options.Merge,
		Resume:               options.Resume,
		FileWorkers:          options.FileWorkers,
		Workers:              options.Workers,
		RetryCount:           options.RetryCount,
		TextGrounding:        options.TextGrounding,
		DPI:                  options.DPI,
		Pages:                options.Pages,
		KeepImages:           options.KeepImages,
		PresentationRenderer: options.PresentationRenderer,
		ArchiveMaxFiles:      options.ArchiveMaxFiles,
		ArchiveMaxBytes:      options.ArchiveMaxBytes,
		DownloadMaxBytes:     options.DownloadMaxBytes,
		DownloadTimeout:      options.DownloadTimeout,
		ExtractProgress:      adaptExtractProgress(options.Progress),
		BatchProgress:        adaptBatchProgress(options.Progress),
		VLMConfig:            vlmConfig(options.Provider, options.BaseURL, options.Model, options.APIKey, options.ImageDetail, options.MaxImageBytes, options.MaxTokens, options.ContextFallbacks, options.MinImageDimension, options.Timeout),
	})
	result := ReadResult{
		Mode:      ReadMode(value.Mode),
		Input:     value.Input,
		OutputDir: value.OutputDir,
	}
	if value.Document != nil {
		summary := summaryFromInternal(*value.Document)
		result.Document = &summary
		result.OK = result.Document.OK
	}
	if value.Batch != nil {
		summary := batchSummaryFromInternal(*value.Batch)
		result.Batch = &summary
		result.OK = result.Batch.OK
	}
	return result, publicError(err)
}
