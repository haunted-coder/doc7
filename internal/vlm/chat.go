package vlm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type AgentMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []AgentToolCall `json:"tool_calls,omitempty"`
}

type AgentTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type AgentToolCall struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function AgentToolFunction `json:"function"`
}

type AgentToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type AgentResponse struct {
	Content      string
	ToolCalls    []AgentToolCall
	Usage        Usage
	RawModel     string
	FinishReason string
}

type agentChatRequest struct {
	Model       string             `json:"model"`
	Messages    []AgentMessage     `json:"messages"`
	Tools       []agentRequestTool `json:"tools,omitempty"`
	ToolChoice  string             `json:"tool_choice,omitempty"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
}

type agentRequestTool struct {
	Type     string    `json:"type"`
	Function AgentTool `json:"function"`
}

type agentChatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   json.RawMessage `json:"content"`
			ToolCalls []AgentToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

func CompleteAgentChatOpenAICompatible(
	ctx context.Context,
	config Config,
	messages []AgentMessage,
	tools []AgentTool,
	httpClient *http.Client,
) (AgentResponse, error) {
	client, err := newOpenAIClient(config, httpClient)
	if err != nil {
		return AgentResponse{}, err
	}
	openai, ok := client.(*openAIClient)
	if !ok {
		return AgentResponse{}, NewError(ConfigError, "failed to create OpenAI-compatible agent client", false, nil)
	}
	return openai.completeAgentChat(ctx, messages, tools)
}

func (c *openAIClient) completeAgentChat(ctx context.Context, messages []AgentMessage, tools []AgentTool) (AgentResponse, error) {
	requestTools := make([]agentRequestTool, 0, len(tools))
	for _, tool := range tools {
		requestTools = append(requestTools, agentRequestTool{Type: "function", Function: tool})
	}
	body := agentChatRequest{
		Model:       c.config.Model,
		Messages:    messages,
		Tools:       requestTools,
		MaxTokens:   c.config.MaxTokens,
		Temperature: deterministicTemperature(),
	}
	if len(requestTools) > 0 {
		body.ToolChoice = "auto"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return AgentResponse{}, NewError(ConfigError, "failed to encode agent request body", false, err)
	}
	if len(c.config.ExtraBody) > 0 {
		payload, err = mergeExtraBody(payload, c.config.ExtraBody)
		if err != nil {
			return AgentResponse{}, err
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return AgentResponse{}, NewError(ConfigError, "failed to create agent request", false, err)
	}
	if apiKey := strings.TrimSpace(c.config.APIKey); apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return AgentResponse{}, NewError(TimeoutError, "agent request canceled or timed out", true, err)
		}
		return AgentResponse{}, NewError(TimeoutError, "agent request failed", true, err)
	}
	rawBody, readErr := readHTTPResponseBody(response.Body, "agent model")
	closeErr := response.Body.Close()
	if readErr != nil {
		return AgentResponse{}, readErr
	}
	if closeErr != nil {
		return AgentResponse{}, NewError(TimeoutError, "failed to close agent response", true, closeErr)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return AgentResponse{}, classifyHTTPError(response.StatusCode, rawBody, c.endpoint)
	}

	var decoded agentChatResponse
	if err := json.Unmarshal(rawBody, &decoded); err != nil {
		return AgentResponse{}, NewError(ParseError, "failed to parse agent response JSON", false, err)
	}
	if len(decoded.Choices) == 0 {
		return AgentResponse{}, NewError(ParseError, "agent response does not contain choices[0]", false, nil)
	}
	choice := decoded.Choices[0]
	content := decodeOptionalMessageContent(choice.Message.Content)
	content, _ = StripLeadingReasoningBlocks(content)
	if strings.TrimSpace(content) == "" && len(choice.Message.ToolCalls) == 0 {
		return AgentResponse{}, NewError(ParseError, "agent response contains neither content nor tool calls", false, nil)
	}
	if completionTruncated(choice.FinishReason) {
		return AgentResponse{}, completionLimitError(c.config.MaxTokens, decoded.Usage)
	}
	return AgentResponse{
		Content:      content,
		ToolCalls:    choice.Message.ToolCalls,
		Usage:        decoded.Usage,
		RawModel:     decoded.Model,
		FinishReason: choice.FinishReason,
	}, nil
}

func decodeOptionalMessageContent(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	content, err := decodeMessageContent(raw)
	if err != nil {
		return ""
	}
	return content
}
