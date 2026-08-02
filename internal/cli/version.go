package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	buildVersion = "dev"
	buildCommit  = "unknown"
	buildDate    = "unknown"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: translate("version.short"),
		Run: func(cmd *cobra.Command, args []string) {
			writeText("doc7 %s", buildVersion)
			writeText("commit: %s", buildCommit)
			writeText("built: %s", buildDate)
			writeText("platform: %s/%s", runtime.GOOS, runtime.GOARCH)
			writeText("go: %s", runtime.Version())
		},
	}
}

func versionString() string {
	if buildCommit == "unknown" && buildDate == "unknown" {
		return fmt.Sprintf("doc7 %s", buildVersion)
	}
	return fmt.Sprintf("doc7 %s (%s, %s)", buildVersion, buildCommit, buildDate)
}
