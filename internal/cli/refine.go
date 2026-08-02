package cli

import (
	"time"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/refine"
	"github.com/magicrew/doc7/internal/vlm"
	"github.com/spf13/cobra"
)

func newRefineCommand() *cobra.Command {
	var flags struct {
		Output            string
		OutputName        string
		Provider          string
		BaseURL           string
		Model             string
		APIKeyEnv         string
		CredentialStore   string
		CredentialAccount string
		CredentialsPath   string
		Timeout           int
		MaxTokens         int
		Profile           string
		Language          string
		MaxInputChars     int
	}
	cmd := &cobra.Command{
		Use:   "refine <path>",
		Short: translate("refine.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localConfig := config.FlagConfig{
				Provider:          flags.Provider,
				BaseURL:           flags.BaseURL,
				Model:             flags.Model,
				APIKeyEnv:         flags.APIKeyEnv,
				CredentialStore:   flags.CredentialStore,
				CredentialAccount: flags.CredentialAccount,
				CredentialsPath:   flags.CredentialsPath,
				TimeoutSeconds:    flags.Timeout,
				MaxTokens:         flags.MaxTokens,
			}
			applyChangedNumericFlags(cmd, &localConfig)
			cfg, err := loadConfig(localConfig, true)
			if err != nil {
				return err
			}
			summary, err := refine.Run(cmd.Context(), args[0], refine.Options{
				OutputPath:    flags.Output,
				OutputName:    flags.OutputName,
				Profile:       flags.Profile,
				Language:      flags.Language,
				MaxInputChars: flags.MaxInputChars,
				VLMConfig: vlm.Config{
					Provider:  cfg.Provider,
					BaseURL:   cfg.BaseURL,
					Model:     cfg.Model,
					APIKey:    cfg.APIKey,
					MaxTokens: cfg.MaxTokens,
					Timeout:   time.Duration(cfg.TimeoutSeconds) * time.Second,
				},
			})
			if cfg.JSONOutput {
				if jsonErr := writeJSON(summary); jsonErr != nil {
					return jsonErr
				}
			} else if summary.Output != "" {
				writeText("refined %s to %s", summary.Source, summary.Output)
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "output Markdown path")
	cmd.Flags().StringVar(&flags.OutputName, "output-name", "", "output Markdown filename when output is not set")
	cmd.Flags().StringVar(&flags.Provider, "provider", "", "VLM provider")
	cmd.Flags().StringVar(&flags.BaseURL, "base-url", "", "OpenAI-compatible base URL")
	cmd.Flags().StringVar(&flags.Model, "model", "", "VLM model name")
	cmd.Flags().StringVar(&flags.APIKeyEnv, "api-key-env", "", "environment variable that stores the API key")
	cmd.Flags().StringVar(&flags.CredentialStore, "credential-store", "", "credential storage: auto, keychain, file, or env")
	cmd.Flags().StringVar(&flags.CredentialAccount, "credential-account", "", "credential account/profile name")
	cmd.Flags().StringVar(&flags.CredentialsPath, "credentials-path", "", "credential file path when credential-store=file")
	cmd.Flags().IntVar(&flags.Timeout, "timeout", 120, "request timeout in seconds")
	cmd.Flags().IntVar(&flags.MaxTokens, "max-tokens", 0, "maximum model output tokens")
	cmd.Flags().StringVar(&flags.Profile, "profile", "report", "refine profile: report, concise, or knowledge")
	cmd.Flags().StringVar(&flags.Language, "language", "auto", "output language: auto or a specific language name")
	cmd.Flags().IntVar(&flags.MaxInputChars, "max-input-chars", 120000, "maximum input characters sent to the text model")
	return cmd
}
