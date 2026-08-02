package cli

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/i18n"
	"github.com/magicrew/doc7/internal/vlm"
	"github.com/spf13/cobra"
)

type configField struct {
	Key         string
	Label       string
	Description string
	Value       string
}

func newConfigCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: translate("config.short"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return showConfig(cmd)
		},
	}
	command.AddCommand(newConfigShowCommand(), newConfigPathCommand(), newConfigSetCommand(), newConfigResetCommand())
	return command
}

func newConfigShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: translate("config.show.short"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return showConfig(cmd)
		},
	}
}

func newConfigPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: translate("config.path.short"),
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			writeText("%s", config.EffectivePath(globals.ConfigPath))
			return nil
		},
	}
}

func showConfig(_ *cobra.Command) error {
	cfg, err := loadConfig(config.FlagConfig{}, false)
	if err != nil {
		return err
	}
	language := i18n.New(cfg.Language).Language()
	path := config.EffectivePath(globals.ConfigPath)
	endpoint := endpointDisplay(cfg.BaseURL)
	credential := credentialDisplay(cfg.APIKeySource)
	if cfg.JSONOutput {
		return writeJSON(map[string]string{
			"config_path":        path,
			"language":           string(language),
			"language_setting":   cfg.Language,
			"provider":           cfg.Provider,
			"base_url":           vlm.RedactedEndpoint(cfg.BaseURL),
			"endpoint_type":      endpointType(cfg.BaseURL),
			"model":              cfg.Model,
			"credential_source":  cfg.APIKeySource,
			"credential_store":   cfg.CredentialStore,
			"credential_account": cfg.CredentialAccount,
			"ppt_renderer":       cfg.PPTRenderer,
			"remote_confirmed":   strconv.FormatBool(cfg.RemoteConfirmed),
		})
	}
	writeText("%s: %s", translate("config.path"), path)
	writeText("%s: %s", translate("config.language"), language)
	writeText("%s: %s", translate("config.language_source"), cfg.Language)
	writeText("%s: %s", configFieldLabel("base_url"), endpoint)
	writeText("%s: %s", configFieldLabel("model"), valueOrNotConfigured(cfg.Model))
	writeText("%s: %s", configFieldLabel("credential_store"), credential)
	writeText("%s: %s", configFieldLabel("ppt_renderer"), cfg.PPTRenderer)
	writeText("%s: %t", configFieldLabel("remote_confirmed"), cfg.RemoteConfirmed)
	return nil
}

func newConfigSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set [key] [value]",
		Short: translate("config.set.short"),
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return printConfigFields()
			}
			key := normalizeConfigKey(args[0])
			value := ""
			if len(args) == 2 {
				value = args[1]
			} else {
				if !stdinIsTerminal() {
					return vlm.NewError(vlm.ConfigError, translate("config.value_required", key), false, nil)
				}
				fmt.Fprintf(os.Stdout, "%s: ", configFieldLabel(key))
				line, err := bufio.NewReader(os.Stdin).ReadString('\n')
				if err != nil {
					return vlm.NewError(vlm.ConfigError, translate("config.value_required", key), false, err)
				}
				value = strings.TrimSpace(line)
			}
			return setConfigValue(cmd, key, value)
		},
	}
}

func printConfigFields() error {
	cfg, err := loadConfig(config.FlagConfig{}, false)
	if err != nil {
		return err
	}
	writeText("%s", translate("config.editable"))
	for _, field := range editableConfigFields(cfg) {
		writeText("  %-20s %-18s %s", field.Key, valueOrNotConfigured(field.Value), field.Description)
	}
	writeText(translate("config.set_examples"))
	return nil
}

func editableConfigFields(cfg config.AppConfig) []configField {
	return []configField{
		{Key: "language", Label: translate("config.language"), Description: translate("config.description.language"), Value: cfg.Language},
		{Key: "provider", Label: translate("config.provider"), Description: translate("config.description.provider"), Value: cfg.Provider},
		{Key: "base_url", Label: translate("config.endpoint"), Description: translate("config.description.base_url"), Value: cfg.BaseURL},
		{Key: "model", Label: translate("config.model"), Description: translate("config.description.model"), Value: cfg.Model},
		{Key: "credential_store", Label: translate("config.credential_store"), Description: translate("config.description.credential_store"), Value: cfg.CredentialStore},
		{Key: "api_key_env", Label: translate("config.api_key_env"), Description: translate("config.description.api_key_env"), Value: cfg.APIKeyEnv},
		{Key: "ppt_renderer", Label: translate("config.renderer"), Description: translate("config.description.ppt_renderer"), Value: cfg.PPTRenderer},
		{Key: "remote_confirmed", Label: translate("config.remote_confirmed"), Description: translate("config.description.remote_confirmed"), Value: strconv.FormatBool(cfg.RemoteConfirmed)},
		{Key: "workers", Label: translate("config.workers"), Description: translate("config.description.workers"), Value: strconv.Itoa(cfg.Workers)},
		{Key: "file_workers", Label: translate("config.file_workers"), Description: translate("config.description.file_workers"), Value: strconv.Itoa(cfg.FileWorkers)},
		{Key: "max_tokens", Label: translate("config.max_tokens"), Description: translate("config.description.max_tokens"), Value: strconv.Itoa(cfg.MaxTokens)},
		{Key: "timeout_seconds", Label: translate("config.timeout"), Description: translate("config.description.timeout"), Value: strconv.Itoa(cfg.TimeoutSeconds)},
	}
}

func setConfigValue(cmd *cobra.Command, key string, value string) error {
	key = normalizeConfigKey(key)
	value = strings.TrimSpace(value)
	if err := validateConfigValue(key, value); err != nil {
		return err
	}
	path, err := config.SetUserValue(globals.ConfigPath, key, value)
	if err != nil {
		return vlm.NewError(vlm.ConfigError, translate("config.update_failed"), false, err)
	}
	localizer := messages
	if key == "language" {
		localizer = i18n.New(value)
	}
	fmt.Fprintln(cmd.OutOrStdout(), localizer.T("config.wrote", key, value))
	writeText("%s: %s", translate("config.path"), path)
	return nil
}

func validateConfigValue(key string, value string) error {
	if value == "" {
		return vlm.NewError(vlm.ConfigError, translate("config.empty_value", key), false, nil)
	}
	switch key {
	case "language", "lang":
		language := i18n.Normalize(value)
		if language == i18n.LanguageEnglish && !strings.HasPrefix(strings.ToLower(value), "en") {
			return vlm.NewError(vlm.ConfigError, translate("config.language_invalid"), false, nil)
		}
	case "provider", "base_url", "model", "credential_store", "credential_account", "api_key_env", "ppt_renderer":
	case "remote_confirmed":
		if value != "true" && value != "false" {
			return vlm.NewError(vlm.ConfigError, translate("config.boolean_invalid", key), false, nil)
		}
	case "workers", "file_workers", "max_tokens", "timeout_seconds":
		number, err := strconv.Atoi(value)
		if err != nil || number <= 0 {
			return vlm.NewError(vlm.ConfigError, translate("config.positive_integer", key), false, err)
		}
	default:
		return vlm.NewError(vlm.ConfigError, translate("config.unknown_key", key), false, nil)
	}
	return nil
}

func normalizeConfigKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(key, "-", "_")))
	if key == "lang" {
		return "language"
	}
	return key
}

func configFieldLabel(key string) string {
	for _, field := range editableConfigFields(config.Default()) {
		if field.Key == key {
			return fmt.Sprintf("%s (%s)", field.Label, field.Key)
		}
	}
	return key
}

func newConfigResetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: translate("config.reset.short"),
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			path, err := config.RemoveUserConfig(globals.ConfigPath)
			if errors.Is(err, os.ErrNotExist) {
				writeText("%s", translate("config.not_found", path))
				return nil
			}
			if err != nil {
				return vlm.NewError(vlm.ConfigError, translate("config.remove_failed"), false, err)
			}
			writeText("%s", translate("config.removed", path))
			return nil
		},
	}
}

func endpointDisplay(value string) string {
	redacted := vlm.RedactedEndpoint(value)
	if redacted == "" {
		return translate("config.not_configured")
	}
	return translate("config."+endpointType(value)) + " · " + redacted
}

func endpointType(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return "remote"
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "local"
	}
	return "remote"
}

func credentialDisplay(source string) string {
	switch {
	case strings.HasPrefix(source, "env:"):
		return translate("config.key_env", strings.TrimPrefix(source, "env:"))
	case strings.HasPrefix(source, "keychain:"):
		return translate("config.key_keychain", strings.TrimPrefix(source, "keychain:"))
	case strings.HasPrefix(source, "file:"):
		return translate("config.key_file", strings.TrimPrefix(source, "file:"))
	default:
		return translate("config.no_key")
	}
}

func valueOrNotConfigured(value string) string {
	if strings.TrimSpace(value) == "" {
		return translate("config.not_configured")
	}
	return value
}
