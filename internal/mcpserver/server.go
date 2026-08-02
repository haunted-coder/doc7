package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/magicrew/doc7/internal/read"
	"github.com/magicrew/doc7/internal/render"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultRetention = 7 * 24 * time.Hour

type Server struct {
	config Config
	server *sdkmcp.Server
}

func New(config Config) (*Server, error) {
	config = normalizeConfig(config)
	absoluteRoot, err := filepath.Abs(config.OutputRoot)
	if err != nil {
		return nil, err
	}
	config.OutputRoot = absoluteRoot
	if err := os.MkdirAll(config.OutputRoot, 0o700); err != nil {
		return nil, err
	}
	if err := cleanupExpired(config.OutputRoot, config.Retention); err != nil {
		return nil, err
	}
	implementation := &sdkmcp.Implementation{Name: "doc7", Version: config.ServiceVersion}
	sdk := sdkmcp.NewServer(implementation, &sdkmcp.ServerOptions{
		Instructions: "Use convert_to_markdown to turn local documents, directories, HTTP(S) URLs, and ZIP archives into AI-ready Markdown.",
		Capabilities: &sdkmcp.ServerCapabilities{},
	})
	value := &Server{config: config, server: sdk}
	sdkmcp.AddTool(sdk, &sdkmcp.Tool{
		Name:        "convert_to_markdown",
		Description: "Convert a document, directory, HTTP(S) URL, or ZIP archive into Markdown and persisted page-level artifacts.",
	}, value.convert)
	return value, nil
}

func DefaultOutputRoot() string {
	if cache, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cache) != "" {
		return filepath.Join(cache, "doc7", "mcp")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".doc7", "mcp")
	}
	return filepath.Join(".doc7", "mcp")
}

func DefaultRetention() time.Duration {
	return defaultRetention
}

func (s *Server) Run(ctx context.Context) error {
	return s.server.Run(ctx, &sdkmcp.StdioTransport{})
}

func (s *Server) convert(ctx context.Context, _ *sdkmcp.CallToolRequest, input ConvertInput) (*sdkmcp.CallToolResult, ConvertOutput, error) {
	input.Input = strings.TrimSpace(input.Input)
	if input.Input == "" {
		return nil, ConvertOutput{}, errors.New("input is required")
	}
	if input.MaxMarkdownChars < 0 {
		return nil, ConvertOutput{}, errors.New("max_markdown_chars must be 0 or greater")
	}
	pages, err := render.NormalizePageSelection(input.Pages)
	if err != nil {
		return nil, ConvertOutput{}, fmt.Errorf("invalid pages: %w", err)
	}
	merge := true
	if input.Merge != nil {
		merge = *input.Merge
	}
	includeMarkdown := true
	if input.IncludeMarkdown != nil {
		includeMarkdown = *input.IncludeMarkdown
	}
	outputDir, err := s.outputDir(input.OutputDir)
	if err != nil {
		return nil, ConvertOutput{}, err
	}
	options := s.config.ReadOptions
	options.OutputDir = outputDir
	options.Pages = pages
	options.Resume = input.Resume
	options.Merge = merge
	if prompt := strings.TrimSpace(input.Prompt); prompt != "" {
		options.PromptName = prompt
	}
	value, runErr := read.Run(ctx, input.Input, options)
	output, outputErr := convertOutput(value, includeMarkdown, input.MaxMarkdownChars)
	if outputErr != nil && runErr == nil {
		runErr = outputErr
	}
	if runErr != nil {
		message := fmt.Errorf("conversion failed; output retained at %s: %w", outputDir, runErr)
		result := &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: message.Error()}}}
		result.SetError(message)
		return result, output, nil
	}
	content := conversionText(output)
	return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: content}}}, output, nil
}

func (s *Server) outputDir(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return filepath.Abs(requested)
	}
	return os.MkdirTemp(s.config.OutputRoot, "job-")
}

func normalizeConfig(config Config) Config {
	if strings.TrimSpace(config.OutputRoot) == "" {
		config.OutputRoot = DefaultOutputRoot()
	}
	if config.Retention <= 0 {
		config.Retention = defaultRetention
	}
	if strings.TrimSpace(config.ServiceVersion) == "" {
		config.ServiceVersion = "dev"
	}
	return config
}

func cleanupExpired(root string, retention time.Duration) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	threshold := time.Now().Add(-retention)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "job-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(threshold) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func convertOutput(value read.Result, includeMarkdown bool, maxMarkdownChars int) (ConvertOutput, error) {
	output := ConvertOutput{
		Mode:      value.Mode,
		Input:     value.Input,
		OutputDir: value.OutputDir,
	}
	if value.Document != nil {
		output.OK = value.Document.OK
		output.MarkdownPath = value.Document.MergedMarkdown
		output.PagesTotal = value.Document.PagesTotal
		output.SourcePagesTotal = value.Document.SourcePagesTotal
		output.PagesProcessed = value.Document.PagesProcessed
		output.PagesRetained = value.Document.PagesRetained
		output.PagesSuccess = value.Document.PagesSuccess
		output.PagesFailed = value.Document.PagesFailed
		output.PagesCached = value.Document.PagesCached
		output.GroundingWarnings = value.Document.GroundingWarnings
		output.Resumed = value.Document.Resumed
		if includeMarkdown && value.Document.MergedMarkdown != "" {
			data, err := os.ReadFile(value.Document.MergedMarkdown)
			if err != nil {
				return output, err
			}
			output.Markdown, output.MarkdownTruncated = limitMarkdown(string(data), maxMarkdownChars)
		}
	}
	if value.Batch != nil {
		output.OK = value.Batch.OK
		output.FilesTotal = value.Batch.FilesTotal
		output.FilesDone = value.Batch.FilesDone
		output.FilesFailed = value.Batch.FilesFailed
		output.GroundingWarnings = value.Batch.GroundingWarnings
	}
	return output, nil
}

func limitMarkdown(value string, maxChars int) (string, bool) {
	if maxChars == 0 {
		return value, false
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value, false
	}
	return string(runes[:maxChars]), true
}

func conversionText(output ConvertOutput) string {
	if output.Markdown != "" {
		if output.MarkdownTruncated {
			return output.Markdown + "\n\n[doc7: inline Markdown truncated; full output: " + output.MarkdownPath + "]"
		}
		return output.Markdown
	}
	return fmt.Sprintf("doc7 conversion complete: mode=%s output=%s", output.Mode, output.OutputDir)
}
