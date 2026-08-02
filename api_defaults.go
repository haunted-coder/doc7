package doc7

import "time"

const defaultMaxImageBytes int64 = 9 * 1024 * 1024

// DefaultOptions returns the same defaults used by the read CLI for one
// document. Callers can override only the fields they need.
func DefaultOptions() Options {
	return Options{
		PromptName:        "auto",
		Merge:             true,
		Workers:           5,
		RetryCount:        3,
		DPI:               220,
		KeepImages:        true,
		Provider:          "openai-compatible",
		ImageDetail:       "high",
		MaxImageBytes:     defaultMaxImageBytes,
		MaxTokens:         8192,
		ContextFallbacks:  2,
		MinImageDimension: 720,
		Timeout:           120 * time.Second,
	}
}

// DefaultBatchOptions returns the same defaults used by the read CLI for a
// recursive directory conversion.
func DefaultBatchOptions() BatchOptions {
	return BatchOptions{
		PromptName:        "auto",
		Merge:             true,
		FileWorkers:       1,
		Workers:           5,
		RetryCount:        3,
		DPI:               220,
		KeepImages:        true,
		Provider:          "openai-compatible",
		ImageDetail:       "high",
		MaxImageBytes:     defaultMaxImageBytes,
		MaxTokens:         8192,
		ContextFallbacks:  2,
		MinImageDimension: 720,
		Timeout:           120 * time.Second,
	}
}
