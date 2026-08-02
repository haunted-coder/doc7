package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/credentials"
	"github.com/magicrew/doc7/internal/vlm"
)

const (
	defaultFileResults  = 24
	maximumFileResults  = 100
	defaultFindDepth    = 4
	maximumFindDepth    = 8
	defaultPreviewLines = 20
	maximumPreviewLines = 80
	maximumPreviewBytes = 32 * 1024
	maximumCountBytes   = 64 * 1024 * 1024
)

var fileChatTools = []vlm.AgentTool{
	{
		Name:        "authorize_directory",
		Description: "Ask the user to enter a local directory and authorize it for read-only tools during this chat session.",
		Parameters:  []byte(`{"type":"object","properties":{},"additionalProperties":false}`),
	},
	{
		Name:        "run_readonly_command",
		Description: "Run one portable read-only filesystem command inside authorized directories. This is not a shell and does not support pipes, redirects, scripts, networking, or writes.",
		Parameters: []byte(`{
  "type": "object",
  "properties": {
    "command": {"type": "string", "enum": ["pwd", "ls", "find", "file", "stat", "wc", "realpath", "head", "tail"]},
    "path": {"type": "string", "description": "Path inside an authorized directory"},
    "document_id": {"type": "string", "description": "Document ID returned by an earlier read-only command"},
    "pattern": {"type": "string", "description": "Filename glob or case-insensitive filename fragment used by find"},
    "max_depth": {"type": "integer", "minimum": 1, "maximum": 8},
    "max_results": {"type": "integer", "minimum": 1, "maximum": 100},
    "lines": {"type": "integer", "minimum": 1, "maximum": 80}
  },
  "required": ["command"],
  "additionalProperties": false
}`),
	},
}

type chatDocumentReference struct {
	ID        string
	Path      string
	Confirmed bool
}

type readonlyCommandArguments struct {
	Command    string `json:"command"`
	Path       string `json:"path"`
	DocumentID string `json:"document_id"`
	Pattern    string `json:"pattern"`
	MaxDepth   int    `json:"max_depth"`
	MaxResults int    `json:"max_results"`
	Lines      int    `json:"lines"`
}

type chatFileEntry struct {
	DocumentID string `json:"document_id,omitempty"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
}

func isFileTool(name string) bool {
	return name == "authorize_directory" || name == "run_readonly_command"
}

func (a *chatAgent) executeFileTool(call vlm.AgentToolCall) chatToolExecution {
	switch call.Function.Name {
	case "authorize_directory":
		return a.executeAuthorizeDirectory()
	case "run_readonly_command":
		return a.executeReadonlyCommand(call.Function.Arguments)
	default:
		return chatToolExecution{result: encodeChatToolResult(false, "unsupported file tool"), status: chatToolContinue}
	}
}

func defaultChatRoots() []string {
	result := make([]string, 0, 4)
	if cwd, err := os.Getwd(); err == nil {
		result = appendUniqueRoot(result, cwd)
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, name := range []string{"Desktop", "Documents", "Downloads"} {
			candidate := filepath.Join(home, name)
			if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
				result = appendUniqueRoot(result, candidate)
			}
		}
	}
	return result
}

func appendUniqueRoot(roots []string, path string) []string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return roots
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return roots
	}
	resolved = filepath.Clean(resolved)
	for _, root := range roots {
		if root == resolved {
			return roots
		}
	}
	return append(roots, resolved)
}

func (a *chatAgent) executeAuthorizeDirectory() chatToolExecution {
	if !stdinIsTerminal() {
		return chatToolExecution{result: encodeChatToolResult(false, "directory authorization requires an interactive terminal"), status: chatToolContinue}
	}
	fmt.Fprintf(os.Stdout, "%s ", translate("chat.authorize_directory.prompt"))
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "failed to read directory: "+err.Error()), status: chatToolContinue}
	}
	path := strings.TrimSpace(line)
	if path == "" {
		return chatToolExecution{result: encodeChatToolResult(false, "directory authorization was cancelled"), status: chatToolContinue}
	}
	resolved, err := resolveExistingPath(path)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return chatToolExecution{result: encodeChatToolResult(false, "the authorized path must be an existing directory"), status: chatToolContinue}
	}
	if isProtectedChatPath(resolved) {
		return chatToolExecution{result: encodeChatToolResult(false, "doc7 configuration and credential paths cannot be authorized"), status: chatToolContinue}
	}
	a.authorizedRoots = appendUniqueRoot(a.authorizedRoots, resolved)
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":               true,
		"authorized_root":  resolved,
		"authorized_roots": a.authorizedRoots,
	}), status: chatToolContinue}
}

func (a *chatAgent) executeReadonlyCommand(raw string) chatToolExecution {
	var arguments readonlyCommandArguments
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "invalid read-only command arguments: "+err.Error()), status: chatToolContinue}
	}
	arguments.Command = strings.ToLower(strings.TrimSpace(arguments.Command))
	if arguments.Command == "pwd" {
		return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
			"ok":               true,
			"metadata_only":    true,
			"authorized_roots": a.authorizedRoots,
		}), status: chatToolContinue}
	}
	path, err := a.resolveReadonlyPath(arguments.Path, arguments.DocumentID)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	switch arguments.Command {
	case "ls":
		return a.executeList(path, normalizedResultLimit(arguments.MaxResults))
	case "find":
		return a.executeFind(path, arguments.Pattern, normalizedFindDepth(arguments.MaxDepth), normalizedResultLimit(arguments.MaxResults))
	case "file":
		return a.executeFileMetadata(path)
	case "stat":
		return a.executeStat(path)
	case "wc":
		return a.executeWordCount(path)
	case "realpath":
		return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
			"ok":            true,
			"metadata_only": true,
			"path":          path,
		}), status: chatToolContinue}
	case "head", "tail":
		return a.executeContentPreview(arguments, path)
	default:
		return chatToolExecution{result: encodeChatToolResult(false, "unsupported read-only command"), status: chatToolContinue}
	}
}

func (a *chatAgent) resolveReadonlyPath(path string, documentID string) (string, error) {
	if documentID = strings.TrimSpace(documentID); documentID != "" {
		document, ok := a.documents[documentID]
		if !ok {
			return "", errors.New("unknown document ID")
		}
		return document.Path, nil
	}
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	resolved, err := resolveExistingPath(path)
	if err != nil {
		return "", err
	}
	if isProtectedChatPath(resolved) {
		return "", errors.New("doc7 configuration and credential paths are protected")
	}
	for _, root := range a.authorizedRoots {
		if pathWithinRoot(resolved, root) {
			return resolved, nil
		}
	}
	return "", errors.New("path is outside the authorized chat directories")
}

func resolveExistingPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errors.New("failed to resolve the user home directory")
		}
		path = filepath.Join(home, strings.TrimLeft(path[1:], `/\`))
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("failed to resolve path")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", errors.New("path does not exist")
	}
	return filepath.Clean(resolved), nil
}

func pathWithinRoot(path string, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func isProtectedChatPath(path string) bool {
	protected := []string{
		config.EffectivePath(globals.ConfigPath),
		credentials.DefaultCredentialsPath(),
	}
	for _, candidate := range protected {
		resolved, err := filepath.Abs(candidate)
		if err == nil && filepath.Clean(resolved) == filepath.Clean(path) {
			return true
		}
	}
	return false
}

func normalizedResultLimit(value int) int {
	if value <= 0 {
		return defaultFileResults
	}
	if value > maximumFileResults {
		return maximumFileResults
	}
	return value
}

func normalizedFindDepth(value int) int {
	if value <= 0 {
		return defaultFindDepth
	}
	if value > maximumFindDepth {
		return maximumFindDepth
	}
	return value
}

func normalizedPreviewLines(value int) int {
	if value <= 0 {
		return defaultPreviewLines
	}
	if value > maximumPreviewLines {
		return maximumPreviewLines
	}
	return value
}
