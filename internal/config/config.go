package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/magicrew/doc7/internal/credentials"
)

type AppConfig struct {
	Language          string
	Provider          string
	BaseURL           string
	Model             string
	APIKey            string
	APIKeySource      string
	APIKeyEnv         string
	CredentialStore   string
	CredentialAccount string
	CredentialsPath   string
	RemoteConfirmed   bool
	PPTRenderer       string
	FileWorkers       int
	Workers           int
	DPI               int
	ImageDetail       string
	MaxImageMB        int
	MaxTokens         int
	ContextFallbacks  int
	MinImageDimension int
	TimeoutSeconds    int
	RetryCount        int
	PromptName        string
	JSONOutput        bool
	NoColor           bool
	Quiet             bool
	Verbose           bool
}

type FlagConfig struct {
	Language          string
	ConfigPath        string
	Provider          string
	BaseURL           string
	Model             string
	APIKeyEnv         string
	CredentialStore   string
	CredentialAccount string
	CredentialsPath   string
	RemoteConfirmed   *bool
	PPTRenderer       string
	FileWorkers       int
	Workers           int
	DPI               int
	ImageDetail       string
	MaxImageMB        int
	MaxTokens         int
	ContextFallbacks  *int
	MinImageDimension int
	TimeoutSeconds    int
	RetryCount        int
	PromptName        string
	OutputDir         string
	JSONOutput        bool
	NoColor           bool
	Quiet             bool
	Verbose           bool
}

func Default() AppConfig {
	return AppConfig{
		Language:          "auto",
		Provider:          "openai-compatible",
		APIKeyEnv:         "DOC7_API_KEY",
		CredentialStore:   credentials.StoreAuto,
		CredentialAccount: "default",
		PPTRenderer:       "auto",
		FileWorkers:       1,
		Workers:           5,
		DPI:               220,
		ImageDetail:       "high",
		MaxImageMB:        9,
		MaxTokens:         8192,
		ContextFallbacks:  2,
		MinImageDimension: 720,
		TimeoutSeconds:    120,
		RetryCount:        3,
		PromptName:        "auto",
	}
}

func Load(path string, env map[string]string) (AppConfig, error) {
	cfg := Default()
	for _, candidate := range configCandidates(path) {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			values, err := readSimpleYAML(candidate)
			if err != nil {
				return cfg, err
			}
			applyMap(&cfg, values)
		}
	}
	applyEnv(&cfg, env)
	return ResolveCredentials(cfg, env)
}

func ResolveCredentials(cfg AppConfig, env map[string]string) (AppConfig, error) {
	result, err := credentials.Resolve(credentials.Options{
		Store:       cfg.CredentialStore,
		Account:     cfg.CredentialAccount,
		Path:        cfg.CredentialsPath,
		APIKeyEnv:   cfg.APIKeyEnv,
		Environment: env,
	})
	if err != nil {
		return cfg, err
	}
	cfg.APIKey = result.Key
	cfg.APIKeySource = result.Source
	return cfg, nil
}

func MergeFlags(base AppConfig, flags FlagConfig) (AppConfig, error) {
	cfg := base
	if flags.Language != "" {
		cfg.Language = flags.Language
	}
	if flags.Provider != "" {
		cfg.Provider = flags.Provider
	}
	if flags.BaseURL != "" {
		cfg.BaseURL = flags.BaseURL
	}
	if flags.Model != "" {
		cfg.Model = flags.Model
	}
	if flags.APIKeyEnv != "" {
		cfg.APIKeyEnv = flags.APIKeyEnv
	}
	if flags.CredentialStore != "" {
		cfg.CredentialStore = flags.CredentialStore
	}
	if flags.CredentialAccount != "" {
		cfg.CredentialAccount = flags.CredentialAccount
	}
	if flags.CredentialsPath != "" {
		cfg.CredentialsPath = flags.CredentialsPath
	}
	if flags.RemoteConfirmed != nil {
		cfg.RemoteConfirmed = *flags.RemoteConfirmed
	}
	if flags.PPTRenderer != "" {
		cfg.PPTRenderer = flags.PPTRenderer
	}
	if flags.FileWorkers > 0 {
		cfg.FileWorkers = flags.FileWorkers
	}
	if flags.Workers > 0 {
		cfg.Workers = flags.Workers
	}
	if flags.DPI > 0 {
		cfg.DPI = flags.DPI
	}
	if flags.ImageDetail != "" {
		cfg.ImageDetail = flags.ImageDetail
	}
	if flags.MaxImageMB > 0 {
		cfg.MaxImageMB = flags.MaxImageMB
	}
	if flags.MaxTokens > 0 {
		cfg.MaxTokens = flags.MaxTokens
	}
	if flags.ContextFallbacks != nil {
		cfg.ContextFallbacks = *flags.ContextFallbacks
	}
	if flags.MinImageDimension > 0 {
		cfg.MinImageDimension = flags.MinImageDimension
	}
	if flags.TimeoutSeconds > 0 {
		cfg.TimeoutSeconds = flags.TimeoutSeconds
	}
	if flags.RetryCount >= 0 {
		cfg.RetryCount = flags.RetryCount
	}
	if flags.PromptName != "" {
		cfg.PromptName = flags.PromptName
	}
	cfg.JSONOutput = flags.JSONOutput
	cfg.NoColor = flags.NoColor
	cfg.Quiet = flags.Quiet
	cfg.Verbose = flags.Verbose
	return cfg, nil
}

func Validate(cfg AppConfig, requireVLM bool) error {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(cfg.Language, "_", "-"))) {
	case "", "auto", "en", "en-us", "en-gb", "zh", "zh-cn", "zh-hans", "zh-tw", "zh-hant":
	default:
		return errors.New("language must be auto, en, or zh-CN")
	}
	if cfg.Workers <= 0 {
		return errors.New("workers must be greater than 0")
	}
	if cfg.FileWorkers <= 0 {
		return errors.New("file workers must be greater than 0")
	}
	if cfg.DPI <= 0 {
		return errors.New("dpi must be greater than 0")
	}
	if cfg.MaxImageMB <= 0 {
		return errors.New("max image MB must be greater than 0")
	}
	if cfg.MaxTokens <= 0 {
		return errors.New("max tokens must be greater than 0")
	}
	if cfg.ContextFallbacks < 0 {
		return errors.New("context fallbacks must not be negative")
	}
	if cfg.ContextFallbacks > 0 && cfg.MinImageDimension <= 0 {
		return errors.New("minimum image dimension must be greater than 0 when context fallbacks are enabled")
	}
	switch cfg.PPTRenderer {
	case "", "auto", "libreoffice", "keynote":
	default:
		return errors.New("ppt renderer must be auto, libreoffice, or keynote")
	}
	if _, err := credentials.NormalizeStore(cfg.CredentialStore); err != nil {
		return err
	}
	if !requireVLM {
		return nil
	}
	if cfg.Provider == "" {
		return errors.New("provider is required")
	}
	if cfg.BaseURL == "" {
		return errors.New("base URL is required")
	}
	if cfg.Model == "" {
		return errors.New("model is required")
	}
	return nil
}

func EnvMap() map[string]string {
	env := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func configCandidates(explicit string) []string {
	if explicit != "" {
		return []string{explicit}
	}
	var candidates []string
	candidates = append(candidates, DefaultConfigPath())
	if cwd, err := os.Getwd(); err == nil {
		for {
			candidates = append(candidates, filepath.Join(cwd, ".doc7.yaml"))
			parent := filepath.Dir(cwd)
			if parent == cwd {
				break
			}
			cwd = parent
		}
	}
	return candidates
}

func DefaultConfigPath() string {
	return filepath.Join(configHome(), "doc7", "config.yaml")
}

// EffectivePath returns the configuration file that Load will use first.
// It also returns the default user path before the file exists, so callers can
// show users exactly where a future config update will be written.
func EffectivePath(explicit string) string {
	for _, candidate := range configCandidates(explicit) {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if explicit != "" {
		return explicit
	}
	return DefaultConfigPath()
}

func SetUserValue(path string, key string, value string) (string, error) {
	path = EffectivePath(path)
	key = normalizeKey(key)
	if key == "lang" {
		key = "language"
	}
	if key == "" {
		return "", errors.New("configuration key is required")
	}
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	replacement := key + ": " + quoteConfigValue(value)
	replaced := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lineKey, _, ok := strings.Cut(trimmed, ":")
		if ok && normalizeKey(lineKey) == key {
			lines[index] = replacement
			replaced = true
			break
		}
	}
	if !replaced {
		for len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, replacement)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	data := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// SetUserValues validates and writes a group of user values as one file update.
// The caller owns semantic validation; this function preserves unrelated lines
// and replaces each selected key in one complete file write.
func SetUserValues(path string, values map[string]string) (string, error) {
	path = EffectivePath(path)
	if len(values) == 0 {
		return "", errors.New("configuration values are required")
	}
	normalizedValues := make(map[string]string, len(values))
	for key, value := range values {
		key = normalizeKey(key)
		if key == "lang" {
			key = "language"
		}
		if key == "" {
			return "", errors.New("configuration key is required")
		}
		normalizedValues[key] = value
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	lines := []string{}
	if len(data) > 0 {
		lines = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	}
	replaced := make(map[string]bool, len(normalizedValues))
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lineKey, _, ok := strings.Cut(trimmed, ":")
		key := normalizeKey(lineKey)
		value, exists := normalizedValues[key]
		if ok && exists && !replaced[key] {
			lines[index] = key + ": " + quoteConfigValue(value)
			replaced[key] = true
		}
	}
	for key, value := range normalizedValues {
		if replaced[key] {
			continue
		}
		for len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, key+": "+quoteConfigValue(value))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func RemoveUserConfig(path string) (string, error) {
	path = EffectivePath(path)
	if err := os.Remove(path); err != nil {
		return path, err
	}
	return path, nil
}

func quoteConfigValue(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func configHome() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return xdg
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config")
	}
	return "."
}

func readSimpleYAML(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = normalizeKey(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		values[key] = value
	}
	return values, scanner.Err()
}

func applyMap(cfg *AppConfig, values map[string]string) {
	for key, value := range values {
		switch normalizeKey(key) {
		case "provider":
			cfg.Provider = value
		case "language", "lang":
			cfg.Language = value
		case "base_url", "baseurl":
			cfg.BaseURL = value
		case "model":
			cfg.Model = value
		case "api_key_env", "apikeyenv":
			cfg.APIKeyEnv = value
		case "credential_store", "credentialstore":
			cfg.CredentialStore = value
		case "credential_account", "credentialaccount":
			cfg.CredentialAccount = value
		case "credentials_path", "credentialspath":
			cfg.CredentialsPath = value
		case "remote_confirmed", "remoteconfirmed":
			cfg.RemoteConfirmed = parseBool(value, cfg.RemoteConfirmed)
		case "ppt_renderer", "pptrenderer", "presentation_renderer", "presentationrenderer":
			cfg.PPTRenderer = value
		case "file_workers", "fileworkers":
			cfg.FileWorkers = parseInt(value, cfg.FileWorkers)
		case "workers":
			cfg.Workers = parseInt(value, cfg.Workers)
		case "dpi":
			cfg.DPI = parseInt(value, cfg.DPI)
		case "image_detail", "imagedetail":
			cfg.ImageDetail = value
		case "max_image_mb", "maximagemb":
			cfg.MaxImageMB = parseInt(value, cfg.MaxImageMB)
		case "max_tokens", "maxtokens":
			cfg.MaxTokens = parseInt(value, cfg.MaxTokens)
		case "context_fallbacks", "contextfallbacks":
			cfg.ContextFallbacks = parseInt(value, cfg.ContextFallbacks)
		case "min_image_dimension", "minimagedimension":
			cfg.MinImageDimension = parseInt(value, cfg.MinImageDimension)
		case "timeout_seconds", "timeoutseconds":
			cfg.TimeoutSeconds = parseInt(value, cfg.TimeoutSeconds)
		case "retry_count", "retrycount":
			cfg.RetryCount = parseInt(value, cfg.RetryCount)
		case "prompt":
			cfg.PromptName = value
		}
	}
}

func applyEnv(cfg *AppConfig, env map[string]string) {
	if value := env["DOC7_LANG"]; value != "" {
		cfg.Language = value
	}
	if value := env["DOC7_PROVIDER"]; value != "" {
		cfg.Provider = value
	}
	if value := env["DOC7_BASE_URL"]; value != "" {
		cfg.BaseURL = value
	}
	if value := env["DOC7_MODEL"]; value != "" {
		cfg.Model = value
	}
	if value := env["DOC7_API_KEY_ENV"]; value != "" {
		cfg.APIKeyEnv = value
	}
	if value := env["DOC7_CREDENTIAL_STORE"]; value != "" {
		cfg.CredentialStore = value
	}
	if value := env["DOC7_CREDENTIAL_ACCOUNT"]; value != "" {
		cfg.CredentialAccount = value
	}
	if value := env["DOC7_CREDENTIALS_PATH"]; value != "" {
		cfg.CredentialsPath = value
	}
	if value := env["DOC7_REMOTE_CONFIRMED"]; value != "" {
		cfg.RemoteConfirmed = parseBool(value, cfg.RemoteConfirmed)
	}
	if value := env["DOC7_PPT_RENDERER"]; value != "" {
		cfg.PPTRenderer = value
	}
	if value := env["DOC7_FILE_WORKERS"]; value != "" {
		cfg.FileWorkers = parseInt(value, cfg.FileWorkers)
	}
	if value := env["DOC7_WORKERS"]; value != "" {
		cfg.Workers = parseInt(value, cfg.Workers)
	}
	if value := env["DOC7_DPI"]; value != "" {
		cfg.DPI = parseInt(value, cfg.DPI)
	}
	if value := env["DOC7_IMAGE_DETAIL"]; value != "" {
		cfg.ImageDetail = value
	}
	if value := env["DOC7_MAX_IMAGE_MB"]; value != "" {
		cfg.MaxImageMB = parseInt(value, cfg.MaxImageMB)
	}
	if value := env["DOC7_MAX_TOKENS"]; value != "" {
		cfg.MaxTokens = parseInt(value, cfg.MaxTokens)
	}
	if value := env["DOC7_CONTEXT_FALLBACKS"]; value != "" {
		cfg.ContextFallbacks = parseInt(value, cfg.ContextFallbacks)
	}
	if value := env["DOC7_MIN_IMAGE_DIMENSION"]; value != "" {
		cfg.MinImageDimension = parseInt(value, cfg.MinImageDimension)
	}
	if value := env["DOC7_TIMEOUT_SECONDS"]; value != "" {
		cfg.TimeoutSeconds = parseInt(value, cfg.TimeoutSeconds)
	}
	if value := env["DOC7_RETRY_COUNT"]; value != "" {
		cfg.RetryCount = parseInt(value, cfg.RetryCount)
	}
	if value := env["DOC7_PROMPT"]; value != "" {
		cfg.PromptName = value
	}
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func parseBool(value string, fallback bool) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}
