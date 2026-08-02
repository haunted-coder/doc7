package convert

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/toolpath"
	"github.com/magicrew/doc7/internal/vlm"
)

const (
	RendererAuto           = "auto"
	RendererLibreOffice    = "libreoffice"
	RendererKeynote        = "keynote"
	externalProcessTimeout = 10 * time.Minute
)

type Options struct {
	OutputDir string
	Renderer  string
}

type Result struct {
	Input       detect.Input `json:"input"`
	OutputPath  string       `json:"output_path"`
	Renderer    string       `json:"renderer"`
	Passthrough bool         `json:"passthrough"`
	StartedAt   time.Time    `json:"started_at"`
	FinishedAt  time.Time    `json:"finished_at"`
}

func ToPDF(ctx context.Context, input detect.Input, options Options) (Result, error) {
	if options.Renderer == "" {
		options.Renderer = RendererAuto
	}
	if options.OutputDir == "" {
		options.OutputDir = input.Name + "_converted"
	}
	started := time.Now()
	if input.Kind == detect.KindPDF {
		return Result{Input: input, OutputPath: input.Path, Renderer: "pdf", Passthrough: true, StartedAt: started, FinishedAt: time.Now()}, nil
	}
	if !detect.IsOffice(input.Kind) {
		return Result{}, vlm.NewError(vlm.RenderError, "convert only supports PDF and Office/OpenDocument inputs", false, nil)
	}
	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return Result{}, err
	}
	outputPath := filepath.Join(options.OutputDir, input.Name+".pdf")
	renderer, err := officeToPDF(ctx, input.Kind, input.Path, outputPath, options.Renderer)
	if err != nil {
		return Result{}, err
	}
	return Result{Input: input, OutputPath: outputPath, Renderer: renderer, StartedAt: started, FinishedAt: time.Now()}, nil
}

func officeToPDF(ctx context.Context, kind detect.Kind, inputPath string, outputPath string, renderer string) (string, error) {
	switch renderer {
	case "", RendererAuto:
		var attempts []string
		if err := convertWithLibreOffice(ctx, inputPath, outputPath); err == nil {
			return RendererLibreOffice, nil
		} else {
			attempts = append(attempts, err.Error())
		}
		if detect.IsPresentation(kind) {
			if err := convertWithKeynote(ctx, inputPath, outputPath); err == nil {
				return RendererKeynote, nil
			} else {
				attempts = append(attempts, err.Error())
			}
		}
		return "", vlm.NewError(vlm.DependencyError, "no Office renderer available: "+strings.Join(attempts, " | "), false, nil)
	case RendererLibreOffice:
		if err := convertWithLibreOffice(ctx, inputPath, outputPath); err != nil {
			return "", err
		}
		return RendererLibreOffice, nil
	case RendererKeynote:
		if !detect.IsPresentation(kind) {
			return "", vlm.NewError(vlm.ConfigError, "Keynote renderer only supports presentation inputs", false, nil)
		}
		if err := convertWithKeynote(ctx, inputPath, outputPath); err != nil {
			return "", err
		}
		return RendererKeynote, nil
	default:
		return "", vlm.NewError(vlm.ConfigError, "unsupported presentation renderer: "+renderer, false, nil)
	}
}

func convertWithLibreOffice(ctx context.Context, inputPath string, outputPath string) error {
	soffice := FindLibreOffice()
	if soffice == "" {
		return vlm.NewError(vlm.DependencyError, "LibreOffice is required for Office/OpenDocument rendering; put LibreOfficePortable beside doc7 under tools or install LibreOffice", false, nil)
	}
	tmp, err := os.MkdirTemp("", "doc7-libreoffice-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	input, err := prepareLibreOfficeInput(inputPath, tmp)
	if err != nil {
		return err
	}
	outDir := filepath.Join(tmp, "out")
	profileDir := filepath.Join(tmp, "profile")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return err
	}
	processCtx, cancel := context.WithTimeout(ctx, externalProcessTimeout)
	defer cancel()
	cmd := exec.CommandContext(processCtx, soffice,
		"--headless",
		"--nologo",
		"--nofirststartwizard",
		"--nolockcheck",
		libreOfficeUserInstallationArg(profileDir),
		"--convert-to", "pdf",
		"--outdir", outDir,
		input,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if processCtx.Err() != nil {
			return vlm.NewError(vlm.TimeoutError, "LibreOffice timed out while converting the document", true, processCtx.Err())
		}
		return vlm.NewError(vlm.RenderError, "LibreOffice failed to convert Office document"+libreOfficeDiagnostics(string(output), outDir), false, err)
	}
	produced, err := findProducedPDF(outDir)
	if err != nil {
		return vlm.NewError(vlm.RenderError, "LibreOffice did not produce a PDF"+libreOfficeDiagnostics(string(output), outDir), false, err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	return copyFile(produced, outputPath)
}

func prepareLibreOfficeInput(inputPath string, workDir string) (string, error) {
	ext := strings.ToLower(filepath.Ext(inputPath))
	prepared := filepath.Join(workDir, "input"+ext)
	if err := copyFile(inputPath, prepared); err != nil {
		return "", err
	}
	return prepared, nil
}

func findProducedPDF(outputDir string) (string, error) {
	pdfs := []string{}
	err := filepath.WalkDir(outputDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".pdf") {
			pdfs = append(pdfs, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(pdfs)
	if len(pdfs) == 0 {
		return "", fmt.Errorf("no PDF found in %s", outputDir)
	}
	return pdfs[0], nil
}

func libreOfficeUserInstallationArg(profileDir string) string {
	absolute, err := filepath.Abs(profileDir)
	if err != nil {
		absolute = profileDir
	}
	slashPath := filepath.ToSlash(absolute)
	if volume := filepath.VolumeName(absolute); volume != "" && !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	return "-env:UserInstallation=" + (&url.URL{Scheme: "file", Path: slashPath}).String()
}

func libreOfficeDiagnostics(output string, outputDir string) string {
	parts := []string{}
	if trimmed := strings.TrimSpace(output); trimmed != "" {
		parts = append(parts, "output: "+trimmed)
	}
	if files := listFilesForDiagnostics(outputDir); files != "" {
		parts = append(parts, "files: "+files)
	}
	if len(parts) == 0 {
		return ""
	}
	return ": " + strings.Join(parts, "; ")
}

func listFilesForDiagnostics(dir string) string {
	files := []string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			files = append(files, walkErr.Error())
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			relative = path
		}
		files = append(files, relative)
		return nil
	})
	if err != nil {
		return err.Error()
	}
	sort.Strings(files)
	const maxFiles = 40
	if len(files) > maxFiles {
		files = append(files[:maxFiles], fmt.Sprintf("... and %d more", len(files)-maxFiles))
	}
	return strings.Join(files, ", ")
}

func FindLibreOffice() string {
	return toolpath.ResolveExecutable(
		toolpath.FromEnv("DOC7_LIBREOFFICE_PATH"),
		toolpath.NearExecutable("tools", "LibreOfficePortable", "App", "libreoffice", "program", "soffice.exe"),
		toolpath.NearExecutable("tools", "LibreOffice", "program", "soffice.exe"),
		toolpath.NearExecutable("tools", "libreoffice", "program", "soffice.exe"),
		`C:\Program Files\LibreOffice\program\soffice.exe`,
		`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
		"/Applications/LibreOffice.app/Contents/MacOS/soffice",
		"soffice",
		"libreoffice",
	)
}

func copyFile(src string, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	target, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer target.Close()
	_, err = io.Copy(target, source)
	return err
}
