package server

import (
	"time"

	"github.com/magicrew/doc7/internal/read"
)

type Config struct {
	DataDir        string
	JobWorkers     int
	QueueSize      int
	MaxUploadBytes int64
	Retention      time.Duration
	AuthToken      string
	ServiceVersion string
	ReadOptions    read.Options
}

type JobStatus string

const (
	StatusQueued    JobStatus = "queued"
	StatusRunning   JobStatus = "running"
	StatusSucceeded JobStatus = "succeeded"
	StatusFailed    JobStatus = "failed"
)

type JobProgress struct {
	PagesTotal       int `json:"pages_total,omitempty"`
	SourcePagesTotal int `json:"source_pages_total,omitempty"`
	PagesProcessed   int `json:"pages_processed,omitempty"`
	PagesRetained    int `json:"pages_retained,omitempty"`
	PagesCompleted   int `json:"pages_completed,omitempty"`
	PagesSuccess     int `json:"pages_success,omitempty"`
	PagesFailed      int `json:"pages_failed,omitempty"`
	PagesCached      int `json:"pages_cached,omitempty"`
	FilesTotal       int `json:"files_total,omitempty"`
	FilesCompleted   int `json:"files_completed,omitempty"`
	FilesFailed      int `json:"files_failed,omitempty"`
}

type JobResult struct {
	Mode              string `json:"mode"`
	PagesTotal        int    `json:"pages_total,omitempty"`
	SourcePagesTotal  int    `json:"source_pages_total,omitempty"`
	PagesProcessed    int    `json:"pages_processed,omitempty"`
	PagesRetained     int    `json:"pages_retained,omitempty"`
	PagesSuccess      int    `json:"pages_success,omitempty"`
	PagesFailed       int    `json:"pages_failed,omitempty"`
	PagesCached       int    `json:"pages_cached,omitempty"`
	GroundingWarnings int    `json:"grounding_warnings,omitempty"`
	Resumed           bool   `json:"resumed,omitempty"`
	FilesTotal        int    `json:"files_total,omitempty"`
	FilesDone         int    `json:"files_done,omitempty"`
	FilesFailed       int    `json:"files_failed,omitempty"`
}

type JobError struct {
	Kind      string `json:"kind"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type Job struct {
	ID         string      `json:"id"`
	Status     JobStatus   `json:"status"`
	InputName  string      `json:"input_name"`
	PromptName string      `json:"prompt_name"`
	Pages      string      `json:"pages,omitempty"`
	Resume     bool        `json:"resume,omitempty"`
	Merge      bool        `json:"merge"`
	CreatedAt  time.Time   `json:"created_at"`
	StartedAt  *time.Time  `json:"started_at,omitempty"`
	FinishedAt *time.Time  `json:"finished_at,omitempty"`
	Progress   JobProgress `json:"progress"`
	Result     *JobResult  `json:"result,omitempty"`
	Error      *JobError   `json:"error,omitempty"`
}

type JobLinks struct {
	Status    string `json:"status"`
	Markdown  string `json:"markdown,omitempty"`
	Artifacts string `json:"artifacts,omitempty"`
	Resume    string `json:"resume,omitempty"`
}

type ResumeRequest struct {
	Pages string `json:"pages,omitempty"`
}

type JobResponse struct {
	Job
	Links JobLinks `json:"links"`
}

type HealthResponse struct {
	OK      bool   `json:"ok"`
	Service string `json:"service"`
	Version string `json:"version,omitempty"`
	Queued  int    `json:"queued"`
	Running int    `json:"running"`
}

type ListResponse struct {
	Jobs []JobResponse `json:"jobs"`
}
