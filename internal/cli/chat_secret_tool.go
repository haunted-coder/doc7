package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/credentials"
	"github.com/magicrew/doc7/internal/vlm"
	"golang.org/x/term"
)

var inputSecretChatTool = vlm.AgentTool{
	Name:        "input_secret",
	Description: "Confirm locally, prompt the user for an API key with terminal echo disabled, then store it without exposing the value to the model. Never accepts the secret as a tool argument.",
	Parameters: []byte(`{
  "type": "object",
  "properties": {
    "kind": {"type": "string", "enum": ["api_key"]}
  },
  "required": ["kind"],
  "additionalProperties": false
}`),
}

type inputSecretArguments struct {
	Kind string `json:"kind"`
}

func (a *chatAgent) executeInputSecret(raw string) chatToolExecution {
	var arguments inputSecretArguments
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "invalid input_secret arguments: "+err.Error()), status: chatToolContinue}
	}
	if strings.TrimSpace(arguments.Kind) != "api_key" {
		return chatToolExecution{result: encodeChatToolResult(false, "input_secret currently supports only api_key"), status: chatToolContinue}
	}
	if !stdinIsTerminal() || !term.IsTerminal(int(os.Stdin.Fd())) {
		return chatToolExecution{result: encodeChatToolResult(false, "secret input requires an interactive terminal; use doc7 setup config --api-key-stdin in non-interactive environments"), status: chatToolContinue}
	}
	store := pendingConfigValue(a.pendingConfig, "credential_store", a.config.CredentialStore)
	account := pendingConfigValue(a.pendingConfig, "credential_account", a.config.CredentialAccount)
	normalizedStore, err := credentials.NormalizeStore(store)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	if normalizedStore == credentials.StoreEnv {
		return chatToolExecution{result: encodeChatToolResult(false, "credential_store=env cannot be written by doc7; set the configured environment variable in the parent shell"), status: chatToolContinue}
	}
	confirmed, err := confirmLocalChatAction(translate("chat.secret.confirm", normalizedStore, account))
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	if !confirmed {
		return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
			"ok":              true,
			"cancelled":       true,
			"content_visible": false,
		}), status: chatToolContinue}
	}
	fmt.Fprintf(os.Stdout, "%s ", translate("chat.secret.prompt"))
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "failed to read the secret from the terminal"), status: chatToolContinue}
	}
	value := strings.TrimSpace(string(secret))
	for index := range secret {
		secret[index] = 0
	}
	if value == "" {
		return chatToolExecution{result: encodeChatToolResult(false, "secret input was empty or cancelled"), status: chatToolContinue}
	}
	source, err := credentials.Store(credentials.Options{
		Store:     normalizedStore,
		Account:   account,
		Path:      a.config.CredentialsPath,
		APIKeyEnv: a.config.APIKeyEnv,
	}, value)
	length := len(value)
	value = ""
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, secretStorageError(err)), status: chatToolContinue}
	}
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":                true,
		"stored":            true,
		"kind":              "api_key",
		"length_bytes":      length,
		"credential_source": source,
		"config_path":       config.EffectivePath(globals.ConfigPath),
		"content_visible":   false,
	}), status: chatToolContinue}
}

func pendingConfigValue(pending *pendingConfigChange, key string, fallback string) string {
	if pending != nil {
		for _, change := range pending.Changes {
			if change.Key == key {
				return change.Value
			}
		}
	}
	return fallback
}

func secretStorageError(err error) string {
	if errors.Is(err, os.ErrPermission) {
		return "permission denied while storing the API key"
	}
	return "failed to store the API key in the configured credential store"
}
