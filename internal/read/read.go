package read

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/magicrew/doc7/internal/archiveinput"
	"github.com/magicrew/doc7/internal/batch"
	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/extract"
	"github.com/magicrew/doc7/internal/remoteinput"
	"github.com/magicrew/doc7/internal/render"
	"github.com/magicrew/doc7/internal/vlm"
)

const (
	ModeDocument = "document"
	ModeBatch    = "batch"
)

type Options struct {
	OutputDir            string
	SourceURL            string
	InputLabel           string
	PromptName           string
	PromptFile           string
	Merge                bool
	Resume               bool
	FileWorkers          int
	Workers              int
	RetryCount           int
	TextGrounding        bool
	DPI                  int
	Pages                string
	KeepImages           bool
	PresentationRenderer string
	ArchiveMaxFiles      int
	ArchiveMaxBytes      int64
	DownloadMaxBytes     int64
	DownloadTimeout      time.Duration
	SingleDocument       bool
	ExtractProgress      extract.ProgressFunc
	BatchProgress        batch.ProgressFunc
	VLMConfig            vlm.Config
}

type Result struct {
	Mode      string
	Input     string
	OutputDir string
	Document  *extract.Summary
	Batch     *batch.Summary
}

func Run(ctx context.Context, inputPath string, options Options) (Result, error) {
	logicalInput := inputPath
	if label := strings.TrimSpace(options.InputLabel); label != "" {
		logicalInput = label
	}
	output := options.OutputDir
	if output == "" {
		var err error
		output, err = defaultOutput(logicalInput)
		if err != nil {
			return Result{}, err
		}
		output, err = resolveDefaultOutput(inputPath, output)
		if err != nil {
			return Result{}, err
		}
	}
	options.OutputDir = output

	if remoteinput.IsHTTPURL(inputPath) {
		return runRemote(ctx, inputPath, options)
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return Result{}, vlm.NewError(vlm.ConfigError, "failed to access input", false, err)
	}
	return runLocal(ctx, inputPath, info, options)
}

func runLocal(ctx context.Context, inputPath string, info os.FileInfo, options Options) (Result, error) {
	if !info.IsDir() && strings.EqualFold(filepath.Ext(inputPath), ".zip") {
		return runArchive(ctx, inputPath, options)
	}
	if info.IsDir() {
		single, err := isSingleDocumentDirectory(inputPath)
		if err != nil {
			return Result{}, err
		}
		if !single {
			if options.SingleDocument {
				return Result{}, vlm.NewError(vlm.ConfigError, "single-document output does not support a directory of documents", false, nil)
			}
			return runBatch(ctx, inputPath, options)
		}
	}
	return runDocument(ctx, inputPath, options)
}

func runDocument(ctx context.Context, inputPath string, options Options) (Result, error) {
	summary, err := extract.Run(ctx, inputPath, extract.Options{
		OutputDir:     options.OutputDir,
		SourceURL:     options.SourceURL,
		InputLabel:    options.InputLabel,
		PromptName:    options.PromptName,
		PromptFile:    options.PromptFile,
		Merge:         options.Merge,
		Resume:        options.Resume,
		Workers:       options.Workers,
		RetryCount:    options.RetryCount,
		TextGrounding: options.TextGrounding,
		Progress:      options.ExtractProgress,
		Render: render.Options{
			OutputDir:            options.OutputDir,
			DPI:                  options.DPI,
			Pages:                options.Pages,
			KeepImages:           options.KeepImages,
			PresentationRenderer: options.PresentationRenderer,
		},
		VLMConfig: options.VLMConfig,
	})
	if err != nil && summary.OutputDir == "" {
		return Result{}, err
	}
	return Result{
		Mode:      ModeDocument,
		Input:     summary.Input,
		OutputDir: summary.OutputDir,
		Document:  &summary,
	}, err
}

func runBatch(ctx context.Context, inputDir string, options Options) (Result, error) {
	summary, err := batch.Run(ctx, inputDir, batch.Options{
		OutputRoot:      options.OutputDir,
		PromptName:      options.PromptName,
		PromptFile:      options.PromptFile,
		Merge:           options.Merge,
		Resume:          options.Resume,
		FileWorkers:     options.FileWorkers,
		Workers:         options.Workers,
		RetryCount:      options.RetryCount,
		TextGrounding:   options.TextGrounding,
		Progress:        options.BatchProgress,
		ExtractProgress: options.ExtractProgress,
		Render: render.Options{
			DPI:                  options.DPI,
			Pages:                options.Pages,
			KeepImages:           options.KeepImages,
			PresentationRenderer: options.PresentationRenderer,
		},
		VLMConfig: options.VLMConfig,
	})
	if err != nil && summary.OutputRoot == "" {
		return Result{}, err
	}
	return Result{
		Mode:      ModeBatch,
		Input:     summary.InputDir,
		OutputDir: summary.OutputRoot,
		Batch:     &summary,
	}, err
}

func runRemote(ctx context.Context, sourceURL string, options Options) (Result, error) {
	sourceDir := filepath.Join(options.OutputDir, "source")
	if err := os.RemoveAll(sourceDir); err != nil {
		return Result{}, vlm.NewError(vlm.ConfigError, "failed to reset remote source directory", false, err)
	}
	result, err := remoteinput.Download(ctx, sourceURL, sourceDir, remoteinput.Options{
		MaxBytes: options.DownloadMaxBytes,
		Timeout:  options.DownloadTimeout,
	})
	if err != nil {
		return Result{}, vlm.NewError(vlm.ConfigError, "failed to download remote input", false, err)
	}
	info, err := os.Stat(result.LocalPath)
	if err != nil {
		return Result{}, vlm.NewError(vlm.ConfigError, "downloaded input is no longer available", false, err)
	}
	options.SourceURL = result.FinalURL
	return runLocal(ctx, result.LocalPath, info, options)
}

func runArchive(ctx context.Context, inputPath string, options Options) (Result, error) {
	sourcesDir := filepath.Join(options.OutputDir, "sources")
	if err := os.RemoveAll(sourcesDir); err != nil {
		return Result{}, vlm.NewError(vlm.ConfigError, "failed to reset ZIP source directory", false, err)
	}
	result, err := archiveinput.ExtractZIP(inputPath, sourcesDir, archiveinput.Options{
		MaxFiles: options.ArchiveMaxFiles,
		MaxBytes: options.ArchiveMaxBytes,
	})
	if err != nil {
		return Result{}, vlm.NewError(vlm.ConfigError, "failed to expand ZIP archive", false, err)
	}
	if result.Files == 0 {
		return Result{}, vlm.NewError(vlm.ConfigError, "ZIP archive does not contain files", false, nil)
	}
	documentsOutput := filepath.Join(options.OutputDir, "documents")
	single, err := isSingleDocumentDirectory(result.ContentRoot)
	if err != nil {
		return Result{}, err
	}
	if single {
		return runDocument(ctx, result.ContentRoot, withOutput(options, documentsOutput))
	}
	if options.SingleDocument {
		return Result{}, vlm.NewError(vlm.ConfigError, "single-document output requires a ZIP archive containing one image or HTML slide document", false, nil)
	}
	return runBatch(ctx, result.ContentRoot, withOutput(options, documentsOutput))
}

func withOutput(options Options, output string) Options {
	options.OutputDir = output
	return options
}

func defaultOutput(inputPath string) (string, error) {
	if remoteinput.IsHTTPURL(inputPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, remoteinput.SuggestedName(inputPath)+"-doc7"), nil
	}
	absolute, err := filepath.Abs(inputPath)
	if err != nil {
		return "", err
	}
	name := filepath.Base(absolute)
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	return filepath.Join(filepath.Dir(absolute), name+"-doc7"), nil
}

func resolveDefaultOutput(inputPath string, output string) (string, error) {
	if _, err := os.Stat(output); errors.Is(err, os.ErrNotExist) {
		return output, nil
	} else if err != nil {
		return "", vlm.NewError(vlm.ConfigError, "failed to inspect default output", false, err)
	}
	if outputMatchesInput(output, inputPath) {
		return output, nil
	}
	suffix := inputExtensionSuffix(inputPath)
	base := output + "-" + suffix
	for sequence := 1; ; sequence++ {
		candidate := base
		if sequence > 1 {
			candidate = fmt.Sprintf("%s-%d", base, sequence)
		}
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", vlm.NewError(vlm.ConfigError, "failed to inspect alternate output", false, err)
		}
		if outputMatchesInput(candidate, inputPath) {
			return candidate, nil
		}
	}
}

func outputMatchesInput(output string, inputPath string) bool {
	data, err := os.ReadFile(filepath.Join(output, "manifest.json"))
	if err != nil {
		return false
	}
	var manifest extract.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return false
	}
	if remoteinput.IsHTTPURL(inputPath) {
		return remoteinput.RedactedURL(manifest.Input.SourceURL) == remoteinput.RedactedURL(inputPath)
	}
	current, err := detect.Detect(inputPath)
	return err == nil && current.SHA256 != "" && current.SHA256 == manifest.Input.SHA256
}

func inputExtensionSuffix(inputPath string) string {
	value := inputPath
	if remoteinput.IsHTTPURL(inputPath) {
		if parsed, err := url.Parse(inputPath); err == nil {
			value = parsed.Path
		}
	}
	suffix := strings.TrimPrefix(strings.ToLower(filepath.Ext(value)), ".")
	if suffix == "" {
		return "document"
	}
	var builder strings.Builder
	for _, r := range suffix {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "document"
	}
	return builder.String()
}

func isSingleDocumentDirectory(path string) (bool, error) {
	for _, marker := range []string{"index.html", "magic.project.js"} {
		if _, err := os.Stat(filepath.Join(path, marker)); err == nil {
			return true, nil
		}
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	hasImage := false
	for _, entry := range entries {
		if entry.IsDir() {
			return false, nil
		}
		entryPath := filepath.Join(path, entry.Name())
		if detect.IsImage(entryPath) {
			hasImage = true
			continue
		}
		if detect.IsSupportedFile(entryPath) {
			return false, nil
		}
	}
	return hasImage, nil
}
