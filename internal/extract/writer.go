package extract

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/magicrew/doc7/internal/render"
	"github.com/magicrew/doc7/internal/vlm"
)

const visualNormalizationVersion = "1"

var (
	markdownImagePattern          = regexp.MustCompile(`!\[([^\]\n]*)\]\((?:<[^>\n]*>|[^)\n]*)\)`)
	markdownReferenceImagePattern = regexp.MustCompile(`!\[([^\]\n]*)\]\[[^\]\n]*\]`)
	markdownLinkPattern           = regexp.MustCompile(`\[([^\]\n]+)\]\((?:<[^>\n]*>|[^)\n]*)\)`)
	htmlImagePattern              = regexp.MustCompile(`(?i)<img\b[^>]*>`)
	htmlImageAltPattern           = regexp.MustCompile(`(?i)\balt\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
)

func writePageMarkdown(path string, page render.PageImage, content string) error {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("<a id=\"page-%d\"></a>\n\n", page.Page))
	builder.WriteString(normalizeMarkdownFragment(content))
	builder.WriteString("\n")
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func writeVisualPageMarkdown(path string, page render.PageImage, content string) error {
	return writePageMarkdown(path, page, normalizeVisualMarkdown(content))
}

func VisualNormalizationVersion() string {
	return visualNormalizationVersion
}

func normalizeVisualMarkdown(content string) string {
	lines := strings.Split(normalizeMarkdownFragment(content), "\n")
	fenceMarker := byte(0)
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if marker := markdownFenceMarker(trimmed); marker != 0 {
			if fenceMarker == 0 {
				fenceMarker = marker
			} else if fenceMarker == marker {
				fenceMarker = 0
			}
			continue
		}
		if fenceMarker != 0 {
			continue
		}
		line = replaceMarkdownImages(line, markdownImagePattern)
		line = replaceMarkdownImages(line, markdownReferenceImagePattern)
		line = replaceHTMLImages(line)
		line = markdownLinkPattern.ReplaceAllString(line, "$1")
		lines[index] = line
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func markdownFenceMarker(line string) byte {
	if strings.HasPrefix(line, "```") {
		return '`'
	}
	if strings.HasPrefix(line, "~~~") {
		return '~'
	}
	return 0
}

func replaceMarkdownImages(line string, pattern *regexp.Regexp) string {
	matches := pattern.FindAllStringSubmatchIndex(line, -1)
	if len(matches) == 1 {
		match := matches[0]
		if strings.TrimSpace(line) == strings.TrimSpace(line[match[0]:match[1]]) {
			return visualBlock(capturedText(line, match, 1))
		}
	}
	return pattern.ReplaceAllStringFunc(line, func(value string) string {
		match := pattern.FindStringSubmatch(value)
		if len(match) < 2 {
			return "[Visual]"
		}
		return visualInline(match[1])
	})
}

func replaceHTMLImages(line string) string {
	matches := htmlImagePattern.FindAllStringIndex(line, -1)
	if len(matches) == 1 && strings.TrimSpace(line) == strings.TrimSpace(line[matches[0][0]:matches[0][1]]) {
		return visualBlock(htmlImageAlt(line[matches[0][0]:matches[0][1]]))
	}
	return htmlImagePattern.ReplaceAllStringFunc(line, func(value string) string {
		return visualInline(htmlImageAlt(value))
	})
}

func htmlImageAlt(value string) string {
	match := htmlImageAltPattern.FindStringSubmatch(value)
	if len(match) < 2 {
		return ""
	}
	for _, candidate := range match[1:] {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func capturedText(line string, match []int, group int) string {
	startIndex := group * 2
	if startIndex+1 >= len(match) || match[startIndex] < 0 || match[startIndex+1] < 0 {
		return ""
	}
	return line[match[startIndex]:match[startIndex+1]]
}

func visualBlock(alt string) string {
	alt = strings.TrimSpace(alt)
	if alt == "" {
		return "> [Visual]"
	}
	if strings.HasPrefix(strings.ToLower(alt), "[visual]") {
		return "> " + alt
	}
	return "> [Visual] " + alt
}

func visualInline(alt string) string {
	alt = strings.TrimSpace(alt)
	if alt == "" {
		return "[Visual]"
	}
	return "[Visual: " + alt + "]"
}

func normalizeMarkdownFragment(content string) string {
	trimmed := strings.TrimSpace(content)
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 2 || !isMarkdownWrapper(strings.TrimSpace(lines[0])) || strings.TrimSpace(lines[len(lines)-1]) != "```" {
		return trimmed
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}

func isMarkdownWrapper(line string) bool {
	return strings.EqualFold(line, "```markdown") || strings.EqualFold(line, "```md")
}

func pageError(err error) *PageError {
	return PageErrorFromError(err)
}

func PageErrorFromError(err error) *PageError {
	if err == nil {
		return nil
	}
	var appErr *vlm.AppError
	if errors.As(err, &appErr) {
		return &PageError{Kind: string(appErr.Kind), Message: appErr.Message}
	}
	return &PageError{Kind: "Error", Message: err.Error()}
}
