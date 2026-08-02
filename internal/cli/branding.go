package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	ansiBlue  = "\x1b[38;2;35;68;220m"
	ansiRed   = "\x1b[38;2;255;72;59m"
	ansiGreen = "\x1b[38;2;21;160;67m"
)

var fullLogo = []string{
	"██████╗  ██████╗  ██████╗███████╗",
	"██╔══██╗██╔═══██╗██╔════╝╚════██║",
	"██║  ██║██║   ██║██║         ██╔╝",
	"██║  ██║██║   ██║██║        ██╔╝ ",
	"██████╔╝╚██████╔╝╚██████╗   ██║  ",
	"╚═════╝  ╚═════╝  ╚═════╝   ╚═╝  ",
}

func configureBranding(root *cobra.Command) {
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		writeFullBranding(cmd)
		defaultHelp(cmd, args)
	})
	root.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		writeCompactBranding(cmd)
	}
}

func writeFullBranding(cmd *cobra.Command) {
	if !brandingEnabled(cmd) {
		return
	}
	w := brandingWriter()
	for _, line := range fullLogo {
		runes := []rune(line)
		cut := len(runes) - 8
		fmt.Fprintf(w, "%s%s%s%s%s%s\n", ansiBold, ansiBlue, string(runes[:cut]), ansiRed, string(runes[cut:]), ansiReset)
	}
	fmt.Fprintf(w, "\n%sdoc7 %s%s  %s%s%s\n", ansiGreen, buildVersion, ansiReset, ansiDim, translate("brand.compact"), ansiReset)
	fmt.Fprintf(w, "%s%s%s\n\n", ansiGreen, translate("brand.tagline"), ansiReset)
}

func writeCompactBranding(cmd *cobra.Command) {
	if !brandingEnabled(cmd) {
		return
	}
	fmt.Fprintf(
		brandingWriter(),
		"%s%sdoc%s%s7%s  %s%s%s\n",
		ansiBold,
		ansiBlue,
		ansiRed,
		ansiBold,
		ansiReset,
		ansiDim,
		translate("brand.compact"),
		ansiReset,
	)
}

func brandingEnabled(cmd *cobra.Command) bool {
	if cmd == nil || globals.JSON || globals.Quiet || globals.NoColor {
		return false
	}
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" || !stderrIsTerminal() {
		return false
	}
	for current := cmd; current != nil; current = current.Parent() {
		switch current.Name() {
		case "completion", "mcp", "version":
			return false
		}
	}
	return true
}

func stderrIsTerminal() bool {
	fd := os.Stderr.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func brandingWriter() io.Writer {
	return colorable.NewColorable(os.Stderr)
}
