// Package doc7 converts documents into Markdown through native conversion or
// an OpenAI-compatible multimodal model.
package doc7

import "time"

// ProgressStage identifies a conversion lifecycle event.
type ProgressStage string

const (
	ProgressRenderStarted   ProgressStage = "render_started"
	ProgressRenderCompleted ProgressStage = "render_completed"
	ProgressRenderFailed    ProgressStage = "render_failed"
	ProgressPageCompleted   ProgressStage = "page_completed"
	ProgressFileStarted     ProgressStage = "file_started"
	ProgressFileCompleted   ProgressStage = "file_completed"
)

// PageError contains a stable, machine-readable conversion error.
type PageError struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// ProgressEvent reports rendering, page, or file completion. Page and file
// events may arrive concurrently when workers are enabled.
type ProgressEvent struct {
	Stage            ProgressStage
	Input            string
	OutputDir        string
	Page             int
	PagesTotal       int
	SourcePagesTotal int
	PagesCompleted   int
	Status           string
	Cached           bool
	Duration         time.Duration
	Message          string
	Error            *PageError
	Index            int
	Total            int
	PagesSuccess     int
	PagesFailed      int
	PagesCached      int
}

// ProgressFunc receives conversion progress events.
type ProgressFunc func(ProgressEvent)

// Options controls one document conversion.
type Options struct {
	OutputDir            string
	SourceURL            string
	InputLabel           string
	PromptName           string
	PromptFile           string
	MergedName           string
	Merge                bool
	Resume               bool
	Workers              int
	RetryCount           int
	TextGrounding        bool
	DPI                  int
	Pages                string
	KeepImages           bool
	PresentationRenderer string
	Provider             string
	BaseURL              string
	Model                string
	APIKey               string
	ImageDetail          string
	MaxImageBytes        int64
	MaxTokens            int
	ContextFallbacks     int
	MinImageDimension    int
	Timeout              time.Duration
	Progress             ProgressFunc
}

// Summary describes one completed document conversion.
type Summary struct {
	OK                bool   `json:"ok"`
	Command           string `json:"command"`
	Mode              string `json:"mode,omitempty"`
	Input             string `json:"input"`
	OutputDir         string `json:"output_dir"`
	ManifestPath      string `json:"manifest"`
	MergedMarkdown    string `json:"merged_markdown"`
	PagesTotal        int    `json:"pages_total"`
	SourcePagesTotal  int    `json:"source_pages_total,omitempty"`
	PagesProcessed    int    `json:"pages_processed,omitempty"`
	PagesRetained     int    `json:"pages_retained,omitempty"`
	PagesSuccess      int    `json:"pages_success"`
	PagesFailed       int    `json:"pages_failed"`
	PagesCached       int    `json:"pages_cached"`
	GroundingWarnings int    `json:"grounding_warnings"`
	Resumed           bool   `json:"resumed,omitempty"`
}

// BatchOptions controls recursive directory conversion.
type BatchOptions struct {
	OutputRoot           string
	PromptName           string
	PromptFile           string
	Merge                bool
	Resume               bool
	DryRun               bool
	FileWorkers          int
	Workers              int
	RetryCount           int
	TextGrounding        bool
	DPI                  int
	Pages                string
	KeepImages           bool
	PresentationRenderer string
	Provider             string
	BaseURL              string
	Model                string
	APIKey               string
	ImageDetail          string
	MaxImageBytes        int64
	MaxTokens            int
	ContextFallbacks     int
	MinImageDimension    int
	Timeout              time.Duration
	Progress             ProgressFunc
}

// BatchSummary describes a recursive directory conversion.
type BatchSummary struct {
	OK                bool        `json:"ok"`
	Command           string      `json:"command"`
	InputDir          string      `json:"input_dir"`
	OutputRoot        string      `json:"output_root"`
	Manifest          string      `json:"manifest"`
	DryRun            bool        `json:"dry_run"`
	FileWorkers       int         `json:"file_workers"`
	FilesTotal        int         `json:"files_total"`
	FilesDone         int         `json:"files_done"`
	FilesFailed       int         `json:"files_failed"`
	GroundingWarnings int         `json:"grounding_warnings"`
	StartedAt         time.Time   `json:"started_at"`
	FinishedAt        time.Time   `json:"finished_at"`
	Items             []BatchItem `json:"items"`
}

// BatchItem describes one document inside a batch.
type BatchItem struct {
	Index             int        `json:"index"`
	Input             string     `json:"input"`
	OutputDir         string     `json:"output_dir"`
	Mode              string     `json:"mode,omitempty"`
	OK                bool       `json:"ok"`
	Status            string     `json:"status"`
	ManifestPath      string     `json:"manifest"`
	MergedMarkdown    string     `json:"merged_markdown"`
	PagesTotal        int        `json:"pages_total"`
	SourcePagesTotal  int        `json:"source_pages_total,omitempty"`
	PagesProcessed    int        `json:"pages_processed,omitempty"`
	PagesRetained     int        `json:"pages_retained,omitempty"`
	PagesSuccess      int        `json:"pages_success"`
	PagesFailed       int        `json:"pages_failed"`
	PagesCached       int        `json:"pages_cached"`
	GroundingWarnings int        `json:"grounding_warnings"`
	Resumed           bool       `json:"resumed,omitempty"`
	Error             *PageError `json:"error,omitempty"`
}
