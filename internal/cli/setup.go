package cli

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/credentials"
	"github.com/magicrew/doc7/internal/vlm"
	"github.com/spf13/cobra"
)

func newSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: translate("setup.short"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(config.FlagConfig{}, false)
			if err != nil {
				return err
			}
			_, err = ensureVisionConfig(cmd.Context(), cfg, "setup.pdf")
			if err != nil {
				return err
			}
			writeText("%s", translate("setup.complete"))
			return nil
		},
	}
	cmd.AddCommand(newSetupDoctorCommand())
	cmd.AddCommand(newSetupInstallCommand())
	cmd.AddCommand(newSetupConfigCommand())
	return cmd
}

func newSetupDoctorCommand() *cobra.Command {
	var checkModel bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: translate("doctor.short"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(config.FlagConfig{}, false)
			if err != nil {
				return err
			}
			return writeDoctor(cmd.Context(), cfg, checkModel, doctorScope{}, "")
		},
	}
	cmd.Flags().BoolVar(&checkModel, "check-model", false, "send a real image request to the configured VLM")
	return cmd
}

func newSetupInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install [dependency...]",
		Short: translate("setup.short"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				args = []string{"libreoffice", "pdf-renderer", "chrome"}
			}
			for _, dep := range args {
				if err := installDependency(dep); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func installDependency(dep string) error {
	if !supportedDependency(dep) {
		return errors.New("unsupported dependency: " + dep)
	}
	if dependencyAvailable(dep) {
		writeText("%s: already available", dep)
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		return installDependencyDarwin(dep)
	case "windows":
		return installDependencyWindows(dep)
	default:
		return errors.New("automatic install currently supports macOS Homebrew or Windows winget; install " + dep + " manually")
	}
}

func supportedDependency(dep string) bool {
	switch dep {
	case "libreoffice", "pdf-renderer", "chrome", "imagemagick", "ghostscript":
		return true
	default:
		return false
	}
}

func dependencyAvailable(dep string) bool {
	switch dep {
	case "libreoffice":
		return checkLibreOffice(true).OK
	case "pdf-renderer":
		return checkPDFRenderer(true).OK
	case "chrome":
		return checkChrome(true).OK
	case "imagemagick":
		_, err := exec.LookPath("magick")
		return err == nil
	case "ghostscript":
		_, err := findGhostscript()
		return err == nil
	default:
		return false
	}
}

func installDependencyDarwin(dep string) error {
	brew, err := exec.LookPath("brew")
	if err != nil {
		return errors.New("Homebrew is required for automatic install; install Homebrew first")
	}
	commands := map[string][]string{
		"libreoffice":  {"install", "--cask", "libreoffice"},
		"chrome":       {"install", "--cask", "google-chrome"},
		"pdf-renderer": {"install", "imagemagick", "ghostscript"},
		"imagemagick":  {"install", "imagemagick"},
		"ghostscript":  {"install", "ghostscript"},
	}
	args, ok := commands[dep]
	if !ok {
		return errors.New("automatic installation is not available for dependency: " + dep)
	}
	cmd := exec.Command(brew, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func installDependencyWindows(dep string) error {
	winget, err := exec.LookPath("winget")
	if err != nil {
		writeWindowsPortableHint(dep)
		return vlm.NewError(vlm.DependencyError, "winget is not available and "+dep+" is still missing", false, err)
	}
	commands := map[string][][]string{
		"libreoffice":  {{"install", "-e", "--id", "TheDocumentFoundation.LibreOffice"}},
		"chrome":       {{"install", "-e", "--id", "Google.Chrome"}},
		"pdf-renderer": {{"install", "-e", "--id", "ImageMagick.ImageMagick"}, {"install", "-e", "--id", "ArtifexSoftware.GhostScript"}},
		"imagemagick":  {{"install", "-e", "--id", "ImageMagick.ImageMagick"}},
		"ghostscript":  {{"install", "-e", "--id", "ArtifexSoftware.GhostScript"}},
	}
	steps, ok := commands[dep]
	if !ok {
		return errors.New("automatic installation is not available for dependency: " + dep)
	}
	for _, args := range steps {
		cmd := exec.Command(winget, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func writeWindowsPortableHint(dep string) {
	writeText("winget is not available; use the portable doc7 kit layout instead.")
	switch dep {
	case "libreoffice":
		writeText(`put LibreOfficePortable under tools\LibreOfficePortable beside doc7.exe`)
	case "pdf-renderer", "imagemagick", "ghostscript":
		writeText(`put MuPDF mutool.exe under tools\mupdf beside doc7.exe`)
	case "chrome":
		writeText("Chrome or Edge is required for HTML, SVG, EPUB, EML, MSG, and IPYNB rendering")
	default:
		writeText("install or unpack %s manually", dep)
	}
}

func newSetupConfigCommand() *cobra.Command {
	var language string
	var provider string
	var baseURL string
	var model string
	var apiKeyEnv string
	var apiKey string
	var apiKeyStdin bool
	var credentialStore string
	var credentialAccount string
	var credentialsPath string
	var pptRenderer string
	var fileWorkers int
	var workers int
	var maxTokens int
	var contextFallbacks int
	var minImageDimension int
	cmd := &cobra.Command{
		Use:   "config",
		Short: translate("setup.config.short"),
		RunE: func(cmd *cobra.Command, args []string) error {
			existing, _ := config.Load(globals.ConfigPath, config.EnvMap())
			if !cmd.Flags().Changed("language") && existing.Language != "" {
				language = existing.Language
			}
			if !cmd.Flags().Changed("provider") && existing.Provider != "" {
				provider = existing.Provider
			}
			if !cmd.Flags().Changed("base-url") && existing.BaseURL != "" {
				baseURL = existing.BaseURL
			}
			if !cmd.Flags().Changed("model") && existing.Model != "" {
				model = existing.Model
			}
			if !cmd.Flags().Changed("api-key-env") && existing.APIKeyEnv != "" {
				apiKeyEnv = existing.APIKeyEnv
			}
			if !cmd.Flags().Changed("credential-store") && existing.CredentialStore != "" {
				credentialStore = existing.CredentialStore
			}
			if !cmd.Flags().Changed("credential-account") && existing.CredentialAccount != "" {
				credentialAccount = existing.CredentialAccount
			}
			if !cmd.Flags().Changed("credentials-path") && existing.CredentialsPath != "" {
				credentialsPath = existing.CredentialsPath
			}
			if !cmd.Flags().Changed("ppt-renderer") && existing.PPTRenderer != "" {
				pptRenderer = existing.PPTRenderer
			}
			if !cmd.Flags().Changed("file-workers") && existing.FileWorkers > 0 {
				fileWorkers = existing.FileWorkers
			}
			if !cmd.Flags().Changed("workers") && existing.Workers > 0 {
				workers = existing.Workers
			}
			if !cmd.Flags().Changed("max-tokens") && existing.MaxTokens > 0 {
				maxTokens = existing.MaxTokens
			}
			if !cmd.Flags().Changed("context-fallbacks") {
				contextFallbacks = existing.ContextFallbacks
			}
			if !cmd.Flags().Changed("min-image-dimension") && existing.MinImageDimension > 0 {
				minImageDimension = existing.MinImageDimension
			}
			if language == "" {
				language = "auto"
			}
			if provider == "" {
				provider = "openai-compatible"
			}
			if apiKeyEnv == "" {
				apiKeyEnv = "DOC7_API_KEY"
			}
			if credentialStore == "" {
				credentialStore = credentials.StoreAuto
			}
			if credentialAccount == "" {
				credentialAccount = "default"
			}
			if pptRenderer == "" {
				pptRenderer = "auto"
			}
			if workers <= 0 {
				workers = 5
			}
			if fileWorkers <= 0 {
				fileWorkers = 1
			}
			if maxTokens <= 0 {
				maxTokens = 8192
			}
			if contextFallbacks < 0 {
				return errors.New("context fallbacks must not be negative")
			}
			if contextFallbacks > 0 && minImageDimension <= 0 {
				return errors.New("minimum image dimension must be greater than 0 when context fallbacks are enabled")
			}
			if _, err := credentials.NormalizeStore(credentialStore); err != nil {
				return err
			}
			stdinKey := ""
			if apiKeyStdin {
				data, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return err
				}
				stdinKey = strings.TrimSpace(string(data))
			}
			if strings.TrimSpace(apiKey) != "" && stdinKey != "" {
				return errors.New("use either --api-key or --api-key-stdin, not both")
			}
			if stdinKey != "" {
				apiKey = stdinKey
			}
			path := globals.ConfigPath
			if path == "" {
				path = config.DefaultConfigPath()
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			data := buildConfigYAML(language, provider, baseURL, model, apiKeyEnv, credentialStore, credentialAccount, credentialsPath, pptRenderer, fileWorkers, workers, maxTokens, contextFallbacks, minImageDimension)
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				return err
			}
			writeText("wrote %s", path)
			if strings.TrimSpace(apiKey) == "" {
				if credentialStore == credentials.StoreEnv {
					writeText("API key will be read from %s when set; unauthenticated endpoints need no key", apiKeyEnv)
				} else {
					writeText("API key was not stored; requests will omit Authorization until a key is configured")
				}
				return nil
			}
			source, err := credentials.Store(credentials.Options{
				Store:     credentialStore,
				Account:   credentialAccount,
				Path:      credentialsPath,
				APIKeyEnv: apiKeyEnv,
			}, apiKey)
			if err != nil {
				return err
			}
			writeText("stored API key in %s", source)
			writeText("next: doc7 doctor")
			return nil
		},
	}
	cmd.Flags().StringVar(&language, "language", "auto", translate("setup.language"))
	cmd.Flags().StringVar(&provider, "provider", "openai-compatible", "VLM provider")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "OpenAI-compatible base URL")
	cmd.Flags().StringVar(&model, "model", "", "VLM model name")
	cmd.Flags().StringVar(&apiKeyEnv, "api-key-env", "DOC7_API_KEY", "environment variable that stores the API key")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key to store in doc7 credential storage")
	cmd.Flags().BoolVar(&apiKeyStdin, "api-key-stdin", false, "read API key from stdin and store it in doc7 credential storage")
	cmd.Flags().StringVar(&credentialStore, "credential-store", credentials.StoreAuto, "credential storage: auto, keychain, file, or env")
	cmd.Flags().StringVar(&credentialAccount, "credential-account", "default", "credential account/profile name")
	cmd.Flags().StringVar(&credentialsPath, "credentials-path", "", "credential file path when --credential-store=file")
	cmd.Flags().StringVar(&pptRenderer, "ppt-renderer", "auto", "presentation renderer: auto, libreoffice, or keynote")
	cmd.Flags().IntVar(&fileWorkers, "file-workers", 1, "concurrent document workers")
	cmd.Flags().IntVar(&workers, "workers", 5, "concurrent page workers")
	cmd.Flags().IntVar(&maxTokens, "max-tokens", 8192, "maximum model output tokens per page")
	cmd.Flags().IntVar(&contextFallbacks, "context-fallbacks", 2, "lower request image resolution this many times when the model context is exhausted")
	cmd.Flags().IntVar(&minImageDimension, "min-image-dimension", 720, "minimum longest image side used by context fallback")
	return cmd
}

func buildConfigYAML(language string, provider string, baseURL string, model string, apiKeyEnv string, credentialStore string, credentialAccount string, credentialsPath string, pptRenderer string, fileWorkers int, workers int, maxTokens int, contextFallbacks int, minImageDimension int) string {
	remoteConfirmed := false
	data := "language: " + quoteConfigValue(language) + "\n" +
		"provider: " + quoteConfigValue(provider) + "\n" +
		"base_url: " + quoteConfigValue(baseURL) + "\n" +
		"model: " + quoteConfigValue(model) + "\n" +
		"api_key_env: " + quoteConfigValue(apiKeyEnv) + "\n" +
		"credential_store: " + quoteConfigValue(credentialStore) + "\n" +
		"credential_account: " + quoteConfigValue(credentialAccount) + "\n" +
		"remote_confirmed: " + strconv.FormatBool(remoteConfirmed) + "\n"
	if strings.TrimSpace(credentialsPath) != "" {
		data += "credentials_path: " + quoteConfigValue(credentialsPath) + "\n"
	}
	data += "ppt_renderer: " + quoteConfigValue(pptRenderer) + "\n" +
		"file_workers: " + strconv.Itoa(fileWorkers) + "\n" +
		"workers: " + strconv.Itoa(workers) + "\n" +
		"dpi: 220\n" +
		"image_detail: high\n" +
		"max_image_mb: 9\n" +
		"max_tokens: " + strconv.Itoa(maxTokens) + "\n" +
		"context_fallbacks: " + strconv.Itoa(contextFallbacks) + "\n" +
		"min_image_dimension: " + strconv.Itoa(minImageDimension) + "\n" +
		"timeout_seconds: 120\n" +
		"retry_count: 3\n" +
		"prompt: auto\n"
	return data
}

func quoteConfigValue(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
