package vlm

import "strings"

const reasoningCloseTag = "</think>"

// StripLeadingReasoningBlocks removes model-internal reasoning emitted before
// the user-visible answer. Tags inside the document body are preserved.
func StripLeadingReasoningBlocks(value string) (string, bool) {
	content := strings.TrimSpace(value)
	removed := false
	for strings.HasPrefix(strings.ToLower(content), "<think>") {
		closeIndex := strings.Index(strings.ToLower(content), reasoningCloseTag)
		if closeIndex < 0 {
			break
		}
		content = strings.TrimSpace(content[closeIndex+len(reasoningCloseTag):])
		removed = true
	}
	for strings.HasPrefix(strings.ToLower(content), reasoningCloseTag) {
		content = strings.TrimSpace(content[len(reasoningCloseTag):])
		removed = true
	}
	return content, removed
}
