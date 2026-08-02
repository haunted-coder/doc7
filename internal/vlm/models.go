package vlm

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

func ListModelsOpenAICompatible(ctx context.Context, config Config, httpClient *http.Client) ([]ModelInfo, error) {
	endpoint, err := modelsEndpoint(config.BaseURL)
	if err != nil {
		return nil, NewError(ConfigError, err.Error(), false, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, NewError(ConfigError, "failed to create models request", false, err)
	}
	if apiKey := strings.TrimSpace(config.APIKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := configuredHTTPClient(config, httpClient).Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, NewError(TimeoutError, "models request canceled or timed out", true, err)
		}
		return nil, NewError(TimeoutError, "models request failed", true, err)
	}
	rawBody, readErr := readHTTPResponseBody(resp.Body, "models")
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, NewError(TimeoutError, "failed to close models response", true, closeErr)
	}
	if resp.StatusCode >= 400 {
		return nil, classifyHTTPError(resp.StatusCode, rawBody, endpoint)
	}

	var decoded struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &decoded); err != nil {
		return nil, NewError(ParseError, "failed to parse models response JSON", false, err)
	}
	if decoded.Data == nil {
		return nil, NewError(ParseError, "models response does not contain data", false, nil)
	}
	models := uniqueModels(decoded.Data)
	if len(models) == 0 {
		return []ModelInfo{}, nil
	}
	return models, nil
}

func uniqueModels(models []ModelInfo) []ModelInfo {
	byID := make(map[string]ModelInfo, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		if _, exists := byID[model.ID]; !exists {
			byID[model.ID] = model
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]ModelInfo, 0, len(ids))
	for _, id := range ids {
		result = append(result, byID[id])
	}
	return result
}
