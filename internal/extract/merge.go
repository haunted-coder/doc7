package extract

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func MergeMarkdown(outputPath string, pagePaths []string) error {
	sortMarkdownPaths(pagePaths)
	var builder strings.Builder
	for i, path := range pagePaths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(strings.TrimSpace(string(data)))
	}
	builder.WriteString("\n")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(builder.String()), 0o644)
}

func FindMarkdownPages(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if strings.EqualFold(filepath.Ext(path), ".md") {
			return []string{path}, nil
		}
		return nil, errors.New("merge input must be a Markdown file or directory")
	}
	candidates := []string{filepath.Join(path, "pages"), path}
	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var pages []string
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
				continue
			}
			pages = append(pages, filepath.Join(dir, entry.Name()))
		}
		sortMarkdownPaths(pages)
		if len(pages) > 0 {
			return pages, nil
		}
	}
	return nil, errors.New("no Markdown pages found; expected <output>/pages/*.md or *.md files")
}

func sortMarkdownPaths(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		left, leftOK := pageNumber(paths[i])
		right, rightOK := pageNumber(paths[j])
		if leftOK && rightOK && left != right {
			return left < right
		}
		return paths[i] < paths[j]
	})
}

func pageNumber(path string) (int, bool) {
	name := filepath.Base(path)
	if !strings.HasPrefix(name, "page_") || !strings.EqualFold(filepath.Ext(name), ".md") {
		return 0, false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(name, "page_"), filepath.Ext(name))
	page, err := strconv.Atoi(value)
	if err != nil || page <= 0 {
		return 0, false
	}
	return page, true
}
