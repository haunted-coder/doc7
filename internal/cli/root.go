package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/i18n"
	"github.com/magicrew/doc7/internal/vlm"
	"github.com/spf13/cobra"
)

type globalFlags struct {
	ConfigPath string
	Language   string
	JSON       bool
	NoColor    bool
	Quiet      bool
	Verbose    bool
	Yes        bool
}

var globals globalFlags
var messages = i18n.New("en")

func Execute() error {
	messages = i18n.New(languagePreference(os.Args[1:]))
	root := newRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, userFacingError(err))
		return err
	}
	return nil
}

func userFacingError(err error) string {
	var appErr *vlm.AppError
	if !errors.As(err, &appErr) {
		return err.Error()
	}
	if appErr.Kind == vlm.AuthError {
		if globals.Verbose {
			return translate("error.remote_unauthorized") + "\n" + err.Error()
		}
		return translate("error.remote_unauthorized")
	}
	return err.Error()
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var appErr *vlm.AppError
	if errors.As(err, &appErr) {
		switch appErr.Kind {
		case vlm.ConfigError:
			return 1
		case vlm.DependencyError:
			return 2
		case vlm.RenderError:
			return 3
		case vlm.PartialError:
			return 5
		default:
			return 4
		}
	}
	return 1
}

func newRootCommand() *cobra.Command {
	rootReadFlags := defaultReadFlags()
	root := &cobra.Command{
		Use:           messages.T("root.usage"),
		Short:         messages.T("root.short"),
		Version:       versionString(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return executeRead(cmd, args[0], &rootReadFlags)
		},
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.PersistentFlags().StringVar(&globals.ConfigPath, "config", "", translate("flag.config"))
	root.PersistentFlags().StringVar(&globals.Language, "lang", "", translate("flag.lang"))
	root.PersistentFlags().BoolVar(&globals.JSON, "json", false, translate("flag.json"))
	root.PersistentFlags().BoolVar(&globals.NoColor, "no-color", false, translate("flag.no_color"))
	root.PersistentFlags().BoolVar(&globals.Quiet, "quiet", false, translate("flag.quiet"))
	root.PersistentFlags().BoolVar(&globals.Verbose, "verbose", false, translate("flag.verbose"))
	root.PersistentFlags().BoolVarP(&globals.Yes, "yes", "y", false, translate("flag.yes"))
	bindRootReadFlags(root, &rootReadFlags)
	root.AddGroup(
		&cobra.Group{ID: "core", Title: messages.T("group.everyday")},
		&cobra.Group{ID: "workflow", Title: messages.T("group.advanced")},
		&cobra.Group{ID: "utility", Title: messages.T("group.utility")},
	)
	root.SetHelpCommandGroupID("utility")
	root.SetCompletionCommandGroupID("utility")
	root.AddCommand(withGroup("core", newReadCommand(), newChatCommand(), newConfigCommand(), newModelsCommand(), newDoctorCommand(), newSetupCommand(), newServeCommand(), newMCPCommand())...)
	root.AddCommand(withGroup("workflow", newBatchCommand(), newExtractCommand(), newConvertCommand(), newRenderCommand(), newMergeCommand(), newRefineCommand(), newExportPPTXCommand())...)
	root.AddCommand(withGroup("utility", newVersionCommand(), newUpdateCommand(), newInternalUpdateCommand())...)
	configureHelp(root)
	configureBranding(root)
	return root
}

func languagePreference(args []string) string {
	configPath := ""
	language := ""
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--lang" && index+1 < len(args):
			language = args[index+1]
			index++
		case strings.HasPrefix(argument, "--lang="):
			language = strings.TrimPrefix(argument, "--lang=")
		case argument == "--config" && index+1 < len(args):
			configPath = args[index+1]
			index++
		case strings.HasPrefix(argument, "--config="):
			configPath = strings.TrimPrefix(argument, "--config=")
		}
	}
	if strings.TrimSpace(language) != "" {
		return language
	}
	if value := strings.TrimSpace(os.Getenv("DOC7_LANG")); value != "" {
		return value
	}
	cfg, err := config.Load(configPath, config.EnvMap())
	if err == nil && strings.TrimSpace(cfg.Language) != "" {
		return cfg.Language
	}
	return "auto"
}

func withGroup(groupID string, commands ...*cobra.Command) []*cobra.Command {
	for _, command := range commands {
		command.GroupID = groupID
	}
	return commands
}

func loadConfig(local config.FlagConfig, requireVLM bool) (config.AppConfig, error) {
	local.Language = globals.Language
	local.ConfigPath = globals.ConfigPath
	local.JSONOutput = globals.JSON
	local.NoColor = globals.NoColor
	local.Quiet = globals.Quiet
	local.Verbose = globals.Verbose
	env := config.EnvMap()
	cfg, err := config.Load(globals.ConfigPath, env)
	if err != nil {
		return cfg, vlm.NewError(vlm.ConfigError, "failed to load config", false, err)
	}
	cfg, err = config.MergeFlags(cfg, local)
	if err != nil {
		return cfg, vlm.NewError(vlm.ConfigError, "failed to merge flags", false, err)
	}
	cfg, err = config.ResolveCredentials(cfg, env)
	if err != nil {
		return cfg, vlm.NewError(vlm.ConfigError, "failed to resolve credentials", false, err)
	}
	if err := config.Validate(cfg, requireVLM); err != nil {
		return cfg, vlm.NewError(vlm.ConfigError, err.Error(), false, err)
	}
	return cfg, nil
}

func translate(key string, args ...interface{}) string {
	return messages.T(key, args...)
}

func writeJSON(value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, string(data))
	return nil
}

func applyChangedNumericFlags(cmd *cobra.Command, flags *config.FlagConfig) {
	if cmd.Flags().Lookup("file-workers") != nil && !cmd.Flags().Changed("file-workers") {
		flags.FileWorkers = 0
	}
	if cmd.Flags().Lookup("workers") == nil || !cmd.Flags().Changed("workers") {
		flags.Workers = 0
	}
	if cmd.Flags().Lookup("dpi") == nil || !cmd.Flags().Changed("dpi") {
		flags.DPI = 0
	}
	if cmd.Flags().Lookup("max-image-mb") == nil || !cmd.Flags().Changed("max-image-mb") {
		flags.MaxImageMB = 0
	}
	if cmd.Flags().Lookup("max-tokens") == nil || !cmd.Flags().Changed("max-tokens") {
		flags.MaxTokens = 0
	}
	if cmd.Flags().Lookup("context-fallbacks") != nil && !cmd.Flags().Changed("context-fallbacks") {
		flags.ContextFallbacks = nil
	}
	if cmd.Flags().Lookup("min-image-dimension") != nil && !cmd.Flags().Changed("min-image-dimension") {
		flags.MinImageDimension = 0
	}
	if cmd.Flags().Lookup("timeout") == nil || !cmd.Flags().Changed("timeout") {
		flags.TimeoutSeconds = 0
	}
	if cmd.Flags().Lookup("retry") == nil || !cmd.Flags().Changed("retry") {
		flags.RetryCount = -1
	}
}

func bindRootReadFlags(cmd *cobra.Command, flags *readFlags) {
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", translate("flag.output"))
	cmd.Flags().StringVar(&flags.BaseURL, "base-url", "", translate("flag.base_url"))
	cmd.Flags().StringVar(&flags.Model, "model", "", translate("flag.model"))
	cmd.Flags().StringVar(&flags.Prompt, "prompt", "", translate("flag.prompt"))
	cmd.Flags().StringVar(&flags.Pages, "pages", "", translate("flag.pages"))
	cmd.Flags().BoolVar(&flags.Stdout, "stdout", false, translate("flag.stdout"))
}

func writeText(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}
