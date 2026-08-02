package mcpserver

import (
	"time"

	"github.com/magicrew/doc7/internal/read"
)

type Config struct {
	OutputRoot     string
	Retention      time.Duration
	ServiceVersion string
	ReadOptions    read.Options
}

type ConvertInput struct {
	Input            string `json:"input" jsonschema:"Local path, HTTP(S) URL, directory, or ZIP archive to convert"`
	OutputDir        string `json:"output_dir,omitempty" jsonschema:"Optional persistent output directory; doc7 creates one when omitted"`
	Prompt           string `json:"prompt,omitempty" jsonschema:"Conversion prompt profile: auto, document, or slide"`
	Pages            string `json:"pages,omitempty" jsonschema:"Optional 1-based pages and inclusive ranges, for example 1,3-5"`
	Resume           bool   `json:"resume,omitempty" jsonschema:"Retry failed pages from an existing output directory while preserving successful pages"`
	Merge            *bool  `json:"merge,omitempty" jsonschema:"Write merged Markdown for each document; defaults to true"`
	IncludeMarkdown  *bool  `json:"include_markdown,omitempty" jsonschema:"Include merged Markdown in the tool response; defaults to true"`
	MaxMarkdownChars int    `json:"max_markdown_chars,omitempty" jsonschema:"Maximum Markdown characters returned inline; 0 means unlimited"`
}

type ConvertOutput struct {
	OK                bool   `json:"ok"`
	Mode              string `json:"mode"`
	Input             string `json:"input"`
	OutputDir         string `json:"output_dir"`
	MarkdownPath      string `json:"markdown_path,omitempty"`
	Markdown          string `json:"markdown,omitempty"`
	MarkdownTruncated bool   `json:"markdown_truncated,omitempty"`
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
