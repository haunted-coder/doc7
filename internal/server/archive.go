package server

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func createArtifacts(outputDir string, destination string) error {
	if outputDir == "" {
		return errors.New("output directory is empty")
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".doc7-artifacts-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	archive := zip.NewWriter(temporary)
	walkErr := filepath.WalkDir(outputDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("job output contains a non-regular file: " + path)
		}
		relative, err := filepath.Rel(outputDir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if name == "." || strings.HasPrefix(name, "../") || filepath.IsAbs(name) {
			return errors.New("job output contains an unsafe path: " + name)
		}
		writer, err := archive.Create(name)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if walkErr != nil {
		_ = archive.Close()
		_ = temporary.Close()
		return walkErr
	}
	if err := archive.Close(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporaryPath, destination)
}
