package notebookinput

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/emailinput"
)

const (
	maxNotebookBytes = int64(64 * 1024 * 1024)
	maxNotebookCells = 10000
	maxCellOutputs   = 100000
)

type Notebook struct {
	RootDir  string
	HTMLPath string
	cleanup  func()
}

type rawNotebook struct {
	Cells         []rawCell                  `json:"cells"`
	Metadata      map[string]json.RawMessage `json:"metadata"`
	NBFormat      int                        `json:"nbformat"`
	NBFormatMinor int                        `json:"nbformat_minor"`
}

type rawCell struct {
	Attachments    map[string]map[string]json.RawMessage `json:"attachments"`
	CellType       string                                `json:"cell_type"`
	ExecutionCount json.RawMessage                       `json:"execution_count"`
	Metadata       map[string]json.RawMessage            `json:"metadata"`
	Outputs        []rawOutput                           `json:"outputs"`
	Source         json.RawMessage                       `json:"source"`
}

type rawOutput struct {
	Data           map[string]json.RawMessage `json:"data"`
	EName          string                     `json:"ename"`
	EValue         string                     `json:"evalue"`
	ExecutionCount json.RawMessage            `json:"execution_count"`
	Name           string                     `json:"name"`
	OutputType     string                     `json:"output_type"`
	Text           json.RawMessage            `json:"text"`
	Traceback      []string                   `json:"traceback"`
}

func Open(path string) (Notebook, error) {
	data, err := readNotebook(path)
	if err != nil {
		return Notebook{}, err
	}
	var parsed rawNotebook
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Notebook{}, fmt.Errorf("failed to parse Jupyter Notebook JSON: %w", err)
	}
	if parsed.NBFormat <= 0 || parsed.Cells == nil {
		return Notebook{}, fmt.Errorf("invalid Jupyter Notebook structure")
	}
	if len(parsed.Cells) > maxNotebookCells {
		return Notebook{}, fmt.Errorf("notebook cell count exceeds %d", maxNotebookCells)
	}
	outputs := 0
	for _, cell := range parsed.Cells {
		outputs += len(cell.Outputs)
		if outputs > maxCellOutputs {
			return Notebook{}, fmt.Errorf("notebook output count exceeds %d", maxCellOutputs)
		}
	}

	temporary, err := os.MkdirTemp("", "doc7-ipynb-*")
	if err != nil {
		return Notebook{}, err
	}
	cleanup := func() { _ = os.RemoveAll(temporary) }
	builder := documentBuilder{root: temporary, cidAssets: map[string]string{}}
	body, err := builder.build(parsed.Cells)
	if err != nil {
		cleanup()
		return Notebook{}, err
	}
	title := notebookTitle(parsed)
	document := emailinput.BuildDocument(emailinput.Document{
		Subject:    title,
		Headers:    notebookHeaders(parsed),
		HTMLBodies: []string{body},
		CIDAssets:  builder.cidAssets,
	})
	htmlPath := filepath.Join(temporary, "notebook.html")
	if err := os.WriteFile(htmlPath, []byte(document), 0o644); err != nil {
		cleanup()
		return Notebook{}, err
	}
	return Notebook{RootDir: temporary, HTMLPath: htmlPath, cleanup: cleanup}, nil
}

func (notebook *Notebook) Close() {
	if notebook.cleanup != nil {
		notebook.cleanup()
		notebook.cleanup = nil
	}
}

func readNotebook(path string) ([]byte, error) {
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
		return nil, fmt.Errorf("IPYNB input is not a regular file: %s", path)
	}
	if info.Size() > maxNotebookBytes {
		return nil, fmt.Errorf("IPYNB input exceeds %d bytes", maxNotebookBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxNotebookBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxNotebookBytes {
		return nil, fmt.Errorf("IPYNB input exceeds %d bytes", maxNotebookBytes)
	}
	return data, nil
}

func notebookTitle(notebook rawNotebook) string {
	if value := metadataString(notebook.Metadata, "title"); value != "" {
		return value
	}
	for _, cell := range notebook.Cells {
		if cell.CellType != "markdown" {
			continue
		}
		source, err := textValue(cell.Source)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(source, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "# ") {
				return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "# "))
			}
		}
	}
	return "Jupyter Notebook"
}

func notebookHeaders(notebook rawNotebook) []emailinput.HeaderField {
	fields := []emailinput.HeaderField{{Name: "Format", Value: fmt.Sprintf("Jupyter Notebook %d.%d", notebook.NBFormat, notebook.NBFormatMinor)}}
	if kernel := metadataObject(notebook.Metadata, "kernelspec"); kernel != nil {
		if value := metadataString(kernel, "display_name"); value != "" {
			fields = append(fields, emailinput.HeaderField{Name: "Kernel", Value: value})
		} else if value := metadataString(kernel, "name"); value != "" {
			fields = append(fields, emailinput.HeaderField{Name: "Kernel", Value: value})
		}
	}
	if language := metadataObject(notebook.Metadata, "language_info"); language != nil {
		value := metadataString(language, "name")
		if version := metadataString(language, "version"); version != "" {
			value = strings.TrimSpace(value + " " + version)
		}
		if value != "" {
			fields = append(fields, emailinput.HeaderField{Name: "Language", Value: value})
		}
	}
	return fields
}

func metadataObject(values map[string]json.RawMessage, key string) map[string]json.RawMessage {
	raw, exists := values[key]
	if !exists {
		return nil
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(raw, &result) != nil {
		return nil
	}
	return result
}

func metadataString(values map[string]json.RawMessage, key string) string {
	raw, exists := values[key]
	if !exists {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func textValue(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	var single string
	if err := json.Unmarshal(trimmed, &single); err == nil {
		return single, nil
	}
	var lines []string
	if err := json.Unmarshal(trimmed, &lines); err == nil {
		return strings.Join(lines, ""), nil
	}
	return "", fmt.Errorf("notebook text value must be a string or string array")
}
