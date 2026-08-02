package toolpath

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func NearExecutable(parts ...string) string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	all := append([]string{filepath.Dir(exe)}, parts...)
	return filepath.Join(all...)
}

func FromEnv(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func ResolveExecutable(candidates ...string) string {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if filepath.IsAbs(candidate) || strings.ContainsAny(candidate, `/\`) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}
