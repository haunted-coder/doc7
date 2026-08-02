package vlm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

func CompleteTextOpenAICompatible(ctx context.Context, config Config, prompt string, httpClient *http.Client) (Response, error) {
	client, err := newOpenAIClient(config, httpClient)
	if err != nil {
		return Response{}, err
	}
	openai, ok := client.(*openAIClient)
	if !ok {
		return Response{}, NewError(ConfigError, "failed to create OpenAI-compatible text client", false, nil)
	}
	return openai.CompleteText(ctx, prompt)
}

func (c *openAIClient) CompleteText(ctx context.Context, prompt string) (Response, error) {
	body := chatCompletionRequest{
		Model:       c.config.Model,
		MaxTokens:   c.config.MaxTokens,
		Temperature: deterministicTemperature(),
		Messages: []chatMessage{
			{
				Role: "user",
				Content: []chatContentPart{
					{Type: "text", Text: prompt},
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
		return Response{}, completionLimitError(c.config.MaxTokens, decoded.Usage)
	}
	return Response{Content: content, Usage: decoded.Usage, RawModel: decoded.Model, FinishReason: finishReason}, nil
}
