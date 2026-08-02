package cli

import (
	"strings"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/vlm"
	"github.com/spf13/cobra"
)

type modelsFlags struct {
	BaseURL           string
	APIKeyEnv         string
	CredentialStore   string
	CredentialAccount string
	CredentialsPath   string
	Timeout           int
}

type listedModel struct {
	ID         string `json:"id"`
	Object     string `json:"object,omitempty"`
	OwnedBy    string `json:"owned_by,omitempty"`
	Configured bool   `json:"configured"`
}

type modelsSummary struct {
	OK              bool          `json:"ok"`
	Command         string        `json:"command"`
	BaseURL         string        `json:"base_url"`
	ConfiguredModel string        `json:"configured_model,omitempty"`
	ConfiguredFound bool          `json:"configured_found"`
	Models          []listedModel `json:"models"`
}

func newModelsCommand() *cobra.Command {
	var flags modelsFlags
	cmd := &cobra.Command{
		Use:   "models",
		Short: translate("models.short"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(config.FlagConfig{
				BaseURL:           flags.BaseURL,
				APIKeyEnv:         flags.APIKeyEnv,
				CredentialStore:   flags.CredentialStore,
				CredentialAccount: flags.CredentialAccount,
				CredentialsPath:   flags.CredentialsPath,
				TimeoutSeconds:    flags.Timeout,
			}, false)
			if err != nil {
				return err
			}
			if strings.TrimSpace(cfg.BaseURL) == "" {
				return vlm.NewError(vlm.ConfigError, "base URL is required; configure it with doc7 setup config --base-url <url>", false, nil)
			}
			models, err := vlm.ListModelsOpenAICompatible(cmd.Context(), readVLMConfig(cfg), nil)
			if err != nil {
				return err
			}
			summary := buildModelsSummary(cfg.BaseURL, cfg.Model, models)
			if cfg.JSONOutput {
				return writeJSON(summary)
			}
			if len(summary.Models) == 0 {
				writeText("no models returned by %s", summary.BaseURL)
				return nil
			}
			for _, model := range summary.Models {
				if model.Configured {
					writeText("%s (configured)", model.ID)
					continue
				}
				writeText("%s", model.ID)
			}
			if summary.ConfiguredModel != "" && !summary.ConfiguredFound {
				writeText("configured model was not returned: %s", summary.ConfiguredModel)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.BaseURL, "base-url", "", "OpenAI-compatible base URL")
	cmd.Flags().StringVar(&flags.APIKeyEnv, "api-key-env", "", "environment variable that stores the API key")
	cmd.Flags().StringVar(&flags.CredentialStore, "credential-store", "", "credential storage: auto, keychain, file, or env")
	cmd.Flags().StringVar(&flags.CredentialAccount, "credential-account", "", "credential account/profile name")
	cmd.Flags().StringVar(&flags.CredentialsPath, "credentials-path", "", "credential file path when credential-store=file")
	cmd.Flags().IntVar(&flags.Timeout, "timeout", 0, "request timeout in seconds")
	return cmd
}

func buildModelsSummary(baseURL string, configuredModel string, models []vlm.ModelInfo) modelsSummary {
	summary := modelsSummary{
		OK:              true,
		Command:         "models",
		BaseURL:         vlm.RedactedEndpoint(baseURL),
		ConfiguredModel: configuredModel,
		Models:          make([]listedModel, 0, len(models)),
	}
	for _, model := range models {
		configured := configuredModel != "" && model.ID == configuredModel
		summary.Models = append(summary.Models, listedModel{ID: model.ID, Object: model.Object, OwnedBy: model.OwnedBy, Configured: configured})
		if configured {
			summary.ConfiguredFound = true
		}
	}
	return summary
}
