package batch

import "time"

type ProgressStage string

const (
	ProgressFileStarted   ProgressStage = "file_started"
	ProgressFileCompleted ProgressStage = "file_completed"
)

type ProgressEvent struct {
	Stage            ProgressStage
	Index            int
	Total            int
	Input            string
	OutputDir        string
	Status           string
	PagesTotal       int
	SourcePagesTotal int
	PagesSuccess     int
	PagesFailed      int
	PagesCached      int
	Duration         time.Duration
	Message          string
}

type ProgressFunc func(ProgressEvent)

func emitProgress(progress ProgressFunc, event ProgressEvent) {
	if progress != nil {
		progress(event)
	}
}
