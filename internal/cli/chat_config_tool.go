package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/discovery"
	"github.com/magicrew/doc7/internal/vlm"
)

var configurationChatTools = []vlm.AgentTool{
	askUserChatTool,
	{
		Name:        "get_configuration",
		Description: "Show the effective doc7 configuration, its file path, editable keys, and credential source without revealing secrets.",
		Parameters:  []byte(`{"type":"object","properties":{},"additionalProperties":false}`),
	},
	{
		Name:        "discover_local_models",
		Description: "Discover models currently exposed by local LM Studio or Ollama-compatible runtimes. This does not change configuration.",
		Parameters: []byte(`{
  "type": "object",
  "properties": {
    "runtime": {"type": "string", "enum": ["any", "lm_studio", "ollama"], "description": "Optional local runtime filter"}
  },
  "additionalProperties": false
}`),
	},
	{
		Name:        "verify_model_configuration",
		Description: "Send a real vision probe to an OpenAI-compatible endpoint and model. Use this before proposing a model switch.",
		Parameters: []byte(`{
  "type": "object",
  "properties": {
    "base_url": {"type": "string", "description": "OpenAI-compatible endpoint ending at /v1"},
    "model": {"type": "string", "description": "Model ID returned by the endpoint"}
  },
  "required": ["base_url", "model"],
  "additionalProperties": false
}`),
	},
	{
		Name:        "set_configuration",
		Description: "Dry-run or apply a small validated configuration change. Without confirmation_id it only creates a proposal and never writes the file.",
		Parameters: []byte(`{
  "type": "object",
  "properties": {
    "changes": {
      "type": "array",
      "minItems": 1,
      "maxItems": 12,
      "items": {
        "type": "object",
        "properties": {
          "key": {"type": "string", "description": "One editable doc7 configuration key"},
          "value": {"type": "string", "description": "New value; never place a secret here"}
        },
        "required": ["key", "value"],
        "additionalProperties": false
      }
    },
    "proposal_id": {"type": "string", "description": "Proposal ID returned by the dry-run"},
    "confirmation_id": {"type": "string", "description": "Interaction ID returned by ask_user after the user selected confirm"}
  },
  "required": ["changes"],
  "additionalProperties": false
}`),
	},
}

type configChange struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type setConfigurationArguments struct {
	Changes        []configChange `json:"changes"`
	ProposalID     string         `json:"proposal_id"`
	ConfirmationID string         `json:"confirmation_id"`
}

type pendingConfigChange struct {
	ID             string
	Changes        []configChange
	ConfirmationID string
}

func isConfigurationTool(name string) bool {
	switch name {
	case "ask_user", "get_configuration", "discover_local_models", "verify_model_configuration", "set_configuration":
		return true
	default:
		return false
	}
}

func (a *chatAgent) executeConfigurationTool(call vlm.AgentToolCall) chatToolExecution {
	switch call.Function.Name {
	case "ask_user":
		return a.executeAskUser(call.Function.Arguments)
	case "get_configuration":
		return a.executeGetConfiguration()
	case "discover_local_models":
		return a.executeDiscoverLocalModels(call.Function.Arguments)
	case "verify_model_configuration":
		return a.executeVerifyModel(call.Function.Arguments)
	case "set_configuration":
		return a.executeSetConfiguration(call.Function.Arguments)
	default:
		return chatToolExecution{result: encodeChatToolResult(false, "unsupported configuration tool"), status: chatToolContinue}
	}
}

func (a *chatAgent) executeGetConfiguration() chatToolExecution {
	fields := editableConfigFields(a.config)
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		value := field.Value
		if field.Key == "base_url" {
			value = vlm.RedactedEndpoint(value)
		}
		values[field.Key] = valueOrNotConfigured(value)
	}
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":                true,
		"config_path":       config.EffectivePath(globals.ConfigPath),
		"values":            values,
		"credential_source": credentialDisplay(a.config.APIKeySource),
		"secret_available":  strings.TrimSpace(a.config.APIKey) != "",
	}), status: chatToolContinue}
}

func (a *chatAgent) executeDiscoverLocalModels(raw string) chatToolExecution {
	var arguments struct {
		Runtime string `json:"runtime"`
	}
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "invalid discover_local_models arguments: "+err.Error()), status: chatToolContinue}
	}
	candidates := discovery.LocalModelsFiltered(a.command.Context(), strings.TrimSpace(arguments.Runtime))
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":         true,
		"candidates": candidates,
	}), status: chatToolContinue}
}

func (a *chatAgent) executeVerifyModel(raw string) chatToolExecution {
	var arguments struct {
		BaseURL string `json:"base_url"`
		Model   string `json:"model"`
	}
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "invalid verify_model_configuration arguments: "+err.Error()), status: chatToolContinue}
	}
	arguments.BaseURL = strings.TrimSpace(arguments.BaseURL)
	arguments.Model = strings.TrimSpace(arguments.Model)
	if err := validateAgentEndpoint(arguments.BaseURL); err != nil || arguments.Model == "" {
		if err == nil {
			err = errors.New("model is required")
		}
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	credentialConfig := a.config
	credentialConfig.BaseURL = arguments.BaseURL
	credentialConfig.Model = arguments.Model
	for _, change := range pendingChanges(a.pendingConfig) {
		switch change.Key {
		case "credential_store":
			credentialConfig.CredentialStore = change.Value
		case "credential_account":
			credentialConfig.CredentialAccount = change.Value
		case "api_key_env":
			credentialConfig.APIKeyEnv = change.Value
		}
	}
	credentialConfig, err := config.ResolveCredentials(credentialConfig, config.EnvMap())
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "failed to load the configured credential source"), status: chatToolContinue}
	}
	err = discovery.VerifyVisionWithAPIKey(a.command.Context(), discovery.Candidate{
		ServerName: "configured endpoint",
		BaseURL:    arguments.BaseURL,
		Model:      arguments.Model,
	}, credentialConfig.APIKey)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":       true,
		"base_url": vlm.RedactedEndpoint(arguments.BaseURL),
		"model":    arguments.Model,
		"message":  translate("chat.model_verified"),
	}), status: chatToolContinue}
}

func pendingChanges(pending *pendingConfigChange) []configChange {
	if pending == nil {
		return nil
	}
	return pending.Changes
}

func (a *chatAgent) executeSetConfiguration(raw string) chatToolExecution {
	var arguments setConfigurationArguments
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "invalid set_configuration arguments: "+err.Error()), status: chatToolContinue}
	}
	changes, err := normalizeConfigChanges(arguments.Changes)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	proposalID := strings.TrimSpace(arguments.ProposalID)
	confirmationID := strings.TrimSpace(arguments.ConfirmationID)
	if proposalID == "" && a.pendingConfig != nil && confirmationID != "" {
		proposalID = a.pendingConfig.ID
	}
	if confirmationID == "" && a.pendingConfig != nil {
		confirmationID = a.pendingConfig.ConfirmationID
	}
	if proposalID == "" {
		proposalID := newChatID()
		a.pendingConfig = &pendingConfigChange{ID: proposalID, Changes: changes}
		return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
			"ok":          true,
			"dry_run":     true,
			"proposal_id": proposalID,
			"changes":     safeConfigChanges(changes),
			"message":     translate("chat.configuration_dry_run"),
		}), status: chatToolContinue}
	}
	if a.pendingConfig == nil || a.pendingConfig.ID != proposalID || !sameConfigChanges(a.pendingConfig.Changes, changes) {
		return chatToolExecution{result: encodeChatToolResult(false, "the configuration proposal is missing, expired, or changed"), status: chatToolContinue}
	}
	if !globals.Yes {
		interaction, ok := a.interactions[confirmationID]
		if !ok || !interaction.Confirmed {
			return chatToolExecution{result: encodeChatToolResult(false, "configuration changes require an ask_user interaction where the user selected confirm"), status: chatToolContinue}
		}
	}
	values := make(map[string]string, len(changes))
	for _, change := range changes {
		values[change.Key] = change.Value
	}
	path, err := config.SetUserValues(globals.ConfigPath, values)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "failed to save configuration: "+err.Error()), status: chatToolContinue}
	}
	updated, err := loadConfig(config.FlagConfig{}, false)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "configuration was written but could not be reloaded: "+err.Error()), status: chatToolContinue}
	}
	a.config = updated
	a.pendingConfig = nil
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":          true,
		"applied":     true,
		"config_path": path,
		"changes":     safeConfigChanges(changes),
	}), status: chatToolConfigurationApplied}
}

func normalizeConfigChanges(changes []configChange) ([]configChange, error) {
	if len(changes) == 0 || len(changes) > 12 {
		return nil, errors.New("set_configuration requires one to twelve changes")
	}
	result := make([]configChange, 0, len(changes))
	seen := make(map[string]bool, len(changes))
	for _, change := range changes {
		key := normalizeConfigKey(change.Key)
		value := strings.TrimSpace(change.Value)
		if key == "api_key" || strings.Contains(key, "secret") || strings.Contains(key, "token") {
			return nil, errors.New("secrets cannot be configured through chat; use doc7 setup config --api-key-stdin")
		}
		if seen[key] {
			return nil, fmt.Errorf("configuration key appears more than once: %s", key)
		}
		if err := validateConfigValue(key, value); err != nil {
			return nil, err
		}
		if key == "provider" && value != "openai-compatible" {
			return nil, errors.New("provider must be openai-compatible")
		}
		if key == "base_url" {
			if err := validateAgentEndpoint(value); err != nil {
				return nil, err
			}
		}
		seen[key] = true
		result = append(result, configChange{Key: key, Value: value})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Key < result[right].Key })
	return result, nil
}

func safeConfigChanges(changes []configChange) []configChange {
	result := make([]configChange, len(changes))
	copy(result, changes)
	for index := range result {
		if result[index].Key == "base_url" {
			result[index].Value = vlm.RedactedEndpoint(result[index].Value)
		}
	}
	return result
}

func sameConfigChanges(left, right []configChange) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validateAgentEndpoint(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("base_url must be a valid HTTP or HTTPS endpoint")
	}
	if parsed.User != nil || parsed.Fragment != "" || len(parsed.RawQuery) > 0 {
		return errors.New("base_url must not contain credentials, fragments, or query parameters")
	}
	return nil
}

func encodeChatJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return `{"ok":false,"message":"failed to encode chat tool result"}`
	}
	return string(data)
}
