package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/magicrew/doc7/internal/batch"
	"github.com/magicrew/doc7/internal/extract"
	"github.com/magicrew/doc7/internal/read"
	"github.com/magicrew/doc7/internal/vlm"
)

const (
	defaultJobWorkers     = 1
	defaultQueueSize      = 64
	defaultMaxUploadBytes = int64(1024 * 1024 * 1024)
	defaultRetention      = 24 * time.Hour
	jobIDBytes            = 16
	bytesPerMiB           = int64(1024 * 1024)
)

var (
	ErrJobNotFound    = errors.New("job not found")
	ErrJobNotReady    = errors.New("job is not ready")
	ErrJobActive      = errors.New("job is already queued or running")
	ErrQueueFull      = errors.New("job queue is full")
	ErrResumeRejected = errors.New("resume rejected")
)

type jobRecord struct {
	Job
	InputPath    string `json:"-"`
	OutputDir    string `json:"-"`
	MarkdownPath string `json:"markdown_path,omitempty"`
	ArtifactPath string `json:"-"`
}

type Manager struct {
	root       string
	jobsDir    string
	config     Config
	baseRead   read.Options
	queue      chan string
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	cleanupWg  sync.WaitGroup
	mu         sync.RWMutex
	jobs       map[string]*jobRecord
	closedOnce sync.Once
}

func NewManager(config Config) (*Manager, error) {
	config = normalizeConfig(config)
	absoluteDataDir, err := filepath.Abs(config.DataDir)
	if err != nil {
		return nil, err
	}
	config.DataDir = absoluteDataDir
	if err := os.MkdirAll(config.DataDir, 0o700); err != nil {
		return nil, err
	}
	jobsDir := filepath.Join(config.DataDir, "jobs")
	if err := os.MkdirAll(jobsDir, 0o700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		root:     config.DataDir,
		jobsDir:  jobsDir,
		config:   config,
		baseRead: config.ReadOptions,
		queue:    make(chan string, config.QueueSize),
		ctx:      ctx,
		cancel:   cancel,
		jobs:     map[string]*jobRecord{},
	}
	queued, err := manager.loadJobs()
	if err != nil {
		cancel()
		return nil, err
	}
	for worker := 0; worker < config.JobWorkers; worker++ {
		manager.wg.Add(1)
		go manager.worker()
	}
	for _, id := range queued {
		manager.queue <- id
	}
	if config.Retention > 0 {
		manager.cleanupWg.Add(1)
		go manager.cleanupLoop()
	}
	return manager, nil
}

func normalizeConfig(config Config) Config {
	if strings.TrimSpace(config.DataDir) == "" {
		config.DataDir = DefaultDataDir()
	}
	if config.JobWorkers <= 0 {
		config.JobWorkers = defaultJobWorkers
	}
	if config.QueueSize <= 0 {
		config.QueueSize = defaultQueueSize
	}
	if config.MaxUploadBytes <= 0 {
		config.MaxUploadBytes = defaultMaxUploadBytes
	}
	if config.Retention <= 0 {
		config.Retention = defaultRetention
	}
	return config
}

func DefaultDataDir() string {
	if cache, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cache) != "" {
		return filepath.Join(cache, "doc7", "server")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".doc7", "server")
	}
	return filepath.Join(".doc7", "server")
}

func DefaultMaxUploadBytes() int64 {
	return defaultMaxUploadBytes
}

func DefaultQueueSize() int {
	return defaultQueueSize
}

func DefaultRetention() time.Duration {
	return defaultRetention
}

func BytesFromMiB(value int64) (int64, error) {
	if value <= 0 {
		return 0, errors.New("size must be greater than 0 MiB")
	}
	maxInt64 := int64(^uint64(0) >> 1)
	if value > maxInt64/bytesPerMiB {
		return 0, errors.New("size is too large")
	}
	return value * bytesPerMiB, nil
}

func (m *Manager) worker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case id := <-m.queue:
			m.run(id)
		}
	}
}

func (m *Manager) loadJobs() ([]string, error) {
	entries, err := os.ReadDir(m.jobsDir)
	if err != nil {
		return nil, err
	}
	queued := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		record, err := m.readRecord(id)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("load job %s: %w", id, err)
		}
		m.restorePaths(record)
		if record.Status == StatusRunning {
			record.Status = StatusQueued
			record.StartedAt = nil
			if err := m.persist(record); err != nil {
				return nil, err
			}
		}
		m.jobs[id] = record
		if record.Status == StatusQueued {
			queued = append(queued, id)
		}
	}
	sort.Strings(queued)
	return queued, nil
}

func (m *Manager) CreateJob(inputName string, promptName string, pages string, merge bool, temporaryPath string, bytes int64) (Job, error) {
	if bytes <= 0 {
		return Job{}, errors.New("uploaded file is empty")
	}
	if bytes > m.config.MaxUploadBytes {
		return Job{}, fmt.Errorf("uploaded file exceeds maximum size %d bytes", m.config.MaxUploadBytes)
	}
	if strings.TrimSpace(promptName) == "" {
		promptName = "auto"
	}
	for attempt := 0; attempt < 10; attempt++ {
		id, err := newJobID()
		if err != nil {
			return Job{}, err
		}
		jobDir := m.jobDir(id)
		if err := os.Mkdir(jobDir, 0o700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return Job{}, err
		}
		inputDir := filepath.Join(jobDir, "input")
		if err := os.Mkdir(inputDir, 0o700); err != nil {
			_ = os.RemoveAll(jobDir)
			return Job{}, err
		}
		destination := filepath.Join(inputDir, inputName)
		if err := os.Rename(temporaryPath, destination); err != nil {
			_ = os.RemoveAll(jobDir)
			return Job{}, err
		}
		now := time.Now().UTC()
		record := &jobRecord{
			Job: Job{
				ID:         id,
				Status:     StatusQueued,
				InputName:  inputName,
				PromptName: promptName,
				Pages:      pages,
				Merge:      merge,
				CreatedAt:  now,
			},
			InputPath: destination,
			OutputDir: filepath.Join(jobDir, "output"),
		}
		if err := m.persist(record); err != nil {
			_ = os.RemoveAll(jobDir)
			return Job{}, err
		}
		m.mu.Lock()
		m.jobs[id] = record
		m.mu.Unlock()
		if err := m.enqueue(id); err != nil {
			m.removeJob(id)
			return Job{}, err
		}
		return record.Job, nil
	}
	return Job{}, errors.New("failed to allocate a unique job id")
}

func (m *Manager) enqueue(id string) error {
	select {
	case m.queue <- id:
		return nil
	default:
		return ErrQueueFull
	}
}

func (m *Manager) TempDir() string {
	return m.root
}

func (m *Manager) Snapshot(id string) (Job, error) {
	m.mu.RLock()
	record, ok := m.jobs[id]
	if !ok {
		m.mu.RUnlock()
		return Job{}, ErrJobNotFound
	}
	value := cloneJob(record.Job)
	m.mu.RUnlock()
	return value, nil
}

func (m *Manager) ResumeJob(id string, pages string) (Job, error) {
	m.mu.RLock()
	record, ok := m.jobs[id]
	if !ok {
		m.mu.RUnlock()
		return Job{}, ErrJobNotFound
	}
	if record.Status == StatusQueued || record.Status == StatusRunning {
		m.mu.RUnlock()
		return Job{}, ErrJobActive
	}
	inputPath := record.InputPath
	outputDir := record.OutputDir
	m.mu.RUnlock()

	if err := extract.ValidateResume(inputPath, outputDir, pages); err != nil {
		var appErr *vlm.AppError
		if errors.As(err, &appErr) {
			return Job{}, fmt.Errorf("%w: %s", ErrResumeRejected, appErr.Message)
		}
		return Job{}, fmt.Errorf("%w: %v", ErrResumeRejected, err)
	}

	m.mu.Lock()
	record, ok = m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return Job{}, ErrJobNotFound
	}
	if record.Status == StatusQueued || record.Status == StatusRunning {
		m.mu.Unlock()
		return Job{}, ErrJobActive
	}
	previous := cloneRecord(record)
	record.Status = StatusQueued
	record.Pages = pages
	record.Resume = true
	record.StartedAt = nil
	record.FinishedAt = nil
	record.Progress = JobProgress{}
	record.Result = nil
	record.Error = nil
	record.MarkdownPath = ""
	record.ArtifactPath = ""
	if err := m.persist(record); err != nil {
		m.mu.Unlock()
		return Job{}, err
	}
	job := cloneJob(record.Job)
	m.mu.Unlock()

	if err := m.enqueue(id); err != nil {
		m.mu.Lock()
		m.jobs[id] = previous
		_ = m.persist(previous)
		m.mu.Unlock()
		return Job{}, err
	}
	return job, nil
}

func (m *Manager) List(limit int) []Job {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	m.mu.RLock()
	values := make([]Job, 0, len(m.jobs))
	for _, record := range m.jobs {
		values = append(values, cloneJob(record.Job))
	}
	m.mu.RUnlock()
	sort.Slice(values, func(i, j int) bool {
		return values[i].CreatedAt.After(values[j].CreatedAt)
	})
	if len(values) > limit {
		values = values[:limit]
	}
	return values
}

func (m *Manager) ReadMarkdown(id string) ([]byte, error) {
	m.mu.RLock()
	record, ok := m.jobs[id]
	if !ok {
		m.mu.RUnlock()
		return nil, ErrJobNotFound
	}
	path := record.MarkdownPath
	status := record.Status
	m.mu.RUnlock()
	if status != StatusSucceeded || path == "" {
		return nil, ErrJobNotReady
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrJobNotReady
	}
	return data, err
}

func (m *Manager) Artifact(id string) (*os.File, os.FileInfo, error) {
	m.mu.RLock()
	record, ok := m.jobs[id]
	if !ok {
		m.mu.RUnlock()
		return nil, nil, ErrJobNotFound
	}
	path := record.ArtifactPath
	status := record.Status
	m.mu.RUnlock()
	if status != StatusSucceeded || path == "" {
		return nil, nil, ErrJobNotReady
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, ErrJobNotReady
	}
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

func (m *Manager) Stats() (int, int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	queued, running := 0, 0
	for _, record := range m.jobs {
		switch record.Status {
		case StatusQueued:
			queued++
		case StatusRunning:
			running++
		}
	}
	return queued, running
}

func (m *Manager) Close() {
	m.closedOnce.Do(func() {
		m.cancel()
		m.wg.Wait()
		m.cleanupWg.Wait()
	})
}

func (m *Manager) run(id string) {
	record, ok := m.begin(id)
	if !ok {
		return
	}
	options := m.baseRead
	options.OutputDir = record.OutputDir
	options.InputLabel = record.InputName
	options.PromptName = record.PromptName
	options.Pages = record.Pages
	options.Resume = record.Resume
	options.Merge = record.Merge
	options.ExtractProgress = func(event extract.ProgressEvent) {
		m.updateExtractProgress(id, event)
	}
	options.BatchProgress = func(event batch.ProgressEvent) {
		m.updateBatchProgress(id, event)
	}
	result, err := read.Run(m.ctx, record.InputPath, options)
	if m.ctx.Err() != nil {
		m.requeueAfterShutdown(id)
		return
	}
	m.finish(id, result, err)
}

func (m *Manager) begin(id string) (*jobRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.jobs[id]
	if !ok || record.Status != StatusQueued {
		return nil, false
	}
	now := time.Now().UTC()
	record.Status = StatusRunning
	record.StartedAt = &now
	record.Error = nil
	_ = m.persist(record)
	return cloneRecord(record), true
}

func (m *Manager) finish(id string, result read.Result, runErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.jobs[id]
	if !ok {
		return
	}
	record.Result = resultSummary(result)
	record.Progress = progressSummary(result)
	if result.Document != nil && result.Document.MergedMarkdown != "" {
		record.MarkdownPath = result.Document.MergedMarkdown
	}
	now := time.Now().UTC()
	record.FinishedAt = &now
	if runErr != nil {
		record.Status = StatusFailed
		record.Error = jobError(runErr)
	} else {
		record.Status = StatusSucceeded
		artifactPath := filepath.Join(m.jobDir(id), "artifacts.zip")
		if err := createArtifacts(record.OutputDir, artifactPath); err != nil {
			record.Status = StatusFailed
			record.Error = &JobError{Kind: string(vlm.DependencyError), Message: "failed to package job artifacts: " + err.Error()}
		} else {
			record.ArtifactPath = artifactPath
		}
	}
	_ = m.persist(record)
}

func (m *Manager) requeueAfterShutdown(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if record, ok := m.jobs[id]; ok && record.Status == StatusRunning {
		record.Status = StatusQueued
		record.StartedAt = nil
		_ = m.persist(record)
	}
}

func (m *Manager) updateExtractProgress(id string, event extract.ProgressEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.jobs[id]
	if !ok {
		return
	}
	if event.PagesTotal > 0 {
		record.Progress.PagesTotal = event.PagesTotal
	}
	if event.SourcePagesTotal > 0 {
		record.Progress.SourcePagesTotal = event.SourcePagesTotal
	}
	if event.Stage == extract.ProgressPageCompleted {
		record.Progress.PagesCompleted = event.PagesCompleted
		if event.Status == extract.StatusSuccess {
			record.Progress.PagesSuccess++
		} else {
			record.Progress.PagesFailed++
		}
		if event.Cached {
			record.Progress.PagesCached++
		}
	}
	_ = m.persist(record)
}

func (m *Manager) updateBatchProgress(id string, event batch.ProgressEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.jobs[id]
	if !ok {
		return
	}
	if event.Total > 0 {
		record.Progress.FilesTotal = event.Total
	}
	if event.Stage == batch.ProgressFileCompleted {
		record.Progress.FilesCompleted++
		if event.Status != "success" && event.Status != "cached" {
			record.Progress.FilesFailed++
		}
	}
	_ = m.persist(record)
}

func (m *Manager) cleanupLoop() {
	defer m.cleanupWg.Done()
	interval := m.config.Retention / 2
	if interval < time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupExpired()
		}
	}
}

func (m *Manager) cleanupExpired() {
	threshold := time.Now().Add(-m.config.Retention)
	for _, job := range m.List(10000) {
		if job.Status != StatusSucceeded && job.Status != StatusFailed {
			continue
		}
		if job.CreatedAt.After(threshold) {
			continue
		}
		m.removeJob(job.ID)
	}
}

func (m *Manager) removeJob(id string) {
	m.mu.Lock()
	delete(m.jobs, id)
	m.mu.Unlock()
	_ = os.RemoveAll(m.jobDir(id))
}

func (m *Manager) jobDir(id string) string {
	return filepath.Join(m.jobsDir, id)
}

func (m *Manager) recordPath(id string) string {
	return filepath.Join(m.jobDir(id), "job.json")
}

func (m *Manager) readRecord(id string) (*jobRecord, error) {
	data, err := os.ReadFile(m.recordPath(id))
	if err != nil {
		return nil, err
	}
	var record jobRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (m *Manager) restorePaths(record *jobRecord) {
	jobDir := m.jobDir(record.ID)
	record.InputPath = filepath.Join(jobDir, "input", record.InputName)
	record.OutputDir = filepath.Join(jobDir, "output")
	if record.ArtifactPath == "" {
		candidate := filepath.Join(jobDir, "artifacts.zip")
		if _, err := os.Stat(candidate); err == nil {
			record.ArtifactPath = candidate
		}
	}
	if record.MarkdownPath != "" && !filepath.IsAbs(record.MarkdownPath) {
		record.MarkdownPath = filepath.Join(jobDir, record.MarkdownPath)
	}
}

func (m *Manager) persist(record *jobRecord) error {
	value := *record
	if value.MarkdownPath != "" {
		if relative, err := filepath.Rel(m.jobDir(record.ID), value.MarkdownPath); err == nil {
			value.MarkdownPath = relative
		}
	}
	value.InputPath = ""
	value.OutputDir = ""
	value.ArtifactPath = ""
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(m.recordPath(record.ID), data)
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, ".doc7-state-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func newJobID() (string, error) {
	data := make([]byte, jobIDBytes)
	if _, err := io.ReadFull(rand.Reader, data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func cloneRecord(record *jobRecord) *jobRecord {
	value := *record
	value.Job = cloneJob(record.Job)
	return &value
}

func cloneJob(value Job) Job {
	if value.StartedAt != nil {
		started := *value.StartedAt
		value.StartedAt = &started
	}
	if value.FinishedAt != nil {
		finished := *value.FinishedAt
		value.FinishedAt = &finished
	}
	if value.Result != nil {
		result := *value.Result
		value.Result = &result
	}
	if value.Error != nil {
		errorValue := *value.Error
		value.Error = &errorValue
	}
	return value
}

func resultSummary(value read.Result) *JobResult {
	if value.Document != nil {
		return &JobResult{
			Mode:              value.Mode,
			PagesTotal:        value.Document.PagesTotal,
			SourcePagesTotal:  value.Document.SourcePagesTotal,
			PagesProcessed:    value.Document.PagesProcessed,
			PagesRetained:     value.Document.PagesRetained,
			PagesSuccess:      value.Document.PagesSuccess,
			PagesFailed:       value.Document.PagesFailed,
			PagesCached:       value.Document.PagesCached,
			GroundingWarnings: value.Document.GroundingWarnings,
			Resumed:           value.Document.Resumed,
		}
	}
	if value.Batch != nil {
		return &JobResult{
			Mode:              value.Mode,
			FilesTotal:        value.Batch.FilesTotal,
			FilesDone:         value.Batch.FilesDone,
			FilesFailed:       value.Batch.FilesFailed,
			GroundingWarnings: value.Batch.GroundingWarnings,
		}
	}
	return nil
}

func progressSummary(value read.Result) JobProgress {
	if value.Document != nil {
		return JobProgress{
			PagesTotal:       value.Document.PagesTotal,
			SourcePagesTotal: value.Document.SourcePagesTotal,
			PagesProcessed:   value.Document.PagesProcessed,
			PagesRetained:    value.Document.PagesRetained,
			PagesCompleted:   value.Document.PagesSuccess + value.Document.PagesFailed,
			PagesSuccess:     value.Document.PagesSuccess,
			PagesFailed:      value.Document.PagesFailed,
			PagesCached:      value.Document.PagesCached,
		}
	}
	if value.Batch != nil {
		return JobProgress{
			FilesTotal:     value.Batch.FilesTotal,
			FilesCompleted: value.Batch.FilesDone,
			FilesFailed:    value.Batch.FilesFailed,
		}
	}
	return JobProgress{}
}

func jobError(err error) *JobError {
	if err == nil {
		return nil
	}
	var appErr *vlm.AppError
	if errors.As(err, &appErr) {
		return &JobError{Kind: string(appErr.Kind), Message: appErr.Message, Retryable: appErr.Retryable}
	}
	return &JobError{Kind: string(vlm.ServerError), Message: err.Error()}
}
