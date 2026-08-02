package extract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/render"
	"github.com/magicrew/doc7/internal/vlm"
)

type resumeState struct {
	Manifest        Manifest
	RawManifest     []byte
	ManifestSHA256  string
	PageSelection   string
	SourcePageCount int
}

// ValidateResume checks an existing output without issuing a model request.
func ValidateResume(inputPath string, outputDir string, requestedPages string) error {
	input, err := detect.Detect(inputPath)
	if err != nil {
		return vlm.NewError(vlm.ConfigError, "failed to detect resume input", false, err)
	}
	pageSelection, err := render.NormalizePageSelection(requestedPages)
	if err != nil {
		return vlm.NewError(vlm.ConfigError, "invalid page selection", false, err)
	}
	_, err = prepareResume(outputDir, input, pageSelection)
	return err
}

func prepareResume(outputDir string, input detect.Input, requestedPages string) (*resumeState, error) {
	manifestPath := filepath.Join(outputDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, vlm.NewError(vlm.ConfigError, "resume requires an existing output manifest", false, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, vlm.NewError(vlm.ConfigError, "failed to read resume manifest", false, err)
	}
	if manifest.Mode != "visual" {
		return nil, vlm.NewError(vlm.ConfigError, "resume only supports visual document outputs", false, nil)
	}
	if manifest.Input.SHA256 == "" || manifest.Input.SHA256 != input.SHA256 {
		return nil, vlm.NewError(vlm.ConfigError, "resume input does not match the existing manifest", false, nil)
	}
	if len(manifest.Pages) == 0 {
		return nil, vlm.NewError(vlm.ConfigError, "resume manifest does not contain pages", false, nil)
	}

	pageStatus := make(map[int]PageStatus, len(manifest.Pages))
	failedPages := make([]int, 0)
	maxPage := 0
	for _, page := range manifest.Pages {
		if page.Page <= 0 {
			return nil, vlm.NewError(vlm.ConfigError, "resume manifest contains an invalid page number", false, nil)
		}
		if _, exists := pageStatus[page.Page]; exists {
			return nil, vlm.NewError(vlm.ConfigError, "resume manifest contains duplicate page numbers", false, nil)
		}
		if page.Status != StatusSuccess && page.Status != StatusError {
			return nil, vlm.NewError(vlm.ConfigError, fmt.Sprintf("resume manifest contains unsupported status for page %d: %s", page.Page, page.Status), false, nil)
		}
		pageStatus[page.Page] = page.Status
		if page.Status == StatusError {
			failedPages = append(failedPages, page.Page)
		}
		if page.Page > maxPage {
			maxPage = page.Page
		}
	}
	sourcePageCount := manifest.Render.SourcePageCount
	if sourcePageCount <= 0 {
		sourcePageCount = maxPage
	}
	if sourcePageCount < maxPage {
		return nil, vlm.NewError(vlm.ConfigError, "resume manifest source page count is inconsistent", false, nil)
	}

	pageSelection := ""
	if strings.TrimSpace(requestedPages) == "" {
		pageSelection, err = pageSelectionFromNumbers(failedPages)
		if err != nil {
			return nil, err
		}
	} else {
		selection, parseErr := render.ParsePageSelection(requestedPages)
		if parseErr != nil {
			return nil, vlm.NewError(vlm.ConfigError, "invalid page selection", false, parseErr)
		}
		selectedPages, expandErr := selection.Pages(sourcePageCount)
		if expandErr != nil {
			return nil, vlm.NewError(vlm.ConfigError, "invalid page selection", false, expandErr)
		}
		for _, page := range selectedPages {
			status, exists := pageStatus[page]
			if !exists {
				return nil, vlm.NewError(vlm.ConfigError, fmt.Sprintf("page %d is not present in the existing output", page), false, nil)
			}
			if status != StatusError {
				return nil, vlm.NewError(vlm.ConfigError, fmt.Sprintf("page %d already succeeded; resume only retries failed pages", page), false, nil)
			}
		}
		pageSelection = selection.String()
	}

	hash := sha256.Sum256(data)
	return &resumeState{
		Manifest:        manifest,
		RawManifest:     data,
		ManifestSHA256:  hex.EncodeToString(hash[:]),
		PageSelection:   pageSelection,
		SourcePageCount: sourcePageCount,
	}, nil
}

func pageSelectionFromNumbers(pages []int) (string, error) {
	if len(pages) == 0 {
		return "", nil
	}
	sort.Ints(pages)
	parts := make([]string, 0, len(pages))
	for _, page := range pages {
		parts = append(parts, strconv.Itoa(page))
	}
	selection, err := render.NormalizePageSelection(strings.Join(parts, ","))
	if err != nil {
		return "", vlm.NewError(vlm.ConfigError, "failed to build failed-page selection", false, err)
	}
	return selection, nil
}

func completedResumeSummary(state *resumeState, input detect.Input, options Options) (Summary, error) {
	mergePaths := make([]string, 0, len(state.Manifest.Pages))
	pagesCached := 0
	groundingWarnings := 0
	for _, page := range state.Manifest.Pages {
		if page.Status != StatusSuccess {
			return Summary{}, vlm.NewError(vlm.ConfigError, fmt.Sprintf("resume manifest contains unsupported status for page %d: %s", page.Page, page.Status), false, nil)
		}
		markdownPath, err := resumeArtifactPath(options.OutputDir, state.Manifest, page.MarkdownPath)
		if err != nil {
			return Summary{}, vlm.NewError(vlm.ConfigError, fmt.Sprintf("resume output path is invalid for page %d", page.Page), false, err)
		}
		if !nonEmptyFile(markdownPath) {
			return Summary{}, vlm.NewError(vlm.ConfigError, fmt.Sprintf("resume output is missing Markdown for page %d", page.Page), false, nil)
		}
		metaPath, err := resumeArtifactPath(options.OutputDir, state.Manifest, page.MetaPath)
		if err != nil {
			return Summary{}, vlm.NewError(vlm.ConfigError, fmt.Sprintf("resume metadata path is invalid for page %d", page.Page), false, err)
		}
		meta, err := readPageMeta(metaPath)
		if err != nil {
			return Summary{}, vlm.NewError(vlm.ConfigError, fmt.Sprintf("resume output is missing metadata for page %d", page.Page), false, err)
		}
		if meta.Page != page.Page || meta.Status != StatusSuccess {
			return Summary{}, vlm.NewError(vlm.ConfigError, fmt.Sprintf("resume metadata is inconsistent for page %d", page.Page), false, nil)
		}
		if page.Cached {
			pagesCached++
		}
		if hasGroundingWarning(meta) {
			groundingWarnings++
		}
		mergePaths = append(mergePaths, markdownPath)
	}

	summary := Summary{
		OK:                true,
		Command:           "extract",
		Mode:              "visual",
		Input:             summaryInput(input),
		OutputDir:         options.OutputDir,
		ManifestPath:      filepath.Join(options.OutputDir, "manifest.json"),
		PagesTotal:        len(state.Manifest.Pages),
		SourcePagesTotal:  state.SourcePageCount,
		PagesRetained:     len(state.Manifest.Pages),
		PagesSuccess:      len(state.Manifest.Pages),
		PagesCached:       pagesCached,
		GroundingWarnings: groundingWarnings,
		Resumed:           true,
	}
	if options.Merge {
		mergedName, err := mergedMarkdownName(input.Name, options.MergedName)
		if err != nil {
			return summary, err
		}
		mergedPath := filepath.Join(options.OutputDir, mergedName)
		if err := MergeMarkdown(mergedPath, mergePaths); err != nil {
			return summary, err
		}
		summary.MergedMarkdown = mergedPath
	}
	return summary, nil
}

func archiveResumeManifest(outputDir string, state *resumeState) (string, error) {
	historyDir := filepath.Join(outputDir, "history")
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		return "", err
	}
	name := "manifest_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10) + ".json"
	path := filepath.Join(historyDir, name)
	if err := os.WriteFile(path, state.RawManifest, 0o644); err != nil {
		return "", err
	}
	return render.RelativeArtifactPath(outputDir, path), nil
}

func mergeResumeResult(outputDir string, input detect.Input, options Options, state *resumeState, current Manifest, currentSummary Summary) (Manifest, Summary, []string, error) {
	historyPath, err := archiveResumeManifest(outputDir, state)
	if err != nil {
		return Manifest{}, Summary{}, nil, err
	}
	processed := make(map[int]ManifestPage, len(current.Pages))
	for _, page := range current.Pages {
		processed[page.Page] = page
	}

	combined := make([]ManifestPage, 0, len(state.Manifest.Pages))
	for _, previousPage := range state.Manifest.Pages {
		if replacement, exists := processed[previousPage.Page]; exists {
			combined = append(combined, replacement)
			continue
		}
		normalized, normalizeErr := normalizeRetainedPage(outputDir, previousPage, options.Render.KeepImages)
		if normalizeErr != nil {
			return Manifest{}, Summary{}, nil, normalizeErr
		}
		combined = append(combined, normalized)
	}
	sort.Slice(combined, func(left int, right int) bool {
		return combined[left].Page < combined[right].Page
	})

	current.Pages = combined
	current.Render.PageCount = len(combined)
	current.Render.SourcePageCount = state.SourcePageCount
	current.Render.PageSelection = state.PageSelection
	current.Resume = &ResumeInfo{
		PreviousManifestPath:   historyPath,
		PreviousManifestSHA256: state.ManifestSHA256,
		PageSelection:          state.PageSelection,
		PagesProcessed:         len(processed),
		PagesRetained:          len(combined) - len(processed),
	}

	current.Summary.PagesTotal = len(combined)
	current.Summary.PagesProcessed = len(processed)
	current.Summary.PagesRetained = len(combined) - len(processed)
	current.Summary.PagesSuccess = 0
	current.Summary.PagesFailed = 0
	current.Summary.PagesCached = currentSummary.PagesCached
	current.Summary.GroundingWarnings = 0

	mergePaths := make([]string, 0, len(combined))
	pageMetas := make([]PageMeta, 0, len(combined))
	for _, page := range combined {
		metaPath, pathErr := resumeArtifactPath(outputDir, current, page.MetaPath)
		if pathErr != nil {
			return Manifest{}, Summary{}, nil, pathErr
		}
		meta, readErr := readPageMeta(metaPath)
		if readErr != nil {
			return Manifest{}, Summary{}, nil, readErr
		}
		pageMetas = append(pageMetas, meta)
		if page.Status == StatusSuccess {
			current.Summary.PagesSuccess++
			markdownPath, markdownPathErr := resumeArtifactPath(outputDir, current, page.MarkdownPath)
			if markdownPathErr != nil {
				return Manifest{}, Summary{}, nil, markdownPathErr
			}
			if !nonEmptyFile(markdownPath) {
				return Manifest{}, Summary{}, nil, fmt.Errorf("resumed page %d is missing Markdown", page.Page)
			}
			mergePaths = append(mergePaths, markdownPath)
		} else {
			current.Summary.PagesFailed++
		}
		if hasGroundingWarning(meta) {
			current.Summary.GroundingWarnings++
		}
	}
	applyResumeProvenance(&current, pageMetas)

	summary := currentSummary
	summary.OK = current.Summary.PagesFailed == 0
	summary.Input = summaryInput(input)
	summary.PagesTotal = len(combined)
	summary.SourcePagesTotal = state.SourcePageCount
	summary.PagesProcessed = len(processed)
	summary.PagesRetained = len(combined) - len(processed)
	summary.PagesSuccess = current.Summary.PagesSuccess
	summary.PagesFailed = current.Summary.PagesFailed
	summary.GroundingWarnings = current.Summary.GroundingWarnings
	summary.Resumed = true
	return current, summary, mergePaths, nil
}

func resumeArtifactPath(outputDir string, manifest Manifest, artifact string) (string, error) {
	artifact = strings.TrimSpace(artifact)
	if artifact == "" {
		return "", errors.New("resume manifest contains an empty artifact path")
	}
	path := filepath.FromSlash(artifact)
	if !looksLikeAbsoluteArtifactPath(artifact) {
		candidate := filepath.Join(outputDir, path)
		if !pathWithin(outputDir, candidate) {
			return "", fmt.Errorf("artifact path escapes the output directory: %q", artifact)
		}
		return candidate, nil
	}

	if filepath.IsAbs(path) && pathWithin(outputDir, path) {
		return path, nil
	}

	legacyOutput := filepath.FromSlash(strings.TrimSpace(manifest.Render.OutputDir))
	if filepath.IsAbs(path) && filepath.IsAbs(legacyOutput) {
		roots := []string{legacyOutput, filepath.Dir(legacyOutput)}
		candidates := make([]string, 0, len(roots))
		for _, root := range roots {
			relative, err := filepath.Rel(root, path)
			if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				continue
			}
			candidate := filepath.Join(outputDir, relative)
			if _, existsErr := os.Stat(candidate); existsErr == nil {
				return candidate, nil
			}
			if isResumeArtifactRelative(relative) {
				candidates = append(candidates, candidate)
			}
		}
		if len(candidates) > 0 {
			return candidates[0], nil
		}
	}

	if relative, ok := portableArtifactRelative(artifact); ok {
		return filepath.Join(outputDir, relative), nil
	}

	return "", fmt.Errorf("cannot map absolute artifact path %q into output directory", artifact)
}

func looksLikeAbsoluteArtifactPath(path string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), `\`, "/")
	if strings.HasPrefix(normalized, "/") {
		return true
	}
	return len(normalized) >= 3 && normalized[1] == ':' && normalized[2] == '/'
}

func portableArtifactRelative(path string) (string, bool) {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), `\`, "/")
	parts := strings.Split(strings.Trim(normalized, "/"), "/")
	for index := len(parts) - 2; index >= 0; index-- {
		if !isResumeArtifactRoot(parts[index]) {
			continue
		}
		for _, part := range parts[index:] {
			if part == "" || part == "." || part == ".." {
				return "", false
			}
		}
		return filepath.FromSlash(strings.Join(parts[index:], "/")), true
	}
	return "", false
}

func pathWithin(root string, path string) bool {
	absoluteRoot, rootErr := filepath.Abs(root)
	absolutePath, pathErr := filepath.Abs(path)
	if rootErr != nil || pathErr != nil {
		return false
	}
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func isResumeArtifactRelative(path string) bool {
	clean := filepath.Clean(path)
	first := clean
	if index := strings.IndexRune(clean, filepath.Separator); index >= 0 {
		first = clean[:index]
	}
	return isResumeArtifactRoot(first)
}

func isResumeArtifactRoot(value string) bool {
	switch value {
	case "images", "pages", "meta", "history":
		return true
	default:
		return false
	}
}

func normalizeRetainedPage(outputDir string, page ManifestPage, keepImages bool) (ManifestPage, error) {
	stem := pageStem(page.Page)
	markdownPath := filepath.Join(outputDir, "pages", stem+".md")
	metaPath := filepath.Join(outputDir, "meta", stem+".json")
	meta, err := readPageMeta(metaPath)
	if err != nil {
		return ManifestPage{}, err
	}
	page.MarkdownPath = render.RelativeArtifactPath(outputDir, markdownPath)
	page.MetaPath = render.RelativeArtifactPath(outputDir, metaPath)
	page.ImagePath = ""
	meta.ImagePath = ""
	if keepImages {
		imagePath, imageErr := findResumeImage(outputDir, stem)
		if imageErr != nil {
			return ManifestPage{}, imageErr
		}
		page.ImagePath = render.RelativeArtifactPath(outputDir, imagePath)
		meta.ImagePath = page.ImagePath
	}
	if err := writeJSON(metaPath, meta); err != nil {
		return ManifestPage{}, err
	}
	return page, nil
}

func findResumeImage(outputDir string, stem string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(outputDir, "images", stem+".*"))
	if err != nil {
		return "", err
	}
	sort.Strings(matches)
	for _, path := range matches {
		if info, statErr := os.Stat(path); statErr == nil && info.Mode().IsRegular() && info.Size() > 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("resumed page image is missing: %s", stem)
}

func readPageMeta(path string) (PageMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PageMeta{}, err
	}
	var meta PageMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return PageMeta{}, err
	}
	return meta, nil
}

func applyResumeProvenance(manifest *Manifest, metas []PageMeta) {
	providers := uniqueStrings(metas, func(meta PageMeta) string { return meta.Provider })
	baseURLs := uniqueStrings(metas, func(meta PageMeta) string { return meta.BaseURL })
	models := uniqueStrings(metas, func(meta PageMeta) string { return meta.Model })
	manifest.VLM.Providers = providers
	manifest.VLM.BaseURLs = baseURLs
	manifest.VLM.Models = models
	if len(providers) == 1 {
		manifest.VLM.Provider = providers[0]
	} else if len(providers) > 1 {
		manifest.VLM.Provider = "mixed"
	}
	if len(baseURLs) == 1 {
		manifest.VLM.BaseURL = baseURLs[0]
	} else if len(baseURLs) > 1 {
		manifest.VLM.BaseURL = "mixed"
	}
	if len(models) == 1 {
		manifest.VLM.Model = models[0]
	} else if len(models) > 1 {
		manifest.VLM.Model = "mixed"
	}
	manifest.VLM.Mixed = len(providers) > 1 || len(baseURLs) > 1 || len(models) > 1 || mixedPageSettings(manifest, metas)
	if !manifest.VLM.Mixed {
		manifest.VLM.Providers = nil
		manifest.VLM.BaseURLs = nil
		manifest.VLM.Models = nil
	}
}

func uniqueStrings(metas []PageMeta, value func(PageMeta) string) []string {
	seen := make(map[string]struct{})
	for _, meta := range metas {
		candidate := strings.TrimSpace(value(meta))
		if candidate != "" {
			seen[candidate] = struct{}{}
		}
	}
	values := make([]string, 0, len(seen))
	for candidate := range seen {
		values = append(values, candidate)
	}
	sort.Strings(values)
	return values
}

func mixedPageSettings(manifest *Manifest, metas []PageMeta) bool {
	promptHashes := make(map[string]struct{})
	for _, meta := range metas {
		if meta.MaxTokens != 0 && meta.MaxTokens != manifest.VLM.MaxTokens {
			return true
		}
		if meta.TextGrounding != manifest.VLM.TextGrounding {
			return true
		}
		if meta.PromptName != "" && meta.PromptName != manifest.VLM.PromptName {
			return true
		}
		if promptHash := strings.TrimSpace(meta.PromptSHA256); promptHash != "" {
			promptHashes[promptHash] = struct{}{}
		}
	}
	return len(promptHashes) > 1
}
