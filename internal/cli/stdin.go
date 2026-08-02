package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/detect"
)

const bytesPerMiB int64 = 1024 * 1024

func stageStdin(reader io.Reader, requestedName string, maxMB int64) (string, string, func(), error) {
	name, err := normalizeStdinName(requestedName)
	if err != nil {
		return "", "", func() {}, err
	}
	maxBytes, err := stdinByteLimit(maxMB)
	if err != nil {
		return "", "", func() {}, err
	}
	temporaryDir, err := os.MkdirTemp("", "doc7-stdin-*")
	if err != nil {
		return "", "", func() {}, err
	}
	cleanup := func() {
		_ = os.RemoveAll(temporaryDir)
	}
	path := filepath.Join(temporaryDir, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	written, copyErr := io.Copy(file, io.LimitReader(reader, maxBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		cleanup()
		return "", "", func() {}, copyErr
	}
	if closeErr != nil {
		cleanup()
		return "", "", func() {}, closeErr
	}
	if written > maxBytes {
		cleanup()
		return "", "", func() {}, fmt.Errorf("stdin input exceeds --stdin-max-mb limit of %d MB", maxMB)
	}
	return path, name, cleanup, nil
}

func stdinByteLimit(maxMB int64) (int64, error) {
	if maxMB <= 0 {
		return 0, errors.New("--stdin-max-mb must be greater than 0")
	}
	maxInt64 := int64(^uint64(0) >> 1)
	if maxMB > maxInt64/bytesPerMiB {
		return 0, errors.New("--stdin-max-mb is too large")
	}
	return maxMB * bytesPerMiB, nil
}

func normalizeStdinName(requestedName string) (string, error) {
	name := strings.TrimSpace(requestedName)
	if name == "" {
		return "", errors.New("--stdin-name is required and must include the input extension")
	}
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return "", errors.New("--stdin-name must be a filename, not a path")
	}
	extension := strings.ToLower(filepath.Ext(name))
	if extension == ".zip" {
		return name, nil
	}
	if extension == "" || !detect.IsSupportedFile(name) {
		return "", fmt.Errorf("unsupported stdin filename extension: %s", extension)
	}
	return name, nil
}
