package extract

import (
	"fmt"
	"os"
	"strings"

	"github.com/magicrew/doc7/internal/detect"
)

const maxPromptFileBytes int64 = 1024 * 1024

func PromptForInput(name string, promptFile string, kind detect.Kind) (string, error) {
	if strings.TrimSpace(promptFile) != "" {
		return promptFromFile(promptFile)
	}
	switch name {
	case "", "auto":
		if detect.IsPresentation(kind) {
			return slidePrompt, nil
		}
		return documentPrompt, nil
	default:
		return Prompt(name)
	}
}

func promptFromFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to access prompt file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("prompt file is not a regular file: %s", path)
	}
	if info.Size() > maxPromptFileBytes {
		return "", fmt.Errorf("prompt file exceeds %d bytes", maxPromptFileBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file: %w", err)
	}
	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", fmt.Errorf("prompt file is empty: %s", path)
	}
	return prompt, nil
}

func Prompt(name string) (string, error) {
	switch name {
	case "document":
		return documentPrompt, nil
	case "slide":
		return slidePrompt, nil
	default:
		return "", fmt.Errorf("unknown prompt: %s", name)
	}
}

func PagePromptHash(basePrompt string, embeddedTextSHA256 string) string {
	if embeddedTextSHA256 == "" {
		return sha256Text(basePrompt)
	}
	return sha256Text("doc7-embedded-text-grounding-" + groundingVersion + "\n" + sha256Text(basePrompt) + "\n" + embeddedTextSHA256)
}

const documentPrompt = `Transcribe the entire document page as a faithful Markdown fragment from top to bottom. First scan the whole page, including small text at the edges and bottom, then write the transcription. Preserve the original language, reading order, hierarchy, and every readable word, number, unit, table row, chart point, formula, label, status, button, footnote, caption, and footer. Use headings, paragraphs, lists, tables, code blocks, and LaTeX as appropriate. Use $$...$$ for standalone displayed formulas and $...$ only for formulas embedded in prose.

Table rules:
- A single visual table must become one continuous Markdown table with one header row and the same column count on every data row.
- Bold, shaded, indented, subtotal, total, and section rows are still data rows; never turn them into a new header or a second table.
- Keep hierarchical row labels in the first column and preserve every value in its original column.
- Do not merge nearby labels into extra columns or invent blank header rows. If formatting is uncertain, preserve the values as ordered plain text rather than silently changing their relationships.

Visual rules:
- Describe every non-text visual only as one or more blockquotes in the form "> [Visual] ..."; never use Markdown image syntax, Mermaid, ASCII art, SVG, diagram code, or invented image links.
- For charts, preserve visible axes, legends, series, values, trends, and conclusions.
- For diagrams, workflows, and multi-part figures, write enough ordered prose that a reader could reconstruct the visible topology without seeing the page.
- State the visual type, title, overall reading direction, spatial arrangement of parts, input order, every readable node or label, and every visible directed connection in sequence.
- Explicitly preserve branches, merges, bypass connections, loops, parallel paths, repeated components and counts, nested or grouped regions, and the final visible destination. Distinguish separate paths instead of collapsing them into a summary.
- Audit each arrow endpoint before writing. For a long bypass arrow that crosses intermediate nodes, state the exact source and destination and name the skipped nodes; never attach an input to the nearest or lowest box merely because the arrow passes beside it.
- Describe only visible structure. Do not infer hidden nodes, implicit outputs, or relationships that are not shown.
- Never replace a visual with only its title or caption. Do not summarize, translate, infer, invent, or omit readable information. Mark unreadable content as "不可读" on Chinese pages or "unreadable" otherwise.

Return only Markdown without metadata, commentary, or enclosing code fences.`

const slidePrompt = `Transcribe the entire presentation slide as a faithful Markdown fragment. Use the visible title as the leading heading. Preserve the original language, reading order, hierarchy, and every readable bullet, label, example, number, unit, table cell, formula, brand, tool, and decision criterion. Use $$...$$ for standalone displayed formulas and $...$ only for formulas embedded in prose.

Visual rules:
- Describe every chart, matrix, funnel, workflow, quadrant, screenshot, or other non-text visual only as one or more blockquotes in the form "> [Visual] ..."; never use Markdown image syntax, Mermaid, ASCII art, SVG, diagram code, or invented image links.
- For charts, preserve visible axes, legends, series, values, trends, and conclusions.
- For diagrams, workflows, and multi-part figures, write enough ordered prose that a reader could reconstruct the visible topology without seeing the slide.
- State the visual type, title, overall reading direction, spatial arrangement of parts, input order, every readable node or label, and every visible directed connection in sequence.
- Explicitly preserve branches, merges, bypass connections, loops, parallel paths, repeated components and counts, nested or grouped regions, comparisons, and the final visible destination. Distinguish separate paths instead of collapsing them into a summary.
- Audit each arrow endpoint before writing. For a long bypass arrow that crosses intermediate nodes, state the exact source and destination and name the skipped nodes; never attach an input to the nearest or lowest box merely because the arrow passes beside it.
- Describe only visible structure. Do not infer hidden nodes, implicit outputs, or relationships that are not shown.
- Never replace a visual with only its title or caption. Do not summarize, translate, infer, invent, or omit readable information. Mark unreadable content as "不可读" on Chinese slides or "unreadable" otherwise.

Return only Markdown without metadata, commentary, or enclosing code fences.`
