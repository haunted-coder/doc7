package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	doc7 "github.com/magicrew/doc7"
)

type options struct {
	BenchDir          string
	OutputDir         string
	BaseURL           string
	Model             string
	APIKeyEnv         string
	Provider          string
	Categories        string
	Parallel          int
	Repeats           int
	Limit             int
	Force             bool
	DryRun            bool
	TextGrounding     bool
	Prompt            string
	ImageDetail       string
	DPI               int
	MaxImageMB        int
	MaxTokens         int
	ContextFallbacks  int
	MinImageDimension int
	Retry             int
	Timeout           time.Duration
}

type document struct {
	Input    string
	Relative string
}

type job struct {
	Document document
	Repeat   int
	Output   string
}

type jobResult struct {
	Job      job
	Status   string
	Duration time.Duration
	Error    string
}

type runManifest struct {
	Benchmark         string    `json:"benchmark"`
	Candidate         string    `json:"candidate"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
	Model             string    `json:"model"`
	Provider          string    `json:"provider"`
	Prompt            string    `json:"prompt"`
	ImageDetail       string    `json:"image_detail"`
	DPI               int       `json:"dpi"`
	MaxImageMB        int       `json:"max_image_mb"`
	MaxTokens         int       `json:"max_tokens"`
	ContextFallbacks  int       `json:"context_fallbacks"`
	MinImageDimension int       `json:"min_image_dimension"`
	Retry             int       `json:"retry"`
	TimeoutSeconds    int       `json:"timeout_seconds"`
	TextGrounding     bool      `json:"text_grounding"`
	Parallel          int       `json:"parallel"`
	Repeats           int       `json:"repeats"`
	DocumentsSelected int       `json:"documents_selected"`
	JobsTotal         int       `json:"jobs_total"`
	Succeeded         int       `json:"succeeded"`
	Skipped           int       `json:"skipped"`
	Failed            int       `json:"failed"`
}

func main() {
	opts := parseOptions()
	if strings.TrimSpace(opts.BenchDir) == "" {
		fatal("--bench-dir is required")
	}
	if !opts.DryRun {
		if strings.TrimSpace(opts.BaseURL) == "" {
			fatal("--base-url is required unless --dry-run is used")
		}
		if strings.TrimSpace(opts.Model) == "" {
			fatal("--model is required unless --dry-run is used")
		}
	}
	if opts.Parallel <= 0 || opts.Repeats <= 0 || opts.Limit < 0 || opts.DPI <= 0 || opts.MaxTokens <= 0 || opts.ContextFallbacks < 0 || opts.MinImageDimension <= 0 || opts.Retry < 0 || opts.Timeout <= 0 {
		fatal("parallel, repeats, dpi, max-tokens, min-image-dimension, and timeout must be positive; limit, context-fallbacks, and retry must not be negative")
	}

	benchDir, err := filepath.Abs(opts.BenchDir)
	if err != nil {
		fatal("failed to resolve benchmark directory: %v", err)
	}
	if opts.OutputDir == "" {
		opts.OutputDir = filepath.Join(benchDir, "doc7")
	} else if !filepath.IsAbs(opts.OutputDir) {
		opts.OutputDir = filepath.Join(benchDir, opts.OutputDir)
	}
	opts.OutputDir, err = filepath.Abs(opts.OutputDir)
	if err != nil {
		fatal("failed to resolve output directory: %v", err)
	}

	documents, err := collectDocuments(filepath.Join(benchDir, "pdfs"), opts.Categories, opts.Limit)
	if err != nil {
		fatal("failed to collect benchmark PDFs: %v", err)
	}
	jobs := makeJobs(documents, opts.OutputDir, opts.Repeats)
	if len(jobs) == 0 {
		fatal("no benchmark PDFs matched the selected categories")
	}
	if opts.DryRun {
		for _, item := range jobs {
			fmt.Printf("%s -> %s\n", item.Document.Relative, item.Output)
		}
		return
	}

	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		fatal("failed to create output directory: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	started := time.Now()
	results := runJobs(ctx, opts, jobs)
	finished := time.Now()
	manifest := buildManifest(opts, started, finished, len(documents), results)
	if err := writeJSON(filepath.Join(opts.OutputDir, "run.json"), manifest); err != nil {
		fatal("failed to write run manifest: %v", err)
	}
	if err := writeErrors(filepath.Join(opts.OutputDir, "errors.jsonl"), results); err != nil {
		fatal("failed to write error log: %v", err)
	}

	for _, result := range results {
		if result.Status == "failed" {
			fmt.Printf("failed  %s: %s\n", result.Job.Document.Relative, result.Error)
		}
	}
	fmt.Printf("completed %d jobs: %d succeeded, %d skipped, %d failed\n", len(results), countStatus(results, "succeeded"), countStatus(results, "skipped"), countStatus(results, "failed"))
	if countStatus(results, "failed") > 0 {
		os.Exit(1)
	}
}

func parseOptions() options {
	defaultBaseURL := strings.TrimSpace(os.Getenv("DOC7_BASE_URL"))
	defaultModel := strings.TrimSpace(os.Getenv("DOC7_MODEL"))
	opts := options{}
	flag.StringVar(&opts.BenchDir, "bench-dir", "", "olmOCR-bench data directory containing pdfs and JSONL files")
	flag.StringVar(&opts.OutputDir, "output-dir", "", "candidate output directory; defaults to <bench-dir>/doc7")
	flag.StringVar(&opts.BaseURL, "base-url", defaultBaseURL, "OpenAI-compatible multimodal endpoint")
	flag.StringVar(&opts.Model, "model", defaultModel, "model ID exposed by the endpoint")
	flag.StringVar(&opts.APIKeyEnv, "api-key-env", "DOC7_API_KEY", "environment variable containing the model API key")
	flag.StringVar(&opts.Provider, "provider", "openai-compatible", "VLM provider")
	flag.StringVar(&opts.Categories, "categories", "", "comma-separated PDF categories; empty selects all")
	flag.IntVar(&opts.Parallel, "parallel", 1, "concurrent benchmark documents")
	flag.IntVar(&opts.Repeats, "repeats", 1, "number of output repeats per document")
	flag.IntVar(&opts.Limit, "limit", 0, "maximum documents after sorting; zero selects all")
	flag.BoolVar(&opts.Force, "force", false, "regenerate existing output files")
	flag.BoolVar(&opts.DryRun, "dry-run", false, "list selected jobs without calling the model")
	flag.BoolVar(&opts.TextGrounding, "text-grounding", false, "enable embedded PDF text as secondary evidence")
	flag.StringVar(&opts.Prompt, "prompt", "document", "doc7 prompt profile")
	flag.StringVar(&opts.ImageDetail, "image-detail", "high", "vision image detail: low, high, or auto")
	flag.IntVar(&opts.DPI, "dpi", 220, "render DPI")
	flag.IntVar(&opts.MaxImageMB, "max-image-mb", 9, "maximum request image size in MB")
	flag.IntVar(&opts.MaxTokens, "max-tokens", 8192, "maximum model output tokens per page")
	flag.IntVar(&opts.ContextFallbacks, "context-fallbacks", 2, "request-image fallbacks when the model context is exhausted")
	flag.IntVar(&opts.MinImageDimension, "min-image-dimension", 720, "minimum longest request-image side")
	flag.IntVar(&opts.Retry, "retry", 0, "retries per page")
	flag.DurationVar(&opts.Timeout, "timeout", 10*time.Minute, "per-document timeout")
	flag.Parse()
	return opts
}

func collectDocuments(pdfRoot string, rawCategories string, limit int) ([]document, error) {
	if _, err := os.Stat(pdfRoot); err != nil {
		return nil, err
	}
	categoryFilter := parseCategories(rawCategories)
	items := make([]document, 0)
	err := filepath.WalkDir(pdfRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".pdf") {
			return nil
		}
		relative, err := filepath.Rel(pdfRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		category := strings.SplitN(relative, "/", 2)[0]
		if len(categoryFilter) > 0 {
			if _, ok := categoryFilter[category]; !ok {
				return nil
			}
		}
		items = append(items, document{Input: path, Relative: relative})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(left int, right int) bool { return items[left].Relative < items[right].Relative })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func parseCategories(value string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result[item] = struct{}{}
		}
	}
	return result
}

func makeJobs(documents []document, outputDir string, repeats int) []job {
	jobs := make([]job, 0, len(documents)*repeats)
	for _, item := range documents {
		base := strings.TrimSuffix(filepath.Base(item.Relative), filepath.Ext(item.Relative))
		directory := filepath.Dir(filepath.FromSlash(item.Relative))
		for repeat := 1; repeat <= repeats; repeat++ {
			outputName := fmt.Sprintf("%s_pg1_repeat%d.md", base, repeat)
			jobs = append(jobs, job{
				Document: item,
				Repeat:   repeat,
				Output:   filepath.Join(outputDir, directory, outputName),
			})
		}
	}
	return jobs
}

func runJobs(ctx context.Context, opts options, jobs []job) []jobResult {
	results := make([]jobResult, 0, len(jobs))
	jobsCh := make(chan job)
	resultsCh := make(chan jobResult)
	var wg sync.WaitGroup
	for index := 0; index < opts.Parallel; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobsCh {
				result := processJob(ctx, opts, item)
				resultsCh <- result
			}
		}()
	}
	go func() {
	producer:
		for _, item := range jobs {
			select {
			case jobsCh <- item:
			case <-ctx.Done():
				break producer
			}
		}
		close(jobsCh)
		wg.Wait()
		close(resultsCh)
	}()
	for result := range resultsCh {
		results = append(results, result)
		if result.Status == "succeeded" || result.Status == "skipped" {
			fmt.Printf("%-9s %s\n", result.Status, result.Job.Document.Relative)
		}
	}
	sort.Slice(results, func(left int, right int) bool {
		if results[left].Job.Document.Relative == results[right].Job.Document.Relative {
			return results[left].Job.Repeat < results[right].Job.Repeat
		}
		return results[left].Job.Document.Relative < results[right].Job.Document.Relative
	})
	return results
}

func processJob(parent context.Context, opts options, item job) jobResult {
	started := time.Now()
	result := jobResult{Job: item}
	if !opts.Force && nonEmptyFile(item.Output) {
		result.Status = "skipped"
		result.Duration = time.Since(started)
		return result
	}
	if err := os.MkdirAll(filepath.Dir(item.Output), 0o755); err != nil {
		return failJob(result, started, err)
	}
	temporary, err := os.MkdirTemp("", "doc7-olmocr-bench-")
	if err != nil {
		return failJob(result, started, err)
	}
	defer os.RemoveAll(temporary)
	ctx, cancel := context.WithTimeout(parent, opts.Timeout)
	defer cancel()

	config := doc7.DefaultOptions()
	config.OutputDir = temporary
	config.PromptName = opts.Prompt
	config.Workers = 1
	config.RetryCount = opts.Retry
	config.TextGrounding = opts.TextGrounding
	config.DPI = opts.DPI
	config.KeepImages = false
	config.Provider = opts.Provider
	config.BaseURL = opts.BaseURL
	config.Model = opts.Model
	config.APIKey = os.Getenv(opts.APIKeyEnv)
	config.ImageDetail = opts.ImageDetail
	config.MaxImageBytes = int64(opts.MaxImageMB) * 1024 * 1024
	config.MaxTokens = opts.MaxTokens
	config.ContextFallbacks = opts.ContextFallbacks
	config.MinImageDimension = opts.MinImageDimension
	config.Timeout = opts.Timeout

	summary, err := doc7.Convert(ctx, item.Document.Input, config)
	if err != nil {
		return failJob(result, started, err)
	}
	if summary.PagesTotal != 1 || summary.PagesSuccess != 1 || summary.MergedMarkdown == "" {
		return failJob(result, started, fmt.Errorf("expected one successful page, got %d total and %d successful", summary.PagesTotal, summary.PagesSuccess))
	}
	data, err := os.ReadFile(summary.MergedMarkdown)
	if err != nil {
		return failJob(result, started, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return failJob(result, started, errors.New("doc7 returned empty Markdown"))
	}
	if err := os.WriteFile(item.Output, data, 0o644); err != nil {
		return failJob(result, started, err)
	}
	result.Status = "succeeded"
	result.Duration = time.Since(started)
	return result
}

func failJob(result jobResult, started time.Time, err error) jobResult {
	result.Status = "failed"
	result.Error = err.Error()
	result.Duration = time.Since(started)
	if result.Job.Output != "" {
		_ = os.MkdirAll(filepath.Dir(result.Job.Output), 0o755)
		_ = os.WriteFile(result.Job.Output, []byte{}, 0o644)
	}
	return result
}

func buildManifest(opts options, started time.Time, finished time.Time, documents int, results []jobResult) runManifest {
	return runManifest{
		Benchmark:         "olmOCR-Bench",
		Candidate:         filepath.Base(opts.OutputDir),
		StartedAt:         started,
		FinishedAt:        finished,
		Model:             opts.Model,
		Provider:          opts.Provider,
		Prompt:            opts.Prompt,
		ImageDetail:       opts.ImageDetail,
		DPI:               opts.DPI,
		MaxImageMB:        opts.MaxImageMB,
		MaxTokens:         opts.MaxTokens,
		ContextFallbacks:  opts.ContextFallbacks,
		MinImageDimension: opts.MinImageDimension,
		Retry:             opts.Retry,
		TimeoutSeconds:    int(opts.Timeout.Seconds()),
		TextGrounding:     opts.TextGrounding,
		Parallel:          opts.Parallel,
		Repeats:           opts.Repeats,
		DocumentsSelected: documents,
		JobsTotal:         len(results),
		Succeeded:         countStatus(results, "succeeded"),
		Skipped:           countStatus(results, "skipped"),
		Failed:            countStatus(results, "failed"),
	}
}

func writeErrors(path string, results []jobResult) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, result := range results {
		if result.Status != "failed" {
			continue
		}
		if err := encoder.Encode(struct {
			Input  string `json:"input"`
			Repeat int    `json:"repeat"`
			Output string `json:"output"`
			Error  string `json:"error"`
		}{Input: result.Job.Document.Relative, Repeat: result.Job.Repeat, Output: result.Job.Output, Error: result.Error}); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(path string, value interface{}) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func nonEmptyFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

func countStatus(results []jobResult, status string) int {
	count := 0
	for _, result := range results {
		if result.Status == status {
			count++
		}
	}
	return count
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
