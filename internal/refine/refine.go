package refine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/magicrew/doc7/internal/vlm"
)

type Options struct {
	OutputPath    string
	OutputName    string
	Profile       string
	Language      string
	MaxInputChars int
	VLMConfig     vlm.Config
}

type Summary struct {
	OK           bool   `json:"ok"`
	Command      string `json:"command"`
	Input        string `json:"input"`
	Source       string `json:"source"`
	Output       string `json:"output"`
	Profile      string `json:"profile"`
	Language     string `json:"language"`
	InputChars   int    `json:"input_chars"`
	OutputChars  int    `json:"output_chars"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	PromptTokens int    `json:"prompt_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	TotalTokens  int    `json:"total_tokens,omitempty"`
}

func Run(ctx context.Context, inputPath string, options Options) (Summary, error) {
	started := time.Now()
	sourcePath, sourceName, err := resolveSource(inputPath)
	if err != nil {
		return Summary{}, vlm.NewError(vlm.ConfigError, "failed to resolve refine input", false, err)
	}
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		return Summary{}, vlm.NewError(vlm.ConfigError, "failed to read refine input", false, err)
	}
	content := strings.TrimSpace(string(raw))
	if content == "" {
		return Summary{}, vlm.NewError(vlm.ConfigError, "refine input is empty", false, nil)
	}
	maxChars := options.MaxInputChars
	if maxChars <= 0 {
		maxChars = 120000
	}
	if len([]rune(content)) > maxChars {
		return Summary{}, vlm.NewError(vlm.ConfigError, "refine input exceeds max input chars; raise --max-input-chars or split the document", false, nil)
	}
	profile := profileOrDefault(options.Profile)
	language := languageOrDefault(options.Language)
	prompt := buildPrompt(content, sourceName, profile, language)
	response, err := vlm.CompleteTextOpenAICompatible(ctx, options.VLMConfig, prompt, nil)
	if err != nil {
		return Summary{}, err
	}
	outputPath, err := resolveOutputPath(inputPath, sourceName, options)
	if err != nil {
		return Summary{}, vlm.NewError(vlm.ConfigError, "failed to resolve refine output", false, err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return Summary{}, err
	}
	refined := strings.TrimSpace(response.Content) + "\n"
	if err := os.WriteFile(outputPath, []byte(refined), 0o644); err != nil {
		return Summary{}, err
	}
	finished := time.Now()
	return Summary{
		OK:           true,
		Command:      "refine",
		Input:        inputPath,
		Source:       sourcePath,
		Output:       outputPath,
		Profile:      profile,
		Language:     language,
		InputChars:   len([]rune(content)),
		OutputChars:  len([]rune(refined)),
		StartedAt:    started.Format(time.RFC3339),
		FinishedAt:   finished.Format(time.RFC3339),
		PromptTokens: response.Usage.PromptTokens,
		OutputTokens: response.Usage.CompletionTokens,
		TotalTokens:  response.Usage.TotalTokens,
	}, nil
}

func resolveSource(path string) (string, string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		if !strings.EqualFold(filepath.Ext(absolute), ".md") {
			return "", "", errors.New("refine input file must be Markdown")
		}
		return absolute, strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute)), nil
	}
	source, err := findDirectoryMarkdown(absolute)
	if err != nil {
		return "", "", err
	}
	return source, sourceNameFromPath(source), nil
}

func findDirectoryMarkdown(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var candidates []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		name := entry.Name()
		if strings.Contains(name, "精炼") || strings.Contains(strings.ToLower(name), "refined") {
			continue
		}
		candidates = append(candidates, filepath.Join(dir, name))
	}
	if len(candidates) == 0 {
		return "", errors.New("directory does not contain a root Markdown file to refine")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return markdownRank(candidates[i]) < markdownRank(candidates[j])
	})
	return candidates[0], nil
}

func markdownRank(path string) int {
	name := filepath.Base(path)
	switch {
	case strings.Contains(name, "视觉理解"):
		return 0
	case strings.Contains(name, "完整描述汇总"):
		return 1
	default:
		return 2
	}
}

func sourceNameFromPath(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	for _, suffix := range []string{"-视觉理解", "_视觉理解", "-完整描述汇总", "_完整描述汇总"} {
		name = strings.TrimSuffix(name, suffix)
	}
	if name == "" || name == "images" {
		return "document"
	}
	return name
}

func resolveOutputPath(inputPath string, sourceName string, options Options) (string, error) {
	if strings.TrimSpace(options.OutputPath) != "" {
		return filepath.Abs(options.OutputPath)
	}
	outputName := strings.TrimSpace(options.OutputName)
	if outputName == "" {
		outputName = sourceName + "-精炼版.md"
	}
	if filepath.Base(outputName) != outputName || strings.ContainsAny(outputName, `/\`) {
		return "", errors.New("refined name must be a filename, not a path")
	}
	if !strings.EqualFold(filepath.Ext(outputName), ".md") {
		outputName += ".md"
	}
	dir := inputPath
	if info, err := os.Stat(inputPath); err == nil && !info.IsDir() {
		dir = filepath.Dir(inputPath)
	}
	return filepath.Abs(filepath.Join(dir, outputName))
}

func profileOrDefault(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "report":
		return "report"
	case "concise":
		return "concise"
	case "knowledge":
		return "knowledge"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func languageOrDefault(value string) string {
	if strings.TrimSpace(value) == "" {
		return "auto"
	}
	return strings.TrimSpace(value)
}

func buildPrompt(content string, sourceName string, profile string, language string) string {
	return `你是 doc7 的 Markdown 精炼阶段。输入来自 doc7 VLM 页级视觉理解结果，通常包含每页图片的描述、页码、来源、模型、页面结构和可复用结论。

你的任务是把它改写成一份高信噪比、可维护、可交接的 Markdown 文档，而不是继续逐页堆叠视觉描述。

语言规则：
1. language=` + language + ` 时，若为 auto，跟随输入文档的主语言。
2. 中文为主时使用简体中文；英文为主时使用英文。
3. 保留原文中的产品名、系统名、字段名、代码、英文缩写和关键数字。

精炼规则：
1. 删除页级调试信息，例如来源、模型、Content、page_xxx.png 这类包装信息。
2. 保留页码引用，用 P01、P02 形式标注关键结论来源。
3. 按主题重组，而不是机械按页复述。
4. 优先保留：目标、问题、关键动作、结果/价值、可复用方法、风险/反思、下一步计划。
5. 删除低价值视觉描述，例如颜色、装饰、普通左右布局，除非它影响理解。
6. 保留所有重要数字、状态、流程步骤、系统边界和决策逻辑。
7. 不要编造输入中没有的信息；不确定就省略或标注不确定。
8. 输出只能是 Markdown 正文，不要解释你的处理过程。

profile=` + profile + ` 的输出要求：
- report：适合述职、复盘、项目汇报，结构包含概览、工作总览、核心项目、方法论、反思、规划。
- concise：更短，优先表格和要点，减少叙述。
- knowledge：偏知识库沉淀，突出概念、模式、规则和可复用 SOP。

建议结构：
# ` + sourceName + ` 精炼版

## 一句话概览

## 结构总览

## 核心内容

## 可复用方法

## 风险与反思

## 下一步

下面是待精炼内容：

` + content
}
