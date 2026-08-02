package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/extract"
	"github.com/spf13/cobra"
)

func newMergeCommand() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "merge <path>",
		Short: translate("merge.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pagePaths, err := markdownPages(args[0])
			if err != nil {
				return err
			}
			if output == "" {
				name := strings.TrimSuffix(filepath.Base(args[0]), filepath.Ext(args[0]))
				if name == "" || name == "." {
					name = "merged"
				}
				info, statErr := os.Stat(args[0])
				if statErr == nil && !info.IsDir() {
					output = filepath.Join(filepath.Dir(args[0]), name+"_merged.md")
				} else {
					output = filepath.Join(args[0], name+"_merged.md")
				}
			}
			if err := extract.MergeMarkdown(output, pagePaths); err != nil {
				return err
			}
			summary := map[string]interface{}{"ok": true, "command": "merge", "output": output, "pages_total": len(pagePaths)}
			if globals.JSON {
				return writeJSON(summary)
			}
			writeText("merged %d pages to %s", len(pagePaths), output)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output Markdown path")
	return cmd
}

func markdownPages(path string) ([]string, error) {
	return extract.FindMarkdownPages(path)
}
