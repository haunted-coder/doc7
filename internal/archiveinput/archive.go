package archiveinput

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Options struct {
	MaxFiles int
	MaxBytes int64
}

type Result struct {
	SourcePath  string `json:"source_path"`
	OutputDir   string `json:"output_dir"`
	ContentRoot string `json:"content_root"`
	Files       int    `json:"files"`
	Bytes       int64  `json:"bytes"`
}

func DefaultOptions() Options {
	return Options{MaxFiles: 10000, MaxBytes: 2 * 1024 * 1024 * 1024}
}

func ExtractZIP(inputPath string, outputDir string, options Options) (Result, error) {
	defaults := DefaultOptions()
	if options.MaxFiles <= 0 {
		options.MaxFiles = defaults.MaxFiles
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = defaults.MaxBytes
	}
	reader, err := zip.OpenReader(inputPath)
	if err != nil {
		return Result{}, err
	}
	defer reader.Close()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Result{}, err
	}

	result := Result{SourcePath: inputPath, OutputDir: outputDir}
	seen := map[string]struct{}{}
	for _, entry := range reader.File {
		if entry.Name == "" {
			continue
		}
		entryKey := strings.ToLower(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(entry.Name, "\\", "/"))))
		if _, ok := seen[entryKey]; ok {
			return Result{}, fmt.Errorf("archive contains duplicate entry: %s", entry.Name)
		}
		seen[entryKey] = struct{}{}
		if entry.FileInfo().IsDir() {
			continue
		}
		if result.Files >= options.MaxFiles {
			return Result{}, fmt.Errorf("archive exceeds maximum file count %d", options.MaxFiles)
		}
		if entry.UncompressedSize64 > uint64(options.MaxBytes-result.Bytes) {
			return Result{}, fmt.Errorf("archive exceeds maximum expanded size %d bytes", options.MaxBytes)
		}
		destination, err := safeDestination(outputDir, entry.Name)
		if err != nil {
			return Result{}, err
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return Result{}, fmt.Errorf("archive contains unsupported symlink: %s", entry.Name)
		}
		written, err := extractFile(entry, destination, options.MaxBytes-result.Bytes)
		if err != nil {
			return Result{}, err
		}
		result.Files++
		result.Bytes += written
	}
	result.ContentRoot, err = contentRoot(outputDir)
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func extractFile(entry *zip.File, destination string, remaining int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return 0, err
	}
	reader, err := entry.Open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".doc7-archive-*")
	if err != nil {
		return 0, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	limited := io.LimitReader(reader, remaining+1)
	written, copyErr := io.Copy(temporary, limited)
	closeErr := temporary.Close()
	if copyErr != nil {
		return 0, copyErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if written > remaining {
		return 0, errors.New("archive expanded size limit exceeded")
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return 0, err
	}
	return written, nil
}

func contentRoot(root string) (string, error) {
	current := root
	for {
		entries, err := os.ReadDir(current)
		if err != nil {
			return "", err
		}
		if len(entries) != 1 || !entries[0].IsDir() {
			return current, nil
		}
		current = filepath.Join(current, entries[0].Name())
	}
}

func safeDestination(root string, name string) (string, error) {
	cleanName := strings.ReplaceAll(name, "\\", "/")
	cleanName = filepath.Clean(filepath.FromSlash(cleanName))
	if cleanName == "." || filepath.IsAbs(cleanName) || filepath.VolumeName(cleanName) != "" {
		return "", fmt.Errorf("archive entry has unsafe path: %s", name)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	destination, err := filepath.Abs(filepath.Join(root, cleanName))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, destination)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry escapes output directory: %s", name)
	}
	return destination, nil
}
