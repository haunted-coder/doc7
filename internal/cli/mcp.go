package cli

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/mcpserver"
	"github.com/spf13/cobra"
)

type mcpFlags struct {
	OutputRoot    string
	Retention     time.Duration
	TextGrounding bool
}

func newMCPCommand() *cobra.Command {
	flags := mcpFlags{Retention: mcpserver.DefaultRetention()}
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: translate("mcp.short"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(config.FlagConfig{}, false)
			if err != nil {
				return err
			}
			service, err := mcpserver.New(mcpserver.Config{
				OutputRoot:     flags.OutputRoot,
				Retention:      flags.Retention,
				ServiceVersion: buildVersion,
				ReadOptions:    readOptionsFromConfig(cfg, flags.TextGrounding),
			})
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return service.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&flags.OutputRoot, "output-root", "", "persistent output root for MCP conversions")
	cmd.Flags().DurationVar(&flags.Retention, "retention", flags.Retention, "retention period for automatically created output directories")
	cmd.Flags().BoolVar(&flags.TextGrounding, "text-grounding", false, "use embedded PDF text as secondary visual evidence")
	return cmd
}
