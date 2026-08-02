package credentials

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	StoreAuto     = "auto"
	StoreEnv      = "env"
	StoreKeychain = "keychain"
	StoreFile     = "file"

	DefaultService = "doc7"
)

type Options struct {
	Store       string
	Account     string
	Path        string
	APIKeyEnv   string
	Environment map[string]string
}

type Result struct {
	Key    string
	Source string
}

func Resolve(opts Options) (Result, error) {
	if result := resolveFromEnv(opts); result.Key != "" {
		return result, nil
	}

	store, err := normalizeStore(opts.Store)
	if err != nil {
		return Result{}, err
	}
	switch store {
	case StoreEnv:
		return Result{}, nil
	case StoreKeychain:
		return findKeychainPassword(accountOrDefault(opts.Account))
	case StoreFile:
		return readFileCredential(pathOrDefault(opts.Path))
	case StoreAuto:
		if keychainPreferred() {
			result, err := findKeychainPassword(accountOrDefault(opts.Account))
			if err != nil {
				return Result{}, err
			}
			if result.Key != "" {
				return result, nil
			}
		}
		return readFileCredential(pathOrDefault(opts.Path))
	default:
		return Result{}, nil
	}
}

func Store(opts Options, apiKey string) (string, error) {
	store, err := normalizeStore(opts.Store)
	if err != nil {
		return "", err
	}
	if store == StoreAuto {
		if keychainPreferred() {
			store = StoreKeychain
		} else {
			store = StoreFile
		}
	}
	switch store {
	case StoreEnv:
		if strings.TrimSpace(apiKey) != "" {
			return "", errors.New("env credential store does not persist API keys; use keychain or file")
		}
		return sourceForEnv(envNameOrDefault(opts.APIKeyEnv)), nil
	case StoreKeychain:
		if strings.TrimSpace(apiKey) == "" {
			return "", errors.New("API key is required for keychain credential store")
		}
		return storeKeychainPassword(accountOrDefault(opts.Account), strings.TrimSpace(apiKey))
	case StoreFile:
		if strings.TrimSpace(apiKey) == "" {
			return "", errors.New("API key is required for file credential store")
		}
		return writeFileCredential(pathOrDefault(opts.Path), strings.TrimSpace(apiKey))
	default:
		return "", errors.New("unsupported credential store: " + store)
	}
}

func NormalizeStore(value string) (string, error) {
	return normalizeStore(value)
}

func DefaultCredentialsPath() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "doc7", "credentials")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "doc7", "credentials")
	}
	return filepath.Join(".doc7", "credentials")
}

func EnvironmentKeys(preferred string) []string {
	// Never infer provider credentials for an arbitrary endpoint. A caller must
	// opt in to a provider-specific environment variable with APIKeyEnv.
	preferred = strings.TrimSpace(preferred)
	if preferred == "" {
		preferred = "DOC7_API_KEY"
	}
	keys := []string{preferred}
	seen := map[string]bool{}
	result := []string{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, key)
	}
	return result
}

func resolveFromEnv(opts Options) Result {
	for _, key := range EnvironmentKeys(opts.APIKeyEnv) {
		if value := opts.Environment[key]; value != "" {
			return Result{Key: value, Source: sourceForEnv(key)}
		}
	}
	return Result{}
}

func readFileCredential(path string) (Result, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Result{}, nil
	}
	if err != nil {
		return Result{}, err
	}
	key := parseCredentialFile(string(data))
	if key == "" {
		return Result{}, nil
	}
	return Result{Key: key, Source: "file:" + path}, nil
}

func writeFileCredential(path string, apiKey string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	data := []byte("api_key: " + quoteYAML(apiKey) + "\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	return "file:" + path, nil
}

func parseCredentialFile(raw string) string {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if key, value, ok := strings.Cut(line, ":"); ok && normalizeKey(key) == "api_key" {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
		return line
	}
	return ""
}

func normalizeStore(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", StoreAuto:
		return StoreAuto, nil
	case StoreEnv:
		return StoreEnv, nil
	case StoreKeychain:
		return StoreKeychain, nil
	case StoreFile:
		return StoreFile, nil
	default:
		return "", errors.New("credential store must be auto, env, keychain, or file")
	}
}

func accountOrDefault(value string) string {
	if strings.TrimSpace(value) == "" {
		return "default"
	}
	return strings.TrimSpace(value)
}

func envNameOrDefault(value string) string {
	if strings.TrimSpace(value) == "" {
		return "DOC7_API_KEY"
	}
	return strings.TrimSpace(value)
}

func pathOrDefault(value string) string {
	if strings.TrimSpace(value) == "" {
		return DefaultCredentialsPath()
	}
	return strings.TrimSpace(value)
}

func sourceForEnv(key string) string {
	return "env:" + key
}

func quoteYAML(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}
