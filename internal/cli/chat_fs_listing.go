package cli

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *chatAgent) executeList(path string, limit int) chatToolExecution {
	entries, err := os.ReadDir(path)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "failed to list directory: "+err.Error()), status: chatToolContinue}
	}
	result := make([]chatFileEntry, 0, min(limit, len(entries)))
	for _, entry := range entries {
		if len(result) >= limit || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		candidate := filepath.Join(path, entry.Name())
		resolved, resolveErr := filepath.EvalSymlinks(candidate)
		if resolveErr != nil || !a.pathAuthorized(resolved) || isProtectedChatPath(resolved) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		result = append(result, a.fileEntry(resolved, info))
	}
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":            true,
		"metadata_only": true,
		"entries":       result,
		"truncated":     len(entries) > len(result),
	}), status: chatToolContinue}
}

func (a *chatAgent) executeFind(root string, pattern string, maxDepth int, limit int) chatToolExecution {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return chatToolExecution{result: encodeChatToolResult(false, "find requires a directory path"), status: chatToolContinue}
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		pattern = "*"
	}
	result := make([]chatFileEntry, 0, limit)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		depth := strings.Count(relative, string(filepath.Separator)) + 1
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") || depth > maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if len(result) >= limit {
			return io.EOF
		}
		if strings.HasPrefix(entry.Name(), ".") || !matchChatFilename(entry.Name(), pattern) {
			return nil
		}
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if resolveErr != nil || !a.pathAuthorized(resolved) || isProtectedChatPath(resolved) {
			return nil
		}
		fileInfo, infoErr := entry.Info()
		if infoErr == nil {
			result = append(result, a.fileEntry(resolved, fileInfo))
		}
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return chatToolExecution{result: encodeChatToolResult(false, "failed to search directory: "+err.Error()), status: chatToolContinue}
	}
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":            true,
		"metadata_only": true,
		"entries":       result,
		"truncated":     len(result) >= limit,
	}), status: chatToolContinue}
}

func matchChatFilename(name string, pattern string) bool {
	name = strings.ToLower(name)
	pattern = strings.ToLower(pattern)
	if strings.ContainsAny(pattern, "*?[") {
		matched, err := filepath.Match(pattern, name)
		return err == nil && matched
	}
	return strings.Contains(name, pattern)
}

func (a *chatAgent) pathAuthorized(path string) bool {
	for _, root := range a.authorizedRoots {
		if pathWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func (a *chatAgent) fileEntry(path string, info os.FileInfo) chatFileEntry {
	kind := "file"
	if info.IsDir() {
		kind = "directory"
	} else if info.Mode()&os.ModeSymlink != 0 {
		kind = "symlink"
	}
	return chatFileEntry{
		DocumentID: a.registerDocument(path),
		Name:       filepath.Base(path),
		Path:       path,
		Kind:       kind,
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
	}
}

func (a *chatAgent) registerDocument(path string) string {
	for id, document := range a.documents {
		if document.Path == path {
			return id
		}
	}
	id := newChatID()
	a.documents[id] = &chatDocumentReference{ID: id, Path: path}
	return id
}

func (a *chatAgent) executeStat(path string) chatToolExecution {
	info, err := os.Stat(path)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "failed to stat path: "+err.Error()), status: chatToolContinue}
	}
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":            true,
		"metadata_only": true,
		"entry":         a.fileEntry(path, info),
	}), status: chatToolContinue}
}

func (a *chatAgent) executeFileMetadata(path string) chatToolExecution {
	info, err := os.Stat(path)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "failed to inspect file: "+err.Error()), status: chatToolContinue}
	}
	mimeType := "inode/directory"
	if !info.IsDir() {
		file, openErr := os.Open(path)
		if openErr != nil {
			return chatToolExecution{result: encodeChatToolResult(false, "failed to inspect file: "+openErr.Error()), status: chatToolContinue}
		}
		defer file.Close()
		buffer := make([]byte, 512)
		count, _ := file.Read(buffer)
		mimeType = http.DetectContentType(buffer[:count])
	}
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":            true,
		"metadata_only": true,
		"entry":         a.fileEntry(path, info),
		"mime_type":     mimeType,
	}), status: chatToolContinue}
}

func (a *chatAgent) executeWordCount(path string) chatToolExecution {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return chatToolExecution{result: encodeChatToolResult(false, "wc requires a regular file"), status: chatToolContinue}
	}
	if info.Size() > maximumCountBytes {
		return chatToolExecution{result: encodeChatToolResult(false, "file is too large for wc"), status: chatToolContinue}
	}
	file, err := os.Open(path)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "failed to count file: "+err.Error()), status: chatToolContinue}
	}
	defer file.Close()
	reader := bufio.NewScanner(file)
	reader.Buffer(make([]byte, 64*1024), 1024*1024)
	lines := 0
	words := 0
	for reader.Scan() {
		lines++
		words += len(bytes.Fields(reader.Bytes()))
	}
	if err := reader.Err(); err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "failed to count file: "+err.Error()), status: chatToolContinue}
	}
	return chatToolExecution{result: encodeChatJSON(map[string]interface{}{
		"ok":            true,
		"metadata_only": true,
		"path":          path,
		"bytes":         info.Size(),
		"lines":         lines,
		"words":         words,
	}), status: chatToolContinue}
}
