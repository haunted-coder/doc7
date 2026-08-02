package cli

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/convert"
	"github.com/magicrew/doc7/internal/render"
	"github.com/magicrew/doc7/internal/vlm"
	"github.com/spf13/cobra"
)

type doctorItem struct {
	Name           string `json:"name"`
	OK             bool   `json:"ok"`
	Detail         string `json:"detail"`
	Requirement    string `json:"requirement,omitempty"`
	Severity       string `json:"severity,omitempty"`
	Installable    bool   `json:"installable,omitempty"`
	InstallCommand string `json:"install_command,omitempty"`
}

func newDoctorCommand() *cobra.Command {
	var checkModel bool
	cmd := &cobra.Command{
		Use:   "doctor [path]",
		Short: translate("doctor.short"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(config.FlagConfig{}, false)
			if err != nil {
				return err
			}
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			scope, err := scopeForDoctorTarget(target)
			if err != nil {
				return vlm.NewError(vlm.ConfigError, "failed to determine doctor scope", false, err)
			}
			return writeDoctor(cmd.Context(), cfg, checkModel, scope, target)
		},
	}
	cmd.Flags().BoolVar(&checkModel, "check-model", false, "send a real image request to the configured VLM")
	return cmd
}

func buildDoctorItems(ctx context.Context, cfg config.AppConfig, checkModel bool, scope doctorScope) []doctorItem {
	items := []doctorItem{
		{Name: "go", OK: true, Detail: runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH, Severity: "required"},
		checkPDFRenderer(scope.PDF),
		checkChrome(scope.HTML),
		checkOfficeRenderer(scope),
		checkKeynote(),
		checkVLMConfig(cfg, scope.VLM),
		checkAPIKey(cfg),
	}
	if checkModel {
		items = append(items, checkVLMProbe(ctx, cfg))
	}
	return items
}

func writeDoctor(ctx context.Context, cfg config.AppConfig, checkModel bool, scope doctorScope, target string) error {
	items := buildDoctorItems(ctx, cfg, checkModel, scope)
	ok := true
	for _, item := range items {
		if !item.OK && item.Severity == "required" {
			ok = false
		}
	}
	summary := map[string]interface{}{"ok": ok, "command": "doctor", "items": items}
	if target != "" {
		summary["target"] = target
	}
	if cfg.JSONOutput {
		if err := writeJSON(summary); err != nil {
			return err
		}
	} else {
		for _, item := range items {
			state := "ok"
			if !item.OK {
				state = "missing"
			}
			writeText("%s: %s - %s", item.Name, state, item.Detail)
			if !item.OK && item.InstallCommand != "" {
				writeText("  fix: %s", item.InstallCommand)
			}
		}
	}
	if !ok {
		return vlm.NewError(vlm.DependencyError, "required dependencies or VLM configuration are missing", false, nil)
	}
	return nil
}

func checkVLMProbe(ctx context.Context, cfg config.AppConfig) doctorItem {
	item := doctorItem{Name: "vlm_probe", Requirement: "verifies the configured endpoint accepts images", Severity: "required"}
	if cfg.BaseURL == "" || cfg.Model == "" {
		item.Detail = "base_url and model must be configured before probing"
		return item
	}

	imagePath, cleanup, err := writeProbeImage()
	if err != nil {
		item.Detail = "failed to create probe image: " + err.Error()
		return item
	}
	defer cleanup()

	client, err := vlm.NewOpenAICompatible(readVLMConfig(cfg), nil)
	if err != nil {
		item.Detail = err.Error()
		return item
	}
	started := time.Now()
	response, err := client.Complete(ctx, vlm.Request{
		Prompt:      "This is a vision connectivity test. If the image contains one solid black square centered on a white background, reply only DOC7_VISION_OK.",
		ImagePath:   imagePath,
		ImageMIME:   "image/png",
		ImageDetail: "low",
	})
	if err != nil {
		item.Detail = err.Error()
		return item
	}
	if !strings.Contains(strings.ToUpper(response.Content), "DOC7_VISION_OK") {
		item.Detail = "endpoint responded but did not confirm image understanding"
		return item
	}
	item.OK = true
	item.Detail = cfg.Model + " confirmed image input in " + time.Since(started).Round(time.Millisecond).String()
	return item
}

func writeProbeImage() (string, func(), error) {
	file, err := os.CreateTemp("", "doc7-vision-probe-*.png")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	canvas := image.NewRGBA(image.Rect(0, 0, 96, 96))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(24, 24, 72, 72), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	if err := png.Encode(file, canvas); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return file.Name(), cleanup, nil
}

func checkPDFRenderer(required bool) doctorItem {
	severity := dependencySeverity(required)
	if path := render.FindMuTool(); path != "" {
		return doctorItem{Name: "pdf_renderer", OK: true, Detail: "MuPDF " + path, Requirement: "required for PDF rendering", Severity: severity}
	}
	magick, magickErr := exec.LookPath("magick")
	gs, gsErr := findGhostscript()
	swift, swiftErr := exec.LookPath("swift")
	if magickErr == nil && gsErr == nil {
		return doctorItem{Name: "pdf_renderer", OK: true, Detail: "ImageMagick " + magick + " with Ghostscript " + gs, Requirement: "required for PDF rendering", Severity: severity}
	}
	if runtime.GOOS == "darwin" && swiftErr == nil {
		return doctorItem{Name: "pdf_renderer", OK: true, Detail: "macOS PDFKit via " + swift, Requirement: "required for PDF rendering", Severity: severity}
	}
	return doctorItem{Name: "pdf_renderer", OK: false, Detail: "no PDF renderer found; put mutool.exe at tools\\mupdf\\mutool.exe or install ImageMagick with Ghostscript", Requirement: "required for PDF rendering", Severity: severity, Installable: true, InstallCommand: "doc7 setup install pdf-renderer"}
}

func dependencySeverity(required bool) string {
	if required {
		return "required"
	}
	return "optional"
}

func findGhostscript() (string, error) {
	for _, name := range []string{"gs", "gswin64c", "gswin32c"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", exec.ErrNotFound
}

func checkChrome(required bool) doctorItem {
	severity := dependencySeverity(required)
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return doctorItem{Name: "chrome", OK: true, Detail: path, Requirement: "required for HTML pages and slides", Severity: severity}
		}
	}
	for _, name := range []string{"google-chrome", "chromium", "chrome", "msedge"} {
		if path, err := exec.LookPath(name); err == nil {
			return doctorItem{Name: "chrome", OK: true, Detail: path, Requirement: "required for HTML pages and slides", Severity: severity}
		}
	}
	return doctorItem{Name: "chrome", OK: false, Detail: "not found", Requirement: "required for HTML pages and slides", Severity: severity, Installable: true, InstallCommand: "doc7 setup install chrome"}
}

func checkLibreOffice(required bool) doctorItem {
	severity := dependencySeverity(required)
	if path := convert.FindLibreOffice(); path != "" {
		version, err := executableVersion(path, "--version")
		if err != nil {
			return doctorItem{Name: "libreoffice", OK: false, Detail: path + " found but not runnable: " + err.Error(), Requirement: "required for Office/OpenDocument rendering", Severity: severity, Installable: true, InstallCommand: "doc7 setup install libreoffice"}
		}
		return doctorItem{Name: "libreoffice", OK: true, Detail: version + " " + path, Requirement: "required for Office/OpenDocument rendering", Severity: severity}
	}
	return doctorItem{Name: "libreoffice", OK: false, Detail: "not found; put LibreOfficePortable at tools\\LibreOfficePortable or install LibreOffice", Requirement: "required for Office/OpenDocument rendering", Severity: severity, Installable: true, InstallCommand: "doc7 setup install libreoffice"}
}

func checkOfficeRenderer(scope doctorScope) doctorItem {
	libreOffice := checkLibreOffice(scope.Office)
	if !scope.Presentation || libreOffice.OK {
		return libreOffice
	}
	keynote := checkKeynote()
	if keynote.OK {
		return doctorItem{
			Name:        "libreoffice",
			OK:          true,
			Detail:      "LibreOffice unavailable; using Keynote fallback " + keynote.Detail,
			Requirement: "required for Office/OpenDocument rendering",
			Severity:    "required",
		}
	}
	return libreOffice
}

func executableVersion(path string, arg string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, arg).CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return "", err
		}
		return "", errWithDetail{err: err, detail: detail}
	}
	version := strings.TrimSpace(string(output))
	if version == "" {
		return filepathBase(path), nil
	}
	return firstLine(version), nil
}

type errWithDetail struct {
	err    error
	detail string
}

func (e errWithDetail) Error() string {
	return e.err.Error() + ": " + e.detail
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSpace(line)
}

func filepathBase(path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return r == '\\' || r == '/'
	})
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

func checkKeynote() doctorItem {
	if runtime.GOOS != "darwin" {
		return doctorItem{Name: "keynote", OK: false, Detail: "only available on macOS", Requirement: "optional macOS fallback for PPT/PPTX", Severity: "optional"}
	}
	if _, err := os.Stat("/Applications/Keynote.app"); err == nil {
		return doctorItem{Name: "keynote", OK: true, Detail: "/Applications/Keynote.app", Requirement: "optional macOS fallback for PPT/PPTX", Severity: "optional"}
	}
	return doctorItem{Name: "keynote", OK: false, Detail: "not found", Requirement: "optional macOS fallback for PPT/PPTX", Severity: "optional"}
}

func checkVLMConfig(cfg config.AppConfig, required bool) doctorItem {
	severity := dependencySeverity(required)
	requirement := "required for visual VLM extraction"
	if !required {
		requirement = "optional for native text conversion"
	}
	missing := []string{}
	if cfg.BaseURL == "" {
		missing = append(missing, "base_url")
	}
	if cfg.Model == "" {
		missing = append(missing, "model")
	}
	if len(missing) > 0 {
		return doctorItem{Name: "vlm_config", OK: false, Detail: "missing " + strings.Join(missing, ", "), Requirement: requirement, Severity: severity, Installable: required, InstallCommand: "doc7 setup config --base-url <url> --model <model> --api-key <key>"}
	}
	endpoint := vlm.RedactedEndpoint(cfg.BaseURL)
	if endpoint == "" {
		return doctorItem{Name: "vlm_config", OK: false, Detail: "base_url must be an absolute HTTP or HTTPS URL", Requirement: requirement, Severity: severity, Installable: required, InstallCommand: "doc7 setup config --base-url <url> --model <model>"}
	}
	return doctorItem{Name: "vlm_config", OK: true, Detail: cfg.Provider + " " + endpoint + " " + cfg.Model, Requirement: requirement, Severity: severity}
}

func checkAPIKey(cfg config.AppConfig) doctorItem {
	if cfg.APIKey != "" {
		return doctorItem{Name: "api_key", OK: true, Detail: credentialSourceDetail(cfg.APIKeySource), Requirement: "required only for authenticated endpoints", Severity: "optional"}
	}
	return doctorItem{Name: "api_key", OK: true, Detail: "not set; requests will omit the Authorization header", Requirement: "required only for authenticated endpoints", Severity: "optional"}
}

func credentialSourceDetail(source string) string {
	if strings.HasPrefix(source, "env:") {
		return strings.TrimPrefix(source, "env:") + " is set"
	}
	if strings.HasPrefix(source, "keychain:") {
		return "doc7 keychain credential is set for account " + strings.TrimPrefix(source, "keychain:")
	}
	if strings.HasPrefix(source, "file:") {
		return "doc7 credentials file is set at " + strings.TrimPrefix(source, "file:")
	}
	return "API key is configured"
}
