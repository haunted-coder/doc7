package doc7

import (
	"github.com/magicrew/doc7/internal/batch"
	"github.com/magicrew/doc7/internal/extract"
)

func adaptExtractProgress(progress ProgressFunc) extract.ProgressFunc {
	if progress == nil {
		return nil
	}
	return func(event extract.ProgressEvent) {
		progress(ProgressEvent{
			Stage:            ProgressStage(event.Stage),
			Input:            event.Input,
			OutputDir:        event.OutputDir,
			Page:             event.Page,
			PagesTotal:       event.PagesTotal,
			SourcePagesTotal: event.SourcePagesTotal,
			PagesCompleted:   event.PagesCompleted,
			Status:           string(event.Status),
			Cached:           event.Cached,
			Duration:         event.Duration,
			Message:          event.Message,
			Error:            pageError(event.Error),
		})
	}
}

func adaptBatchProgress(progress ProgressFunc) batch.ProgressFunc {
	if progress == nil {
		return nil
	}
	return func(event batch.ProgressEvent) {
		progress(ProgressEvent{
			Stage:            ProgressStage(event.Stage),
			Input:            event.Input,
			OutputDir:        event.OutputDir,
			Status:           event.Status,
			Duration:         event.Duration,
			Message:          event.Message,
			Index:            event.Index,
			Total:            event.Total,
			PagesTotal:       event.PagesTotal,
			SourcePagesTotal: event.SourcePagesTotal,
			PagesSuccess:     event.PagesSuccess,
			PagesFailed:      event.PagesFailed,
			PagesCached:      event.PagesCached,
		})
	}
}

func pageError(value *extract.PageError) *PageError {
	if value == nil {
		return nil
	}
	return &PageError{Kind: value.Kind, Message: value.Message}
}
