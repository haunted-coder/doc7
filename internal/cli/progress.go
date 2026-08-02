package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/magicrew/doc7/internal/batch"
	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/extract"
	"github.com/magicrew/doc7/internal/logx"
)

type progressReporter struct {
	mu     sync.Mutex
	logger logx.Logger
}

func newProgressReporter(cfg config.AppConfig) *progressReporter {
	return &progressReporter{
		logger: logx.New(os.Stderr, cfg.Quiet, cfg.Verbose),
	}
}

func (r *progressReporter) reportExtract(event extract.ProgressEvent) {
	switch event.Stage {
	case extract.ProgressRenderStarted:
		r.info("%s", translate("progress.rendering", progressInput(event.Input)))
	case extract.ProgressRenderCompleted:
		if event.SourcePagesTotal > 0 && event.SourcePagesTotal != event.PagesTotal {
			r.info("%s", translate("progress.rendered_selected", event.PagesTotal, event.SourcePagesTotal, progressInput(event.Input), progressDuration(event.Duration)))
		} else {
			r.info("%s", translate("progress.rendered", event.PagesTotal, progressInput(event.Input), progressDuration(event.Duration)))
		}
	case extract.ProgressRenderFailed:
		r.info("%s", translate("progress.render_failed", progressInput(event.Input), progressMessage(event.Message)))
	case extract.ProgressPageCompleted:
		status := translate("progress.done")
		if event.Cached {
			status = translate("progress.cached")
		} else if event.Status == extract.StatusError {
			status = translate("progress.failed")
		}
		pageTotal := event.PagesTotal
		if event.SourcePagesTotal > 0 {
			pageTotal = event.SourcePagesTotal
		}
		completion := translate("progress.complete_count", event.PagesCompleted)
		if event.PagesTotal > 0 && event.PagesTotal != pageTotal {
			completion = translate("progress.selected_complete_count", event.PagesCompleted, event.PagesTotal)
		}
		line := translate("progress.page", progressInput(event.Input), event.Page, pageTotal, completion, status)
		if event.Duration > 0 && !event.Cached {
			line += translate("progress.in", progressDuration(event.Duration))
		}
		if message := pageProgressMessage(event); message != "" {
			line += ": " + message
		}
		r.info("%s", line)
	}
}

func (r *progressReporter) reportBatch(event batch.ProgressEvent) {
	switch event.Stage {
	case batch.ProgressFileStarted:
		r.info("%s", translate("progress.file_processing", event.Index, event.Total, progressInput(event.Input)))
	case batch.ProgressFileCompleted:
		status := localizedProgressStatus(event.Status)
		line := translate("progress.file_result", event.Index, event.Total, progressInput(event.Input), status, event.PagesSuccess, event.PagesTotal)
		if event.PagesCached > 0 {
			line += translate("progress.cached_count", event.PagesCached)
		}
		if event.SourcePagesTotal > event.PagesTotal {
			line += translate("progress.source_pages", event.SourcePagesTotal)
		}
		line += progressClosingParenthesis()
		if event.Duration > 0 {
			line += translate("progress.in", progressDuration(event.Duration))
		}
		if message := progressMessage(event.Message); message != "" {
			line += ": " + message
		}
		r.info("%s", line)
	}
}

func localizedProgressStatus(status string) string {
	switch status {
	case "", "done", "success":
		return translate("progress.done")
	case "cached":
		return translate("progress.cached")
	case "failed", "error":
		return translate("progress.failed")
	default:
		return status
	}
}

func progressClosingParenthesis() string {
	if messages.Chinese() {
		return "）"
	}
	return ")"
}

func (r *progressReporter) info(format string, args ...interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logger.Infof(format, args...)
}

func pageProgressMessage(event extract.ProgressEvent) string {
	if event.Error != nil {
		return progressMessage(event.Error.Message)
	}
	return progressMessage(event.Message)
}

func progressInput(input string) string {
	cleaned := filepath.Clean(strings.TrimSpace(input))
	if cleaned == "." || cleaned == string(filepath.Separator) || cleaned == "" {
		return input
	}
	return filepath.Base(cleaned)
}

func progressMessage(message string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
}

func progressDuration(duration time.Duration) string {
	if duration < time.Second {
		return fmt.Sprintf("%dms", duration.Milliseconds())
	}
	return duration.Round(100 * time.Millisecond).String()
}
