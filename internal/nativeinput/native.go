package nativeinput

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/magicrew/doc7/internal/detect"
)

const (
	converterVersion = "1"
	maxInputBytes    = int64(64 * 1024 * 1024)
)

type Result struct {
	Format   string
	Markdown string
}

func Version() string {
	return converterVersion
}

func Convert(path string, kind detect.Kind) (Result, error) {
	data, err := readInput(path)
	if err != nil {
		return Result{}, err
	}
	switch kind {
	case detect.KindMarkdown:
		return Result{Format: "markdown", Markdown: ensureTrailingNewline(normalizeLineEndings(string(data)))}, nil
	case detect.KindText:
		return Result{Format: "text", Markdown: ensureTrailingNewline(normalizeLineEndings(string(data)))}, nil
	case detect.KindCSV:
		markdown, err := delimitedToMarkdown(data, ',')
		return Result{Format: "csv", Markdown: markdown}, err
	case detect.KindTSV:
		markdown, err := delimitedToMarkdown(data, '\t')
		return Result{Format: "tsv", Markdown: markdown}, err
	case detect.KindJSON:
		markdown, err := jsonToMarkdown(data)
		return Result{Format: "json", Markdown: markdown}, err
	case detect.KindXML:
		return Result{Format: "xml", Markdown: fencedBlock("xml", string(data))}, nil
	case detect.KindYAML:
		return Result{Format: "yaml", Markdown: fencedBlock("yaml", string(data))}, nil
	default:
		return Result{}, fmt.Errorf("unsupported native input type: %s", kind)
	}
}

func readInput(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("native input is not a regular file: %s", path)
	}
	if info.Size() > maxInputBytes {
		return nil, fmt.Errorf("native input exceeds %d bytes", maxInputBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxInputBytes {
		return nil, fmt.Errorf("native input exceeds %d bytes", maxInputBytes)
	}
	return data, nil
}

func delimitedToMarkdown(data []byte, delimiter rune) (string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("failed to parse delimited text: %w", err)
	}
	if len(rows) == 0 {
		return "", nil
	}
	columns := 0
	for _, row := range rows {
		if len(row) > columns {
			columns = len(row)
		}
	}
	if columns == 0 {
		return "", nil
	}
	header := normalizedRow(rows[0], columns)
	for index := range header {
		if strings.TrimSpace(header[index]) == "" {
			header[index] = fmt.Sprintf("Column %d", index+1)
		}
	}
	var builder strings.Builder
	writeTableRow(&builder, header)
	separator := make([]string, columns)
	for index := range separator {
		separator[index] = "---"
	}
	writeTableRow(&builder, separator)
	for _, row := range rows[1:] {
		writeTableRow(&builder, normalizedRow(row, columns))
	}
	return builder.String(), nil
}

func normalizedRow(row []string, columns int) []string {
	result := make([]string, columns)
	for index := 0; index < columns && index < len(row); index++ {
		result[index] = markdownTableCell(row[index])
	}
	return result
}

func writeTableRow(builder *strings.Builder, row []string) {
	builder.WriteString("| ")
	for index, value := range row {
		if index > 0 {
			builder.WriteString(" | ")
		}
		builder.WriteString(value)
	}
	builder.WriteString(" |\n")
}

func markdownTableCell(value string) string {
	value = normalizeLineEndings(value)
	value = strings.ReplaceAll(value, "|", `\|`)
	return strings.ReplaceAll(value, "\n", "<br>")
}

func jsonToMarkdown(data []byte) (string, error) {
	trimmed := bytes.TrimSpace(data)
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, trimmed, "", "  "); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}
	return fencedBlock("json", formatted.String()), nil
}

func fencedBlock(language string, content string) string {
	content = strings.TrimRight(normalizeLineEndings(content), "\n")
	fence := "```"
	for strings.Contains(content, fence) {
		fence += "`"
	}
	return fence + language + "\n" + content + "\n" + fence + "\n"
}

func normalizeLineEndings(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\r", "\n")
}

func ensureTrailingNewline(value string) string {
	if value == "" || strings.HasSuffix(value, "\n") {
		return value
	}
	return value + "\n"
}
