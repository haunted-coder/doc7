package extract

import "time"

type ProgressStage string

const (
	ProgressRenderStarted   ProgressStage = "render_started"
	ProgressRenderCompleted ProgressStage = "render_completed"
	ProgressRenderFailed    ProgressStage = "render_failed"
	ProgressPageCompleted   ProgressStage = "page_completed"
)

type ProgressEvent struct {
	Stage            ProgressStage
	Input            string
	OutputDir        string
	Page             int
	PagesTotal       int
	SourcePagesTotal int
	PagesCompleted   int
	Status           PageStatus
	Cached           bool
	Duration         time.Duration
	Message          string
	Error            *PageError
}

type ProgressFunc func(ProgressEvent)

func emitProgress(progress ProgressFunc, event ProgressEvent) {
	if progress != nil {
		progress(event)
	}
}
