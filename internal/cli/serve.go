package cli

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/read"
	docserver "github.com/magicrew/doc7/internal/server"
	"github.com/spf13/cobra"
)

type serveFlags struct {
	Address       string
	DataDir       string
	AuthTokenEnv  string
	JobWorkers    int
	QueueSize     int
	MaxUploadMB   int64
	Retention     time.Duration
	TextGrounding bool
}

func newServeCommand() *cobra.Command {
	flags := serveFlags{
		Address:      docserver.DefaultAddress,
		AuthTokenEnv: "DOC7_SERVER_TOKEN",
		JobWorkers:   1,
		QueueSize:    docserver.DefaultQueueSize(),
		MaxUploadMB:  docserver.DefaultMaxUploadBytes() / bytesPerMiB,
		Retention:    docserver.DefaultRetention(),
	}
	cmd := &cobra.Command{
		Use:   "serve",
		Short: translate("serve.short"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(config.FlagConfig{}, false)
			if err != nil {
				return err
			}
			maxUploadBytes, err := docserver.BytesFromMiB(flags.MaxUploadMB)
			if err != nil {
				return err
			}
			authToken := ""
			if strings.TrimSpace(flags.AuthTokenEnv) != "" {
				authToken = strings.TrimSpace(os.Getenv(flags.AuthTokenEnv))
			}
			service, err := docserver.New(docserver.Config{
				DataDir:        flags.DataDir,
				JobWorkers:     flags.JobWorkers,
				QueueSize:      flags.QueueSize,
				MaxUploadBytes: maxUploadBytes,
				Retention:      flags.Retention,
				AuthToken:      authToken,
				ServiceVersion: versionString(),
				ReadOptions:    readOptionsFromConfig(cfg, flags.TextGrounding),
			})
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			writeText("doc7 server listening on %s", flags.Address)
			return service.Serve(ctx, flags.Address)
		},
	}
	cmd.Flags().StringVar(&flags.Address, "addr", flags.Address, "HTTP listen address")
	cmd.Flags().StringVar(&flags.DataDir, "data-dir", "", "persistent job data directory")
	cmd.Flags().StringVar(&flags.AuthTokenEnv, "auth-token-env", flags.AuthTokenEnv, "environment variable containing the bearer token")
	cmd.Flags().IntVar(&flags.JobWorkers, "job-workers", flags.JobWorkers, "concurrent document jobs")
	cmd.Flags().IntVar(&flags.QueueSize, "queue-size", flags.QueueSize, "maximum queued jobs")
	cmd.Flags().Int64Var(&flags.MaxUploadMB, "max-upload-mb", flags.MaxUploadMB, "maximum uploaded file size in MB")
	cmd.Flags().DurationVar(&flags.Retention, "retention", flags.Retention, "retention period for completed jobs")
	cmd.Flags().BoolVar(&flags.TextGrounding, "text-grounding", false, "use embedded PDF text as secondary visual evidence")
	return cmd
}

func readOptionsFromConfig(cfg config.AppConfig, textGrounding bool) read.Options {
	return read.Options{
		PromptName:           cfg.PromptName,
		Merge:                true,
		FileWorkers:          cfg.FileWorkers,
		Workers:              cfg.Workers,
		RetryCount:           cfg.RetryCount,
		TextGrounding:        textGrounding,
		DPI:                  cfg.DPI,
		KeepImages:           true,
		PresentationRenderer: cfg.PPTRenderer,
		VLMConfig:            readVLMConfig(cfg),
	}
}
