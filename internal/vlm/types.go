package vlm

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Config struct {
	Provider          string
	BaseURL           string
	Model             string
	APIKey            string
	ImageDetail       string
	MaxImageBytes     int64
	MaxTokens         int
	ContextFallbacks  int
	MinImageDimension int
	Timeout           time.Duration
	ExtraBody         map[string]json.RawMessage
}

type Request struct {
	Prompt      string
	ImagePath   string
	ImageMIME   string
	ImageDetail string
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Response struct {
	Content                  string
	Usage                    Usage
	RawModel                 string
	FinishReason             string
	RequestImageMaxDimension int
	ContextFallbacksUsed     int
}

type Client interface {
	Complete(ctx context.Context, request Request) (Response, error)
}

func NewOpenAICompatible(config Config, httpClient *http.Client) (Client, error) {
	return newOpenAIClient(config, httpClient)
}
