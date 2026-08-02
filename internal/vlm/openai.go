package vlm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type openAIClient struct {
	config   Config
	http     *http.Client
	endpoint string
}

func newOpenAIClient(config Config, httpClient *http.Client) (Client, error) {
	endpoint, err := chatCompletionsEndpoint(config.BaseURL)
	if err != nil {
		return nil, NewError(ConfigError, err.Error(), false, err)
	}
	if config.Model == "" {
		return nil, NewError(ConfigError, "model is required", false, nil)
	}
	httpClient = configuredHTTPClient(config, httpClient)
	return &openAIClient{config: config, http: httpClient, endpoint: endpoint}, nil
}

func configuredHTTPClient(config Config, httpClient *http.Client) *http.Client {
	if httpClient != nil {
		return httpClient
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func (c *openAIClient) Complete(ctx context.Context, request Request) (Response, error) {
	maxImageBytes := c.config.MaxImageBytes
	imageMaxDimension := 0
	contextFallbacksUsed := 0
	providerLimitAdjusted := false
	for {
		dataURL, err := dataURL(request.ImagePath, request.ImageMIME, maxImageBytes, imageMaxDimension)
		if err != nil {
			return Response{}, err
		}
		body := chatCompletionRequest{
			Model:       c.config.Model,
			MaxTokens:   c.config.MaxTokens,
			Temperature: deterministicTemperature(),
			Messages: []chatMessage{
				{
					Role: "user",
					Content: []chatContentPart{
						{Type: "text", Text: request.Prompt},
						{Type: "image_url", ImageURL: &chatImageURL{URL: dataURL, Detail: detailOrDefault(request.ImageDetail)}},
					},
				},
			},
		}
		payload, err := json.Marshal(body)
		if err != nil {
			return Response{}, NewError(ConfigError, "failed to encode request body", false, err)
		}
		if len(c.config.ExtraBody) > 0 {
			payload, err = mergeExtraBody(payload, c.config.ExtraBody)
			if err != nil {
				return Response{}, err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
		if err != nil {
			return Response{}, NewError(ConfigError, "failed to create request", false, err)
		}
		if apiKey := strings.TrimSpace(c.config.APIKey); apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return Response{}, NewError(TimeoutError, "request canceled or timed out", true, err)
			}
			return Response{}, NewError(TimeoutError, "request failed", true, err)
		}
		rawBody, readErr := readHTTPResponseBody(resp.Body, "model")
		closeErr := resp.Body.Close()
		if readErr != nil {
			return Response{}, readErr
		}
		if closeErr != nil {
			return Response{}, NewError(TimeoutError, "failed to close model response", true, closeErr)
		}
		if resp.StatusCode >= 400 {
			if !providerLimitAdjusted {
				if retryLimit, ok := providerImageLimit(rawBody, maxImageBytes); ok {
					maxImageBytes = retryLimit
					providerLimitAdjusted = true
					continue
				}
			}
			if providerContextLimitExceeded(rawBody) && contextFallbacksUsed < c.config.ContextFallbacks {
				nextDimension, ok := nextContextImageDimension(request.ImagePath, imageMaxDimension, c.config.MinImageDimension)
				if ok {
					imageMaxDimension = nextDimension
					contextFallbacksUsed++
					continue
				}
			}
			return Response{}, classifyHTTPError(resp.StatusCode, rawBody, c.endpoint)
		}
		var decoded chatCompletionResponse
		if err := json.Unmarshal(rawBody, &decoded); err != nil {
			return Response{}, NewError(ParseError, "failed to parse response JSON", false, err)
		}
		if len(decoded.Choices) == 0 {
			return Response{}, NewError(ParseError, "response does not contain choices[0].message.content", false, nil)
		}
		content, err := decodeMessageContent(decoded.Choices[0].Message.Content)
		if err != nil {
			return Response{}, err
		}
		content, _ = StripLeadingReasoningBlocks(content)
		if strings.TrimSpace(content) == "" {
			return Response{}, NewError(ParseError, "model response contains no user-visible content", false, nil)
		}
		finishReason := decoded.Choices[0].FinishReason
		if completionTruncated(finishReason) {
			if contextFallbackEligible(c.config.MaxTokens, decoded.Usage) && contextFallbacksUsed < c.config.ContextFallbacks {
				nextDimension, ok := nextContextImageDimension(request.ImagePath, imageMaxDimension, c.config.MinImageDimension)
				if ok {
					imageMaxDimension = nextDimension
					contextFallbacksUsed++
					continue
				}
			}
			return Response{}, completionLimitError(c.config.MaxTokens, decoded.Usage)
		}
		return Response{
			Content:                  content,
			Usage:                    decoded.Usage,
			RawModel:                 decoded.Model,
			FinishReason:             finishReason,
			RequestImageMaxDimension: imageMaxDimension,
			ContextFallbacksUsed:     contextFallbacksUsed,
		}, nil
	}
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
}

func deterministicTemperature() *float64 {
	value := 0.0
	return &value
}

type chatMessage struct {
	Role    string            `json:"role"`
	Content []chatContentPart `json:"content"`
}

type chatContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *chatImageURL `json:"image_url,omitempty"`
}

type chatImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

func completionTruncated(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length", "max_tokens":
		return true
	default:
		return false
	}
}

func completionLimitError(configuredLimit int, usage Usage) error {
	message := "model output was truncated"
	if usage.CompletionTokens > 0 {
		message += fmt.Sprintf(" after %d completion tokens", usage.CompletionTokens)
	}
	if usage.PromptTokens > 0 || usage.TotalTokens > 0 {
		message += fmt.Sprintf(" (%d prompt, %d total)", usage.PromptTokens, usage.TotalTokens)
	}
	if configuredLimit > 0 && usage.CompletionTokens > 0 && usage.CompletionTokens < configuredLimit {
		message += fmt.Sprintf("; the provider stopped before configured max_tokens=%d, usually because the model context window or provider generation limit was reached", configuredLimit)
		err := NewError(ParseError, message+"; increase the model context window or reduce input size (for images, lower --dpi or --image-detail)", false, nil)
		err.Usage = &usage
		return err
	}
	if configuredLimit > 0 {
		message += fmt.Sprintf(" at configured max_tokens=%d; increase --max-tokens", configuredLimit)
	} else {
		message += "; increase the model output token limit"
	}
	err := NewError(ParseError, message, false, nil)
	err.Usage = &usage
	return err
}

func decodeMessageContent(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return "", NewError(ParseError, "response does not contain choices[0].message.content", false, nil)
		}
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", NewError(ParseError, "response message content must be text or text parts", false, err)
	}
	var content strings.Builder
	for _, part := range parts {
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		if content.Len() > 0 {
			content.WriteByte('\n')
		}
		content.WriteString(part.Text)
	}
	if content.Len() == 0 {
		return "", NewError(ParseError, "response does not contain choices[0].message.content", false, nil)
	}
	return content.String(), nil
}

func mergeExtraBody(base []byte, extra map[string]json.RawMessage) ([]byte, error) {
	body := map[string]json.RawMessage{}
	if err := json.Unmarshal(base, &body); err != nil {
		return nil, NewError(ConfigError, "failed to decode base request body", false, err)
	}
	for key, raw := range extra {
		if key == "" {
			continue
		}
		if !json.Valid(raw) {
			return nil, NewError(ConfigError, "invalid extra body JSON for "+key, false, nil)
		}
		body[key] = raw
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, NewError(ConfigError, "failed to encode merged request body", false, err)
	}
	return payload, nil
}

func dataURL(path string, providedMIME string, maxBytes int64, maxDimension int) (string, error) {
	imagePath := path
	mimeType := providedMIME
	cleanups := []func(){}
	if maxDimension > 0 {
		prepared, preparedMIME, cleanup, err := prepareImageForDimension(imagePath, maxDimension)
		if err != nil {
			return "", err
		}
		imagePath = prepared
		if preparedMIME != "" {
			mimeType = preparedMIME
		}
		cleanups = append(cleanups, cleanup)
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxImageBytes
	}
	if info, err := os.Stat(imagePath); err == nil && info.Size() > maxBytes {
		prepared, preparedMIME, cleanup, err := prepareOversizeImage(imagePath, maxBytes)
		if err != nil {
			return "", err
		}
		imagePath = prepared
		mimeType = preparedMIME
		cleanups = append(cleanups, cleanup)
	}
	defer func() {
		for index := len(cleanups) - 1; index >= 0; index-- {
			cleanups[index]()
		}
	}()
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return "", NewError(ConfigError, "failed to read image", false, err)
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(imagePath)))
	}
	if mimeType == "" {
		mimeType = "image/png"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)), nil
}

func detailOrDefault(value string) string {
	if value == "" {
		return "high"
	}
	return value
}

func classifyHTTPError(status int, body []byte, endpoint string) error {
	message := fmt.Sprintf("HTTP %d", status)
	if statusText := http.StatusText(status); statusText != "" {
		message += " " + statusText
	}
	if redactedEndpoint := RedactedEndpoint(endpoint); redactedEndpoint != "" {
		message += " from " + redactedEndpoint
	}
	if detail := strings.TrimSpace(string(body)); detail != "" {
		message += ": " + detail
	}
	switch {
	case status == http.StatusTooManyRequests:
		return NewError(RateLimitError, message, true, nil)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return NewError(AuthError, message, false, nil)
	case status == http.StatusNotFound || status == http.StatusBadRequest:
		return NewError(ModelError, message, false, nil)
	case status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout:
		message += "; the HTTP gateway or proxy could not obtain a usable response from the model server. Verify that the model server is running, listens on a network-reachable address, and the configured base URL and proxy route reach the correct upstream"
		return NewError(ServerError, message, true, nil)
	case status >= 500:
		return NewError(ServerError, message, true, nil)
	default:
		return NewError(ModelError, message, false, nil)
	}
}
