package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func (a *chatAgent) executeContentPreview(arguments readonlyCommandArguments, path string) chatToolExecution {
	lines := normalizedPreviewLines(arguments.Lines)
	if err := validateChatPreview(path); err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	confirmed, err := confirmLocalChatAction(translate("chat.preview.confirm", arguments.Command, path, lines))
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	if !confirmed {
		return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
			"ok":                       true,
			"cancelled":                true,
			"content_visible_to_model": false,
		}), status: chatToolContinue}
	}
	content, err := readChatPreview(path, arguments.Command, lines)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":                       true,
		"content_visible_to_model": true,
		"path":                     path,
		"command":                  arguments.Command,
		"lines":                    lines,
		"content":                  content,
	}), status: chatToolContinue}
}

func confirmLocalChatAction(question string) (bool, error) {
	if !stdinIsTerminal() {
		return false, errors.New("this action requires an interactive terminal")
	}
	writeText("%s", question)
	writeText("  1. %s", translate("chat.local_confirm.confirm"))
	writeText("  2. %s", translate("chat.local_confirm.cancel"))
	fmt.Fprintf(os.Stdout, "%s ", translate("chat.local_confirm.prompt"))
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, errors.New("failed to read local confirmation")
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "1", "y", "yes":
		return true, nil
	case "", "2", "n", "no", "q", "quit", "cancel":
		return false, nil
	default:
		return false, errors.New("invalid confirmation choice")
	}
}

func readChatPreview(path string, command string, lines int) (string, error) {
	if err := validateChatPreview(path); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("failed to open file for preview")
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, maximumCountBytes+1))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	selected := make([]string, 0, lines)
	for scanner.Scan() {
		line := scanner.Text()
		if command == "head" {
			selected = append(selected, line)
			if len(selected) >= lines {
				break
			}
			continue
		}
		if len(selected) < lines {
			selected = append(selected, line)
			continue
		}
		copy(selected, selected[1:])
		selected[len(selected)-1] = line
	}
	if err := scanner.Err(); err != nil {
		return "", errors.New("failed to read text preview")
	}
	content := strings.Join(selected, "\n")
	if len(content) > maximumPreviewBytes {
		content = content[:maximumPreviewBytes]
		for !utf8.ValidString(content) && len(content) > 0 {
			content = content[:len(content)-1]
		}
	}
	return content, nil
}

func validateChatPreview(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("content preview requires a regular file")
	}
	if info.Size() > maximumCountBytes {
		return errors.New("file is too large for content preview")
	}
	file, err := os.Open(path)
	if err != nil {
		return errors.New("failed to open file for preview")
	}
	defer file.Close()
	probe := make([]byte, 512)
	count, _ := file.Read(probe)
	if !isPreviewText(path, probe[:count]) {
		return errors.New("head and tail only support text files")
	}
	return nil
}

func isPreviewText(path string, probe []byte) bool {
	mimeType := http.DetectContentType(probe)
	if strings.HasPrefix(mimeType, "text/") || strings.Contains(mimeType, "json") || strings.Contains(mimeType, "xml") {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".txt", ".csv", ".tsv", ".json", ".yaml", ".yml", ".xml", ".html", ".htm", ".log":
		return utf8.Valid(probe)
	default:
		return false
	}
}
