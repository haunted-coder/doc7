package extract

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/render"
	"github.com/magicrew/doc7/internal/vlm"
)

type Options struct {
	OutputDir     string
	SourceURL     string
	InputLabel    string
	PromptName    string
	PromptFile    string
	MergedName    string
	Merge         bool
	Resume        bool
	Workers       int
	RetryCount    int
	TextGrounding bool
	Progress      ProgressFunc
	Render        render.Options
	VLMConfig     vlm.Config
}

type Summary struct {
	OK                bool   `json:"ok"`
	Command           string `json:"command"`
	Mode              string `json:"mode,omitempty"`
	Input             string `json:"input"`
	OutputDir         string `json:"output_dir"`
	ManifestPath      string `json:"manifest"`
	MergedMarkdown    string `json:"merged_markdown"`
	RefinedMarkdown   string `json:"refined_markdown,omitempty"`
	PagesTotal        int    `json:"pages_total"`
	SourcePagesTotal  int    `json:"source_pages_total,omitempty"`
	PagesProcessed    int    `json:"pages_processed,omitempty"`
	PagesRetained     int    `json:"pages_retained,omitempty"`
	PagesSuccess      int    `json:"pages_success"`
	PagesFailed       int    `json:"pages_failed"`
	PagesCached       int    `json:"cached"`
	GroundingWarnings int    `json:"grounding_warnings"`
	Resumed           bool   `json:"resumed,omitempty"`
}

func Run(ctx context.Context, inputPath string, options Options) (Summary, error) {
	input, err := detect.Detect(inputPath)
	if err != nil {
		return Summary{}, vlm.NewError(vlm.ConfigError, "failed to detect input", false, err)
	}
	input.SourceURL = strings.TrimSpace(options.SourceURL)
	progressPath := inputLabel(input, options)
	if options.OutputDir == "" {
		options.OutputDir = input.Name + "_output"
	}
	options.Render.OutputDir = options.OutputDir
	options.Render.TextGrounding = options.TextGrounding
	if options.Workers <= 0 {
		options.Workers = 5
	}
	if options.PromptName == "" {
		options.PromptName = "auto"
	}
	pageSelection, err := render.NormalizePageSelection(options.Render.Pages)
	if err != nil {
		return Summary{}, vlm.NewError(vlm.ConfigError, "invalid page selection", false, err)
	}
	options.Render.Pages = pageSelection
	var resume *resumeState
	if options.Resume {
		if detect.IsNative(input.Kind) {
			return Summary{}, vlm.NewError(vlm.ConfigError, "resume only supports visual document inputs", false, nil)
		}
		resume, err = prepareResume(options.OutputDir, input, pageSelection)
		if err != nil {
			return Summary{}, err
		}
		if resume.PageSelection == "" {
			return completedResumeSummary(resume, input, options)
		}
		options.Render.Pages = resume.PageSelection
	}
	if detect.IsNative(input.Kind) {
		if pageSelection != "" && pageSelection != "1" {
			return Summary{}, vlm.NewError(vlm.ConfigError, "native inputs have only page 1", false, nil)
		}
		return runNative(ctx, input, options)
	}
	prompt, err := PromptForInput(options.PromptName, options.PromptFile, input.Kind)
	if err != nil {
		return Summary{}, vlm.NewError(vlm.ConfigError, "failed to load prompt", false, err)
	}
	if options.PromptFile != "" {
		options.PromptName = "custom"
	}
	client, err := vlm.NewOpenAICompatible(options.VLMConfig, nil)
	if err != nil {
		return Summary{}, err
	}
	if err := ensureOutputDirs(options.OutputDir); err != nil {
		return Summary{}, err
	}
	if !options.Render.KeepImages {
		defer func() {
			_ = os.RemoveAll(filepath.Join(options.OutputDir, "images"))
		}()
	}
	renderStarted := time.Now()
	emitProgress(options.Progress, ProgressEvent{
		Stage:     ProgressRenderStarted,
		Input:     progressPath,
		OutputDir: options.OutputDir,
	})
	renderResult, err := render.Render(ctx, input, options.Render)
	if err != nil {
		emitProgress(options.Progress, ProgressEvent{
			Stage:     ProgressRenderFailed,
			Input:     progressPath,
			OutputDir: options.OutputDir,
			Message:   err.Error(),
		})
		return Summary{}, err
	}
	if resume != nil && renderResult.SourcePageCount != resume.SourcePageCount {
		return Summary{}, vlm.NewError(vlm.ConfigError, "resume source page count does not match the existing manifest", false, nil)
	}
	emitProgress(options.Progress, ProgressEvent{
		Stage:            ProgressRenderCompleted,
		Input:            progressPath,
		OutputDir:        options.OutputDir,
		PagesTotal:       len(renderResult.Pages),
		SourcePagesTotal: renderResult.SourcePageCount,
		Duration:         time.Since(renderStarted),
	})
	started := time.Now()
	pageResults := make([]pageResult, len(renderResult.Pages))
	var completedPages atomic.Int64
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < options.Workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				result := processPage(ctx, client, progressPath, renderResult.Pages[index], prompt, options)
				pageResults[index] = result
				emitProgress(options.Progress, pageProgressEvent(progressPath, options.OutputDir, renderResult.SourcePageCount, len(renderResult.Pages), int(completedPages.Add(1)), result))
			}
		}()
	}
	dispatching := true
	for index := range renderResult.Pages {
		select {
		case jobs <- index:
		case <-ctx.Done():
			dispatching = false
		}
		if !dispatching {
			break
		}
	}
	close(jobs)
	wg.Wait()
	for index, result := range pageResults {
		if result.Page.Page == 0 {
			result = canceledPageResult(progressPath, renderResult.Pages[index], prompt, options, ctx.Err())
			pageResults[index] = result
			emitProgress(options.Progress, pageProgressEvent(progressPath, options.OutputDir, renderResult.SourcePageCount, len(renderResult.Pages), int(completedPages.Add(1)), result))
		}
	}

	manifest, summary, mergePaths := buildManifest(manifestInput(input, options), renderResult, pageResults, options, started, time.Now())
	if resume != nil {
		manifest, summary, mergePaths, err = mergeResumeResult(options.OutputDir, input, options, resume, manifest, summary)
		if err != nil {
			return summary, err
		}
	}
	manifestPath := filepath.Join(options.OutputDir, "manifest.json")
	if err := writeJSON(manifestPath, manifest); err != nil {
		return summary, err
	}
	summary.ManifestPath = manifestPath
	if options.Merge {
		mergedName, err := mergedMarkdownName(input.Name, options.MergedName)
		if err != nil {
			return summary, err
		}
		merged := filepath.Join(options.OutputDir, mergedName)
		if len(mergePaths) > 0 {
			if err := MergeMarkdown(merged, mergePaths); err != nil {
				return summary, err
			}
			summary.MergedMarkdown = merged
		}
	}
	summary.OK = summary.PagesFailed == 0
	if summary.PagesFailed > 0 {
		return summary, vlm.NewError(vlm.PartialError, "some pages failed", false, nil)
	}
	return summary, nil
}

func inputLabel(input detect.Input, options Options) string {
	if label := strings.TrimSpace(options.InputLabel); label != "" {
		return label
	}
	return input.Path
}

func manifestInput(input detect.Input, options Options) detect.Input {
	input.Path = inputLabel(input, options)
	return input
}

func pageProgressEvent(inputPath string, outputDir string, sourcePagesTotal int, pagesTotal int, pagesCompleted int, result pageResult) ProgressEvent {
	return ProgressEvent{
		Stage:            ProgressPageCompleted,
		Input:            inputPath,
		OutputDir:        outputDir,
		Page:             result.Page.Page,
		PagesTotal:       pagesTotal,
		SourcePagesTotal: sourcePagesTotal,
		PagesCompleted:   pagesCompleted,
		Status:           result.Meta.Status,
		Cached:           result.Meta.Cached,
		Duration:         time.Duration(result.Meta.DurationMS) * time.Millisecond,
		Error:            result.Meta.Error,
	}
}

func mergedMarkdownName(inputName string, requested string) (string, error) {
	name := strings.TrimSpace(requested)
	if name == "" {
		return inputName + ".md", nil
	}
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return "", vlm.NewError(vlm.ConfigError, "merged name must be a filename, not a path", false, nil)
	}
	if !strings.EqualFold(filepath.Ext(name), ".md") {
		name += ".md"
	}
	return name, nil
}

type pageResult struct {
	Page         render.PageImage
	MarkdownPath string
	MetaPath     string
	Meta         PageMeta
}

func processPage(ctx context.Context, client vlm.Client, inputPath string, page render.PageImage, prompt string, options Options) pageResult {
	started := time.Now()
	pageMarkdown := filepath.Join(options.OutputDir, "pages", pageStem(page.Page)+".md")
	pageMeta := filepath.Join(options.OutputDir, "meta", pageStem(page.Page)+".json")
	promptHash := PagePromptHash(prompt, page.EmbeddedTextSHA256)
	key := VisualCacheKey(page.SHA256, promptHash, options.VLMConfig.Provider, options.VLMConfig.BaseURL, options.VLMConfig.Model, imageDetail(options), options.Render.DPI, options.VLMConfig.MaxTokens, maxImageBytes(options), options.VLMConfig.ContextFallbacks, options.VLMConfig.MinImageDimension)

	meta := PageMeta{
		Version:               "1",
		Page:                  page.Page,
		InputPath:             inputPath,
		ImagePath:             persistedImagePath(options, page.ImagePath),
		ImageSHA256:           page.SHA256,
		EmbeddedTextSHA256:    page.EmbeddedTextSHA256,
		EmbeddedTextChars:     page.EmbeddedTextChars,
		EmbeddedTextTruncated: page.EmbeddedTextTruncated,
		EmbeddedTextSupported: page.EmbeddedTextSupported,
		EmbeddedTextChecked:   page.EmbeddedTextChecked,
		TextGrounding:         options.TextGrounding,
		CacheKey:              key,
		PromptName:            options.PromptName,
		PromptSHA256:          promptHash,
		Provider:              options.VLMConfig.Provider,
		BaseURL:               vlm.RedactedEndpoint(options.VLMConfig.BaseURL),
		Model:                 options.VLMConfig.Model,
		MaxTokens:             options.VLMConfig.MaxTokens,
	}
	if cached(pageMeta, pageMarkdown, key) {
		preserveCachedPageOutcome(pageMeta, &meta)
		meta.Status = StatusSuccess
		meta.Cached = true
		meta.DurationMS = time.Since(started).Milliseconds()
		if err := writeJSON(pageMeta, meta); err != nil {
			meta.Status = StatusError
			meta.Error = pageError(err)
		}
		return pageResult{Page: page, MarkdownPath: pageMarkdown, MetaPath: pageMeta, Meta: meta}
	}
	response, err := completeWithRetry(ctx, client, vlm.Request{Prompt: prompt, ImagePath: page.ImagePath, ImageMIME: mime.TypeByExtension(filepath.Ext(page.ImagePath)), ImageDetail: imageDetail(options)}, options.RetryCount)
	if err != nil {
		meta.DurationMS = time.Since(started).Milliseconds()
		meta.Status = StatusError
		if usage, ok := vlm.UsageFromError(err); ok {
			meta.Usage = usage
		}
		meta.Error = pageError(err)
		if writeErr := writeJSON(pageMeta, meta); writeErr != nil {
			meta.Error = pageError(writeErr)
		}
		return pageResult{Page: page, MarkdownPath: pageMarkdown, MetaPath: pageMeta, Meta: meta}
	}
	meta.Status = StatusSuccess
	meta.Usage = response.Usage
	meta.RequestImageMaxDimension = response.RequestImageMaxDimension
	meta.ContextFallbacksUsed = response.ContextFallbacksUsed
	if options.TextGrounding && page.EmbeddedTextSupported && !page.EmbeddedTextChecked {
		meta.GroundingSkipped = true
		meta.GroundingError = pageError(vlm.NewError(vlm.DependencyError, "embedded text grounding is unavailable because no PDF text extractor could be run", false, nil))
	} else if options.TextGrounding && page.EmbeddedTextChecked {
		grounded := groundMarkdown(ctx, client, page, response.Content, imageDetail(options), options.RetryCount)
		meta.GroundingChecked = grounded.Checked
		meta.GroundingCorrected = grounded.Corrected
		meta.GroundingSkipped = grounded.Skipped
		meta.GroundingUnresolved = grounded.Unresolved
		if grounded.Error != nil {
			meta.GroundingError = pageError(grounded.Error)
		}
		meta.Usage = addUsage(meta.Usage, grounded.Usage)
		if grounded.Content != "" {
			response.Content = grounded.Content
		}
	}
	meta.DurationMS = time.Since(started).Milliseconds()
	if err := writeVisualPageMarkdown(pageMarkdown, page, response.Content); err != nil {
		meta.Status = StatusError
		meta.Error = pageError(err)
		if writeErr := writeJSON(pageMeta, meta); writeErr != nil {
			meta.Error = pageError(writeErr)
		}
		return pageResult{Page: page, MarkdownPath: pageMarkdown, MetaPath: pageMeta, Meta: meta}
	}
	if err := writeJSON(pageMeta, meta); err != nil {
		meta.Status = StatusError
		meta.Error = pageError(err)
	}
	return pageResult{Page: page, MarkdownPath: pageMarkdown, MetaPath: pageMeta, Meta: meta}
}

func preserveCachedPageOutcome(path string, meta *PageMeta) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cachedMeta PageMeta
	if err := json.Unmarshal(data, &cachedMeta); err != nil || cachedMeta.CacheKey != meta.CacheKey {
		return
	}
	meta.Usage = cachedMeta.Usage
	meta.RequestImageMaxDimension = cachedMeta.RequestImageMaxDimension
	meta.ContextFallbacksUsed = cachedMeta.ContextFallbacksUsed
	meta.GroundingChecked = cachedMeta.GroundingChecked
	meta.GroundingCorrected = cachedMeta.GroundingCorrected
	meta.GroundingSkipped = cachedMeta.GroundingSkipped
	meta.GroundingUnresolved = cachedMeta.GroundingUnresolved
	meta.GroundingError = cachedMeta.GroundingError
}

func canceledPageResult(inputPath string, page render.PageImage, prompt string, options Options, cause error) pageResult {
	if cause == nil {
		cause = context.Canceled
	}
	markdownPath := filepath.Join(options.OutputDir, "pages", pageStem(page.Page)+".md")
	metaPath := filepath.Join(options.OutputDir, "meta", pageStem(page.Page)+".json")
	promptHash := PagePromptHash(prompt, page.EmbeddedTextSHA256)
	meta := PageMeta{
		Version:               "1",
		Page:                  page.Page,
		InputPath:             inputPath,
		ImagePath:             persistedImagePath(options, page.ImagePath),
		ImageSHA256:           page.SHA256,
		EmbeddedTextSHA256:    page.EmbeddedTextSHA256,
		EmbeddedTextChars:     page.EmbeddedTextChars,
		EmbeddedTextTruncated: page.EmbeddedTextTruncated,
		EmbeddedTextSupported: page.EmbeddedTextSupported,
		EmbeddedTextChecked:   page.EmbeddedTextChecked,
		TextGrounding:         options.TextGrounding,
		CacheKey:              VisualCacheKey(page.SHA256, promptHash, options.VLMConfig.Provider, options.VLMConfig.BaseURL, options.VLMConfig.Model, imageDetail(options), options.Render.DPI, options.VLMConfig.MaxTokens, maxImageBytes(options), options.VLMConfig.ContextFallbacks, options.VLMConfig.MinImageDimension),
		PromptName:            options.PromptName,
		PromptSHA256:          promptHash,
		Provider:              options.VLMConfig.Provider,
		BaseURL:               vlm.RedactedEndpoint(options.VLMConfig.BaseURL),
		Model:                 options.VLMConfig.Model,
		Status:                StatusError,
		Error:                 pageError(vlm.NewError(vlm.TimeoutError, "page processing canceled", false, cause)),
	}
	_ = writeJSON(metaPath, meta)
	return pageResult{Page: page, MarkdownPath: markdownPath, MetaPath: metaPath, Meta: meta}
}

func cached(metaPath string, markdownPath string, key string) bool {
	if !nonEmptyFile(markdownPath) {
		return false
	}
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}
	var meta PageMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return false
	}
	return meta.Status == StatusSuccess && meta.CacheKey == key
}

func completeWithRetry(ctx context.Context, client vlm.Client, request vlm.Request, retryCount int) (vlm.Response, error) {
	var last error
	for attempt := 0; attempt <= retryCount; attempt++ {
		if err := ctx.Err(); err != nil {
			return vlm.Response{}, vlm.NewError(vlm.TimeoutError, "request canceled or timed out", false, err)
		}
		response, err := client.Complete(ctx, request)
		if err == nil {
			return response, nil
		}
		last = err
		var appErr *vlm.AppError
		if errors.As(err, &appErr) && !appErr.Retryable {
			return vlm.Response{}, err
		}
		if attempt == retryCount {
			break
		}
		select {
		case <-ctx.Done():
			return vlm.Response{}, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return vlm.Response{}, last
}

func buildManifest(input detect.Input, renderResult render.Result, pageResults []pageResult, options Options, started time.Time, finished time.Time) (Manifest, Summary, []string) {
	manifest := Manifest{Version: "1", Command: "extract", Mode: "visual", Input: input}
	manifest.Render.DPI = options.Render.DPI
	manifest.Render.PageCount = len(renderResult.Pages)
	manifest.Render.SourcePageCount = renderResult.SourcePageCount
	manifest.Render.PageSelection = renderResult.PageSelection
	if options.Render.KeepImages {
		manifest.Render.ImageFormat = "png"
		manifest.Render.OutputDir = "images"
	} else {
		manifest.Render.ImageFormat = "none"
	}
	manifest.VLM.Provider = options.VLMConfig.Provider
	manifest.VLM.BaseURL = vlm.RedactedEndpoint(options.VLMConfig.BaseURL)
	manifest.VLM.Model = options.VLMConfig.Model
	manifest.VLM.PromptName = options.PromptName
	manifest.VLM.ImageDetail = imageDetail(options)
	manifest.VLM.MaxImageBytes = maxImageBytes(options)
	manifest.VLM.MaxTokens = options.VLMConfig.MaxTokens
	manifest.VLM.ContextFallbacks = options.VLMConfig.ContextFallbacks
	manifest.VLM.MinImageDimension = options.VLMConfig.MinImageDimension
	manifest.VLM.TextGrounding = options.TextGrounding
	if options.TextGrounding {
		manifest.VLM.EmbeddedTextVersion = render.EmbeddedTextVersion()
		manifest.VLM.GroundingVersion = groundingVersion
	}
	manifest.VLM.Temperature = 0
	manifest.VLM.OutputNormalization = VisualNormalizationVersion()
	manifest.VLM.TimeoutSeconds = int(options.VLMConfig.Timeout.Seconds())
	manifest.VLM.RetryCount = options.RetryCount
	manifest.VLM.Workers = options.Workers
	manifest.Summary.PagesTotal = len(pageResults)
	manifest.Summary.StartedAt = started
	manifest.Summary.FinishedAt = finished

	summaryInput := input.Path
	if input.SourceURL != "" {
		summaryInput = input.SourceURL
	}
	summary := Summary{Command: "extract", Mode: "visual", Input: summaryInput, OutputDir: options.OutputDir, PagesTotal: len(pageResults), SourcePagesTotal: renderResult.SourcePageCount}
	var mergePaths []string
	for _, result := range pageResults {
		manifest.Pages = append(manifest.Pages, renderManifestPage(options.OutputDir, result.Page, result.MarkdownPath, result.MetaPath, result.Meta, options.Render.KeepImages))
		if result.Meta.Status == StatusSuccess {
			manifest.Summary.PagesSuccess++
			summary.PagesSuccess++
			mergePaths = append(mergePaths, result.MarkdownPath)
		} else {
			manifest.Summary.PagesFailed++
			summary.PagesFailed++
		}
		if result.Meta.Cached {
			manifest.Summary.PagesCached++
			summary.PagesCached++
		}
		if hasGroundingWarning(result.Meta) {
			manifest.Summary.GroundingWarnings++
			summary.GroundingWarnings++
		}
	}
	return manifest, summary, mergePaths
}

func persistedImagePath(options Options, path string) string {
	if !options.Render.KeepImages {
		return ""
	}
	return render.RelativeArtifactPath(options.OutputDir, path)
}

func hasGroundingWarning(meta PageMeta) bool {
	return meta.EmbeddedTextTruncated || meta.GroundingSkipped || meta.GroundingError != nil || len(meta.GroundingUnresolved) > 0
}

func ensureOutputDirs(outputDir string) error {
	for _, dir := range []string{"images", "pages", "meta"} {
		if err := os.MkdirAll(filepath.Join(outputDir, dir), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func pageStem(page int) string {
	if page < 10 {
		return "page_00" + strconvItoa(page)
	}
	if page < 100 {
		return "page_0" + strconvItoa(page)
	}
	return "page_" + strconvItoa(page)
}

func imageDetail(options Options) string {
	if options.VLMConfig.ImageDetail != "" {
		return options.VLMConfig.ImageDetail
	}
	return "high"
}

func maxImageBytes(options Options) int64 {
	if options.VLMConfig.MaxImageBytes > 0 {
		return options.VLMConfig.MaxImageBytes
	}
	return 9 * 1024 * 1024
}
