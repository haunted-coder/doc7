package batch

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/magicrew/doc7/internal/extract"
)

type fileJob struct {
	Index     int
	Total     int
	InputPath string
	OutputDir string
}

type fileResult struct {
	Index int
	Item  ItemSummary
	Err   error
}

func runFiles(ctx context.Context, files []string, outputNames []string, options Options) ([]ItemSummary, int, int, error) {
	if options.DryRun {
		items := make([]ItemSummary, 0, len(files))
		for index, path := range files {
			items = append(items, ItemSummary{
				Index:     index + 1,
				Input:     path,
				OutputDir: filepath.Join(options.OutputRoot, outputNames[index]),
				OK:        true,
				Status:    "planned",
			})
		}
		return items, len(items), 0, nil
	}

	workerCount := options.FileWorkers
	if workerCount <= 0 {
		workerCount = 1
	}
	if workerCount > len(files) {
		workerCount = len(files)
	}

	jobs := make(chan fileJob)
	results := make(chan fileResult)
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				results <- processFile(ctx, job, options)
			}
		}()
	}

	go func() {
		defer close(jobs)
		for index, path := range files {
			select {
			case jobs <- fileJob{Index: index + 1, Total: len(files), InputPath: path, OutputDir: filepath.Join(options.OutputRoot, outputNames[index])}:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	collected := make([]fileResult, len(files))
	received := make([]bool, len(files))
	for result := range results {
		if result.Index <= 0 || result.Index > len(files) {
			continue
		}
		collected[result.Index-1] = result
		received[result.Index-1] = true
	}

	items := make([]ItemSummary, 0, len(files))
	filesDone := 0
	filesFailed := 0
	var firstErr error
	for index, result := range collected {
		if !received[index] {
			continue
		}
		items = append(items, result.Item)
		if result.Item.OK {
			filesDone++
		} else {
			filesFailed++
		}
		if firstErr == nil && result.Err != nil {
			firstErr = result.Err
		}
	}
	if firstErr == nil && ctx.Err() != nil && len(items) < len(files) {
		firstErr = ctx.Err()
	}
	return items, filesDone, filesFailed, firstErr
}

func processFile(ctx context.Context, job fileJob, options Options) fileResult {
	started := time.Now()
	emitProgress(options.Progress, ProgressEvent{
		Stage:     ProgressFileStarted,
		Index:     job.Index,
		Total:     job.Total,
		Input:     job.InputPath,
		OutputDir: job.OutputDir,
	})
	if item, ok := completedItem(job.Index, job.InputPath, job.OutputDir, options); ok {
		emitFileCompleted(options.Progress, job, item, time.Since(started), nil)
		return fileResult{Index: job.Index, Item: item}
	}

	itemOptions := extract.Options{
		OutputDir:     job.OutputDir,
		PromptName:    options.PromptName,
		PromptFile:    options.PromptFile,
		Merge:         options.Merge,
		Resume:        options.Resume,
		Workers:       options.Workers,
		RetryCount:    options.RetryCount,
		TextGrounding: options.TextGrounding,
		Progress:      options.ExtractProgress,
		Render:        options.Render,
		VLMConfig:     options.VLMConfig,
	}
	itemOptions.Render.OutputDir = job.OutputDir
	extractSummary, err := extract.Run(ctx, job.InputPath, itemOptions)
	item := ItemSummary{
		Index:             job.Index,
		Input:             job.InputPath,
		OutputDir:         job.OutputDir,
		Mode:              extractSummary.Mode,
		OK:                extractSummary.OK && err == nil,
		Status:            "success",
		ManifestPath:      extractSummary.ManifestPath,
		MergedMarkdown:    extractSummary.MergedMarkdown,
		PagesTotal:        extractSummary.PagesTotal,
		SourcePagesTotal:  extractSummary.SourcePagesTotal,
		PagesProcessed:    extractSummary.PagesProcessed,
		PagesRetained:     extractSummary.PagesRetained,
		PagesSuccess:      extractSummary.PagesSuccess,
		PagesFailed:       extractSummary.PagesFailed,
		PagesCached:       extractSummary.PagesCached,
		GroundingWarnings: extractSummary.GroundingWarnings,
		Resumed:           extractSummary.Resumed,
	}
	if err != nil {
		item.Status = "failed"
		item.Error = extract.PageErrorFromError(err)
	}
	emitFileCompleted(options.Progress, job, item, time.Since(started), err)
	return fileResult{Index: job.Index, Item: item, Err: err}
}

func emitFileCompleted(progress ProgressFunc, job fileJob, item ItemSummary, duration time.Duration, err error) {
	message := ""
	if item.Error != nil {
		message = item.Error.Message
	} else if err != nil {
		message = err.Error()
	}
	emitProgress(progress, ProgressEvent{
		Stage:            ProgressFileCompleted,
		Index:            job.Index,
		Total:            job.Total,
		Input:            job.InputPath,
		OutputDir:        job.OutputDir,
		Status:           item.Status,
		PagesTotal:       item.PagesTotal,
		SourcePagesTotal: item.SourcePagesTotal,
		PagesSuccess:     item.PagesSuccess,
		PagesFailed:      item.PagesFailed,
		PagesCached:      item.PagesCached,
		Duration:         duration,
		Message:          message,
	})
}
