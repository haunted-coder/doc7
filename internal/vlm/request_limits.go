package vlm

import (
	"image"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var imageLimitPattern = regexp.MustCompile(`(?i)([0-9]+)\s*bytes`)

func contextFallbackEligible(configuredLimit int, usage Usage) bool {
	if usage.CompletionTokens <= 0 {
		return false
	}
	return configuredLimit <= 0 || usage.CompletionTokens < configuredLimit
}

func providerContextLimitExceeded(body []byte) bool {
	message := strings.ToLower(string(body))
	if strings.Contains(message, "prompt is too long") || strings.Contains(message, "too many tokens") {
		return true
	}
	if !strings.Contains(message, "context") || !strings.Contains(message, "token") {
		return false
	}
	return strings.Contains(message, "exceed") ||
		strings.Contains(message, "too long") ||
		strings.Contains(message, "maximum") ||
		strings.Contains(message, "limit") ||
		strings.Contains(message, "available")
}

func nextContextImageDimension(path string, current int, minimum int) (int, bool) {
	if minimum <= 0 {
		return 0, false
	}
	base := current
	if base <= 0 {
		file, err := os.Open(path)
		if err != nil {
			return 0, false
		}
		config, _, err := image.DecodeConfig(file)
		_ = file.Close()
		if err != nil {
			return 0, false
		}
		base = max(config.Width, config.Height)
	}
	if base <= minimum {
		return 0, false
	}
	next := base * 2 / 3
	if next < minimum {
		next = minimum
	}
	if next >= base {
		return 0, false
	}
	return next, true
}

func providerImageLimit(body []byte, currentLimit int64) (int64, bool) {
	message := string(body)
	lower := strings.ToLower(message)
	if !strings.Contains(lower, "image") || !strings.Contains(lower, "base64") || !strings.Contains(lower, "exceeds") {
		return 0, false
	}
	matches := imageLimitPattern.FindAllStringSubmatch(message, -1)
	if len(matches) < 2 {
		return 0, false
	}
	providerLimit, err := strconv.ParseInt(matches[len(matches)-1][1], 10, 64)
	if err != nil || providerLimit <= 0 {
		return 0, false
	}
	// Base64 expands the raw image by roughly one third. Additional headroom
	// covers padding and providers that include surrounding request data.
	rawLimit := providerLimit * 7 / 10
	if rawLimit <= 0 || (currentLimit > 0 && rawLimit >= currentLimit) {
		return 0, false
	}
	return rawLimit, true
}
