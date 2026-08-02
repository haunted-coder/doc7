package batch

import (
	"path/filepath"
	"strings"
	"unicode"
)

func relativeOutputName(inputRoot string, path string, index int) string {
	relative, err := filepath.Rel(inputRoot, path)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		return numberedName(index, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	}
	return sanitizeRelativeOutputName(strings.TrimSuffix(relative, filepath.Ext(relative)))
}

func relativeOutputNames(inputRoot string, paths []string) []string {
	baseNames := make([]string, len(paths))
	counts := make(map[string]int, len(paths))
	for index, path := range paths {
		name := relativeOutputName(inputRoot, path, index+1)
		baseNames[index] = name
		counts[outputNameKey(name)]++
	}
	result := make([]string, len(paths))
	used := make(map[string]struct{}, len(paths))
	for index, path := range paths {
		name := baseNames[index]
		if counts[outputNameKey(name)] > 1 {
			extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
			if extension == "" {
				extension = "file"
			}
			name = appendOutputNameSuffix(name, safeName(extension))
		}
		candidate := name
		for sequence := 2; ; sequence++ {
			key := outputNameKey(candidate)
			if _, exists := used[key]; !exists {
				used[key] = struct{}{}
				result[index] = candidate
				break
			}
			candidate = appendOutputNameSuffix(name, leftPad3(sequence))
		}
	}
	return result
}

func appendOutputNameSuffix(name string, suffix string) string {
	dir := filepath.Dir(name)
	base := filepath.Base(name) + "_" + suffix
	if dir == "." {
		return base
	}
	return filepath.Join(dir, base)
}

func outputNameKey(name string) string {
	return strings.ToLower(filepath.Clean(name))
}

func numberedName(index int, name string) string {
	return leftPad3(index) + "_" + safeName(name)
}

func safeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "document"
	}
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\\|?*`, r) {
			if !lastUnderscore {
				builder.WriteRune('_')
				lastUnderscore = true
			}
			continue
		}
		builder.WriteRune(r)
		lastUnderscore = false
	}
	name := strings.Trim(builder.String(), " .")
	if name == "" {
		return "document"
	}
	extension := filepath.Ext(name)
	base := strings.TrimSuffix(name, extension)
	if isWindowsReservedName(base) {
		base = "_" + base
	}
	name = base + extension
	const maxComponentRunes = 120
	if len([]rune(name)) > maxComponentRunes {
		baseRunes := []rune(base)
		keep := maxComponentRunes - len([]rune(extension))
		if keep < 1 {
			keep = 1
		}
		if len(baseRunes) > keep {
			base = string(baseRunes[:keep])
		}
		name = base + extension
	}
	return name
}

func sanitizeRelativeOutputName(value string) string {
	parts := strings.Split(filepath.ToSlash(value), "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			continue
		}
		cleaned = append(cleaned, safeName(part))
	}
	if len(cleaned) == 0 {
		return "document"
	}
	return filepath.Join(cleaned...)
}

func isWindowsReservedName(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

func leftPad3(value int) string {
	if value < 10 {
		return "00" + strconvItoa(value)
	}
	if value < 100 {
		return "0" + strconvItoa(value)
	}
	return strconvItoa(value)
}
