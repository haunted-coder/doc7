package batch

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/extract"
	"github.com/magicrew/doc7/internal/render"
	"github.com/magicrew/doc7/internal/vlm"
)

type Options struct {
	OutputRoot      string
	PromptName      string
	PromptFile      string
	Merge           bool
	Resume          bool
	DryRun          bool
	FileWorkers     int
	Workers         int
	RetryCount      int
	TextGrounding   bool
	Progress        ProgressFunc
	ExtractProgress extract.ProgressFunc
	Render          render.Options
	VLMConfig       vlm.Config
}

type Summary struct {
	OK                bool          `json:"ok"`
	Command           string        `json:"command"`
	InputDir          string        `json:"input_dir"`
	OutputRoot        string        `json:"output_root"`
	Manifest          string        `json:"manifest"`
	DryRun            bool          `json:"dry_run"`
	FileWorkers       int           `json:"file_workers"`
	FilesTotal        int           `json:"files_total"`
	FilesDone         int           `json:"files_done"`
	FilesFailed       int           `json:"files_failed"`
	GroundingWarnings int           `json:"grounding_warnings"`
	StartedAt         time.Time     `json:"started_at"`
	FinishedAt        time.Time     `json:"finished_at"`
	Items             []ItemSummary `json:"items"`
}

type ItemSummary struct {
	Index             int                `json:"index"`
	Input             string             `json:"input"`
	OutputDir         string             `json:"output_dir"`
	Mode              string             `json:"mode,omitempty"`
	OK                bool               `json:"ok"`
	Status            string             `json:"status"`
	ManifestPath      string             `json:"manifest"`
	MergedMarkdown    string             `json:"merged_markdown"`
	PagesTotal        int                `json:"pages_total"`
	SourcePagesTotal  int                `json:"source_pages_total,omitempty"`
	PagesProcessed    int                `json:"pages_processed,omitempty"`
	PagesRetained     int                `json:"pages_retained,omitempty"`
	PagesSuccess      int                `json:"pages_success"`
	PagesFailed       int                `json:"pages_failed"`
	PagesCached       int                `json:"pages_cached"`
	GroundingWarnings int                `json:"grounding_warnings"`
	Resumed           bool               `json:"resumed,omitempty"`
	Error             *extract.PageError `json:"error,omitempty"`
}

func Run(ctx context.Context, inputDir string, options Options) (Summary, error) {
	absoluteInput, err := filepath.Abs(inputDir)
	if err != nil {
		return Summary{}, err
	}
	if options.OutputRoot == "" {
		options.OutputRoot = filepath.Join(absoluteInput, "doc7_batch_output")
	}
	absoluteOutput, err := filepath.Abs(options.OutputRoot)
	if err != nil {
		return Summary{}, err
	}
	options.OutputRoot = absoluteOutput
	pageSelection, err := render.NormalizePageSelection(options.Render.Pages)
	if err != nil {
		return Summary{}, vlm.NewError(vlm.ConfigError, "invalid page selection", false, err)
	}
	options.Render.Pages = pageSelection
	files, err := supportedFiles(absoluteInput, absoluteOutput)
	if err != nil {
		return Summary{}, err
	}
	if len(files) == 0 {
		return Summary{}, vlm.NewError(vlm.ConfigError, "directory does not contain supported documents", false, nil)
	}
	if err := os.MkdirAll(options.OutputRoot, 0o755); err != nil {
		return Summary{}, err
	}
	if options.FileWorkers <= 0 {
		options.FileWorkers = 1
	}
	outputNames := relativeOutputNames(absoluteInput, files)

	started := time.Now()
	summary := Summary{
		Command:     "batch",
		InputDir:    absoluteInput,
		OutputRoot:  options.OutputRoot,
		DryRun:      options.DryRun,
		FileWorkers: options.FileWorkers,
		FilesTotal:  len(files),
		StartedAt:   started,
	}
	var firstErr error
	summary.Items, summary.FilesDone, summary.FilesFailed, firstErr = runFiles(ctx, files, outputNames, options)
	for _, item := range summary.Items {
		summary.GroundingWarnings += item.GroundingWarnings
	}
	summary.FinishedAt = time.Now()
	summary.OK = summary.FilesFailed == 0 && summary.FilesDone == summary.FilesTotal
	summary.Manifest = filepath.Join(options.OutputRoot, "batch_summary.json")
	if err := writeSummary(summary.Manifest, summary); err != nil && firstErr == nil {
		firstErr = err
	}
	return summary, firstErr
}

func supportedFiles(inputDir string, outputRoot string) ([]string, error) {
	files := []string{}
	outputRoot = filepath.Clean(outputRoot)
	err := filepath.WalkDir(inputDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if filepath.Clean(path) == outputRoot {
				return filepath.SkipDir
			}
			return nil
		}
		if detect.IsSupportedFile(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func completedItem(index int, inputPath string, outputDir string, options Options) (ItemSummary, bool) {
	manifestPath := filepath.Join(outputDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ItemSummary{}, false
	}
	var manifest extract.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ItemSummary{}, false
	}
	if manifest.Summary.PagesTotal == 0 || manifest.Summary.PagesFailed != 0 || manifest.Summary.PagesSuccess != manifest.Summary.PagesTotal {
		return ItemSummary{}, false
	}
	if !manifestArtifactsComplete(manifest, outputDir) {
		return ItemSummary{}, false
	}
	currentInput, err := detect.Detect(inputPath)
	if err != nil || currentInput.SHA256 == "" || currentInput.SHA256 != manifest.Input.SHA256 {
		return ItemSummary{}, false
	}
	mergedMarkdown := ""
	if options.Merge {
		mergedMarkdown = filepath.Join(outputDir, currentInput.Name+".md")
		if !nonEmptyRegularFile(mergedMarkdown) {
			return ItemSummary{}, false
		}
	}
	if options.Resume && resumeCanUseCachedItem(manifest, options) {
		return cachedItem(index, inputPath, outputDir, manifestPath, manifest, mergedMarkdown), true
	}
	if manifest.Mode == "native" {
		if !matchesNativeGeneration(currentInput, manifest, outputDir, options) {
			return ItemSummary{}, false
		}
	} else if !matchesGeneration(currentInput, manifest, outputDir, options) {
		return ItemSummary{}, false
	}
	return cachedItem(index, inputPath, outputDir, manifestPath, manifest, mergedMarkdown), true
}

func resumeCanUseCachedItem(manifest extract.Manifest, options Options) bool {
	if strings.TrimSpace(options.Render.Pages) != "" {
		return false
	}
	if manifest.Mode == "native" {
		return true
	}
	keepImages := manifest.Render.ImageFormat != "none"
	return keepImages == options.Render.KeepImages
}

func cachedItem(index int, inputPath string, outputDir string, manifestPath string, manifest extract.Manifest, mergedMarkdown string) ItemSummary {
	return ItemSummary{
		Index:             index,
		Input:             inputPath,
		OutputDir:         outputDir,
		Mode:              manifest.Mode,
		OK:                true,
		Status:            "cached",
		ManifestPath:      manifestPath,
		MergedMarkdown:    mergedMarkdown,
		PagesTotal:        manifest.Summary.PagesTotal,
		SourcePagesTotal:  manifest.Render.SourcePageCount,
		PagesProcessed:    manifest.Summary.PagesProcessed,
		PagesRetained:     manifest.Summary.PagesRetained,
		PagesSuccess:      manifest.Summary.PagesSuccess,
		PagesFailed:       manifest.Summary.PagesFailed,
		PagesCached:       manifest.Summary.PagesTotal,
		GroundingWarnings: manifest.Summary.GroundingWarnings,
		Resumed:           manifest.Resume != nil,
	}
}

func matchesNativeGeneration(input detect.Input, manifest extract.Manifest, outputDir string, options Options) bool {
	if !detect.IsNative(input.Kind) || options.PromptFile != "" {
		return false
	}
	if manifest.Render.PageSelection != options.Render.Pages {
		return false
	}
	if manifest.VLM.Provider != "native" || manifest.Render.ImageFormat != "none" || len(manifest.Pages) != 1 {
		return false
	}
	for _, page := range manifest.Pages {
		meta, ok := readPageMeta(page.MetaPath)
		if !ok || meta.Status != extract.StatusSuccess || meta.Model == "" {
			return false
		}
		if meta.CacheKey != extract.NativeCacheKey(input, meta.Model) {
			return false
		}
	}
	return true
}

func matchesGeneration(input detect.Input, manifest extract.Manifest, outputDir string, options Options) bool {
	if manifest.Render.DPI != options.Render.DPI {
		return false
	}
	expectedImageFormat := "none"
	if options.Render.KeepImages {
		expectedImageFormat = "png"
	}
	if manifest.Render.ImageFormat != expectedImageFormat {
		return false
	}
	if manifest.Render.PageSelection != options.Render.Pages {
		return false
	}
	if manifest.VLM.Provider != options.VLMConfig.Provider || manifest.VLM.Model != options.VLMConfig.Model {
		return false
	}
	if manifest.VLM.BaseURL != vlm.RedactedEndpoint(options.VLMConfig.BaseURL) {
		return false
	}
	if manifest.VLM.ImageDetail != effectiveImageDetail(options.VLMConfig.ImageDetail) {
		return false
	}
	if manifest.VLM.MaxImageBytes != effectiveMaxImageBytes(options.VLMConfig.MaxImageBytes) {
		return false
	}
	if manifest.VLM.MaxTokens != options.VLMConfig.MaxTokens {
		return false
	}
	if manifest.VLM.ContextFallbacks != options.VLMConfig.ContextFallbacks || manifest.VLM.MinImageDimension != options.VLMConfig.MinImageDimension {
		return false
	}
	if manifest.VLM.Temperature != 0 {
		return false
	}
	if manifest.VLM.OutputNormalization != extract.VisualNormalizationVersion() {
		return false
	}
	if manifest.VLM.TextGrounding != options.TextGrounding {
		return false
	}
	if options.TextGrounding && manifest.VLM.EmbeddedTextVersion != render.EmbeddedTextVersion() {
		return false
	}
	if options.TextGrounding && manifest.VLM.GroundingVersion != extract.GroundingVersion() {
		return false
	}
	promptName := options.PromptName
	if promptName == "" {
		promptName = "auto"
	}
	if strings.TrimSpace(options.PromptFile) != "" {
		promptName = "custom"
	}
	if manifest.VLM.PromptName != promptName || len(manifest.Pages) == 0 {
		return false
	}
	prompt, err := extract.PromptForInput(options.PromptName, options.PromptFile, input.Kind)
	if err != nil {
		return false
	}
	for _, page := range manifest.Pages {
		meta, ok := readPageMeta(page.MetaPath)
		if !ok || meta.Status != extract.StatusSuccess {
			return false
		}
		pagePromptSHA256 := extract.PagePromptHash(prompt, meta.EmbeddedTextSHA256)
		if meta.PromptSHA256 != pagePromptSHA256 {
			return false
		}
		expectedKey := extract.VisualCacheKey(
			meta.ImageSHA256,
			pagePromptSHA256,
			options.VLMConfig.Provider,
			options.VLMConfig.BaseURL,
			options.VLMConfig.Model,
			effectiveImageDetail(options.VLMConfig.ImageDetail),
			options.Render.DPI,
			options.VLMConfig.MaxTokens,
			effectiveMaxImageBytes(options.VLMConfig.MaxImageBytes),
			options.VLMConfig.ContextFallbacks,
			options.VLMConfig.MinImageDimension,
		)
		if meta.CacheKey != expectedKey || page.CacheKey != expectedKey {
			return false
		}
	}
	return true
}

func effectiveImageDetail(value string) string {
	if strings.TrimSpace(value) == "" {
		return "high"
	}
	return value
}

func effectiveMaxImageBytes(value int64) int64 {
	if value <= 0 {
		return 9 * 1024 * 1024
	}
	return value
}

func writeSummary(path string, summary Summary) error {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
