package extract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/nativeinput"
	"github.com/magicrew/doc7/internal/render"
	"github.com/magicrew/doc7/internal/vlm"
)

const nativeMode = "native"

func runNative(ctx context.Context, input detect.Input, options Options) (Summary, error) {
	if err := ctx.Err(); err != nil {
		return Summary{}, vlm.NewError(vlm.TimeoutError, "native conversion canceled", false, err)
	}
	if options.PromptFile != "" {
		return Summary{}, vlm.NewError(vlm.ConfigError, "custom prompts are only supported for visual document inputs", false, nil)
	}
	if err := ensureOutputDirs(options.OutputDir); err != nil {
		return Summary{}, err
	}
	converted, err := nativeinput.Convert(input.Path, input.Kind)
	if err != nil {
		return Summary{}, vlm.NewError(vlm.ConfigError, "failed to convert native input", false, err)
	}
	started := time.Now()
	page := render.PageImage{Page: 1, SourcePath: input.Path, SHA256: input.SHA256}
	pageMarkdown := filepath.Join(options.OutputDir, "pages", pageStem(page.Page)+".md")
	pageMeta := filepath.Join(options.OutputDir, "meta", pageStem(page.Page)+".json")
	promptHash := sha256Text(nativeinput.Version())
	cacheKey := nativeCacheKey(input, converted.Format)
	progressPath := inputLabel(input, options)
	meta := nativePageMeta(input, progressPath, page, converted.Format, promptHash, cacheKey)
	if cached(pageMeta, pageMarkdown, cacheKey) {
		if data, readErr := os.ReadFile(pageMeta); readErr == nil {
			_ = json.Unmarshal(data, &meta)
		}
		meta.Cached = true
		meta.DurationMS = time.Since(started).Milliseconds()
	} else {
		if err := writePageMarkdown(pageMarkdown, page, converted.Markdown); err != nil {
			return Summary{}, err
		}
		meta.DurationMS = time.Since(started).Milliseconds()
	}
	if err := writeJSON(pageMeta, meta); err != nil {
		return Summary{}, err
	}
	emitProgress(options.Progress, ProgressEvent{
		Stage:            ProgressPageCompleted,
		Input:            progressPath,
		OutputDir:        options.OutputDir,
		Page:             1,
		PagesTotal:       1,
		SourcePagesTotal: 1,
		PagesCompleted:   1,
		Status:           meta.Status,
		Cached:           meta.Cached,
		Duration:         time.Duration(meta.DurationMS) * time.Millisecond,
		Error:            meta.Error,
	})
	finished := time.Now()
	manifest := nativeManifest(manifestInput(input, options), page, pageMarkdown, pageMeta, meta, options, started, finished)
	manifestPath := filepath.Join(options.OutputDir, "manifest.json")
	if err := writeJSON(manifestPath, manifest); err != nil {
		return Summary{}, err
	}
	summary := Summary{
		OK:               true,
		Command:          "extract",
		Mode:             nativeMode,
		Input:            summaryInput(input),
		OutputDir:        options.OutputDir,
		ManifestPath:     manifestPath,
		PagesTotal:       1,
		SourcePagesTotal: 1,
		PagesSuccess:     1,
		PagesCached:      metaCached(meta),
	}
	if options.Merge {
		mergedName, err := mergedMarkdownName(input.Name, options.MergedName)
		if err != nil {
			return summary, err
		}
		merged := filepath.Join(options.OutputDir, mergedName)
		if err := MergeMarkdown(merged, []string{pageMarkdown}); err != nil {
			return summary, err
		}
		summary.MergedMarkdown = merged
	}
	return summary, nil
}

func nativePageMeta(input detect.Input, inputPath string, page render.PageImage, format string, promptHash string, cacheKey string) PageMeta {
	return PageMeta{
		Version:      "1",
		Page:         page.Page,
		InputPath:    inputPath,
		ImageSHA256:  input.SHA256,
		CacheKey:     cacheKey,
		PromptName:   nativeMode,
		PromptSHA256: promptHash,
		Provider:     nativeMode,
		Model:        format,
		Status:       StatusSuccess,
	}
}

func nativeManifest(input detect.Input, page render.PageImage, pageMarkdown string, pageMetaPath string, meta PageMeta, options Options, started time.Time, finished time.Time) Manifest {
	manifest := Manifest{Version: "1", Command: "extract", Mode: nativeMode, Input: input}
	manifest.Render.PageCount = 1
	manifest.Render.SourcePageCount = 1
	manifest.Render.PageSelection = options.Render.Pages
	manifest.Render.ImageFormat = "none"
	manifest.VLM.Provider = nativeMode
	manifest.VLM.Model = meta.Model
	manifest.VLM.PromptName = nativeMode
	manifest.Summary.PagesTotal = 1
	manifest.Summary.PagesSuccess = 1
	manifest.Summary.PagesCached = metaCached(meta)
	manifest.Summary.StartedAt = started
	manifest.Summary.FinishedAt = finished
	manifest.Pages = []ManifestPage{renderManifestPage(options.OutputDir, page, pageMarkdown, pageMetaPath, meta, false)}
	return manifest
}

func nativeCacheKey(input detect.Input, format string) string {
	return cacheKey(input.SHA256, sha256Text(nativeinput.Version()), nativeMode, "", format, "", 0, 0, 0, 0, 0)
}

func NativeCacheKey(input detect.Input, format string) string {
	return nativeCacheKey(input, format)
}

func summaryInput(input detect.Input) string {
	if strings.TrimSpace(input.SourceURL) != "" {
		return input.SourceURL
	}
	return input.Path
}

func metaCached(meta PageMeta) int {
	if meta.Cached {
		return 1
	}
	return 0
}
