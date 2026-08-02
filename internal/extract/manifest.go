package extract

import (
	"encoding/json"
	"os"
	"time"

	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/render"
	"github.com/magicrew/doc7/internal/vlm"
)

type PageStatus string

const (
	StatusSuccess PageStatus = "success"
	StatusError   PageStatus = "error"
)

type PageMeta struct {
	Version                  string     `json:"version"`
	Page                     int        `json:"page"`
	InputPath                string     `json:"input_path"`
	ImagePath                string     `json:"image_path,omitempty"`
	ImageSHA256              string     `json:"image_sha256"`
	EmbeddedTextSHA256       string     `json:"embedded_text_sha256,omitempty"`
	EmbeddedTextChars        int        `json:"embedded_text_chars,omitempty"`
	EmbeddedTextTruncated    bool       `json:"embedded_text_truncated,omitempty"`
	EmbeddedTextSupported    bool       `json:"embedded_text_supported,omitempty"`
	EmbeddedTextChecked      bool       `json:"embedded_text_checked,omitempty"`
	TextGrounding            bool       `json:"text_grounding,omitempty"`
	GroundingChecked         bool       `json:"grounding_checked,omitempty"`
	GroundingCorrected       bool       `json:"grounding_corrected,omitempty"`
	GroundingSkipped         bool       `json:"grounding_skipped,omitempty"`
	GroundingUnresolved      []string   `json:"grounding_unresolved,omitempty"`
	GroundingError           *PageError `json:"grounding_error,omitempty"`
	CacheKey                 string     `json:"cache_key"`
	PromptName               string     `json:"prompt_name"`
	PromptSHA256             string     `json:"prompt_sha256"`
	Provider                 string     `json:"provider"`
	BaseURL                  string     `json:"base_url"`
	Model                    string     `json:"model"`
	MaxTokens                int        `json:"max_tokens"`
	RequestImageMaxDimension int        `json:"request_image_max_dimension,omitempty"`
	ContextFallbacksUsed     int        `json:"context_fallbacks_used,omitempty"`
	Status                   PageStatus `json:"status"`
	Cached                   bool       `json:"cached"`
	DurationMS               int64      `json:"duration_ms"`
	Usage                    vlm.Usage  `json:"usage"`
	Error                    *PageError `json:"error"`
}

type PageError struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type ResumeInfo struct {
	PreviousManifestPath   string `json:"previous_manifest_path"`
	PreviousManifestSHA256 string `json:"previous_manifest_sha256"`
	PageSelection          string `json:"page_selection"`
	PagesProcessed         int    `json:"pages_processed"`
	PagesRetained          int    `json:"pages_retained"`
}

type Manifest struct {
	Version string       `json:"version"`
	Command string       `json:"command"`
	Mode    string       `json:"mode,omitempty"`
	Input   detect.Input `json:"input"`
	Render  struct {
		DPI             int    `json:"dpi"`
		PageCount       int    `json:"page_count"`
		SourcePageCount int    `json:"source_page_count,omitempty"`
		PageSelection   string `json:"page_selection,omitempty"`
		ImageFormat     string `json:"image_format"`
		OutputDir       string `json:"output_dir,omitempty"`
	} `json:"render"`
	VLM struct {
		Provider            string   `json:"provider"`
		Providers           []string `json:"providers,omitempty"`
		BaseURL             string   `json:"base_url"`
		BaseURLs            []string `json:"base_urls,omitempty"`
		Model               string   `json:"model"`
		Models              []string `json:"models,omitempty"`
		Mixed               bool     `json:"mixed,omitempty"`
		PromptName          string   `json:"prompt_name"`
		ImageDetail         string   `json:"image_detail"`
		MaxImageBytes       int64    `json:"max_image_bytes"`
		MaxTokens           int      `json:"max_tokens"`
		ContextFallbacks    int      `json:"context_fallbacks"`
		MinImageDimension   int      `json:"min_image_dimension"`
		TextGrounding       bool     `json:"text_grounding"`
		EmbeddedTextVersion string   `json:"embedded_text_version,omitempty"`
		GroundingVersion    string   `json:"grounding_version,omitempty"`
		Temperature         float64  `json:"temperature"`
		OutputNormalization string   `json:"output_normalization"`
		TimeoutSeconds      int      `json:"timeout_seconds"`
		RetryCount          int      `json:"retry_count"`
		Workers             int      `json:"workers"`
	} `json:"vlm"`
	Summary struct {
		PagesTotal        int       `json:"pages_total"`
		PagesProcessed    int       `json:"pages_processed,omitempty"`
		PagesRetained     int       `json:"pages_retained,omitempty"`
		PagesSuccess      int       `json:"pages_success"`
		PagesFailed       int       `json:"pages_failed"`
		PagesCached       int       `json:"pages_cached"`
		GroundingWarnings int       `json:"grounding_warnings"`
		StartedAt         time.Time `json:"started_at"`
		FinishedAt        time.Time `json:"finished_at"`
	} `json:"summary"`
	Resume *ResumeInfo    `json:"resume,omitempty"`
	Pages  []ManifestPage `json:"pages"`
}

type ManifestPage struct {
	Page                     int        `json:"page"`
	ImagePath                string     `json:"image_path,omitempty"`
	MarkdownPath             string     `json:"markdown_path"`
	MetaPath                 string     `json:"meta_path"`
	EmbeddedTextSHA256       string     `json:"embedded_text_sha256,omitempty"`
	EmbeddedTextChars        int        `json:"embedded_text_chars,omitempty"`
	EmbeddedTextTruncated    bool       `json:"embedded_text_truncated,omitempty"`
	EmbeddedTextSupported    bool       `json:"embedded_text_supported,omitempty"`
	EmbeddedTextChecked      bool       `json:"embedded_text_checked,omitempty"`
	Status                   PageStatus `json:"status"`
	Cached                   bool       `json:"cached"`
	CacheKey                 string     `json:"cache_key"`
	DurationMS               int64      `json:"duration_ms"`
	Usage                    vlm.Usage  `json:"usage"`
	RequestImageMaxDimension int        `json:"request_image_max_dimension,omitempty"`
	ContextFallbacksUsed     int        `json:"context_fallbacks_used,omitempty"`
	Error                    *PageError `json:"error"`
}

func writeJSON(path string, value interface{}) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func renderManifestPage(outputDir string, page render.PageImage, mdPath string, metaPath string, meta PageMeta, keepImages bool) ManifestPage {
	imagePath := ""
	if keepImages {
		imagePath = render.RelativeArtifactPath(outputDir, page.ImagePath)
	}
	return ManifestPage{
		Page:                     page.Page,
		ImagePath:                imagePath,
		MarkdownPath:             render.RelativeArtifactPath(outputDir, mdPath),
		MetaPath:                 render.RelativeArtifactPath(outputDir, metaPath),
		EmbeddedTextSHA256:       page.EmbeddedTextSHA256,
		EmbeddedTextChars:        page.EmbeddedTextChars,
		EmbeddedTextTruncated:    page.EmbeddedTextTruncated,
		EmbeddedTextSupported:    page.EmbeddedTextSupported,
		EmbeddedTextChecked:      page.EmbeddedTextChecked,
		Status:                   meta.Status,
		Cached:                   meta.Cached,
		CacheKey:                 meta.CacheKey,
		DurationMS:               meta.DurationMS,
		Usage:                    meta.Usage,
		RequestImageMaxDimension: meta.RequestImageMaxDimension,
		ContextFallbacksUsed:     meta.ContextFallbacksUsed,
		Error:                    meta.Error,
	}
}
